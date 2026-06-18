package termpane

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// controllableSession is a test double for termmux.InteractiveSession.
// It records Write/Resize calls and allows controlling output via readerCh.
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

// startTestManager creates a SessionManager with a registered controllable
// session, starts the worker, and returns everything needed for testing.
// The cleanup function cancels the context and waits for the worker to stop.
func startTestManager(t *testing.T) (*termmux.SessionManager, *controllableSession, termmux.SessionID, func()) {
	t.Helper()

	m := termmux.NewSessionManager(termmux.WithTermSize(24, 80))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- m.Run(ctx) }()
	<-m.Started()

	session := newControllableSession()
	id, err := m.Register(session, termmux.SessionTarget{Name: "test", Kind: termmux.SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Pump some output to transition to Running state.
	session.readerCh <- []byte("ready")
	deadline := time.After(2 * time.Second)
	for {
		snap := m.Snapshot(id)
		if snap != nil && strings.Contains(snap.GetPlainText(), "ready") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for session to reach Running state")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cleanup := func() {
		cancel()
		<-errCh
	}
	return m, session, id, cleanup
}

// TestNewModel verifies that NewModel creates a Model with the correct
// session ID, bounds, and an initial snapshot.
func TestNewModel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	if model.SessionID() != sid {
		t.Errorf("SessionID() = %d, want %d", model.SessionID(), sid)
	}
	if model.Bounds() != bounds {
		t.Errorf("Bounds() = %v, want %v", model.Bounds(), bounds)
	}

	model.mu.Lock()
	snap := model.snap
	model.mu.Unlock()
	if snap == nil {
		t.Error("initial snapshot is nil")
	}
}

// TestView_GenerationCache verifies that View returns the cached result
// when the snapshot generation has not changed.
func TestView_GenerationCache(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}
	// TODO: this test deadlocks inside Model.viewContentAndCursor while the
	// bridgeEvents goroutine is blocked waiting for output from a session
	// registered directly via SessionManager.Register (no backing pane). Skip
	// until the termui/termpane test harness creates proper panes.
	t.Skip("broken: registered session lacks backing pane / deadlocks View")

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// First View call renders and caches.
	v1 := model.View()

	// Second View call should return cached result (same generation).
	// We verify by checking that cachedGen matches the snapshot's Gen.
	model.mu.Lock()
	gen := model.cachedGen
	model.mu.Unlock()

	model.mu.Lock()
	snapGen := model.snap.Gen
	model.mu.Unlock()

	if gen != snapGen {
		t.Errorf("cachedGen %d != snap.Gen %d", gen, snapGen)
	}

	// Call View again — should return same content without re-rendering.
	v2 := model.View()
	if v1.Content != v2.Content {
		t.Error("View() returned different content on second call with same generation")
	}
}

// TestView_UpdatedContent verifies that View re-renders when the
// snapshot generation changes after new output is produced.
func TestView_UpdatedContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Get initial view.
	v1 := model.View()

	// Produce new output to increment the generation.
	session.readerCh <- []byte("updated content here")

	// Wait for the snapshot to update.
	deadline := time.After(2 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && strings.Contains(snap.GetPlainText(), "updated") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for updated snapshot")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Simulate an outputMsg arriving via Update to refresh the snapshot.
	model.mu.Lock()
	model.snap = mgr.Snapshot(sid)
	model.mu.Unlock()

	// View should now re-render with new content.
	v2 := model.View()
	if v1.Content == v2.Content {
		t.Error("View() returned same content after generation change")
	}
}

// TestView_CursorPosition verifies that View sets the cursor position
// from the ScreenSnapshot, offset by the bounds position.
func TestView_CursorPosition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	// Pane at offset (5, 3) with size 40x10.
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 5, Y: 3},
		Size:     coordinate.Size{Width: 40, Height: 10},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Manually set a snapshot with known cursor position.
	scr := vt.NewScreen(10, 40)
	scr.CurRow = 2
	scr.CurCol = 10
	model.mu.Lock()
	model.snap = termmux.NewScreenSnapshot(999, scr, 10, 40, time.Now())
	model.cachedGen = 0 // Force re-render.
	model.mu.Unlock()

	v := model.View()
	if v.Cursor == nil {
		t.Fatal("Cursor is nil, expected cursor at offset position")
	}
	// Cursor should be at (10+5, 2+3) = (15, 5).
	if v.Cursor.Position.X != 15 {
		t.Errorf("Cursor.X = %d, want 15", v.Cursor.Position.X)
	}
	if v.Cursor.Position.Y != 5 {
		t.Errorf("Cursor.Y = %d, want 5", v.Cursor.Position.Y)
	}
}

// TestView_CursorOutsideBounds verifies that the cursor is not set when
// the cursor position falls outside the pane's bounds.
func TestView_CursorOutsideBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 40, Height: 10},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Cursor at row 50 — outside the 10-row pane.
	scr := vt.NewScreen(24, 80)
	scr.CurRow = 50
	scr.CurCol = 0
	model.mu.Lock()
	model.snap = termmux.NewScreenSnapshot(998, scr, 24, 80, time.Now())
	model.cachedGen = 0
	model.mu.Unlock()

	v := model.View()
	if v.Cursor != nil {
		t.Errorf("Cursor should be nil when outside bounds, got %+v", v.Cursor)
	}
}

