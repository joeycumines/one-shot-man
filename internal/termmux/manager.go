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

	// started is closed by Run when the worker goroutine begins
	// processing. Used by sendRequest to detect calls before Run.
	started chan struct{}

	// readerCtx is a context derived from Run's ctx parameter. It is
	// cancelled during shutdown to signal all per-session reader
	// goroutines to exit. This prevents goroutine leaks when the
	// manager shuts down while sessions still have open Reader channels.
	readerCtx    context.Context
	readerCancel context.CancelFunc

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
		started:      make(chan struct{}),
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

	// Create a reader context for per-session goroutines. Cancelled
	// during shutdown to ensure they exit promptly.
	m.readerCtx, m.readerCancel = context.WithCancel(ctx)
	defer m.readerCancel()

	// Signal that the worker has started. This unblocks sendRequest
	// callers that were waiting for the worker to be ready.
	close(m.started)

	for {
		select {
		case <-ctx.Done():
			m.shutdownSessions()
			// Close reqChan so any callers blocked on sendRequest
			// will panic-recover with ErrManagerNotRunning.
			m.closeReqChan()
			return ctx.Err()
		case req, ok := <-m.reqChan:
			if !ok {
				m.shutdownSessions()
				return nil
			}
			m.dispatch(req)
		case so := <-m.mergedOutput:
			m.handleSessionOutput(so)
		}
	}
}

// Close signals the worker goroutine to stop by closing the request channel.
// It blocks until the worker has finished processing. Safe to call multiple
// times — subsequent calls are no-ops.
func (m *SessionManager) Close() {
	m.closeReqChan()
	<-m.done
}

// Started returns a channel that is closed when the worker goroutine has
// started processing requests. Callers that need to guarantee the manager
// is ready before sending requests can wait on this channel:
//
//	go mgr.Run(ctx)
//	<-mgr.Started()
//	mgr.Register(...)
func (m *SessionManager) Started() <-chan struct{} {
	return m.started
}

