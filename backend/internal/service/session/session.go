// Package session — юзкейсы сессии (POST /sessions, GET /sessions/me,
// PATCH /sessions/me, POST /sessions/onboarding-seen —
// zan-backend-tz-v3.md §2, §4). Порт Repository объявлен здесь, где
// используется, а не в internal/repo, где реализуется
// (BACKEND_CODING_STANDARDS.md §1.1).
package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
)

// TTL — скользящее окно сессии: expires_at = last_seen_at + 90 дней,
// продлевается при каждом запросе (zan-backend-tz-v3.md §2.1). Экспортная
// константа — httpserver использует то же значение как max-age у
// session_token cookie, чтобы cookie не переживала/не протухала раньше
// самой сессии. Не вынесено в ENV — продукт не просил делать это значение
// конфигурируемым.
const TTL = 90 * 24 * time.Hour

var (
	// ErrNotFound — сессии с таким id нет в хранилище. Возвращается
	// Repository.GetByID; на уровне Service преобразуется в тот же
	// ErrInvalidToken, что и подделанная подпись — клиенту всё равно
	// положен один 401 без автосоздания (v3 §5.5).
	ErrNotFound = errors.New("session: not found")

	// ErrInvalidToken — токен не прошёл проверку подписи, либо сессия по
	// извлечённому из него id не найдена.
	ErrInvalidToken = errors.New("session: invalid token")

	// ErrExpired — подпись валидна, но скользящее окно сессии истекло.
	ErrExpired = errors.New("session: expired")
)

// Repository — порт доступа к хранилищу сессий.
type Repository interface {
	Create(ctx context.Context, s domain.Session) error
	GetByID(ctx context.Context, id string) (domain.Session, error)
	Update(ctx context.Context, s domain.Session) error
}

// Service — бизнес-логика сессий.
type Service struct {
	repo   Repository
	clock  clock.Clock
	idgen  idgen.Generator
	signer *TokenSigner
}

// New собирает Service с внедрёнными зависимостями (без DI-фреймворка,
// вызывается вручную из cmd/api — BACKEND_CODING_STANDARDS.md §2).
func New(repo Repository, clk clock.Clock, ids idgen.Generator, signer *TokenSigner) *Service {
	return &Service{repo: repo, clock: clk, idgen: ids, signer: signer}
}

// Create — POST /sessions: заводит новую анонимную сессию и возвращает её
// вместе с подписанным токеном. Дефолтный Language — "ru": клиент не
// передаёт язык при создании (v3 §4 API-таблица отдаёт на вход только
// пустое тело), уточняется позже через PATCH /sessions/me.
func (s *Service) Create(ctx context.Context) (domain.Session, string, error) {
	now := s.clock.Now()
	sess := domain.Session{
		ID:             s.idgen.NewID(),
		Language:       domain.LanguageRu,
		OnboardingSeen: false,
		CreatedAt:      now,
		LastSeenAt:     now,
		ExpiresAt:      now.Add(TTL),
	}
	if err := s.repo.Create(ctx, sess); err != nil {
		return domain.Session{}, "", fmt.Errorf("session: create: %w", err)
	}
	return sess, s.signer.Sign(sess.ID), nil
}

// Authenticate проверяет токен (заголовок/cookie — извлекает вызывающий
// код в httpserver) и продлевает скользящее окно (v3 §2.1: "продлевается
// при каждом запросе") — используется session-middleware на каждый
// запрос под /sessions/me и далее под остальными защищёнными группами
// маршрутов по мере их появления в следующих стадиях.
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Session, error) {
	id, err := s.signer.Verify(token)
	if err != nil {
		return domain.Session{}, ErrInvalidToken
	}

	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return domain.Session{}, ErrInvalidToken
		}
		return domain.Session{}, fmt.Errorf("session: authenticate: %w", err)
	}

	now := s.clock.Now()
	if sess.IsExpired(now) {
		return domain.Session{}, ErrExpired
	}

	sess.LastSeenAt = now
	sess.ExpiresAt = now.Add(TTL)
	if err := s.repo.Update(ctx, sess); err != nil {
		return domain.Session{}, fmt.Errorf("session: touch: %w", err)
	}
	return sess, nil
}

// UpdatePreferences — PATCH /sessions/me: обновляет только переданные
// поля (nil — не трогать). Валидность значений (что language/theme —
// одно из допустимых) проверена на границе httpserver (domain.ParseXxx)
// ещё до вызова этого метода.
func (s *Service) UpdatePreferences(ctx context.Context, id string, language *domain.Language, theme *domain.Theme) (domain.Session, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Session{}, fmt.Errorf("session: update preferences: %w", err)
	}
	if language != nil {
		sess.Language = *language
	}
	if theme != nil {
		sess.Theme = theme
	}
	if err := s.repo.Update(ctx, sess); err != nil {
		return domain.Session{}, fmt.Errorf("session: update preferences: %w", err)
	}
	return sess, nil
}

// MarkOnboardingSeen — POST /sessions/onboarding-seen.
func (s *Service) MarkOnboardingSeen(ctx context.Context, id string) (domain.Session, error) {
	sess, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Session{}, fmt.Errorf("session: mark onboarding seen: %w", err)
	}
	sess.OnboardingSeen = true
	if err := s.repo.Update(ctx, sess); err != nil {
		return domain.Session{}, fmt.Errorf("session: mark onboarding seen: %w", err)
	}
	return sess, nil
}
