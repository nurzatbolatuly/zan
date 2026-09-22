// Package clamav — тонкий клиент протокола clamd INSTREAM (ClamAV),
// реализует internal/service/file.AVScanner (Stage 8, BACKEND_PLAN.md §6:
// "Security-обход" — заменяет прежнюю MVP-заглушку file.NoopAVScanner,
// которая ничего не сканировала и только логировала явный WARN на каждый
// вызов).
//
// Протокол clamd INSTREAM (docs.clamav.net/manual/Usage/Scanning.html):
// клиент открывает TCP-соединение, шлёт "zINSTREAM\0", затем данные
// чанками (4 байта big-endian длина + сами байты), завершает чанком
// нулевой длины; сервер отвечает одной строкой — "stream: OK",
// "stream: <virus name> FOUND" или "stream: <message> ERROR".
//
// Никакой сторонней библиотеки-клиента не подключаем — протокол
// укладывается в полсотни строк, тащить зависимость под это избыточно
// (BACKEND_CODING_STANDARDS.md §3).
package clamav

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// maxChunkSize — clamd по умолчанию ограничивает один INSTREAM-чанк ~25 МБ
// (StreamMaxLength в clamd.conf), но никакого смысла отправлять чанками
// больше нескольких КБ за раз нет — 4 КБ, тот же порядок, что и типичный
// сетевой MTU-буфер.
const maxChunkSize = 4096

// ErrInfected — файл распознан как заражённый (не ошибка инфраструктуры).
// internal/service/file.Service.Upload не различает эту причину отказа от
// любой другой ошибки Scan (fail-closed: недоступность clamd тоже
// блокирует загрузку) — но тест самого этого пакета обязан отличать
// "FOUND" от "ERROR"/сетевого сбоя.
var ErrInfected = errors.New("clamav: file infected")

// Scanner — реализует internal/service/file.AVScanner поверх clamd.
// Тестируется реальным TCP-листенером на loopback (тот же приём, что
// httptest.Server для HTTP-клиентов в этом кодбейзе, BACKEND_CODING_STANDARDS.md
// §10) — фейковый Dialer не нужен, адрес просто указывает на локальный
// фейковый clamd.
type Scanner struct {
	addr    string
	timeout time.Duration
}

// New — addr в форме "host:port" (docker-compose: "clamav:3310").
// timeout — на весь скан одного файла (соединение + передача + ответ), не
// на каждый чанк отдельно — тот же приём, что config.Config.LLMTimeout.
func New(addr string, timeout time.Duration) *Scanner {
	return &Scanner{addr: addr, timeout: timeout}
}

// Scan возвращает nil, если clamd признал файл чистым; ErrInfected, если
// найден вирус; любую другую ошибку — если сама проверка не состоялась
// (сеть/протокол) — вызывающий код (file.Service.Upload) в любом случае
// трактует ненулевую ошибку как отказ (fail-closed), но разница видна в
// логе (%w сохраняет исходную причину).
func (s *Scanner) Scan(ctx context.Context, data []byte) error {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("clamav: dial %s: %w", s.addr, err)
	}
	defer func() { _ = conn.Close() }()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	if err := s.stream(conn, data); err != nil {
		return err
	}

	// clamd отвечает одной строкой и закрывает соединение (не завершает её
	// '\n' надёжно во всех версиях) — читаем до EOF, не до разделителя.
	reply, err := io.ReadAll(conn)
	if err != nil {
		return fmt.Errorf("clamav: read reply: %w", err)
	}
	return parseReply(string(reply))
}

func (s *Scanner) stream(conn net.Conn, data []byte) error {
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("clamav: send INSTREAM: %w", err)
	}

	var lenBuf [4]byte
	for offset := 0; offset < len(data); offset += maxChunkSize {
		end := min(offset+maxChunkSize, len(data))
		chunk := data[offset:end]

		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(chunk))) //nolint:gosec // len(chunk) <= maxChunkSize (4096), overflow недостижим
		if _, err := conn.Write(lenBuf[:]); err != nil {
			return fmt.Errorf("clamav: send chunk length: %w", err)
		}
		if _, err := conn.Write(chunk); err != nil {
			return fmt.Errorf("clamav: send chunk: %w", err)
		}
	}

	// Завершающий чанк нулевой длины — сигнал clamd "конец потока".
	binary.BigEndian.PutUint32(lenBuf[:], 0)
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("clamav: send terminator: %w", err)
	}
	return nil
}

// parseReply — "stream: OK" (чисто), "stream: <name> FOUND" (заражён),
// "stream: <message> ERROR" (clamd не смог обработать поток — тоже отказ,
// не проглатываем как "чисто").
func parseReply(reply string) error {
	reply = strings.TrimRight(reply, "\x00\r\n")
	switch {
	case strings.HasSuffix(reply, "OK"):
		return nil
	case strings.HasSuffix(reply, "FOUND"):
		return fmt.Errorf("%w: %s", ErrInfected, reply)
	default:
		return fmt.Errorf("clamav: unexpected reply: %s", reply)
	}
}
