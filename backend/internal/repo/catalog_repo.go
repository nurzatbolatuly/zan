package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/catalog"
)

// CatalogRepo — реализация catalog.Repository (Stage 2).
type CatalogRepo struct {
	db *pgxpool.Pool
}

// NewCatalogRepo строит CatalogRepo поверх общего пула соединений.
func NewCatalogRepo(db *pgxpool.Pool) *CatalogRepo {
	return &CatalogRepo{db: db}
}

func (r *CatalogRepo) ListServices(ctx context.Context) ([]domain.Service, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, type_label, name, price, is_active, updated_at
		FROM core.services
		ORDER BY id
	`)
	if err != nil {
		return nil, fmt.Errorf("repo: list services: %w", err)
	}
	defer rows.Close()

	var services []domain.Service
	for rows.Next() {
		var sr serviceRow
		if err := rows.Scan(&sr.id, &sr.typeLabel, &sr.name, &sr.price, &sr.isActive, &sr.updatedAt); err != nil {
			return nil, fmt.Errorf("repo: list services: scan: %w", err)
		}
		services = append(services, sr.toDomain())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: list services: %w", err)
	}
	return services, nil
}

func (r *CatalogRepo) GetServiceByID(ctx context.Context, id string) (domain.Service, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, type_label, name, price, is_active, updated_at
		FROM core.services
		WHERE id = $1
	`, id)

	var sr serviceRow
	if err := row.Scan(&sr.id, &sr.typeLabel, &sr.name, &sr.price, &sr.isActive, &sr.updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Service{}, catalog.ErrServiceNotFound
		}
		return domain.Service{}, fmt.Errorf("repo: get service: %w", err)
	}
	return sr.toDomain(), nil
}

func (r *CatalogRepo) UpdateService(ctx context.Context, id string, priceTenge int, isActive bool) (domain.Service, error) {
	price, err := toInt32(priceTenge)
	if err != nil {
		return domain.Service{}, err
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.services
		SET price = $2, is_active = $3, updated_at = now()
		WHERE id = $1
		RETURNING id, type_label, name, price, is_active, updated_at
	`, id, price, isActive)

	var sr serviceRow
	if err := row.Scan(&sr.id, &sr.typeLabel, &sr.name, &sr.price, &sr.isActive, &sr.updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Service{}, catalog.ErrServiceNotFound
		}
		return domain.Service{}, fmt.Errorf("repo: update service: %w", err)
	}
	return sr.toDomain(), nil
}

func (r *CatalogRepo) ListTariffs(ctx context.Context) ([]domain.Tariff, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, discount_percent, items, is_active, sort_order, created_at, updated_at
		FROM core.tariffs
		ORDER BY sort_order, created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("repo: list tariffs: %w", err)
	}
	defer rows.Close()

	var tariffs []domain.Tariff
	for rows.Next() {
		var tr tariffRow
		if err := rows.Scan(&tr.id, &tr.name, &tr.discountPercent, &tr.items, &tr.isActive, &tr.sortOrder, &tr.createdAt, &tr.updatedAt); err != nil {
			return nil, fmt.Errorf("repo: list tariffs: scan: %w", err)
		}
		t, err := tr.toDomain()
		if err != nil {
			return nil, err
		}
		tariffs = append(tariffs, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("repo: list tariffs: %w", err)
	}
	return tariffs, nil
}

func (r *CatalogRepo) GetTariffByID(ctx context.Context, id string) (domain.Tariff, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, name, discount_percent, items, is_active, sort_order, created_at, updated_at
		FROM core.tariffs
		WHERE id = $1
	`, pgID)

	var tr tariffRow
	if err := row.Scan(&tr.id, &tr.name, &tr.discountPercent, &tr.items, &tr.isActive, &tr.sortOrder, &tr.createdAt, &tr.updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Tariff{}, catalog.ErrTariffNotFound
		}
		return domain.Tariff{}, fmt.Errorf("repo: get tariff: %w", err)
	}
	return tr.toDomain()
}

