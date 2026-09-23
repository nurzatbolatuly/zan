package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
	"zan-backend/internal/service/thread"
)

func newTestThread(id, sessionID string, now time.Time) domain.Thread {
	return domain.Thread{
		ID:            id,
		SessionID:     sessionID,
		ServiceID:     "qa",
		Status:        domain.ThreadStatusAwaitingPayment,
		Title:         "Как оформить развод?",
		PreviewText:   "Ожидает оплаты",
		MessageCount:  1,
		CreatedAt:     now,
		LastMessageAt: &now,
	}
}

func newTestUserMessage(id string, now time.Time) domain.Message {
	return domain.Message{
		ID:        id,
		Sender:    domain.MessageSenderUser,
		InputType: domain.MessageInputTypeText,
		Text:      "Как оформить развод?",
		CreatedAt: now,
	}
}

func TestThreadRepo_CreateThread_InsertsThreadAndFirstMessage(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-1111-1111-1111-111111111111", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("22222222-2222-2222-2222-222222222222", sess.ID, now),
		newTestUserMessage("33333333-3333-3333-3333-333333333333", now))

	require.NoError(t, err)
	require.Equal(t, domain.ThreadStatusAwaitingPayment, created.Status)
	require.Equal(t, 1, created.MessageCount)
	require.Equal(t, domain.MessageSenderUser, msg.Sender)

	got, err := threadRepo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.Equal(t, created.Title, got.Title)

	msgs, err := threadRepo.GetMessages(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	require.Equal(t, "Как оформить развод?", msgs[0].Text)
}

func TestThreadRepo_GetByID_NotFound(t *testing.T) {
	pool := setupTestDB(t)
	threadRepo := repo.NewThreadRepo(pool)

	_, err := threadRepo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	require.ErrorIs(t, err, thread.ErrThreadNotFound)
}

