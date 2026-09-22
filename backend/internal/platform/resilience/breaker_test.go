package resilience_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/resilience"
)

func TestBreaker_Execute_PassesThroughWhenClosed(t *testing.T) {
	b := resilience.NewBreaker(resilience.BreakerConfig{
		Name:                      "test",
		ConsecutiveFailuresToTrip: 3,
		OpenTimeout:               time.Minute,
	})

	require.NoError(t, b.Execute(context.Background(), func(context.Context) error { return nil }))
}

func TestBreaker_Execute_OpensAfterConsecutiveFailures(t *testing.T) {
	b := resilience.NewBreaker(resilience.BreakerConfig{
		Name:                      "test",
		ConsecutiveFailuresToTrip: 3,
		OpenTimeout:               time.Minute,
	})

	for range 3 {
		err := b.Execute(context.Background(), func(context.Context) error { return errBoom })
		require.ErrorIs(t, err, errBoom)
	}

	// 4-й вызов должен быть отклонён немедленно, без вызова fn.
	called := false
	err := b.Execute(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	require.ErrorIs(t, err, resilience.ErrBreakerOpen)
	require.False(t, called, "fn must not be called while breaker is open")
}

func TestBreaker_Execute_ClosesAgainAfterTimeout(t *testing.T) {
	b := resilience.NewBreaker(resilience.BreakerConfig{
		Name:                      "test",
		ConsecutiveFailuresToTrip: 1,
		OpenTimeout:               50 * time.Millisecond,
	})

	require.ErrorIs(t, b.Execute(context.Background(), func(context.Context) error { return errBoom }), errBoom)
	require.ErrorIs(t, b.Execute(context.Background(), func(context.Context) error { return nil }), resilience.ErrBreakerOpen)

	time.Sleep(100 * time.Millisecond)

	require.NoError(t, b.Execute(context.Background(), func(context.Context) error { return nil }))
}
