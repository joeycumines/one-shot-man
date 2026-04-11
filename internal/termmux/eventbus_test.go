package termmux

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewEventBus
// ---------------------------------------------------------------------------

func TestNewEventBus(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	if bus == nil {
		t.Fatal("NewEventBus returned nil")
	}
	if bus.subscribers == nil {
		t.Error("subscribers map is nil")
	}
	if bus.nextID != 1 {
		t.Errorf("nextID = %d, want 1", bus.nextID)
	}
	if bus.closed {
		t.Error("closed should be false initially")
	}
}

// ---------------------------------------------------------------------------
// Subscribe
// ---------------------------------------------------------------------------

func TestEventBus_Subscribe_DefaultBuffer(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	id, ch := bus.Subscribe(0)
	if id != 1 {
		t.Errorf("id = %d, want 1", id)
	}
	if ch == nil {
		t.Fatal("channel is nil")
	}
	if cap(ch) != 64 {
		t.Errorf("cap = %d, want 64 (default)", cap(ch))
	}
}

func TestEventBus_Subscribe_CustomBuffer(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	_, ch := bus.Subscribe(128)
	if cap(ch) != 128 {
		t.Errorf("cap = %d, want 128", cap(ch))
	}
}

func TestEventBus_Subscribe_MonotonicIDs(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	id1, _ := bus.Subscribe(1)
	id2, _ := bus.Subscribe(1)
	id3, _ := bus.Subscribe(1)

	if id1 != 1 || id2 != 2 || id3 != 3 {
		t.Errorf("IDs = %d, %d, %d — want 1, 2, 3", id1, id2, id3)
	}
}

func TestEventBus_Subscribe_AfterClose(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	bus.Close()

	id, ch := bus.Subscribe(1)
	if id != 0 {
		t.Errorf("id = %d, want 0 (closed bus)", id)
	}
	// Channel should be immediately closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel")
		}
	default:
		t.Error("channel should be closed, not blocking")
	}
}

// ---------------------------------------------------------------------------
// Unsubscribe
// ---------------------------------------------------------------------------

func TestEventBus_Unsubscribe(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	id, ch := bus.Subscribe(1)
	ok := bus.Unsubscribe(id)
	if !ok {
		t.Error("Unsubscribe returned false for existing subscriber")
	}

	// Channel should be closed.
	select {
	case _, open := <-ch:
		if open {
			t.Error("expected closed channel after Unsubscribe")
		}
	default:
		t.Error("channel should be closed, not blocking")
	}
}

func TestEventBus_Unsubscribe_NonExistent(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	if bus.Unsubscribe(999) {
		t.Error("Unsubscribe returned true for non-existent ID")
	}
}

func TestEventBus_Unsubscribe_StopsDelivery(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	id, ch := bus.Subscribe(64)

	// Publish one event before unsubscribe.
	bus.Publish(Event{Kind: EventBell, Time: time.Now()})

	// Drain the event.
	select {
	case evt := <-ch:
		if evt.Kind != EventBell {
			t.Errorf("Kind = %s, want bell", evt.Kind)
		}
	default:
		t.Fatal("expected event before Unsubscribe")
	}

	bus.Unsubscribe(id)

	// Publish after unsubscribe — should not reach the closed channel.
	bus.Publish(Event{Kind: EventResize, Time: time.Now()})

	// Channel is closed, so reading returns zero value immediately.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("received event after Unsubscribe")
		}
	default:
		// Channel closed — correct.
	}
}

// ---------------------------------------------------------------------------
// Publish
// ---------------------------------------------------------------------------

