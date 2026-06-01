package splitlayout

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/focus"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// controllableSession is a test double for termmux.InteractiveSession.
type controllableSession struct {
	writeMu     sync.Mutex
	writtenData []byte
	writeErr    error
	resizeCalls []struct{ rows, cols int }
	closeCalled bool
	doneCh      chan struct{}
	readerCh    chan []byte
}

func newControllableSession() *controllableSession {
	return &controllableSession{
		doneCh:   make(chan struct{}),
		readerCh: make(chan []byte, 64),
	}
}

func (s *controllableSession) Done() <-chan struct{} { return s.doneCh }
func (s *controllableSession) Reader() <-chan []byte { return s.readerCh }
func (s *controllableSession) Close() error {
	s.closeCalled = true
	select {
	case <-s.doneCh:
	default:
		close(s.doneCh)
	}
	return nil
}

func (s *controllableSession) Write(data []byte) (int, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	s.writtenData = append(s.writtenData, data...)
	return len(data), nil
}

func (s *controllableSession) Resize(rows, cols int) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.resizeCalls = append(s.resizeCalls, struct{ rows, cols int }{rows, cols})
	return nil
}

func (s *controllableSession) Written() []byte {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]byte, len(s.writtenData))
	copy(cp, s.writtenData)
	return cp
}

func (s *controllableSession) Resizes() []struct{ rows, cols int } {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	cp := make([]struct{ rows, cols int }, len(s.resizeCalls))
	copy(cp, s.resizeCalls)
	return cp
}

// startTestManager creates a SessionManager with registered controllable
// sessions, starts the worker, and returns everything needed for testing.
func startTestManager(t *testing.T, sessionCount int) (*termmux.SessionManager, []*controllableSession, []termmux.SessionID, func()) {
	t.Helper()

	m := termmux.NewSessionManager(termmux.WithTermSize(24, 80))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	sessions := make([]*controllableSession, sessionCount)
	ids := make([]termmux.SessionID, sessionCount)

	for i := 0; i < sessionCount; i++ {
		session := newControllableSession()
		id, err := m.Register(session, termmux.SessionTarget{
			Name: fmt.Sprintf("test-%d", i),
			Kind: termmux.SessionKindPTY,
		})
		require.NoError(t, err, "Register session %d", i)
		sessions[i] = session
		ids[i] = id

		// Pump output to transition to Running state.
		session.readerCh <- []byte("ready")
		deadline := time.After(2 * time.Second)
		for {
			snap := m.Snapshot(id)
			if snap != nil && strings.Contains(snap.PlainText, "ready") {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("timed out waiting for session %d to reach Running state", i)
			case <-time.After(10 * time.Millisecond):
			}
		}
	}

	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, sessions, ids, cleanup
}

// --- Layout Tests (no SessionManager needed) ---

func TestSplitLayout_New(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, _, cleanup := startTestManager(t, 0)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	assert.Empty(t, sl.Panes(), "new SplitLayout should have no panes")
	assert.Equal(t, layout.Horizontal, sl.direction, "default direction should be Horizontal")
}

func TestSplitLayout_AddPane(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	panes := sl.Panes()
	assert.Len(t, panes, 2, "should have 2 panes after adding 2")
	assert.Equal(t, ids[0], panes[0])
	assert.Equal(t, ids[1], panes[1])
}

func TestSplitLayout_RemovePane(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	err := sl.RemovePane(ids[0])
	assert.NoError(t, err, "RemovePane should succeed for existing pane")

	panes := sl.Panes()
	assert.Len(t, panes, 1, "should have 1 pane after removing one")
	assert.Equal(t, ids[1], panes[0])

	// Remove non-existent pane.
	err = sl.RemovePane(999)
	assert.Error(t, err, "RemovePane should error for non-existent pane")
}

