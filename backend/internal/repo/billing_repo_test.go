package repo_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/billing"
)

// Платежи/баланс ссылаются на core.sessions (FK) — каждому тесту этого
// файла нужна реальная сессия, заводим её через newTestSession/SessionRepo
// (internal/repo/session_repo_test.go, тот же пакет repo_test).

func TestBillingRepo_GetBalance_AllActiveServicesDefaultZero(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-1111-1111-1111-111111111111", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	billingRepo := repo.NewBillingRepo(pool, &idgen.Fake{IDs: []string{"22222222-2222-2222-2222-222222222222"}})

	balance, err := billingRepo.GetBalance(context.Background(), sess.ID)

	require.NoError(t, err)
	require.Len(t, balance, 2) // qa/doc, оба 0 (000003_stage2_billing.up.sql)
	for _, c := range balance {
		require.Equal(t, 0, c.Quantity)
	}
}

func TestBillingRepo_CreateAndGetPayment(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("33333333-3333-3333-3333-333333333333", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	billingRepo := repo.NewBillingRepo(pool, &idgen.Fake{})
	now := time.Now().UTC().Truncate(time.Microsecond)

	payment := domain.Payment{
		ID:          "44444444-4444-4444-4444-444444444444",
		SessionID:   sess.ID,
		Kind:        domain.PaymentKindCustom,
		Items:       []domain.BundleItem{{ServiceID: "qa", Qty: 2}, {ServiceID: "doc", Qty: 1}},
		AmountTenge: 10700,
		Status:      domain.PaymentStatusPending,
		Provider:    "mock",
		CreatedAt:   now,
	}
	created, err := billingRepo.CreatePayment(context.Background(), payment)
	require.NoError(t, err)
	require.Equal(t, payment.ID, created.ID)

	got, err := billingRepo.GetPaymentByID(context.Background(), payment.ID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusPending, got.Status)
	require.Equal(t, payment.Items, got.Items)
	require.Equal(t, 10700, got.AmountTenge)
	require.Nil(t, got.PaidAt)

	_, err = billingRepo.GetPaymentByID(context.Background(), "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, billing.ErrPaymentNotFound)
}

func TestBillingRepo_ConfirmPayment_CreditsBalanceAndIsIdempotent(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("55555555-5555-5555-5555-555555555555", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	billingRepo := repo.NewBillingRepo(pool, &idgen.Fake{
		IDs: []string{
			"66666666-6666-6666-6666-666666666666", // user_credits row id (первый confirm)
		},
	})
	ctx := context.Background()

	payment := domain.Payment{
		ID:          "77777777-7777-7777-7777-777777777777",
		SessionID:   sess.ID,
		Kind:        domain.PaymentKindCustom,
		Items:       []domain.BundleItem{{ServiceID: "qa", Qty: 3}},
		AmountTenge: 8700,
		Status:      domain.PaymentStatusPending,
		Provider:    "mock",
		CreatedAt:   time.Now().UTC(),
	}
	_, err := billingRepo.CreatePayment(ctx, payment)
	require.NoError(t, err)

	confirmed, applied, err := billingRepo.ConfirmPayment(ctx, payment.ID, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, applied)
	require.Equal(t, domain.PaymentStatusSuccess, confirmed.Status)
	require.NotNil(t, confirmed.PaidAt)

	balance, err := billingRepo.GetBalance(ctx, sess.ID)
	require.NoError(t, err)
	qaCredit := findCredit(balance, "qa")
	require.Equal(t, 3, qaCredit.Quantity)

	// Повторный confirm — applied=false, баланс не растёт (idempotent).
	again, applied, err := billingRepo.ConfirmPayment(ctx, payment.ID, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, applied)
	require.Equal(t, domain.PaymentStatusSuccess, again.Status)

	balance, err = billingRepo.GetBalance(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, 3, findCredit(balance, "qa").Quantity)
}

func TestBillingRepo_FailStalePending(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("88888888-8888-8888-8888-888888888888", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	billingRepo := repo.NewBillingRepo(pool, &idgen.Fake{})
	ctx := context.Background()

	old := time.Now().UTC().Add(-time.Hour)
	payment := domain.Payment{
		ID:          "99999999-9999-9999-9999-999999999999",
		SessionID:   sess.ID,
		Kind:        domain.PaymentKindCustom,
		Items:       []domain.BundleItem{{ServiceID: "qa", Qty: 1}},
		AmountTenge: 2900,
		Status:      domain.PaymentStatusPending,
		Provider:    "mock",
		CreatedAt:   old,
	}
	_, err := billingRepo.CreatePayment(ctx, payment)
	require.NoError(t, err)

	n, err := billingRepo.FailStalePending(ctx, time.Now().UTC().Add(-billing.StalePendingAfter))
	require.NoError(t, err)
	require.Equal(t, 1, n)

	got, err := billingRepo.GetPaymentByID(ctx, payment.ID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusFailed, got.Status)
}

// TestBillingRepo_DebitCredit_ConcurrentDebitsLastUnit — BACKEND_PLAN.md
// Stage 2 DoD: "конкурентный тест на списание последней единицы баланса
// (два параллельных запроса — ровно один списывает)". Проверяет реальную
// SELECT ... FOR UPDATE-семантику на настоящем Postgres, не мок
// (zan-backend-tz-v3.md §5.5).
func TestBillingRepo_DebitCredit_ConcurrentDebitsLastUnit(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	billingRepo := repo.NewBillingRepo(pool, &idgen.Fake{
		IDs: []string{"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"},
	})
	ctx := context.Background()
	require.NoError(t, billingRepo.CreditBalance(ctx, sess.ID, "qa", 1))

	const attempts = 8
	results := make([]bool, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ok, err := billingRepo.DebitCredit(ctx, sess.ID, "qa")
			require.NoError(t, err)
			results[i] = ok
		}(i)
	}
	wg.Wait()

	succeeded := 0
	for _, ok := range results {
		if ok {
			succeeded++
		}
	}
	require.Equal(t, 1, succeeded, "ровно один из %d параллельных запросов должен списать последнюю единицу", attempts)

	balance, err := billingRepo.GetBalance(ctx, sess.ID)
	require.NoError(t, err)
	require.Equal(t, 0, findCredit(balance, "qa").Quantity)
}

func findCredit(balance []domain.UserCredit, serviceID string) domain.UserCredit {
	for _, c := range balance {
		if c.ServiceID == serviceID {
			return c
		}
	}
	return domain.UserCredit{}
}
