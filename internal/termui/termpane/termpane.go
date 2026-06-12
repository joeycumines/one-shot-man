// Package termpane provides a bubbletea v2 Model that bridges a termmux
// terminal session into the bubbletea rendering pipeline. It subscribes to
// EventBus output events, renders ScreenSnapshot content with generation-
// checked caching, and forwards keyboard/mouse input to the PTY.
package termpane

import (
	"log/slog"
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// outputMsg wraps a termmux.Event delivered from the EventBus subscription
// goroutine to the bubbletea Update loop. It is an unexported type so only
// this package can produce it — external code cannot inject fake output
// messages.
type outputMsg termmux.Event

// Model implements tea.Model and bridges a single termmux session into the
// bubbletea rendering pipeline. Each Model owns one session and one
// EventBus subscription.
//
// Lifecycle:
//
//	NewModel → Init → Update loop → Close (on QuitMsg or external shutdown)
//
// All mutable state is protected by mu. View() is safe to call concurrently
// from the bubbletea goroutine.
type Model struct {
	mu sync.Mutex

	// sessionID identifies the termmux session this pane renders.
	sessionID termmux.SessionID

	// manager provides access to Snapshot, ResizeSession, and Input.
	manager *termmux.SessionManager

	// bounds is the terminal region this pane occupies. Cursor positions
	// are offset by bounds.Position when rendering.
	bounds coordinate.Rect

	// subID is the EventBus subscription ID returned by Subscribe.
	subID int

	// eventCh receives events from the EventBus subscription goroutine.
	// The goroutine filters by sessionID and forwards matching events.
	eventCh <-chan termmux.Event

	// outputCh is the channel consumed by the tea.Cmd returned from Init.
	// The subscription goroutine writes to outputCh; the Cmd reads from it
	// and returns the event as a tea.Msg (outputMsg).
	outputCh chan termmux.Event

	// done is closed by Close to signal the subscription goroutine to exit.
	done chan struct{}

	// wg tracks the bridge goroutine. Close waits on it before closing
	// outputCh to avoid "send on closed channel" panics.
	wg sync.WaitGroup

	// snap holds the most recent ScreenSnapshot from the session manager.
	// Updated in Update when an outputMsg arrives.
	snap *termmux.ScreenSnapshot

	// cachedView is the rendered Content string from the last View call.
	cachedView string

	// cachedGen is the generation counter of cachedView. If the current
	// snapshot's Gen matches cachedGen, View returns the cache without
	// re-rendering.
	cachedGen uint64

	// closed tracks whether Close has been called (idempotent guard).
	closed bool

	// appCursor mirrors the active screen's ApplicationCursor mode flag.
	// When true, arrow keys and home/end are encoded using SS3 sequences
	// (ESC O{A-D/H/F) instead of CSI sequences, matching the application
	// cursor mode set by DECSET ?1h (DECCKM).
	appCursor bool
}

// NewModel creates a Model that renders the given termmux session within
// the specified bounds. It subscribes to the SessionManager's EventBus for
// output events and starts a goroutine that bridges events to the
// bubbletea Update loop.
//
// The caller must call Close when the Model is no longer needed to
// unsubscribe from the EventBus and stop the bridge goroutine.
func NewModel(sessionID termmux.SessionID, manager *termmux.SessionManager, bounds coordinate.Rect) *Model {
	m := &Model{
		sessionID: sessionID,
		manager:   manager,
		bounds:    bounds,
		outputCh:  make(chan termmux.Event, 64),
		done:      make(chan struct{}),
	}

	// Subscribe to the manager's EventBus for output events.
	m.subID, m.eventCh = manager.Subscribe(64)

	// Start the bridge goroutine: reads from EventBus subscription,
	// filters by sessionID, and forwards to outputCh for the tea.Cmd.
	m.wg.Add(1)
	go m.bridgeEvents()

	// Get initial snapshot.
	m.snap = manager.Snapshot(sessionID)
	if m.snap != nil {
		m.appCursor = m.snap.ApplicationCursor
	}

	return m
}

// bridgeEvents reads events from the EventBus subscription channel and
// forwards those matching this Model's sessionID to outputCh. It exits
// when either the subscription channel or the done channel is closed.
func (m *Model) bridgeEvents() {
	defer m.wg.Done()
	for {
		select {
		case <-m.done:
			return
		case evt, ok := <-m.eventCh:
			if !ok {
				// Subscription channel closed (unsubscribed or bus closed).
				return
			}
			// Filter: only forward events for our session.
			if evt.SessionID != m.sessionID {
				continue
			}
			select {
			case m.outputCh <- evt:
			case <-m.done:
				return
			}
		}
	}
}

// Init implements tea.Model. It returns a tea.Cmd that blocks on outputCh
// and delivers the next event as an outputMsg. This is the standard
// channel-based async Cmd pattern for bridging external events into
// bubbletea.
func (m *Model) Init() tea.Cmd {
	return m.waitForOutput
}

// waitForOutput is a tea.Cmd that blocks until an event arrives on outputCh
// or the done channel is closed. It returns the event wrapped as an
// outputMsg, or nil if the model is shutting down.
func (m *Model) waitForOutput() tea.Msg {
	select {
	case evt, ok := <-m.outputCh:
		if !ok {
			return nil
		}
		return outputMsg(evt)
	case <-m.done:
		return nil
	}
}

// Update implements tea.Model. It handles:
//   - outputMsg: refreshes the snapshot from the manager and re-subscribes
//     for the next event.
//   - tea.WindowSizeMsg: updates bounds and resizes the session.
//   - tea.KeyPressMsg: forwards key input to the PTY.
//   - tea.MouseMsg: forwards mouse input to the PTY (with coordinate offset).
//   - tea.QuitMsg: unsubscribes and cleans up.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case outputMsg:
		// Refresh snapshot from the manager (authoritative source).
		m.mu.Lock()
		m.snap = m.manager.Snapshot(m.sessionID)
		if m.snap != nil {
			m.appCursor = m.snap.ApplicationCursor
		}
		m.mu.Unlock()
		// Re-subscribe for the next event.
		return m, m.waitForOutput

	case tea.WindowSizeMsg:
		m.mu.Lock()
		m.bounds.Size.Width = msg.Width
		m.bounds.Size.Height = msg.Height
		sid := m.sessionID
		rows := msg.Height
		cols := msg.Width
		m.mu.Unlock()
		// Resize the session's VTerm and PTY to match the new bounds.
		if err := m.manager.ResizeSession(sid, rows, cols); err != nil {
			slog.Debug("termpane resize session failed", "sessionID", sid, "error", err)
		}
		return m, nil

	case tea.KeyPressMsg:
		m.forwardKey(msg)
		return m, nil

	case tea.MouseMsg:
		m.forwardMouse(msg)
		return m, nil

	case tea.QuitMsg:
		m.Close()
		return m, tea.Quit

	default:
		return m, nil
	}
}