// closeReqChan idempotently closes reqChan.
func (m *SessionManager) closeReqChan() {
	defer func() { recover() }() // ignore double-close panic
	close(m.reqChan)
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

// sendRequest sends a request to the worker goroutine and blocks until the
// worker replies. Returns ErrManagerNotRunning if the worker has not started
// (Run not called) or has stopped (reqChan closed).
func (m *SessionManager) sendRequest(kind requestKind, payload any) (resp response) {
	// Fast-path guard: worker must have started.
	select {
	case <-m.started:
	default:
		return response{err: ErrManagerNotRunning}
	}

	reply := make(chan response, 1)
	req := request{kind: kind, payload: payload, reply: reply}
	defer func() {
		if r := recover(); r != nil {
			// reqChan was closed — worker has stopped.
			resp = response{err: ErrManagerNotRunning}
		}
	}()
	m.reqChan <- req

	// Wait for the worker's response. Also select on done to prevent
	// deadlock if the worker exits before processing this request
	// (e.g., context cancellation while requests are buffered).
	select {
	case resp = <-reply:
		return resp
	case <-m.done:
		return response{err: ErrManagerNotRunning}
	}
}

// Register adds a new session to the manager and returns its unique SessionID.
// The first registered session automatically becomes the active input target.
func (m *SessionManager) Register(session InteractiveSession, target SessionTarget) (SessionID, error) {
	resp := m.sendRequest(reqRegister, &registerPayload{session: session, target: target})
	if resp.err != nil {
		return 0, resp.err
	}
	return resp.value.(SessionID), nil
}

// Unregister closes and removes a session by ID.
func (m *SessionManager) Unregister(id SessionID) error {
	return m.sendRequest(reqUnregister, id).err
}

// Activate switches the active input target to the session with the given ID.
func (m *SessionManager) Activate(id SessionID) error {
	return m.sendRequest(reqActivate, id).err
}

// Input writes data to the active session's PTY.
func (m *SessionManager) Input(data []byte) error {
	return m.sendRequest(reqInput, data).err
}

// Resize broadcasts new terminal dimensions to all sessions.
func (m *SessionManager) Resize(rows, cols int) error {
	return m.sendRequest(reqResize, &resizePayload{rows: rows, cols: cols}).err
}

// Snapshot returns the latest screen snapshot for the given session, or nil
// if the session does not exist.
func (m *SessionManager) Snapshot(id SessionID) *ScreenSnapshot {
	resp := m.sendRequest(reqSnapshot, id)
	if resp.value == nil {
		return nil
	}
	snap, _ := resp.value.(*ScreenSnapshot)
	return snap
}

// ActiveID returns the active session's ID, or 0 if no session is active.
func (m *SessionManager) ActiveID() SessionID {
	resp := m.sendRequest(reqActiveID, nil)
	if resp.err != nil {
		return 0
	}
	return resp.value.(SessionID)
}

// Sessions returns a list of all managed sessions as value copies.
func (m *SessionManager) Sessions() []SessionInfo {
	resp := m.sendRequest(reqSessions, nil)
	if resp.err != nil {
		return nil
	}
	return resp.value.([]SessionInfo)
}

// dispatch routes a request to the appropriate handler. This method runs
// exclusively within the worker goroutine started by Run.
func (m *SessionManager) dispatch(req request) {
	var resp response
	switch req.kind {
	case reqRegister:
		resp = m.handleRegister(req.payload.(*registerPayload))
	case reqUnregister:
		resp = m.handleUnregister(req.payload.(SessionID))
	case reqActivate:
		resp = m.handleActivate(req.payload.(SessionID))
	case reqInput:
		resp = m.handleInput(req.payload.([]byte))
	case reqResize:
		resp = m.handleResize(req.payload.(*resizePayload))
	case reqSnapshot:
		resp = m.handleSnapshot(req.payload.(SessionID))
	case reqActiveID:
		resp = response{value: m.activeID}
	case reqSessions:
		resp = m.handleSessions()
	case reqClose:
		m.shutdownSessions()
		resp = response{}
	default:
		resp = response{err: fmt.Errorf("termmux: unknown request kind %d", req.kind)}
	}
	if req.reply != nil {
		req.reply <- resp
	}
}

// handleRegister creates a new managed session, assigns a SessionID, and
// stores it in the sessions map. The first registered session becomes active.
func (m *SessionManager) handleRegister(p *registerPayload) response {
	id := m.nextID
	m.nextID++
	m.snapshotGen++

	v := vt.NewVTerm(m.termRows, m.termCols)
	v.BellFn = func() {
		m.eventBus.emit(EventBell, id)
	}

	ms := &managedSession{
		session:    p.session,
		vterm:      v,
		state:      SessionCreated,
		target:     p.target,
		lastActive: time.Now(),
	}

	snap := &ScreenSnapshot{
		Gen:       m.snapshotGen,
		Rows:      m.termRows,
		Cols:      m.termCols,
		Timestamp: time.Now(),
	}
	ms.snapshot.Store(snap)
	m.sessions[id] = ms

	if m.activeID == 0 {
		m.activeID = id
	}

	m.eventBus.emit(EventSessionRegistered, id)

	// Spawn a per-session goroutine that pipes the session's Reader()
	// output into the merged output channel for worker processing.
	m.startReaderGoroutine(id, p.session)

	return response{value: id}
}

// handleUnregister validates the session exists and is not already closed,
// closes it, removes it from the map, and clears activeID if needed.
func (m *SessionManager) handleUnregister(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	if ms.state == SessionClosed {
		return response{err: fmt.Errorf("%w: already closed", ErrInvalidTransition)}
	}

	// Close the underlying session (ignore errors — best effort on teardown).
	_ = ms.session.Close()
	ms.state = SessionClosed
	delete(m.sessions, id)

	if m.activeID == id {
		m.activeID = 0
	}

	m.eventBus.emit(EventSessionClosed, id)
	return response{}
}

// handleActivate switches the active session. The session must exist and
// be in Created or Running state.
func (m *SessionManager) handleActivate(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	if ms.state != SessionCreated && ms.state != SessionRunning {
		return response{err: fmt.Errorf("%w: cannot activate session in state %s",
			ErrInvalidTransition, ms.state)}
	}
	m.activeID = id
	ms.lastActive = time.Now()
	m.eventBus.emit(EventSessionActivated, id)
	return response{}
}

// handleInput writes data to the active session's PTY.
func (m *SessionManager) handleInput(data []byte) response {
	if m.activeID == 0 {
		return response{err: ErrSessionNotFound}
	}
	ms, ok := m.sessions[m.activeID]
	if !ok {
		return response{err: fmt.Errorf("%w: active session %d disappeared",
			ErrSessionNotFound, m.activeID)}
	}
	if ms.state != SessionRunning && ms.state != SessionCreated {
		return response{err: fmt.Errorf("%w: session %d in state %s",
			ErrInvalidTransition, m.activeID, ms.state)}
	}
	_, err := ms.session.Write(data)
	return response{err: err}
}

// handleResize broadcasts the new terminal dimensions to all non-closed sessions.
func (m *SessionManager) handleResize(p *resizePayload) response {
	m.termRows = p.rows
	m.termCols = p.cols
	for id, ms := range m.sessions {
		if ms.state == SessionClosed {
			continue
		}
		ms.vterm.Resize(p.rows, p.cols)
		_ = ms.session.Resize(p.rows, p.cols)
		_ = id // used for iteration
	}
	m.eventBus.emit(EventResize, 0)
	return response{}
}

// handleSnapshot returns the latest screen snapshot for the given session.
func (m *SessionManager) handleSnapshot(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{}
	}
	return response{value: ms.snapshot.Load()}
}

