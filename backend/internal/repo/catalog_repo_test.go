package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/catalog"
)

func TestCatalogRepo_ListServices_SeededByMigration(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewCatalogRepo(pool)

	services, err := r.ListServices(context.Background())

	require.NoError(t, err)
	require.Len(t, services, 2) // migrations/000003_stage2_billing.up.sql — qa/doc
	byID := make(map[string]domain.Service, len(services))
	for _, s := range services {
		byID[s.ID] = s
	}
	require.Equal(t, 2900, byID["qa"].PriceTenge)
	require.Equal(t, 4900, byID["doc"].PriceTenge)
	require.True(t, byID["qa"].IsActive)
}

func TestCatalogRepo_UpdateService(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewCatalogRepo(pool)

	updated, err := r.UpdateService(context.Background(), "qa", 3500, false)

	require.NoError(t, err)
	require.Equal(t, 3500, updated.PriceTenge)
	require.False(t, updated.IsActive)

	_, err = r.UpdateService(context.Background(), "unknown", 100, true)
	require.ErrorIs(t, err, catalog.ErrServiceNotFound)
}

func TestCatalogRepo_GetServiceByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewCatalogRepo(pool)

	_, err := r.GetServiceByID(context.Background(), "unknown")

	require.ErrorIs(t, err, catalog.ErrServiceNotFound)
}

func TestCatalogRepo_TariffCRUD(t *testing.T) {
	pool := setupTestDB(t)
	r := repo.NewCatalogRepo(pool)
	ctx := context.Background()

	created, err := r.CreateTariff(ctx, domain.Tariff{
		ID:              "3fa85f64-5717-4562-b3fc-2c963f66afa6",
		Name:            "Пакет вопросов",
		DiscountPercent: 15,
		Items:           []domain.BundleItem{{ServiceID: "qa", Qty: 3}},
	})
	require.NoError(t, err)
	require.True(t, created.IsActive)
	require.Equal(t, 1, created.SortOrder)
	require.Equal(t, []domain.BundleItem{{ServiceID: "qa", Qty: 3}}, created.Items)

	got, err := r.GetTariffByID(ctx, created.ID)
	require.NoError(t, err)
	require.Equal(t, created, got)

	updated, err := r.UpdateTariff(ctx, domain.Tariff{
		ID:              created.ID,
		Name:            "Для бизнеса",
		DiscountPercent: 25,
		Items:           []domain.BundleItem{{ServiceID: "qa", Qty: 10}, {ServiceID: "doc", Qty: 3}},
	})
	require.NoError(t, err)
	require.Equal(t, "Для бизнеса", updated.Name)
	require.Equal(t, 25, updated.DiscountPercent)

	deactivated, err := r.DeactivateTariff(ctx, created.ID)
	require.NoError(t, err)
	require.False(t, deactivated.IsActive)

	all, err := r.ListTariffs(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)

	_, err = r.GetTariffByID(ctx, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, catalog.ErrTariffNotFound)
}