func TestThreadRepo_AppendMessage_IncrementsCounters(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("44444444-4444-4444-4444-444444444444", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, _, err := threadRepo.CreateThread(context.Background(),
		newTestThread("55555555-5555-5555-5555-555555555555", sess.ID, now),
		newTestUserMessage("66666666-6666-6666-6666-666666666666", now))
	require.NoError(t, err)

	later := now.Add(time.Minute)
	updated, err := threadRepo.AppendMessage(context.Background(), domain.Message{
		ID:        "77777777-7777-7777-7777-777777777777",
		ThreadID:  created.ID,
		Sender:    domain.MessageSenderAssistant,
		InputType: domain.MessageInputTypeText,
		Text:      "Ответ ассистента",
		Sources:   []domain.Source{{Ref: "ст. 15 ЗоБС", Quote: "..."}},
		CreatedAt: later,
	})

	require.NoError(t, err)
	require.Equal(t, 2, updated.MessageCount)
	require.WithinDuration(t, later, *updated.LastMessageAt, time.Second)

	msgs, err := threadRepo.GetMessages(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, msgs, 2)
	require.Equal(t, []domain.Source{{Ref: "ст. 15 ЗоБС", Quote: "..."}}, msgs[1].Sources)
}

func TestThreadRepo_UpdateStatus_GuardedByFromStatus(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("88888888-8888-8888-8888-888888888888", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, _, err := threadRepo.CreateThread(context.Background(),
		newTestThread("99999999-9999-9999-9999-999999999999", sess.ID, now),
		newTestUserMessage("aaaaaaaa-1111-1111-1111-111111111111", now))
	require.NoError(t, err)

	updated, ok, err := threadRepo.UpdateStatus(context.Background(), created.ID,
		[]domain.ThreadStatus{domain.ThreadStatusAwaitingPayment}, domain.ThreadStatusProcessing, "Ассистент готовит ответ…")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, domain.ThreadStatusProcessing, updated.Status)

	// Гонка/повторный вызов: статус уже не AwaitingPayment -> ok=false, не ошибка.
	_, ok, err = threadRepo.UpdateStatus(context.Background(), created.ID,
		[]domain.ThreadStatus{domain.ThreadStatusAwaitingPayment}, domain.ThreadStatusProcessing, "x")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestThreadRepo_SoftDelete_ExcludesFromListAndGetByID(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("22222222-3333-3333-3333-333333333333", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, _, err := threadRepo.CreateThread(context.Background(),
		newTestThread("33333333-4444-4444-4444-444444444444", sess.ID, now),
		newTestUserMessage("44444444-5555-5555-5555-555555555555", now))
	require.NoError(t, err)

	ok, err := threadRepo.SoftDelete(context.Background(), created.ID, now)
	require.NoError(t, err)
	require.True(t, ok)

	// GetByID по-прежнему находит строку (soft delete, не физическое
	// удаление) — фильтрация "не показывать удалённые" — забота
	// thread.Service (см. Thread.IsDeleted), не репозитория.
	got, err := threadRepo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)
	require.True(t, got.IsDeleted())

	items, total, err := threadRepo.ListThreads(context.Background(), sess.ID, thread.ListFilter{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Equal(t, 0, total)
	require.Empty(t, items)

	// Повторное удаление — идемпотентно, ok=false.
	ok, err = threadRepo.SoftDelete(context.Background(), created.ID, now)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestThreadRepo_ListThreads_FiltersByStatusSearchAndPaginates(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("55555555-6666-6666-6666-666666666666", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	base := time.Now().UTC().Truncate(time.Microsecond)

	makeThread := func(id, msgID, title string, status domain.ThreadStatus, createdAt time.Time) {
		th := newTestThread(id, sess.ID, createdAt)
		th.Title = title
		th.Status = status
		_, _, err := threadRepo.CreateThread(context.Background(), th, newTestUserMessage(msgID, createdAt))
		require.NoError(t, err)
	}
	makeThread("66666666-0001-0001-0001-000000000001", "66666666-0001-0001-0001-100000000001", "Развод и алименты", domain.ThreadStatusDone, base)
	makeThread("66666666-0002-0002-0002-000000000002", "66666666-0002-0002-0002-100000000002", "Трудовой договор", domain.ThreadStatusAwaitingPayment, base.Add(time.Second))
	makeThread("66666666-0003-0003-0003-000000000003", "66666666-0003-0003-0003-100000000003", "Развод без детей", domain.ThreadStatusDone, base.Add(2*time.Second))

	items, total, err := threadRepo.ListThreads(context.Background(), sess.ID, thread.ListFilter{
		Search: "развод", Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Len(t, items, 2)
	require.Equal(t, "Развод без детей", items[0].Title) // created_at DESC

	doneStatus := domain.ThreadStatusDone
	items, total, err = threadRepo.ListThreads(context.Background(), sess.ID, thread.ListFilter{
		Status: &doneStatus, Page: 1, PageSize: 1,
	})
	require.NoError(t, err)
	require.Equal(t, 2, total, "total считает все совпадения, не только текущую страницу")
	require.Len(t, items, 1)
}

func TestThreadRepo_SetMessageFeedback_OnlyOwnSessionMessage(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("77777777-8888-8888-8888-888888888888", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))
	otherSess := newTestSession("99999999-8888-8888-8888-888888888888", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), otherSess))

	threadRepo := repo.NewThreadRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)
	created, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("aaaaaaaa-9999-9999-9999-999999999999", sess.ID, now),
		newTestUserMessage("bbbbbbbb-9999-9999-9999-999999999999", now))
	require.NoError(t, err)
	_ = created

	_, err = threadRepo.SetMessageFeedback(context.Background(), msg.ID, otherSess.ID, domain.MessageFeedbackLike)
	require.ErrorIs(t, err, thread.ErrMessageNotFound)

	updated, err := threadRepo.SetMessageFeedback(context.Background(), msg.ID, sess.ID, domain.MessageFeedbackDislike)
	require.NoError(t, err)
	require.NotNil(t, updated.Feedback)
	require.Equal(t, domain.MessageFeedbackDislike, *updated.Feedback)
}

func TestThreadRepo_GetMessagesAndConversation_AttachFilesToTheirMessage(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-6666-6666-6666-666666666666", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	fileRepo := repo.NewFileRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	created, msg, err := threadRepo.CreateThread(context.Background(),
		newTestThread("22222222-6666-6666-6666-666666666666", sess.ID, now),
		newTestUserMessage("33333333-6666-6666-6666-666666666666", now))
	require.NoError(t, err)

	f, err := fileRepo.Create(context.Background(), newTestFileAttachment("44444444-6666-6666-6666-666666666666", sess.ID, now))
	require.NoError(t, err)
	extracted := "ДОГОВОР АРЕНДЫ"
	require.NoError(t, fileRepo.SetProcessingResult(context.Background(), f.ID, domain.FileProcessingStatusProcessed, &extracted))
	require.NoError(t, fileRepo.AttachToMessage(context.Background(), msg.ID, []string{f.ID}))

	conversation, err := threadRepo.GetConversation(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, conversation, 1)
	require.Len(t, conversation[0].Attachments, 1)
	require.Equal(t, "contract.pdf", conversation[0].Attachments[0].OriginalName)
	require.Equal(t, extracted, *conversation[0].Attachments[0].ExtractedText)

	// REST-история — метаданные вложения без извлечённого текста.
	msgs, err := threadRepo.GetMessages(context.Background(), created.ID)
	require.NoError(t, err)
	require.Len(t, msgs[0].Attachments, 1)
	require.Equal(t, f.ID, msgs[0].Attachments[0].ID)
	require.Nil(t, msgs[0].Attachments[0].ExtractedText)
}
