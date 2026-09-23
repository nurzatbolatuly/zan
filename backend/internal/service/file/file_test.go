package file_test

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
	"zan-backend/internal/service/file"
)

// fakeRepo — in-memory реализация file.Repository (BACKEND_CODING_STANDARDS.md
// §10: юниты без БД, реальная семантика — internal/repo/file_repo_test.go).
type fakeRepo struct {
	files map[string]domain.FileAttachment
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{files: make(map[string]domain.FileAttachment)}
}

func (r *fakeRepo) Create(_ context.Context, f domain.FileAttachment) (domain.FileAttachment, error) {
	r.files[f.ID] = f
	return f, nil
}

func (r *fakeRepo) GetByID(_ context.Context, id string) (domain.FileAttachment, error) {
	f, ok := r.files[id]
	if !ok {
		return domain.FileAttachment{}, file.ErrNotFound
	}
	return f, nil
}

func (r *fakeRepo) SetProcessingResult(_ context.Context, id string, status domain.FileProcessingStatus, extractedText *string) error {
	f := r.files[id]
	f.ProcessingStatus = status
	f.ExtractedText = extractedText
	r.files[id] = f
	return nil
}

func (r *fakeRepo) ValidateAvailable(_ context.Context, sessionID string, ids []string) error {
	for _, id := range ids {
		f, ok := r.files[id]
		if !ok || f.SessionID != sessionID || f.IsAttached() {
			return file.ErrNotFound
		}
	}
	return nil
}

func (r *fakeRepo) AttachToMessage(_ context.Context, messageID string, ids []string) error {
	for _, id := range ids {
		f := r.files[id]
		f.MessageID = &messageID
		r.files[id] = f
	}
	return nil
}

type fakeStorage struct {
	putCalls int
	putErr   error
}

func (s *fakeStorage) Put(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	s.putCalls++
	return s.putErr
}

func (s *fakeStorage) PresignGetPublic(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.public/" + key, nil
}

func (s *fakeStorage) PresignGetInternal(_ context.Context, key string, _ time.Duration) (string, error) {
	return "https://storage.internal/" + key, nil
}

type fakeExtractor struct {
	text  string
	err   error
	calls int
}

func (e *fakeExtractor) ExtractFile(context.Context, string, string) (string, error) {
	e.calls++
	return e.text, e.err
}

type fakeAVScanner struct {
	rejectErr error
}

func (s *fakeAVScanner) Scan(context.Context, []byte) error {
	return s.rejectErr
}

func newTestService(repo *fakeRepo, storage *fakeStorage, extractor *fakeExtractor, av *fakeAVScanner) *file.Service {
	return file.New(repo, storage, extractor, av, clock.Fake{T: time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)}, &idgen.Fake{IDs: []string{"file-1", "file-2"}}, 20*1024*1024)
}

func TestService_Upload_AcceptsAnyMimeType(t *testing.T) {
	tests := []struct {
		name         string
		mimeType     string
		wantMimeType string
	}{
		{name: "known but not extractable", mimeType: "application/zip", wantMimeType: "application/zip"},
		{name: "empty content type", mimeType: "", wantMimeType: "application/octet-stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := &fakeStorage{}
			extractor := &fakeExtractor{text: "must not be used"}
			svc := newTestService(newFakeRepo(), storage, extractor, &fakeAVScanner{})

			created, _, err := svc.Upload(context.Background(), file.UploadRequest{
				SessionID: "s1", OriginalName: "archive.bin", MimeType: tt.mimeType, Data: []byte("x"),
			})
			require.NoError(t, err)
			require.Equal(t, tt.wantMimeType, created.MimeType)
			require.Equal(t, 1, storage.putCalls)
			require.Zero(t, extractor.calls, "non-extractable type must not reach helper/")
			require.Equal(t, domain.FileProcessingStatusError, created.ProcessingStatus)
			require.Nil(t, created.ExtractedText)
		})
	}
}

func TestService_Upload_RejectsTooLarge(t *testing.T) {
	svc := file.New(newFakeRepo(), &fakeStorage{}, &fakeExtractor{}, &fakeAVScanner{}, clock.Real{}, &idgen.Fake{IDs: []string{"file-1"}}, 4)

	_, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "s1", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("12345"),
	})
	require.ErrorIs(t, err, file.ErrTooLarge)
}

func TestService_Upload_RejectsAVScanFailure(t *testing.T) {
	storage := &fakeStorage{}
	svc := newTestService(newFakeRepo(), storage, &fakeExtractor{}, &fakeAVScanner{rejectErr: errors.New("infected")})

	_, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "s1", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("x"),
	})
	require.ErrorIs(t, err, file.ErrRejectedByAVScanner)
	require.Zero(t, storage.putCalls, "must not upload to storage after AV rejection")
}

func TestService_Upload_SuccessfulExtractSetsProcessedStatus(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, &fakeExtractor{text: "extracted content"}, &fakeAVScanner{})

	created, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "s1", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("pdf bytes"),
	})
	require.NoError(t, err)
	require.Equal(t, domain.FileProcessingStatusProcessed, created.ProcessingStatus)
	require.NotNil(t, created.ExtractedText)
	require.Equal(t, "extracted content", *created.ExtractedText)

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, domain.FileProcessingStatusProcessed, stored.ProcessingStatus)
}

func TestService_Upload_ExtractionFailureStillSavesUpload(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, &fakeExtractor{err: errors.New("corrupted pdf")}, &fakeAVScanner{})

	created, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "s1", OriginalName: "broken.pdf", MimeType: "application/pdf", Data: []byte("garbage"),
	})
	require.NoError(t, err, "upload itself must succeed even when extraction fails")
	require.Equal(t, domain.FileProcessingStatusError, created.ProcessingStatus)
	require.Nil(t, created.ExtractedText)
}

func TestService_GetByID_NotFoundForOtherSession(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, &fakeExtractor{text: "x"}, &fakeAVScanner{})

	created, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "owner", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("x"),
	})
	require.NoError(t, err)

	_, _, err = svc.GetByID(context.Background(), "someone-else", created.ID)
	require.ErrorIs(t, err, file.ErrNotFound)
}

func TestService_GetByID_ReturnsFreshPresignedURL(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, &fakeExtractor{text: "x"}, &fakeAVScanner{})

	created, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "owner", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("x"),
	})
	require.NoError(t, err)

	got, url, err := svc.GetByID(context.Background(), "owner", created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Contains(t, url, created.ObjectKey)
}

func TestService_ValidateAvailable_EmptyListIsNoop(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeStorage{}, &fakeExtractor{}, &fakeAVScanner{})
	require.NoError(t, svc.ValidateAvailable(context.Background(), "s1", nil))
}

func TestService_ValidateAvailable_RejectsFileFromOtherSession(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, &fakeExtractor{text: "x"}, &fakeAVScanner{})

	created, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "owner", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("x"),
	})
	require.NoError(t, err)

	err = svc.ValidateAvailable(context.Background(), "someone-else", []string{created.ID})
	require.ErrorIs(t, err, file.ErrNotFound)
}

func TestService_AttachToMessage_SetsMessageID(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeStorage{}, &fakeExtractor{text: "x"}, &fakeAVScanner{})

	created, _, err := svc.Upload(context.Background(), file.UploadRequest{
		SessionID: "owner", OriginalName: "a.pdf", MimeType: "application/pdf", Data: []byte("x"),
	})
	require.NoError(t, err)

	require.NoError(t, svc.AttachToMessage(context.Background(), "msg-1", []string{created.ID}))

	stored, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.True(t, stored.IsAttached())
	require.Equal(t, "msg-1", *stored.MessageID)
}
