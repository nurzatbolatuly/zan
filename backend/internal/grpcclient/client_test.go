package grpcclient_test

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"zan-backend/internal/domain"
	zanv1 "zan-backend/internal/genproto/zan/rpc/v1"
	"zan-backend/internal/grpcclient"
	"zan-backend/internal/platform/logger"
	"zan-backend/internal/platform/resilience"
)

// fakeServer — реализует все три Stage-4 сервисера поверх настраиваемых
// колбэков, тот же паттерн, что BACKEND_CODING_STANDARDS.md §10 предписывает
// для тестов gRPC-клиента: "bufconn против тестового сервера-заглушки, без
// поднятия реального helper/ в юнит-тестах Go".
type fakeServer struct {
	zanv1.UnimplementedFilesServiceServer
	zanv1.UnimplementedDocumentsServiceServer
	zanv1.UnimplementedSttServiceServer
	zanv1.UnimplementedRagServiceServer

	extract     func(ctx context.Context, req *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error)
	transcribe  func(ctx context.Context, req *zanv1.TranscribeRequest) (*zanv1.TranscribeResponse, error)
	render      func(ctx context.Context, req *zanv1.RenderRequest) (*zanv1.RenderResponse, error)
	search      func(ctx context.Context, req *zanv1.SearchRequest) (*zanv1.SearchResponse, error)
	callCount   atomic.Int32
	lastMD      atomic.Pointer[metadata.MD]
	internalSec string
}

func (f *fakeServer) Extract(ctx context.Context, req *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error) {
	f.recordCall(ctx)
	return f.extract(ctx, req)
}

func (f *fakeServer) Transcribe(ctx context.Context, req *zanv1.TranscribeRequest) (*zanv1.TranscribeResponse, error) {
	f.recordCall(ctx)
	return f.transcribe(ctx, req)
}

func (f *fakeServer) Render(ctx context.Context, req *zanv1.RenderRequest) (*zanv1.RenderResponse, error) {
	f.recordCall(ctx)
	return f.render(ctx, req)
}

func (f *fakeServer) Search(ctx context.Context, req *zanv1.SearchRequest) (*zanv1.SearchResponse, error) {
	f.recordCall(ctx)
	return f.search(ctx, req)
}

func (f *fakeServer) recordCall(ctx context.Context) {
	f.callCount.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	mdCopy := md.Copy()
	f.lastMD.Store(&mdCopy)
}

// setupClient поднимает fakeServer поверх bufconn и возвращает готовый
// grpcclient.Client вместе с самим fakeServer (для проверки метадаты/числа
// вызовов) — тот же bufconn-паттерн, что dial_test.go использует для Dial.
func setupClient(t *testing.T, srv *fakeServer, breaker *resilience.Breaker) *grpcclient.Client {
	t.Helper()
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	t.Cleanup(func() { _ = lis.Close() })

	grpcServer := grpc.NewServer()
	zanv1.RegisterFilesServiceServer(grpcServer, srv)
	zanv1.RegisterDocumentsServiceServer(grpcServer, srv)
	zanv1.RegisterSttServiceServer(grpcServer, srv)
	zanv1.RegisterRagServiceServer(grpcServer, srv)
	go func() { _ = grpcServer.Serve(lis) }()
	t.Cleanup(grpcServer.Stop)

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpcclient.Dial("passthrough:///bufnet", grpc.WithContextDialer(dialer))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	if breaker == nil {
		breaker = resilience.NewBreaker(resilience.BreakerConfig{
			Name: "test", ConsecutiveFailuresToTrip: 100, OpenTimeout: time.Minute,
		})
	}
	return grpcclient.NewClient(conn, srv.internalSec, breaker)
}

func TestClient_ExtractFile_ReturnsText(t *testing.T) {
	srv := &fakeServer{internalSec: "test-internal-secret"}
	srv.extract = func(_ context.Context, req *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error) {
		require.Equal(t, "https://storage.internal/uploads/a.pdf", req.GetFileUrl())
		require.Equal(t, "application/pdf", req.GetMimeType())
		return &zanv1.ExtractResponse{Text: "extracted text"}, nil
	}
	client := setupClient(t, srv, nil)

	text, err := client.ExtractFile(context.Background(), "https://storage.internal/uploads/a.pdf", "application/pdf")
	require.NoError(t, err)
	require.Equal(t, "extracted text", text)
}

func TestClient_Call_PropagatesTraceIDAndInternalSecret(t *testing.T) {
	srv := &fakeServer{internalSec: "test-internal-secret"}
	srv.extract = func(context.Context, *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error) {
		return &zanv1.ExtractResponse{Text: "x"}, nil
	}
	client := setupClient(t, srv, nil)

	ctx := logger.WithTraceID(context.Background(), "trace-abc-123")
	_, err := client.ExtractFile(ctx, "https://storage.internal/x.pdf", "application/pdf")
	require.NoError(t, err)

	md := srv.lastMD.Load()
	require.NotNil(t, md)
	require.Equal(t, []string{"trace-abc-123"}, md.Get("x-trace-id"))
	require.Equal(t, []string{"test-internal-secret"}, md.Get("x-internal-secret"))
}

