// Package splitlayout provides a bubbletea v2 tea.Model that composites
// multiple termmux terminal sessions into a single View using the Compositor
// and FocusGroup packages. Each pane is a lightweight struct tracking a
// session ID, bounds, and last generation — NOT a full termpane.Model.
//
// Pane content uses the ANSI capture (NOT the full-screen capture) to avoid CUP
// sequences that break compositing. Only the focused pane's cursor is rendered
// as a tea.Cursor; unfocused pane cursors are rendered as dim block characters
// within the cell content.
package splitlayout

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/compositor"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/focus"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// outputMsg wraps a termmux.Event delivered from the EventBus subscription
// goroutine to the bubbletea Update loop.
type outputMsg termmux.Event

// Pane tracks a single terminal session within the split layout.
type Pane struct {
	ID      termmux.SessionID
	Bounds  coordinate.Rect
	LastGen uint64
	// cursorRow/cursorCol/cursorVisible mirror the capture the pane content
	// came from, so View pairs content with its own generation's cursor
	// instead of a torn read from a newer snapshot.
	cursorRow     int
	cursorCol     int
	cursorVisible bool
}

// SplitLayout is a bubbletea v2 Model that composites multiple termmux
// terminal sessions into a single View with focus management and input routing.
type SplitLayout struct {
	mu sync.Mutex

	panes     []Pane
	ratios    []float64
	direction layout.Direction
	bounds    coordinate.Rect

	manager *termmux.SessionManager
	comp    *compositor.Compositor
	focus   *focus.FocusGroup

	// eventCh is the manager's shared broadcast channel. The bridge goroutine
	// is a registered subscriber: it receives, acknowledges with AckEvent
	// immediately, and only then filters and forwards.
	eventCh <-chan termmux.Event

	// paneIDs is a lock-free snapshot of the session IDs this layout renders,
	// for the bridge goroutine's filter. It MUST NOT take sl.mu: the mutex is
	// held across blocking manager IPC elsewhere (e.g. AddPane holds it while
	// ResizeSession waits on the worker), and the worker in turn waits on this
	// subscriber's acknowledgement — taking the lock inside the subscriber
	// loop closes that cycle into a deadlock. Mutations publish a fresh set.
	paneIDs atomic.Pointer[map[termmux.SessionID]struct{}]

	// outputWatch is the conflated any-session output signal owned by this
	// layout. Output is not a bus event (see internal/termmux/output_watch.go);
	// the layout refreshes every pane snapshot when it fires. Each consumer
	// owns its own watcher slot, so no other consumer can starve this one.
	outputWatch *termmux.OutputWatcher
	outputCh    chan termmux.Event
	done        chan struct{}
	wg          sync.WaitGroup

	closed         bool
	outputChClosed bool
	// subscribed reports whether Init has taken the manager event
	// subscription. Close unsubscribes only when it has: the bus counts
	// subscribers with a single integer and treats an unpaired decrement as
	// misuse, so a Close without a preceding Init must not decrement.
	subscribed bool
}

// SplitLayoutOption configures a SplitLayout.
type SplitLayoutOption interface {
	applySplitLayoutOption(cfg *splitLayoutConfig) error
}

type splitLayoutConfig struct {
	direction layout.Direction
	ratios    []float64
}

// DirectionOption sets the split direction.
type DirectionOption struct {
	direction layout.Direction
}

func WithDirection(d layout.Direction) *DirectionOption {
	return &DirectionOption{direction: d}
}

func (o *DirectionOption) applySplitLayoutOption(cfg *splitLayoutConfig) error {
	cfg.direction = o.direction
	return nil
}

var _ SplitLayoutOption = (*DirectionOption)(nil)

// RatiosOption sets the pane size ratios.
type RatiosOption struct {
	ratios []float64
}

func WithRatios(ratios []float64) *RatiosOption {
	return &RatiosOption{ratios: ratios}
}

func (o *RatiosOption) applySplitLayoutOption(cfg *splitLayoutConfig) error {
	cp := make([]float64, len(o.ratios))
	copy(cp, o.ratios)
	cfg.ratios = cp
	return nil
}

var _ SplitLayoutOption = (*RatiosOption)(nil)

