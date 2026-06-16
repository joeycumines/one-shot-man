package termmux

import (
	"log/slog"
	"maps"
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

	// EventTitle is published when OSC 0 or OSC 2 sets the window title.
	// Data is the title string.
	EventTitle

	// EventWorkingDirectory is published when OSC 7 sets the working directory.
	// Data is the directory URI string.
	EventWorkingDirectory

	// EventClipboard is published when OSC 52 accesses the clipboard.
	// Data is the clipboard payload string (base64-encoded content after the
	// semicolon within the OSC data, e.g., "c;base64data").
	EventClipboard

	// EventActivity is published when a background session produces output
	// after being idle for a configurable duration.
	EventActivity

	// EventSilence is published when a session produces no output for a
	// configurable duration.
	EventSilence

	// EventWindowUpdated is published when a window's pane layout changes
	// (panes added, removed, or moved between windows).
	EventWindowUpdated
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
	case EventTitle:
		return "title"
	case EventWorkingDirectory:
		return "working-directory"
	case EventClipboard:
		return "clipboard"
	case EventActivity:
		return "activity"
	case EventSilence:
		return "silence"
	case EventWindowUpdated:
		return "window-updated"
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
	//   EventSessionOutput     → []byte (raw output chunk)
	//   EventResize            → [2]int{rows, cols}
	//   EventTitle             → string (window title)
	//   EventWorkingDirectory  → string (directory URI)
	//   EventClipboard         → string (clipboard payload)
	// Other kinds carry nil.
	Data any

	// Time records when the event was created.
	Time time.Time
}

// subscriberMap is an immutable snapshot of the current subscriber set.
// Mutations (Subscribe, Unsubscribe, Close) create a new copy and atomically
// swap the pointer, so Publish can iterate without holding a lock.
type subscriberMap struct {
	m map[int]chan<- Event
}

// EventBus provides typed, non-blocking fan-out event delivery to multiple
// subscribers. Publish is lock-free for the subscriber list read (atomic
// load via copy-on-write) and takes only a shared read lock during the
// send loop to prevent send-on-closed-channel races. Multiple Publish
// calls run concurrently; Subscribe/Unsubscribe/Close acquire an exclusive
// write lock to mutate the list and close channels safely.
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
	mu            sync.RWMutex
	subscribers   atomic.Pointer[subscriberMap]
	nextID        int // protected by mu write lock
	closed        atomic.Bool
	droppedEvents atomic.Int64
}

// NewEventBus creates an EventBus ready for use.
func NewEventBus() *EventBus {
	b := &EventBus{nextID: 1}
	b.subscribers.Store(&subscriberMap{m: make(map[int]chan<- Event)})
	return b
}

// Subscribe registers a new subscriber and returns its unique ID and a
// read-only event channel. The channel is buffered to bufSize; if bufSize
// is less than 1, it defaults to EventBusBufferSize. Events are delivered via non-blocking
// sends — a full channel means events are dropped for that subscriber
// (the EventBus never blocks the publisher).
//
// The returned channel is closed when Unsubscribe is called with the
// returned ID, or when Close is called on the EventBus.
func (b *EventBus) Subscribe(bufSize int) (int, <-chan Event) {
	if bufSize < 1 {
		bufSize = EventBusBufferSize
	}
	ch := make(chan Event, bufSize)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.Load() {
		close(ch)
		return 0, ch
	}

	id := b.nextID
	b.nextID++

	cur := b.subscribers.Load()
	newSnap := &subscriberMap{m: make(map[int]chan<- Event, len(cur.m)+1)}
	maps.Copy(newSnap.m, cur.m)
	newSnap.m[id] = ch
	b.subscribers.Store(newSnap)

	return id, ch
}

// Unsubscribe removes a subscriber by ID and closes its channel. Returns
// true if the subscriber existed, false if it was already removed or
// never registered.
func (b *EventBus) Unsubscribe(id int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	cur := b.subscribers.Load()
	ch, ok := cur.m[id]
	if !ok {
		return false
	}

	newSnap := &subscriberMap{m: make(map[int]chan<- Event, len(cur.m))}
	for k, v := range cur.m {
		if k != id {
			newSnap.m[k] = v
		}
	}
	b.subscribers.Store(newSnap)
	close(ch)
	return true
}

// Publish delivers an event to all registered subscribers using non-blocking
// sends. If a subscriber's channel is full, the event is silently dropped
// for that subscriber — the publisher is never blocked.
//
// Publish reads the subscriber list atomically via copy-on-write and holds
// only a shared read lock during the send loop, allowing multiple Publish
// calls to proceed concurrently. The read lock prevents Unsubscribe/Close
// from closing channels mid-send.
//
// After Close has been called, Publish is a no-op.
func (b *EventBus) Publish(event Event) {
	if b.closed.Load() {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed.Load() {
		return
	}

	snap := b.subscribers.Load()
	if snap == nil || len(snap.m) == 0 {
		return
	}

	for _, ch := range snap.m {
		select {
		case ch <- event:
		default:
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

	if b.closed.Load() {
		return
	}
	b.closed.Store(true)

	cur := b.subscribers.Load()
	for _, ch := range cur.m {
		close(ch)
	}
	b.subscribers.Store(&subscriberMap{m: make(map[int]chan<- Event)})
}

// DroppedCount returns the cumulative number of events that could not be
// delivered to at least one subscriber because its channel buffer was full.
// Safe to call concurrently from any goroutine.
func (b *EventBus) DroppedCount() int64 {
	return b.droppedEvents.Load()
}

// DataAsBytes returns the Data field as []byte if the event kind is
// EventSessionOutput; otherwise it returns nil, false.
func (e Event) DataAsBytes() ([]byte, bool) {
	if e.Kind != EventSessionOutput {
		return nil, false
	}
	if e.Data == nil {
		return nil, true
	}
	data, ok := e.Data.([]byte)
	return data, ok
}

// DataAsDims returns the Data field as [2]int{rows, cols} if the event kind
// is EventResize; otherwise it returns [2]int{0, 0}, false.
func (e Event) DataAsDims() ([2]int, bool) {
	if e.Kind != EventResize {
		return [2]int{0, 0}, false
	}
	data, ok := e.Data.([2]int)
	return data, ok
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

// emitData is like emit but attaches a kind-specific payload to the event.
// Used for events that carry additional data (e.g., EventResize with dimensions).
func (b *EventBus) emitData(kind EventKind, sessionID SessionID, data any) {
	b.Publish(Event{
		Kind:      kind,
		SessionID: sessionID,
		Data:      data,
		Time:      time.Now(),
	})
}