func TestClient_TranscribeAudio_SendsLang(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	var gotLang zanv1.Lang
	srv.transcribe = func(_ context.Context, req *zanv1.TranscribeRequest) (*zanv1.TranscribeResponse, error) {
		gotLang = req.GetLang()
		return &zanv1.TranscribeResponse{Text: "привет"}, nil
	}
	client := setupClient(t, srv, nil)

	text, err := client.TranscribeAudio(context.Background(), "https://storage.internal/a.webm", "audio/webm", domain.LanguageRu)
	require.NoError(t, err)
	require.Equal(t, "привет", text)
	require.Equal(t, zanv1.Lang_LANG_RU, gotLang)
}

func TestClient_RenderDocument_ReturnsFileURLAndObjectKey(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	srv.render = func(_ context.Context, req *zanv1.RenderRequest) (*zanv1.RenderResponse, error) {
		require.Equal(t, "Заявление", req.GetTitle())
		require.Len(t, req.GetSections(), 1)
		require.Equal(t, zanv1.RenderFormat_RENDER_FORMAT_PDF, req.GetFormat())
		return &zanv1.RenderResponse{FileUrl: "https://storage.public/generated/x.pdf", ObjectKey: "generated/x.pdf"}, nil
	}
	client := setupClient(t, srv, nil)

	result, err := client.RenderDocument(context.Background(), "Заявление",
		[]domain.Finding{{Title: "Пункт 1", Body: "текст"}}, grpcclient.RenderFormatPDF)
	require.NoError(t, err)
	require.Equal(t, "https://storage.public/generated/x.pdf", result.FileURL)
	require.Equal(t, "generated/x.pdf", result.ObjectKey)
}

func TestClient_SearchSources_ReturnsMatchesAndSendsTopK(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	srv.search = func(_ context.Context, req *zanv1.SearchRequest) (*zanv1.SearchResponse, error) {
		require.Equal(t, "какой срок исковой давности?", req.GetQueryText())
		require.Equal(t, int32(3), req.GetTopK())
		return &zanv1.SearchResponse{Matches: []*zanv1.SearchMatch{
			{Ref: "ст. 178 ГК РК", Quote: "три года", Score: 0.92},
		}}, nil
	}
	client := setupClient(t, srv, nil)

	matches, err := client.SearchSources(context.Background(), "какой срок исковой давности?", 3)

	require.NoError(t, err)
	require.Equal(t, []grpcclient.RagMatch{{Ref: "ст. 178 ГК РК", Quote: "три года", Score: 0.92}}, matches)
}

func TestClient_SearchSources_EmptyMatchesIsNotAnError(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	srv.search = func(context.Context, *zanv1.SearchRequest) (*zanv1.SearchResponse, error) {
		return &zanv1.SearchResponse{}, nil
	}
	client := setupClient(t, srv, nil)

	matches, err := client.SearchSources(context.Background(), "ничего похожего в корпусе", 5)

	require.NoError(t, err)
	require.Empty(t, matches)
}

func TestClient_SearchSources_PropagatesHelperUnavailable(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	srv.search = func(context.Context, *zanv1.SearchRequest) (*zanv1.SearchResponse, error) {
		return nil, status.Error(codes.Unavailable, "helper down")
	}
	client := setupClient(t, srv, nil)

	_, err := client.SearchSources(context.Background(), "вопрос", 5)

	require.Error(t, err)
	require.True(t, grpcclient.IsUnavailable(err))
}

func TestClient_ExtractFile_RetriesOnDeadlineExceeded(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	attempts := 0
	srv.extract = func(context.Context, *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error) {
		attempts++
		if attempts < 2 {
			return nil, status.Error(codes.DeadlineExceeded, "timeout")
		}
		return &zanv1.ExtractResponse{Text: "ok after retry"}, nil
	}
	client := setupClient(t, srv, nil)

	text, err := client.ExtractFile(context.Background(), "https://storage.internal/x.pdf", "application/pdf")
	require.NoError(t, err)
	require.Equal(t, "ok after retry", text)
	require.Equal(t, 2, attempts)
}

func TestClient_ExtractFile_DoesNotRetryOnInvalidArgument(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	attempts := 0
	srv.extract = func(context.Context, *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error) {
		attempts++
		return nil, status.Error(codes.InvalidArgument, "corrupted file")
	}
	client := setupClient(t, srv, nil)

	_, err := client.ExtractFile(context.Background(), "https://storage.internal/x.pdf", "application/pdf")
	require.Error(t, err)
	require.True(t, grpcclient.IsInvalidArgument(err))
	require.Equal(t, 1, attempts)
}

func TestClient_ExtractFile_BreakerOpensAfterRepeatedUnavailable(t *testing.T) {
	srv := &fakeServer{internalSec: "s"}
	srv.extract = func(context.Context, *zanv1.ExtractRequest) (*zanv1.ExtractResponse, error) {
		return nil, status.Error(codes.Unavailable, "helper down")
	}
	breaker := resilience.NewBreaker(resilience.BreakerConfig{
		Name: "test", ConsecutiveFailuresToTrip: 2, OpenTimeout: time.Minute,
	})
	client := setupClient(t, srv, breaker)

	for range 2 {
		_, err := client.ExtractFile(context.Background(), "https://storage.internal/x.pdf", "application/pdf")
		require.Error(t, err)
		require.True(t, grpcclient.IsUnavailable(err))
	}

	callsBefore := srv.callCount.Load()
	_, err := client.ExtractFile(context.Background(), "https://storage.internal/x.pdf", "application/pdf")
	require.Error(t, err)
	require.True(t, grpcclient.IsUnavailable(err))
	require.Equal(t, callsBefore, srv.callCount.Load(), "breaker open must short-circuit before reaching the server")
}
