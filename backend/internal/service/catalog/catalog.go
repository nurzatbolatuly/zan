// Package catalog — юзкейсы услуг и тарифов (GET /services, GET /tariffs,
// /admin/services, /admin/tariffs — zan-backend-tz-v2.md §3.4/§3.7).
// Расчёт цены тарифа делегирован internal/service/pricing (единая
// формула, переиспользуется здесь и в internal/service/billing —
// BACKEND_PLAN.md §5, Stage 2). Порт Repository объявлен здесь, где
// используется (BACKEND_CODING_STANDARDS.md §1.1).
package catalog

import (
	"context"
	"errors"
	"fmt"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/pricing"
)

const (
	// MinItemQty/MaxItemQty — лимит qty на услугу в тарифе/custom-наборе,
	// 1–20 "как в прототипе" (zan-backend-tz-v2.md §2.3, §4.1).
	MinItemQty = 1
	MaxItemQty = 20

	// MinDiscountPercent/MaxDiscountPercent — zan-backend-tz-v2.md §2.3.
	MinDiscountPercent = 0
	MaxDiscountPercent = 90
)

var (
	// ErrServiceNotFound — нет услуги с таким id.
	ErrServiceNotFound = errors.New("catalog: service not found")

	// ErrTariffNotFound — нет тарифа с таким id.
	ErrTariffNotFound = errors.New("catalog: tariff not found")

	// ErrInvalidItems — items пусты, содержат неизвестный service_id или
	// qty вне [MinItemQty; MaxItemQty] (zan-backend-tz-v2.md §2.3/§4.1).
	ErrInvalidItems = errors.New("catalog: invalid items")

	// ErrInvalidDiscount — discount_percent вне [MinDiscountPercent; MaxDiscountPercent].
	ErrInvalidDiscount = errors.New("catalog: invalid discount percent")
)

// Repository — порт доступа к хранилищу услуг/тарифов.
type Repository interface {
	ListServices(ctx context.Context) ([]domain.Service, error)
	GetServiceByID(ctx context.Context, id string) (domain.Service, error)
	UpdateService(ctx context.Context, id string, priceTenge int, isActive bool) (domain.Service, error)

	ListTariffs(ctx context.Context) ([]domain.Tariff, error)
	GetTariffByID(ctx context.Context, id string) (domain.Tariff, error)
	CreateTariff(ctx context.Context, t domain.Tariff) (domain.Tariff, error)
	UpdateTariff(ctx context.Context, t domain.Tariff) (domain.Tariff, error)
	DeactivateTariff(ctx context.Context, id string) (domain.Tariff, error)
}

// Service — бизнес-логика каталога и расчёта цены.
type Service struct {
	repo  Repository
	idgen idgen.Generator
}

// New собирает Service с внедрёнными зависимостями.
func New(repo Repository, ids idgen.Generator) *Service {
	return &Service{repo: repo, idgen: ids}
}

// TariffPrice — Tariff вместе с посчитанной по текущим ценам услуг ценой
// (zan-backend-tz-v2.md §2.3: "цена всегда считается на лету, не хранится
// статично").
type TariffPrice struct {
	Tariff        domain.Tariff
	SubtotalTenge int
	TotalTenge    int
}

// ListActiveServices — GET /services: активные услуги с ценами.
func (s *Service) ListActiveServices(ctx context.Context) ([]domain.Service, error) {
	all, err := s.repo.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list active services: %w", err)
	}
	return filterActiveServices(all), nil
}

// ListAllServices — GET /admin/services: все услуги, включая неактивные
// (zan-backend-tz-v2.md §3.7).
func (s *Service) ListAllServices(ctx context.Context) ([]domain.Service, error) {
	all, err := s.repo.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list all services: %w", err)
	}
	return all, nil
}

// UpdateServicePricing — PUT /admin/services/{id}: обновляет price/is_active
// (zan-backend-tz-v2.md §3.7). Не пересчитывает уже оформленные Payment
// (§4.1) — те хранят AmountTenge как исторический факт.
func (s *Service) UpdateServicePricing(ctx context.Context, id string, priceTenge int, isActive bool) (domain.Service, error) {
	if priceTenge < 0 {
		return domain.Service{}, fmt.Errorf("%w: price must be non-negative", ErrInvalidItems)
	}
	if _, err := s.repo.GetServiceByID(ctx, id); err != nil {
		return domain.Service{}, err
	}
	updated, err := s.repo.UpdateService(ctx, id, priceTenge, isActive)
	if err != nil {
		return domain.Service{}, fmt.Errorf("catalog: update service pricing: %w", err)
	}
	return updated, nil
}

// ListActiveTariffs — GET /tariffs: активные бандлы с рассчитанной ценой.
func (s *Service) ListActiveTariffs(ctx context.Context) ([]TariffPrice, error) {
	tariffs, err := s.repo.ListTariffs(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list active tariffs: %w", err)
	}
	prices, err := s.servicePriceMap(ctx)
	if err != nil {
		return nil, err
	}
	return withPrices(filterActiveTariffs(tariffs), prices), nil
}

// ListAllTariffs — GET /admin/tariffs: все бандлы, включая неактивные.
func (s *Service) ListAllTariffs(ctx context.Context) ([]TariffPrice, error) {
	tariffs, err := s.repo.ListTariffs(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: list all tariffs: %w", err)
	}
	prices, err := s.servicePriceMap(ctx)
	if err != nil {
		return nil, err
	}
	return withPrices(tariffs, prices), nil
}

// GetTariffByID — используется и напрямую (PUT/DELETE-хендлеры), и
// internal/service/billing (checkout kind=tariff — та же формула цены,
// что и у GET /tariffs, BACKEND_PLAN.md §5).
func (s *Service) GetTariffByID(ctx context.Context, id string) (domain.Tariff, error) {
	t, err := s.repo.GetTariffByID(ctx, id)
	if err != nil {
		return domain.Tariff{}, err
	}
	return t, nil
}

