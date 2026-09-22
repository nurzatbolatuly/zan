package billing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
)

// fakeCatalog — минимальная реализация billing.CatalogReader для юнит-тестов,
// без БД (BACKEND_CODING_STANDARDS.md §10).
type fakeCatalog struct {
	services map[string]domain.Service
	tariffs  map[string]domain.Tariff
}

func newFakeCatalog() *fakeCatalog {
	return &fakeCatalog{
		services: map[string]domain.Service{
			"qa":  {ID: "qa", PriceTenge: 2900, IsActive: true},
			"doc": {ID: "doc", PriceTenge: 4900, IsActive: true},
		},
		tariffs: make(map[string]domain.Tariff),
	}
}

func (c *fakeCatalog) ListAllServices(context.Context) ([]domain.Service, error) {
	out := make([]domain.Service, 0, len(c.services))
	for _, s := range c.services {
		out = append(out, s)
	}
	return out, nil
}

func (c *fakeCatalog) GetTariffByID(_ context.Context, id string) (domain.Tariff, error) {
	t, ok := c.tariffs[id]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	return t, nil
}

func (c *fakeCatalog) ValidateItems(_ context.Context, items []domain.BundleItem) error {
	if len(items) == 0 {
		return catalog.ErrInvalidItems
	}
	for _, item := range items {
		if item.Qty < 1 || item.Qty > 20 {
			return catalog.ErrInvalidItems
		}
		if _, ok := c.services[item.ServiceID]; !ok {
			return catalog.ErrInvalidItems
		}
	}
	return nil
}

// fakeRepo — in-memory billing.Repository. DebitCredit/ConfirmPayment не
// нуждаются в реальной SELECT...FOR UPDATE семантике для этих тестов —
// конкурентность и атомарность на реальном Postgres проверяет
// internal/repo/billing_repo_test.go (testcontainers).
type fakeRepo struct {
	credits  map[string]int // sessionID+"|"+serviceID -> quantity
	payments map[string]domain.Payment
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{credits: make(map[string]int), payments: make(map[string]domain.Payment)}
}

func creditKey(sessionID, serviceID string) string { return sessionID + "|" + serviceID }

func (r *fakeRepo) GetBalance(_ context.Context, sessionID string) ([]domain.UserCredit, error) {
	out := make([]domain.UserCredit, 0)
	for _, serviceID := range []string{"qa", "doc"} {
		out = append(out, domain.UserCredit{ServiceID: serviceID, Quantity: r.credits[creditKey(sessionID, serviceID)]})
	}
	return out, nil
}

func (r *fakeRepo) CreatePayment(_ context.Context, p domain.Payment) (domain.Payment, error) {
	r.payments[p.ID] = p
	return p, nil
}

func (r *fakeRepo) GetPaymentByID(_ context.Context, id string) (domain.Payment, error) {
	p, ok := r.payments[id]
	if !ok {
		return domain.Payment{}, billing.ErrPaymentNotFound
	}
	return p, nil
}

func (r *fakeRepo) ConfirmPayment(_ context.Context, id string, now time.Time) (domain.Payment, bool, error) {
	p, ok := r.payments[id]
	if !ok {
		return domain.Payment{}, false, billing.ErrPaymentNotFound
	}
	if p.Status != domain.PaymentStatusPending {
		return p, false, nil
	}
	p.Status = domain.PaymentStatusSuccess
	p.PaidAt = &now
	r.payments[id] = p
	for _, item := range p.Items {
		r.credits[creditKey(p.SessionID, item.ServiceID)] += item.Qty
	}
	return p, true, nil
}

func (r *fakeRepo) FailStalePending(_ context.Context, olderThan time.Time) (int, error) {
	n := 0
	for id, p := range r.payments {
		if p.Status == domain.PaymentStatusPending && p.CreatedAt.Before(olderThan) {
			p.Status = domain.PaymentStatusFailed
			r.payments[id] = p
			n++
		}
	}
	return n, nil
}

func (r *fakeRepo) DebitCredit(_ context.Context, sessionID, serviceID string) (bool, error) {
	key := creditKey(sessionID, serviceID)
	if r.credits[key] < 1 {
		return false, nil
	}
	r.credits[key]--
	return true, nil
}

func (r *fakeRepo) CreditBalance(_ context.Context, sessionID, serviceID string, qty int) error {
	r.credits[creditKey(sessionID, serviceID)] += qty
	return nil
}

const (
	testSessionID = "3fa85f64-5717-4562-b3fc-2c963f66afa6"
	testPaymentID = "4fa85f64-5717-4562-b3fc-2c963f66afa7"
)

func newTestService(repo *fakeRepo, cat *fakeCatalog, now time.Time) *billing.Service {
	return billing.New(repo, cat, clock.Fake{T: now}, &idgen.Fake{IDs: []string{testPaymentID}})
}

