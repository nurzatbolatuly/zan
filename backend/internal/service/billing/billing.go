// Package billing — юзкейсы баланса и оплаты (GET /balance,
// POST /payments/checkout, POST /payments/{id}/confirm, GET /payments/{id}
// — zan-backend-tz-v2.md §3.4/§3.5/§4.2). Списание/начисление — под
// SELECT ... FOR UPDATE, гонка описана в zan-backend-tz-v3.md §5.5;
// реализация — internal/repo/billing_repo.go (единственное место, где это
// решается транзакцией, не переизобретается на вызывающей стороне).
package billing

import (
	"context"
	"errors"
	"fmt"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/pricing"
)

// StalePendingAfter — pending-платёж старше этого возраста считается
// "брошенным" фоновой задачей (zan-backend-tz-v3.md §5.1: "30 минут").
const StalePendingAfter = 30 * time.Minute

var (
	// ErrPaymentNotFound — платежа с таким id нет, либо он принадлежит
	// другой сессии (repo не различает эти два случая наружу — тот же
	// принцип, что и у session.ErrInvalidToken: не подсказывать
	// злоумышленнику, что чужой id вообще существует).
	ErrPaymentNotFound = errors.New("billing: payment not found")

	// ErrPaymentNotPending — confirm вызван для платежа, который уже не
	// pending. Для уже-success это не ошибка на уровне HTTP (идемпотентно
	// возвращается прежний результат — zan-backend-tz-v3.md §5.1), Service
	// разруливает это сам и наружу ErrPaymentNotPending не отдаёт; ошибка
	// долетает до вызывающего кода только для уже-failed платежа.
	ErrPaymentNotPending = errors.New("billing: payment is not pending")

	// ErrTariffInactive — checkout kind=tariff на архивный тариф.
	ErrTariffInactive = errors.New("billing: tariff is inactive")

	// ErrInsufficientBalance — DebitCredit: на балансе нет ни одной единицы услуги.
	ErrInsufficientBalance = errors.New("billing: insufficient balance")

	// ErrInvalidCheckout — форма запроса checkout не соответствует Kind
	// (zan-backend-tz-v2.md §3.5/§4.1): нет tariff_id при kind=tariff,
	// больше одной позиции при kind=single_service, неизвестный kind.
	// Состав/qty самих items проверяет CatalogReader.ValidateItems
	// (catalog.ErrInvalidItems) — эта ошибка про форму запроса, не про items.
	ErrInvalidCheckout = errors.New("billing: invalid checkout request")
)

// Repository — порт доступа к балансу и платежам.
type Repository interface {
	GetBalance(ctx context.Context, sessionID string) ([]domain.UserCredit, error)

	CreatePayment(ctx context.Context, p domain.Payment) (domain.Payment, error)
	GetPaymentByID(ctx context.Context, id string) (domain.Payment, error)
	// ConfirmPayment — атомарно: если платёж pending — переводит в
	// success, начисляет все Items на баланс сессии и возвращает
	// (updated, true, nil). Если платёж уже не pending — не трогает
	// ничего и возвращает (текущее состояние, false, nil): "успешно
	// разобрались, что применять нечего", не ошибка транспорта.
	ConfirmPayment(ctx context.Context, id string, now time.Time) (payment domain.Payment, applied bool, err error)
	// FailStalePending — помечает pending-платежи старше olderThan как
	// failed, возвращает сколько строк изменено (для лога).
	FailStalePending(ctx context.Context, olderThan time.Time) (int, error)

	// DebitCredit — атомарно списывает 1 единицу ServiceID с баланса
	// SessionID под SELECT ... FOR UPDATE. false — баланс уже был 0
	// (обычный сценарий, не ошибка репозитория).
	DebitCredit(ctx context.Context, sessionID, serviceID string) (bool, error)
	// CreditBalance — атомарно начисляет qty единиц ServiceID (UPSERT).
	// Используется и при confirm (внутри ConfirmPayment), и отдельно —
	// возврат кредита при ошибке треда (zan-backend-tz-v2.md §4.2 п.3,
	// реализуется вызывающей стороной в Stage 3).
	CreditBalance(ctx context.Context, sessionID, serviceID string, qty int) error
}