func TestEventBus_Publish_SingleSubscriber(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	_, ch := bus.Subscribe(64)

	evt := Event{
		Kind:      EventSessionRegistered,
		SessionID: 42,
		Time:      time.Now(),
	}
	bus.Publish(evt)

	select {
	case got := <-ch:
		if got.Kind != EventSessionRegistered {
			t.Errorf("Kind = %s, want session-registered", got.Kind)
		}
		if got.SessionID != 42 {
			t.Errorf("SessionID = %d, want 42", got.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventBus_Publish_FanOut(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	const numSubs = 5
	channels := make([]<-chan Event, numSubs)
	for i := range numSubs {
		_, channels[i] = bus.Subscribe(64)
	}

	evt := Event{
		Kind:      EventSessionExited,
		SessionID: 7,
		Time:      time.Now(),
	}
	bus.Publish(evt)

	for i, ch := range channels {
		select {
		case got := <-ch:
			if got.Kind != EventSessionExited {
				t.Errorf("subscriber %d: Kind = %s, want session-exited", i, got.Kind)
			}
			if got.SessionID != 7 {
				t.Errorf("subscriber %d: SessionID = %d, want 7", i, got.SessionID)
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timed out", i)
		}
	}
}

func TestEventBus_Publish_NonBlocking_SlowSubscriber(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	// Subscribe with buffer size 1.
	_, ch := bus.Subscribe(1)

	// Fill the buffer.
	bus.Publish(Event{Kind: EventBell, SessionID: 1, Time: time.Now()})

	// This should NOT block — the event is silently dropped.
	done := make(chan struct{})
	go func() {
		bus.Publish(Event{Kind: EventResize, SessionID: 2, Time: time.Now()})
		close(done)
	}()

	select {
	case <-done:
		// Good — Publish returned without blocking.
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on full subscriber channel")
	}

	// Drain the one buffered event.
	got := <-ch
	if got.Kind != EventBell {
		t.Errorf("Kind = %s, want bell", got.Kind)
	}
}

func TestEventBus_Publish_AfterClose(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	bus.Close()

	// Publish after Close should be a no-op (no panic).
	bus.Publish(Event{Kind: EventBell, Time: time.Now()})
}

func TestEventBus_Publish_NoSubscribers(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	// No subscribers — Publish should be a no-op (no panic).
	bus.Publish(Event{Kind: EventResize, Time: time.Now()})
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

func TestEventBus_Close_ClosesAllChannels(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()

	const numSubs = 3
	channels := make([]<-chan Event, numSubs)
	for i := range numSubs {
		_, channels[i] = bus.Subscribe(64)
	}

	bus.Close()

	for i, ch := range channels {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("subscriber %d: channel not closed after Close", i)
			}
		default:
			t.Errorf("subscriber %d: channel should be closed, not blocking", i)
		}
	}
}

func TestEventBus_Close_Idempotent(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	bus.Close()
	bus.Close() // Should not panic.
}

// ---------------------------------------------------------------------------
// emit (internal method)
// ---------------------------------------------------------------------------

func TestEventBus_Emit(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	_, ch := bus.Subscribe(64)

	bus.emit(EventSessionRegistered, 99)

	select {
	case got := <-ch:
		if got.Kind != EventSessionRegistered {
			t.Errorf("Kind = %s, want session-registered", got.Kind)
		}
		if got.SessionID != 99 {
			t.Errorf("SessionID = %d, want 99", got.SessionID)
		}
		if got.Time.IsZero() {
			t.Error("Time is zero")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for emitted event")
	}
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestEventBus_ConcurrentSubscribeUnsubscribePublish(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()

	const goroutines = 10
	const iterations = 500

	var wg sync.WaitGroup

	// Concurrent publishers.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for i := range iterations {
				bus.Publish(Event{
					Kind:      EventKind(i % 7),
					SessionID: SessionID(i),
					Time:      time.Now(),
				})
			}
		}()
	}

	// Concurrent subscribers / unsubscribers.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				id, ch := bus.Subscribe(1)
				// Drain to prevent blocking.
				select {
				case <-ch:
				default:
				}
				bus.Unsubscribe(id)
			}
		}()
	}

	wg.Wait()
	bus.Close()
}

func TestEventBus_ConcurrentPublishClose(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()

	_, ch := bus.Subscribe(64)

	var wg sync.WaitGroup

	// Publisher goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 1000 {
			bus.Publish(Event{Kind: EventBell, SessionID: SessionID(i), Time: time.Now()})
		}
	}()

	// Closer goroutine.
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(time.Microsecond) // Let some publishes happen.
		bus.Close()
	}()

	wg.Wait()

	// Drain remaining events.
	for range ch {
	}
}

