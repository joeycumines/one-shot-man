package termmux

import (
	"sync"
	"time"

	"github.com/joeycumines/go-bigbuff"
)

// EventKind identifies the type of event published through the EventBus.
type EventKind int

const (
	// EventSessionRegistered is published when a session is added to the manager.
	EventSessionRegistered EventKind = iota

	// EventSessionActivated is published when the active session changes.
	EventSessionActivated

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
	//   EventResize            → [2]int{rows, cols}
	//   EventTitle             → string (window title)
	//   EventWorkingDirectory  → string (directory URI)
	//   EventClipboard         → string (clipboard payload)
	// Other kinds carry nil.
	Data any

	// Time records when the event was created.
	Time time.Time
}

// EventBus is a lossless, totally-ordered fan-out of Events, built on
// github.com/joeycumines/go-bigbuff.ChanPubSub.
//
// Delivery contract — read before adding a subscriber:
//
//   - There is ONE shared, unbuffered broadcast channel, not one channel per
//     subscriber. A publish performs exactly one synchronous send per
//     registered subscriber.
//   - Publish blocks until EVERY subscriber has received the value and called
//     Wait. There is no drop, no timeout, no cancellation, and no error return.
//     A subscriber that stops consuming therefore stalls all publishers; a
//     leaked subscriber stalls them permanently.
//   - Subscribers MUST run a dedicated goroutine on a tight
//     "receive → Wait → hand off downstream" cycle, and MUST unregister
//     (UnsubscribeEvents) on every exit path. All queueing, batching and
//     backpressure belong DOWNSTREAM of Wait, never before it.
//   - A subscriber goroutine must NEVER call Publish (immediate self-deadlock).
//   - Subscriber counts must exactly equal the number of goroutines receiving:
//     extra registered subscribers steal sends from real ones.
//   - Publish order is a single total order; every subscriber observes the
//     same sequence.
//
// See [SessionManager.SubscribeEvents] for the intended subscription entry
// point, which increments the subscriber count and returns the shared channel
// in one step.
type EventBus struct {
	pubsub    *bigbuff.ChanPubSub[chan Event, Event]
	closeOnce sync.Once
}

// NewEventBus creates an EventBus ready for use. The broadcast channel is
// unbuffered by construction — ChanPubSub rejects a buffered channel.
func NewEventBus() *EventBus {
	return &EventBus{
		pubsub: bigbuff.NewChanPubSub(make(chan Event)),
	}
}

// Channel returns the shared broadcast channel. Receiving from it is only
// legal while the caller is a registered subscriber (see Subscribe), and each
// received value MUST be followed immediately by Wait — prefer the
// SessionManager.SubscribeEvents/AckEvent pair, which names that intent.
func (b *EventBus) Channel() <-chan Event {
	return b.pubsub.C()
}

// Subscribe registers the caller as a subscriber by incrementing the shared
// subscriber count. The caller must be about to receive on Channel and must
// pair this with exactly one Unsubscribe.
func (b *EventBus) Subscribe() {
	b.pubsub.Subscribe()
}

// Unsubscribe removes a previously registered subscriber. It drains any
// in-flight send on the departing subscriber's behalf, so a publisher blocked
// on this subscriber is released.
func (b *EventBus) Unsubscribe() {
	b.pubsub.Unsubscribe()
}

// Publish delivers an event to every registered subscriber. It blocks until
// all of them have received the event and acknowledged it with Wait; it never
// drops an event and never returns an error. When there are no subscribers it
// returns immediately.
//
// Publish is called exclusively by the SessionManager worker goroutine, which
// is what makes the "close only after the worker has stopped" lifecycle in
// Run safe: there is no publisher left to race the channel close.
func (b *EventBus) Publish(event Event) {
	b.pubsub.Send(event)
}

// Wait acknowledges the value most recently received by the calling
// subscriber. It MUST be called exactly once per received value, immediately
// after receiving it, and at no other time. Skipping it stalls the publisher.
func (b *EventBus) Wait() {
	b.pubsub.Wait()
}

// Close closes the shared broadcast channel, waking every subscriber with a
// closed channel. It is idempotent.
//
// Close must only be called once no Publish can occur: a Send racing a channel
// close panics and permanently breaks the bus. The SessionManager satisfies
// this by closing only after its worker goroutine has returned. Subscribers
// observe the close as a receive with ok == false and MUST NOT call Wait for
// it.
func (b *EventBus) Close() {
	b.closeOnce.Do(func() {
		close(b.pubsub.C())
	})
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
