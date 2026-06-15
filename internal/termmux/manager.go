package termmux

import (
	"cmp"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"
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
// The cell grid is stored as a *vt.Screen and rendered representations
// (plain text, ANSI, full screen) are computed lazily on first access via
// GetPlainText, GetANSI, and GetFullScreen. This avoids rendering work on
// output chunks where no consumer reads the snapshot, reducing memory
// footprint for unconsumed snapshots to cell-grid size only.
type ScreenSnapshot struct {
	// Gen is a monotonically increasing generation counter, incremented
	// each time the worker publishes a new snapshot for this session.
	// Consumers can compare generations to detect changes.
	Gen uint64

	// screen holds the cell grid captured from the VTerm. Rendering methods
	// traverse this grid lazily on first access.
	screen *vt.Screen

	// Cached render outputs, guarded by sync.Once for thread-safe lazy init.
	plainTextOnce  sync.Once
	plainTextCache string

	ansiOnce  sync.Once
	ansiCache string

	fullScreenOnce  sync.Once
	fullScreenCache string

	// Rows is the terminal height at the time of capture.
	Rows int

	// Cols is the terminal width at the time of capture.
	Cols int

	// CursorRow is the cursor's row position (0-indexed) at capture time.
	CursorRow int

	// CursorCol is the cursor's column position (0-indexed) at capture time.
	CursorCol int

	// MouseTracking indicates the child's active mouse tracking level.
	// Values: 0=none, 1=basic (1000), 2=button-event (1002), 3=any-event (1003).
	MouseTracking int

	// MouseSGR is true when the child has enabled SGR mouse encoding (1006).
	MouseSGR bool

	// InsertMode is true when the child has enabled insert/replace mode (IRM,
	// ANSI mode 4). In insert mode, printable characters are inserted at the
	// cursor position, shifting existing text right. In replace mode (default),
	// characters overwrite the existing character at the cursor.
	InsertMode bool

	// BracketedPaste is true when the child has enabled bracketed paste mode
	// (DECSET ?2004h). When true, pasted content should be wrapped with
	// ESC[200~ and ESC[201~ delimiters.
	BracketedPaste bool

	// ApplicationCursor is true when the child has enabled application cursor
	// mode (DECSET ?1h, DECCKM). When true, arrow keys and home/end use SS3
	// sequences (ESC O{A-D/H/F) instead of CSI sequences.
	ApplicationCursor bool

	// KeypadApplication is true when the child has enabled keypad application
	// mode (DECSET ?66h, DECKPAM). When true, keypad keys send SS3 sequences
	// (ESC O p–y for digits, ESC O M for enter, etc.) instead of ASCII.
	KeypadApplication bool

	// CursorShape is the current cursor shape (DECSCUSR). Values: 0=default,
	// 1=blink-block, 2=steady-block, 3=blink-underline, 4=steady-underline,
	// 5=blink-bar, 6=steady-bar.
	CursorShape int

	// FocusReporting is true when the child has enabled focus event reporting
	// (DECSET ?1004h). When true, focus-in/focus-out events should be sent as
	// ESC[I and ESC[O via ResponseWriter.
	FocusReporting bool

	// SynchronizedOutput is true when the child has enabled synchronized output
	// mode (DECSET ?2026h). When true, rapid updates are batched and the
	// consumer should defer rendering until SynchronizedOutput becomes false.
	SynchronizedOutput bool

	// AutoWrap is true when auto-wrap mode (DECAWM, DECSET ?7h) is active.
	// When true (default), characters at the right margin wrap to the next line.
	AutoWrap bool

	// LineFeedNewLine is true when line feed new line mode (LNM, ANSI mode 20)
	// is active. When true, LF also performs a carriage return.
	LineFeedNewLine bool

	// Timestamp records when this snapshot was created.
	Timestamp time.Time
}

// GetPlainText lazily computes and returns the screen content without ANSI
// escape sequences. Suitable for text search, clipboard copy, and plain-text
// capture. The result is cached after the first call.
func (s *ScreenSnapshot) GetPlainText() string {
	s.plainTextOnce.Do(func() {
		if s.screen != nil {
			s.plainTextCache, _, _ = vt.RenderAll(s.screen)
		}
	})
	return s.plainTextCache
}

// GetANSI lazily computes and returns the screen content with SGR escape
// sequences preserved. Suitable for embedding in a TUI component (e.g.,
// lipgloss pane). The result is cached after the first call.
func (s *ScreenSnapshot) GetANSI() string {
	s.ansiOnce.Do(func() {
		if s.screen != nil {
			_, s.ansiCache, _ = vt.RenderAll(s.screen)
		}
	})
	return s.ansiCache
}

// GetFullScreen lazily computes and returns the screen content with CUP
// (cursor position) escape sequences for full terminal restoration. Used
// during passthrough re-entry for flicker-free screen redraw. The result
// is cached after the first call.
func (s *ScreenSnapshot) GetFullScreen() string {
	s.fullScreenOnce.Do(func() {
		if s.screen != nil {
			_, _, s.fullScreenCache = vt.RenderAll(s.screen)
		}
	})
	return s.fullScreenCache
}

// NewScreenSnapshot creates a ScreenSnapshot backed by the given cell grid.
// Rendering is deferred until GetPlainText, GetANSI, or GetFullScreen is
// called. This is the primary constructor for production snapshots.
func NewScreenSnapshot(gen uint64, scr *vt.Screen, rows, cols int, ts time.Time) *ScreenSnapshot {
	return &ScreenSnapshot{
		Gen:       gen,
		screen:    scr,
		Rows:      rows,
		Cols:      cols,
		CursorRow: scr.CurRow,
		CursorCol: scr.CurCol,
		Timestamp: ts,
	}
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

	// reqExportState asks the worker to build and return a
	// [PersistedManagerState] snapshot of all managed sessions.
	// Payload: nil. Reply value: *PersistedManagerState.
	reqExportState

	// reqTermSize asks the worker to return the current terminal
	// dimensions. Payload: nil. Reply value: [2]int{rows, cols}.
	reqTermSize

	// reqRestoreState asks the worker to restore sessions from a
	// persisted state snapshot. Payload: *restoreStatePayload.
	// Reply value: *RestoreResult.
	reqRestoreState

	// reqResizeSession asks the worker to resize a single session's
	// VTerm and PTY. Payload: *resizeSessionPayload. Reply value: nil.
	reqResizeSession

	// reqScreen asks the worker to return a deep copy of the session's
	// VTerm screen (with Cells and Attr). Payload: SessionID.
	// Reply value: *vt.Screen.
	reqScreen

	// reqNewPane asks the worker to create a new pane with a session.
	// Payload: *newPanePayload. Reply value: PaneID.
	reqNewPane

	// reqClosePane asks the worker to close a pane and its session.
	// Payload: PaneID. Reply value: nil.
	reqClosePane

	// reqFocusPane asks the worker to switch focus to a pane.
	// Payload: PaneID. Reply value: nil.
	reqFocusPane

	// reqResizePane asks the worker to resize a pane's split ratio.
	// Payload: *resizePanePayload. Reply value: nil.
	reqResizePane

	// reqFocusAt asks the worker to focus the pane at the given coordinates.
	// Payload: *focusAtPayload. Reply value: PaneID.
	reqFocusAt

	// reqResizePaneAt asks the worker to resize the pane adjacent to the
	// divider at the given coordinates. Payload: *resizePaneAtPayload. Reply value: nil.
	reqResizePaneAt

	// reqPanes asks the worker to return the current pane list.
	// Payload: nil. Reply value: []Pane.
	reqPanes

	// reqFocusNextPane asks the worker to move focus to an adjacent pane.
	// Payload: NavigationDirection. Reply value: PaneID.
	reqFocusNextPane

	// reqActivePaneID asks the worker to return the active pane's ID.
	// Payload: nil. Reply value: PaneID.
	reqActivePaneID

	// reqIsCopyModeActive asks whether copy mode is active for a session.
	// Payload: SessionID. Reply value: bool.
	reqIsCopyModeActive

	// reqScrollCopyMode asks the worker to scroll in copy mode.
	// Payload: *scrollCopyModePayload. Reply value: bool.
	reqScrollCopyMode

	// reqEnterCopyMode asks the worker to enter copy mode for a session.
	// Payload: SessionID. Reply value: none.
	reqEnterCopyMode

	// reqExitCopyMode asks the worker to exit copy mode for a session.
	// Payload: SessionID. Reply value: none.
	reqExitCopyMode

	// reqSelectStart sets the copy-mode selection start position.
	// Payload: selectPayload. Reply value: none.
	reqSelectStart

	// reqSelectEnd sets the copy-mode selection end position.
	// Payload: selectPayload. Reply value: none.
	reqSelectEnd

	// reqNewWindow asks the worker to create a new window.
	// Payload: *newWindowPayload. Reply value: WindowID.
	reqNewWindow

	// reqNextWindow asks the worker to switch to the next window.
	// Payload: none. Reply value: WindowID.
	reqNextWindow

	// reqPrevWindow asks the worker to switch to the previous window.
	// Payload: none. Reply value: WindowID.
	reqPrevWindow

	// reqRenameWindow asks the worker to rename a window.
	// Payload: *renameWindowPayload. Reply value: none.
	reqRenameWindow

	// reqCloseWindow asks the worker to close a window.
	// Payload: WindowID. Reply value: none.
	reqCloseWindow

	// reqActiveWindowID asks the worker to return the active window's ID.
	// Payload: none. Reply value: WindowID.
	reqActiveWindowID

	// reqWindows asks the worker to return a list of all windows.
	// Payload: none. Reply value: []*Window.
	reqWindows

	// reqSetSynchronizePanes asks the worker to toggle synchronized panes.
	// Payload: bool. Reply value: none.
	reqSetSynchronizePanes

	// reqSynchronizePanes asks the worker to return the synchronize state.
	// Payload: none. Reply value: bool.
	reqSynchronizePanes

	// reqSetMonitorConfig sets the monitoring configuration for a session.
	// Payload: monitorConfigPayload. Reply value: none.
	reqSetMonitorConfig

	// reqMonitorConfig returns the monitoring configuration for a session.
	// Payload: SessionID. Reply value: MonitorConfig.
	reqMonitorConfig

	// reqVisualBellActive returns whether a visual bell flash is active.
	// Payload: SessionID. Reply value: bool.
	reqVisualBellActive

	// reqCheckSilenceMonitors checks all sessions for silence threshold
	// violations and emits events. Payload: none. Reply value: int.
	reqCheckSilenceMonitors

	// reqSetRemainOnExit sets the global remain-on-exit default.
	// Payload: bool. Reply value: none.
	reqSetRemainOnExit

	// reqRemainOnExit returns the global remain-on-exit default.
	// Payload: none. Reply value: bool.
	reqRemainOnExit

	// reqSetPaneRemainOnExit sets remain-on-exit for a specific pane.
	// Payload: paneRemainOnExitPayload. Reply value: none.
	reqSetPaneRemainOnExit

	// reqPaneRemainOnExit returns remain-on-exit for a specific pane.
	// Payload: PaneID. Reply value: bool.
	reqPaneRemainOnExit

	// reqPaneExited returns whether a pane has exited.
	// Payload: PaneID. Reply value: bool.
	reqPaneExited

	// reqRespawnSession respawns a session that is in Exited state.
	// Payload: SessionID. Reply value: SessionID.
	reqRespawnSession

	// reqSwapPanes swaps two panes' positions.
	// Payload: swapPanesPayload. Reply value: none.
	reqSwapPanes

	// reqZoomPane toggles zoom on a pane.
	// Payload: PaneID. Reply value: none.
	reqZoomPane

	// reqZoomedPane returns the currently zoomed pane ID.
	// Payload: none. Reply value: PaneID.
	reqZoomedPane

	// reqSetPipeFile sets a file path for piping pane output.
	// Payload: pipeFilePayload. Reply value: none.
	reqSetPipeFile

	// reqClearPipe clears the pipe writer for a session.
	// Payload: SessionID. Reply value: none.
	reqClearPipe

	// reqDisplayMessage sets a transient message on a session.
	// Payload: displayMessagePayload. Reply value: none.
	reqDisplayMessage

	// reqActiveMessage returns the active message text for a session.
	// Payload: SessionID. Reply value: string.
	reqActiveMessage

	// reqCapturePane returns region text from a session's snapshot.
	// Payload: capturePanePayload. Reply value: string.
	reqCapturePane

	// reqCopySelection returns the current copy-mode selection as an OSC 52
	// clipboard sequence for the given session.
	// Payload: SessionID. Reply value: string.
	reqCopySelection

	// reqLockSession locks a session with a password.
	// Payload: lockPayload. Reply value: none.
	reqLockSession

	// reqUnlockSession attempts to unlock a session.
	// Payload: lockPayload. Reply value: bool.
	reqUnlockSession

	// reqIsLocked returns whether a session is locked.
	// Payload: SessionID. Reply value: bool.
	reqIsLocked

	// reqBreakPane breaks the active pane out of its window into a new one.
	// Payload: WindowID. Reply value: WindowID.
	reqBreakPane

	// reqJoinPane joins the active pane from one window into another.
	// Payload: joinPanePayload. Reply value: none.
	reqJoinPane

	// reqAddPaneToWindow adds a session as a pane to a specific window.
	// Payload: *addPaneToWindowPayload. Reply value: PaneID.
	reqAddPaneToWindow
)

// registerPayload carries the arguments for a reqRegister request.
type registerPayload struct {
	session InteractiveSession
	target  SessionTarget
}

// monitorConfigPayload carries the session ID and new monitor config.
type monitorConfigPayload struct {
	sessionID SessionID
	config    MonitorConfig
}

// paneRemainOnExitPayload carries the pane ID and remain-on-exit value.
type paneRemainOnExitPayload struct {
	paneID PaneID
	value  bool
}

// swapPanesPayload carries two pane IDs to swap.
type swapPanesPayload struct {
	a, b PaneID
}

// pipeFilePayload carries the session ID and file path for pipe-pane.
type pipeFilePayload struct {
	sessionID SessionID
	path      string
}

// displayMessage is a transient message shown as an overlay.
type displayMessage struct {
	text      string
	expiresAt time.Time
}

// displayMessagePayload carries the session ID, text, and duration.
type displayMessagePayload struct {
	sessionID SessionID
	text      string
	duration  time.Duration
}

// capturePanePayload carries the session ID and row range.
type capturePanePayload struct {
	sessionID SessionID
	startLine int
	endLine   int
}

// selectPayload carries the session ID and selection coordinates.
type selectPayload struct {
	sessionID SessionID
	row       int
	col       int
}

// lockPayload carries the session ID and plaintext password.
type lockPayload struct {
	sessionID SessionID
	password  string
}

// resizePayload carries the new terminal dimensions for a reqResize request.
type resizePayload struct {
	rows int
	cols int
}

// resizeSessionPayload carries the target session ID and new dimensions for a
// reqResizeSession request.
type resizeSessionPayload struct {
	id   SessionID
	rows int
	cols int
}

// restoreStatePayload carries the arguments for a reqRestoreState request.
type restoreStatePayload struct {
	state   *PersistedManagerState
	factory func(PersistedSession) (InteractiveSession, error)
}

// newPanePayload carries the arguments for a reqNewPane request.
type newPanePayload struct {
	session   InteractiveSession
	target    SessionTarget
	direction SplitDirection
}

// resizePanePayload carries the pane ID and ratio for a reqResizePane request.
type resizePanePayload struct {
	id    PaneID
	ratio float64
}

// focusAtPayload carries the row and column for a reqFocusAt request.
type focusAtPayload struct {
	row int
	col int
}

// resizePaneAtPayload carries the row, column, and ratio for a
// reqResizePaneAt request.
type resizePaneAtPayload struct {
	row   int
	col   int
	ratio float64
}

// scrollCopyModePayload carries the session ID and scroll delta.
type scrollCopyModePayload struct {
	id    SessionID
	delta int
}

// newWindowPayload carries the name for a new window.
type newWindowPayload struct {
	name string
}

// renameWindowPayload carries the window ID and new name.
type renameWindowPayload struct {
	id   WindowID
	name string
}

// breakPanePayload carries the source window ID for a reqBreakPane request.
type breakPanePayload struct {
	windowID WindowID
}

// joinPanePayload carries source and target window IDs for a reqJoinPane request.
type joinPanePayload struct {
	sourceWindowID WindowID
	targetWindowID WindowID
}

// addPaneToWindowPayload carries arguments for reqAddPaneToWindow.
type addPaneToWindowPayload struct {
	session   InteractiveSession
	target    SessionTarget
	windowID  WindowID
	direction SplitDirection
}

// RestoreResult describes the outcome of a [SessionManager.RestoreFromState]
// call. It reports how many sessions were successfully re-created and which
// ones failed.
type RestoreResult struct {
	// Restored lists the session IDs that were successfully restored.
	Restored []SessionID

	// Failed lists the session IDs that could not be restored, along with
	// the error that prevented restoration.
	Failed []RestoreFailure
}

// RestoreFailure records why a single session could not be restored.
type RestoreFailure struct {
	SessionID SessionID
	Error     error
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

	remainOnExit bool

	pipeWriter atomic.Pointer[io.Writer]

	message atomic.Pointer[displayMessage]

	lock SessionLock
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

	// passthroughSessionID is the session ID currently in passthrough
	// mode, or 0 if none. Only the worker goroutine reads/writes this.
	// It prevents concurrent passthrough calls and ensures tee
	// enable/disable operates on the correct session.
	passthroughSessionID SessionID

	paneMgr *paneManager

	// windowMgr manages windows, each with its own pane layout.
	windowMgr *WindowManager

	// monitors tracks per-pane monitoring state keyed by SessionID.
	monitors map[SessionID]*MonitorState

	// visualBellDuration is how long a visual bell flash lasts.
	visualBellDuration time.Duration
}

// ManagerOption configures a SessionManager. Pass options to NewSessionManager.
type ManagerOption func(*SessionManager)

// WithTermSize sets the initial terminal dimensions. Defaults to DefaultRows rows, DefaultCols cols.
func WithTermSize(rows, cols int) ManagerOption {
	return func(m *SessionManager) {
		m.termRows = rows
		m.termCols = cols
	}
}

// WithRequestBuffer sets the capacity of the request channel. Defaults to DefaultChannelBuffer.
func WithRequestBuffer(cap int) ManagerOption {
	return func(m *SessionManager) {
		m.reqChan = make(chan request, cap)
	}
}

// WithMergedOutputBuffer sets the capacity of the merged output channel. Defaults to DefaultChannelBuffer.
func WithMergedOutputBuffer(cap int) ManagerOption {
	return func(m *SessionManager) {
		m.mergedOutput = make(chan sessionOutput, cap)
	}
}

// NewSessionManager creates a SessionManager with the given options.
// Call Run to start the worker goroutine.
func NewSessionManager(opts ...ManagerOption) *SessionManager {
	m := &SessionManager{
		reqChan:            make(chan request, DefaultChannelBuffer),
		mergedOutput:       make(chan sessionOutput, DefaultChannelBuffer),
		eventBus:           NewEventBus(),
		done:               make(chan struct{}),
		started:            make(chan struct{}),
		sessions:           make(map[SessionID]*managedSession),
		nextID:             1,
		termRows:           DefaultRows,
		termCols:           DefaultCols,
		paneMgr:            newPaneManager(LayoutVertical, DefaultCols, DefaultRows),
		windowMgr:          NewWindowManager(LayoutVertical, DefaultCols, DefaultRows),
		monitors:           make(map[SessionID]*MonitorState),
		visualBellDuration: 200 * time.Millisecond,
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
// (defaults to EventBusBufferSize if < 1). Events are delivered via non-blocking sends —
// a slow subscriber's events are silently dropped.
func (m *SessionManager) Subscribe(bufSize int) (int, <-chan Event) {
	return m.eventBus.Subscribe(bufSize)
}

// Unsubscribe removes a previously registered event subscriber and closes
// its channel. Returns true if the subscriber existed.
func (m *SessionManager) Unsubscribe(id int) bool {
	return m.eventBus.Unsubscribe(id)
}

// EventsDropped returns the cumulative number of events that could not be
// delivered to at least one subscriber because its channel buffer was full.
// Safe to call from any goroutine.
func (m *SessionManager) EventsDropped() int64 {
	return m.eventBus.DroppedCount()
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

// ResizeSession resizes a single session's VTerm and PTY to the given
// dimensions. Returns ErrSessionNotFound if the session does not exist.
func (m *SessionManager) ResizeSession(id SessionID, rows, cols int) error {
	return m.sendRequest(reqResizeSession, &resizeSessionPayload{id: id, rows: rows, cols: cols}).err
}

// TermSize returns the current terminal dimensions known to the manager.
func (m *SessionManager) TermSize() (rows, cols int) {
	resp := m.sendRequest(reqTermSize, nil)
	if resp.value == nil {
		return 0, 0
	}
	size := resp.value.([2]int)
	return size[0], size[1]
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

// Screen returns a deep copy of the VTerm screen for the given session,
// including Cells with Attr (colors, bold, italic, etc.). Returns nil
// if the session does not exist.
func (m *SessionManager) Screen(id SessionID) *vt.Screen {
	resp := m.sendRequest(reqScreen, id)
	if resp.value == nil {
		return nil
	}
	scr, _ := resp.value.(*vt.Screen)
	return scr
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

// NewPane creates a new pane by registering a session and adding it to
// the pane layout. The pane is split from the active pane in the given
// direction. Returns the new PaneID.
func (m *SessionManager) NewPane(session InteractiveSession, target SessionTarget, direction SplitDirection) (PaneID, error) {
	resp := m.sendRequest(reqNewPane, &newPanePayload{session: session, target: target, direction: direction})
	if resp.err != nil {
		return 0, resp.err
	}
	return resp.value.(PaneID), nil
}

// ClosePane removes the pane and its associated session. The session is
// unregistered and its resources are released.
func (m *SessionManager) ClosePane(id PaneID) error {
	return m.sendRequest(reqClosePane, id).err
}

// FocusPane switches the active pane to the one with the given ID.
// The associated session becomes the active input target.
func (m *SessionManager) FocusPane(id PaneID) error {
	return m.sendRequest(reqFocusPane, id).err
}

// ResizePane adjusts the split ratio for the given pane.
func (m *SessionManager) ResizePane(id PaneID, ratio float64) error {
	return m.sendRequest(reqResizePane, &resizePanePayload{id: id, ratio: ratio}).err
}

// Panes returns a snapshot of all panes with their computed geometries.
func (m *SessionManager) Panes() []Pane {
	resp := m.sendRequest(reqPanes, nil)
	if resp.err != nil {
		return nil
	}
	return resp.value.([]Pane)
}

// FocusNextPane moves focus to the adjacent pane in the given direction.
// Returns the new active PaneID.
func (m *SessionManager) FocusNextPane(direction NavigationDirection) PaneID {
	return m.paneMgr.FocusNext(direction)
}

// FocusAt focuses the pane at the given screen coordinates and updates the
// active session to that pane's session. Returns the focused PaneID or an
// error if no pane is at the coordinate.
func (m *SessionManager) FocusAt(row, col int) (PaneID, error) {
	resp := m.sendRequest(reqFocusAt, &focusAtPayload{row: row, col: col})
	if resp.err != nil {
		return 0, resp.err
	}
	return resp.value.(PaneID), nil
}

// ResizePaneAt resizes the pane adjacent to the divider at the given screen
// coordinates and propagates the new geometry to the affected sessions.
// Returns an error if no divider is at the coordinate.
func (m *SessionManager) ResizePaneAt(row, col int, ratio float64) error {
	return m.sendRequest(reqResizePaneAt, &resizePaneAtPayload{row: row, col: col, ratio: ratio}).err
}

// ActivePaneID returns the currently focused pane ID, or 0 if no panes exist.
func (m *SessionManager) ActivePaneID() PaneID {
	return m.paneMgr.ActivePaneID()
}

// IsCopyModeActive reports whether copy mode is active for the given session.
func (m *SessionManager) IsCopyModeActive(id SessionID) bool {
	resp := m.sendRequest(reqIsCopyModeActive, id)
	if resp.value == nil {
		return false
	}
	return resp.value.(bool)
}

// ScrollCopyMode scrolls the viewport by delta lines in copy mode for the given
// session. Returns false if copy mode is not active or the session does not exist.
func (m *SessionManager) ScrollCopyMode(id SessionID, delta int) bool {
	resp := m.sendRequest(reqScrollCopyMode, &scrollCopyModePayload{id: id, delta: delta})
	if resp.value == nil {
		return false
	}
	return resp.value.(bool)
}

// EnterCopyMode enters copy mode for the given session.
func (m *SessionManager) EnterCopyMode(id SessionID) error {
	return m.sendRequest(reqEnterCopyMode, id).err
}

// ExitCopyMode exits copy mode for the given session.
func (m *SessionManager) ExitCopyMode(id SessionID) error {
	return m.sendRequest(reqExitCopyMode, id).err
}

// SelectStart sets the copy-mode selection start position for the given session.
func (m *SessionManager) SelectStart(id SessionID, row, col int) error {
	return m.sendRequest(reqSelectStart, selectPayload{sessionID: id, row: row, col: col}).err
}

// SelectEnd sets the copy-mode selection end position for the given session.
func (m *SessionManager) SelectEnd(id SessionID, row, col int) error {
	return m.sendRequest(reqSelectEnd, selectPayload{sessionID: id, row: row, col: col}).err
}

// NewWindow creates a new window with the given name and returns its ID.
func (m *SessionManager) NewWindow(name string) (WindowID, error) {
	resp := m.sendRequest(reqNewWindow, &newWindowPayload{name: name})
	if resp.err != nil {
		return 0, resp.err
	}
	return resp.value.(WindowID), nil
}

// NextWindow switches to the next window. Returns the new active WindowID.
func (m *SessionManager) NextWindow() WindowID {
	resp := m.sendRequest(reqNextWindow, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(WindowID)
}

// PrevWindow switches to the previous window. Returns the new active WindowID.
func (m *SessionManager) PrevWindow() WindowID {
	resp := m.sendRequest(reqPrevWindow, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(WindowID)
}

// RenameWindow changes the name of the window with the given ID.
func (m *SessionManager) RenameWindow(id WindowID, name string) error {
	return m.sendRequest(reqRenameWindow, &renameWindowPayload{id: id, name: name}).err
}

// CloseWindow closes the window with the given ID.
func (m *SessionManager) CloseWindow(id WindowID) error {
	return m.sendRequest(reqCloseWindow, id).err
}

// ActiveWindowID returns the currently active window ID, or 0 if no windows exist.
func (m *SessionManager) ActiveWindowID() WindowID {
	resp := m.sendRequest(reqActiveWindowID, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(WindowID)
}

// Windows returns a snapshot of all windows in order.
func (m *SessionManager) Windows() []*Window {
	resp := m.sendRequest(reqWindows, nil)
	if resp.value == nil {
		return nil
	}
	return resp.value.([]*Window)
}

// WindowPanes returns a map of WindowID → panes for all windows.
func (m *SessionManager) WindowPanes() map[WindowID][]Pane {
	result := make(map[WindowID][]Pane)
	for _, w := range m.windowMgr.Windows() {
		result[w.ID] = w.paneMgr.Panes()
	}
	return result
}

// SetSynchronizePanes enables or disables synchronized pane input.
func (m *SessionManager) SetSynchronizePanes(v bool) {
	_ = m.sendRequest(reqSetSynchronizePanes, v)
}

// SynchronizePanes reports whether synchronized pane input is enabled.
func (m *SessionManager) SynchronizePanes() bool {
	resp := m.sendRequest(reqSynchronizePanes, nil)
	if resp.value == nil {
		return false
	}
	return resp.value.(bool)
}

// SetMonitorConfig sets the monitoring configuration for the given session.
func (m *SessionManager) SetMonitorConfig(id SessionID, cfg MonitorConfig) error {
	resp := m.sendRequest(reqSetMonitorConfig, monitorConfigPayload{sessionID: id, config: cfg})
	return resp.err
}

// MonitorConfig returns the monitoring configuration for the given session.
func (m *SessionManager) MonitorConfig(id SessionID) (MonitorConfig, error) {
	resp := m.sendRequest(reqMonitorConfig, id)
	if resp.err != nil {
		return MonitorConfig{}, resp.err
	}
	if resp.value == nil {
		return MonitorConfig{}, nil
	}
	return resp.value.(MonitorConfig), nil
}

// VisualBellActive reports whether a visual bell flash is currently displayed
// for the given session. The flash expires after the configured duration.
func (m *SessionManager) VisualBellActive(id SessionID) (bool, error) {
	resp := m.sendRequest(reqVisualBellActive, id)
	if resp.err != nil {
		return false, resp.err
	}
	if resp.value == nil {
		return false, nil
	}
	return resp.value.(bool), nil
}

// CheckSilenceMonitors checks all sessions with silence monitoring enabled
// and emits EventSilence for any that have exceeded their threshold.
// Returns the number of silence events emitted. Intended to be called
// periodically from an external timer.
func (m *SessionManager) CheckSilenceMonitors() int {
	resp := m.sendRequest(reqCheckSilenceMonitors, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(int)
}

func (m *SessionManager) SetRemainOnExit(v bool) {
	_ = m.sendRequest(reqSetRemainOnExit, v)
}

func (m *SessionManager) RemainOnExit() bool {
	resp := m.sendRequest(reqRemainOnExit, nil)
	if resp.value == nil {
		return false
	}
	return resp.value.(bool)
}

func (m *SessionManager) SetPaneRemainOnExit(id PaneID, v bool) error {
	resp := m.sendRequest(reqSetPaneRemainOnExit, paneRemainOnExitPayload{paneID: id, value: v})
	return resp.err
}

func (m *SessionManager) PaneRemainOnExit(id PaneID) (bool, error) {
	resp := m.sendRequest(reqPaneRemainOnExit, id)
	if resp.err != nil {
		return false, resp.err
	}
	if resp.value == nil {
		return false, nil
	}
	return resp.value.(bool), nil
}

func (m *SessionManager) PaneExited(id PaneID) bool {
	resp := m.sendRequest(reqPaneExited, id)
	if resp.value == nil {
		return false
	}
	return resp.value.(bool)
}

func (m *SessionManager) RespawnSession(id SessionID) (SessionID, error) {
	resp := m.sendRequest(reqRespawnSession, id)
	if resp.err != nil {
		return 0, resp.err
	}
	if resp.value == nil {
		return 0, nil
	}
	return resp.value.(SessionID), nil
}

func (m *SessionManager) SwapPanes(a, b PaneID) error {
	resp := m.sendRequest(reqSwapPanes, swapPanesPayload{a: a, b: b})
	return resp.err
}

func (m *SessionManager) ZoomPane(id PaneID) {
	_ = m.sendRequest(reqZoomPane, id)
}

func (m *SessionManager) ZoomedPane() PaneID {
	resp := m.sendRequest(reqZoomedPane, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(PaneID)
}

func (m *SessionManager) SetPipeFile(id SessionID, path string) error {
	resp := m.sendRequest(reqSetPipeFile, pipeFilePayload{sessionID: id, path: path})
	return resp.err
}

func (m *SessionManager) ClearPipe(id SessionID) error {
	resp := m.sendRequest(reqClearPipe, id)
	return resp.err
}

func (m *SessionManager) DisplayMessage(id SessionID, text string, duration time.Duration) error {
	resp := m.sendRequest(reqDisplayMessage, displayMessagePayload{sessionID: id, text: text, duration: duration})
	return resp.err
}

func (m *SessionManager) ActiveMessage(id SessionID) string {
	resp := m.sendRequest(reqActiveMessage, id)
	if resp.err != nil {
		return ""
	}
	return resp.value.(string)
}

func (m *SessionManager) CapturePane(id SessionID, startLine, endLine int) string {
	resp := m.sendRequest(reqCapturePane, capturePanePayload{sessionID: id, startLine: startLine, endLine: endLine})
	if resp.err != nil {
		return ""
	}
	return resp.value.(string)
}

func (m *SessionManager) CopyPaneToClipboard(id SessionID) string {
	text := m.CapturePane(id, 0, -1)
	if text == "" {
		return ""
	}
	return encodeOSC52(text)
}

func encodeOSC52(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	return fmt.Sprintf("\x1b]52;c;%s\x1b\\", encoded)
}

func (m *SessionManager) CopySelection(id SessionID) string {
	resp := m.sendRequest(reqCopySelection, id)
	if resp.err != nil {
		return ""
	}
	if resp.value == nil {
		return ""
	}
	return resp.value.(string)
}

func (m *SessionManager) LockSession(id SessionID, password string) error {
	resp := m.sendRequest(reqLockSession, lockPayload{sessionID: id, password: password})
	return resp.err
}

func (m *SessionManager) UnlockSession(id SessionID, password string) bool {
	resp := m.sendRequest(reqUnlockSession, lockPayload{sessionID: id, password: password})
	if resp.err != nil {
		return false
	}
	return resp.value.(bool)
}

func (m *SessionManager) IsLocked(id SessionID) bool {
	resp := m.sendRequest(reqIsLocked, id)
	if resp.err != nil {
		return false
	}
	return resp.value.(bool)
}

// BreakPane removes the active pane from the given window and moves it
// into a brand new window. Returns the new WindowID.
func (m *SessionManager) BreakPane(windowID WindowID) (WindowID, error) {
	resp := m.sendRequest(reqBreakPane, &breakPanePayload{windowID: windowID})
	if resp.err != nil {
		return 0, resp.err
	}
	return resp.value.(WindowID), nil
}

// JoinPane moves the active pane from sourceWindowID into targetWindowID.
func (m *SessionManager) JoinPane(sourceWindowID, targetWindowID WindowID) error {
	return m.sendRequest(reqJoinPane, &joinPanePayload{
		sourceWindowID: sourceWindowID,
		targetWindowID: targetWindowID,
	}).err
}

// AddPaneToWindow creates a new pane from a session in the specified window.
func (m *SessionManager) AddPaneToWindow(session InteractiveSession, target SessionTarget, windowID WindowID, direction SplitDirection) (PaneID, error) {
	resp := m.sendRequest(reqAddPaneToWindow, &addPaneToWindowPayload{
		session: session, target: target, windowID: windowID, direction: direction,
	})
	if resp.err != nil {
		return 0, resp.err
	}
	return resp.value.(PaneID), nil
}

func (m *SessionManager) NewChooser(active SessionID) *Chooser {
	sessions := m.Sessions()
	slices.SortFunc(sessions, func(a, b SessionInfo) int {
		return cmp.Compare(a.ID, b.ID)
	})
	items := make([]ChooserItem, 0, len(sessions))
	for i, s := range sessions {
		items = append(items, ChooserItem{
			ID:    s.ID,
			Name:  s.Target.Name,
			Kind:  s.Target.Kind,
			Index: i,
		})
	}
	return NewChooser(items, active)
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
	case reqResizeSession:
		resp = m.handleResizeSession(req.payload.(*resizeSessionPayload))
	case reqSnapshot:
		resp = m.handleSnapshot(req.payload.(SessionID))
	case reqActiveID:
		resp = response{value: m.activeID}
	case reqSessions:
		resp = m.handleSessions()
	case reqClose:
		m.shutdownSessions()
		resp = response{}
	case reqActiveWriter:
		resp = m.handleActiveWriter()
	case reqEnablePassthroughTee:
		resp = m.handleEnablePassthroughTee(req.payload.(*passthroughTeePayload))
	case reqDisablePassthroughTee:
		resp = m.handleDisablePassthroughTee()
	case reqExportState:
		resp = m.handleExportState()
	case reqTermSize:
		resp = response{value: [2]int{m.termRows, m.termCols}}
	case reqRestoreState:
		resp = m.handleRestoreState(req.payload.(*restoreStatePayload))
	case reqScreen:
		resp = m.handleScreen(req.payload.(SessionID))
	case reqNewPane:
		resp = m.handleNewPane(req.payload.(*newPanePayload))
	case reqClosePane:
		resp = m.handleClosePane(req.payload.(PaneID))
	case reqFocusPane:
		resp = m.handleFocusPane(req.payload.(PaneID))
	case reqResizePane:
		resp = m.handleResizePane(req.payload.(*resizePanePayload))
	case reqFocusAt:
		resp = m.handleFocusAt(req.payload.(*focusAtPayload))
	case reqResizePaneAt:
		resp = m.handleResizePaneAt(req.payload.(*resizePaneAtPayload))
	case reqPanes:
		resp = m.handlePanes()
	case reqFocusNextPane:
		resp = m.handleFocusNextPane(req.payload.(NavigationDirection))
	case reqActivePaneID:
		resp = m.handleActivePaneID()
	case reqIsCopyModeActive:
		resp = m.handleIsCopyModeActive(req.payload.(SessionID))
	case reqScrollCopyMode:
		resp = m.handleScrollCopyMode(req.payload.(*scrollCopyModePayload))
	case reqEnterCopyMode:
		resp = m.handleEnterCopyMode(req.payload.(SessionID))
	case reqExitCopyMode:
		resp = m.handleExitCopyMode(req.payload.(SessionID))
	case reqSelectStart:
		resp = m.handleSelectStart(req.payload.(selectPayload))
	case reqSelectEnd:
		resp = m.handleSelectEnd(req.payload.(selectPayload))
	case reqNewWindow:
		resp = m.handleNewWindow(req.payload.(*newWindowPayload))
	case reqNextWindow:
		resp = m.handleNextWindow()
	case reqPrevWindow:
		resp = m.handlePrevWindow()
	case reqRenameWindow:
		resp = m.handleRenameWindow(req.payload.(*renameWindowPayload))
	case reqCloseWindow:
		resp = m.handleCloseWindow(req.payload.(WindowID))
	case reqActiveWindowID:
		resp = m.handleActiveWindowID()
	case reqWindows:
		resp = m.handleWindows()
	case reqSetSynchronizePanes:
		resp = m.handleSetSynchronizePanes(req.payload.(bool))
	case reqSynchronizePanes:
		resp = m.handleSynchronizePanes()
	case reqSetMonitorConfig:
		resp = m.handleSetMonitorConfig(req.payload)
	case reqMonitorConfig:
		resp = m.handleMonitorConfig(req.payload)
	case reqVisualBellActive:
		resp = m.handleVisualBellActive(req.payload)
	case reqCheckSilenceMonitors:
		resp = m.handleCheckSilenceMonitors()
	case reqSetRemainOnExit:
		resp = m.handleSetRemainOnExit(req.payload)
	case reqRemainOnExit:
		resp = m.handleRemainOnExit()
	case reqSetPaneRemainOnExit:
		resp = m.handleSetPaneRemainOnExit(req.payload)
	case reqPaneRemainOnExit:
		resp = m.handlePaneRemainOnExit(req.payload)
	case reqPaneExited:
		resp = m.handlePaneExited(req.payload)
	case reqRespawnSession:
		resp = m.handleRespawnSession(req.payload)
	case reqSwapPanes:
		resp = m.handleSwapPanes(req.payload)
	case reqZoomPane:
		resp = m.handleZoomPane(req.payload)
	case reqZoomedPane:
		resp = m.handleZoomedPane()
	case reqSetPipeFile:
		resp = m.handleSetPipeFile(req.payload)
	case reqClearPipe:
		resp = m.handleClearPipe(req.payload)
	case reqDisplayMessage:
		resp = m.handleDisplayMessage(req.payload)
	case reqActiveMessage:
		resp = m.handleActiveMessage(req.payload)
	case reqCapturePane:
		resp = m.handleCapturePane(req.payload)
	case reqCopySelection:
		resp = m.handleCopySelection(req.payload)
	case reqLockSession:
		resp = m.handleLockSession(req.payload)
	case reqUnlockSession:
		resp = m.handleUnlockSession(req.payload)
	case reqIsLocked:
		resp = m.handleIsLocked(req.payload)
	case reqBreakPane:
		resp = m.handleBreakPane(req.payload)
	case reqJoinPane:
		resp = m.handleJoinPane(req.payload)
	case reqAddPaneToWindow:
		resp = m.handleAddPaneToWindow(req.payload)
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
		if mon, ok := m.monitors[id]; ok && mon.Config.Bell {
			mon.VisualBell = VisualBellState{
				Active:    true,
				StartedAt: time.Now(),
			}
		}
	}

	ms := &managedSession{
		session:      p.session,
		vterm:        v,
		state:        SessionCreated,
		target:       p.target,
		lastActive:   time.Now(),
		remainOnExit: m.paneMgr.RemainOnExit(),
	}

	// Wire DA/DSR response callback so VT responses are routed back to
	// the child process via the session's stdin. This enables programs
	// like vim, htop, and less to query terminal capabilities and
	// cursor position. Must be after ms creation since the closure
	// captures ms.
	v.ResponseWriter = func(data []byte) {
		if ms.session != nil {
			if _, err := ms.session.Write(data); err != nil {
				slog.Debug("response write failed", "sessionID", id, "error", err)
			}
		}
	}

	// Wire OSC handler to emit typed events on the EventBus.
	// OSC 0/2 → EventTitle, OSC 7 → EventWorkingDirectory,
	// OSC 52 → EventClipboard.
	v.OSCHandler = func(code int, data string) {
		switch code {
		case 0, 2:
			m.eventBus.emitData(EventTitle, id, data)
		case 7:
			m.eventBus.emitData(EventWorkingDirectory, id, data)
		case 52:
			m.eventBus.emitData(EventClipboard, id, data)
		}
	}

	// Wire DCS handler. During passthrough, DCS sequences are
	// forwarded to the real terminal via the passthrough output
	// writer. In embedded mode, DCS is silently discarded.
	// Future: handle DECRQSS to respond with current SGR/DECSTBM state.
	v.DCSHandler = func(data []byte) {
		// DCS passthrough forwarding is handled by the
		// passthrough tee mechanism — DCS data is included
		// in the raw output stream forwarded to the terminal.
		// For embedded mode, DCS is discarded for now.
	}

	snap := NewScreenSnapshot(m.snapshotGen, &vt.Screen{}, m.termRows, m.termCols, time.Now())
	// Initial snapshot has cursor at origin (0,0).
	ms.snapshot.Store(snap)
	m.sessions[id] = ms
	m.monitors[id] = NewMonitorState(MonitorConfig{})

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

	// Close the underlying session — log errors rather than silently discarding.
	if err := ms.session.Close(); err != nil {
		slog.Warn("session close failed during unregister", "sessionID", id, "error", err)
	}
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

	if m.paneMgr.Synchronize() {
		var firstErr error
		for _, ms := range m.sessions {
			if ms.state != SessionRunning && ms.state != SessionCreated {
				continue
			}
			if _, err := ms.session.Write(data); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return response{err: firstErr}
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
	m.paneMgr.setSize(p.cols, p.rows)
	for id, ms := range m.sessions {
		if ms.state == SessionClosed {
			continue
		}
		ms.vterm.Resize(p.rows, p.cols)
		if err := ms.session.Resize(p.rows, p.cols); err != nil {
			slog.Warn("session resize failed", "sessionID", id, "rows", p.rows, "cols", p.cols, "error", err)
		}
	}
	m.eventBus.emitData(EventResize, 0, [2]int{p.rows, p.cols})
	return response{}
}

// handleResizeSession resizes a single session's VTerm and PTY.
func (m *SessionManager) handleResizeSession(p *resizeSessionPayload) response {
	ms, ok := m.sessions[p.id]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.id)}
	}
	if ms.state == SessionClosed {
		return response{err: fmt.Errorf("%w: session %d is closed", ErrInvalidTransition, p.id)}
	}
	ms.vterm.Resize(p.rows, p.cols)
	if err := ms.session.Resize(p.rows, p.cols); err != nil {
		slog.Warn("session resize failed", "sessionID", p.id, "rows", p.rows, "cols", p.cols, "error", err)
	}
	m.eventBus.emitData(EventResize, p.id, [2]int{p.rows, p.cols})
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

// handleScreen returns a deep copy of the session's VTerm screen with
// Cells and Attr preserved. Returns nil if the session does not exist.
func (m *SessionManager) handleScreen(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{}
	}
	return response{value: ms.vterm.ActiveScreen()}
}

func (m *SessionManager) handleNewPane(p *newPanePayload) response {
	regResp := m.handleRegister(&registerPayload{session: p.session, target: p.target})
	if regResp.err != nil {
		return regResp
	}
	sessionID := regResp.value.(SessionID)

	paneID, err := m.paneMgr.Create(sessionID, p.direction)
	if err != nil {
		m.handleUnregister(sessionID)
		return response{err: err}
	}

	ms, ok := m.sessions[sessionID]
	if ok {
		m.paneMgr.setVTerm(paneID, ms.vterm)
	}

	m.activeID = sessionID

	return response{value: paneID}
}

func (m *SessionManager) handleClosePane(id PaneID) response {
	sessionID := m.paneMgr.removeSessionID(id)

	if err := m.paneMgr.Remove(id); err != nil {
		return response{err: err}
	}

	if sessionID != 0 {
		unregResp := m.handleUnregister(sessionID)
		if unregResp.err != nil {
			slog.Warn("session unregister failed during pane close", "sessionID", sessionID, "error", unregResp.err)
		}
	}

	if activeSessionID := m.paneMgr.activeSessionID(); activeSessionID != 0 {
		m.activeID = activeSessionID
	} else {
		m.activeID = 0
	}

	return response{}
}

func (m *SessionManager) handleFocusPane(id PaneID) response {
	if err := m.paneMgr.Focus(id); err != nil {
		return response{err: err}
	}

	sessionID := m.paneMgr.activeSessionID()
	if sessionID != 0 {
		m.activeID = sessionID
		ms, ok := m.sessions[sessionID]
		if ok {
			ms.lastActive = time.Now()
		}
	}

	return response{}
}

func (m *SessionManager) handleResizePane(p *resizePanePayload) response {
	if err := m.paneMgr.Resize(p.id, p.ratio); err != nil {
		return response{err: err}
	}

	panes := m.paneMgr.Panes()
	for _, pane := range panes {
		ms, ok := m.sessions[pane.SessionID]
		if !ok || ms.state == SessionClosed {
			continue
		}
		ms.vterm.Resize(pane.Geometry.Rows, pane.Geometry.Cols)
		if err := ms.session.Resize(pane.Geometry.Rows, pane.Geometry.Cols); err != nil {
			slog.Warn("session resize failed during pane resize", "sessionID", pane.SessionID, "error", err)
		}
	}

	return response{}
}

func (m *SessionManager) handleFocusAt(p *focusAtPayload) response {
	id, err := m.paneMgr.FocusAt(p.row, p.col)
	if err != nil {
		return response{err: err}
	}

	sessionID := m.paneMgr.activeSessionID()
	if sessionID != 0 {
		m.activeID = sessionID
		if ms, ok := m.sessions[sessionID]; ok {
			ms.lastActive = time.Now()
		}
	}

	return response{value: id}
}

func (m *SessionManager) handleResizePaneAt(p *resizePaneAtPayload) response {
	if err := m.paneMgr.ResizePaneAt(p.row, p.col, p.ratio); err != nil {
		return response{err: err}
	}

	panes := m.paneMgr.Panes()
	for _, pane := range panes {
		ms, ok := m.sessions[pane.SessionID]
		if !ok || ms.state == SessionClosed {
			continue
		}
		ms.vterm.Resize(pane.Geometry.Rows, pane.Geometry.Cols)
		if err := ms.session.Resize(pane.Geometry.Rows, pane.Geometry.Cols); err != nil {
			slog.Warn("session resize failed during pane resize at coordinate", "sessionID", pane.SessionID, "error", err)
		}
	}

	return response{}
}

func (m *SessionManager) handlePanes() response {
	return response{value: m.paneMgr.Panes()}
}

func (m *SessionManager) handleFocusNextPane(direction NavigationDirection) response {
	nextID := m.paneMgr.FocusNext(direction)
	return response{value: nextID}
}

func (m *SessionManager) handleActivePaneID() response {
	return response{value: m.paneMgr.ActivePaneID()}
}

func (m *SessionManager) handleIsCopyModeActive(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{value: false}
	}
	return response{value: ms.vterm.InCopyMode()}
}

func (m *SessionManager) handleScrollCopyMode(p *scrollCopyModePayload) response {
	ms, ok := m.sessions[p.id]
	if !ok {
		return response{value: false}
	}
	return response{value: ms.vterm.ScrollCopyMode(p.delta)}
}

func (m *SessionManager) handleEnterCopyMode(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	ms.vterm.EnterCopyMode()
	return response{}
}

func (m *SessionManager) handleExitCopyMode(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	ms.vterm.ExitCopyMode()
	return response{}
}

func (m *SessionManager) handleSelectStart(p selectPayload) response {
	ms, ok := m.sessions[p.sessionID]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	ms.vterm.SelectStart(p.row, p.col)
	return response{}
}

func (m *SessionManager) handleSelectEnd(p selectPayload) response {
	ms, ok := m.sessions[p.sessionID]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	ms.vterm.SelectEnd(p.row, p.col)
	return response{}
}

func (m *SessionManager) handleNewWindow(p *newWindowPayload) response {
	id := m.windowMgr.NewWindow(p.name)
	return response{value: id}
}

func (m *SessionManager) handleNextWindow() response {
	id := m.windowMgr.NextWindow()
	return response{value: id}
}

func (m *SessionManager) handlePrevWindow() response {
	id := m.windowMgr.PrevWindow()
	return response{value: id}
}

func (m *SessionManager) handleRenameWindow(p *renameWindowPayload) response {
	if err := m.windowMgr.RenameWindow(p.id, p.name); err != nil {
		return response{err: err}
	}
	return response{}
}

func (m *SessionManager) handleCloseWindow(id WindowID) response {
	if err := m.windowMgr.CloseWindow(id); err != nil {
		return response{err: err}
	}
	return response{}
}

func (m *SessionManager) handleActiveWindowID() response {
	return response{value: m.windowMgr.ActiveWindowID()}
}

func (m *SessionManager) handleWindows() response {
	return response{value: m.windowMgr.Windows()}
}

func (m *SessionManager) handleSetSynchronizePanes(v bool) response {
	m.paneMgr.SetSynchronize(v)
	return response{}
}

func (m *SessionManager) handleSynchronizePanes() response {
	return response{value: m.paneMgr.Synchronize()}
}

func (m *SessionManager) handleSetMonitorConfig(payload any) response {
	p, ok := payload.(monitorConfigPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	if _, exists := m.sessions[p.sessionID]; !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	mon, exists := m.monitors[p.sessionID]
	if !exists {
		mon = NewMonitorState(p.config)
		m.monitors[p.sessionID] = mon
	} else {
		mon.Config = p.config
	}
	return response{}
}

func (m *SessionManager) handleMonitorConfig(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	mon, exists := m.monitors[id]
	if !exists {
		return response{value: MonitorConfig{}}
	}
	return response{value: mon.Config}
}

func (m *SessionManager) handleVisualBellActive(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	mon, exists := m.monitors[id]
	if !exists || !mon.VisualBell.Active {
		return response{value: false}
	}
	if time.Since(mon.VisualBell.StartedAt) >= m.visualBellDuration {
		mon.VisualBell.Active = false
		return response{value: false}
	}
	return response{value: true}
}

func (m *SessionManager) handleCheckSilenceMonitors() response {
	now := time.Now()
	count := 0
	for id, mon := range m.monitors {
		if !mon.Config.Silence || mon.Config.SilenceThreshold <= 0 || mon.SilenceFired {
			continue
		}
		if ms, exists := m.sessions[id]; exists && ms.state != SessionRunning && ms.state != SessionCreated {
			continue
		}
		if now.Sub(mon.LastOutputAt) >= mon.Config.SilenceThreshold {
			mon.SilenceFired = true
			m.eventBus.emit(EventSilence, id)
			count++
		}
	}
	return response{value: count}
}

func (m *SessionManager) handleSetRemainOnExit(payload any) response {
	v, ok := payload.(bool)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	m.paneMgr.SetRemainOnExit(v)
	return response{}
}

func (m *SessionManager) handleRemainOnExit() response {
	return response{value: m.paneMgr.RemainOnExit()}
}

func (m *SessionManager) handleSetPaneRemainOnExit(payload any) response {
	p, ok := payload.(paneRemainOnExitPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	return response{err: m.paneMgr.SetPaneRemainOnExit(p.paneID, p.value)}
}

func (m *SessionManager) handlePaneRemainOnExit(payload any) response {
	id, ok := payload.(PaneID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	v, err := m.paneMgr.PaneRemainOnExit(id)
	if err != nil {
		return response{err: err}
	}
	return response{value: v}
}

func (m *SessionManager) handlePaneExited(payload any) response {
	id, ok := payload.(PaneID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	return response{value: m.paneMgr.PaneExited(id)}
}

func (m *SessionManager) handleRespawnSession(payload any) response {
	oldID, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[oldID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, oldID)}
	}
	if ms.state != SessionExited {
		return response{err: fmt.Errorf("termmux: session %d has not exited (state=%v)", oldID, ms.state)}
	}

	_ = ms.session.Close()

	newSession := NewCaptureSession(CaptureConfig{})
	newID := m.nextID
	m.nextID++

	v := vt.NewVTerm(m.termRows, m.termCols)
	v.BellFn = func() {
		m.eventBus.emit(EventBell, newID)
		if mon, ok := m.monitors[newID]; ok && mon.Config.Bell {
			mon.VisualBell = VisualBellState{Active: true, StartedAt: time.Now()}
		}
	}

	newMS := &managedSession{
		session:      newSession,
		vterm:        v,
		state:        SessionCreated,
		target:       ms.target,
		lastActive:   time.Now(),
		remainOnExit: m.paneMgr.RemainOnExit(),
	}

	v.ResponseWriter = func(data []byte) {
		if newMS.session != nil {
			if _, err := newMS.session.Write(data); err != nil {
				slog.Debug("response write failed", "sessionID", newID, "error", err)
			}
		}
	}

	v.OSCHandler = func(code int, data string) {
		switch code {
		case 0, 2:
			m.eventBus.emitData(EventTitle, newID, data)
		case 7:
			m.eventBus.emitData(EventWorkingDirectory, newID, data)
		case 52:
			m.eventBus.emitData(EventClipboard, newID, data)
		}
	}

	v.DCSHandler = func(data []byte) {}

	snap := NewScreenSnapshot(m.snapshotGen, &vt.Screen{}, m.termRows, m.termCols, time.Now())
	newMS.snapshot.Store(snap)
	m.sessions[newID] = newMS
	m.monitors[newID] = NewMonitorState(MonitorConfig{})

	m.snapshotGen++

	if m.activeID == oldID {
		m.activeID = newID
	}

	delete(m.sessions, oldID)

	m.eventBus.emit(EventSessionRegistered, newID)
	m.startReaderGoroutine(newID, newSession)

	return response{value: newID}
}

func (m *SessionManager) handleSwapPanes(payload any) response {
	p, ok := payload.(swapPanesPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	if !m.paneMgr.engine.Swap(p.a, p.b) {
		return response{err: fmt.Errorf("termmux: swap failed: one or both panes not found")}
	}
	return response{}
}

func (m *SessionManager) handleZoomPane(payload any) response {
	id, ok := payload.(PaneID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	if m.paneMgr.engine.ZoomedPane() == id {
		m.paneMgr.engine.Unzoom()
	} else {
		m.paneMgr.engine.Zoom(id)
	}
	return response{}
}

func (m *SessionManager) handleZoomedPane() response {
	return response{value: m.paneMgr.engine.ZoomedPane()}
}

func (m *SessionManager) handleSetPipeFile(payload any) response {
	p, ok := payload.(pipeFilePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[p.sessionID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	f, err := os.OpenFile(p.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return response{err: fmt.Errorf("termmux: pipe-pane open: %w", err)}
	}
	var w io.Writer = f
	ms.pipeWriter.Store(&w)
	return response{}
}

func (m *SessionManager) handleClearPipe(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[id]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	if old := ms.pipeWriter.Swap(nil); old != nil {
		if c, ok := (*old).(io.Closer); ok {
			_ = c.Close()
		}
	}
	return response{}
}

func (m *SessionManager) handleDisplayMessage(payload any) response {
	p, ok := payload.(displayMessagePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[p.sessionID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	dur := p.duration
	if dur <= 0 {
		dur = 3 * time.Second
	}
	ms.message.Store(&displayMessage{
		text:      p.text,
		expiresAt: time.Now().Add(dur),
	})
	return response{}
}

func (m *SessionManager) handleActiveMessage(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[id]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	msg := ms.message.Load()
	if msg == nil || time.Now().After(msg.expiresAt) {
		if msg != nil {
			ms.message.Store(nil)
		}
		return response{value: ""}
	}
	return response{value: msg.text}
}

func (m *SessionManager) handleCapturePane(payload any) response {
	p, ok := payload.(capturePanePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[p.sessionID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	snap := ms.snapshot.Load()
	if snap == nil {
		return response{value: ""}
	}
	text := snap.GetPlainText()
	if text == "" {
		return response{value: ""}
	}
	lines := strings.Split(text, "\n")
	start := max(p.startLine, 0)
	end := p.endLine
	if end < 0 || end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return response{value: ""}
	}
	return response{value: strings.Join(lines[start:end], "\n")}
}

func (m *SessionManager) handleCopySelection(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[id]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	text := ms.vterm.SelectedText()
	if text == "" {
		return response{value: ""}
	}
	return response{value: encodeOSC52(text)}
}

func (m *SessionManager) handleLockSession(payload any) response {
	p, ok := payload.(lockPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[p.sessionID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	if err := ms.lock.Lock(p.password); err != nil {
		return response{err: err}
	}
	return response{}
}

func (m *SessionManager) handleUnlockSession(payload any) response {
	p, ok := payload.(lockPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[p.sessionID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}
	return response{value: ms.lock.Unlock(p.password)}
}

func (m *SessionManager) handleIsLocked(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[id]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	return response{value: ms.lock.IsLocked()}
}

func (m *SessionManager) handleBreakPane(payload any) response {
	p, ok := payload.(*breakPanePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	// If windowID is 0, check the SessionManager's own paneMgr first.
	if p.windowID == 0 {
		if m.paneMgr.Binding(m.paneMgr.ActivePaneID()) != nil {
			newID, err := m.breakPaneFromSessionMgr()
			return response{value: newID, err: err}
		}
		return response{err: fmt.Errorf("%w: no active pane", ErrPaneNotFound)}
	}
	newID, err := m.windowMgr.BreakPane(p.windowID)
	return response{value: newID, err: err}
}

// breakPaneFromSessionMgr breaks the active pane from the SessionManager's
// own paneMgr into a new window. Used when panes haven't been assigned
// to any window yet.
func (m *SessionManager) breakPaneFromSessionMgr() (WindowID, error) {
	activePaneID := m.paneMgr.ActivePaneID()
	if activePaneID == 0 {
		return 0, fmt.Errorf("%w: no active pane", ErrPaneNotFound)
	}

	// Get the binding from SessionManager's paneMgr.
	binding := m.paneMgr.Binding(activePaneID)
	if binding == nil {
		return 0, fmt.Errorf("%w: %d", ErrPaneNotFound, activePaneID)
	}

	// Create a new window.
	newWindowID := m.windowMgr.NewWindow("")
	newWindow := m.windowMgr.Window(newWindowID)
	if newWindow == nil {
		return 0, fmt.Errorf("termmux: failed to create new window")
	}

	// Remove the pane from SessionManager's paneMgr.
	if err := m.paneMgr.Remove(activePaneID); err != nil {
		return 0, fmt.Errorf("%w: %d", ErrPaneNotFound, activePaneID)
	}

	// Add the pane to the new window.
	newPaneID, err := newWindow.paneMgr.Create(binding.SessionID, SplitRight)
	if err != nil {
		return 0, fmt.Errorf("termmux: failed to create pane in new window")
	}
	newWindow.paneMgr.setVTerm(newPaneID, binding.VTerm)
	newWindow.paneMgr.panes[newPaneID].Title = binding.Title
	newWindow.paneMgr.panes[newPaneID].LastActive = binding.LastActive

	return newWindowID, nil
}

func (m *SessionManager) handleJoinPane(payload any) response {
	p, ok := payload.(*joinPanePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	// Source windowID of 0 means pane is in SessionManager's own paneMgr.
	if p.sourceWindowID == 0 {
		if m.paneMgr.Binding(m.paneMgr.ActivePaneID()) == nil {
			return response{err: fmt.Errorf("%w: no active pane to join", ErrPaneNotFound)}
		}
		if err := m.joinPaneFromSessionMgr(p.targetWindowID); err != nil {
			return response{err: err}
		}
		return response{}
	}
	return response{err: m.windowMgr.JoinPane(p.sourceWindowID, p.targetWindowID)}
}

// joinPaneFromSessionMgr joins the active pane from the SessionManager's
// own paneMgr into the target window.
func (m *SessionManager) joinPaneFromSessionMgr(targetWindowID WindowID) error {
	activePaneID := m.paneMgr.ActivePaneID()
	if activePaneID == 0 {
		return fmt.Errorf("%w: no active pane", ErrPaneNotFound)
	}
	binding := m.paneMgr.Binding(activePaneID)
	if binding == nil {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, activePaneID)
	}
	targetWindow := m.windowMgr.Window(targetWindowID)
	if targetWindow == nil {
		return fmt.Errorf("%w: %d", ErrWindowNotFound, targetWindowID)
	}
	// Remove from SessionManager's paneMgr.
	if err := m.paneMgr.Remove(activePaneID); err != nil {
		return fmt.Errorf("%w: %d", ErrPaneNotFound, activePaneID)
	}
	// Add to target window.
	newPaneID, err := targetWindow.paneMgr.Create(binding.SessionID, SplitRight)
	if err != nil {
		return fmt.Errorf("termmux: failed to create pane in target window")
	}
	targetWindow.paneMgr.setVTerm(newPaneID, binding.VTerm)
	targetWindow.paneMgr.panes[newPaneID].Title = binding.Title
	targetWindow.paneMgr.panes[newPaneID].LastActive = binding.LastActive
	return nil
}

func (m *SessionManager) handleAddPaneToWindow(payload any) response {
	p, ok := payload.(*addPaneToWindowPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	regResp := m.handleRegister(&registerPayload{session: p.session, target: p.target})
	if regResp.err != nil {
		return regResp
	}
	sessionID := regResp.value.(SessionID)

	w := m.windowMgr.Window(p.windowID)
	if w == nil {
		m.handleUnregister(sessionID)
		return response{err: fmt.Errorf("%w: %d", ErrWindowNotFound, p.windowID)}
	}

	paneID, err := w.paneMgr.Create(sessionID, p.direction)
	if err != nil {
		m.handleUnregister(sessionID)
		return response{err: err}
	}

	ms, ok := m.sessions[sessionID]
	if ok {
		w.paneMgr.setVTerm(paneID, ms.vterm)
	}

	return response{value: paneID}
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
		if ms.vterm.SynchronizedOutput() {
			m.eventBus.emitData(EventSessionOutput, so.id, nil)
		}
		if ms.state.validTransition(SessionExited) {
			ms.state = SessionExited
			m.eventBus.emit(EventSessionExited, so.id)
			if ms.remainOnExit {
				return
			}
			if err := ms.session.Close(); err != nil {
				slog.Warn("session close failed on exit", "sessionID", so.id, "error", err)
			}
			ms.state = SessionClosed
			if m.activeID == so.id {
				m.activeID = 0
			}
			delete(m.sessions, so.id)
			m.eventBus.emit(EventSessionClosed, so.id)
		} else if ms.state == SessionCreated {
			if err := ms.session.Close(); err != nil {
				slog.Warn("session close failed during immediate exit", "sessionID", so.id, "error", err)
			}
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
		if _, err := (*w).Write(so.data); err != nil {
			slog.Warn("passthrough tee write failed", "sessionID", so.id, "error", err)
		}
	}

	if w := ms.pipeWriter.Load(); w != nil {
		if _, err := (*w).Write(so.data); err != nil {
			slog.Warn("pipe-pane write failed", "sessionID", so.id, "error", err)
		}
	}

	// Update VTerm with the raw output.
	if _, err := ms.vterm.Write(so.data); err != nil {
		slog.Debug("vterm write failed", "sessionID", so.id, "error", err)
	}

	// Publish a new immutable snapshot.
	m.snapshotGen++
	scr := ms.vterm.ActiveScreen()
	snap := &ScreenSnapshot{
		Gen:                m.snapshotGen,
		screen:             scr,
		Rows:               m.termRows,
		Cols:               m.termCols,
		CursorRow:          scr.CurRow,
		CursorCol:          scr.CurCol,
		MouseTracking:      int(scr.MouseTracking),
		MouseSGR:           scr.MouseSGR,
		InsertMode:         scr.InsertMode,
		BracketedPaste:     scr.BracketedPaste,
		ApplicationCursor:  scr.ApplicationCursor,
		KeypadApplication:  scr.KeypadApplication,
		CursorShape:        scr.CursorShape,
		FocusReporting:     scr.FocusReporting,
		SynchronizedOutput: scr.SynchronizedOutput,
		AutoWrap:           scr.AutoWrap,
		LineFeedNewLine:    scr.LineFeedNewLine,
		Timestamp:          time.Now(),
	}
	ms.snapshot.Store(snap)

	// When synchronized output mode is active, skip event emission to
	// reduce flicker. The snapshot is still stored (so Snapshot() returns
	// current state), but consumers won't be notified until the mode is
	// disabled. The next output chunk that turns off synchronized output
	// will naturally emit the event since SynchronizedOutput() returns
	// false after the VTerm processes the DECRST ?2026l.
	if !scr.SynchronizedOutput {
		m.eventBus.emitData(EventSessionOutput, so.id, so.data)
	}

	// Monitoring: update LastOutputAt and check activity/silence.
	if mon, ok := m.monitors[so.id]; ok {
		now := time.Now()
		wasIdle := now.Sub(mon.LastOutputAt)
		mon.LastOutputAt = now
		mon.SilenceFired = false

		// Activity: background pane produced output after being idle.
		if mon.Config.Activity && !mon.ActivityFired && so.id != m.activeID {
			if mon.Config.ActivityThreshold <= 0 || wasIdle >= mon.Config.ActivityThreshold {
				mon.ActivityFired = true
				m.eventBus.emit(EventActivity, so.id)
			}
		}
	}
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

// handleActiveWriter returns the active session's InteractiveSession as an
// io.Writer. Used by Passthrough to write directly to the PTY without routing
// through the worker (low-latency path).
func (m *SessionManager) handleActiveWriter() response {
	if m.activeID == 0 {
		return response{err: ErrSessionNotFound}
	}
	ms, ok := m.sessions[m.activeID]
	if !ok {
		return response{err: fmt.Errorf("%w: active session %d disappeared", ErrSessionNotFound, m.activeID)}
	}
	if ms.state != SessionRunning && ms.state != SessionCreated {
		return response{err: fmt.Errorf("%w: session %d in state %s", ErrInvalidTransition, m.activeID, ms.state)}
	}
	return response{value: io.Writer(ms.session)}
}

// handleEnablePassthroughTee sets a tee writer on the active session so that
// raw output chunks are forwarded to the provided writer in addition to the
// VTerm. Used during passthrough for low-latency stdout forwarding.
//
// The payload is a *passthroughTeePayload containing the writer and the
// target session ID. This ensures the tee is set on the specific session
// the caller is passthroughing, not whatever happens to be active later.
//
// Returns ErrPassthroughActive if passthrough is already in progress.
func (m *SessionManager) handleEnablePassthroughTee(p *passthroughTeePayload) response {
	if m.passthroughSessionID != 0 {
		return response{err: ErrPassthroughActive}
	}
	ms, ok := m.sessions[p.id]
	if !ok {
		return response{err: fmt.Errorf("%w: session %d", ErrSessionNotFound, p.id)}
	}
	ms.passthroughWriter.Store(&p.w)
	m.passthroughSessionID = p.id
	return response{}
}

// handleDisablePassthroughTee clears the tee writer on the session that
// was previously enabled. Uses passthroughSessionID to ensure it clears
// the correct session regardless of whether activeID has changed.
//
// If the session was removed between enable and disable, the handler
// silently succeeds: the session is already gone and clearing its tee
// is a no-op. Resetting passthroughSessionID still correctly ends the
// passthrough guard.
func (m *SessionManager) handleDisablePassthroughTee() response {
	if m.passthroughSessionID == 0 {
		return response{} // no-op — already disabled
	}
	ms, ok := m.sessions[m.passthroughSessionID]
	if ok {
		ms.passthroughWriter.Store(nil)
	}
	m.passthroughSessionID = 0
	return response{}
}

// activeWriter returns an io.Writer pointing to the active session's PTY
// input. Used by Passthrough for direct stdin forwarding.
func (m *SessionManager) activeWriter() (io.Writer, error) {
	resp := m.sendRequest(reqActiveWriter, nil)
	if resp.err != nil {
		return nil, resp.err
	}
	return resp.value.(io.Writer), nil
}

// passthroughTeePayload carries the writer and target session for tee
// enable/disable operations.
type passthroughTeePayload struct {
	w  io.Writer
	id SessionID
}

// enablePassthroughTee asks the worker to start teeing raw output from
// the given session to w (in addition to VTerm processing).
func (m *SessionManager) enablePassthroughTee(id SessionID, w io.Writer) error {
	return m.sendRequest(reqEnablePassthroughTee, &passthroughTeePayload{w: w, id: id}).err
}

// disablePassthroughTee asks the worker to stop teeing raw output.
func (m *SessionManager) disablePassthroughTee() error {
	return m.sendRequest(reqDisablePassthroughTee, nil).err
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
			if err := ms.session.Close(); err != nil {
				slog.Warn("session close failed during shutdown", "sessionID", id, "error", err)
			}
			ms.state = SessionClosed
			m.eventBus.emit(EventSessionClosed, id)
		}
		delete(m.sessions, id)
	}
	m.activeID = 0
	m.paneMgr.Close()
}

// sortSessionIDs sorts a slice of SessionIDs in descending order.
// Uses cmp.Compare to avoid integer overflow when subtracting uint64
// values (SessionID is uint64; subtraction overflow produces incorrect
// ordering when the difference exceeds the maximum signed int value).
func sortSessionIDs(ids []SessionID) {
	slices.SortFunc(ids, func(a, b SessionID) int { return cmp.Compare(b, a) })
}
