package voice_test

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/platform/clock"
	"zan-backend/internal/platform/idgen"
	"zan-backend/internal/service/voice"
)

type fakeStorage struct {
	deletedKeys []string
	deleteErr   error
}

func (s *fakeStorage) Put(context.Context, string, io.Reader, int64, string) error { return nil }

func (s *fakeStorage) PresignGetInternal(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.internal/" + key, nil
}

func (s *fakeStorage) Delete(_ context.Context, key string) error {
	s.deletedKeys = append(s.deletedKeys, key)
	return s.deleteErr
}

type fakeTranscriber struct {
	text     string
	err      error
	gotLang  domain.Language
	gotCalls int
}

func (t *fakeTranscriber) TranscribeAudio(_ context.Context, _ string, _ string, lang domain.Language) (string, error) {
	t.gotCalls++
	t.gotLang = lang
	return t.text, t.err
}

func newTestService(storage *fakeStorage, transcriber *fakeTranscriber) *voice.Service {
	return voice.New(storage, transcriber, clock.Real{}, &idgen.Fake{IDs: []string{"voice-1"}}, 15*1024*1024)
}

func TestService_Transcribe_RejectsUnsupportedMimeType(t *testing.T) {
	svc := newTestService(&fakeStorage{}, &fakeTranscriber{})

	_, err := svc.Transcribe(context.Background(), voice.TranscribeRequest{
		SessionID: "s1", MimeType: "video/mp4", Data: []byte("x"),
	})
	require.ErrorIs(t, err, voice.ErrUnsupportedMimeType)
}

func TestService_Transcribe_RejectsTooLarge(t *testing.T) {
	svc := voice.New(&fakeStorage{}, &fakeTranscriber{}, clock.Real{}, &idgen.Fake{IDs: []string{"voice-1"}}, 4)

	_, err := svc.Transcribe(context.Background(), voice.TranscribeRequest{
		SessionID: "s1", MimeType: "audio/webm", Data: []byte("12345"),
	})
	require.ErrorIs(t, err, voice.ErrTooLarge)
}

func TestService_Transcribe_ReturnsTextAndCleansUpTempFile(t *testing.T) {
	storage := &fakeStorage{}
	svc := newTestService(storage, &fakeTranscriber{text: "привет мир"})

	text, err := svc.Transcribe(context.Background(), voice.TranscribeRequest{
		SessionID: "s1", MimeType: "audio/webm", Lang: domain.LanguageRu, Data: []byte("audio bytes"),
	})
	require.NoError(t, err)
	require.Equal(t, "привет мир", text)
	require.Len(t, storage.deletedKeys, 1, "temp audio object must be deleted after successful transcribe")
}

func TestService_Transcribe_PassesLanguageHint(t *testing.T) {
	transcriber := &fakeTranscriber{text: "text"}
	svc := newTestService(&fakeStorage{}, transcriber)

	_, err := svc.Transcribe(context.Background(), voice.TranscribeRequest{
		SessionID: "s1", MimeType: "audio/webm", Lang: domain.LanguageKz, Data: []byte("x"),
	})
	require.NoError(t, err)
	require.Equal(t, domain.LanguageKz, transcriber.gotLang)
}

func TestService_Transcribe_EmptyTranscriptIsError(t *testing.T) {
	storage := &fakeStorage{}
	svc := newTestService(storage, &fakeTranscriber{text: ""})

	_, err := svc.Transcribe(context.Background(), voice.TranscribeRequest{
		SessionID: "s1", MimeType: "audio/webm", Data: []byte("silence"),
	})
	require.ErrorIs(t, err, voice.ErrEmptyTranscript)
	require.Len(t, storage.deletedKeys, 1, "temp file must still be cleaned up even when transcript is empty")
}

func TestService_Transcribe_PropagatesProviderError(t *testing.T) {
	storage := &fakeStorage{}
	providerErr := errors.New("stt provider unavailable")
	svc := newTestService(storage, &fakeTranscriber{err: providerErr})

	_, err := svc.Transcribe(context.Background(), voice.TranscribeRequest{
		SessionID: "s1", MimeType: "audio/webm", Data: []byte("x"),
	})
	require.ErrorIs(t, err, providerErr)
	require.Len(t, storage.deletedKeys, 1, "temp file cleanup must still run when the provider call fails")
}
