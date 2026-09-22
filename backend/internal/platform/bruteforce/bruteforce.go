// Package bruteforce — блокировка IP после N неудачных попыток
// (zan-backend-tz-v3.md §5.7: "подбор/перебор admin-secret — rate limiting
// + блокировка IP после N неудачных попыток на /admin/*", BACKEND_PLAN.md
// Stage 7). In-memory состояние процесса — то же осознанное MVP-ограничение
// (single-instance, без Redis), что и у internal/platform/ratelimit
// (BACKEND_PLAN.md §1.1).
package bruteforce

import (
	"sync"
	"time"

	"zan-backend/internal/platform/clock"
)

// entry — состояние одного IP: счётчик подряд идущих неудач + момент, до
// которого IP заблокирован (нулевое время — не заблокирован).
type entry struct {
	consecutiveFailures int
	blockedUntil        time.Time
}

// Store — состояние между запросами внутри процесса: явная структура с
// мьютексом, поле в composition root (BACKEND_CODING_STANDARDS.md §2), не
// package-level var — тот же принцип, что internal/platform/ratelimit.Store.
type Store struct {
	mu            sync.Mutex
	entries       map[string]*entry
	maxFailures   int
	blockDuration time.Duration
	clock         clock.Clock
}

// NewStore строит Store: maxFailures подряд идущих неудач блокируют IP на
// blockDuration. clk — внедряется как зависимость (BACKEND_CODING_STANDARDS.md
// §10, тот же приём, что и у thread.Service) для детерминированных тестов
// без реального time.Sleep.
func NewStore(maxFailures int, blockDuration time.Duration, clk clock.Clock) *Store {
	return &Store{
		entries:       make(map[string]*entry),
		maxFailures:   maxFailures,
		blockDuration: blockDuration,
		clock:         clk,
	}
}

// Allowed сообщает, разрешена ли ещё одна попытка авторизации с этого IP —
// false, только пока не истёк текущий blockDuration; сам факт проверки не
// считается ни успехом, ни неудачей (RecordFailure/RecordSuccess вызывает
// вызывающий middleware по итогу самой попытки).
func (s *Store) Allowed(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[ip]
	if !ok {
		return true
	}
	return e.blockedUntil.IsZero() || s.clock.Now().After(e.blockedUntil)
}

// RecordFailure — неудачная попытка (неверный/отсутствующий токен). При
// достижении maxFailures подряд — блокирует IP на blockDuration и сбрасывает
// счётчик (следующая блокировка снова потребует maxFailures неудач подряд
// после её истечения, не накапливается бесконечно).
func (s *Store) RecordFailure(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[ip]
	if !ok {
		e = &entry{}
		s.entries[ip] = e
	}
	e.consecutiveFailures++
	if e.consecutiveFailures >= s.maxFailures {
		e.blockedUntil = s.clock.Now().Add(s.blockDuration)
		e.consecutiveFailures = 0
	}
}

// RecordSuccess — верный токен: сбрасывает счётчик неудач этого IP
// (успешная авторизация означает, что предыдущие неудачи не были частью
// осмысленного перебора — не наказываем легитимного администратора,
// однажды опечатавшегося, за прошлые опечатки). Уже активная блокировка
// (e.blockedUntil в будущем) не снимается успехом — токен, пришедший в
// пределах блокировки, не должен был быть даже проверен (см. AdminAuth:
// Allowed проверяется раньше сравнения токена).
func (s *Store) RecordSuccess(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[ip]; ok {
		e.consecutiveFailures = 0
	}
}
