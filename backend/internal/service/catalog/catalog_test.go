package catalog_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/catalog"
)

type fakeRepo struct {
	services map[string]domain.Service
	tariffs  map[string]domain.Tariff
	nextSort int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		services: map[string]domain.Service{
			"qa":  {ID: "qa", TypeLabel: "Консультация", Name: "Вопрос-ответ", PriceTenge: 2900, IsActive: true},
			"doc": {ID: "doc", TypeLabel: "Документ", Name: "Подготовка документа", PriceTenge: 4900, IsActive: true},
		},
		tariffs: make(map[string]domain.Tariff),
	}
}

func (r *fakeRepo) ListServices(context.Context) ([]domain.Service, error) {
	out := make([]domain.Service, 0, len(r.services))
	for _, s := range r.services {
		out = append(out, s)
	}
	return out, nil
}

func (r *fakeRepo) GetServiceByID(_ context.Context, id string) (domain.Service, error) {
	s, ok := r.services[id]
	if !ok {
		return domain.Service{}, catalog.ErrServiceNotFound
	}
	return s, nil
}

func (r *fakeRepo) UpdateService(_ context.Context, id string, priceTenge int, isActive bool) (domain.Service, error) {
	s, ok := r.services[id]
	if !ok {
		return domain.Service{}, catalog.ErrServiceNotFound
	}
	s.PriceTenge = priceTenge
	s.IsActive = isActive
	r.services[id] = s
	return s, nil
}

func (r *fakeRepo) ListTariffs(context.Context) ([]domain.Tariff, error) {
	out := make([]domain.Tariff, 0, len(r.tariffs))
	for _, t := range r.tariffs {
		out = append(out, t)
	}
	return out, nil
}

func (r *fakeRepo) GetTariffByID(_ context.Context, id string) (domain.Tariff, error) {
	t, ok := r.tariffs[id]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	return t, nil
}

func (r *fakeRepo) CreateTariff(_ context.Context, t domain.Tariff) (domain.Tariff, error) {
	r.nextSort++
	t.IsActive = true
	t.SortOrder = r.nextSort
	r.tariffs[t.ID] = t
	return t, nil
}

func (r *fakeRepo) UpdateTariff(_ context.Context, t domain.Tariff) (domain.Tariff, error) {
	existing, ok := r.tariffs[t.ID]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	existing.Name = t.Name
	existing.DiscountPercent = t.DiscountPercent
	existing.Items = t.Items
	r.tariffs[t.ID] = existing
	return existing, nil
}

func (r *fakeRepo) DeactivateTariff(_ context.Context, id string) (domain.Tariff, error) {
	t, ok := r.tariffs[id]
	if !ok {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}
	t.IsActive = false
	r.tariffs[id] = t
	return t, nil
}

const testTariffID = "b1a85f64-5717-4562-b3fc-2c963f66afa6"

func newTestService(repo *fakeRepo) *catalog.Service {
	return catalog.New(repo, &idgen.Fake{IDs: []string{testTariffID}})
}

func TestListActiveServices_FiltersInactive(t *testing.T) {
	repo := newFakeRepo()
	svc := repo.services["doc"]
	svc.IsActive = false
	repo.services["doc"] = svc

	active, err := newTestService(repo).ListActiveServices(context.Background())

	require.NoError(t, err)
	require.Len(t, active, 1)
	require.Equal(t, "qa", active[0].ID)
}

func TestUpdateServicePricing(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	updated, err := svc.UpdateServicePricing(context.Background(), "qa", 3500, false)
	require.NoError(t, err)
	require.Equal(t, 3500, updated.PriceTenge)
	require.False(t, updated.IsActive)

	_, err = svc.UpdateServicePricing(context.Background(), "unknown", 100, true)
	require.ErrorIs(t, err, catalog.ErrServiceNotFound)
}

func TestCreateTariff_Validation(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)
	ctx := context.Background()

	tests := []struct {
		name            string
		discountPercent int
		items           []domain.BundleItem
		wantErr         error
	}{
		{"empty items", 10, nil, catalog.ErrInvalidItems},
		{"qty too low", 10, []domain.BundleItem{{ServiceID: "qa", Qty: 0}}, catalog.ErrInvalidItems},
		{"qty too high", 10, []domain.BundleItem{{ServiceID: "qa", Qty: 21}}, catalog.ErrInvalidItems},
		{"unknown service", 10, []domain.BundleItem{{ServiceID: "nope", Qty: 1}}, catalog.ErrInvalidItems},
		{"discount too high", 91, []domain.BundleItem{{ServiceID: "qa", Qty: 1}}, catalog.ErrInvalidDiscount},
		{"discount negative", -1, []domain.BundleItem{{ServiceID: "qa", Qty: 1}}, catalog.ErrInvalidDiscount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateTariff(ctx, "Пакет", tt.discountPercent, tt.items)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestCreateTariff_Success(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo)

	created, err := svc.CreateTariff(context.Background(), "Для бизнеса", 15,
		[]domain.BundleItem{{ServiceID: "qa", Qty: 3}})

	require.NoError(t, err)
	require.Equal(t, testTariffID, created.ID)
	require.True(t, created.IsActive)
	require.Equal(t, 1, created.SortOrder)
}

func TestListActiveTariffs_ComputesPriceAndFiltersInactive(t *testing.T) {
	repo := newFakeRepo()
	repo.tariffs["archived"] = domain.Tariff{
		ID: "archived", Name: "Архивный", DiscountPercent: 10, IsActive: false,
		Items: []domain.BundleItem{{ServiceID: "qa", Qty: 1}},
	}
	repo.tariffs["visible"] = domain.Tariff{
		ID: "visible", Name: "Пакет вопросов", DiscountPercent: 15, IsActive: true,
		Items: []domain.BundleItem{{ServiceID: "qa", Qty: 3}},
	}
	svc := newTestService(repo)

	prices, err := svc.ListActiveTariffs(context.Background())

	require.NoError(t, err)
	require.Len(t, prices, 1)
	require.Equal(t, "visible", prices[0].Tariff.ID)
	require.Equal(t, 8700, prices[0].SubtotalTenge)
	require.Equal(t, 7400, prices[0].TotalTenge)
}

func TestDeactivateTariff_NotFound(t *testing.T) {
	svc := newTestService(newFakeRepo())

	_, err := svc.DeactivateTariff(context.Background(), "unknown")

	require.ErrorIs(t, err, catalog.ErrTariffNotFound)
}
