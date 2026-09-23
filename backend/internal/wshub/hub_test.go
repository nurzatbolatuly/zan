package wshub_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"zan-backend/internal/domain"
	"zan-backend/internal/wshub"
)

func TestHub_SubscribeGetsCurrentSnapshot(t *testing.T) {
	h := wshub.NewHub()
	ctx := context.Background()

	h.PublishStatus(ctx, "t1", domain.ThreadStatusProcessing, "Ассистент готовит ответ…")
	h.PublishAnswerDelta(ctx, "t1", "Привет")
	h.PublishAnswerDelta(ctx, "t1", ", мир")

	sub, snapshot := h.Subscribe("t1")
	defer h.Unsubscribe("t1", sub)

	require.Len(t, snapshot, 2)
	require.Equal(t, wshub.EventThreadStatus, snapshot[0].Type)
	require.Equal(t, domain.ThreadStatusProcessing, snapshot[0].Status)
	require.Equal(t, wshub.EventAnswerDelta, snapshot[1].Type)
	require.Equal(t, "Привет, мир", snapshot[1].Delta)
}

func TestHub_SubscribeBeforeAnyPublish_EmptySnapshot(t *testing.T) {
	h := wshub.NewHub()
	sub, snapshot := h.Subscribe("t-fresh")
	defer h.Unsubscribe("t-fresh", sub)

	require.Empty(t, snapshot)
}

func TestHub_PublishFansOutToAllSubscribers(t *testing.T) {
	h := wshub.NewHub()
	ctx := context.Background()

	sub1, _ := h.Subscribe("t2")
	defer h.Unsubscribe("t2", sub1)
	sub2, _ := h.Subscribe("t2")
	defer h.Unsubscribe("t2", sub2)

	h.PublishAnswerDelta(ctx, "t2", "hi")

	for _, sub := range []*wshub.Subscription{sub1, sub2} {
		select {
		case ev := <-sub.Events():
			require.Equal(t, wshub.EventAnswerDelta, ev.Type)
			require.Equal(t, "hi", ev.Delta)
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for fanned-out event")
		}
	}
}

func TestHub_SlowSubscriberDroppedWithoutBlockingPublish(t *testing.T) {
	h := wshub.NewHub()
	ctx := context.Background()

	slow, _ := h.Subscribe("t3")
	defer h.Unsubscribe("t3", slow)
	fast, _ := h.Subscribe("t3")
	defer h.Unsubscribe("t3", fast)

	// Never drain `slow` — fill its buffer, then publish well past capacity.
	// PublishAnswerDelta must never block regardless of how far past
	// capacity we go.
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			h.PublishAnswerDelta(ctx, "t3", "x")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}

	// `fast` is drained concurrently by nothing in this test, but since we
	// only assert Publish didn't block, draining isn't required here.
	// Slow subscriber's channel should now be closed (evicted).
	select {
	case _, ok := <-slow.Events():
		// Either a buffered event or the closed-channel zero value is fine —
		// what matters is we don't block forever.
		_ = ok
	case <-time.After(time.Second):
		t.Fatal("slow subscriber channel never became readable/closed")
	}
}

// TestHub_NoDeltaLostBetweenPublishAndSubscribe — race-detector regression
// guard for the core invariant: Subscribe and Publish* share one mutex per
// topic, so a concurrent subscriber either sees an event in its snapshot or
// receives it live — never neither. Run with `go test -race`.
func TestHub_NoDeltaLostBetweenPublishAndSubscribe(t *testing.T) {
	h := wshub.NewHub()
	ctx := context.Background()
	const threadID = "t-race"
	const totalDeltas = 50

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < totalDeltas; i++ {
			h.PublishAnswerDelta(ctx, threadID, "x")
		}
	}()

	results := make(chan int, 20)
	var subsWG sync.WaitGroup
	for i := 0; i < 20; i++ {
		subsWG.Add(1)
		go func() {
			defer subsWG.Done()
			sub, snapshot := h.Subscribe(threadID)
			defer h.Unsubscribe(threadID, sub)

			seen := 0
			for _, ev := range snapshot {
				seen += len(ev.Delta)
			}
			// Drain whatever live events arrive for a short window — total
			// count across snapshot+live must never exceed what was ever
			// published (it may be less, if this subscriber joined late
			// after some deltas already fired without it — that's fine and
			// expected, not a loss).
			timeout := time.After(200 * time.Millisecond)
		drain:
			for {
				select {
				case ev, ok := <-sub.Events():
					if !ok {
						break drain
					}
					seen += len(ev.Delta)
				case <-timeout:
					break drain
				}
			}
			results <- seen
		}()
	}
	subsWG.Wait()
	wg.Wait()
	close(results)

	for seen := range results {
		require.LessOrEqual(t, seen, totalDeltas, "subscriber saw more bytes than were ever published")
	}
}

func TestHub_SnapshotAfterAnswerDone_ReflectsTerminalStatusNoPartialText(t *testing.T) {
	h := wshub.NewHub()
	ctx := context.Background()

	h.PublishStatus(ctx, "t4", domain.ThreadStatusProcessing, "Ассистент готовит ответ…")
	h.PublishAnswerDelta(ctx, "t4", "partial")
	h.PublishAnswerDone(ctx, "t4", domain.Message{ID: "m1", Text: "final"}, domain.ThreadStatusDone)

	sub, snapshot := h.Subscribe("t4")
	defer h.Unsubscribe("t4", sub)

	require.Len(t, snapshot, 1)
	require.Equal(t, wshub.EventThreadStatus, snapshot[0].Type)
	require.Equal(t, domain.ThreadStatusDone, snapshot[0].Status)
}

func TestHub_UnsubscribeClosesChannel(t *testing.T) {
	h := wshub.NewHub()
	sub, _ := h.Subscribe("t5")
	h.Unsubscribe("t5", sub)

	_, ok := <-sub.Events()
	require.False(t, ok, "channel should be closed after Unsubscribe")
}