func (r *CatalogRepo) CreateTariff(ctx context.Context, t domain.Tariff) (domain.Tariff, error) {
	id, err := parseUUID(t.ID)
	if err != nil {
		return domain.Tariff{}, err
	}
	items, err := json.Marshal(t.Items)
	if err != nil {
		return domain.Tariff{}, fmt.Errorf("repo: marshal tariff items: %w", err)
	}
	discountPercent, err := toInt32(t.DiscountPercent)
	if err != nil {
		return domain.Tariff{}, err
	}

	row := r.db.QueryRow(ctx, `
		INSERT INTO core.tariffs (id, name, discount_percent, items, sort_order)
		VALUES ($1, $2, $3, $4, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM core.tariffs))
		RETURNING id, name, discount_percent, items, is_active, sort_order, created_at, updated_at
	`, id, t.Name, discountPercent, items)

	var tr tariffRow
	if err := row.Scan(&tr.id, &tr.name, &tr.discountPercent, &tr.items, &tr.isActive, &tr.sortOrder, &tr.createdAt, &tr.updatedAt); err != nil {
		return domain.Tariff{}, fmt.Errorf("repo: create tariff: %w", err)
	}
	return tr.toDomain()
}

func (r *CatalogRepo) UpdateTariff(ctx context.Context, t domain.Tariff) (domain.Tariff, error) {
	id, err := parseUUID(t.ID)
	if err != nil {
		return domain.Tariff{}, err
	}
	items, err := json.Marshal(t.Items)
	if err != nil {
		return domain.Tariff{}, fmt.Errorf("repo: marshal tariff items: %w", err)
	}
	discountPercent, err := toInt32(t.DiscountPercent)
	if err != nil {
		return domain.Tariff{}, err
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.tariffs
		SET name = $2, discount_percent = $3, items = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, name, discount_percent, items, is_active, sort_order, created_at, updated_at
	`, id, t.Name, discountPercent, items)

	var tr tariffRow
	if err := row.Scan(&tr.id, &tr.name, &tr.discountPercent, &tr.items, &tr.isActive, &tr.sortOrder, &tr.createdAt, &tr.updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Tariff{}, catalog.ErrTariffNotFound
		}
		return domain.Tariff{}, fmt.Errorf("repo: update tariff: %w", err)
	}
	return tr.toDomain()
}

func (r *CatalogRepo) DeactivateTariff(ctx context.Context, id string) (domain.Tariff, error) {
	pgID, err := parseUUID(id)
	if err != nil {
		return domain.Tariff{}, catalog.ErrTariffNotFound
	}

	row := r.db.QueryRow(ctx, `
		UPDATE core.tariffs
		SET is_active = false, updated_at = now()
		WHERE id = $1
		RETURNING id, name, discount_percent, items, is_active, sort_order, created_at, updated_at
	`, pgID)

	var tr tariffRow
	if err := row.Scan(&tr.id, &tr.name, &tr.discountPercent, &tr.items, &tr.isActive, &tr.sortOrder, &tr.createdAt, &tr.updatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.Tariff{}, catalog.ErrTariffNotFound
		}
		return domain.Tariff{}, fmt.Errorf("repo: deactivate tariff: %w", err)
	}
	return tr.toDomain()
}

// serviceRow/tariffRow — формы строк core.services/core.tariffs под Scan;
// не запросы, чистое хранилище отсканированных колонок (см. sessionRow).

type serviceRow struct {
	id        string
	typeLabel string
	name      string
	price     int32
	isActive  bool
	updatedAt pgtype.Timestamptz
}

func (sr serviceRow) toDomain() domain.Service {
	return domain.Service{
		ID:         sr.id,
		TypeLabel:  sr.typeLabel,
		Name:       sr.name,
		PriceTenge: int(sr.price),
		IsActive:   sr.isActive,
		UpdatedAt:  sr.updatedAt.Time,
	}
}

type tariffRow struct {
	id              pgtype.UUID
	name            string
	discountPercent int32
	items           []byte
	isActive        bool
	sortOrder       int32
	createdAt       pgtype.Timestamptz
	updatedAt       pgtype.Timestamptz
}

func (tr tariffRow) toDomain() (domain.Tariff, error) {
	var items []domain.BundleItem
	if err := json.Unmarshal(tr.items, &items); err != nil {
		return domain.Tariff{}, fmt.Errorf("repo: tariff row: unmarshal items: %w", err)
	}
	return domain.Tariff{
		ID:              fromPgUUID(tr.id),
		Name:            tr.name,
		DiscountPercent: int(tr.discountPercent),
		Items:           items,
		IsActive:        tr.isActive,
		SortOrder:       int(tr.sortOrder),
		CreatedAt:       tr.createdAt.Time,
		UpdatedAt:       tr.updatedAt.Time,
	}, nil
}