// NewSplitLayout creates a SplitLayout with the given SessionManager and
// bounds. Panes are added via AddPane. Default direction is Horizontal with
// equal ratios.
func NewSplitLayout(manager *termmux.SessionManager, bounds coordinate.Rect, opts ...SplitLayoutOption) *SplitLayout {
	cfg := splitLayoutConfig{
		direction: layout.Horizontal,
	}

	for _, o := range opts {
		if err := o.applySplitLayoutOption(&cfg); err != nil {
			slog.Error("splitlayout option failed", "error", err)
		}
	}

	sl := &SplitLayout{
		manager:   manager,
		comp:      compositor.NewCompositor(bounds.Size.Width, bounds.Size.Height),
		focus:     focus.NewFocusGroup(),
		bounds:    bounds,
		direction: cfg.direction,
		ratios:    cfg.ratios,
		outputCh:  make(chan termmux.Event, 64),
		done:      make(chan struct{}),
	}

	return sl
}

// AddPane appends a pane for the given session ID, recalculates layout, and
// adds the pane to the compositor and focus group. Chainable.
func (sl *SplitLayout) AddPane(id termmux.SessionID) *SplitLayout {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	pane := Pane{ID: id}
	sl.panes = append(sl.panes, pane)
	sl.publishPaneIDsLocked()

	sl.recomputeLayoutLocked()

	// Add pane to compositor at Z=0.
	sl.comp.AddPane(sessionIDStr(id), "", pane.Bounds, 0)

	// Add to focus group.
	sl.focus.Add(focus.Focusable{
		ID:     sessionIDStr(id),
		Bounds: pane.Bounds,
	})

	// Resize the session to match its pane bounds.
	if err := sl.manager.ResizeSession(id, pane.Bounds.Size.Height, pane.Bounds.Size.Width); err != nil {
		slog.Debug("splitlayout resize session on add failed", "sessionID", id, "error", err)
	}

	return sl
}

// RemovePane removes the pane for the given session ID, recalculates layout.
// Returns an error if no pane with that ID exists.
func (sl *SplitLayout) RemovePane(id termmux.SessionID) error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	idx := -1
	for i, p := range sl.panes {
		if p.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("splitlayout: pane %d not found", id)
	}

	sl.panes = append(sl.panes[:idx], sl.panes[idx+1:]...)
	sl.publishPaneIDsLocked()
	sl.comp.RemovePane(sessionIDStr(id))
	sl.focus.Remove(sessionIDStr(id))

	sl.recomputeLayoutLocked()

	return nil
}

// publishPaneIDsLocked republishes the lock-free pane-ID snapshot for the
// bridge goroutine. The caller must hold sl.mu. The set is replaced wholesale
// rather than mutated, so a reader holding the old pointer always sees a
// consistent set.
func (sl *SplitLayout) publishPaneIDsLocked() {
	set := make(map[termmux.SessionID]struct{}, len(sl.panes))
	for _, p := range sl.panes {
		set[p.ID] = struct{}{}
	}
	sl.paneIDs.Store(&set)
}

// Panes returns the session IDs of all panes in order.
func (sl *SplitLayout) Panes() []termmux.SessionID {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	ids := make([]termmux.SessionID, len(sl.panes))
	for i, p := range sl.panes {
		ids[i] = p.ID
	}
	return ids
}

// PaneBounds returns the computed bounds for the pane with the given session
// ID. Returns an error if no pane with that ID exists.
func (sl *SplitLayout) PaneBounds(id termmux.SessionID) (coordinate.Rect, error) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for _, p := range sl.panes {
		if p.ID == id {
			return p.Bounds, nil
		}
	}
	return coordinate.Rect{}, fmt.Errorf("splitlayout: pane %d not found", id)
}

// recomputeLayoutLocked recalculates pane bounds from the current direction
// and ratios. Must be called with mu held.
func (sl *SplitLayout) recomputeLayoutLocked() {
	if len(sl.panes) == 0 {
		return
	}

	// Build ratios: if count doesn't match panes, default to equal.
	ratios := sl.ratios
	if len(ratios) != len(sl.panes) {
		n := len(sl.panes)
		ratios = make([]float64, n)
		v := 1.0 / float64(n)
		for i := range ratios {
			ratios[i] = v
		}
	}

	rects := layout.Split(sl.bounds, sl.direction, ratios)

	// Update each pane's bounds and the compositor/focus group.
	for i, p := range sl.panes {
		sl.panes[i].Bounds = rects[i]

		// Update compositor pane position.
		sl.comp.AddPane(sessionIDStr(p.ID), "", rects[i], 0)

		// Update focus group bounds.
		sl.focus.SetBounds(sessionIDStr(p.ID), rects[i])

		// Resize the session to match its new bounds.
		if err := sl.manager.ResizeSession(p.ID, rects[i].Size.Height, rects[i].Size.Width); err != nil {
			slog.Debug("splitlayout resize session on recompute failed", "sessionID", p.ID, "error", err)
		}
	}

	// Resize compositor canvas.
	sl.comp.Resize(sl.bounds.Size.Width, sl.bounds.Size.Height)

	// Generate border chrome between panes.
	sl.generateBordersLocked()
}

