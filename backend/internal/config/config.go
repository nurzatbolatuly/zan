// Package config собирает конфигурацию процесса из ENV один раз в
// composition root (cmd/api) и раздаётся дальше явным пробросом через
// конструкторы — см. BACKEND_CODING_STANDARDS.md §2 (никаких глобальных
// мутабельных переменных конфигурации).
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config — полный список переменных окружения сервиса backend на Stage 0.
// Секции, которых ещё нет (БД, S3, admin-secret, LLM-провайдер), добавляются
// в этот же struct по мере того, как их реально начинает читать код
// (Stage 1+) — не заводятся заранее "про запас".
type Config struct {
	// Env — имя окружения (dev/staging/prod), влияет только на поведение
	// логирования и диагностику, не на бизнес-логику.
	Env string `env:"ENV" envDefault:"dev"`

	// HTTPAddr — адрес, на котором backend слушает публичный REST/JSON API.
	HTTPAddr string `env:"HTTP_ADDR" envDefault:":8080"`

	// LogLevel — минимальный уровень логирования (debug|info|warn|error).
	// Контракт уровней — backend-roadmap.md §6.4.
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`

	// GRPCHelperTarget — адрес gRPC-сервера helper/ (host:port). Читается
	// уже на Stage 0 (нужен в docker-compose.yml), но пока никем не
	// используется — internal/grpcclient начинает реально дозваниваться
	// сюда с Stage 4/5 (BACKEND_PLAN.md), когда у helper появится первый
	// настоящий RPC.
	GRPCHelperTarget string `env:"GRPC_HELPER_TARGET" envDefault:"localhost:9090"`

	// DatabaseURL — DSN схемы core (BACKEND_PLAN.md §1.3). Дефолт совпадает
	// с DATABASE_URL по умолчанию в корневом Makefile (миграции гоняются
	// отдельным CLI golang-migrate, не этим процессом) — не секрет, просто
	// адрес локальной инфраструктуры, поэтому дефолт безопасен.
	DatabaseURL string `env:"DATABASE_URL" envDefault:"postgres://zan:zan@localhost:5432/zan?sslmode=disable"`

	// AdminToken — статичный секрет для X-Admin-Token на /admin/*
	// (zan-backend-tz-v3.md §2.3, BACKEND_PLAN.md §6 п.1). Без envDefault
	// намеренно: секрет обязан быть явно задан в каждом окружении, а не
	// молча взят из значения по умолчанию, которое легко забыть сменить.
	AdminToken string `env:"ADMIN_TOKEN,required"`

	// SessionHMACSecret — ключ подписи opaque-токена сессии
	// (zan-backend-tz-v3.md §2.1, BACKEND_PLAN.md §1.1). Тоже без
	// envDefault — та же причина, что у AdminToken.
	SessionHMACSecret string `env:"SESSION_HMAC_SECRET,required"`

	// ThreadFreeUntil — окно бесплатных уточнений внутри треда после
	// оплаты (zan-backend-tz-v2.md §4.3, BACKEND_PLAN.md §6 п.8: "72 часа,
	// конфигурируемо через ENV на случай пересмотра продуктом без
	// деплоя кода").
	ThreadFreeUntil time.Duration `env:"THREAD_FREE_UNTIL" envDefault:"72h"`

	// InternalSecret — общий секрет Go<->Python поверх сетевой изоляции
	// (BACKEND_PLAN.md §1: "shared secret — вторая линия обороны"),
	// внутренние вызовы аутентифицируются метадатой x-internal-secret.
	// Без envDefault — та же причина, что у AdminToken/SessionHMACSecret.
	InternalSecret string `env:"INTERNAL_SECRET,required"`

	// --- S3-совместимое хранилище файлов (Stage 4, BACKEND_PLAN.md §3.1) ---

	// S3Endpoint — адрес S3 API, каким его видят контейнеры backend/helper
	// в одной docker-сети ("http://minio:9000" в docker-compose). Используется
	// и для реальных операций (Put/Head/CreateBucket), и для presign-ссылок,
	// которые должен разыменовать helper/ (тот же docker-сеть — Go->Python
	// плечо контракта, BACKEND_PLAN.md §3.1: "файлы передаются по ссылке").
	S3Endpoint string `env:"S3_ENDPOINT" envDefault:"http://localhost:9000"`

	// S3PublicEndpoint — адрес, по которому presigned-ссылка разыменовывается
	// снаружи docker-сети (браузер клиента, `curl` из scripts/smoke-test.sh,
	// `go test` интеграционных тестов, запущенный на хосте) — в dev это тот
	// же MinIO, но опубликованный на localhost, а не по внутреннему DNS-имени
	// "minio", которое снаружи docker-сети не резолвится. Пусто по умолчанию —
	// падает обратно на S3Endpoint (прод, где один и тот же адрес виден и
	// изнутри, и снаружи, см. storage.Config.resolvePublicEndpoint).
	S3PublicEndpoint string `env:"S3_PUBLIC_ENDPOINT"`

	// S3Region — SigV4 требует непустой регион, MinIO сам его игнорирует.
	S3Region string `env:"S3_REGION" envDefault:"us-east-1"`

	// S3AccessKey/S3SecretKey — учётные данные MinIO (совпадают с
	// MINIO_ROOT_USER/MINIO_ROOT_PASSWORD в docker-compose.yml — не заводим
	// отдельный дублирующий набор переменных, тот же приём, что у DatabaseURL
	// и POSTGRES_*).
	S3AccessKey string `env:"S3_ACCESS_KEY,required"`
	S3SecretKey string `env:"S3_SECRET_KEY,required"`

	// S3Bucket — единый бакет на оба сервиса, вложенные файлы под "uploads/",
	// сгенерированные документы под "generated/" (BACKEND_PLAN.md §3.1).
	S3Bucket string `env:"S3_BUCKET" envDefault:"zan-files"`

	// --- Лимиты загрузки (BACKEND_PLAN.md §6 п.13, зафиксировано Stage 4) ---

	// FileMaxSizeBytes — лимит POST /files/upload (zan-backend-tz-v3.md §5.3).
	FileMaxSizeBytes int64 `env:"FILE_MAX_SIZE_BYTES" envDefault:"20971520"` // 20 МБ

	// VoiceMaxSizeBytes — лимит POST /voice/transcribe — короткая голосовая
	// заметка, не полноценный файл, лимит меньше файлового.
	VoiceMaxSizeBytes int64 `env:"VOICE_MAX_SIZE_BYTES" envDefault:"15728640"` // 15 МБ

	// --- LLM-провайдер (Stage 5, BACKEND_PLAN.md §3 — изначально
	// "Anthropic API", заменён на OpenAI по прямому запросу пользователя в
	// Stage 8; internal/agent не завязан на конкретного провайдера сильнее,
	// чем нужно — см. BACKEND_LOG.md) ---

	// OpenAIAPIKey — без envDefault намеренно, та же причина, что у
	// AdminToken/SessionHMACSecret/InternalSecret: секрет, который нельзя
	// молча взять из значения по умолчанию. Без реального ключа LLM-вызов
	// предсказуемо падает на 401 — обрабатывается тем же путём, что
	// таймаут/5xx (status=error, кредит возвращается), не роняет процесс.
	OpenAIAPIKey string `env:"OPENAI_API_KEY,required"`

	// OpenAIModel — конфигурируемо (не хардкод), чтобы смена модели не
	// требовала пересборки бинаря — тот же принцип, что WhisperModelSize
	// на стороне Python (BACKEND_PLAN.md §6 п.10).
	OpenAIModel string `env:"OPENAI_MODEL" envDefault:"gpt-4o"`

	// OpenAIBaseURL — переопределяется в тестах/локальной разработке на
	// адрес фейкового сервера (httptest) вместо реального api.openai.com —
	// internal/agent не завязан на конкретный хост.
	OpenAIBaseURL string `env:"OPENAI_BASE_URL" envDefault:"https://api.openai.com"`

	// OpenAIMaxTokens — лимит длины ответа модели (структурированный
	// JSON с answer_text/sources/findings — достаточно с запасом для
	// развёрнутого юридического ответа). Отправляется как
	// max_completion_tokens (internal/agent/llm_client.go), не устаревший
	// max_tokens Chat Completions API.
	OpenAIMaxTokens int `env:"OPENAI_MAX_TOKENS" envDefault:"2048"`

	// LLMTimeout — таймаут одной попытки HTTP-вызова к LLM-провайдеру (до
	// ретраев — internal/agent.llmRetryPolicy делает до 2 повторов поверх
	// этого таймаута на каждую попытку, backend-roadmap.md §5.2).
	LLMTimeout time.Duration `env:"LLM_TIMEOUT" envDefault:"30s"`

	// RagTopK — сколько кандидатов-источников запрашивать у
	// RagService.Search на каждый вызов LLM (BACKEND_PLAN.md §3.1: "Go сам
	// решает, что из этого передать в промпт").
	RagTopK int `env:"RAG_TOP_K" envDefault:"5"`

	// --- Антивирус (Stage 8, BACKEND_PLAN.md §6 "Security-обход") ---

	// ClamAVAddr — "host:port" clamd (internal/platform/clamav.Scanner).
	// Дефолт совпадает с именем сервиса в docker-compose.yml ("clamav:3310"
	// — тот же приём, что GRPCHelperTarget/S3Endpoint).
	ClamAVAddr string `env:"CLAMAV_ADDR" envDefault:"clamav:3310"`

	// ClamAVTimeout — на весь скан одного файла (соединение + передача +
	// ответ), не на каждый чанк отдельно.
	ClamAVTimeout time.Duration `env:"CLAMAV_TIMEOUT" envDefault:"10s"`

	// --- CORS (Stage 8, BACKEND_PLAN.md §6 п.3 "Деплой") ---

	// CORSAllowedOrigins — точные origin'ы задеплоенного фронта (схема+
	// хост+порт, через запятую — caarlos0/env парсит []string по запятой
	// по умолчанию), см. httpserver.CORS. Пусто по умолчанию — см.
	// комментарий у CORS() в internal/httpserver/middleware.go, почему это
	// безопасный дефолт, а не забытая настройка.
	CORSAllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS"`
}

