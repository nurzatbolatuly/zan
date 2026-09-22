package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/file"
)

func newTestFileAttachment(id, sessionID string, now time.Time) domain.FileAttachment {
	return domain.FileAttachment{
		ID:               id,
		SessionID:        sessionID,
		ObjectKey:        "uploads/" + sessionID + "/" + id + ".pdf",
		OriginalName:     "contract.pdf",
		MimeType:         "application/pdf",
		SizeBytes:        1024,
		Purpose:          domain.FilePurposeAnalysisInput,
		ProcessingStatus: domain.FileProcessingStatusPending,
		CreatedAt:        now,
	}
}

func TestFileRepo_Create_InsertsUnattachedFile(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-1111-1111-1111-111111111111", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	fileRepo := repo.NewFileRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, err := fileRepo.Create(context.Background(), newTestFileAttachment("22222222-2222-2222-2222-222222222222", sess.ID, now))
	require.NoError(t, err)
	require.False(t, created.IsAttached())
	require.Equal(t, domain.FileProcessingStatusPending, created.ProcessingStatus)
	require.Nil(t, created.ExtractedText)

	got, err := fileRepo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created, got)
}

func TestFileRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	fileRepo := repo.NewFileRepo(pool)

	_, err := fileRepo.GetByID(context.Background(), "99999999-9999-9999-9999-999999999999")
	require.ErrorIs(t, err, file.ErrNotFound)
}

func TestFileRepo_SetProcessingResult_UpdatesStatusAndText(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("33333333-1111-1111-1111-111111111111", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	fileRepo := repo.NewFileRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, err := fileRepo.Create(context.Background(), newTestFileAttachment("44444444-1111-1111-1111-111111111111", sess.ID, now))
	require.NoError(t, err)

	text := "extracted contract text"
	require.NoError(t, fileRepo.SetProcessingResult(context.Background(), created.ID, domain.FileProcessingStatusProcessed, &text))

	got, err := fileRepo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, domain.FileProcessingStatusProcessed, got.ProcessingStatus)
	require.Equal(t, text, *got.ExtractedText)
}

func TestFileRepo_ValidateAvailable_RejectsForeignAndAttachedFiles(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	owner := newTestSession("55555555-1111-1111-1111-111111111111", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), owner))
	other := newTestSession("66666666-1111-1111-1111-111111111111", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), other))

	fileRepo := repo.NewFileRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	available, err := fileRepo.Create(context.Background(), newTestFileAttachment("77777777-1111-1111-1111-111111111111", owner.ID, now))
	require.NoError(t, err)

	// Доступен своей сессии.
	require.NoError(t, fileRepo.ValidateAvailable(context.Background(), owner.ID, []string{available.ID}))

	// Недоступен чужой сессии.
	require.ErrorIs(t, fileRepo.ValidateAvailable(context.Background(), other.ID, []string{available.ID}), file.ErrNotFound)

	// Недоступен, если уже привязан к сообщению.
	threadRepo := repo.NewThreadRepo(pool)
	_, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("88888888-1111-1111-1111-111111111111", owner.ID, now),
		newTestUserMessage("99999999-1111-1111-1111-111111111111", now))
	require.NoError(t, err)
	require.NoError(t, fileRepo.AttachToMessage(context.Background(), msg.ID, []string{available.ID}))
	require.ErrorIs(t, fileRepo.ValidateAvailable(context.Background(), owner.ID, []string{available.ID}), file.ErrNotFound)
}

func TestFileRepo_AttachToMessage_SetsMessageID(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-2222-2222-2222-222222222222", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	fileRepo := repo.NewFileRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, err := fileRepo.Create(context.Background(), newTestFileAttachment("22222222-3333-3333-3333-333333333333", sess.ID, now))
	require.NoError(t, err)

	threadRepo := repo.NewThreadRepo(pool)
	_, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("33333333-4444-4444-4444-444444444444", sess.ID, now),
		newTestUserMessage("44444444-5555-5555-5555-555555555555", now))
	require.NoError(t, err)

	require.NoError(t, fileRepo.AttachToMessage(context.Background(), msg.ID, []string{created.ID}))

	got, err := fileRepo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.True(t, got.IsAttached())
	require.Equal(t, msg.ID, *got.MessageID)
}

