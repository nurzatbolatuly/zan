// Command api — точка входа backend/ (composition root,
// BACKEND_CODING_STANDARDS.md §1.1): собирает конфиг/логгер/роутер вручную,
// без DI-фреймворка, и запускает HTTP-сервер с graceful shutdown.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"zan-backend/internal/agent"
	"zan-backend/internal/config"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/httpserver"
	"zan-backend/internal/platform/bruteforce"
	"zan-backend/internal/platform/clamav"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/ratelimit"
	"zan-backend/internal/platform/resilience"
	"zan-backend/internal/platform/storage"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/analytics"
	"zan-backend/internal/service/billing"
	"zan-backend/internal/service/catalog"
	"zan-backend/internal/service/clientlog"
	"zan-backend/internal/service/document"
	"zan-backend/internal/service/file"
	"zan-backend/internal/service/prompt"
	"zan-backend/internal/service/session"
	"zan-backend/internal/service/thread"
	"zan-backend/internal/service/voice"
)

// helperBreakerConfig — один circuit breaker на весь клиент к helper/
// (BACKEND_PLAN.md §3.3: отказ одного RPC значит, что лежит весь процесс
// Python, не конкретный сервис внутри него). Конкретные цифры — рекомендация
// архитектора для MVP (тот же приём, что sessionCreateRatePerSecond ниже),
// пересмотреть по метрикам прод.
var helperBreakerConfig = resilience.BreakerConfig{
	Name:                      "helper",
	ConsecutiveFailuresToTrip: 5,
	OpenTimeout:               30 * time.Second,
}

// llmBreakerConfig — отдельный breaker от helperBreakerConfig (Stage 5):
// отказ Anthropic API и отказ helper/ — независимые отказные домены, один
// не должен размыкать breaker другого. Конкретные цифры — та же
// рекомендация архитектора для MVP, что у helperBreakerConfig.
var llmBreakerConfig = resilience.BreakerConfig{
	Name:                      "anthropic",
	ConsecutiveFailuresToTrip: 5,
	OpenTimeout:               30 * time.Second,
}

// sessionCreateRateLimit — барьер против спама на POST /sessions
// (BACKEND_PLAN.md, Stage 1): создание сессии — редкое действие одного
// реального пользователя, не то, что легитимно вызывается пачками с
// одного IP. Конкретные цифры не заданы продуктом — рекомендация
// архитектора для MVP, пересмотреть по метрикам прод (тот же принцип, что
// у BACKEND_PLAN.md §6 п.12 про polling/SSE).
const (
	sessionCreateRatePerSecond = rate.Limit(1.0 / 10.0) // 1 запрос/10 секунд в устойчивом режиме
	sessionCreateRateBurst     = 5
)

// threadCreateRateLimit — Stage 7 (zan-backend-tz-v3.md §5.7): POST /threads
// списывает баланс и синхронно вызывает LLM — заметно дороже, чем
// POST /sessions, но легитимный пользователь всё же создаёт несколько
// тредов подряд (разные вопросы) чаще, чем создаёт сессии. По IP — щедрее,
// чем per-session (общий барьер на много сессий за одним NAT); по
// session_id — строже (одна реальная переписка не создаёт тред раз в
// секунду). Оба — рекомендация архитектора для MVP, не заданы продуктом,
// пересмотреть по метрикам прод (тот же принцип, что sessionCreateRateLimit).
const (
	threadCreateRatePerSecondByIP = rate.Limit(1.0)
	threadCreateRateBurstByIP     = 10
	threadCreateRatePerSession    = rate.Limit(1.0 / 5.0) // 1 запрос/5 секунд
	threadCreateRateBurstSession  = 3
)

// clientLogRateLimit — Stage 7 (zan-backend-tz-v3.md §6.3: "без rate-лимита
// ниже разумного, но с базовой защитой от спама") — щедрее любого другого
// лимита: страница фронта может залогировать несколько независимых событий
// за секунду (например, каскад ошибок), это не должно тонуть в 429.
const (
	clientLogRatePerSecond = rate.Limit(5.0)
	clientLogRateBurst     = 20
)

