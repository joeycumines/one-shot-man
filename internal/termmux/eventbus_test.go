package termmux

import (
	"sync"
	"testing"
	"time"
)

// promptSubscriber is a test subscriber implementing the bus contract: it
// receives, acknowledges immediately via Wait, and only then hands the event
// to a downstream channel. Keeping the acknowledgement first is what makes a
// subscriber safe on the shared unbuffered bus.
type promptSubscriber struct {
	ch      <-chan Event
	bus     *EventBus
	events  chan Event
	done    chan struct{}
	stopped chan struct{}
	once    sync.Once
}

func newPromptSubscriber(t *testing.T, bus *EventBus) *promptSubscriber {
	t.Helper()
	bus.Subscribe()
	ps := &promptSubscriber{
		ch:      bus.Channel(),
		bus:     bus,
		events:  make(chan Event, 4096),
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
	go func() {
		defer close(ps.stopped)
		for {
			select {
			case <-ps.done:
				return
			case evt, ok := <-ps.ch:
				if !ok {
					return
				}
				ps.bus.Wait()
				select {
				case ps.events <- evt:
				case <-ps.done:
					return
				}
			}
		}
	}()
	t.Cleanup(ps.stop)
	return ps
}

// stop halts the subscriber and then releases the subscription, in that order.
// The order matters: a goroutine still parked in a receive after Unsubscribe
// would consume a send intended for another subscriber.
func (ps *promptSubscriber) stop() {
	ps.once.Do(func() {
		close(ps.done)
		<-ps.stopped
		ps.bus.Unsubscribe()
	})
}

// recv waits for the next delivered event.
func (ps *promptSubscriber) recv(t *testing.T) Event {
	t.Helper()
	select {
	case evt := <-ps.events:
		return evt
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
		return Event{}
	}
}

// TestEventBus_PublishDeliversToSingleSubscriber covers the base contract: a
// published event reaches a prompt subscriber with kind and session intact.
func TestEventBus_PublishDeliversToSingleSubscriber(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()
	sub := newPromptSubscriber(t, bus)

	bus.Publish(Event{Kind: EventBell, SessionID: 42})
	got := sub.recv(t)
	if got.Kind != EventBell {
		t.Errorf("Kind = %v, want %v", got.Kind, EventBell)
	}
	if got.SessionID != 42 {
		t.Errorf("SessionID = %d, want 42", got.SessionID)
	}
}

// TestEventBus_FanOutReachesEverySubscriber verifies that one publish is
// received by every registered subscriber — the core reason the bus exists.
func TestEventBus_FanOutReachesEverySubscriber(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	const subscribers = 5
	subs := make([]*promptSubscriber, subscribers)
	for i := range subs {
		subs[i] = newPromptSubscriber(t, bus)
	}

	bus.Publish(Event{Kind: EventSessionActivated, SessionID: 7})

	for i, sub := range subs {
		got := sub.recv(t)
		if got.Kind != EventSessionActivated || got.SessionID != 7 {
			t.Errorf("subscriber %d got %+v, want activated for session 7", i, got)
		}
	}
}

// TestEventBus_NoDropsUnderSlowSubscriber is the behavioural inversion of the
// old lossy bus. A subscriber that sleeps while the publisher fires a burst
// must still receive EVERY event, in order. The old EventBus dropped silently
// here; this asserts the lossless contract that replaces it.
func TestEventBus_NoDropsUnderSlowSubscriber(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	// Subscriber whose downstream channel is small and drained slowly.
	bus.Subscribe()
	defer bus.Unsubscribe()
	ch := bus.Channel()

	const total = 256
	received := make([]Event, 0, total)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for len(received) < total {
			evt, ok := <-ch
			if !ok {
				return
			}
			bus.Wait() // contract: acknowledge before doing anything else
			// Drain deliberately slowly so the publisher is forced to wait.
			received = append(received, evt)
			time.Sleep(time.Microsecond)
		}
	}()

	for i := range total {
		bus.Publish(Event{Kind: EventBell, SessionID: SessionID(i + 1)})
	}

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("subscriber did not receive every event — publisher stalled or event lost")
	}

	if len(received) != total {
		t.Fatalf("received %d events, want %d (lossless contract violated)", len(received), total)
	}
	for i, evt := range received {
		if evt.Kind != EventBell || evt.SessionID != SessionID(i+1) {
			t.Fatalf("event %d out of order or corrupted: %+v", i, evt)
		}
	}
}

// TestEventBus_OrderingIsTotalAcrossSubscribers asserts every subscriber
// observes the same sequence, which is what makes bus order usable as a
// correctness guarantee for consumers and the JavaScript bridge.
func TestEventBus_OrderingIsTotalAcrossSubscribers(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	subs := []*promptSubscriber{
		newPromptSubscriber(t, bus),
		newPromptSubscriber(t, bus),
		newPromptSubscriber(t, bus),
	}

	const total = 64
	for i := range total {
		bus.Publish(Event{Kind: EventSessionActivated, SessionID: SessionID(i + 1)})
	}

	for i, sub := range subs {
		for want := range total {
			got := sub.recv(t)
			if got.SessionID != SessionID(want+1) {
				t.Fatalf("subscriber %d position %d: SessionID = %d, want %d",
					i, want, got.SessionID, want+1)
			}
		}
	}
}

