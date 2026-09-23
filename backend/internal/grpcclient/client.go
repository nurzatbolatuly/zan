package grpcclient

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"zan-backend/internal/domain"
	zanv1 "zan-backend/internal/genproto/zan/rpc/v1"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/resilience"
)

const (
	internalSecretMetadataKey = "x-internal-secret" //nolint:gosec // имя метадата-заголовка, не сам секрет
	traceIDMetadataKey        = "x-trace-id"
)

// retryPolicy — до 2 повторов сверх первой попытки, и только на
// DEADLINE_EXCEEDED (BACKEND_PLAN.md §3.3: "DEADLINE_EXCEEDED — Go делает
// ретраи"; zan-backend-tz-v3.md §5.2: "таймаут... до 2 повторов с backoff").
// UNAVAILABLE не ретраится здесь — это сигнал для circuit breaker (см.
// Client.call), INVALID_ARGUMENT/INTERNAL не ретраятся вовсе (не помогут).
var retryPolicy = resilience.RetryPolicy{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond}

// Client — обёртка над сгенерированными клиентами FilesService/
// DocumentsService/SttService (BACKEND_CODING_STANDARDS.md §7.3.3). Один
// Breaker на всех трёх намеренно — отказ одного RPC
// helper/ значит, что лежит весь процесс Python, не конкретный сервис
// внутри него.
type Client struct {
	files          zanv1.FilesServiceClient
	documents      zanv1.DocumentsServiceClient
	stt            zanv1.SttServiceClient
	breaker        *resilience.Breaker
	internalSecret string
}

// NewClient строит Client поверх уже установленного соединения (grpcclient.Dial).
func NewClient(conn *grpc.ClientConn, internalSecret string, breaker *resilience.Breaker) *Client {
	return &Client{
		files:          zanv1.NewFilesServiceClient(conn),
		documents:      zanv1.NewDocumentsServiceClient(conn),
		stt:            zanv1.NewSttServiceClient(conn),
		breaker:        breaker,
		internalSecret: internalSecret,
	}
}