func TestCheckout_Custom_ComputesAmountWithoutDiscount(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())

	p, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindCustom,
		Items:     []domain.BundleItem{{ServiceID: "qa", Qty: 2}, {ServiceID: "doc", Qty: 1}},
	})

	require.NoError(t, err)
	require.Equal(t, testPaymentID, p.ID)
	require.Equal(t, domain.PaymentStatusPending, p.Status)
	require.Equal(t, 10700, p.AmountTenge) // 2*2900 + 1*4900, без скидки (v2 §4.1)
}

func TestCheckout_Tariff_CopiesItemsAndAppliesDiscount(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	cat.tariffs["b2"] = domain.Tariff{
		ID: "b2", DiscountPercent: 15, IsActive: true,
		Items: []domain.BundleItem{{ServiceID: "qa", Qty: 3}},
	}
	svc := newTestService(repo, cat, time.Now())
	tariffID := "b2"

	p, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindTariff,
		TariffID:  &tariffID,
	})

	require.NoError(t, err)
	require.Equal(t, 7400, p.AmountTenge) // 8700*0.85 -> round to nearest 10
	require.Equal(t, []domain.BundleItem{{ServiceID: "qa", Qty: 3}}, p.Items)
}

func TestCheckout_Tariff_RejectsInactiveTariff(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	cat.tariffs["archived"] = domain.Tariff{ID: "archived", IsActive: false}
	svc := newTestService(repo, cat, time.Now())
	tariffID := "archived"

	_, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindTariff,
		TariffID:  &tariffID,
	})

	require.ErrorIs(t, err, billing.ErrTariffInactive)
}

func TestCheckout_SingleService_RejectsMultipleItems(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())

	_, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindSingleService,
		Items:     []domain.BundleItem{{ServiceID: "qa", Qty: 1}, {ServiceID: "doc", Qty: 1}},
	})

	require.ErrorIs(t, err, billing.ErrInvalidCheckout)
}

func TestConfirmPayment_CreditsBalanceOnce(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())

	created, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindCustom,
		Items:     []domain.BundleItem{{ServiceID: "qa", Qty: 2}},
	})
	require.NoError(t, err)

	confirmed, err := svc.ConfirmPayment(context.Background(), created.ID, testSessionID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusSuccess, confirmed.Status)

	balance, err := svc.GetBalance(context.Background(), testSessionID)
	require.NoError(t, err)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 2})

	// Повторный confirm — идемпотентно, баланс не растёт (zan-backend-tz-v3.md §5.1).
	again, err := svc.ConfirmPayment(context.Background(), created.ID, testSessionID)
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusSuccess, again.Status)

	balance, err = svc.GetBalance(context.Background(), testSessionID)
	require.NoError(t, err)
	require.Contains(t, balance, domain.UserCredit{ServiceID: "qa", Quantity: 2})
}

func TestConfirmPayment_RejectsAlreadyFailed(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())

	created, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindCustom,
		Items:     []domain.BundleItem{{ServiceID: "qa", Qty: 1}},
	})
	require.NoError(t, err)

	p := repo.payments[created.ID]
	p.Status = domain.PaymentStatusFailed
	repo.payments[created.ID] = p

	_, err = svc.ConfirmPayment(context.Background(), created.ID, testSessionID)
	require.ErrorIs(t, err, billing.ErrPaymentNotPending)
}

func TestConfirmPayment_RejectsForeignSession(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())

	created, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindCustom,
		Items:     []domain.BundleItem{{ServiceID: "qa", Qty: 1}},
	})
	require.NoError(t, err)

	_, err = svc.ConfirmPayment(context.Background(), created.ID, "other-session")
	require.ErrorIs(t, err, billing.ErrPaymentNotFound)
}

func TestDebitCredit_InsufficientBalance(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())

	err := svc.DebitCredit(context.Background(), testSessionID, "qa")

	require.ErrorIs(t, err, billing.ErrInsufficientBalance)
}

func TestDebitCredit_Success(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	svc := newTestService(repo, cat, time.Now())
	require.NoError(t, repo.CreditBalance(context.Background(), testSessionID, "qa", 1))

	err := svc.DebitCredit(context.Background(), testSessionID, "qa")
	require.NoError(t, err)

	err = svc.DebitCredit(context.Background(), testSessionID, "qa")
	require.ErrorIs(t, err, billing.ErrInsufficientBalance)
}

func TestExpireStalePending(t *testing.T) {
	repo, cat := newFakeRepo(), newFakeCatalog()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	svc := newTestService(repo, cat, now)

	old, err := svc.Checkout(context.Background(), billing.CheckoutRequest{
		SessionID: testSessionID,
		Kind:      domain.PaymentKindCustom,
		Items:     []domain.BundleItem{{ServiceID: "qa", Qty: 1}},
	})
	require.NoError(t, err)

	svcLater := newTestService(repo, cat, now.Add(billing.StalePendingAfter+time.Minute))
	n, err := svcLater.ExpireStalePending(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, n)

	failed := repo.payments[old.ID]
	require.Equal(t, domain.PaymentStatusFailed, failed.Status)
}
