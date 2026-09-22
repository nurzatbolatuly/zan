// Package analytics — GET /admin/analytics/overview (BACKEND_PLAN.md
// Stage 7, zan-backend-tz-v2.md §3.8): агрегаты по своей БД, без внешних
// систем. Тонкий service поверх одного репозиторного метода — тот же
// принцип "каждая область — свой internal/service/X", что и у остальных
// пакетов (BACKEND_CODING_STANDARDS.md §1.1), даже когда бизнес-правил
// почти нет: тут единственная точка, где в будущем появится фильтрация по
// периоду/сегменту, если продукт попросит.
package analytics

import (
	"context"
	"fmt"

	"zan-backend/internal/domain"
)

// Repository — порт доступа к агрегатам. Объявлен здесь, где используется
// (BACKEND_CODING_STANDARDS.md §1.1) — *repo.AnalyticsRepo реализует его
// поверх pgx напрямую, без codegen.
type Repository interface {
	GetOverview(ctx context.Context) (domain.AnalyticsOverview, error)
}

// Service — бизнес-логика аналитики.
type Service struct {
	repo Repository
}

// New собирает Service.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// GetOverview — GET /admin/analytics/overview.
func (s *Service) GetOverview(ctx context.Context) (domain.AnalyticsOverview, error) {
	overview, err := s.repo.GetOverview(ctx)
	if err != nil {
		return domain.AnalyticsOverview{}, fmt.Errorf("analytics: get overview: %w", err)
	}
	return overview, nil
}
