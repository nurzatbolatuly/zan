// Package grpcclient — связь backend/ с helper/ (BACKEND_PLAN.md §3):
// единая точка client-side interceptor'ов (ретраи/circuit breaker/лог/
// trace-id), которые появятся здесь на Stage 4/5 вместе с первым реальным
// RPC (RagService/FilesService/DocumentsService/SttService). На Stage 0
// helper ещё не отдаёт ни одного бизнес-сервиса — пакет несёт только Dial,
// который Stage 4/5 переиспользует не переписывая.
package grpcclient

import (
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// Dial открывает gRPC-соединение с helper/. Транспорт по умолчанию —
// insecure: оба сервиса живут в одной приватной docker/VPC-сети, не за
// публичным ingress (BACKEND_PLAN.md §1.2 — "сервис недоступен из
// публичного ingress... shared secret — вторая линия обороны"), TLS между
// ними на этом этапе избыточен. Keepalive — чтобы разрыв соединения
// обнаруживался быстро, а не только по таймауту следующего вызова.
//
// grpc.NewClient (в отличие от устаревшего grpc.Dial) не блокируется и не
// устанавливает соединение синхронно — оно поднимается лениво при первом
// RPC, поэтому Dial не принимает context.Context, звонить нечему до первого
// вызова. extraOpts — точка расширения для тестов (например, свой dialer
// поверх bufconn, см. dial_test.go), а не задел под гипотетическое будущее.
//
// maxMessageSize — дефолт grpc-go (4 МБ) слишком мал для FilesService.Extract:
// извлечённый текст большого PDF/скана может превысить его, хотя сам файл
// передаётся по ссылке, а не в сообщении (BACKEND_PLAN.md §3.1). Явный
// больший лимит — осознанный выбор, закрывает часть открытого пункта
// BACKEND_PLAN.md §8 "лимиты на размер unary-сообщений gRPC" уже на Stage 4,
// не откладывая до Stage 8.
const maxMessageSize = 16 * 1024 * 1024 // 16 МБ

func Dial(target string, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := append([]grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                20 * time.Second,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxMessageSize),
			grpc.MaxCallSendMsgSize(maxMessageSize),
		),
	}, extraOpts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: dial %q: %w", target, err)
	}
	return conn, nil
}
