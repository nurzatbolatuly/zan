package grpcclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/test/bufconn"

	"zan-backend/internal/grpcclient"
)

// TestDial_EstablishesConnectionOverBufconn — на Stage 0 у helper ещё нет ни
// одного зарегистрированного бизнес-сервиса (см. BACKEND_PLAN.md, Stage 0
// DoD: "grpcurl list показывает пустой список"), поэтому тест проверяет то,
// что реально можно проверить сейчас — что Dial способен довести соединение
// до состояния READY на голом gRPC-сервере, не вызывая ни один RPC.
func TestDial_EstablishesConnectionOverBufconn(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)
	t.Cleanup(func() { _ = lis.Close() })

	server := grpc.NewServer()
	go func() {
		_ = server.Serve(lis)
	}()
	t.Cleanup(server.Stop)

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	// "passthrough:///" — иначе grpc резолвит "bufnet" через дефолтный DNS-
	// резолвер (который для этого имени ничего не найдёт) прежде, чем дойти
	// до WithContextDialer; passthrough отдаёт таргет как единственный адрес
	// напрямую, без сетевого резолва.
	conn, err := grpcclient.Dial("passthrough:///bufnet", grpc.WithContextDialer(dialer))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	conn.Connect()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return
		}
		if !conn.WaitForStateChange(ctx, state) {
			t.Fatalf("connection did not become ready in time, last state: %s", state)
		}
	}
}

// Примечание: тест на "Dial возвращает ошибку для невалидного таргета" сюда
// сознательно не добавлен — google.golang.org/grpc.NewClient не проверяет
// таргет синхронно ни при одной форме invalid-строки (эмпирически
// проверено), ошибка проявится только при попытке реального соединения
// (connectivity.TransientFailure), не в Dial(). Тест на несуществующий
// путь кода был бы тестом, который лжёт о том, что проверяет.