// GetServiceByID — используется internal/service/thread (Stage 3): POST
// /threads должен списать баланс за существующую активную услугу, та же
// проверка существования, что и у ValidateItems, но без формы items.
func (s *Service) GetServiceByID(ctx context.Context, id string) (domain.Service, error) {
	svc, err := s.repo.GetServiceByID(ctx, id)
	if err != nil {
		return domain.Service{}, err
	}
	return svc, nil
}

// CreateTariff — POST /admin/tariffs: {name, discount_percent, items}
// (zan-backend-tz-v2.md §3.7).
func (s *Service) CreateTariff(ctx context.Context, name string, discountPercent int, items []domain.BundleItem) (domain.Tariff, error) {
	if err := s.validateDiscount(discountPercent); err != nil {
		return domain.Tariff{}, err
	}
	if err := s.ValidateItems(ctx, items); err != nil {
		return domain.Tariff{}, err
	}

	created, err := s.repo.CreateTariff(ctx, domain.Tariff{
		ID:              s.idgen.NewID(),
		Name:            name,
		DiscountPercent: discountPercent,
		Items:           items,
	})
	if err != nil {
		return domain.Tariff{}, fmt.Errorf("catalog: create tariff: %w", err)
	}
	return created, nil
}

// UpdateTariff — PUT /admin/tariffs/{id}: полная замена name/discount_percent/items.
func (s *Service) UpdateTariff(ctx context.Context, id, name string, discountPercent int, items []domain.BundleItem) (domain.Tariff, error) {
	if err := s.validateDiscount(discountPercent); err != nil {
		return domain.Tariff{}, err
	}
	if err := s.ValidateItems(ctx, items); err != nil {
		return domain.Tariff{}, err
	}
	if _, err := s.repo.GetTariffByID(ctx, id); err != nil {
		return domain.Tariff{}, err
	}

	updated, err := s.repo.UpdateTariff(ctx, domain.Tariff{
		ID:              id,
		Name:            name,
		DiscountPercent: discountPercent,
		Items:           items,
	})
	if err != nil {
		return domain.Tariff{}, fmt.Errorf("catalog: update tariff: %w", err)
	}
	return updated, nil
}

// DeactivateTariff — DELETE /admin/tariffs/{id}: is_active=false, архив,
// история платежей не трогается (zan-backend-tz-v2.md §3.7).
func (s *Service) DeactivateTariff(ctx context.Context, id string) (domain.Tariff, error) {
	updated, err := s.repo.DeactivateTariff(ctx, id)
	if err != nil {
		return domain.Tariff{}, fmt.Errorf("catalog: deactivate tariff: %w", err)
	}
	return updated, nil
}

// servicePriceMap строит serviceID -> priceTenge по всем услугам — тариф
// может ссылаться на услугу, которую позже деактивировали, цена всё
// равно должна посчитаться (репозиторий, не только каталог, решает,
// показывать ли такой тариф активным).
func (s *Service) servicePriceMap(ctx context.Context) (map[string]int, error) {
	services, err := s.repo.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("catalog: load service prices: %w", err)
	}
	prices := make(map[string]int, len(services))
	for _, svc := range services {
		prices[svc.ID] = svc.PriceTenge
	}
	return prices, nil
}

// ValidateItems — items непустые, qty каждой позиции в [MinItemQty;
// MaxItemQty], service_id существует (zan-backend-tz-v2.md §2.3/§4.1).
// Не полагается на DB-констрейнт: items — jsonb, без FK на services.
func (s *Service) ValidateItems(ctx context.Context, items []domain.BundleItem) error {
	if len(items) == 0 {
		return fmt.Errorf("%w: items must not be empty", ErrInvalidItems)
	}
	for _, item := range items {
		if item.Qty < MinItemQty || item.Qty > MaxItemQty {
			return fmt.Errorf("%w: qty for %q must be between %d and %d", ErrInvalidItems, item.ServiceID, MinItemQty, MaxItemQty)
		}
		if _, err := s.repo.GetServiceByID(ctx, item.ServiceID); err != nil {
			if errors.Is(err, ErrServiceNotFound) {
				return fmt.Errorf("%w: unknown service_id %q", ErrInvalidItems, item.ServiceID)
			}
			return fmt.Errorf("catalog: validate items: %w", err)
		}
	}
	return nil
}

func (s *Service) validateDiscount(discountPercent int) error {
	if discountPercent < MinDiscountPercent || discountPercent > MaxDiscountPercent {
		return fmt.Errorf("%w: must be between %d and %d", ErrInvalidDiscount, MinDiscountPercent, MaxDiscountPercent)
	}
	return nil
}

func filterActiveServices(services []domain.Service) []domain.Service {
	active := make([]domain.Service, 0, len(services))
	for _, svc := range services {
		if svc.IsActive {
			active = append(active, svc)
		}
	}
	return active
}

func filterActiveTariffs(tariffs []domain.Tariff) []domain.Tariff {
	active := make([]domain.Tariff, 0, len(tariffs))
	for _, t := range tariffs {
		if t.IsActive {
			active = append(active, t)
		}
	}
	return active
}

func withPrices(tariffs []domain.Tariff, prices map[string]int) []TariffPrice {
	result := make([]TariffPrice, 0, len(tariffs))
	for _, t := range tariffs {
		bundle := pricing.Compute(t.Items, prices, t.DiscountPercent)
		result = append(result, TariffPrice{
			Tariff:        t,
			SubtotalTenge: bundle.SubtotalTenge,
			TotalTenge:    bundle.TotalTenge,
		})
	}
	return result
}