// adminBruteforce — Stage 7 (zan-backend-tz-v3.md §5.7): блокировка IP
// после N неудачных попыток подряд на /admin/*. Цифры — рекомендация
// архитектора для MVP: 5 неудач подряд — заведомо больше, чем случайная
// опечатка легитимного администратора, но телом на порядки меньше, чем
// нужно для эффективного перебора секрета; 15 минут блокировки достаточно,
// чтобы сделать перебор непрактичным, не наказывая администратора на весь день.
const (
	adminMaxConsecutiveFailures = 5
	adminBlockDuration          = 15 * time.Minute
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		cfg, err := config.Load()
		if err != nil {
			os.Exit(1)
		}
		os.Exit(runHealthcheck(cfg.HTTPAddr))
	}

	if err := run(); err != nil {
		slog.Error("backend exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	level, err := logger.ParseLevel(cfg.LogLevel)
	if err != nil {
		return err
	}
	l := logger.New(os.Stdout, level)
	slog.SetDefault(l)

	l.Info("starting", slog.String("env", cfg.Env), slog.String("http_addr", cfg.HTTPAddr))

	dbCtx, cancelDB := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := pgxpool.New(dbCtx, cfg.DatabaseURL)
	cancelDB()
	if err != nil {
		return err
	}
	defer pool.Close()

	storageCtx, cancelStorage := context.WithTimeout(context.Background(), 10*time.Second)
	s3Client, err := storage.New(storageCtx, storage.Config{
		Endpoint:       cfg.S3Endpoint,
		PublicEndpoint: cfg.S3PublicEndpoint,
		Region:         cfg.S3Region,
		AccessKey:      cfg.S3AccessKey,
		SecretKey:      cfg.S3SecretKey,
		Bucket:         cfg.S3Bucket,
	})
	cancelStorage()
	if err != nil {
		return err
	}

	helperConn, err := grpcclient.Dial(cfg.GRPCHelperTarget)
	if err != nil {
		return err
	}
	defer func() { _ = helperConn.Close() }()
	helperClient := grpcclient.NewClient(helperConn, cfg.InternalSecret, resilience.NewBreaker(helperBreakerConfig))

	sessions := session.New(
		repo.NewSessionRepo(pool),
		clock.Real{},
		idgen.UUIDGenerator{},
		session.NewTokenSigner(cfg.SessionHMACSecret),
	)
	catalogSvc := catalog.New(repo.NewCatalogRepo(pool), idgen.UUIDGenerator{})
	billingSvc := billing.New(repo.NewBillingRepo(pool, idgen.UUIDGenerator{}), catalogSvc, clock.Real{}, idgen.UUIDGenerator{})
	fileSvc := file.New(
		repo.NewFileRepo(pool),
		s3Client,
		helperClient,
		clamav.New(cfg.ClamAVAddr, cfg.ClamAVTimeout),
		clock.Real{},
		idgen.UUIDGenerator{},
		cfg.FileMaxSizeBytes,
	)
	voiceSvc := voice.New(s3Client, helperClient, clock.Real{}, idgen.UUIDGenerator{}, cfg.VoiceMaxSizeBytes)
	promptSvc := prompt.New(repo.NewPromptRepo(pool), clock.Real{})
	// agent.NewClient(...) — Stage 5, заменяет thread.NewStubAgent() (Stage 3)
	// за тем же портом thread.Agent, без изменений в остальной сборке
	// зависимостей (Strategy, BACKEND_PLAN.md). helperClient (*grpcclient.Client)
	// удовлетворяет agent.RagSearcher структурно, promptSvc — agent.PromptProvider.
	agentClient := agent.NewClient(promptSvc, helperClient, agent.Config{
		BaseURL:     cfg.AnthropicBaseURL,
		APIKey:      cfg.AnthropicAPIKey,
		Model:       cfg.AnthropicModel,
		MaxTokens:   cfg.AnthropicMaxTokens,
		HTTPTimeout: cfg.LLMTimeout,
		RagTopK:     cfg.RagTopK,
		Breaker:     llmBreakerConfig,
	})
	threadSvc := thread.New(
		repo.NewThreadRepo(pool),
		billingSvc,
		catalogSvc,
		fileSvc,
		agentClient,
		clock.Real{},
		idgen.UUIDGenerator{},
		cfg.ThreadFreeUntil,
	)
	// documentSvc — Stage 6 ("Агент 'Документы'"): собственные экземпляры
	// repo.NewThreadRepo/repo.NewFileRepo (тонкие обёртки над тем же pool,
	// без состояния — тот же приём, что и у остальных service.New(...) выше,
	// не переиспользуют repo threadSvc/fileSvc). agentClient удовлетворяет
	// document.Generator (GenerateDocument), helperClient — document.Renderer
	// (RenderDocument, Stage 4), s3Client — document.Storage (PresignGetPublic)
	// структурно, без адаптеров.
	documentSvc := document.New(
		repo.NewThreadRepo(pool),
		repo.NewFileRepo(pool),
		billingSvc,
		catalogSvc,
		agentClient,
		helperClient,
		s3Client,
		clock.Real{},
		idgen.UUIDGenerator{},
	)
	analyticsSvc := analytics.New(repo.NewAnalyticsRepo(pool))
	clientLogSvc := clientlog.New()

	router := httpserver.NewRouter(l, httpserver.Deps{
		Sessions:          sessions,
		Catalog:           catalogSvc,
		Billing:           billingSvc,
		Thread:            threadSvc,
		Documents:         documentSvc,
		Files:             fileSvc,
		Voice:             voiceSvc,
		Prompts:           promptSvc,
		Analytics:         analyticsSvc,
		ClientLogs:        clientLogSvc,
		FileMaxSizeBytes:  cfg.FileMaxSizeBytes,
		VoiceMaxSizeBytes: cfg.VoiceMaxSizeBytes,
		// Secure-cookie требует HTTPS — в dev локальный стек поднят по
		// голому HTTP (docker-compose), браузер такую cookie не примет.
		AdminToken:             cfg.AdminToken,
		SessionCookieSecure:    cfg.Env != "dev",
		SessionRateLimit:       ratelimit.NewStore(sessionCreateRatePerSecond, sessionCreateRateBurst),
		ThreadRateLimit:        ratelimit.NewStore(threadCreateRatePerSecondByIP, threadCreateRateBurstByIP),
		ThreadSessionRateLimit: ratelimit.NewStore(threadCreateRatePerSession, threadCreateRateBurstSession),
		ClientLogRateLimit:     ratelimit.NewStore(clientLogRatePerSecond, clientLogRateBurst),
		AdminBruteforce:        bruteforce.NewStore(adminMaxConsecutiveFailures, adminBlockDuration, clock.Real{}),
		CORSAllowedOrigins:     cfg.CORSAllowedOrigins,
	})
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second, // защита от Slowloris — клиент не может держать соединение открытым бесконечно, не досылая заголовки
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runStalePaymentExpirer(ctx, l, billingSvc)

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case <-ctx.Done():
		l.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	l.Info("shutdown complete")
	return nil
}