// forwardKey converts a bubbletea KeyPressMsg to terminal bytes and writes
// them to the session's PTY via the manager.
func (m *Model) forwardKey(msg tea.KeyPressMsg) {
	keyStr := msg.String()
	seq, ok := termmux.KeyToTermBytes(keyStr, m.appCursor)
	if !ok {
		// Unrecognized key — try the text field for printable characters.
		if msg.Key().Text != "" {
			seq = msg.Key().Text
		} else {
			slog.Debug("termpane unrecognized key", "key", keyStr)
			return
		}
	}
	if err := m.manager.Input([]byte(seq)); err != nil {
		slog.Debug("termpane key forward failed", "key", keyStr, "error", err)
	}
}

// forwardMouse converts a bubbletea MouseMsg to an SGR escape sequence and
// writes it to the session's PTY. Screen coordinates are offset by the
// pane's bounds position to produce pane-local coordinates.
func (m *Model) forwardMouse(msg tea.MouseMsg) {
	mouse := msg.Mouse()

	m.mu.Lock()
	offsetRow := m.bounds.Position.Y
	offsetCol := m.bounds.Position.X
	m.mu.Unlock()

	evt := termmux.MouseEvent{
		X:     mouse.X,
		Y:     mouse.Y,
		Shift: mouse.Mod.Contains(tea.ModShift),
		Alt:   mouse.Mod.Contains(tea.ModAlt),
		Ctrl:  mouse.Mod.Contains(tea.ModCtrl),
	}

	// Determine button and event type from the concrete message type.
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		evt.Type = termmux.MouseClick
		evt.Button = mouseButtonToTermmux(msg.Button)
	case tea.MouseReleaseMsg:
		evt.Type = termmux.MouseRelease
		evt.Button = mouseButtonToTermmux(msg.Button)
	case tea.MouseMotionMsg:
		evt.Type = termmux.MouseMotion
		evt.Button = mouseButtonToTermmux(msg.Button)
	case tea.MouseWheelMsg:
		evt.Type = termmux.MouseWheel
		evt.Button = mouseButtonToTermmux(msg.Button)
	default:
		return
	}

	seq, ok := termmux.MouseToSGR(evt, offsetRow, offsetCol)
	if !ok {
		return
	}
	if err := m.manager.Input([]byte(seq)); err != nil {
		slog.Debug("termpane mouse forward failed", "error", err)
	}
}