// ExtractFile — FilesService.Extract (BACKEND_PLAN.md Stage 4). fileURL —
// presigned-ссылка на собственный storage backend'а (см.
// internal/platform/storage.Client.PresignGetInternal), не то, что клиент
// передал сам — SSRF-защита обеспечивается тем, что Python получает
// исключительно ссылки, которые сгенерировал сам backend.
func (c *Client) ExtractFile(ctx context.Context, fileURL, mimeType string) (string, error) {
	var resp *zanv1.ExtractResponse
	err := c.call(ctx, "FilesService/Extract", func(ctx context.Context) error {
		var err error
		resp, err = c.files.Extract(ctx, &zanv1.ExtractRequest{FileUrl: fileURL, MimeType: mimeType})
		return err
	})
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

// TranscribeAudio — SttService.Transcribe (BACKEND_PLAN.md Stage 4).
func (c *Client) TranscribeAudio(ctx context.Context, fileURL, mimeType string, lang domain.Language) (string, error) {
	var resp *zanv1.TranscribeResponse
	err := c.call(ctx, "SttService/Transcribe", func(ctx context.Context) error {
		var err error
		resp, err = c.stt.Transcribe(ctx, &zanv1.TranscribeRequest{
			FileUrl:  fileURL,
			MimeType: mimeType,
			Lang:     toProtoLang(lang),
		})
		return err
	})
	if err != nil {
		return "", err
	}
	return resp.GetText(), nil
}

// RenderFormat — формат рендера документа, не зависит от сгенерированного
// zanv1.RenderFormat (BACKEND_CODING_STANDARDS.md §1.2: wire-типы не текут
// дальше границы клиента/сервисера).
type RenderFormat string

const (
	RenderFormatPDF  RenderFormat = "pdf"
	RenderFormatDOCX RenderFormat = "docx"
)

// RenderResult — см. documents.proto RenderResponse: FileURL готов к
// немедленной отдаче, ObjectKey — стабильная ссылка для того, кто должен
// пережить истечение presigned-ссылки (Stage 6).
type RenderResult struct {
	FileURL   string
	ObjectKey string
}

// RenderDocument — DocumentsService.Render (BACKEND_PLAN.md Stage 4 —
// базовый рендер-пайплайн; вызывающий юзкейс "Агент Документы" — Stage 6).
func (c *Client) RenderDocument(ctx context.Context, title string, sections []domain.Finding, format RenderFormat) (RenderResult, error) {
	protoFormat, err := toProtoRenderFormat(format)
	if err != nil {
		return RenderResult{}, err
	}
	protoSections := make([]*zanv1.DocumentSection, len(sections))
	for i, s := range sections {
		protoSections[i] = &zanv1.DocumentSection{Title: s.Title, Body: s.Body}
	}

	var resp *zanv1.RenderResponse
	err = c.call(ctx, "DocumentsService/Render", func(ctx context.Context) error {
		var err error
		resp, err = c.documents.Render(ctx, &zanv1.RenderRequest{
			Title:    title,
			Sections: protoSections,
			Format:   protoFormat,
		})
		return err
	})
	if err != nil {
		return RenderResult{}, err
	}
	return RenderResult{FileURL: resp.GetFileUrl(), ObjectKey: resp.GetObjectKey()}, nil
}

// call — единая точка сквозных забот клиента к helper/
// (BACKEND_CODING_STANDARDS.md §7): метадата (trace_id/x-internal-secret),
// логирование старта/итога вызова, ретраи, circuit breaker. method — только
// для логов ("FilesService/Extract" и т.п.), не влияет на маршрутизацию.
func (c *Client) call(ctx context.Context, method string, fn func(ctx context.Context) error) error {
	l := logger.FromContext(ctx)
	l.Info("grpc_call_started", slog.Group("context", slog.String("method", method)))
	started := time.Now()

	err := c.breaker.Execute(ctx, func(ctx context.Context) error {
		return retryPolicy.Do(ctx, shouldRetry, func(ctx context.Context) error {
			return fn(c.withOutgoingMetadata(ctx))
		})
	})

	durationMs := time.Since(started).Milliseconds()
	if err != nil {
		l.Error("grpc_call_failed", slog.Group("context",
			slog.String("method", method),
			slog.Int64("duration_ms", durationMs),
			slog.String("error", err.Error()),
		))
		return err
	}
	l.Info("grpc_call_succeeded", slog.Group("context",
		slog.String("method", method),
		slog.Int64("duration_ms", durationMs),
	))
	return nil
}

func (c *Client) withOutgoingMetadata(ctx context.Context) context.Context {
	ctx = metadata.AppendToOutgoingContext(ctx, internalSecretMetadataKey, c.internalSecret)
	if traceID, ok := logger.TraceIDFromContext(ctx); ok && traceID != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, traceIDMetadataKey, traceID)
	}
	return ctx
}

func toProtoLang(lang domain.Language) zanv1.Lang {
	switch lang {
	case domain.LanguageRu:
		return zanv1.Lang_LANG_RU
	case domain.LanguageKz:
		return zanv1.Lang_LANG_KZ
	default:
		return zanv1.Lang_LANG_UNSPECIFIED
	}
}

func toProtoRenderFormat(format RenderFormat) (zanv1.RenderFormat, error) {
	switch format {
	case RenderFormatPDF:
		return zanv1.RenderFormat_RENDER_FORMAT_PDF, nil
	case RenderFormatDOCX:
		return zanv1.RenderFormat_RENDER_FORMAT_DOCX, nil
	default:
		return zanv1.RenderFormat_RENDER_FORMAT_UNSPECIFIED, &UnsupportedRenderFormatError{Format: format}
	}
}

// UnsupportedRenderFormatError — невалидный RenderFormat дошёл до клиента
// (программная ошибка вызывающего кода, не пользовательский ввод — HTTP-слой
// Stage 6 обязан провалидировать format ещё на границе запроса).
type UnsupportedRenderFormatError struct {
	Format RenderFormat
}

func (e *UnsupportedRenderFormatError) Error() string {
	return "grpcclient: unsupported render format " + string(e.Format)
}
