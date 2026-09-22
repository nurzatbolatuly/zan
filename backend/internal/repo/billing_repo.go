package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/billing"
)

// BillingRepo — реализация billing.Repository (Stage 2). ConfirmPayment/
// DebitCredit — единственные два места во всём backend/, где открывается
// явная транзакция (BeginTx): атомарность списания/начисления
// (SELECT ... FOR UPDATE — zan-backend-tz-v2.md §4.2) требует нескольких
// запросов под одной блокировкой.
type BillingRepo struct {
	db *pgxpool.Pool
	// idgen — нужен только для id новой строки core.user_credits в
	// UPSERT (upsertUserCredit): у domain.UserCredit нет собственного ID
	// (баланс идентифицируется парой session_id+service_id, PK строки —
	// деталь хранения, не бизнес-значение), поэтому его генерирует сам
	// репозиторий, а не вызывающий service-слой.
	idgen idgen.Generator
}

// NewBillingRepo строит BillingRepo поверх общего пула соединений.
func NewBillingRepo(db *pgxpool.Pool, ids idgen.Generator) *BillingRepo {
	return &BillingRepo{db: db, idgen: ids}
}

func (r *BillingRepo) GetBalance(ctx context.Context, sessionID string) ([]domain.UserCredit, error) {
	pgSessionID, err := parseUUID(sessionID)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.Query(ctx, `
		SELECT s.id AS service_id, COALESCE(uc.quantity, 0)::int AS quantity
		FROM core.services s
		LEFT JOIN core.user_credits uc ON uc.service_id = s.id AND uc.session_id = $1
		WHERE s.is_active = true
		ORDER BY s.id
	`, pgSessionID)
	if err != nil {
		return nil, fmt.Errorf("repo: get balance: %w", err)
	}
	defer rows.Close()

	var credits []domain.UserCredit
	for rows.Next() {
		var serviceID string
		var quantity int32
		if err := rows.Scan(&serviceID, &quantity); err != nil {
			return nil, fmt.Errorf("repo: get balance: scan: %w", err)
		}
		credits = append(credits, domain.UserCredit{ServiceID: serviceID, Quantity: int(quantity)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: get balance: %w", err)
	}
	return credits, nil
}

func (r *BillingRepo) CreatePayment(ctx context.Context, p domain.Payment) (domain.Payment, error) {
	id, err := parseUUID(p.ID)
	if err != nil {
		return domain.Payment{}, err
	}
	sessionID, err := parseUUID(p.SessionID)
	if err != nil {
		return domain.Payment{}, err
	}
	tariffID, err := optionalUUID(p.TariffID)
	if err != nil {
		return domain.Payment{}, err
	}
	threadID, err := optionalUUID(p.ThreadID)
	if err != nil {
		return domain.Payment{}, err
	}
	items, err := json.Marshal(p.Items)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("repo: marshal payment items: %w", err)
	}
	amount, err := toInt32(p.AmountTenge)
	if err != nil {
		return domain.Payment{}, err
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO core.payments (id, session_id, kind, tariff_id, items, amount, status, provider, created_at, thread_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, id, sessionID, string(p.Kind), tariffID, items, amount, string(p.Status), p.Provider, toTimestamptz(p.CreatedAt), threadID)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("repo: create payment: %w", err)
	}
	return p, nil
}

func (r *BillingRepo) GetPaymentByID(ctx context.Context, id string) (domain.Payment, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Payment{}, billing.ErrPaymentNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, session_id, kind, tariff_id, items, amount, status, provider, created_at, paid_at, thread_id
		FROM core.payments
		WHERE id = $1
	`, pgID)

	var pr paymentRow
	if err := pr.scan(row); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Payment{}, billing.ErrPaymentNotFound
		}
		return domain.Payment{}, fmt.Errorf("repo: get payment: %w", err)
	}
	return pr.toDomain()
}

// ConfirmPayment — см. billing.Repository.ConfirmPayment. Транзакция:
// перевод pending->success (условный UPDATE, переход только из pending) +
// начисление всех Items на баланс — либо всё вместе, либо ничего.
func (r *BillingRepo) ConfirmPayment(ctx context.Context, id string, now time.Time) (domain.Payment, bool, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Payment{}, false, billing.ErrPaymentNotFound
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Payment{}, false, fmt.Errorf("repo: begin confirm payment: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	markRow := tx.QueryRow(ctx, `
		UPDATE core.payments
		SET status = 'success', paid_at = $2
		WHERE id = $1 AND status = 'pending'
		RETURNING id, session_id, kind, tariff_id, items, amount, status, provider, created_at, paid_at, thread_id
	`, pgID, toTimestamptz(now))

	var pr paymentRow
	if err := pr.scan(markRow); err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return domain.Payment{}, false, fmt.Errorf("repo: mark payment success: %w", err)
		}
		// 0 обновлённых строк — платёж уже не pending (success/failed).
		// Ничего не меняем (транзакция откатывается в defer без записей),
		// отдаём текущее состояние как есть — вызывающий Service решает,
		// идемпотентность это или ошибка (billing.Service.ConfirmPayment).
		currentRow := tx.QueryRow(ctx, `
			SELECT id, session_id, kind, tariff_id, items, amount, status, provider, created_at, paid_at, thread_id
			FROM core.payments
			WHERE id = $1
		`, pgID)
		var current paymentRow
		if err := current.scan(currentRow); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.Payment{}, false, billing.ErrPaymentNotFound
			}
			return domain.Payment{}, false, fmt.Errorf("repo: get payment after no-op confirm: %w", err)
		}
		p, pErr := current.toDomain()
		return p, false, pErr
	}

	var items []domain.BundleItem
	if err := json.Unmarshal(pr.items, &items); err != nil {
		return domain.Payment{}, false, fmt.Errorf("repo: confirm payment: unmarshal items: %w", err)
	}
	for _, item := range items {
		creditID, err := parseUUID(r.idgen.NewID())
		if err != nil {
			return domain.Payment{}, false, err
		}
		qty, err := toInt32(item.Qty)
		if err != nil {
			return domain.Payment{}, false, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.user_credits (id, session_id, service_id, quantity, updated_at)
			VALUES ($1, $2, $3, $4, now())
			ON CONFLICT (session_id, service_id)
			DO UPDATE SET quantity = core.user_credits.quantity + EXCLUDED.quantity, updated_at = now()
		`, creditID, pr.sessionID, item.ServiceID, qty); err != nil {
			return domain.Payment{}, false, fmt.Errorf("repo: confirm payment: credit balance: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Payment{}, false, fmt.Errorf("repo: commit confirm payment: %w", err)
	}

	p, err := pr.toDomain()
	return p, true, err
}

func (r *BillingRepo) FailStalePending(ctx context.Context, olderThan time.Time) (int, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE core.payments
		SET status = 'failed'
		WHERE status = 'pending' AND created_at < $1
	`, toTimestamptz(olderThan))
	if err != nil {
		return 0, fmt.Errorf("repo: fail stale pending: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// DebitCredit — SELECT ... FOR UPDATE + условный decrement в одной
// транзакции (zan-backend-tz-v2.md §4.2, гонка описана в v3 §5.5).
func (r *BillingRepo) DebitCredit(ctx context.Context, sessionID, serviceID string) (bool, error) {
	pgSessionID, err := parseUUID(sessionID)
	if err != nil {
		return false, err
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("repo: begin debit credit: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	row := tx.QueryRow(ctx, `
		SELECT quantity FROM core.user_credits WHERE session_id = $1 AND service_id = $2 FOR UPDATE
	`, pgSessionID, serviceID)
	var qty int32
	if err := row.Scan(&qty); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // баланс никогда не заводился для этой услуги — 0 единиц.
		}
		return false, fmt.Errorf("repo: lock user credit: %w", err)
	}
	if qty < 1 {
		return false, nil // транзакция откатится в defer, ничего не менялось.
	}

	if _, err := tx.Exec(ctx, `
		UPDATE core.user_credits
		SET quantity = quantity - 1, updated_at = now()
		WHERE session_id = $1 AND service_id = $2
	`, pgSessionID, serviceID); err != nil {
		return false, fmt.Errorf("repo: decrement user credit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("repo: commit debit credit: %w", err)
	}
	return true, nil
}

func (r *BillingRepo) CreditBalance(ctx context.Context, sessionID, serviceID string, qty int) error {
	pgSessionID, err := parseUUID(sessionID)
	if err != nil {
		return err
	}
	creditID, err := parseUUID(r.idgen.NewID())
	if err != nil {
		return err
	}
	quantity, err := toInt32(qty)
	if err != nil {
		return err
	}

	// delta может быть положительным (начисление после confirm/возврат при
	// ошибке треда) — списание идёт отдельным путём (см. DebitCredit выше),
	// т.к. требует проверки "хватает ли" перед вычитанием, чего UPSERT
	// сделать не может.
	if _, err := r.db.Exec(ctx, `
		INSERT INTO core.user_credits (id, session_id, service_id, quantity, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (session_id, service_id)
		DO UPDATE SET quantity = core.user_credits.quantity + EXCLUDED.quantity, updated_at = now()
	`, creditID, pgSessionID, serviceID, quantity); err != nil {
		return fmt.Errorf("repo: credit balance: %w", err)
	}
	return nil
}

func optionalUUID(id *string) (pgtype.UUID, error) {
	if id == nil {
		return pgtype.UUID{}, nil
	}
	return parseUUID(*id)
}

// paymentRow — форма строки core.payments под Scan; не запрос, чистое
// хранилище отсканированных колонок (см. sessionRow). scan() принимает
// pgx.Row напрямую, чтобы не дублировать список полей в каждом вызывающем
// месте (GetPaymentByID/ConfirmPayment — оба SELECT с одинаковым списком
// колонок, разница только в WHERE/источнике строки — pool vs tx).
type paymentRow struct {
	id        pgtype.UUID
	sessionID pgtype.UUID
	kind      string
	tariffID  pgtype.UUID
	items     []byte
	amount    int32
	status    string
	provider  string
	createdAt pgtype.Timestamptz
	paidAt    pgtype.Timestamptz
	threadID  pgtype.UUID
}

func (pr *paymentRow) scan(row pgx.Row) error {
	return row.Scan(
		&pr.id, &pr.sessionID, &pr.kind, &pr.tariffID, &pr.items,
		&pr.amount, &pr.status, &pr.provider, &pr.createdAt, &pr.paidAt, &pr.threadID,
	)
}

func (pr paymentRow) toDomain() (domain.Payment, error) {
	kind, err := domain.ParsePaymentKind(pr.kind)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("repo: payment row: %w", err)
	}
	status, err := domain.ParsePaymentStatus(pr.status)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("repo: payment row: %w", err)
	}

	var items []domain.BundleItem
	if err := json.Unmarshal(pr.items, &items); err != nil {
		return domain.Payment{}, fmt.Errorf("repo: payment row: unmarshal items: %w", err)
	}

	var tariffID *string
	if pr.tariffID.Valid {
		id := fromPgUUID(pr.tariffID)
		tariffID = &id
	}
	var threadID *string
	if pr.threadID.Valid {
		id := fromPgUUID(pr.threadID)
		threadID = &id
	}
	var paidAt *time.Time
	if pr.paidAt.Valid {
		t := pr.paidAt.Time
		paidAt = &t
	}

	return domain.Payment{
		ID:          fromPgUUID(pr.id),
		SessionID:   fromPgUUID(pr.sessionID),
		Kind:        kind,
		TariffID:    tariffID,
		ThreadID:    threadID,
		Items:       items,
		AmountTenge: int(pr.amount),
		Status:      status,
		Provider:    pr.provider,
		CreatedAt:   pr.createdAt.Time,
		PaidAt:      paidAt,
	}, nil
}