// CatalogReader — то немногое, что billing нужно от каталога, чтобы
// посчитать сумму checkout той же формулой, что и GET /tariffs
// (internal/service/pricing, BACKEND_PLAN.md §5). Порт объявлен здесь, а
// не в internal/service/catalog, где реализуется (*catalog.Service
// удовлетворяет ему без изменений — BACKEND_CODING_STANDARDS.md §1.1).
type CatalogReader interface {
	ListAllServices(ctx context.Context) ([]domain.Service, error)
	GetTariffByID(ctx context.Context, id string) (domain.Tariff, error)
	ValidateItems(ctx context.Context, items []domain.BundleItem) error
}

// Service — бизнес-логика баланса и платежей.
type Service struct {
	repo    Repository
	catalog CatalogReader
	clock   clock.Clock
	idgen   idgen.Generator
}

// New собирает Service с внедрёнными зависимостями.
func New(repo Repository, cat CatalogReader, clk clock.Clock, ids idgen.Generator) *Service {
	return &Service{repo: repo, catalog: cat, clock: clk, idgen: ids}
}

// GetBalance — GET /balance.
func (s *Service) GetBalance(ctx context.Context, sessionID string) ([]domain.UserCredit, error) {
	credits, err := s.repo.GetBalance(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("billing: get balance: %w", err)
	}
	return credits, nil
}

// CheckoutRequest — вход POST /payments/checkout (zan-backend-tz-v2.md §3.5).
// Items используется для kind=single_service/custom; для kind=tariff
// состав берётся из самого тарифа (TariffID) — переданный Items в этом
// случае игнорируется.
type CheckoutRequest struct {
	SessionID string
	Kind      domain.PaymentKind
	TariffID  *string
	Items     []domain.BundleItem
	ThreadID  *string
}

// Checkout — POST /payments/checkout: считает сумму по той же формуле,
// что и GET /tariffs (internal/service/pricing), создаёт Payment(pending).
func (s *Service) Checkout(ctx context.Context, req CheckoutRequest) (domain.Payment, error) {
	items, discountPercent, tariffID, err := s.resolveCheckoutItems(ctx, req)
	if err != nil {
		return domain.Payment{}, err
	}

	prices, err := s.servicePrices(ctx, items)
	if err != nil {
		return domain.Payment{}, err
	}
	amount := pricing.Compute(items, prices, discountPercent).TotalTenge

	payment := domain.Payment{
		ID:          s.idgen.NewID(),
		SessionID:   req.SessionID,
		Kind:        req.Kind,
		TariffID:    tariffID,
		ThreadID:    req.ThreadID,
		Items:       items,
		AmountTenge: amount,
		Status:      domain.PaymentStatusPending,
		Provider:    "mock",
		CreatedAt:   s.clock.Now(),
	}

	created, err := s.repo.CreatePayment(ctx, payment)
	if err != nil {
		return domain.Payment{}, fmt.Errorf("billing: checkout: %w", err)
	}
	return created, nil
}

// resolveCheckoutItems — items+discount для расчёта суммы, в зависимости
// от Kind (zan-backend-tz-v2.md §4.1).
func (s *Service) resolveCheckoutItems(ctx context.Context, req CheckoutRequest) (items []domain.BundleItem, discountPercent int, tariffID *string, err error) {
	switch req.Kind {
	case domain.PaymentKindTariff:
		if req.TariffID == nil {
			return nil, 0, nil, fmt.Errorf("%w: tariff_id is required for kind=tariff", ErrInvalidCheckout)
		}
		tariff, tErr := s.catalog.GetTariffByID(ctx, *req.TariffID)
		if tErr != nil {
			return nil, 0, nil, tErr
		}
		if !tariff.IsActive {
			return nil, 0, nil, ErrTariffInactive
		}
		// Копия items тарифа на момент покупки (zan-backend-tz-v2.md §2.5)
		// — если тариф позже изменится, уже созданные Payment это не тронет.
		return append([]domain.BundleItem(nil), tariff.Items...), tariff.DiscountPercent, req.TariffID, nil

	case domain.PaymentKindSingleService, domain.PaymentKindCustom:
		if err := s.catalog.ValidateItems(ctx, req.Items); err != nil {
			return nil, 0, nil, err
		}
		if req.Kind == domain.PaymentKindSingleService && len(req.Items) != 1 {
			return nil, 0, nil, fmt.Errorf("%w: single_service must have exactly one item", ErrInvalidCheckout)
		}
		// custom и single_service — без скидки (zan-backend-tz-v2.md §4.1).
		return req.Items, 0, nil, nil

	default:
		return nil, 0, nil, fmt.Errorf("%w: unknown kind %q", ErrInvalidCheckout, req.Kind)
	}
}

