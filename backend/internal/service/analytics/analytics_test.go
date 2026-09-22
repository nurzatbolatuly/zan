package analytics_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/service/analytics"
)

type fakeRepo struct {
	overview domain.AnalyticsOverview
	err      error
}

func (r fakeRepo) GetOverview(context.Context) (domain.AnalyticsOverview, error) {
	return r.overview, r.err
}

func TestGetOverview_ReturnsRepositoryResultAsIs(t *testing.T) {
	avg := 12.5
	satisfaction := 0.75
	want := domain.AnalyticsOverview{
		TotalThreads: 42,
		StatusBreakdown: map[domain.ThreadStatus]int{
			domain.ThreadStatusDone:  30,
			domain.ThreadStatusError: 5,
		},
		AvgProcessingTimeSec: &avg,
		SatisfactionRate:     &satisfaction,
	}
	svc := analytics.New(fakeRepo{overview: want})

	got, err := svc.GetOverview(context.Background())

	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestGetOverview_PropagatesRepositoryError(t *testing.T) {
	svc := analytics.New(fakeRepo{err: errors.New("db unavailable")})

	_, err := svc.GetOverview(context.Background())

	require.Error(t, err)
}
