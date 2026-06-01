// Package splitlayout provides a bubbletea v2 tea.Model that composites
// multiple termmux terminal sessions into a single View using the Compositor
// and FocusGroup packages. Each pane is a lightweight struct tracking a
// session ID, bounds, and last generation — NOT a full termpane.Model.
//
// Pane content uses snap.ANSI (NOT FullScreen) to avoid CUP sequences that
// break compositing. Only the focused pane's cursor is rendered as a
// tea.Cursor; unfocused pane cursors are rendered as dim block characters
// within the cell content.
package splitlayout

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

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
}

// SplitLayout is a bubbletea v2 Model that composites multiple termmux
// terminal sessions into a single View with focus management and input routing.
type SplitLayout struct {
	mu sync.Mutex

	panes     []Pane
	ratios    []float64
	direction layout.Direction
	bounds    coordinate.Rect

	manager  *termmux.SessionManager
	comp     *compositor.Compositor
	focus    *focus.FocusGroup

	subID   int
	eventCh <-chan termmux.Event
	outputCh chan termmux.Event
	done    chan struct{}
	wg      sync.WaitGroup

	closed bool
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
	sl.comp.RemovePane(sessionIDStr(id))
	sl.focus.Remove(sessionIDStr(id))

	sl.recomputeLayoutLocked()

	return nil
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
			// Trim trailing newline.
			content = strings.TrimSuffix(content, "\n")
			bounds := coordinate.Rect{
				Position: coordinate.Position{X: borderX, Y: sl.bounds.Position.Y},
				Size:     coordinate.Size{Width: 1, Height: height},
			}
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(content)
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
			styled := lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(content)
			sl.comp.AddChrome(borderChromeID(i), styled, bounds, 1)
		}
	}
}

// Init implements tea.Model. It subscribes to the manager's EventBus, starts
// the bridge goroutine, and returns the waitForOutput cmd.
func (sl *SplitLayout) Init() tea.Cmd {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	sl.subID, sl.eventCh = sl.manager.Subscribe(64)

	sl.wg.Add(1)
	go sl.bridgeEvents()

	return sl.waitForOutput
}

// bridgeEvents reads events from the EventBus subscription channel and
// forwards those matching any of our sessions to outputCh.
func (sl *SplitLayout) bridgeEvents() {
	defer sl.wg.Done()

	// Build set of session IDs we care about.
	sessionIDs := make(map[termmux.SessionID]bool)
	sl.mu.Lock()
	for _, p := range sl.panes {
		sessionIDs[p.ID] = true
	}
	sl.mu.Unlock()

	for {
		select {
		case <-sl.done:
			return
		case evt, ok := <-sl.eventCh:
			if !ok {
				return
			}
			// Filter: only forward events for our sessions (or global events).
			if evt.SessionID != 0 {
				sl.mu.Lock()
				ids := make(map[termmux.SessionID]bool)
				for _, p := range sl.panes {
					ids[p.ID] = true
				}
				sl.mu.Unlock()

				if !ids[evt.SessionID] {
					continue
				}
			}
			select {
			case sl.outputCh <- evt:
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
	seq, ok := termmux.KeyToTermBytes(keyStr)
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

// refreshPanes updates all pane snapshots from the manager and pushes them
// into the compositor.
func (sl *SplitLayout) refreshPanes() {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	for i := range sl.panes {
		snap := sl.manager.Snapshot(sl.panes[i].ID)
		if snap == nil {
			continue
		}
		// Use ANSI (NOT FullScreen) — FullScreen has CUP sequences that break compositing.
		sl.comp.UpdatePaneIfNew(sessionIDStr(sl.panes[i].ID), snap.ANSI, snap.Gen)
		sl.panes[i].LastGen = snap.Gen
	}
}

// View implements tea.Model. It renders all pane content through the
// compositor and sets the cursor for the focused pane only.
func (sl *SplitLayout) View() tea.View {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	// Push latest snapshots into compositor.
	for i := range sl.panes {
		snap := sl.manager.Snapshot(sl.panes[i].ID)
		if snap == nil {
			continue
		}
		sl.comp.UpdatePaneIfNew(sessionIDStr(sl.panes[i].ID), snap.ANSI, snap.Gen)
		sl.panes[i].LastGen = snap.Gen
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

	snap := sl.manager.Snapshot(sessionID)
	if snap == nil {
		return v
	}

	cursorRow := snap.CursorRow + active.Bounds.Position.Y
	cursorCol := snap.CursorCol + active.Bounds.Position.X

	// Cursor is visible only if it falls within the pane's bounds.
	cursorVisible := cursorRow >= active.Bounds.Position.Y &&
		cursorRow < active.Bounds.Position.Y+active.Bounds.Size.Height &&
		cursorCol >= active.Bounds.Position.X &&
		cursorCol < active.Bounds.Position.X+active.Bounds.Size.Width

	if cursorVisible {
		v.Cursor = tea.NewCursor(cursorCol, cursorRow)
	}

	return v
}

// Close unsubscribes from the EventBus, signals the bridge goroutine, waits
// for it to finish, and closes outputCh. Idempotent.
func (sl *SplitLayout) Close() error {
	sl.mu.Lock()
	defer sl.mu.Unlock()

	if sl.closed {
		return nil
	}
	sl.closed = true

	close(sl.done)

	sl.manager.Unsubscribe(sl.subID)

	sl.wg.Wait()

	close(sl.outputCh)

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