// mouseButtonToTermmux converts a tea.MouseButton to a termmux.MouseButton.
func mouseButtonToTermmux(b tea.MouseButton) termmux.MouseButton {
	switch b {
	case tea.MouseLeft:
		return termmux.MouseLeft
	case tea.MouseMiddle:
		return termmux.MouseMiddle
	case tea.MouseRight:
		return termmux.MouseRight
	case tea.MouseWheelUp:
		return termmux.MouseWheelUp
	case tea.MouseWheelDown:
		return termmux.MouseWheelDown
	default:
		return termmux.MouseNone
	}
}

// View implements tea.Model. It renders the terminal session's screen
// content into a tea.View with generation-checked caching.
//
// If the snapshot generation matches cachedGen, the cached view is returned
// without re-rendering. Otherwise, FullScreen content from the ScreenSnapshot
// is rendered, cursor position is set (offset by bounds.Position), and the
// result is cached.
func (m *Model) View() tea.View {
	m.mu.Lock()
	defer m.mu.Unlock()

	// No snapshot available — render empty.
	if m.snap == nil {
		return tea.NewView("")
	}

	// Generation check: skip re-render if unchanged.
	if m.snap.Gen == m.cachedGen && m.cachedView != "" {
		v := tea.NewView(m.cachedView)
		return v
	}

	// Render the full screen content.
	content := m.snap.FullScreen
	if content == "" {
		// Fallback to ANSI if FullScreen is empty.
		content = m.snap.ANSI
	}

	// Build cursor position offset by bounds.
	cursorRow := m.snap.CursorRow + m.bounds.Position.Y
	cursorCol := m.snap.CursorCol + m.bounds.Position.X

	// Determine cursor visibility: cursor is visible only if it falls
	// within the pane's bounds.
	cursorVisible := cursorRow >= m.bounds.Position.Y &&
		cursorRow < m.bounds.Position.Y+m.bounds.Size.Height &&
		cursorCol >= m.bounds.Position.X &&
		cursorCol < m.bounds.Position.X+m.bounds.Size.Width

	v := tea.NewView(content)

	if cursorVisible {
		v.Cursor = tea.NewCursor(cursorCol, cursorRow)
	}

	// Cache the result.
	m.cachedView = content
	m.cachedGen = m.snap.Gen

	return v
}

// Close unsubscribes from the EventBus, signals the bridge goroutine to
// exit, and closes outputCh. It is safe to call multiple times.
func (m *Model) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}
	m.closed = true

	// Signal the bridge goroutine to stop.
	close(m.done)

	// Unsubscribe from the EventBus (closes the subscription channel).
	m.manager.Unsubscribe(m.subID)

	// Wait for the bridge goroutine to exit before closing outputCh.
	// Without this, the goroutine could write to outputCh after it's
	// closed, causing a "send on closed channel" panic.
	m.wg.Wait()

	// Close outputCh so any pending waitForOutput Cmd returns nil.
	close(m.outputCh)

	return nil
}

// SetBounds updates the terminal region this pane occupies. This is used
// when the pane is resized or repositioned within a layout.
func (m *Model) SetBounds(bounds coordinate.Rect) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bounds = bounds
}

// SessionID returns the termmux session ID this pane is bound to.
func (m *Model) SessionID() termmux.SessionID {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessionID
}

// Bounds returns the current terminal region this pane occupies.
func (m *Model) Bounds() coordinate.Rect {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bounds
}
