package termmux

import (
	"testing"
	"time"
)

func TestSessionManager_WindowSwitch_ActivePaneAndInput(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	// Create a window with one pane and ensure it becomes reachable through
	// the window-specific route once we switch to it.
	w1, err := m.NewWindow("w1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	s1 := newControllableSession()
	p1, err := m.AddPaneToWindow(s1, SessionTarget{Name: "pane-w1", Kind: SessionKindPTY}, w1, SplitRight)
	if err != nil {
		t.Fatalf("AddPaneToWindow: %v", err)
	}

	w2, err := m.NewWindow("w2")
	if err != nil {
		t.Fatalf("NewWindow 2: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.AddPaneToWindow(s2, SessionTarget{Name: "pane-w2", Kind: SessionKindPTY}, w2, SplitRight)
	if err != nil {
		t.Fatalf("AddPaneToWindow 2: %v", err)
	}

	// Start with w2 active.
	if got := m.NextWindow(); got != w2 {
		t.Fatalf("NextWindow = %d, want %d", got, w2)
	}

	if got := m.ActiveWindowID(); got != w2 {
		t.Errorf("ActiveWindowID = %d, want %d", got, w2)
	}

	if got := m.ActivePaneID(); got != p2 {
		t.Errorf("ActivePaneID = %d, want %d", got, p2)
	}

	panes := m.Panes()
	if len(panes) != 1 || panes[0].ID != p2 {
		t.Errorf("Panes() = %+v, want one pane with id %d", panes, p2)
	}

	if err := m.Input([]byte("s2-data")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(s2.Written()); got != "s2-data" {
		t.Errorf("s2 written = %q, want %q", got, "s2-data")
	}
	if got := string(s1.Written()); got != "" {
		t.Errorf("s1 written = %q, want empty", got)
	}

	// Switch back to w1 and verify routing follows.
	if got := m.PrevWindow(); got != w1 {
		t.Fatalf("PrevWindow = %d, want %d", got, w1)
	}

	if got := m.ActiveWindowID(); got != w1 {
		t.Errorf("ActiveWindowID after prev = %d, want %d", got, w1)
	}
	if got := m.ActivePaneID(); got != p1 {
		t.Errorf("ActivePaneID after prev = %d, want %d", got, p1)
	}

	if err := m.Input([]byte("s1-data")); err != nil {
		t.Fatalf("Input after switch: %v", err)
	}
	if got := string(s1.Written()); got != "s1-data" {
		t.Errorf("s1 written = %q, want %q", got, "s1-data")
	}

	_ = p1
	_ = time.Now()
}

func TestSessionManager_WindowSwitch_FirstPaneBecomesActive(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2a := newControllableSession()
	p2a, _ := m.AddPaneToWindow(s2a, SessionTarget{Name: "p2a", Kind: SessionKindPTY}, w2, SplitRight)
	s2b := newControllableSession()
	p2b, _ := m.AddPaneToWindow(s2b, SessionTarget{Name: "p2b", Kind: SessionKindPTY}, w2, SplitDown)

	_ = p2b
	if got := m.NextWindow(); got != w2 {
		t.Fatalf("NextWindow = %d, want %d", got, w2)
	}

	// When switching to a window with multiple panes, the first pane created
	// (the active pane by default) should drive m.activeID.
	if got := m.ActivePaneID(); got != p2a {
		t.Errorf("ActivePaneID = %d, want %d", got, p2a)
	}

	if err := m.Input([]byte("x")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(s2a.Written()); got != "x" {
		t.Errorf("s2a written = %q, want %q", got, "x")
	}
	if got := string(s2b.Written()); got != "" {
		t.Errorf("s2b written = %q, want empty", got)
	}

	// Move focus within the active window and confirm input follows.
	if got := m.FocusNextPane(NavDown); got != p2b {
		t.Errorf("FocusNextPane = %d, want %d", got, p2b)
	}
	if err := m.Input([]byte("y")); err != nil {
		t.Fatalf("Input after focus next: %v", err)
	}
	if got := string(s2b.Written()); got != "y" {
		t.Errorf("s2b written = %q, want %q", got, "y")
	}

	_ = p1
}

func TestSessionManager_WindowSwitch_EmitsActivatedEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	_, _ = m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	_, _ = m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w2, SplitRight)

	subID, events := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	_ = m.NextWindow()

	var activated SessionID
	timeout := time.After(2 * time.Second)
drain:
	for {
		select {
		case evt := <-events:
			if evt.Kind == EventSessionActivated {
				activated = evt.SessionID
				break drain
			}
		case <-timeout:
			break drain
		}
	}

	sessions := m.Sessions()
	var s2id SessionID
	for _, s := range sessions {
		if s.Target.Name == "p2" {
			s2id = s.ID
			break
		}
	}
	if s2id == 0 {
		t.Fatal("session for p2 not found")
	}

	if activated != s2id {
		t.Errorf("EventSessionActivated sessionID = %d, want %d", activated, s2id)
	}
}

func TestSessionManager_Window_ActivateSessionSetsActiveWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	_, _ = m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	_, _ = m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w2, SplitRight)

	sessions := m.Sessions()
	var s1id SessionID
	for _, s := range sessions {
		if s.Target.Name == "p1" {
			s1id = s.ID
			break
		}
	}
	if s1id == 0 {
		t.Fatal("session for p1 not found")
	}

	if err := m.Activate(s1id); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	if got := m.ActiveWindowID(); got != w1 {
		t.Errorf("ActiveWindowID = %d, want %d after explicit activate", got, w1)
	}

	if err := m.Input([]byte("z")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(s1.Written()); got != "z" {
		t.Errorf("s1 written = %q, want %q", got, "z")
	}
}

func TestSessionManager_Window_FocusAtWorksInWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	_, _ = m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	p2, _ := m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w2, SplitRight)

	_ = m.NextWindow()

	got, err := m.FocusAt(5, 5)
	if err != nil {
		t.Fatalf("FocusAt: %v", err)
	}
	if got != p2 {
		t.Errorf("FocusAt = %d, want %d", got, p2)
	}

	if err := m.Input([]byte("a")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(s2.Written()); got != "a" {
		t.Errorf("s2 written = %q, want %q", got, "a")
	}
}

func TestSessionManager_Window_ResizePaneAtWorksInWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	_, _ = m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	if err := m.SetLayoutMode(w2, LayoutHorizontal); err != nil {
		t.Fatalf("SetLayoutMode w2: %v", err)
	}
	s2 := newControllableSession()
	p2a, _ := m.AddPaneToWindow(s2, SessionTarget{Name: "p2a", Kind: SessionKindPTY}, w2, SplitRight)
	s2b := newControllableSession()
	p2b, _ := m.AddPaneToWindow(s2b, SessionTarget{Name: "p2b", Kind: SessionKindPTY}, w2, SplitRight)

	_ = m.NextWindow()

	// The divider between the two panes in w2 should be roughly mid-screen.
	// Clicking near the top-center should resize that divider.
	if err := m.ResizePaneAt(5, 40, 0.7); err != nil {
		t.Fatalf("ResizePaneAt: %v", err)
	}

	panes := m.Panes()
	if len(panes) != 2 {
		t.Fatalf("Panes() = %d, want 2", len(panes))
	}

	var ratioOK bool
	for _, p := range panes {
		if p.ID == p2a || p.ID == p2b {
			if p.Geometry.Rows > 0 && p.Geometry.Cols > 0 {
				ratioOK = true
			}
		}
	}
	if !ratioOK {
		t.Errorf("ResizePaneAt did not produce valid pane geometries: %+v", panes)
	}
}
