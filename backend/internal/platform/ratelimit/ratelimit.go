// Package ratelimit — in-memory rate limiting по IP на golang.org/x/time/rate
// (BACKEND_PLAN.md §1.1: "in-memory, per-IP/per-session на старте; вынести в
// Redis, если деплой станет multi-instance" — осознанное ограничение
// MVP-масштаба, не забытый недостаток).
package ratelimit

import (
	"sync"

	"golang.org/x/time/rate"
)

// Store — состояние между запросами внутри процесса: явная структура с
// мьютексом, поле в composition root (BACKEND_CODING_STANDARDS.md §2), не
// package-level var.
type Store struct {
	mu       sync.Mutex
	limiters map[string]*rate.Limiter
	r        rate.Limit
	burst    int
}

// NewStore строит Store с лимитом r событий в секунду и запасом burst на
// IP-адрес.
func NewStore(r rate.Limit, burst int) *Store {
	return &Store{
		limiters: make(map[string]*rate.Limiter),
		r:        r,
		burst:    burst,
	}
}

// Allow сообщает, разрешён ли ещё один запрос с этого IP прямо сейчас.
func (s *Store) Allow(ip string) bool {
	return s.limiterFor(ip).Allow()
}

func (s *Store) limiterFor(ip string) *rate.Limiter {
	s.mu.Lock()
	defer s.mu.Unlock()

	l, ok := s.limiters[ip]
	if !ok {
		l = rate.NewLimiter(s.r, s.burst)
		s.limiters[ip] = l
	}
	return l
}
