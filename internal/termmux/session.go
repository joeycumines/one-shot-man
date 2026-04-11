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

// InteractiveSession is the minimal contract for terminal endpoints managed
// by [SessionManager].
//
// Implementations provide the essential PTY lifecycle operations: writing
// input, resizing, closing, completion signalling, and streaming raw output
// via a channel. Screen capture (VTerm), session metadata (Target), and
// lifecycle tracking (IsRunning) are the SessionManager's responsibility —
// they are NOT part of this interface.
//
// Concrete types such as [CaptureSession] and [StringIOSession] may offer
// additional methods (Target, Passthrough, etc.) beyond this interface
// for direct callers that hold the concrete type.
type InteractiveSession interface {
	// Write sends raw bytes to the session's PTY stdin.
	Write([]byte) (int, error)

	// Resize changes the PTY dimensions and delivers SIGWINCH.
	Resize(rows, cols int) error

	// Close terminates the session and releases resources.
	Close() error

	// Done returns a channel that is closed when the session terminates.
	// Callers can select on this channel to react to session completion
	// without polling.
	Done() <-chan struct{}

	// Reader returns a channel that streams raw PTY output chunks.
	// A nil value on the channel is never sent; instead, the channel is
	// closed when the session's output ends (process exit / PTY EOF).
	// The channel is safe to read from any goroutine.
	Reader() <-chan []byte
}