// TestEventBus_PublishDuringUnsubscribe is a regression test for the
// send-on-closed-channel panic. With the snapshot-then-release approach,
// Publish could panic if Unsubscribe closed a channel between snapshot
// and delivery. The fix holds the mutex during non-blocking sends.
func TestEventBus_PublishDuringUnsubscribe(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()

	const goroutines = 20
	const iterations = 1000

	var wg sync.WaitGroup

	// Mix of concurrent Publish and Unsubscribe — must not panic.
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			for range iterations {
				id, ch := bus.Subscribe(1)
				// Immediately publish and unsubscribe in tight sequence.
				bus.Publish(Event{Kind: EventBell, Time: time.Now()})
				bus.Unsubscribe(id)
				// Drain closed channel.
				for range ch {
				}
			}
		}()
	}

	wg.Wait()
	bus.Close()
}

// ---------------------------------------------------------------------------
// EventKind.String
// ---------------------------------------------------------------------------

func TestEventKind_AllStrings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind EventKind
		want string
	}{
		{EventSessionRegistered, "session-registered"},
		{EventSessionActivated, "session-activated"},
		{EventSessionOutput, "session-output"},
		{EventSessionExited, "session-exited"},
		{EventSessionClosed, "session-closed"},
		{EventResize, "resize"},
		{EventBell, "bell"},
		{EventKind(99), "unknown"},
	}
	for _, tc := range cases {
		if got := tc.kind.String(); got != tc.want {
			t.Errorf("EventKind(%d).String() = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// DroppedCount
// ---------------------------------------------------------------------------

func TestEventBus_DroppedCount_InitiallyZero(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	if got := bus.DroppedCount(); got != 0 {
		t.Errorf("DroppedCount() = %d, want 0", got)
	}
}

func TestEventBus_DroppedCount_CountsDroppedEvents(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	// Subscribe with buffer size 1 — second publish fills buffer,
	// third publish drops.
	_, ch := bus.Subscribe(1)

	bus.Publish(Event{Kind: EventBell, SessionID: 1, Time: time.Now()})
	bus.Publish(Event{Kind: EventResize, SessionID: 2, Time: time.Now()})
	bus.Publish(Event{Kind: EventSessionOutput, SessionID: 3, Time: time.Now()})

	// At least 2 events should have been dropped (buffer holds 1).
	if got := bus.DroppedCount(); got < 2 {
		t.Errorf("DroppedCount() = %d, want >= 2", got)
	}

	// First event should have been delivered.
	select {
	case evt := <-ch:
		if evt.Kind != EventBell {
			t.Errorf("first event Kind = %s, want bell", evt.Kind)
		}
		if evt.SessionID != 1 {
			t.Errorf("first event SessionID = %d, want 1", evt.SessionID)
		}
	default:
		t.Fatal("expected at least one event in channel")
	}
}

func TestEventBus_DroppedCount_MultipleSubscribers(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	// Two subscribers, each with buffer 1.
	_, ch1 := bus.Subscribe(1)
	_, ch2 := bus.Subscribe(1)

	// Fill both buffers.
	bus.Publish(Event{Kind: EventBell, Time: time.Now()})
	// This drops for both subscribers.
	bus.Publish(Event{Kind: EventResize, Time: time.Now()})

	// 2 drops: one per subscriber.
	if got := bus.DroppedCount(); got != 2 {
		t.Errorf("DroppedCount() = %d, want 2", got)
	}

	// Both received the first event.
	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.Kind != EventBell {
				t.Errorf("subscriber %d: Kind = %s, want bell", i, evt.Kind)
			}
		default:
			t.Fatalf("subscriber %d: expected event in channel", i)
		}
	}
}

func TestEventBus_DroppedCount_NoDropsWithLargeBuffer(t *testing.T) {
	t.Parallel()

	bus := NewEventBus()
	defer bus.Close()

	_, _ = bus.Subscribe(64)

	for range 10 {
		bus.Publish(Event{Kind: EventBell, Time: time.Now()})
	}

	if got := bus.DroppedCount(); got != 0 {
		t.Errorf("DroppedCount() = %d, want 0 (large buffer, no drops)", got)
	}
}
