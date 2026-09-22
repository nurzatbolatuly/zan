package resilience_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/resilience"
)

var errBoom = errors.New("boom")

func TestRetryPolicy_Do_SucceedsWithoutRetry(t *testing.T) {
	policy := resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0

	err := policy.Do(context.Background(), func(error) bool { return true }, func(context.Context) error {
		calls++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, calls)
}

func TestRetryPolicy_Do_RetriesWhileShouldRetry(t *testing.T) {
	policy := resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0

	err := policy.Do(context.Background(), func(error) bool { return true }, func(context.Context) error {
		calls++
		if calls < 3 {
			return errBoom
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 3, calls)
}

func TestRetryPolicy_Do_StopsWhenShouldRetryFalse(t *testing.T) {
	policy := resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0

	err := policy.Do(context.Background(), func(error) bool { return false }, func(context.Context) error {
		calls++
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, 1, calls)
}

func TestRetryPolicy_Do_GivesUpAfterMaxAttempts(t *testing.T) {
	policy := resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond}
	calls := 0

	err := policy.Do(context.Background(), func(error) bool { return true }, func(context.Context) error {
		calls++
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, 3, calls)
}

func TestRetryPolicy_Do_StopsOnContextCancel(t *testing.T) {
	policy := resilience.RetryPolicy{MaxAttempts: 5, BaseDelay: 50 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	err := policy.Do(ctx, func(error) bool { return true }, func(context.Context) error {
		calls++
		if calls == 1 {
			cancel()
		}
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, 1, calls)
}

func TestRetryPolicy_Do_DefaultsToOneAttempt(t *testing.T) {
	policy := resilience.RetryPolicy{}
	calls := 0

	err := policy.Do(context.Background(), func(error) bool { return true }, func(context.Context) error {
		calls++
		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	require.Equal(t, 1, calls)
}