// generateBordersLocked creates chrome layers for borders between panes.
// Must be called with mu held.
func (sl *SplitLayout) generateBordersLocked() {
	// Remove existing border chrome.
	for i := 0; i < len(sl.panes)+1; i++ {
		sl.comp.RemoveChrome(borderChromeID(i))
	}

	if len(sl.panes) < 2 {
		return
	}

	height := sl.bounds.Size.Height
	width := sl.bounds.Size.Width
	focusedIdx := sl.focus.ActiveIndex()

	focusedBorderColor := lipgloss.Color("12") // bright blue
	dimBorderColor := lipgloss.Color("8")      // grey

	switch sl.direction {
	case layout.Horizontal:
		// Vertical border lines between horizontal panes.
		for i := 0; i < len(sl.panes)-1; i++ {
			// Border goes at the right edge of pane i.
			borderX := sl.panes[i].Bounds.Position.X + sl.panes[i].Bounds.Size.Width
			// If border would be at the very right edge of the layout, skip.
			if borderX >= sl.bounds.Position.X+width {
				continue
			}
			content := strings.Repeat("│\n", height)
			content = strings.TrimSuffix(content, "\n")
			bounds := coordinate.Rect{
				Position: coordinate.Position{X: borderX, Y: sl.bounds.Position.Y},
				Size:     coordinate.Size{Width: 1, Height: height},
			}
			// Highlight border if adjacent to focused pane.
			color := dimBorderColor
			if i == focusedIdx || i+1 == focusedIdx {
				color = focusedBorderColor
			}
			styled := lipgloss.NewStyle().Foreground(color).Render(content)
			sl.comp.AddChrome(borderChromeID(i), styled, bounds, 1)
		}
	case layout.Vertical:
		// Horizontal border lines between vertical panes.
		for i := 0; i < len(sl.panes)-1; i++ {
			borderY := sl.panes[i].Bounds.Position.Y + sl.panes[i].Bounds.Size.Height
			if borderY >= sl.bounds.Position.Y+height {
				continue
			}
			content := strings.Repeat("─", width)
			bounds := coordinate.Rect{
				Position: coordinate.Position{X: sl.bounds.Position.X, Y: borderY},
				Size:     coordinate.Size{Width: width, Height: 1},
			}
			color := dimBorderColor
			if i == focusedIdx || i+1 == focusedIdx {
				color = focusedBorderColor
			}
			styled := lipgloss.NewStyle().Foreground(color).Render(content)
			sl.comp.AddChrome(borderChromeID(i), styled, bounds, 1)
		}
	}
}

// Init implements tea.Model. It subscribes to the manager's EventBus, starts
// the bridge goroutine, and returns the waitForOutput cmd.
func (sl *SplitLayout) Init() tea.Cmd {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Guard against Init being called more than once: the bus counts
	// subscribers, so a second SubscribeEvents would need a second Close to
	// balance, and a second bridge goroutine would compete for the same
	// subscription.
	if sl.subscribed {
		return sl.waitForOutput
	}
	sl.subscribed = true

	// Publish the initial pane set before the bridge starts filtering.
	// sl.mu is already held here (deferred unlock at Init entry).
	sl.publishPaneIDsLocked()

	// Prompt subscriber: every received event is acknowledged with AckEvent
	// before filtering, so the manager worker is never stalled.
	sl.eventCh = sl.manager.SubscribeEvents()

	// Output arrives as a conflated wake-up rather than a bus event.
	sl.outputWatch = sl.manager.WatchAnyOutput()

	sl.wg.Add(1)
	go sl.bridgeEvents()

	sl.wg.Add(1)
	go sl.watchOutput()

	return sl.waitForOutput
}