func (s *Service) servicePrices(ctx context.Context, items []domain.BundleItem) (map[string]int, error) {
	services, err := s.catalog.ListAllServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("billing: load service prices: %w", err)
	}
	prices := make(map[string]int, len(items))
	for _, svc := range services {
		prices[svc.ID] = svc.PriceTenge
	}
	return prices, nil
}

// GetPayment — GET /payments/{id}. ownerSessionID — вызывающая сессия;
// платёж чужой сессии отдаётся как ErrPaymentNotFound, не 403 (не палим
// сам факт существования чужого payment_id).
func (s *Service) GetPayment(ctx context.Context, id, ownerSessionID string) (domain.Payment, error) {
	p, err := s.repo.GetPaymentByID(ctx, id)
	if err != nil {
		return domain.Payment{}, err
	}
	if p.SessionID != ownerSessionID {
		return domain.Payment{}, ErrPaymentNotFound
	}
	return p, nil
}

// ConfirmPayment — POST /payments/{id}/confirm: эмуляция успешной оплаты,
// идемпотентна (zan-backend-tz-v3.md §5.1 — повторный confirm для уже
// success ничего не меняет и не ошибка).
func (s *Service) ConfirmPayment(ctx context.Context, id, ownerSessionID string) (domain.Payment, error) {
	p, err := s.repo.GetPaymentByID(ctx, id)
	if err != nil {
		return domain.Payment{}, err
	}
	if p.SessionID != ownerSessionID {
		return domain.Payment{}, ErrPaymentNotFound
	}

	if p.Status == domain.PaymentStatusFailed {
		return domain.Payment{}, ErrPaymentNotPending
	}
	if p.Status == domain.PaymentStatusSuccess {
		return p, nil // идемпотентно — ничего не начисляем повторно.
	}

	confirmed, applied, err := s.repo.ConfirmPayment(ctx, id, s.clock.Now())
	if err != nil {
		return domain.Payment{}, fmt.Errorf("billing: confirm payment: %w", err)
	}
	if !applied {
		// Гонка: между GetPaymentByID выше и ConfirmPayment платёж уже
		// перевели в терминальный статус параллельным запросом —
		// перечитываем и разбираем как обычный терминальный случай.
		return s.ConfirmPayment(ctx, id, ownerSessionID)
	}
	return confirmed, nil
}

// ExpireStalePending — фоновая задача (BACKEND_PLAN.md, Stage 2):
// pending-платежи старше StalePendingAfter -> failed.
func (s *Service) ExpireStalePending(ctx context.Context) (int, error) {
	n, err := s.repo.FailStalePending(ctx, s.clock.Now().Add(-StalePendingAfter))
	if err != nil {
		return 0, fmt.Errorf("billing: expire stale pending: %w", err)
	}
	return n, nil
}

// DebitCredit — списывает 1 единицу serviceID с баланса sessionID, если
// хватает (zan-backend-tz-v2.md §4.2 п.2). Используется Stage 3 при
// создании треда/сообщения; строится здесь, т.к. атомарность списания —
// ответственность billing, не thread (BACKEND_PLAN.md §1.1).
func (s *Service) DebitCredit(ctx context.Context, sessionID, serviceID string) error {
	ok, err := s.repo.DebitCredit(ctx, sessionID, serviceID)
	if err != nil {
		return fmt.Errorf("billing: debit credit: %w", err)
	}
	if !ok {
		return ErrInsufficientBalance
	}
	return nil
}

// RefundCredit — возвращает qty единиц serviceID на баланс sessionID
// (zan-backend-tz-v2.md §4.2 п.3 — "возврат при ошибке").
func (s *Service) RefundCredit(ctx context.Context, sessionID, serviceID string, qty int) error {
	if err := s.repo.CreditBalance(ctx, sessionID, serviceID, qty); err != nil {
		return fmt.Errorf("billing: refund credit: %w", err)
	}
	return nil
}
