package clamav_test

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/platform/clamav"
)

// fakeClamd — минимальный TCP-сервер, говорящий по протоколу clamd
// INSTREAM: читает "zINSTREAM\0", затем чанки (реконструирует исходные
// данные — тест проверяет, что клиент прислал именно то, что дали в Scan),
// затем отвечает reply. Один listener — один accept на тест.
func fakeClamd(t *testing.T, reply string) (addr string, received func() []byte) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	dataCh := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		r := bufio.NewReader(conn)
		header := make([]byte, len("zINSTREAM\x00"))
		if _, err := io.ReadFull(r, header); err != nil {
			return
		}

		var payload []byte
		for {
			var lenBuf [4]byte
			if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
				return
			}
			n := binary.BigEndian.Uint32(lenBuf[:])
			if n == 0 {
				break
			}
			chunk := make([]byte, n)
			if _, err := io.ReadFull(r, chunk); err != nil {
				return
			}
			payload = append(payload, chunk...)
		}
		dataCh <- payload

		_, _ = conn.Write([]byte(reply))
	}()

	return ln.Addr().String(), func() []byte {
		select {
		case d := <-dataCh:
			return d
		case <-time.After(time.Second):
			t.Fatal("fakeClamd: server never received a full stream")
			return nil
		}
	}
}

func TestScanner_Scan_Clean(t *testing.T) {
	addr, received := fakeClamd(t, "stream: OK\x00")
	s := clamav.New(addr, time.Second)

	data := []byte("hello, this is a clean test file")
	err := s.Scan(context.Background(), data)

	require.NoError(t, err)
	require.Equal(t, data, received())
}

func TestScanner_Scan_Infected(t *testing.T) {
	addr, received := fakeClamd(t, "stream: Eicar-Test-Signature FOUND\x00")
	s := clamav.New(addr, time.Second)

	data := []byte("fake eicar payload")
	err := s.Scan(context.Background(), data)

	require.Error(t, err)
	require.True(t, errors.Is(err, clamav.ErrInfected))
	require.Equal(t, data, received())
}

func TestScanner_Scan_ClamdError(t *testing.T) {
	addr, _ := fakeClamd(t, "stream: Size limit exceeded ERROR\x00")
	s := clamav.New(addr, time.Second)

	err := s.Scan(context.Background(), []byte("data"))

	require.Error(t, err)
	require.False(t, errors.Is(err, clamav.ErrInfected))
}

func TestScanner_Scan_ChunksLargerThanMaxChunkSize(t *testing.T) {
	addr, received := fakeClamd(t, "stream: OK\x00")
	s := clamav.New(addr, time.Second)

	// Больше одного чанка (maxChunkSize=4096) — проверяет, что реассемблинг
	// на "сервере" (и, значит, чанкинг на клиенте) не теряет и не
	// переставляет байты на границах чанков.
	data := make([]byte, 10_000)
	for i := range data {
		data[i] = byte(i % 251)
	}

	err := s.Scan(context.Background(), data)

	require.NoError(t, err)
	require.Equal(t, data, received())
}

func TestScanner_Scan_DialFailure(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close()) // порт свободен, но никто на нём не слушает

	s := clamav.New(addr, time.Second)
	err = s.Scan(context.Background(), []byte("data"))

	require.Error(t, err)
}
