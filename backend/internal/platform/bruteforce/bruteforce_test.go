package bruteforce_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/bruteforce"
	"zan-backend/internal/platform/clock"
)

func TestStore_Allowed_TrueForUnknownIP(t *testing.T) {
	s := bruteforce.NewStore(3, time.Minute, clock.Real{})

	require.True(t, s.Allowed("1.2.3.4"))
}

func TestStore_RecordFailure_BlocksAfterMaxFailures(t *testing.T) {
	s := bruteforce.NewStore(3, time.Minute, clock.Real{})

	s.RecordFailure("1.2.3.4")
	s.RecordFailure("1.2.3.4")
	require.True(t, s.Allowed("1.2.3.4"), "still allowed before the 3rd failure")

	s.RecordFailure("1.2.3.4")
	require.False(t, s.Allowed("1.2.3.4"), "blocked on the 3rd consecutive failure")
}

func TestStore_RecordFailure_DoesNotAffectOtherIPs(t *testing.T) {
	s := bruteforce.NewStore(1, time.Minute, clock.Real{})

	s.RecordFailure("1.2.3.4")

	require.False(t, s.Allowed("1.2.3.4"))
	require.True(t, s.Allowed("5.6.7.8"))
}

func TestStore_RecordSuccess_ResetsFailureCounter(t *testing.T) {
	s := bruteforce.NewStore(3, time.Minute, clock.Real{})

	s.RecordFailure("1.2.3.4")
	s.RecordFailure("1.2.3.4")
	s.RecordSuccess("1.2.3.4")
	s.RecordFailure("1.2.3.4")
	s.RecordFailure("1.2.3.4")

	require.True(t, s.Allowed("1.2.3.4"), "counter was reset by the success in between")
}

func TestStore_Block_ExpiresAfterBlockDuration(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := bruteforce.NewStore(1, time.Minute, fake)

	s.RecordFailure("1.2.3.4")
	require.False(t, s.Allowed("1.2.3.4"))

	fake.T = fake.T.Add(59 * time.Second)
	require.False(t, s.Allowed("1.2.3.4"), "block has not expired yet")

	fake.T = fake.T.Add(2 * time.Second)
	require.True(t, s.Allowed("1.2.3.4"), "block expired")
}

func TestStore_RecordSuccess_DoesNotLiftAnActiveBlock(t *testing.T) {
	s := bruteforce.NewStore(1, time.Minute, clock.Real{})

	s.RecordFailure("1.2.3.4")
	require.False(t, s.Allowed("1.2.3.4"))

	// AdminAuth проверяет Allowed ДО сравнения токена — верный токен,
	// пришедший в пределах активной блокировки, физически не мог бы дойти
	// до RecordSuccess. Этот тест фиксирует, что сам Store не подстраховывает
	// вызывающий код, если бы тот случайно вызвал RecordSuccess напрямую.
	s.RecordSuccess("1.2.3.4")
	require.False(t, s.Allowed("1.2.3.4"))
}

func TestStore_BlockResets_RequiresMaxFailuresAgainAfterExpiry(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	s := bruteforce.NewStore(2, time.Minute, fake)

	s.RecordFailure("1.2.3.4")
	s.RecordFailure("1.2.3.4")
	require.False(t, s.Allowed("1.2.3.4"))

	fake.T = fake.T.Add(time.Minute + time.Second)
	require.True(t, s.Allowed("1.2.3.4"))

	s.RecordFailure("1.2.3.4")
	require.True(t, s.Allowed("1.2.3.4"), "one failure after expiry must not re-trip immediately")
}