func TestSplitLayout_Layout_Horizontal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds, WithDirection(layout.Horizontal))
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	b0, err := sl.PaneBounds(ids[0])
	require.NoError(t, err)

	b1, err := sl.PaneBounds(ids[1])
	require.NoError(t, err)

	// Horizontal split: panes share full height, split width.
	assert.Equal(t, 24, b0.Size.Height, "pane 0 height should be full height")
	assert.Equal(t, 24, b1.Size.Height, "pane 1 height should be full height")
	assert.Equal(t, 40, b0.Size.Width, "pane 0 width should be half (80/2)")
	assert.Equal(t, 40, b1.Size.Width, "pane 1 width should be half (80/2)")
	assert.Equal(t, 0, b0.Position.X, "pane 0 should start at x=0")
	assert.Equal(t, 40, b1.Position.X, "pane 1 should start at x=40")
}

func TestSplitLayout_Layout_Vertical(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds, WithDirection(layout.Vertical))
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	b0, err := sl.PaneBounds(ids[0])
	require.NoError(t, err)

	b1, err := sl.PaneBounds(ids[1])
	require.NoError(t, err)

	// Vertical split: panes share full width, split height.
	assert.Equal(t, 80, b0.Size.Width, "pane 0 width should be full width")
	assert.Equal(t, 80, b1.Size.Width, "pane 1 width should be full width")
	assert.Equal(t, 12, b0.Size.Height, "pane 0 height should be half (24/2)")
	assert.Equal(t, 12, b1.Size.Height, "pane 1 height should be half (24/2)")
	assert.Equal(t, 0, b0.Position.Y, "pane 0 should start at y=0")
	assert.Equal(t, 12, b1.Position.Y, "pane 1 should start at y=12")
}

func TestSplitLayout_Layout_Ratios(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds,
		WithDirection(layout.Horizontal),
		WithRatios([]float64{0.75, 0.25}),
	)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	b0, err := sl.PaneBounds(ids[0])
	require.NoError(t, err)

	b1, err := sl.PaneBounds(ids[1])
	require.NoError(t, err)

	// 75/25 split of 80 columns: 60 + 20.
	assert.Equal(t, 60, b0.Size.Width, "pane 0 should get 75%% of width")
	assert.Equal(t, 20, b1.Size.Width, "pane 1 should get 25%% of width")
}

func TestSplitLayout_Layout_ThreePanes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 3)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 90, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds, WithDirection(layout.Horizontal))
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])
	sl.AddPane(ids[2])

	// Equal split of 90 columns: 30 + 30 + 30.
	for i, id := range ids {
		b, err := sl.PaneBounds(id)
		require.NoError(t, err)
		assert.Equal(t, 30, b.Size.Width, "pane %d width should be 30", i)
		assert.Equal(t, i*30, b.Position.X, "pane %d should start at x=%d", i, i*30)
	}
}

// --- Focus Tests ---

func TestSplitLayout_FocusCycling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// Initially, first pane should be focused.
	active := sl.focus.Active()
	assert.Equal(t, sessionIDStr(ids[0]), active.ID, "first pane should be focused initially")

	// Tab cycles to next pane.
	_, cmd := sl.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	assert.Nil(t, cmd, "Tab should not produce a cmd (consumed)")
	active = sl.focus.Active()
	assert.Equal(t, sessionIDStr(ids[1]), active.ID, "Tab should cycle to second pane")

	// Tab wraps around.
	sl.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	active = sl.focus.Active()
	assert.Equal(t, sessionIDStr(ids[0]), active.ID, "Tab should wrap to first pane")

	// Shift+Tab cycles backward.
	sl.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	active = sl.focus.Active()
	assert.Equal(t, sessionIDStr(ids[1]), active.ID, "Shift+Tab should cycle backward to second pane")
}

func TestSplitLayout_MouseFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// Initially first pane focused.
	assert.Equal(t, sessionIDStr(ids[0]), sl.focus.Active().ID)

	// Click in second pane (x=50, y=5 — within pane 1's bounds at x=40..80).
	clickMsg := tea.MouseClickMsg{
		X:      50,
		Y:      5,
		Button: tea.MouseLeft,
	}
	sl.Update(clickMsg)

	active := sl.focus.Active()
	assert.Equal(t, sessionIDStr(ids[1]), active.ID, "click in pane 1 should switch focus")

	// Click in first pane (x=10, y=5 — within pane 0's bounds at x=0..40).
	clickMsg = tea.MouseClickMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseLeft,
	}
	sl.Update(clickMsg)

	active = sl.focus.Active()
	assert.Equal(t, sessionIDStr(ids[0]), active.ID, "click in pane 0 should switch focus back")
}