// Load читает Config из переменных окружения процесса. Возвращает ошибку,
// а не паникует — невалидный конфиг это ожидаемый сценарий отказа при
// старте, который вызывающий код (main) решает, как обработать (см.
// BACKEND_CODING_STANDARDS.md §6.1 — panic допустим только для того, что
// уже поймано на старте, но сама загрузка конфига возвращает error, не
// паникует изнутри пакета).
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse env: %w", err)
	}
	if err := validateSecrets(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// minSecretLength — Stage 8 (BACKEND_PLAN.md §6 "сила admin-secret и
// x-internal-secret"): нижняя граница длины для секретов, перебор которых
// напрямую открывает доступ (X-Admin-Token, подпись сессии, gRPC-метадата
// между Go и Python). 32 байта — минимум, который делает брутфорс
// практически бессмысленным (не привязано к конкретному алгоритму хэширования,
// секреты сравниваются как значения, не как пароли с KDF) и легко
// генерируется (`openssl rand -hex 32` даёт ровно 64 печатных символа).
const minSecretLength = 32

// weakDefaultSecrets — плейсхолдеры из .env.example/docker-compose.yml —
// синтаксически проходят проверку длины (см. minSecretLength), но
// намеренно узнаваемы, чтобы их нельзя было случайно унести в прод
// копированием .env без замены (тот же сценарий, из-за которого
// AdminToken/SessionHMACSecret/InternalSecret не имеют envDefault).
var weakDefaultSecrets = map[string]bool{
	"devadmintokendevadmintokendevadmintoken1":            true,
	"devhmacsecretchangemedevhmacsecretchangeme1":         true,
	"devinternalsecretchangemedevinternalsecretchangeme1": true,
}

// validateSecrets — проверяется после env.Parse, а не через теги `env` —
// caarlos0/env не умеет "длина не меньше N" (только required/notEmpty),
// а протаскивать здесь произвольный validator-фреймворк ради трёх полей
// избыточно (BACKEND_CODING_STANDARDS.md §3: не тащить зависимость под то,
// что покрывается десятком строк).
func validateSecrets(cfg Config) error {
	for _, s := range []struct {
		name  string
		value string
	}{
		{"ADMIN_TOKEN", cfg.AdminToken},
		{"SESSION_HMAC_SECRET", cfg.SessionHMACSecret},
		{"INTERNAL_SECRET", cfg.InternalSecret},
	} {
		if len(s.value) < minSecretLength {
			return fmt.Errorf("config: %s must be at least %d characters (got %d) — generate one with `openssl rand -hex 32`",
				s.name, minSecretLength, len(s.value))
		}
		if weakDefaultSecrets[s.value] {
			return fmt.Errorf("config: %s is set to a known local-dev placeholder — generate a real secret with `openssl rand -hex 32`", s.name)
		}
	}
	return nil
}