// TestFileRepo_CreateGenerated_InsertsAndLinksToThread — Stage 6
// (BACKEND_PLAN.md): POST /threads/{id}/generate-document рендерит оба
// формата, каждый — своя строка core.file_attachments с общим thread_id/
// message_id (см. internal/service/document.Service.renderOne).
func TestFileRepo_CreateGenerated_InsertsAndLinksToThread(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-6666-6666-6666-666666666666", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	createdThread, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("22222222-6666-6666-6666-666666666666", sess.ID, now),
		newTestUserMessage("33333333-6666-6666-6666-666666666666", now))
	require.NoError(t, err)

	fileRepo := repo.NewFileRepo(pool)
	pdf := domain.FileAttachment{
		ID:               "44444444-6666-6666-6666-666666666666",
		SessionID:        sess.ID,
		MessageID:        &msg.ID,
		ThreadID:         &createdThread.ID,
		ObjectKey:        "generated/44444444-6666-6666-6666-666666666666.pdf",
		OriginalName:     "Документ.pdf",
		MimeType:         "application/pdf",
		Purpose:          domain.FilePurposeGeneratedOutput,
		OutputFormats:    []string{"pdf", "docx"},
		ProcessingStatus: domain.FileProcessingStatusProcessed,
		CreatedAt:        now,
	}
	created, err := fileRepo.CreateGenerated(context.Background(), pdf)
	require.NoError(t, err)
	require.Equal(t, createdThread.ID, *created.ThreadID)
	require.Equal(t, msg.ID, *created.MessageID)
	require.Equal(t, []string{"pdf", "docx"}, created.OutputFormats)

	got, err := fileRepo.GetLatestGenerated(context.Background(), createdThread.ID, "application/pdf")
	require.NoError(t, err)
	require.Equal(t, created, got)
}

// TestFileRepo_GetLatestGenerated_ReturnsMostRecentPerFormat — повторная
// генерация (второй вызов POST /threads/{id}/generate-document) не
// переиспользует старую строку — GetLatestGenerated должен вернуть именно
// последнюю по created_at.
func TestFileRepo_GetLatestGenerated_ReturnsMostRecentPerFormat(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-7777-7777-7777-777777777777", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	createdThread, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("22222222-7777-7777-7777-777777777777", sess.ID, now),
		newTestUserMessage("33333333-7777-7777-7777-777777777777", now))
	require.NoError(t, err)

	fileRepo := repo.NewFileRepo(pool)
	older := now.Add(-time.Hour)
	_, err = fileRepo.CreateGenerated(context.Background(), domain.FileAttachment{
		ID: "44444444-7777-7777-7777-777777777777", SessionID: sess.ID, MessageID: &msg.ID, ThreadID: &createdThread.ID,
		ObjectKey: "generated/old.pdf", OriginalName: "old.pdf", MimeType: "application/pdf",
		Purpose: domain.FilePurposeGeneratedOutput, OutputFormats: []string{"pdf", "docx"},
		ProcessingStatus: domain.FileProcessingStatusProcessed, CreatedAt: older,
	})
	require.NoError(t, err)
	newer, err := fileRepo.CreateGenerated(context.Background(), domain.FileAttachment{
		ID: "55555555-7777-7777-7777-777777777777", SessionID: sess.ID, MessageID: &msg.ID, ThreadID: &createdThread.ID,
		ObjectKey: "generated/new.pdf", OriginalName: "new.pdf", MimeType: "application/pdf",
		Purpose: domain.FilePurposeGeneratedOutput, OutputFormats: []string{"pdf", "docx"},
		ProcessingStatus: domain.FileProcessingStatusProcessed, CreatedAt: now,
	})
	require.NoError(t, err)

	got, err := fileRepo.GetLatestGenerated(context.Background(), createdThread.ID, "application/pdf")
	require.NoError(t, err)
	require.Equal(t, newer.ID, got.ID)
}

func TestFileRepo_GetLatestGenerated_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	fileRepo := repo.NewFileRepo(pool)

	_, err := fileRepo.GetLatestGenerated(context.Background(), "99999999-9999-9999-9999-999999999999", "application/pdf")
	require.ErrorIs(t, err, file.ErrNotFound)
}
