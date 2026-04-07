package termmux

import "time"

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
	//   EventResize         → *resizePayload
	// Other kinds carry nil.
	Data any

	// Time records when the event was created.
	Time time.Time
}

// EventBus provides typed, non-blocking fan-out event delivery to multiple
// subscribers. It is the ONLY type in the termmux package that uses a mutex
// (to protect the subscriber list).
//
// The full implementation is added in a subsequent task. This declaration
// establishes the type so that SessionManager can reference it.
type EventBus struct {
	// Fields will be added by the EventBus implementation task.
	_ struct{} // prevent unkeyed literals
}

// emit publishes an event to all registered subscribers. This is the
// worker-goroutine-only publish path — external code uses the public
// Subscribe API (added in a subsequent task). The kind's String method
// is used for debug logging when subscribers are added.
func (b *EventBus) emit(kind EventKind, sessionID SessionID) {
	_ = Event{
		Kind:      kind,
		SessionID: sessionID,
		Time:      time.Now(),
	}
	// Subscriber fan-out is implemented in the EventBus task.
	// For now, construct the event to exercise the Event type.
	_ = kind.String()
}
