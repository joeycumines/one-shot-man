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
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

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

	// Locked reports whether the session was locked when this snapshot
	// was published.
	Locked bool

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

	// Message is the active display-message overlay text for this session,
	// or empty when no message is queued or the front message has expired.
	Message string

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

// Clone returns a shallow copy of s with fresh sync.Once fields. The returned
// snapshot shares the underlying screen and may have its metadata mutated
// before publication.
func (s *ScreenSnapshot) Clone() *ScreenSnapshot {
	return &ScreenSnapshot{
		Gen:                s.Gen,
		screen:             s.screen,
		Rows:               s.Rows,
		Cols:               s.Cols,
		CursorRow:          s.CursorRow,
		CursorCol:          s.CursorCol,
		MouseTracking:      s.MouseTracking,
		MouseSGR:           s.MouseSGR,
		InsertMode:         s.InsertMode,
		BracketedPaste:     s.BracketedPaste,
		ApplicationCursor:  s.ApplicationCursor,
		KeypadApplication:  s.KeypadApplication,
		CursorShape:        s.CursorShape,
		FocusReporting:     s.FocusReporting,
		SynchronizedOutput: s.SynchronizedOutput,
		AutoWrap:           s.AutoWrap,
		LineFeedNewLine:    s.LineFeedNewLine,
		Locked:             s.Locked,
		Message:            s.Message,
		Timestamp:          s.Timestamp,
	}
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

	reqSendKeys

	// reqResetActivity resets the activity-fired flag for a session.
	// Payload: SessionID. Reply value: none.
	reqResetActivity

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

	// reqResizePaneDelta asks the worker to resize a pane by a directional
	// cell delta. Payload: *resizePaneDeltaPayload. Reply value: nil.
	reqResizePaneDelta

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

	// reqHandleCopyModeKey asks the worker to dispatch a copy-mode key.
	// Payload: handleCopyModeKeyPayload. Reply value: none.
	reqHandleCopyModeKey

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

	// reqMoveWindow asks the worker to move a window to a target index.
	// Payload: moveWindowPayload. Reply value: none.
	reqMoveWindow

	// reqSwapWindows asks the worker to swap two windows' positions.
	// Payload: swapWindowsPayload. Reply value: none.
	reqSwapWindows

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

	// reqSetPipe configures a pipe target (file or command) for pane output.
	// Payload: setPipePayload. Reply value: none.
	reqSetPipe

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

	// reqUnlockPrompt returns the current lock-prompt state for a session.
	// Payload: SessionID. Reply value: unlockPromptResponse.
	reqUnlockPrompt

	// reqBreakPane breaks the active pane out of its window into a new one.
	// Payload: WindowID. Reply value: WindowID.
	reqBreakPane

	// reqJoinPane joins the active pane from one window into another.
	// Payload: joinPanePayload. Reply value: none.
	reqJoinPane

	// reqAddPaneToWindow adds a session as a pane to a specific window.
	// Payload: *addPaneToWindowPayload. Reply value: PaneID.
	reqAddPaneToWindow

	// reqSplitPane adds a session as a pane split from an existing pane.
	// Payload: *splitPanePayload. Reply value: PaneID.
	reqSplitPane

	// reqSetLayoutMode asks the worker to set the layout mode for a window.
	// Payload: *setLayoutModePayload. Reply value: none.
	reqSetLayoutMode

	// reqLayoutMode asks the worker for the layout mode of a window.
	// Payload: WindowID. Reply value: string.
	reqLayoutMode
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

// setPipePayload carries the session ID and pipe configuration.
type setPipePayload struct {
	sessionID SessionID
	config    PipeConfig
}

// displayMessage is a transient message shown as an overlay.
type displayMessage struct {
	text      string
	expiresAt time.Time
}

// maxDisplayMessages caps the per-session display-message queue to prevent
// unbounded growth if callers enqueue faster than messages expire.
const maxDisplayMessages = 32

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

// handleCopyModeKeyPayload carries the session ID and key to dispatch.
type handleCopyModeKeyPayload struct {
	sessionID SessionID
	key       string
}

// lockPayload carries the session ID and plaintext password.
type lockPayload struct {
	sessionID SessionID
	password  string
}

// unlockPromptResponse carries the current lock prompt state for a session.
type unlockPromptResponse struct {
	active    bool
	maskLen   int
	message   string
	expiresAt time.Time
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

// resizePaneDeltaPayload carries the pane ID, direction, and cell delta for a
// reqResizePaneDelta request.
type resizePaneDeltaPayload struct {
	id        PaneID
	direction string
	delta     int
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

// moveWindowPayload carries the window ID and target index.
type moveWindowPayload struct {
	id          WindowID
	targetIndex int
}

// swapWindowsPayload carries two window IDs to swap.
type swapWindowsPayload struct {
	a, b WindowID
}

// sendKeysPayload carries a session ID and the named keys to inject.
type sendKeysPayload struct {
	id   SessionID
	keys []string
}

// breakPanePayload carries the pane ID to break into a new window.
type breakPanePayload struct {
	paneID PaneID
}

// joinPanePayload carries the pane ID to move and the target window ID.
type joinPanePayload struct {
	paneID         PaneID
	targetWindowID WindowID
}

// addPaneToWindowPayload carries arguments for reqAddPaneToWindow.
type addPaneToWindowPayload struct {
	session   InteractiveSession
	target    SessionTarget
	windowID  WindowID
	direction SplitDirection
}

// splitPanePayload carries arguments for reqSplitPane.
type splitPanePayload struct {
	activePane PaneID
	cfg        CaptureConfig
	direction  SplitDirection
}

// setLayoutModePayload carries the window ID and new layout mode.
type setLayoutModePayload struct {
	windowID WindowID
	mode     LayoutMode
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

// PipeConfig configures where a pane's raw PTY output is teed.
// Exactly one of Path or Command should be non-empty. Setting both returns
// an error from SetPipe.
type PipeConfig struct {
	// Path, when non-empty, opens the file for append and writes each
	// raw output chunk to it.
	Path string

	// Command, when non-empty, spawns the external command and writes
	// each raw output chunk to its stdin.
	Command string

	// Args are the command-line arguments passed to Command.
	Args []string
}

// pipeProcess wraps a spawned external command and its stdin pipe.
// It implements io.Writer and io.Closer so it can be stored in
// managedSession.pipeWriter and torn down on pane exit or ClearPipe.
type pipeProcess struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	closeOnce sync.Once
	done      chan struct{}
}

// Write forwards raw PTY bytes to the command's stdin. If the pipe has
// been closed, writes are silently dropped.
func (p *pipeProcess) Write(data []byte) (int, error) {
	if p.stdin == nil {
		return len(data), nil
	}
	return p.stdin.Write(data)
}

// Close closes the command's stdin and waits for the process to exit.
// If it does not terminate in a short grace period it is killed.
// It is safe to call Close multiple times.
func (p *pipeProcess) Close() error {
	p.closeOnce.Do(func() {
		if p.stdin != nil {
			_ = p.stdin.Close()
		}
		if p.cmd != nil && p.cmd.Process != nil {
			select {
			case <-p.done:
			case <-time.After(200 * time.Millisecond):
				_ = p.cmd.Process.Kill()
				<-p.done
			}
		} else {
			close(p.done)
		}
	})
	return nil
}

// reap waits for the process to exit and then closes p.done. It is
// started once when the pipe process is created so that an external
// command which exits on its own is reaped promptly rather than
// remaining a zombie until ClearPipe or session close runs.
func (p *pipeProcess) reap() {
	_ = p.cmd.Wait()
	close(p.done)
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

	captureConfig CaptureConfig

	pipeWriter atomic.Pointer[io.Writer]

	// messageQueue holds queued display messages for this session.
	// It is only accessed by the worker goroutine; no synchronization is needed.
	messageQueue []displayMessage

	lock SessionLock

	// Password buffer for the unlock prompt. Owned exclusively by the
	// worker goroutine while the session is locked.
	password      []rune
	passwordCarry []byte

	// unlockMessage holds a transient "wrong password" hint shown by the
	// lock overlay. Nil when empty; worker goroutine only.
	unlockMessage *displayMessage

	// copySearcher holds the in-progress copy-mode search state while the
	// session is in copy mode. Worker goroutine only.
	copySearcher *CopyModeSearcher
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

	// activeWindowID is the window whose paneManager currently drives
	// pane-level operations. Zero means the root/default paneManager.
	activeWindowID WindowID

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

// activePaneManager returns the pane manager that currently owns focus:
// the root pane manager when no window is active, otherwise the active
// window's pane manager. The worker goroutine is the only caller.
func (m *SessionManager) activePaneManager() *paneManager {
	if m.activeWindowID != 0 {
		if w := m.windowMgr.Window(m.activeWindowID); w != nil {
			return w.paneMgr
		}
	}
	return m.paneMgr
}

// windowIDForSession returns the WindowID that contains the given session,
// or 0 if the session lives in the root/default pane manager.
func (m *SessionManager) windowIDForSession(id SessionID) WindowID {
	if m.paneMgr.PaneIDForSession(id) != 0 {
		return 0
	}
	for _, w := range m.windowMgr.Windows() {
		if w.paneMgr.PaneIDForSession(id) != 0 {
			return w.ID
		}
	}
	return 0
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

// SendKeys converts named keys to terminal bytes and writes them to the
// specified session's PTY. Unrecognized key names return an error.
func (m *SessionManager) SendKeys(id SessionID, keys ...string) error {
	return m.sendRequest(reqSendKeys, sendKeysPayload{id: id, keys: keys}).err
}

// ResetActivity clears the activity-fired flag for the given session so it can
// fire EventActivity again.
func (m *SessionManager) ResetActivity(id SessionID) {
	_ = m.sendRequest(reqResetActivity, id)
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
	resp := m.sendRequest(reqFocusNextPane, direction)
	if resp.value == nil {
		return 0
	}
	return resp.value.(PaneID)
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

// ResizePaneDelta resizes a pane by a directional cell delta. Direction must
// be one of "left", "right", "up", or "down"; positive delta always grows in
// that direction (right/down) or shrinks toward the opposite edge (left/up).
// The new geometry is stored on the pane binding and propagated to the child
// PTY/VTerm. Returns ErrPaneNotFound if the pane does not exist.
func (m *SessionManager) ResizePaneDelta(paneID PaneID, direction string, delta int) error {
	return m.sendRequest(reqResizePaneDelta, &resizePaneDeltaPayload{id: paneID, direction: direction, delta: delta}).err
}

// ActivePaneID returns the currently focused pane ID, or 0 if no panes exist.
func (m *SessionManager) ActivePaneID() PaneID {
	resp := m.sendRequest(reqActivePaneID, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(PaneID)
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

// HandleCopyModeKey dispatches a copy-mode key for the given session.
func (m *SessionManager) HandleCopyModeKey(id SessionID, key string) error {
	return m.sendRequest(reqHandleCopyModeKey, handleCopyModeKeyPayload{sessionID: id, key: key}).err
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

// MoveWindow moves the window with the given ID to the target index in the
// window order. The active window is preserved.
func (m *SessionManager) MoveWindow(id WindowID, targetIndex int) error {
	return m.sendRequest(reqMoveWindow, moveWindowPayload{id: id, targetIndex: targetIndex}).err
}

// SwapWindows exchanges the positions of two windows in the window order.
func (m *SessionManager) SwapWindows(a, b WindowID) error {
	return m.sendRequest(reqSwapWindows, swapWindowsPayload{a: a, b: b}).err
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

// SetSynchronizePanes enables or disables synchronized pane input for the
// active window. If no window is active, the root/default pane manager is used.
func (m *SessionManager) SetSynchronizePanes(v bool) error {
	return m.sendRequest(reqSetSynchronizePanes, v).err
}

// SynchronizePanes reports whether synchronized pane input is enabled for
// the active window. If no window is active, the root/default pane manager is used.
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

// SetLayoutMode sets the layout mode for the given window. Use windowID 0 to
// target the root/default pane manager.
func (m *SessionManager) SetLayoutMode(id WindowID, mode LayoutMode) error {
	return m.sendRequest(reqSetLayoutMode, &setLayoutModePayload{windowID: id, mode: mode}).err
}

// LayoutMode returns the current layout mode for the given window. Use
// windowID 0 to query the root/default pane manager.
func (m *SessionManager) LayoutMode(id WindowID) (LayoutMode, error) {
	resp := m.sendRequest(reqLayoutMode, id)
	if resp.err != nil {
		return LayoutTiled, resp.err
	}
	if resp.value == nil {
		return LayoutTiled, nil
	}
	return resp.value.(LayoutMode), nil
}

func (m *SessionManager) ZoomPane(id PaneID) {
	_ = m.sendRequest(reqZoomPane, id)
}

// ToggleZoom toggles zoom state for the pane with the given ID.
// It is an alias for ZoomPane and exists to satisfy callers that expect
// a ToggleZoom operation.
func (m *SessionManager) ToggleZoom(id PaneID) {
	m.ZoomPane(id)
}

func (m *SessionManager) ZoomedPane() PaneID {
	resp := m.sendRequest(reqZoomedPane, nil)
	if resp.value == nil {
		return 0
	}
	return resp.value.(PaneID)
}

func (m *SessionManager) SetPipe(id SessionID, cfg PipeConfig) error {
	resp := m.sendRequest(reqSetPipe, setPipePayload{sessionID: id, config: cfg})
	return resp.err
}

func (m *SessionManager) SetPipeFile(id SessionID, path string) error {
	return m.SetPipe(id, PipeConfig{Path: path})
}

func (m *SessionManager) PipePaneCommand(id SessionID, cmd string, args []string) error {
	if cmd == "" {
		return fmt.Errorf("termmux: pipe-pane command is required")
	}
	return m.SetPipe(id, PipeConfig{Command: cmd, Args: args})
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

// UnlockPrompt returns the current lock prompt state for a session.
// The response is useful for rendering an overlay without exposing the
// actual password buffer. If the session is not locked, active is false.
func (m *SessionManager) UnlockPrompt(id SessionID) unlockPromptResponse {
	resp := m.sendRequest(reqUnlockPrompt, id)
	if resp.err != nil {
		return unlockPromptResponse{}
	}
	pr, _ := resp.value.(unlockPromptResponse)
	return pr
}

// BreakPane removes the pane with the given ID from its current window
// (or the default pane manager) and moves it into a brand new window.
// Returns the new WindowID, the moved pane's new PaneID, and the moved
// SessionID. The moved pane becomes active in the new window.
func (m *SessionManager) BreakPane(paneID PaneID) (WindowID, PaneID, SessionID, error) {
	resp := m.sendRequest(reqBreakPane, &breakPanePayload{paneID: paneID})
	if resp.err != nil {
		return 0, 0, 0, resp.err
	}
	res := resp.value.(breakJoinResult)
	return res.windowID, res.paneID, res.sessionID, nil
}

// JoinPane moves the pane with the given ID into targetWindowID. The pane
// may live in any existing window or the default pane manager. Returns the
// moved pane's new PaneID and the moved SessionID. The moved pane becomes
// active in the target window.
func (m *SessionManager) JoinPane(paneID PaneID, targetWindowID WindowID) (PaneID, SessionID, error) {
	resp := m.sendRequest(reqJoinPane, &joinPanePayload{paneID: paneID, targetWindowID: targetWindowID})
	if resp.err != nil {
		return 0, 0, resp.err
	}
	res := resp.value.(breakJoinResult)
	return res.paneID, res.sessionID, nil
}

// breakJoinResult is the worker response for break/join operations.
type breakJoinResult struct {
	windowID  WindowID
	paneID    PaneID
	sessionID SessionID
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

// SplitPaneHorizontal creates a new pane below activePane using cfg. An empty
// cfg.Command produces a placeholder session instead of a real process.
func (m *SessionManager) SplitPaneHorizontal(activePane PaneID, cfg CaptureConfig) (PaneID, error) {
	return m.splitPane(activePane, cfg, SplitDown)
}

// SplitPaneVertical creates a new pane to the right of activePane using cfg.
// An empty cfg.Command produces a placeholder session instead of a real process.
func (m *SessionManager) SplitPaneVertical(activePane PaneID, cfg CaptureConfig) (PaneID, error) {
	return m.splitPane(activePane, cfg, SplitRight)
}

func (m *SessionManager) splitPane(activePane PaneID, cfg CaptureConfig, direction SplitDirection) (PaneID, error) {
	resp := m.sendRequest(reqSplitPane, &splitPanePayload{activePane: activePane, cfg: cfg, direction: direction})
	if resp.err != nil {
		return 0, resp.err
	}
	if resp.value == nil {
		return 0, nil
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
	case reqSendKeys:
		resp = m.handleSendKeys(req.payload.(sendKeysPayload))
	case reqResetActivity:
		resp = m.handleResetActivity(req.payload.(SessionID))
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
	case reqResizePaneDelta:
		resp = m.handleResizePaneDelta(req.payload.(*resizePaneDeltaPayload))
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
	case reqHandleCopyModeKey:
		resp = m.handleCopyModeKey(req.payload.(handleCopyModeKeyPayload))
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
	case reqMoveWindow:
		resp = m.handleMoveWindow(req.payload.(moveWindowPayload))
	case reqSwapWindows:
		resp = m.handleSwapWindows(req.payload.(swapWindowsPayload))
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
	case reqSetPipe:
		resp = m.handleSetPipe(req.payload)
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
	case reqUnlockPrompt:
		resp = m.handleUnlockPrompt(req.payload)
	case reqBreakPane:
		resp = m.handleBreakPane(req.payload)
	case reqJoinPane:
		resp = m.handleJoinPane(req.payload)
	case reqAddPaneToWindow:
		resp = m.handleAddPaneToWindow(req.payload)
	case reqSplitPane:
		resp = m.handleSplitPane(req.payload.(*splitPanePayload))
	case reqSetLayoutMode:
		resp = m.handleSetLayoutMode(req.payload)
	case reqLayoutMode:
		resp = m.handleLayoutMode(req.payload)
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

	if cs, ok := p.session.(*CaptureSession); ok {
		ms.captureConfig = cs.ExportConfig()
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

	// Close any active pipe before closing the underlying session.
	m.closePipeForSession(ms)

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
	m.activeWindowID = m.windowIDForSession(id)
	m.windowMgr.setActive(m.activeWindowID)
	if mon, ok := m.monitors[id]; ok {
		mon.ActivityFired = false
	}
	m.eventBus.emit(EventSessionActivated, id)
	return response{}
}

// handleInput routes input to the active session. Locked sessions consume
// keystrokes through the unlock prompt instead of the child PTY.
func (m *SessionManager) handleInput(data []byte) response {
	activeSession := m.activeID
	if m.activeWindowID != 0 {
		activeSession = m.activePaneManager().activeSessionID()
	}
	if activeSession == 0 {
		return response{err: ErrSessionNotFound}
	}

	pm := m.activePaneManager()
	if pm != nil && pm.Synchronize() {
		var firstErr error
		for _, id := range pm.AllSessionIDs() {
			ms, ok := m.sessions[id]
			if !ok {
				continue
			}
			if ms.state != SessionRunning && ms.state != SessionCreated {
				continue
			}
			if ms.lock.IsLocked() {
				if resp := m.handleLockedInput(data, ms, id); resp.err != nil && firstErr == nil {
					firstErr = resp.err
				}
				continue
			}
			if _, err := ms.session.Write(data); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return response{err: firstErr}
	}

	ms, ok := m.sessions[activeSession]
	if !ok {
		return response{err: fmt.Errorf("%w: active session %d disappeared",
			ErrSessionNotFound, activeSession)}
	}
	if ms.state != SessionRunning && ms.state != SessionCreated {
		return response{err: fmt.Errorf("%w: session %d in state %s",
			ErrInvalidTransition, activeSession, ms.state)}
	}
	if ms.lock.IsLocked() {
		return m.handleLockedInput(data, ms, activeSession)
	}
	_, err := ms.session.Write(data)
	return response{err: err}
}

func (m *SessionManager) handleSendKeys(p sendKeysPayload) response {
	ms, ok := m.sessions[p.id]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.id)}
	}
	if ms.state != SessionRunning && ms.state != SessionCreated {
		return response{err: fmt.Errorf("%w: session %d in state %s", ErrInvalidTransition, p.id, ms.state)}
	}
	if ms.lock.IsLocked() {
		return response{err: fmt.Errorf("termmux: cannot send keys to locked session %d", p.id)}
	}

	var buf strings.Builder
	for _, key := range p.keys {
		seq, ok := KeyToTermBytes(key, false, false)
		if !ok {
			return response{err: fmt.Errorf("termmux: unrecognized key %q", key)}
		}
		buf.WriteString(seq)
	}

	_, err := ms.session.Write([]byte(buf.String()))
	return response{err: err}
}

func (m *SessionManager) handleResetActivity(id SessionID) response {
	if mon, ok := m.monitors[id]; ok {
		mon.ActivityFired = false
	}
	return response{}
}

// inputKey labels control bytes handled by the unlock prompt.
type inputKey byte

const (
	keyBackspace inputKey = 0x08
	keyDelete    inputKey = 0x7F
	keyEnter     inputKey = 0x0D
	keyLineFeed  inputKey = 0x0A
	keyEscape    inputKey = 0x1B
)

// handleLockedInput consumes keystrokes for the unlock prompt. It builds a
// password buffer, handles backspace/escape, and submits on enter.
func (m *SessionManager) handleLockedInput(data []byte, ms *managedSession, _ SessionID) response {
	data = append(ms.passwordCarry, data...)
	ms.passwordCarry = nil

	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 && len(data) < utf8.UTFMax {
			ms.passwordCarry = append([]byte(nil), data...)
			break
		}
		data = data[size:]

		if len(ms.password) == 0 && r == 0 {
			continue
		}

		switch {
		case r == rune(keyEscape):
			ms.password = nil
		case r == rune(keyBackspace) || r == rune(keyDelete):
			if len(ms.password) > 0 {
				ms.password = ms.password[:len(ms.password)-1]
			}
		case r == rune(keyEnter) || r == rune(keyLineFeed):
			password := string(ms.password)
			ms.password = nil
			ms.passwordCarry = nil
			if ms.lock.Unlock(password) {
				ms.unlockMessage = nil
				return response{}
			}
			ms.unlockMessage = &displayMessage{
				text:      "wrong password",
				expiresAt: time.Now().Add(2 * time.Second),
			}
		case r >= ' ' && unicode.IsPrint(r):
			ms.password = append(ms.password, r)
		}
	}
	return response{}
}

// handleResize broadcasts the new terminal dimensions to all non-closed sessions.
func (m *SessionManager) handleResize(p *resizePayload) response {
	m.termRows = p.rows
	m.termCols = p.cols
	m.paneMgr.setSize(p.cols, p.rows)
	for _, w := range m.windowMgr.Windows() {
		w.paneMgr.setSize(p.cols, p.rows)
	}
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

// handleSnapshot returns the latest screen snapshot for the given session,
// making sure the transient display message reflects the current queue state
// (including any expired messages) so callers do not see stale overlays.
func (m *SessionManager) handleSnapshot(id SessionID) response {
	ms, ok := m.sessions[id]
	if !ok {
		return response{}
	}
	snap := ms.snapshot.Load()
	if snap == nil {
		return response{}
	}
	now := time.Now()
	activeMsg := m.activeMessageForSession(id, now)
	locked := ms.lock.IsLocked()
	if snap.Message != activeMsg || snap.Locked != locked {
		snap = snap.Clone()
		snap.Message = activeMsg
		snap.Locked = locked
		snap.Timestamp = now
	}
	return response{value: snap}
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

	pm := m.activePaneManager()
	paneID, err := pm.Create(sessionID, p.direction)
	if err != nil {
		m.handleUnregister(sessionID)
		return response{err: err}
	}

	ms, ok := m.sessions[sessionID]
	if ok {
		pm.setVTerm(paneID, ms.vterm)
	}

	m.activeID = sessionID

	return response{value: paneID}
}

func (m *SessionManager) handleClosePane(id PaneID) response {
	pm := m.activePaneManager()

	sessionID := pm.removeSessionID(id)

	if err := pm.Remove(id); err != nil {
		return response{err: err}
	}

	if sessionID != 0 {
		unregResp := m.handleUnregister(sessionID)
		if unregResp.err != nil {
			slog.Warn("session unregister failed during pane close", "sessionID", sessionID, "error", unregResp.err)
		}
	}

	if activeSessionID := pm.activeSessionID(); activeSessionID != 0 {
		m.activeID = activeSessionID
	} else {
		m.activeID = 0
	}

	return response{}
}

func (m *SessionManager) handleFocusPane(id PaneID) response {
	pm := m.activePaneManager()

	if err := pm.Focus(id); err != nil {
		return response{err: err}
	}

	sessionID := pm.activeSessionID()
	if sessionID != 0 {
		m.activeID = sessionID
		m.activeWindowID = m.windowIDForSession(sessionID)
		ms, ok := m.sessions[sessionID]
		if ok {
			ms.lastActive = time.Now()
		}
	}

	return response{}
}

func (m *SessionManager) handleResizePane(p *resizePanePayload) response {
	pm := m.activePaneManager()

	if err := pm.Resize(p.id, p.ratio); err != nil {
		return response{err: err}
	}

	panes := pm.Panes()
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
	pm := m.activePaneManager()

	id, err := pm.FocusAt(p.row, p.col)
	if err != nil {
		return response{err: err}
	}

	sessionID := pm.activeSessionID()
	if sessionID != 0 {
		m.activeID = sessionID
		m.activeWindowID = m.windowIDForSession(sessionID)
		if ms, ok := m.sessions[sessionID]; ok {
			ms.lastActive = time.Now()
		}
	}

	return response{value: id}
}

func (m *SessionManager) handleResizePaneAt(p *resizePaneAtPayload) response {
	pm := m.activePaneManager()

	if err := pm.ResizePaneAt(p.row, p.col, p.ratio); err != nil {
		return response{err: err}
	}

	panes := pm.Panes()
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

func (m *SessionManager) handleResizePaneDelta(p *resizePaneDeltaPayload) response {
	pm := m.activePaneManager()

	geo, err := pm.ResizePaneDelta(p.id, p.direction, p.delta)
	if err != nil {
		return response{err: err}
	}

	if err := pm.setGeometry(p.id, geo); err != nil {
		return response{err: err}
	}

	sessionID := pm.removeSessionID(p.id)
	if sessionID != 0 {
		if ms, ok := m.sessions[sessionID]; ok && ms.state != SessionClosed {
			ms.vterm.Resize(geo.Rows, geo.Cols)
			if err := ms.session.Resize(geo.Rows, geo.Cols); err != nil {
				slog.Warn("session resize failed during pane resize delta", "sessionID", sessionID, "error", err)
			}
		}
	}

	windowID := m.windowIDForPane(p.id)
	m.emitWindowUpdated(windowID, windowID)

	return response{}
}

func (m *SessionManager) handlePanes() response {
	pm := m.activePaneManager()
	return response{value: pm.Panes()}
}

func (m *SessionManager) handleFocusNextPane(direction NavigationDirection) response {
	pm := m.activePaneManager()
	nextID := pm.FocusNext(direction)
	if nextID != 0 {
		if sid := pm.activeSessionID(); sid != 0 {
			m.activeID = sid
			m.activeWindowID = m.windowIDForSession(sid)
			if ms, ok := m.sessions[sid]; ok {
				ms.lastActive = time.Now()
			}
		}
	}
	return response{value: nextID}
}

func (m *SessionManager) handleActivePaneID() response {
	return response{value: m.activePaneManager().ActivePaneID()}
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

func (m *SessionManager) handleCopyModeKey(p handleCopyModeKeyPayload) response {
	ms, ok := m.sessions[p.sessionID]
	if !ok {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}

	action := defaultCopyModeKeyHandler.HandleKey(p.key)
	if !ms.vterm.InCopyMode() {
		switch action.Kind {
		case CopyModeActionNone, CopyModeActionExitCopyMode:
			return response{}
		case CopyModeActionEnterCopyMode:
			ms.vterm.EnterCopyMode()
			ms.copySearcher = nil
			return response{}
		}
		return response{err: fmt.Errorf("termmux: session %d is not in copy mode", p.sessionID)}
	}

	switch action.Kind {
	case CopyModeActionMoveLeft:
		ms.vterm.MoveCopyModeCursor(0, -action.N)
	case CopyModeActionMoveRight:
		ms.vterm.MoveCopyModeCursor(0, action.N)
	case CopyModeActionMoveUp:
		ms.vterm.ScrollCopyMode(action.N)
	case CopyModeActionMoveDown:
		ms.vterm.ScrollCopyMode(-action.N)
	case CopyModeActionHalfPageUp:
		ms.vterm.ScrollCopyMode(action.N)
	case CopyModeActionHalfPageDown:
		ms.vterm.ScrollCopyMode(-action.N)
	case CopyModeActionPageUp:
		ms.vterm.ScrollCopyMode(m.termRows)
	case CopyModeActionPageDown:
		ms.vterm.ScrollCopyMode(-m.termRows)
	case CopyModeActionTopOfScrollback:
		ms.vterm.ScrollCopyModeToTop()
	case CopyModeActionBottomOfScrollback:
		ms.vterm.ScrollCopyModeToBottom()
	case CopyModeActionBeginningOfLine:
		ms.vterm.SetCopyModeCursorCol(0)
	case CopyModeActionEndOfLine:
		ms.vterm.SetCopyModeCursorCol(m.termCols - 1)
	case CopyModeActionNextWord:
		ms.vterm.MoveCopyModeCursor(0, 5)
	case CopyModeActionPrevWord:
		ms.vterm.MoveCopyModeCursor(0, -5)
	case CopyModeActionExitCopyMode:
		ms.vterm.ExitCopyMode()
		ms.copySearcher = nil
	case CopyModeActionSelectStart:
		row, col := ms.vterm.CopyModeCursorPosition()
		ms.vterm.SelectStart(row, col)
	case CopyModeActionCopyAndExit:
		row, col := ms.vterm.CopyModeCursorPosition()
		ms.vterm.SelectEnd(row, col)
		ms.vterm.CopySelection()
		ms.vterm.ExitCopyMode()
	case CopyModeActionSearchForward:
		if ms.copySearcher == nil {
			ms.copySearcher = NewCopyModeSearcher()
		}
		row, col := ms.vterm.CopyModeCursorPosition()
		absRow := ms.vterm.CopyModeScrollOffset() + row
		ms.copySearcher.StartSearch(SearchForward, absRow, col)
	case CopyModeActionSearchBackward:
		if ms.copySearcher == nil {
			ms.copySearcher = NewCopyModeSearcher()
		}
		row, col := ms.vterm.CopyModeCursorPosition()
		absRow := ms.vterm.CopyModeScrollOffset() + row
		ms.copySearcher.StartSearch(SearchBackward, absRow, col)
	case CopyModeActionNextMatch, CopyModeActionPrevMatch:
		// Search execution is intentionally delegated to the JS search bindings.
	case CopyModeActionEnterCopyMode, CopyModeActionNone:
		// No state change required.
	}

	return response{}
}

func (m *SessionManager) handleNewWindow(p *newWindowPayload) response {
	id := m.windowMgr.NewWindow(p.name)
	return response{value: id}
}

func (m *SessionManager) handleNextWindow() response {
	id := m.windowMgr.NextWindow()
	m.activeWindowID = id
	m.activateFirstPaneInActiveWindow()
	return response{value: id}
}

func (m *SessionManager) handlePrevWindow() response {
	id := m.windowMgr.PrevWindow()
	m.activeWindowID = id
	m.activateFirstPaneInActiveWindow()
	return response{value: id}
}

// activateFirstPaneInActiveWindow focuses the active window's first pane,
// updates m.activeID to that pane's session, and emits EventSessionActivated.
func (m *SessionManager) activateFirstPaneInActiveWindow() {
	pm := m.activePaneManager()
	if pm == nil {
		m.activeID = 0
		return
	}

	paneID := pm.ActivePaneID()
	if paneID == 0 {
		m.activeID = 0
		return
	}

	if err := pm.Focus(paneID); err != nil {
		m.activeID = 0
		return
	}

	sessionID := pm.activeSessionID()
	if sessionID == 0 {
		m.activeID = 0
		return
	}

	m.activeID = sessionID
	m.activeWindowID = m.windowIDForSession(sessionID)
	if ms, ok := m.sessions[sessionID]; ok {
		ms.lastActive = time.Now()
	}
	m.eventBus.emit(EventSessionActivated, sessionID)
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

	// If the closed window was active, follow the WindowManager's new active
	// window so pane operations stay consistent.
	if m.activeWindowID == id {
		m.activeWindowID = m.windowMgr.ActiveWindowID()
		m.activateFirstPaneInActiveWindow()
	}

	return response{}
}

func (m *SessionManager) handleMoveWindow(p moveWindowPayload) response {
	return response{err: m.windowMgr.MoveWindow(p.id, p.targetIndex)}
}

func (m *SessionManager) handleSwapWindows(p swapWindowsPayload) response {
	return response{err: m.windowMgr.SwapWindows(p.a, p.b)}
}

func (m *SessionManager) handleActiveWindowID() response {
	if m.activeWindowID != 0 {
		return response{value: m.activeWindowID}
	}
	return response{value: m.windowMgr.ActiveWindowID()}
}

func (m *SessionManager) handleWindows() response {
	return response{value: m.windowMgr.Windows()}
}

func (m *SessionManager) handleSetSynchronizePanes(v bool) response {
	pm := m.activePaneManager()
	if pm != nil {
		pm.SetSynchronize(v)
	}
	return response{}
}

func (m *SessionManager) handleSynchronizePanes() response {
	pm := m.activePaneManager()
	if pm == nil {
		return response{value: false}
	}
	return response{value: pm.Synchronize()}
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
	return response{err: m.activePaneManager().SetPaneRemainOnExit(p.paneID, p.value)}
}

func (m *SessionManager) handlePaneRemainOnExit(payload any) response {
	id, ok := payload.(PaneID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	v, err := m.activePaneManager().PaneRemainOnExit(id)
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
	return response{value: m.activePaneManager().PaneExited(id)}
}

func (m *SessionManager) rebindSessionID(oldID, newID SessionID, vterm *vt.VTerm) error {
	if paneID := m.paneMgr.PaneIDForSession(oldID); paneID != 0 {
		return m.paneMgr.RebindSession(paneID, newID, vterm)
	}
	for _, w := range m.windowMgr.Windows() {
		if paneID := w.paneMgr.PaneIDForSession(oldID); paneID != 0 {
			return w.paneMgr.RebindSession(paneID, newID, vterm)
		}
	}
	return nil
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

	if err := ms.session.Close(); err != nil {
		slog.Debug("respawn old session close failed", "sessionID", oldID, "error", err)
	}

	cfg := ms.captureConfig
	newSession := NewCaptureSession(cfg)
	newID := m.nextID
	m.nextID++

	if cfg.Command != "" {
		if err := newSession.Start(m.readerCtx); err != nil {
			return response{err: fmt.Errorf("termmux: respawn start failed: %w", err)}
		}
		if err := newSession.Resize(m.termRows, m.termCols); err != nil {
			slog.Debug("respawn resize failed", "sessionID", newID, "error", err)
		}
	}

	v := vt.NewVTerm(m.termRows, m.termCols)
	v.BellFn = func() {
		m.eventBus.emit(EventBell, newID)
		if mon, ok := m.monitors[newID]; ok && mon.Config.Bell {
			mon.VisualBell = VisualBellState{Active: true, StartedAt: time.Now()}
		}
	}

	newMS := &managedSession{
		session:       newSession,
		vterm:         v,
		state:         SessionCreated,
		target:        ms.target,
		lastActive:    time.Now(),
		remainOnExit:  ms.remainOnExit,
		captureConfig: cfg,
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

	if err := m.rebindSessionID(oldID, newID, v); err != nil {
		slog.Debug("respawn rebind failed", "sessionID", newID, "error", err)
	}

	wasActive := m.activeID == oldID
	if wasActive {
		m.activeID = newID
	}

	delete(m.sessions, oldID)

	m.eventBus.emit(EventSessionRegistered, newID)
	if wasActive {
		m.eventBus.emit(EventSessionActivated, newID)
	}
	m.startReaderGoroutine(newID, newSession)

	return response{value: newID}
}

func (m *SessionManager) handleSwapPanes(payload any) response {
	p, ok := payload.(swapPanesPayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}

	windowA := m.windowIDForPane(p.a)
	windowB := m.windowIDForPane(p.b)

	pmA := m.paneManagerForWindow(windowA)
	pmB := m.paneManagerForWindow(windowB)

	if pmA == nil || pmA.Binding(p.a) == nil {
		return response{err: fmt.Errorf("%w: %d", ErrPaneNotFound, p.a)}
	}
	if pmB == nil || pmB.Binding(p.b) == nil {
		return response{err: fmt.Errorf("%w: %d", ErrPaneNotFound, p.b)}
	}

	if pmA == pmB {
		// Both panes live in the same pane manager. Swap metadata through the
		// pane manager and update the layout order without applying the
		// metadata swap a second time.
		if err := pmA.Swap(p.a, p.b); err != nil {
			return response{err: err}
		}
		if !pmA.engine.swapLayoutOnly(p.a, p.b) {
			return response{err: fmt.Errorf("termmux: swap failed: one or both panes not found in layout")}
		}
	} else {
		// Panes live in different windows: exchange only their metadata so the
		// sessions, titles, and lifecycle state trade places while the pane
		// geometries and IDs stay in their original windows.
		swapPaneBindingMetadata(pmA.Binding(p.a), pmB.Binding(p.b))
	}

	// Keep the active session consistent with the pane that currently has focus.
	if activePM := m.activePaneManager(); activePM != nil {
		if activeSID := activePM.activeSessionID(); activeSID != 0 {
			m.activeID = activeSID
		}
	}

	// Notify subscribers for every affected window.
	m.emitWindowUpdated(windowA, windowA)
	if windowA != windowB {
		m.emitWindowUpdated(windowB, windowB)
	}

	return response{value: true}
}

// paneManagerForWindow returns the pane manager for the given window ID, or the
// root/default pane manager when id is 0.
func (m *SessionManager) paneManagerForWindow(id WindowID) *paneManager {
	if id == 0 {
		return m.paneMgr
	}
	if w := m.windowMgr.Window(id); w != nil {
		return w.paneMgr
	}
	return nil
}

func (m *SessionManager) emitWindowUpdated(source, target WindowID) {
	m.eventBus.emitData(EventWindowUpdated, 0, windowUpdateData{SourceWindow: source, TargetWindow: target})
}

func (m *SessionManager) handleZoomPane(payload any) response {
	id, ok := payload.(PaneID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	pm := m.activePaneManager()
	if pm.engine.ZoomedPane() == id {
		pm.engine.Unzoom()
	} else {
		pm.engine.Zoom(id)
	}
	return response{}
}

func (m *SessionManager) handleZoomedPane() response {
	return response{value: m.activePaneManager().engine.ZoomedPane()}
}

func (m *SessionManager) handleSetPipe(payload any) response {
	p, ok := payload.(setPipePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[p.sessionID]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, p.sessionID)}
	}

	cfg := p.config
	hasPath := cfg.Path != ""
	hasCmd := cfg.Command != ""
	if !hasPath && !hasCmd {
		return response{err: fmt.Errorf("termmux: pipe-pane requires Path or Command")}
	}
	if hasPath && hasCmd {
		return response{err: fmt.Errorf("termmux: pipe-pane Path and Command are mutually exclusive")}
	}

	m.closePipeForSession(ms)

	if hasPath {
		f, err := os.OpenFile(cfg.Path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
		if err != nil {
			return response{err: fmt.Errorf("termmux: pipe-pane open: %w", err)}
		}
		var w io.Writer = f
		ms.pipeWriter.Store(&w)
		return response{}
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return response{err: fmt.Errorf("termmux: pipe-pane stdin pipe: %w", err)}
	}
	if err := cmd.Start(); err != nil {
		if cerr := stdin.Close(); cerr != nil {
			slog.Debug("pipe-pane stdin close failed", "error", cerr)
		}
		return response{err: fmt.Errorf("termmux: pipe-pane start command: %w", err)}
	}
	pp := &pipeProcess{cmd: cmd, stdin: stdin, done: make(chan struct{})}
	go pp.reap()
	var w io.Writer = pp
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
	m.closePipeForSession(ms)
	return response{}
}

// closePipeForSession closes any active pipe (file or command) for the
// session. It is safe to call when no pipe is set.
func (m *SessionManager) closePipeForSession(ms *managedSession) {
	if old := ms.pipeWriter.Swap(nil); old != nil {
		if c, ok := (*old).(io.Closer); ok {
			_ = c.Close()
		}
	}
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
	ms.messageQueue = append(ms.messageQueue, displayMessage{
		text:      p.text,
		expiresAt: time.Now().Add(dur),
	})
	if len(ms.messageQueue) > maxDisplayMessages {
		ms.messageQueue = ms.messageQueue[len(ms.messageQueue)-maxDisplayMessages:]
	}
	now := time.Now()
	if snap := ms.snapshot.Load(); snap != nil {
		m.snapshotGen++
		newSnap := snap.Clone()
		newSnap.Gen = m.snapshotGen
		newSnap.Message = m.activeMessageForSession(p.sessionID, now)
		newSnap.Timestamp = now
		ms.snapshot.Store(newSnap)
	}
	return response{}
}

// activeMessageForSession returns the current non-expired message for a
// session, advancing the queue past any expired front entries.
func (m *SessionManager) activeMessageForSession(id SessionID, now time.Time) string {
	ms, exists := m.sessions[id]
	if !exists {
		return ""
	}
	for len(ms.messageQueue) > 0 && now.After(ms.messageQueue[0].expiresAt) {
		ms.messageQueue = ms.messageQueue[1:]
	}
	if len(ms.messageQueue) > 0 {
		return ms.messageQueue[0].text
	}
	return ""
}

func (m *SessionManager) handleActiveMessage(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	return response{value: m.activeMessageForSession(id, time.Now())}
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
	ms.password = nil
	ms.passwordCarry = nil
	ms.unlockMessage = nil
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

func (m *SessionManager) handleUnlockPrompt(payload any) response {
	id, ok := payload.(SessionID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	ms, exists := m.sessions[id]
	if !exists {
		return response{err: fmt.Errorf("%w: %d", ErrSessionNotFound, id)}
	}
	pr := unlockPromptResponse{active: ms.lock.IsLocked(), maskLen: len(ms.password)}
	if msg := ms.unlockMessage; msg != nil && time.Now().Before(msg.expiresAt) {
		pr.message = msg.text
		pr.expiresAt = msg.expiresAt
	}
	return response{value: pr}
}

func (m *SessionManager) handleBreakPane(payload any) response {
	p, ok := payload.(*breakPanePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}

	sourceWindowID := m.windowIDForPane(p.paneID)
	if sourceWindowID == 0 && m.paneMgr.Binding(p.paneID) == nil {
		return response{err: fmt.Errorf("%w: %d", ErrPaneNotFound, p.paneID)}
	}

	var newWindowID WindowID
	var newPaneID PaneID
	var sessionID SessionID
	var err error

	if sourceWindowID == 0 {
		newWindowID = m.windowMgr.NewWindow("")
		newWindow := m.windowMgr.Window(newWindowID)
		if newWindow == nil {
			return response{err: fmt.Errorf("termmux: failed to create new window")}
		}
		newPaneID, sessionID, err = m.paneMgr.transferPaneToWindow(p.paneID, newWindow.paneMgr, SplitRight)
	} else {
		newWindowID, newPaneID, sessionID, err = m.windowMgr.BreakPane(sourceWindowID, p.paneID)
	}
	if err != nil {
		return response{err: err}
	}

	m.eventBus.emitData(EventWindowUpdated, 0, windowUpdateData{sourceWindowID, newWindowID})

	// break-pane always makes the new window active and focuses the moved
	// session, matching tmux's default behavior.
	m.activeID = sessionID
	m.activeWindowID = newWindowID
	m.windowMgr.setActive(newWindowID)
	m.eventBus.emit(EventSessionActivated, sessionID)

	return response{value: breakJoinResult{windowID: newWindowID, paneID: newPaneID, sessionID: sessionID}}
}

func (m *SessionManager) handleJoinPane(payload any) response {
	p, ok := payload.(*joinPanePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}

	sourceWindowID := m.windowIDForPane(p.paneID)
	if sourceWindowID == 0 && m.paneMgr.Binding(p.paneID) == nil {
		return response{err: fmt.Errorf("%w: %d", ErrPaneNotFound, p.paneID)}
	}

	targetWindow := m.windowMgr.Window(p.targetWindowID)
	if targetWindow == nil {
		return response{err: fmt.Errorf("%w: %d", ErrWindowNotFound, p.targetWindowID)}
	}
	if sourceWindowID == p.targetWindowID {
		return response{err: fmt.Errorf("termmux: cannot join pane into the same window")}
	}

	var newPaneID PaneID
	var sessionID SessionID
	var err error
	if sourceWindowID == 0 {
		newPaneID, sessionID, err = m.paneMgr.transferPaneToWindow(p.paneID, targetWindow.paneMgr, SplitRight)
	} else {
		newPaneID, sessionID, err = m.windowMgr.JoinPane(sourceWindowID, p.targetWindowID, p.paneID)
	}
	if err != nil {
		return response{err: err}
	}

	m.eventBus.emitData(EventWindowUpdated, 0, windowUpdateData{sourceWindowID, p.targetWindowID})

	m.activeID = sessionID
	m.activeWindowID = p.targetWindowID
	m.windowMgr.setActive(p.targetWindowID)
	m.eventBus.emit(EventSessionActivated, sessionID)

	return response{value: breakJoinResult{paneID: newPaneID, sessionID: sessionID}}
}

// windowUpdateData is the payload for EventWindowUpdated.
type windowUpdateData struct {
	SourceWindow WindowID
	TargetWindow WindowID
}

// windowIDForPane returns the window containing pane id, or 0 if the pane
// lives in the default (root) pane manager.
func (m *SessionManager) windowIDForPane(id PaneID) WindowID {
	if m.paneMgr.Binding(id) != nil {
		return 0
	}
	for _, w := range m.windowMgr.Windows() {
		if w.paneMgr.Binding(id) != nil {
			return w.ID
		}
	}
	return 0
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

func (m *SessionManager) handleSplitPane(p *splitPanePayload) response {
	if resp := m.handleFocusPane(p.activePane); resp.err != nil {
		return resp
	}

	var session InteractiveSession
	var target SessionTarget
	if p.cfg.Command == "" {
		session = newDefaultInteractiveSession()
		target = SessionTarget{Name: defaultSessionName(p.cfg.Name), Kind: p.cfg.Kind}
		if target.Kind == SessionKindUnknown {
			target.Kind = SessionKindPTY
		}
	} else {
		cs := NewCaptureSession(p.cfg)
		if err := cs.Start(m.readerCtx); err != nil {
			return response{err: err}
		}
		session = cs
		target = SessionTarget{Name: p.cfg.Name, Kind: p.cfg.Kind}
	}

	regResp := m.handleRegister(&registerPayload{session: session, target: target})
	if regResp.err != nil {
		if err := session.Close(); err != nil {
			slog.Debug("split pane session close failed", "error", err)
		}
		return regResp
	}
	sessionID := regResp.value.(SessionID)

	pm := m.activePaneManager()
	paneID, err := pm.Create(sessionID, p.direction)
	if err != nil {
		m.handleUnregister(sessionID)
		return response{err: err}
	}

	ms, ok := m.sessions[sessionID]
	if ok {
		pm.setVTerm(paneID, ms.vterm)
	}

	m.activeID = sessionID
	return response{value: paneID}
}

func (m *SessionManager) handleSetLayoutMode(payload any) response {
	p, ok := payload.(*setLayoutModePayload)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	pm := m.paneManagerForWindow(p.windowID)
	if pm == nil {
		return response{err: fmt.Errorf("%w: %d", ErrWindowNotFound, p.windowID)}
	}
	pm.setMode(p.mode)
	if p.windowID != 0 {
		if w := m.windowMgr.Window(p.windowID); w != nil {
			w.Layout = p.mode
		}
	}
	m.emitWindowUpdated(p.windowID, p.windowID)
	return response{}
}

func (m *SessionManager) handleLayoutMode(payload any) response {
	id, ok := payload.(WindowID)
	if !ok {
		return response{err: fmt.Errorf("%w: invalid payload type", ErrInvalidPayload)}
	}
	pm := m.paneManagerForWindow(id)
	if pm == nil {
		return response{err: fmt.Errorf("%w: %d", ErrWindowNotFound, id)}
	}
	return response{value: pm.mode()}
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
				m.closePipeForSession(ms)
				m.markSessionPaneExited(so.id)
				return
			}
			m.closePipeForSession(ms)
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
			m.closePipeForSession(ms)
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
	now := time.Now()
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
		Locked:             ms.lock.IsLocked(),
		Message:            m.activeMessageForSession(so.id, now),
		Timestamp:          now,
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

		if mon.Config.Activity && so.id != m.activeID {
			if mon.ActivityFired && mon.Config.ActivityResetThreshold > 0 && wasIdle >= mon.Config.ActivityResetThreshold {
				mon.ActivityFired = false
			}
			if !mon.ActivityFired && (mon.Config.ActivityThreshold <= 0 || wasIdle >= mon.Config.ActivityThreshold) {
				mon.ActivityFired = true
				m.eventBus.emit(EventActivity, so.id)
			}
		}
	}
}

func (m *SessionManager) markSessionPaneExited(sid SessionID) {
	if pid := m.paneMgr.PaneIDForSession(sid); pid != 0 {
		m.paneMgr.MarkPaneExited(pid)
		return
	}
	for _, w := range m.windowMgr.Windows() {
		pm := w.paneMgr
		if pm == nil {
			continue
		}
		if pid := pm.PaneIDForSession(sid); pid != 0 {
			pm.MarkPaneExited(pid)
			return
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
			m.closePipeForSession(ms)
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