// --- View Tests ---

func TestSplitLayout_View_EmptyPanes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, _, cleanup := startTestManager(t, 0)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	v := sl.View()
	assert.NotNil(t, v, "View should return non-nil tea.View")
}

func TestSplitLayout_View_WithPanes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, sessions, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// Produce output in first session.
	sessions[0].readerCh <- []byte("hello from pane 0")

	// Wait for snapshot to update.
	deadline := time.After(2 * time.Second)
	for {
		snap := mgr.Snapshot(ids[0])
		if snap != nil && strings.Contains(snap.PlainText, "hello") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for pane 0 snapshot")
		case <-time.After(10 * time.Millisecond):
		}
	}

	v := sl.View()
	assert.NotEmpty(t, v.Content, "View should have content when panes have output")
}

func TestSplitLayout_View_CursorFocused(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// First pane is focused by default. Its cursor should appear in the View.
	// The cursor position is determined by the snapshot's cursor position
	// offset by the pane's bounds position.
	v := sl.View()
	// With default snapshot, cursor is at (0,0) in the session, offset by
	// pane 0's position (0,0), so cursor should be at (0,0).
	if v.Cursor != nil {
		// Cursor position should be within pane 0's bounds.
		assert.GreaterOrEqual(t, v.Cursor.Position.X, 0)
		assert.GreaterOrEqual(t, v.Cursor.Position.Y, 0)
	}
}

// --- Close Tests ---

func TestSplitLayout_Close(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 1)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	sl.AddPane(ids[0])

	// Init to start the bridge goroutine.
	sl.Init()

	err := sl.Close()
	assert.NoError(t, err, "first Close should succeed")

	// Idempotent close.
	err = sl.Close()
	assert.NoError(t, err, "second Close should also succeed")
}

func TestSplitLayout_Close_WithoutInit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, _, cleanup := startTestManager(t, 0)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)

	// Close without Init should not panic.
	err := sl.Close()
	assert.NoError(t, err, "Close without Init should succeed")
}

// --- Quit Tests ---

func TestSplitLayout_QuitMsg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 1)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	sl.AddPane(ids[0])
	sl.Init()

	_, cmd := sl.Update(tea.QuitMsg{})
	assert.NotNil(t, cmd, "QuitMsg should return tea.Quit cmd")

	sl.mu.Lock()
	closed := sl.closed
	sl.mu.Unlock()
	assert.True(t, closed, "SplitLayout should be closed after QuitMsg")
}

// --- WindowSize Tests ---

func TestSplitLayout_WindowSizeMsg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// Send WindowSizeMsg.
	sl.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	sl.mu.Lock()
	assert.Equal(t, 120, sl.bounds.Size.Width)
	assert.Equal(t, 40, sl.bounds.Size.Height)
	sl.mu.Unlock()

	// Verify pane bounds were recalculated.
	b0, err := sl.PaneBounds(ids[0])
	require.NoError(t, err)
	assert.Equal(t, 60, b0.Size.Width, "pane 0 should be half of 120")
	assert.Equal(t, 40, b0.Size.Height, "pane 0 height should be 40")
}

// --- Key Forwarding Tests ---

func TestSplitLayout_KeyForwarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, sessions, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// Activate the first session so Input goes to it.
	require.NoError(t, mgr.Activate(ids[0]))

	// Send a key press for "a".
	sl.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})

	// Verify the session received the input.
	written := sessions[0].Written()
	assert.NotEmpty(t, written, "session 0 should receive input after key press")
	assert.Contains(t, string(written), "a")
}

// --- Mouse Forwarding Tests ---

func TestSplitLayout_MouseForwarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, sessions, ids, cleanup := startTestManager(t, 2)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	sl := NewSplitLayout(mgr, bounds)
	defer sl.Close()

	sl.AddPane(ids[0])
	sl.AddPane(ids[1])

	// Activate the first session so Input goes to it.
	require.NoError(t, mgr.Activate(ids[0]))

	// Click in first pane (x=10, y=5).
	clickMsg := tea.MouseClickMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseLeft,
	}
	sl.Update(clickMsg)

	// Verify the session received SGR-encoded input.
	written := sessions[0].Written()
	assert.NotEmpty(t, written, "session 0 should receive input after mouse click")
	s := string(written)
	assert.True(t, strings.HasPrefix(s, "\x1b[<"), "expected SGR mouse sequence, got %q", s)
}

