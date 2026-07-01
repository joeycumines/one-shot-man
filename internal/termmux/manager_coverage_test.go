package termmux

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestSessionManager_Screen(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	cs := NewCaptureSession(CaptureConfig{Command: buildIdleProgram(t)})
	if err := cs.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	id, err := m.Register(cs, SessionTarget{Name: "screen-test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := cs.WriteString("hello screen\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}

	waitForSnapshotContains(t, m, id, "hello screen", 2*time.Second)

	scr := m.Screen(id)
	if scr == nil {
		t.Fatal("Screen returned nil")
	}
	if scr.Rows == 0 || scr.Cols == 0 {
		t.Errorf("screen dimensions zero: %dx%d", scr.Rows, scr.Cols)
	}

	if m.Screen(9999) != nil {
		t.Error("Screen for unknown session should return nil")
	}
}

func TestSessionManager_PaneNavigationAndState(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	p1, err := m.NewPane(s1, SessionTarget{Name: "pane-1", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.NewPane(s2, SessionTarget{Name: "pane-2", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}

	active := m.ActivePaneID()
	if active != p1 && active != p2 {
		t.Fatalf("ActivePaneID = %d, want %d or %d", active, p1, p2)
	}

	if next := m.FocusNextPane(NavNext); next != active {
		if next != p1 && next != p2 {
			t.Errorf("FocusNextPane returned unexpected %d", next)
		}
	}

	panes := m.Panes()
	if len(panes) != 2 {
		t.Fatalf("Panes() = %d, want 2", len(panes))
	}

	if err := m.ResizePane(p2, 0.7); err != nil {
		t.Errorf("ResizePane: %v", err)
	}

	geoms := m.Panes()
	if len(geoms) != 2 {
		t.Fatalf("Panes() = %d, want 2", len(geoms))
	}
	p2Row := geoms[1].Geometry.Row + geoms[1].Geometry.Rows/2
	p2Col := geoms[1].Geometry.Col + geoms[1].Geometry.Cols/2
	focused, err := m.FocusAt(p2Row, p2Col)
	if err != nil {
		t.Fatalf("FocusAt: %v", err)
	}
	if focused != p2 {
		t.Errorf("FocusAt = %d, want %d", focused, p2)
	}

	if m.ActivePaneID() != p2 {
		t.Errorf("ActivePaneID = %d, want %d after FocusAt", m.ActivePaneID(), p2)
	}

	m.Input([]byte("after-focus"))
	if !bytes.Contains(s2.Written(), []byte("after-focus")) {
		t.Error("input not delivered to focused pane's session")
	}
	if bytes.Contains(s1.Written(), []byte("after-focus")) {
		t.Error("input delivered to unfocused pane's session")
	}

	dividerRow := geoms[0].Geometry.Row + geoms[0].Geometry.Rows
	if err := m.ResizePaneAt(dividerRow, p2Col, 0.8); err != nil {
		t.Fatalf("ResizePaneAt: %v", err)
	}
	if len(s1.Resizes()) == 0 {
		t.Error("expected session 1 to receive Resize after ResizePaneAt")
	}
	if len(s2.Resizes()) == 0 {
		t.Error("expected session 2 to receive Resize after ResizePaneAt")
	}

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Errorf("SwapPanes: %v", err)
	}

	m.ZoomPane(p1)
	if m.ZoomedPane() != p1 {
		t.Errorf("ZoomedPane = %d, want %d", m.ZoomedPane(), p1)
	}

	if err := m.SetPaneRemainOnExit(p1, true); err != nil {
		t.Errorf("SetPaneRemainOnExit: %v", err)
	}
	rem, err := m.PaneRemainOnExit(p1)
	if err != nil || !rem {
		t.Errorf("PaneRemainOnExit = %v,%v, want true,nil", rem, err)
	}
	if m.PaneExited(p1) {
		t.Error("PaneExited should be false for running pane")
	}
}

func TestSessionManager_WindowOperations(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, err := m.NewWindow("first")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if w1 == 0 {
		t.Fatal("NewWindow returned zero ID")
	}

	w2, err := m.NewWindow("second")
	if err != nil {
		t.Fatalf("NewWindow 2: %v", err)
	}

	active := m.ActiveWindowID()
	if active != w1 && active != w2 {
		t.Fatalf("ActiveWindowID = %d, want %d or %d", active, w1, w2)
	}

	if next := m.NextWindow(); next == 0 {
		t.Error("NextWindow returned zero")
	}
	if prev := m.PrevWindow(); prev == 0 {
		t.Error("PrevWindow returned zero")
	}

	if err := m.RenameWindow(w1, "renamed"); err != nil {
		t.Errorf("RenameWindow: %v", err)
	}

	windows := m.Windows()
	if len(windows) != 2 {
		t.Errorf("Windows() = %d, want 2", len(windows))
	}

	panes := m.WindowPanes()
	if len(panes) != 2 {
		t.Errorf("WindowPanes() = %d windows, want 2", len(panes))
	}

	if err := m.CloseWindow(w1); err != nil {
		t.Errorf("CloseWindow: %v", err)
	}
	if len(m.Windows()) != 1 {
		t.Errorf("after close windows = %d, want 1", len(m.Windows()))
	}
}
