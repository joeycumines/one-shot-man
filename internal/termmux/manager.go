package termmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// SessionID is a unique, monotonically increasing identifier assigned by the
// SessionManager when a session is registered. Zero is never assigned and
// is used as the sentinel "no session" value.
type SessionID uint64

// SessionState tracks the lifecycle of a managed session within the worker
// goroutine. Transitions are enforced by the worker — no external code may
// change a session's state directly.
//
// Valid transitions:
//
//	Created  → Running   (first output received)
//	Running  → Exited    (process exited, output drained)
//	Exited   → Closed    (unregister or shutdown)
//	Created  → Closed    (unregister before start)
type SessionState int

const (
	// SessionCreated means the session is registered but has not yet
	// produced output.
	SessionCreated SessionState = iota

	// SessionRunning means the session is actively producing output.
	SessionRunning

	// SessionExited means the session's process has exited and all
	// output has been drained through the VTerm.
	SessionExited

	// SessionClosed means all resources have been released. This is
	// a terminal state.
	SessionClosed
)

// String returns a human-readable name for the session state.
func (s SessionState) String() string {
	switch s {
	case SessionCreated:
		return "created"
	case SessionRunning:
		return "running"
	case SessionExited:
		return "exited"
	case SessionClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// validTransition reports whether transitioning from the current state to
// next is permitted by the lifecycle model.
func (s SessionState) validTransition(next SessionState) bool {
	switch s {
	case SessionCreated:
		return next == SessionRunning || next == SessionClosed
	case SessionRunning:
		return next == SessionExited
	case SessionExited:
		return next == SessionClosed
	case SessionClosed:
		return false
	default:
		return false
	}
}

// ScreenSnapshot is an immutable, point-in-time capture of a session's
// virtual terminal screen. It is published via atomic.Pointer by the worker
// goroutine and may be read concurrently by any number of goroutines without
// synchronization.
//
// All fields are value types (string, int, time.Time) — there is no shared
// mutable state. Callers receive a pointer to an immutable struct.
type ScreenSnapshot struct {
	// Gen is a monotonically increasing generation counter, incremented
	// each time the worker publishes a new snapshot for this session.
	// Consumers can compare generations to detect changes.
	Gen uint64

	// PlainText is the screen content without ANSI escape sequences.
	// Suitable for text search, clipboard copy, and plain-text capture.
	PlainText string

	// ANSI is the screen content with SGR escape sequences preserved.
	// Suitable for embedding in a TUI component (e.g., lipgloss pane).
	ANSI string

	// FullScreen is the screen content with CUP (cursor position)
	// escape sequences for full terminal restoration. Used during
	// passthrough re-entry for flicker-free screen redraw.
	FullScreen string

	// Rows is the terminal height at the time of capture.
	Rows int

	// Cols is the terminal width at the time of capture.
	Cols int

	// Timestamp records when this snapshot was created.
	Timestamp time.Time
}

// SessionInfo is an immutable summary of a managed session, safe for
// concurrent reads. It is returned by the Sessions() query method as a
// value copy — mutations to the returned slice do not affect the worker.
type SessionInfo struct {
	// ID is the unique identifier assigned at registration.
	ID SessionID

	// Target is the session's metadata (name, kind, stable ID).
	Target SessionTarget

	// State is the current lifecycle state.
	State SessionState

	// IsActive is true when this session is the active input target.
	IsActive bool
}

// requestKind identifies the type of request sent to the worker goroutine
// via the request channel. Each kind maps to a specific handler function.
type requestKind int

const (
	// reqRegister asks the worker to register a new session.
	// Payload: *registerPayload. Reply value: SessionID.
	reqRegister requestKind = iota

	// reqUnregister asks the worker to close and remove a session.
	// Payload: SessionID. Reply value: nil.
	reqUnregister

	// reqActivate asks the worker to switch the active session.
	// Payload: SessionID. Reply value: nil.
	reqActivate

	// reqInput asks the worker to write data to the active session.
	// Payload: []byte. Reply value: nil.
	reqInput

	// reqResize asks the worker to resize all sessions' VTerms and PTYs.
	// Payload: *resizePayload. Reply value: nil.
	reqResize

	// reqSnapshot asks the worker to return the latest screen snapshot
	// for a session. Payload: SessionID. Reply value: *ScreenSnapshot.
	reqSnapshot

	// reqActiveID asks the worker to return the active session's ID.
	// Payload: nil. Reply value: SessionID.
	reqActiveID

	// reqSessions asks the worker to return a list of all sessions.
	// Payload: nil. Reply value: []SessionInfo.
	reqSessions

	// reqClose asks the worker to initiate graceful shutdown.
	// Payload: nil. Reply value: nil.
	reqClose

	// reqActiveWriter asks the worker to return an io.Writer pointing
	// to the active session's PTY input. Used by the Passthrough
	// implementation for direct stdin forwarding.
	// Payload: nil. Reply value: io.Writer.
	reqActiveWriter

	// reqEnablePassthroughTee asks the worker to start teeing raw
	// output from the active session to a provided io.Writer, in
	// addition to feeding the VTerm. Used during passthrough for
	// low-latency stdout forwarding.
	// Payload: io.Writer. Reply value: nil.
	reqEnablePassthroughTee

	// reqDisablePassthroughTee asks the worker to stop teeing raw
	// output. Payload: nil. Reply value: nil.
	reqDisablePassthroughTee
)

// registerPayload carries the arguments for a reqRegister request.
type registerPayload struct {
	session InteractiveSession
	target  SessionTarget
}

// resizePayload carries the new terminal dimensions for a reqResize request.
type resizePayload struct {
	rows int
	cols int
}

// request is a typed message sent from a public API method to the worker
// goroutine via reqChan. The caller blocks on the reply channel until the
// worker has processed the request and sent a response.
type request struct {
	kind    requestKind
	payload any
	reply   chan<- response
}

// response is the worker's answer to a request. It carries either a typed
// value or an error (never both — a non-nil error means value is nil).
type response struct {
	value any
	err   error
}

// sessionOutput is a chunk of raw PTY output accompanied by the session's
// ID. Per-session reader goroutines send these to the worker's mergedOutput
// channel. A nil data field is the EOF sentinel indicating the session's
// output has ended.
type sessionOutput struct {
	id   SessionID
	data []byte // nil means EOF
}

// managedSession is the worker-owned wrapper around an InteractiveSession.
// All fields are accessed exclusively by the worker goroutine (except
// snapshot, which is published via atomic.Pointer for concurrent reads).
type managedSession struct {
	// session is the underlying interactive session (PTY, capture, etc.).
	session InteractiveSession

	// vterm is the virtual terminal owned by the worker goroutine.
	// Output chunks are written here to build the screen buffer.
	vterm *vt.VTerm

	// snapshot holds the latest immutable screen capture, published
	// via atomic.Pointer.Store after each VTerm update. Any goroutine
	// may call snapshot.Load() without synchronization.
	snapshot atomic.Pointer[ScreenSnapshot]

	// state tracks the session lifecycle (Created → Running → Exited → Closed).
	state SessionState

	// target carries the session's metadata (name, kind, ID).
	target SessionTarget

	// lastActive records the last time this session was the active input target.
	lastActive time.Time

	// outputCh receives raw PTY output chunks from the per-session
	// reader goroutine. The worker reads from this via mergedOutput.
	outputCh <-chan []byte

	// passthroughWriter, when non-nil, receives a copy of each raw output
	// chunk before VTerm processing. Used during passthrough for
	// low-latency stdout forwarding.
	passthroughWriter atomic.Pointer[io.Writer]
}

// SessionManager coordinates multiple interactive terminal sessions using a
// single worker goroutine that owns all mutable state. Public methods send
// requests to the worker via a channel and block on the reply.
//
// Create with NewSessionManager, then call Run to start the worker.
// All mutation methods (Register, Unregister, Activate, Input, Resize)
// block until the worker processes the request. Query methods (Snapshot,
// ActiveID, Sessions) also go through the worker for consistency but are
// fast (map lookups and value copies).
//
// The zero value is not usable. Use NewSessionManager.
type SessionManager struct {
	// reqChan receives requests from public API methods. The worker
	// goroutine is the sole consumer. Buffered to reduce contention.
	reqChan chan request

	// mergedOutput receives raw PTY output from all per-session reader
	// goroutines. The worker is the sole consumer.
	mergedOutput chan sessionOutput

	// eventBus provides fan-out event delivery to subscribers.
	eventBus *EventBus

	// done is closed when Run returns, signaling that the worker has
	// stopped and all resources have been released.
	done chan struct{}

	// --- Fields below are owned exclusively by the worker goroutine. ---
	// They are listed here for documentation but MUST NOT be accessed
	// outside the worker's select loop.

	// sessions maps registered session IDs to their managed wrappers.
	sessions map[SessionID]*managedSession

	// activeID is the session that receives input via Input().
	activeID SessionID

	// nextID is the monotonic counter for assigning SessionIDs.
	// Starts at 1 (0 is the sentinel "no session" value).
	nextID SessionID

	// termRows and termCols are the current terminal dimensions,
	// broadcast to all sessions on resize.
	termRows int
	termCols int

	// snapshotGen is the monotonic counter for ScreenSnapshot.Gen.
	snapshotGen uint64
}

// ManagerOption configures a SessionManager. Pass options to NewSessionManager.
type ManagerOption func(*SessionManager)

// WithTermSize sets the initial terminal dimensions. Defaults to 24 rows, 80 cols.
func WithTermSize(rows, cols int) ManagerOption {
	return func(m *SessionManager) {
		m.termRows = rows
		m.termCols = cols
	}
}

// WithRequestBuffer sets the capacity of the request channel. Defaults to 64.
func WithRequestBuffer(cap int) ManagerOption {
	return func(m *SessionManager) {
		m.reqChan = make(chan request, cap)
	}
}

// WithMergedOutputBuffer sets the capacity of the merged output channel. Defaults to 64.
func WithMergedOutputBuffer(cap int) ManagerOption {
	return func(m *SessionManager) {
		m.mergedOutput = make(chan sessionOutput, cap)
	}
}

// NewSessionManager creates a SessionManager with the given options.
// Call Run to start the worker goroutine.
func NewSessionManager(opts ...ManagerOption) *SessionManager {
	m := &SessionManager{
		reqChan:      make(chan request, 64),
		mergedOutput: make(chan sessionOutput, 64),
		eventBus:     NewEventBus(),
		done:         make(chan struct{}),
		sessions:     make(map[SessionID]*managedSession),
		nextID:       1,
		termRows:     24,
		termCols:     80,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// ErrManagerNotRunning is returned when a method is called before Run or
// after the manager has been closed.
var ErrManagerNotRunning = errors.New("termmux: session manager is not running")

// ErrSessionNotFound is returned when an operation references a SessionID
// that does not exist in the manager.
var ErrSessionNotFound = errors.New("termmux: session not found")

// ErrInvalidTransition is returned when a session state transition is
// not permitted by the lifecycle model.
var ErrInvalidTransition = errors.New("termmux: invalid state transition")

// Run starts the SessionManager worker goroutine and blocks until ctx is
// cancelled or Close is called. All request processing and state mutations
// happen exclusively within this goroutine. Run must be called exactly once.
func (m *SessionManager) Run(ctx context.Context) error {
	defer close(m.done)
	defer m.eventBus.Close()
	for {
		select {
		case <-ctx.Done():
			m.drainSessions()
			return ctx.Err()
		case req, ok := <-m.reqChan:
			if !ok {
				m.drainSessions()
				return nil
			}
			m.dispatch(req)
		}
	}
}

// Close signals the worker goroutine to stop by closing the request channel.
// It blocks until the worker has finished processing.
func (m *SessionManager) Close() {
	close(m.reqChan)
	<-m.done
}

// Subscribe registers a subscriber for events produced by this manager.
// The returned channel receives events; it is closed when Unsubscribe is
// called or the manager shuts down. bufSize controls the channel buffer
// (defaults to 64 if < 1). Events are delivered via non-blocking sends —
// a slow subscriber's events are silently dropped.
func (m *SessionManager) Subscribe(bufSize int) (int, <-chan Event) {
	return m.eventBus.Subscribe(bufSize)
}

// Unsubscribe removes a previously registered event subscriber and closes
// its channel. Returns true if the subscriber existed.
func (m *SessionManager) Unsubscribe(id int) bool {
	return m.eventBus.Unsubscribe(id)
}

// dispatch routes a request to the appropriate handler. This method runs
// exclusively within the worker goroutine started by Run.
func (m *SessionManager) dispatch(req request) {
	var resp response
	switch req.kind {
	case reqRegister:
		p := req.payload.(*registerPayload)
		id := SessionID(m.nextID)
		m.nextID++
		m.snapshotGen++
		ms := &managedSession{
			session:    p.session,
			vterm:      vt.NewVTerm(int(m.termRows), int(m.termCols)),
			state:      SessionCreated,
			target:     p.target,
			lastActive: time.Now(),
			outputCh:   make(chan []byte, 64),
		}
		if !ms.state.validTransition(SessionRunning) {
			resp = response{err: fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, ms.state, SessionRunning)}
			break
		}
		m.sessions[id] = ms
		if m.activeID == 0 {
			m.activeID = id
		}
		snap := &ScreenSnapshot{
			Gen:       m.snapshotGen,
			Rows:      m.termRows,
			Cols:      m.termCols,
			Timestamp: time.Now(),
		}
		ms.snapshot.Store(snap)
		m.eventBus.emit(EventSessionRegistered, id)
		resp = response{value: id}
	default:
		resp = response{err: errors.New("termmux: unimplemented request kind")}
	}
	if req.reply != nil {
		req.reply <- resp
	}
}

// drainSessions transitions all sessions to Closed state. Called during
// shutdown from the worker goroutine.
func (m *SessionManager) drainSessions() {
	for _, ms := range m.sessions {
		ms.state = SessionClosed
	}
}
