package termmux

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// EventKind identifies the type of event published through the EventBus.
type EventKind int

const (
	// EventSessionRegistered is published when a session is added to the manager.
	EventSessionRegistered EventKind = iota

	// EventSessionActivated is published when the active session changes.
	EventSessionActivated

	// EventSessionOutput is published when new output is processed from a session.
	EventSessionOutput

	// EventSessionExited is published when a session's process exits.
	EventSessionExited

	// EventSessionClosed is published when a session is fully unregistered.
	EventSessionClosed

	// EventResize is published when the terminal dimensions change.
	EventResize

	// EventBell is published when a BEL character (0x07) is processed.
	EventBell
)

// String returns a human-readable name for the event kind.
func (k EventKind) String() string {
	switch k {
	case EventSessionRegistered:
		return "session-registered"
	case EventSessionActivated:
		return "session-activated"
	case EventSessionOutput:
		return "session-output"
	case EventSessionExited:
		return "session-exited"
	case EventSessionClosed:
		return "session-closed"
	case EventResize:
		return "resize"
	case EventBell:
		return "bell"
	default:
		return "unknown"
	}
}

// Event is a typed notification emitted by the SessionManager's worker
// goroutine and delivered to subscribers via the EventBus. Events are
// immutable values — subscribers may read all fields without synchronization.
type Event struct {
	// Kind identifies the event type.
	Kind EventKind

	// SessionID identifies the session that produced this event.
	// Zero for events not tied to a specific session (e.g., EventResize).
	SessionID SessionID

	// Data carries kind-specific payload. The concrete type depends on Kind:
	//   EventSessionOutput  → []byte (raw output chunk)
	//   EventResize         → [2]int{rows, cols}
	// Other kinds carry nil.
	Data any

	// Time records when the event was created.
	Time time.Time
}

// EventBus provides typed, non-blocking fan-out event delivery to multiple
// subscribers. It is the ONLY type in the termmux package that uses a mutex
// (to protect the subscriber list). The mutex is acceptable here because
// Subscribe/Unsubscribe are infrequent operations, and Publish holds the
// lock only during non-blocking sends (select with default), keeping the
// critical section O(subscribers) with each iteration being O(1).
//
// Typical usage:
//
//	bus := NewEventBus()
//	id, ch := bus.Subscribe(64)
//	go func() {
//	    for evt := range ch {
//	        // handle event
//	    }
//	}()
//	bus.Publish(Event{Kind: EventBell})
//	bus.Unsubscribe(id)
//	bus.Close()
type EventBus struct {
	mu          sync.Mutex
	subscribers map[int]chan<- Event
	nextID      int
	closed      bool

	// droppedEvents counts events that could not be delivered because a
	// subscriber's channel was full. Accessed atomically outside the mutex.
	droppedEvents atomic.Int64
}

// NewEventBus creates an EventBus ready for use.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[int]chan<- Event),
		nextID:      1,
	}
}

// Subscribe registers a new subscriber and returns its unique ID and a
// read-only event channel. The channel is buffered to bufSize; if bufSize
// is less than 1, it defaults to 64. Events are delivered via non-blocking
// sends — a full channel means events are dropped for that subscriber
// (the EventBus never blocks the publisher).
//
// The returned channel is closed when Unsubscribe is called with the
// returned ID, or when Close is called on the EventBus.
func (b *EventBus) Subscribe(bufSize int) (int, <-chan Event) {
	if bufSize < 1 {
		bufSize = 64
	}
	ch := make(chan Event, bufSize)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		close(ch)
		return 0, ch
	}

	id := b.nextID
	b.nextID++
	b.subscribers[id] = ch
	return id, ch
}

// Unsubscribe removes a subscriber by ID and closes its channel. Returns
// true if the subscriber existed, false if it was already removed or
// never registered.
func (b *EventBus) Unsubscribe(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch, ok := b.subscribers[id]
	if !ok {
		return false
	}
	delete(b.subscribers, id)
	close(ch)
	return true
}

// Publish delivers an event to all registered subscribers using non-blocking
// sends. If a subscriber's channel is full, the event is silently dropped
// for that subscriber — the publisher is never blocked.
//
// The mutex is held for the entire delivery loop. This is safe because
// every send uses select-with-default (non-blocking, O(1) per subscriber),
// so the critical section is O(subscribers) with bounded, microsecond-scale
// hold time. Holding the lock prevents the send-on-closed-channel panic
// that would occur if Unsubscribe/Close closed a channel between snapshot
// and delivery.
//
// After Close has been called, Publish is a no-op.
func (b *EventBus) Publish(event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || len(b.subscribers) == 0 {
		return
	}

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Channel full — event dropped for this subscriber.
			b.droppedEvents.Add(1)
			slog.Debug("event dropped", "eventKind", event.Kind, "sessionId", event.SessionID)
		}
	}
}

// Close closes all subscriber channels and prevents further Publish calls.
// Subsequent calls to Close are no-ops. Subscribe called after Close
// returns a pre-closed channel.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
}

// DroppedCount returns the cumulative number of events that could not be
// delivered to at least one subscriber because its channel buffer was full.
// Safe to call concurrently from any goroutine.
func (b *EventBus) DroppedCount() int64 {
	return b.droppedEvents.Load()
}

// emit is the internal publish path used by the SessionManager worker
// goroutine. It constructs an Event and publishes it through the bus.
func (b *EventBus) emit(kind EventKind, sessionID SessionID) {
	b.Publish(Event{
		Kind:      kind,
		SessionID: sessionID,
		Time:      time.Now(),
	})
}