// bridgeEvents is the prompt subscriber loop for this layout. The manager's
// broadcast channel is shared and unbuffered with one synchronous send per
// registered subscriber, so the sequence per value is strictly:
//
//	receive → AckEvent → filter/forward downstream
//
// AckEvent must precede the session-set lookup and the downstream send, because
// the publisher is blocked until this subscriber acknowledges.
//
// The loop exits on Close (done) or bus close, and never receives after
// Unsubscribe: Close stops it before decrementing the subscriber count.
func (sl *SplitLayout) bridgeEvents() {
	defer sl.wg.Done()

	for {
		select {
		case <-sl.done:
			return
		case evt, ok := <-sl.eventCh:
			if !ok {
				// Bus closed by the manager. Must NOT acknowledge a close.
				return
			}
			// Acknowledge before doing anything else — the publisher is
			// waiting on exactly this call.
			sl.manager.AckEvent()

			// Filter: only forward events for our sessions (or global
			// events). The set is read lock-free: taking sl.mu here would
			// deadlock against callers that hold it across manager IPC
			// (e.g. AddPane → ResizeSession → worker → this subscriber).
			if evt.SessionID != 0 {
				ids := sl.paneIDs.Load()
				if ids == nil {
					continue
				}
				if _, ok := (*ids)[evt.SessionID]; !ok {
					continue
				}
			}
			// Forward, but never at the cost of the subscriber cycle: a tea
			// consumer that stops draining outputCh must not stall the
			// manager worker. A dropped forward is harmless — the next
			// delivered event re-triggers refreshPanes from the snapshots.
			select {
			case sl.outputCh <- evt:
			case <-sl.done:
				return
			default:
			}
		}
	}
}

// watchOutput forwards conflated output wake-ups into the same outputMsg path
// the bus uses, so pane snapshots refresh on output without output travelling
// the lossless bus. The layout refreshes every pane on any wake-up, which is
// correct because refreshPanes re-reads each session's snapshot.
func (sl *SplitLayout) watchOutput() {
	defer sl.wg.Done()
	if sl.outputWatch == nil {
		return
	}
	wake := sl.outputWatch.C()
	if wake == nil {
		return
	}
	for {
		select {
		case <-sl.done:
			return
		case _, ok := <-wake:
			if !ok {
				return
			}
			// Any session's output refreshes all panes; the message's
			// session is therefore immaterial, and 0 denotes "global".
			select {
			case sl.outputCh <- termmux.Event{Kind: termmux.EventBell}:
			case <-sl.done:
				return
			}
		}
	}
}

// waitForOutput is a tea.Cmd that blocks until an event arrives on outputCh
// or the done channel is closed.
func (sl *SplitLayout) waitForOutput() tea.Msg {
	select {
	case evt, ok := <-sl.outputCh:
		if !ok {
			return nil
		}
		return outputMsg(evt)
	case <-sl.done:
		return nil
	}
}

// Update implements tea.Model.
func (sl *SplitLayout) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case outputMsg:
		// Refresh ALL pane snapshots from manager.
		sl.refreshPanes()
		return sl, sl.waitForOutput

	case tea.WindowSizeMsg:
		sl.mu.Lock()
		sl.bounds.Size.Width = msg.Width
		sl.bounds.Size.Height = msg.Height
		sl.recomputeLayoutLocked()
		sl.mu.Unlock()
		return sl, sl.waitForOutput

	case tea.KeyPressMsg:
		return sl, sl.handleKey(msg)

	case tea.MouseMsg:
		return sl, sl.handleMouse(msg)

	case tea.QuitMsg:
		sl.Close()
		return sl, tea.Quit

	default:
		return sl, nil
	}
}

