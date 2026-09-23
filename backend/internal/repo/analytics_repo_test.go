package repo_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/repo"
)

// TestAnalyticsRepo_GetOverview_AggregatesAcrossThreadsAndMessages — Stage 7
// (BACKEND_PLAN.md): total_threads/status_breakdown/avg_processing_time_sec/
// satisfaction_rate на реальных данных — done+error+awaiting_payment треды, оценённые
// и неоценённые ответы ассистента, soft-deleted тред исключён из всего.
func TestAnalyticsRepo_GetOverview_AggregatesAcrossThreadsAndMessages(t *testing.T) {
	pool := setupTestDB(t)
	sessionRepo := repo.NewSessionRepo(pool)
	sess := newTestSession("11111111-9999-9999-9999-999999999999", time.Now().UTC())
	require.NoError(t, sessionRepo.Create(context.Background(), sess))

	threadRepo := repo.NewThreadRepo(pool)
	analyticsRepo := repo.NewAnalyticsRepo(pool)
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Тред 1: done, ответ ассистента оценён "like", processing_time_ms=2000.
	t1, _, err := threadRepo.CreateThread(context.Background(),
		newTestThread("22222222-9999-9999-9999-999999999999", sess.ID, now),
		newTestUserMessage("33333333-9999-9999-9999-999999999999", now))
	require.NoError(t, err)
	_, ok, err := threadRepo.UpdateStatus(context.Background(), t1.ID, []domain.ThreadStatus{domain.ThreadStatusAwaitingPayment}, domain.ThreadStatusProcessing, "processing")
	require.NoError(t, err)
	require.True(t, ok)
	processingMs1 := 2000
	_, err = threadRepo.AppendMessage(context.Background(), domain.Message{
		ID: "44444444-9999-9999-9999-999999999999", ThreadID: t1.ID, Sender: domain.MessageSenderAssistant,
		InputType: domain.MessageInputTypeText, Text: "Ответ", ProcessingTimeMs: &processingMs1, CreatedAt: now,
	})
	require.NoError(t, err)
	_, ok, err = threadRepo.UpdateStatus(context.Background(), t1.ID, []domain.ThreadStatus{domain.ThreadStatusProcessing}, domain.ThreadStatusDone, "Ответ")
	require.NoError(t, err)
	require.True(t, ok)
	msgs1, err := threadRepo.GetMessages(context.Background(), t1.ID)
	require.NoError(t, err)
	assistantMsgID1 := msgs1[len(msgs1)-1].ID
	_, err = threadRepo.SetMessageFeedback(context.Background(), assistantMsgID1, sess.ID, domain.MessageFeedbackLike)
	require.NoError(t, err)

	// Тред 2: error, ответ ассистента оценён "dislike", processing_time_ms=4000.
	t2, _, err := threadRepo.CreateThread(context.Background(),
		newTestThread("55555555-9999-9999-9999-999999999999", sess.ID, now),
		newTestUserMessage("66666666-9999-9999-9999-999999999999", now))
	require.NoError(t, err)
	_, ok, err = threadRepo.UpdateStatus(context.Background(), t2.ID, []domain.ThreadStatus{domain.ThreadStatusAwaitingPayment}, domain.ThreadStatusProcessing, "processing")
	require.NoError(t, err)
	require.True(t, ok)
	processingMs2 := 4000
	_, err = threadRepo.AppendMessage(context.Background(), domain.Message{
		ID: "77777777-9999-9999-9999-999999999999", ThreadID: t2.ID, Sender: domain.MessageSenderAssistant,
		InputType: domain.MessageInputTypeText, Text: "Не удалось обработать запрос", ProcessingTimeMs: &processingMs2, CreatedAt: now,
	})
	require.NoError(t, err)
	_, ok, err = threadRepo.UpdateStatus(context.Background(), t2.ID, []domain.ThreadStatus{domain.ThreadStatusProcessing}, domain.ThreadStatusError, "Не удалось обработать запрос")
	require.NoError(t, err)
	require.True(t, ok)
	msgs2, err := threadRepo.GetMessages(context.Background(), t2.ID)
	require.NoError(t, err)
	_, err = threadRepo.SetMessageFeedback(context.Background(), msgs2[len(msgs2)-1].ID, sess.ID, domain.MessageFeedbackDislike)
	require.NoError(t, err)

	// Тред 3: остаётся awaiting_payment, без ответа ассистента (не должен влиять на
	// avg_processing_time_sec/satisfaction_rate — только на total/breakdown).
	_, _, err = threadRepo.CreateThread(context.Background(),
		newTestThread("88888888-9999-9999-9999-999999999999", sess.ID, now),
		newTestUserMessage("99999999-9999-9999-9999-999999999999", now))
	require.NoError(t, err)

	// Тред 4: soft-deleted — исключён отовсюду.
	t4, _, err := threadRepo.CreateThread(context.Background(),
		newTestThread("aaaaaaaa-9999-9999-9999-999999999999", sess.ID, now),
		newTestUserMessage("bbbbbbbb-9999-9999-9999-999999999999", now))
	require.NoError(t, err)
	_, err = threadRepo.SoftDelete(context.Background(), t4.ID, now)
	require.NoError(t, err)

	overview, err := analyticsRepo.GetOverview(context.Background())
	require.NoError(t, err)

	require.Equal(t, 3, overview.TotalThreads, "soft-deleted thread excluded")
	require.Equal(t, 1, overview.StatusBreakdown[domain.ThreadStatusDone])
	require.Equal(t, 1, overview.StatusBreakdown[domain.ThreadStatusError])
	require.Equal(t, 1, overview.StatusBreakdown[domain.ThreadStatusAwaitingPayment])
	require.Equal(t, 0, overview.StatusBreakdown[domain.ThreadStatusCanceled])

	require.NotNil(t, overview.AvgProcessingTimeSec)
	require.InDelta(t, 3.0, *overview.AvgProcessingTimeSec, 0.001) // (2000+4000)/2 мс -> 3.0с

	require.NotNil(t, overview.SatisfactionRate)
	require.InDelta(t, 0.5, *overview.SatisfactionRate, 0.001) // 1 like из 2 оценённых
}

func TestAnalyticsRepo_GetOverview_EmptyDatabase_ReturnsNilAveragesAndZeroTotal(t *testing.T) {
	pool := setupTestDB(t)
	analyticsRepo := repo.NewAnalyticsRepo(pool)

	overview, err := analyticsRepo.GetOverview(context.Background())

	require.NoError(t, err)
	require.Equal(t, 0, overview.TotalThreads)
	require.Nil(t, overview.AvgProcessingTimeSec)
	require.Nil(t, overview.SatisfactionRate)
}