// TestEventBus_ConcurrentPublishersPreserveOrderPerOrigin verifies total order
// is consistent even under concurrent publishers: each publisher's own
// sequence is observed in order by every subscriber, because broadcasts are
// serialized end to end.
func TestEventBus_ConcurrentPublishersPreserveOrderPerOrigin(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	const origins = 4
	const perOrigin = 32

	subs := []*promptSubscriber{
		newPromptSubscriber(t, bus),
		newPromptSubscriber(t, bus),
	}

	var wg sync.WaitGroup
	for origin := range origins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for seq := range perOrigin {
				bus.Publish(Event{
					Kind:      EventSessionActivated,
					SessionID: SessionID(origin*1000 + seq + 1),
				})
			}
		}()
	}
	wg.Wait()

	// Give the publisher a moment for the final acknowledgements to land, then
	// verify per-origin ordering for each subscriber.
	for i, sub := range subs {
		next := make([]int, origins)
		for range origins * perOrigin {
			got := sub.recv(t)
			origin := (int(got.SessionID) - 1) / 1000
			seq := (int(got.SessionID) - 1) % 1000
			if seq != next[origin] {
				t.Fatalf("subscriber %d: origin %d delivered seq %d, want %d",
					i, origin, seq, next[origin])
			}
			next[origin]++
		}
	}
}

// TestEventBus_SubscriberChurnDoesNotStall verifies the contract under
// subscribe/unsubscribe churn: publishers must keep completing while
// subscribers come and go mid-burst.
func TestEventBus_SubscriberChurnDoesNotStall(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	const publishes = 500
	published := make(chan struct{})
	go func() {
		defer close(published)
		for i := range publishes {
			bus.Publish(Event{Kind: EventBell, SessionID: SessionID(i + 1)})
		}
	}()

	// Continuously add and remove short-lived subscribers while publishing.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				bus.Subscribe()
				select {
				case <-bus.Channel():
					bus.Wait()
				default:
				}
				bus.Unsubscribe()
			}
		}()
	}

	select {
	case <-published:
	case <-time.After(30 * time.Second):
		close(stop)
		wg.Wait()
		t.Fatal("publisher stalled under subscriber churn — contract violated")
	}
	close(stop)
	wg.Wait()
}

// TestEventBus_UnsubscribeReleasesBlockedPublisher verifies that unsubscribing
// is a genuine escape hatch: a publisher blocked on a subscriber that has
// stopped consuming must be released when that subscriber unregisters.
func TestEventBus_UnsubscribeReleasesBlockedPublisher(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()

	// Register a subscriber that never receives.
	bus.Subscribe()

	// This publish blocks: one synchronous send, no receiver.
	published := make(chan struct{})
	go func() {
		defer close(published)
		bus.Publish(Event{Kind: EventBell})
	}()

	select {
	case <-published:
		t.Fatal("publish completed with no receiver — expected it to block")
	case <-time.After(50 * time.Millisecond):
	}

	// Unsubscribing drains the in-flight send on the departing subscriber's
	// behalf, releasing the blocked publisher.
	bus.Unsubscribe()

	select {
	case <-published:
	case <-time.After(5 * time.Second):
		t.Fatal("publisher still blocked after the subscriber unsubscribed")
	}
}

// TestEventBus_CloseClosesSubscriberChannel verifies subscribers observe
// closure as a receive with ok == false.
func TestEventBus_CloseClosesSubscriberChannel(t *testing.T) {
	bus := NewEventBus()
	bus.Subscribe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, ok := <-bus.Channel()
		if ok {
			t.Error("expected closed channel, got a value")
		}
	}()

	bus.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("subscriber did not observe channel close")
	}
}

// TestEventBus_CloseIdempotent verifies Close may be called more than once
// without panicking — the manager closes on every shutdown path.
func TestEventBus_CloseIdempotent(t *testing.T) {
	bus := NewEventBus()
	bus.Close()
	bus.Close()
}

// TestEventBus_PublishWithNoSubscribersIsNoOp verifies a manager with no
// JavaScript consumer and no Go subscriber publishes safely.
func TestEventBus_PublishWithNoSubscribersIsNoOp(t *testing.T) {
	bus := NewEventBus()
	bus.Publish(Event{Kind: EventBell})
	bus.Close()
}

// TestEventBus_EmitSetsTimeAndKind verifies the worker-facing emit helpers.
func TestEventBus_EmitSetsTimeAndKind(t *testing.T) {
	bus := NewEventBus()
	defer bus.Close()
	sub := newPromptSubscriber(t, bus)

	before := time.Now()
	bus.emitData(EventResize, 0, [2]int{24, 80})
	got := sub.recv(t)

	if got.Kind != EventResize {
		t.Errorf("Kind = %v, want EventResize", got.Kind)
	}
	if got.Time.Before(before) {
		t.Errorf("Time = %v, want >= %v", got.Time, before)
	}
	dims, ok := got.DataAsDims()
	if !ok || dims != [2]int{24, 80} {
		t.Errorf("DataAsDims() = %v, %v; want [24 80], true", dims, ok)
	}
}

// TestEventKind_AllStrings verifies every kind has a stable JS-facing name.
func TestEventKind_AllStrings(t *testing.T) {
	cases := map[EventKind]string{
		EventSessionRegistered: "session-registered",
		EventSessionActivated:  "session-activated",
		EventSessionExited:     "session-exited",
		EventSessionClosed:     "session-closed",
		EventResize:            "resize",
		EventBell:              "bell",
		EventTitle:             "title",
		EventWorkingDirectory:  "working-directory",
		EventClipboard:         "clipboard",
		EventActivity:          "activity",
		EventSilence:           "silence",
		EventWindowUpdated:     "window-updated",
	}
	for kind, want := range cases {
		if got := kind.String(); got != want {
			t.Errorf("EventKind(%d).String() = %q, want %q", kind, got, want)
		}
	}
	if got := EventKind(999).String(); got != "unknown" {
		t.Errorf("unknown kind String() = %q, want %q", got, "unknown")
	}
}
