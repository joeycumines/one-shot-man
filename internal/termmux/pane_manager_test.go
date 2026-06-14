package termmux

import (
	"context"
	"testing"
	"time"
)

func TestPaneManagerNewPane(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "test-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	if paneID == 0 {
		t.Fatal("NewPane returned zero PaneID")
	}

	panes := m.Panes()
	if len(panes) != 1 {
		t.Fatalf("Panes() returned %d panes, want 1", len(panes))
	}
	if panes[0].ID != paneID {
		t.Errorf("pane ID = %d, want %d", panes[0].ID, paneID)
	}
	if panes[0].SessionID == 0 {
		t.Error("pane SessionID is zero")
	}
	if !panes[0].Focus {
		t.Error("first pane should have focus")
	}
}

func TestPaneManagerMultiplePanes(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	panes := m.Panes()
	if len(panes) != 2 {
		t.Fatalf("Panes() returned %d panes, want 2", len(panes))
	}

	if panes[0].ID != p1 {
		t.Errorf("pane[0].ID = %d, want %d", panes[0].ID, p1)
	}
	if panes[1].ID != p2 {
		t.Errorf("pane[1].ID = %d, want %d", panes[1].ID, p2)
	}

	if panes[0].SessionID == panes[1].SessionID {
		t.Error("panes should have different session IDs")
	}
}

func TestPaneManagerClosePane(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	if err := m.ClosePane(p2); err != nil {
		t.Fatalf("ClosePane: %v", err)
	}

	panes := m.Panes()
	if len(panes) != 1 {
		t.Fatalf("after close: Panes() returned %d panes, want 1", len(panes))
	}
	if panes[0].ID != p1 {
		t.Errorf("remaining pane ID = %d, want %d", panes[0].ID, p1)
	}

	if !s2.closeCalled.Load() {
		t.Error("closed pane's session was not closed")
	}
}

func TestPaneManagerClosePaneNotFound(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	err := m.ClosePane(PaneID(999))
	if err == nil {
		t.Fatal("ClosePane with nonexistent ID should return error")
	}
}

func TestPaneManagerFocusPane(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	if err := m.FocusPane(p1); err != nil {
		t.Fatalf("FocusPane: %v", err)
	}

	panes := m.Panes()
	for _, p := range panes {
		if p.ID == p1 && !p.Focus {
			t.Error("pane-1 should have focus after FocusPane")
		}
		if p.ID == p2 && p.Focus {
			t.Error("pane-2 should not have focus after FocusPane(p1)")
		}
	}

	activeID := m.ActiveID()
	if activeID == 0 {
		t.Fatal("ActiveID should not be zero after focusing a pane")
	}
}

func TestPaneManagerFocusPaneNotFound(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	err := m.FocusPane(PaneID(999))
	if err == nil {
		t.Fatal("FocusPane with nonexistent ID should return error")
	}
}

func TestPaneManagerResizePane(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	if err := m.ResizePane(p1, 0.7); err != nil {
		t.Fatalf("ResizePane: %v", err)
	}

	panes := m.Panes()
	if len(panes) != 2 {
		t.Fatalf("Panes() returned %d panes, want 2", len(panes))
	}

	for _, p := range panes {
		if p.Geometry.Rows == 0 {
			t.Errorf("pane %d geometry should have nonzero rows after resize", p.ID)
		}
	}

	_ = p1
	_ = p2
}

func TestPaneManagerResizePaneNotFound(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	err := m.ResizePane(PaneID(999), 0.5)
	if err == nil {
		t.Fatal("ResizePane with nonexistent ID should return error")
	}
}

func TestPaneManagerCloseLastPane(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	if err := m.ClosePane(p1); err != nil {
		t.Fatalf("ClosePane: %v", err)
	}

	panes := m.Panes()
	if len(panes) != 0 {
		t.Fatalf("after closing last pane: Panes() returned %d panes, want 0", len(panes))
	}

	if !s1.closeCalled.Load() {
		t.Error("closed pane's session was not closed")
	}
}