// handleKey processes a key press. Tab/Shift+Tab cycle focus; other keys are
// forwarded to the focused pane's session.
func (sl *SplitLayout) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.Key()

	// Tab cycles focus forward.
	if key.Code == tea.KeyTab && key.Mod == 0 {
		sl.focus.Next()
		return nil
	}

	// Shift+Tab cycles focus backward.
	if key.Code == tea.KeyTab && key.Mod.Contains(tea.ModShift) {
		sl.focus.Prev()
		return nil
	}

	// Forward to focused pane's session.
	active := sl.focus.Active()
	if active.ID == "" {
		return nil
	}

	sl.mu.Lock()
	sessionID := paneIDFromStr(active.ID)
	sl.mu.Unlock()

	if sessionID == 0 {
		return nil
	}

	// Convert key to terminal bytes.
	keyStr := msg.String()
	appCursor := false
	appKeypad := false
	if snap := sl.manager.Snapshot(sessionID); snap != nil {
		appCursor = snap.ApplicationCursor
		appKeypad = snap.KeypadApplication
	}
	seq, ok := termmux.KeyToTermBytes(keyStr, appCursor, appKeypad)
	if !ok {
		if key.Text != "" {
			seq = key.Text
		} else {
			slog.Debug("splitlayout unrecognized key", "key", keyStr)
			return nil
		}
	}

	if err := sl.manager.Input([]byte(seq)); err != nil {
		slog.Debug("splitlayout key forward failed", "key", keyStr, "error", err)
	}

	return nil
}

// handleMouse processes a mouse event. Clicks switch focus; all mouse events
// are forwarded to the focused pane's session with coordinate offset.
func (sl *SplitLayout) handleMouse(msg tea.MouseMsg) tea.Cmd {
	mouse := msg.Mouse()

	// Hit test to switch focus on click.
	if _, isClick := msg.(tea.MouseClickMsg); isClick {
		if item, hit := sl.focus.HitTest(mouse.X, mouse.Y); hit {
			sl.focus.Focus(item.ID)
		}
	}

	// Forward to focused pane's session.
	active := sl.focus.Active()
	if active.ID == "" {
		return nil
	}

	sl.mu.Lock()
	sessionID := paneIDFromStr(active.ID)
	offsetRow := active.Bounds.Position.Y
	offsetCol := active.Bounds.Position.X
	sl.mu.Unlock()

	if sessionID == 0 {
		return nil
	}

	evt := termmux.MouseEvent{
		X:     mouse.X,
		Y:     mouse.Y,
		Shift: mouse.Mod.Contains(tea.ModShift),
		Alt:   mouse.Mod.Contains(tea.ModAlt),
		Ctrl:  mouse.Mod.Contains(tea.ModCtrl),
	}

	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		evt.Type = termmux.MouseClick
		evt.Button = termmux.MouseButtonTea(msg.Button)
	case tea.MouseReleaseMsg:
		evt.Type = termmux.MouseRelease
		evt.Button = termmux.MouseButtonTea(msg.Button)
	case tea.MouseMotionMsg:
		evt.Type = termmux.MouseMotion
		evt.Button = termmux.MouseButtonTea(msg.Button)
	case tea.MouseWheelMsg:
		evt.Type = termmux.MouseWheel
		evt.Button = termmux.MouseButtonTea(msg.Button)
	default:
		return nil
	}

	seq, ok := termmux.MouseToSGR(evt, offsetRow, offsetCol)
	if !ok {
		return nil
	}

	if err := sl.manager.Input([]byte(seq)); err != nil {
		slog.Debug("splitlayout mouse forward failed", "error", err)
	}

	return nil
}

// capturePane fetches one ANSI capture and records its content plus cursor
// state on the pane. Content and cursor always come from the same capture,
// so frames are never torn across generations.
func (sl *SplitLayout) capturePane(i int) {
	// Use ANSI (NOT FullScreen) — FullScreen has CUP sequences that break compositing.
	capture, err := sl.manager.CaptureScreen(sl.panes[i].ID, termmux.CaptureOptions{Kind: termmux.CaptureANSI})
	if err != nil {
		return
	}
	sl.comp.UpdatePaneIfNew(sessionIDStr(sl.panes[i].ID), capture.Text, capture.Snapshot.Gen)
	sl.panes[i].LastGen = capture.Snapshot.Gen
	sl.panes[i].cursorRow = capture.Snapshot.CursorRow
	sl.panes[i].cursorCol = capture.Snapshot.CursorCol
	sl.panes[i].cursorVisible = capture.Snapshot.CursorVisible
}

// refreshPanes updates all pane snapshots from the manager and pushes them
// into the compositor.
func (sl *SplitLayout) refreshPanes() {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for i := range sl.panes {
		sl.capturePane(i)
	}
}