// --- Unit Tests (no SessionManager needed) ---

func TestSessionIDStr(t *testing.T) {
	assert.Equal(t, "pane-1", sessionIDStr(1))
	assert.Equal(t, "pane-42", sessionIDStr(42))
	assert.Equal(t, "pane-0", sessionIDStr(0))
}

func TestPaneIDFromStr(t *testing.T) {
	assert.Equal(t, termmux.SessionID(1), paneIDFromStr("pane-1"))
	assert.Equal(t, termmux.SessionID(42), paneIDFromStr("pane-42"))
	assert.Equal(t, termmux.SessionID(0), paneIDFromStr("pane-0"))
	assert.Equal(t, termmux.SessionID(0), paneIDFromStr("invalid"))
	assert.Equal(t, termmux.SessionID(0), paneIDFromStr(""))
}

func TestBorderChromeID(t *testing.T) {
	assert.Equal(t, "border-0", borderChromeID(0))
	assert.Equal(t, "border-1", borderChromeID(1))
}

func TestMouseButtonToTermmux(t *testing.T) {
	assert.Equal(t, termmux.MouseLeft, mouseButtonToTermmux(tea.MouseLeft))
	assert.Equal(t, termmux.MouseMiddle, mouseButtonToTermmux(tea.MouseMiddle))
	assert.Equal(t, termmux.MouseRight, mouseButtonToTermmux(tea.MouseRight))
	assert.Equal(t, termmux.MouseWheelUp, mouseButtonToTermmux(tea.MouseWheelUp))
	assert.Equal(t, termmux.MouseWheelDown, mouseButtonToTermmux(tea.MouseWheelDown))
	assert.Equal(t, termmux.MouseNone, mouseButtonToTermmux(tea.MouseNone))
}

// --- FocusGroup Integration Tests ---

func TestFocusGroup_HitTest_Horizontal(t *testing.T) {
	fg := focus.NewFocusGroup(
		focus.Focusable{
			ID:     "pane-1",
			Bounds: coordinate.Rect{Position: coordinate.Position{X: 0, Y: 0}, Size: coordinate.Size{Width: 40, Height: 24}},
		},
		focus.Focusable{
			ID:     "pane-2",
			Bounds: coordinate.Rect{Position: coordinate.Position{X: 40, Y: 0}, Size: coordinate.Size{Width: 40, Height: 24}},
		},
	)

	// Click in first pane.
	item, hit := fg.HitTest(10, 5)
	assert.True(t, hit)
	assert.Equal(t, "pane-1", item.ID)

	// Click in second pane.
	item, hit = fg.HitTest(50, 5)
	assert.True(t, hit)
	assert.Equal(t, "pane-2", item.ID)

	// Click outside both panes.
	_, hit = fg.HitTest(100, 5)
	assert.False(t, hit)
}

// --- Options Tests ---

func TestWithDirection(t *testing.T) {
	opt := WithDirection(layout.Vertical)
	cfg := splitLayoutConfig{direction: layout.Horizontal}
	err := opt.applySplitLayoutOption(&cfg)
	assert.NoError(t, err)
	assert.Equal(t, layout.Vertical, cfg.direction)
}

func TestWithRatios(t *testing.T) {
	opt := WithRatios([]float64{0.7, 0.3})
	cfg := splitLayoutConfig{}
	err := opt.applySplitLayoutOption(&cfg)
	assert.NoError(t, err)
	assert.Equal(t, []float64{0.7, 0.3}, cfg.ratios)

	// Verify it's a copy, not a reference.
	cfg.ratios[0] = 0.5
	// Original should be unchanged (we can't check, but the copy in cfg is independent).
}

// --- Compile-time interface checks ---

var _ tea.Model = (*SplitLayout)(nil)
var _ SplitLayoutOption = (*DirectionOption)(nil)
var _ SplitLayoutOption = (*RatiosOption)(nil)
