package termmux

import "fmt"

// SessionKind describes the broad class of an interactive terminal endpoint.
type SessionKind string

const (
	// SessionKindUnknown is the zero value.
	SessionKindUnknown SessionKind = ""

	// SessionKindPTY identifies a PTY-backed interactive session.
	SessionKindPTY SessionKind = "pty"

	// SessionKindCapture identifies a capture-only PTY-backed session.
	SessionKindCapture SessionKind = "capture"
)

// SessionTarget identifies a named interactive session endpoint.
//
// It intentionally carries only generic metadata so callers can label Claude,
// verification shells, or future interactive panes without baking product
// concepts into the mux core.
type SessionTarget struct {
	// ID is an optional stable identifier for the session.
	ID string

	// Name is an optional human-readable label.
	Name string

	// Kind classifies the endpoint.
	Kind SessionKind
}

// IsZero reports whether the target carries no identifying metadata.
func (t SessionTarget) IsZero() bool {
	return t.ID == "" && t.Name == "" && t.Kind == SessionKindUnknown
}

// String returns a human-readable representation of the session kind.
func (k SessionKind) String() string {
	if k == SessionKindUnknown {
		return "unknown"
	}
	return string(k)
}

// String returns a compact human-readable representation.
func (t SessionTarget) String() string {
	switch {
	case t.Name != "" && t.ID != "" && t.Kind != SessionKindUnknown:
		return fmt.Sprintf("%s[%s:%s]", t.Name, t.Kind, t.ID)
	case t.Name != "" && t.Kind != SessionKindUnknown:
		return fmt.Sprintf("%s[%s]", t.Name, t.Kind)
	case t.Name != "":
		return t.Name
	case t.ID != "" && t.Kind != SessionKindUnknown:
		return fmt.Sprintf("%s[%s]", t.ID, t.Kind)
	case t.ID != "":
		return t.ID
	case t.Kind != SessionKindUnknown:
		return t.Kind.String()
	default:
		return "unknown"
	}
}

// WithName returns a copy of the target with Name set.
func (t SessionTarget) WithName(name string) SessionTarget {
	t.Name = name
	return t
}

// WithID returns a copy of the target with ID set.
func (t SessionTarget) WithID(id string) SessionTarget {
	t.ID = id
	return t
}

// WithKind returns a copy of the target with Kind set.
func (t SessionTarget) WithKind(kind SessionKind) SessionTarget {
	t.Kind = kind
	return t
}

// InteractiveSession is the shared contract for terminal endpoints that can
// render inline in the TUI and accept direct operator input.
//
// The contract is intentionally narrow: it captures the common behavior needed
// by both mux-backed PTY sessions and standalone CaptureSession instances,
// without forcing callers to know which implementation they received.
type InteractiveSession interface {
	Target() SessionTarget
	SetTarget(SessionTarget)
	Output() string
	Screen() string
	Resize(rows, cols int) error
	Write([]byte) (int, error)
	Close() error

	// Done returns a channel that is closed when the session terminates.
	// Callers can select on this channel to react to session completion
	// without polling. For Mux sessions, the channel closes when the
	// attached child exits or is detached. For CaptureSession, the
	// channel closes when the underlying process exits. If no session
	// is active, the returned channel is already closed.
	Done() <-chan struct{}

	// IsRunning reports whether the session is actively processing.
	// For Mux sessions, this is true when a child is attached.
	// For CaptureSession, this is true when the process is running
	// (started and not yet exited).
	IsRunning() bool
}