func TestPaneManagerTerminalResizeRecomputesGeometry(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	_, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	_, err = m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	before := m.Panes()

	if err := m.Resize(48, 160); err != nil {
		t.Fatalf("Resize: %v", err)
	}

	after := m.Panes()

	if len(before) != len(after) {
		t.Fatalf("pane count changed after resize: before=%d after=%d", len(before), len(after))
	}

	for i := range after {
		if after[i].Geometry.Cols <= before[i].Geometry.Cols {
			t.Errorf("pane %d: Cols did not increase after resize (before=%d after=%d)", i, before[i].Geometry.Cols, after[i].Geometry.Cols)
		}
		if after[i].Geometry.Rows <= before[i].Geometry.Rows {
			t.Errorf("pane %d: Rows did not increase after resize (before=%d after=%d)", i, before[i].Geometry.Rows, after[i].Geometry.Rows)
		}
	}
}

func TestPaneManagerNewPaneCreatesRealSession(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	session := newControllableSession()
	paneID, err := m.NewPane(session, SessionTarget{Name: "real-session", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	panes := m.Panes()
	if len(panes) != 1 {
		t.Fatalf("Panes() = %d, want 1", len(panes))
	}

	sid := panes[0].SessionID
	if sid == 0 {
		t.Fatal("pane should have a nonzero SessionID")
	}

	sessions := m.Sessions()
	found := false
	for _, s := range sessions {
		if s.ID == sid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session %d not found in Sessions() list", sid)
	}

	session.readerCh <- []byte("hello from pane")
	waitForSnapshotContains(t, m, sid, "hello from pane", 2*time.Second)

	_ = paneID
}

func TestPaneManagerClosePaneUnregistersSession(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	panes := m.Panes()
	p2SessionID := panes[1].SessionID

	if err := m.ClosePane(p2); err != nil {
		t.Fatalf("ClosePane: %v", err)
	}

	sessions := m.Sessions()
	for _, s := range sessions {
		if s.ID == p2SessionID {
			t.Errorf("session %d should have been unregistered after ClosePane", p2SessionID)
		}
	}

	remaining := m.Panes()
	if len(remaining) != 1 {
		t.Fatalf("Panes() = %d, want 1", len(remaining))
	}
	if remaining[0].ID != p1 {
		t.Errorf("remaining pane ID = %d, want %d", remaining[0].ID, p1)
	}
}

func TestPaneManagerFocusSwitchesActiveSession(t *testing.T) {

	m, cleanup := startManager(t, WithTermSize(24, 80))
	t.Cleanup(cleanup)

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	panes := m.Panes()
	p1SessionID := panes[0].SessionID
	p2SessionID := panes[1].SessionID

	if err := m.FocusPane(p1); err != nil {
		t.Fatalf("FocusPane(p1): %v", err)
	}

	activeID := m.ActiveID()
	if activeID != p1SessionID {
		t.Errorf("ActiveID = %d, want %d after FocusPane(p1)", activeID, p1SessionID)
	}

	if err := m.FocusPane(p2); err != nil {
		t.Fatalf("FocusPane(p2): %v", err)
	}

	activeID = m.ActiveID()
	if activeID != p2SessionID {
		t.Errorf("ActiveID = %d, want %d after FocusPane(p2)", activeID, p2SessionID)
	}
}

func TestPaneManagerShutdownClosesAllPanes(t *testing.T) {

	mgr := NewSessionManager(WithTermSize(24, 80))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.Run(ctx)
	}()
	<-mgr.Started()

	s1 := newControllableSession()
	_, err := mgr.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	_, err = mgr.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	cancel()
	<-errCh

	if !s1.closeCalled.Load() {
		t.Error("session 1 was not closed during shutdown")
	}
	if !s2.closeCalled.Load() {
		t.Error("session 2 was not closed during shutdown")
	}
}