// capturePaneIfStale re-fetches pane i only when the manager holds a newer
// generation than the pane last rendered. The Gen probe is itself one cheap
// Snapshot round-trip per pane per View; only the heavier CaptureScreen
// render is skipped when nothing changed.
func (sl *SplitLayout) capturePaneIfStale(i int) {
	snap := sl.manager.Snapshot(sl.panes[i].ID)
	if snap == nil {
		return
	}
	if snap.Gen == sl.panes[i].LastGen {
		return
	}
	sl.capturePane(i)
}

// View implements tea.Model. It renders all pane content through the
// compositor and sets the cursor for the focused pane only.
func (sl *SplitLayout) View() tea.View {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Push only stale panes into the compositor; unchanged generations keep
	// the refreshPanes result (one Gen-probe Snapshot per pane, no re-render).
	for i := range sl.panes {
		sl.capturePaneIfStale(i)
	}

	rendered := sl.comp.Render()

	v := tea.NewView(rendered)

	// Set cursor for focused pane only.
	active := sl.focus.Active()
	if active.ID == "" {
		return v
	}

	sessionID := paneIDFromStr(active.ID)
	if sessionID == 0 {
		return v
	}

	var cursorRow, cursorCol int
	var cursorVisible bool
	found := false
	for i := range sl.panes {
		if sl.panes[i].ID == sessionID {
			b := sl.panes[i].Bounds
			cursorRow = sl.panes[i].cursorRow + b.Position.Y
			cursorCol = sl.panes[i].cursorCol + b.Position.X
			// The cursor shows only when the child left it visible and its
			// position falls within the pane's bounds.
			cursorVisible = sl.panes[i].cursorVisible &&
				cursorRow >= b.Position.Y &&
				cursorRow < b.Position.Y+b.Size.Height &&
				cursorCol >= b.Position.X &&
				cursorCol < b.Position.X+b.Size.Width
			found = true
			break
		}
	}
	if !found {
		return v
	}

	if cursorVisible {
		v.Cursor = tea.NewCursor(cursorCol, cursorRow)
	}

	return v
}

// Close unsubscribes from the EventBus, signals the bridge goroutine, waits
// for it to finish, and closes outputCh. Idempotent.
func (sl *SplitLayout) Close() error {
	sl.mu.Lock()
	if sl.closed {
		sl.mu.Unlock()
		return nil
	}
	sl.closed = true
	close(sl.done)
	subscribed := sl.subscribed
	sl.mu.Unlock()

	// Join the bridge goroutine BEFORE releasing the subscription, and do not
	// reorder these. The shared broadcast channel's unsubscribe path drains an
	// in-flight send on the departing subscriber's behalf, so a goroutine still
	// parked in a receive races that drain and can steal the value, stranding
	// the acknowledgement accounting.
	sl.wg.Wait()
	if sl.outputWatch != nil {
		sl.outputWatch.Release()
		sl.outputWatch = nil
	}
	if subscribed {
		sl.manager.UnsubscribeEvents()
	}

	sl.mu.Lock()
	defer sl.mu.Unlock()
	if !sl.outputChClosed {
		sl.outputChClosed = true
		close(sl.outputCh)
	}

	return nil
}

// FocusPane focuses the pane with the given session ID. Returns false if no
// pane with that ID exists.
func (sl *SplitLayout) FocusPane(id termmux.SessionID) bool {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	return sl.focus.Focus(sessionIDStr(id))
}

// SetDirection sets the layout direction and recomputes pane bounds.
func (sl *SplitLayout) SetDirection(d layout.Direction) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	sl.direction = d
	sl.recomputeLayoutLocked()
}

// SetRatios sets the pane size ratios and recomputes pane bounds.
func (sl *SplitLayout) SetRatios(ratios []float64) {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	sl.ratios = make([]float64, len(ratios))
	copy(sl.ratios, ratios)
	sl.recomputeLayoutLocked()
}

// sessionIDStr converts a termmux.SessionID to a string for use as
// compositor and focus group identifiers.
func sessionIDStr(id termmux.SessionID) string {
	return fmt.Sprintf("pane-%d", id)
}

// paneIDFromStr converts a compositor/focus identifier back to a
// termmux.SessionID. Returns 0 if the format is invalid.
func paneIDFromStr(s string) termmux.SessionID {
	var id uint64
	if _, err := fmt.Sscanf(s, "pane-%d", &id); err != nil {
		return 0
	}
	return termmux.SessionID(id)
}

// borderChromeID returns the chrome layer ID for the i-th border.
func borderChromeID(i int) string {
	return fmt.Sprintf("border-%d", i)
}