// handleSessions builds a list of SessionInfo values from the sessions map.
func (m *SessionManager) handleSessions() response {
	infos := make([]SessionInfo, 0, len(m.sessions))
	for id, ms := range m.sessions {
		infos = append(infos, SessionInfo{
			ID:       id,
			Target:   ms.target,
			State:    ms.state,
			IsActive: id == m.activeID,
		})
	}
	return response{value: infos}
}

// handleSessionOutput processes a chunk of raw PTY output from the merged
// output channel. A nil data field is the EOF sentinel.
func (m *SessionManager) handleSessionOutput(so sessionOutput) {
	ms, ok := m.sessions[so.id]
	if !ok {
		return // Session already removed — discard.
	}

	// EOF sentinel: transition to Exited (from Running) or directly to
	// Closed (from Created — process exited without producing output).
	if so.data == nil {
		if ms.state.validTransition(SessionExited) {
			ms.state = SessionExited
			m.eventBus.emit(EventSessionExited, so.id)
		} else if ms.state == SessionCreated {
			// Process exited immediately without output (e.g., /bin/true).
			// Skip Exited and go directly to Closed.
			_ = ms.session.Close()
			ms.state = SessionClosed
			if m.activeID == so.id {
				m.activeID = 0
			}
			delete(m.sessions, so.id)
			m.eventBus.emit(EventSessionClosed, so.id)
		}
		return
	}

	// Transition Created → Running on first output.
	if ms.state == SessionCreated {
		ms.state = SessionRunning
	}

	// Tee to passthrough writer if active.
	if w := ms.passthroughWriter.Load(); w != nil {
		// Best-effort write — don't let passthrough errors affect VTerm.
		_, _ = (*w).Write(so.data)
	}

	// Update VTerm with the raw output.
	_, _ = ms.vterm.Write(so.data)

	// Publish a new immutable snapshot.
	m.snapshotGen++
	snap := &ScreenSnapshot{
		Gen:        m.snapshotGen,
		PlainText:  ms.vterm.String(),
		ANSI:       ms.vterm.ContentANSI(),
		FullScreen: ms.vterm.RenderFullScreen(),
		Rows:       m.termRows,
		Cols:       m.termCols,
		Timestamp:  time.Now(),
	}
	ms.snapshot.Store(snap)
	m.eventBus.emit(EventSessionOutput, so.id)
}

// startReaderGoroutine spawns a goroutine that reads from a session's
// Reader() channel and forwards each chunk to the merged output channel
// as sessionOutput{id, data}. When the Reader channel closes (EOF), it
// sends a nil-data sentinel.
//
// The goroutine respects readerCtx: if the context is cancelled (manager
// shutdown), the goroutine exits without attempting to send the EOF
// sentinel, since the worker may no longer be consuming mergedOutput.
//
// If session.Reader() returns nil (session not yet started), the goroutine
// polls periodically until Reader becomes available or the session's Done
// channel closes.
func (m *SessionManager) startReaderGoroutine(id SessionID, session InteractiveSession) {
	go func() {
		ch := waitForReader(m.readerCtx, session)
		if ch == nil {
			// Session closed or context cancelled before Reader available.
			select {
			case m.mergedOutput <- sessionOutput{id: id}:
			case <-m.readerCtx.Done():
			}
			return
		}

		for {
			select {
			case data, ok := <-ch:
				if !ok {
					// Reader channel closed (EOF).
					select {
					case m.mergedOutput <- sessionOutput{id: id}:
					case <-m.readerCtx.Done():
					}
					return
				}
				select {
				case m.mergedOutput <- sessionOutput{id: id, data: data}:
				case <-m.readerCtx.Done():
					return
				}
			case <-m.readerCtx.Done():
				return
			}
		}
	}()
}

// waitForReader polls session.Reader() until it returns a non-nil channel.
// Returns nil if the context is cancelled or the session's Done channel
// closes before Reader becomes available.
func waitForReader(ctx context.Context, session InteractiveSession) <-chan []byte {
	ch := session.Reader()
	if ch != nil {
		return ch
	}
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-session.Done():
			return nil
		case <-tick.C:
			if ch = session.Reader(); ch != nil {
				return ch
			}
		}
	}
}

// shutdownSessions performs deterministic, ordered shutdown of all sessions.
// Sessions are closed in descending ID order to ensure reproducible behavior.
func (m *SessionManager) shutdownSessions() {
	// Collect and sort session IDs in descending order.
	ids := make([]SessionID, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	sortSessionIDs(ids)

	// Close each session in sorted order.
	for _, id := range ids {
		ms := m.sessions[id]
		if ms.state != SessionClosed {
			_ = ms.session.Close()
			ms.state = SessionClosed
			m.eventBus.emit(EventSessionClosed, id)
		}
		delete(m.sessions, id)
	}
	m.activeID = 0
}

// sortSessionIDs sorts a slice of SessionIDs in descending order.
func sortSessionIDs(ids []SessionID) {
	// Simple insertion sort — session count is always small.
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] > ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
