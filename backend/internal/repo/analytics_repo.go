package repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"zan-backend/internal/domain"
)

// AnalyticsRepo — реализация analytics.Repository поверх pgx напрямую
// (BACKEND_CODING_STANDARDS.md §1.1). Два запроса, не один — считают
// агрегаты по двум разным таблицам (core.threads/core.messages), которые
// не связаны напрямую FK, годным для одного JOIN способом (тред может
// вообще не иметь оценённых сообщений) — раздельные запросы читаются
// прямее, чем LEFT JOIN с дублированием строк под GROUP BY.
type AnalyticsRepo struct {
	db *pgxpool.Pool
}

// NewAnalyticsRepo строит AnalyticsRepo поверх общего пула соединений.
func NewAnalyticsRepo(db *pgxpool.Pool) *AnalyticsRepo {
	return &AnalyticsRepo{db: db}
}

// GetOverview — GET /admin/analytics/overview (zan-backend-tz-v2.md §3.8).
// Soft-deleted треды (deleted_at IS NOT NULL) исключены из обоих
// агрегатов — тот же фильтр, что уже применяет GET /threads (пользователь
// их удалил, для админ-обзора текущего состояния они не в счёт).
func (r *AnalyticsRepo) GetOverview(ctx context.Context) (domain.AnalyticsOverview, error) {
	statusRows, err := r.db.Query(ctx, `
		SELECT status, count(*)
		FROM core.threads
		WHERE deleted_at IS NULL
		GROUP BY status
	`)
	if err != nil {
		return domain.AnalyticsOverview{}, fmt.Errorf("repo: analytics overview: thread status breakdown: %w", err)
	}
	defer statusRows.Close()

	breakdown := make(map[domain.ThreadStatus]int, 6)
	total := 0
	for statusRows.Next() {
		var rawStatus string
		var count int64
		if err := statusRows.Scan(&rawStatus, &count); err != nil {
			return domain.AnalyticsOverview{}, fmt.Errorf("repo: analytics overview: scan status breakdown: %w", err)
		}
		status, err := domain.ParseThreadStatus(rawStatus)
		if err != nil {
			return domain.AnalyticsOverview{}, fmt.Errorf("repo: analytics overview: %w", err)
		}
		breakdown[status] = int(count)
		total += int(count)
	}
	if err := statusRows.Err(); err != nil {
		return domain.AnalyticsOverview{}, fmt.Errorf("repo: analytics overview: thread status breakdown: %w", err)
	}

	// avg(...) на пустом множестве строк даёт SQL NULL (pgtype.Float8.Valid
	// == false), не 0 — ровно то различие, которое domain.AnalyticsOverview
	// обязана сохранить наружу (см. её doc-комментарий). rated_count/
	// liked_count — count(*) FILTER, всегда NOT NULL (0 при отсутствии строк).
	row := r.db.QueryRow(ctx, `
		SELECT
		    avg(processing_time_ms) FILTER (WHERE processing_time_ms IS NOT NULL),
		    count(*) FILTER (WHERE feedback IS NOT NULL),
		    count(*) FILTER (WHERE feedback = 'like')
		FROM core.messages AS m
		JOIN core.threads AS t ON t.id = m.thread_id
		WHERE m.sender = 'assistant'
		  AND t.deleted_at IS NULL
	`)

	var avgProcessingMs pgtype.Float8
	var ratedCount, likedCount int64
	if err := row.Scan(&avgProcessingMs, &ratedCount, &likedCount); err != nil {
		return domain.AnalyticsOverview{}, fmt.Errorf("repo: analytics overview: message aggregates: %w", err)
	}

	var avgProcessingTimeSec *float64
	if avgProcessingMs.Valid {
		v := avgProcessingMs.Float64 / 1000
		avgProcessingTimeSec = &v
	}
	var satisfactionRate *float64
	if ratedCount > 0 {
		v := float64(likedCount) / float64(ratedCount)
		satisfactionRate = &v
	}

	return domain.AnalyticsOverview{
		TotalThreads:         total,
		StatusBreakdown:      breakdown,
		AvgProcessingTimeSec: avgProcessingTimeSec,
		SatisfactionRate:     satisfactionRate,
	}, nil
}