// TestUpdate_KeyPressForwarding verifies that key events are forwarded
// to the PTY via the session manager's Input method.
func TestUpdate_KeyPressForwarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Activate the session so Input goes to it.
	if err := mgr.Activate(sid); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Send a KeyPressMsg for "a".
	key := tea.Key{Text: "a", Code: 'a'}
	_, cmd := model.Update(tea.KeyPressMsg(key))
	if cmd != nil {
		t.Errorf("Update(KeyPressMsg) returned non-nil cmd, want nil")
	}

	// Verify the session received the input.
	written := session.Written()
	if len(written) == 0 {
		t.Fatal("session received no input after KeyPressMsg")
	}
	if string(written) != "a" {
		t.Errorf("session received %q, want %q", string(written), "a")
	}
}

// TestUpdate_QuitCleanup verifies that receiving a QuitMsg causes
// the model to unsubscribe from the EventBus and clean up.
func TestUpdate_QuitCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)

	// Send QuitMsg via Update.
	_, cmd := model.Update(tea.QuitMsg{})
	if cmd == nil {
		t.Error("Update(QuitMsg) returned nil cmd, want tea.Quit")
	}

	// Verify Close was called.
	model.mu.Lock()
	closed := model.closed
	model.mu.Unlock()
	if !closed {
		t.Error("model not closed after QuitMsg")
	}
}

// TestClose_Idempotent verifies that calling Close multiple times
// does not panic or cause errors.
func TestClose_Idempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)

	// Call Close twice — should not panic.
	if err := model.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := model.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestSetBounds verifies that SetBounds updates the bounds.
func TestSetBounds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	newBounds := coordinate.Rect{
		Position: coordinate.Position{X: 10, Y: 5},
		Size:     coordinate.Size{Width: 40, Height: 12},
	}
	model.SetBounds(newBounds)

	if model.Bounds() != newBounds {
		t.Errorf("Bounds() = %v, want %v", model.Bounds(), newBounds)
	}
}

// TestView_NilSnapshot verifies that View returns an empty view
// when the snapshot is nil.
func TestView_NilSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, _, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Force nil snapshot.
	model.mu.Lock()
	model.snap = nil
	model.cachedGen = 0
	model.mu.Unlock()

	v := model.View()
	if v.Content != "" {
		t.Errorf("View() with nil snapshot returned %q, want empty", v.Content)
	}
}

// TestUpdate_OutputMsgRefreshesSnapshot verifies that receiving an
// outputMsg causes the model to refresh its snapshot from the manager.
func TestUpdate_OutputMsgRefreshesSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Get initial snapshot gen.
	initialSnap := mgr.Snapshot(sid)
	if initialSnap == nil {
		t.Fatal("initial snapshot is nil")
	}
	initialGen := initialSnap.Gen

	// Produce new output.
	session.readerCh <- []byte("new output for refresh test")

	// Wait for the manager to process the output.
	deadline := time.After(2 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && snap.Gen > initialGen {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for new snapshot generation")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Simulate an outputMsg arriving via Update.
	evt := termmux.Event{
		Kind:      termmux.EventSessionOutput,
		SessionID: sid,
	}
	_, cmd := model.Update(outputMsg(evt))

	// The cmd should be waitForOutput (re-subscribe for next event).
	if cmd == nil {
		t.Error("Update(outputMsg) returned nil cmd, want waitForOutput")
	}

	// Verify the snapshot was refreshed.
	model.mu.Lock()
	snap := model.snap
	model.mu.Unlock()
	if snap == nil {
		t.Fatal("snapshot is nil after outputMsg")
	}
	if snap.Gen <= initialGen {
		t.Errorf("snapshot Gen %d <= initial Gen %d", snap.Gen, initialGen)
	}
}

// TestUpdate_WindowSizeMsg verifies that WindowSizeMsg updates bounds
// and resizes the session.
func TestUpdate_WindowSizeMsg(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 80, Height: 24},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Send WindowSizeMsg.
	_, cmd := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Errorf("Update(WindowSizeMsg) returned non-nil cmd, want nil")
	}

	// Verify bounds were updated.
	b := model.Bounds()
	if b.Size.Width != 120 || b.Size.Height != 40 {
		t.Errorf("Bounds after resize = %v, want 120x40", b.Size)
	}

	// Verify session was resized.
	resizes := session.Resizes()
	if len(resizes) == 0 {
		t.Fatal("session received no resize calls")
	}
	last := resizes[len(resizes)-1]
	if last.rows != 40 || last.cols != 120 {
		t.Errorf("resize call = %dx%d, want 40x120", last.rows, last.cols)
	}
}

// TestMouseForwarding verifies that mouse events are forwarded to the PTY
// with coordinate offset.
func TestMouseForwarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	mgr, session, sid, cleanup := startTestManager(t)
	defer cleanup()

	// Pane at offset (5, 3).
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 5, Y: 3},
		Size:     coordinate.Size{Width: 40, Height: 10},
	}

	model := NewModel(sid, mgr, bounds)
	defer model.Close()

	// Activate the session so Input goes to it.
	if err := mgr.Activate(sid); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	// Send a MouseClickMsg at screen position (10, 8).
	clickMsg := tea.MouseClickMsg{
		X:      10,
		Y:      8,
		Button: tea.MouseLeft,
	}
	_, cmd := model.Update(clickMsg)
	if cmd != nil {
		t.Errorf("Update(MouseClickMsg) returned non-nil cmd, want nil")
	}

	// Verify the session received SGR-encoded input.
	written := session.Written()
	if len(written) == 0 {
		t.Fatal("session received no input after MouseClickMsg")
	}
	// The SGR sequence should contain pane-local coordinates (10-5+1=6, 8-3+1=6).
	s := string(written)
	if !strings.HasPrefix(s, "\x1b[<") {
		t.Errorf("expected SGR mouse sequence, got %q", s)
	}
}
