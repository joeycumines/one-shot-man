package termmux

import (
	"testing"
	"time"
)

func TestSessionManager_BreakPane_RefocusesSourceWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, err := m.NewWindow("w1")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	s1 := newControllableSession()
	p1, err := m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)
	if err != nil {
		t.Fatalf("AddPaneToWindow: %v", err)
	}

	s2 := newControllableSession()
	p2, err := m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w1, SplitRight)
	if err != nil {
		t.Fatalf("AddPaneToWindow: %v", err)
	}

	if got := m.NextWindow(); got != w1 {
		t.Fatalf("NextWindow = %d, want %d", got, w1)
	}
	if got := m.ActivePaneID(); got != p1 {
		t.Fatalf("ActivePaneID = %d, want %d", got, p1)
	}

	newWID, newPID, movedSID, err := m.BreakPane(p1)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}
	if newWID == 0 {
		t.Fatal("BreakPane returned zero window ID")
	}
	if newPID == 0 {
		t.Fatal("BreakPane returned zero pane ID")
	}
	if movedSID == 0 {
		t.Fatal("BreakPane returned zero session ID")
	}

	switch back := m.ActiveWindowID(); back {
	case w1:
		t.Fatalf("active window stayed %d; expected switch to new window", back)
	case newWID:
		// expected
	default:
		t.Fatalf("ActiveWindowID = %d, want %d", back, newWID)
	}

	if got := m.ActivePaneID(); got != newPID {
		t.Errorf("ActivePaneID = %d, want %d", got, newPID)
	}

	// Source window's remaining pane should now be active.
	panes := m.WindowPanes()
	if len(panes[w1]) != 1 || panes[w1][0].ID != p2 {
		t.Errorf("source window panes = %+v, want single pane %d", panes[w1], p2)
	}
	if !panes[w1][0].Focus {
		t.Errorf("source window pane %d not focused after break", p2)
	}

	_ = p2
}

func TestSessionManager_BreakPane_MovesSessionToNewWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	newWID, newPID, movedSID, err := m.BreakPane(p1)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}

	panes := m.WindowPanes()
	if len(panes[newWID]) != 1 {
		t.Fatalf("new window panes = %d, want 1", len(panes[newWID]))
	}
	if panes[newWID][0].ID != newPID {
		t.Errorf("new window pane ID = %d, want %d", panes[newWID][0].ID, newPID)
	}
	if panes[newWID][0].SessionID != movedSID {
		t.Errorf("new window session ID = %d, want %d", panes[newWID][0].SessionID, movedSID)
	}
	if !panes[newWID][0].Focus {
		t.Errorf("new window pane not focused")
	}

	if err := m.Input([]byte("x")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(s1.Written()); got != "x" {
		t.Errorf("s1 written = %q, want %q", got, "x")
	}
}

func TestSessionManager_BreakPane_FromRootPaneManager(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	rootPane, err := m.NewPane(s1, SessionTarget{Name: "root", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}

	newWID, newPID, _, err := m.BreakPane(rootPane)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}

	panes := m.WindowPanes()
	if len(panes[newWID]) != 1 || panes[newWID][0].ID != newPID {
		t.Errorf("new window panes = %+v, want pane %d", panes[newWID], newPID)
	}

	if got := m.ActiveWindowID(); got != newWID {
		t.Errorf("ActiveWindowID = %d, want %d", got, newWID)
	}
}

func TestSessionManager_JoinPane_MovesToTargetAndBecomesActive(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	_, _ = m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w2, SplitRight)

	newPID, movedSID, err := m.JoinPane(p1, w2)
	if err != nil {
		t.Fatalf("JoinPane: %v", err)
	}
	if newPID == 0 {
		t.Fatal("JoinPane returned zero pane ID")
	}
	if movedSID == 0 {
		t.Fatal("JoinPane returned zero session ID")
	}

	if got := m.ActiveWindowID(); got != w2 {
		t.Errorf("ActiveWindowID = %d, want %d", got, w2)
	}
	if got := m.ActivePaneID(); got != newPID {
		t.Errorf("ActivePaneID = %d, want %d", got, newPID)
	}

	panes := m.WindowPanes()
	if len(panes[w2]) != 2 {
		t.Fatalf("target window panes = %d, want 2", len(panes[w2]))
	}
	found := false
	for _, p := range panes[w2] {
		if p.ID == newPID {
			found = true
			if p.SessionID != movedSID {
				t.Errorf("moved pane session ID = %d, want %d", p.SessionID, movedSID)
			}
			if !p.Focus {
				t.Errorf("moved pane not focused in target window")
			}
		}
	}
	if !found {
		t.Errorf("moved pane %d not found in target window panes", newPID)
	}

	if len(panes[w1]) != 0 {
		t.Errorf("source window panes = %d, want 0", len(panes[w1]))
	}
}

func TestSessionManager_JoinPane_InputRoutesToMovedSession(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	_, _ = m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w2, SplitRight)

	if _, _, err := m.JoinPane(p1, w2); err != nil {
		t.Fatalf("JoinPane: %v", err)
	}

	if err := m.Input([]byte("joined")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(s1.Written()); got != "joined" {
		t.Errorf("s1 (moved session) written = %q, want %q", got, "joined")
	}
	if got := string(s2.Written()); got != "" {
		t.Errorf("s2 written = %q, want empty", got)
	}
}

func TestSessionManager_JoinPane_FromRootPaneManager(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	rootPane, _ := m.NewPane(s1, SessionTarget{Name: "root", Kind: SessionKindPTY}, SplitRight)

	w2, _ := m.NewWindow("w2")

	newPID, movedSID, err := m.JoinPane(rootPane, w2)
	if err != nil {
		t.Fatalf("JoinPane: %v", err)
	}

	panes := m.WindowPanes()
	if len(panes[w2]) != 1 || panes[w2][0].ID != newPID {
		t.Errorf("target window panes = %+v, want pane %d", panes[w2], newPID)
	}
	if panes[w2][0].SessionID != movedSID {
		t.Errorf("target window session ID = %d, want %d", panes[w2][0].SessionID, movedSID)
	}
}

func TestSessionManager_BreakJoin_EmitsEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "p1", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	p2, _ := m.AddPaneToWindow(s2, SessionTarget{Name: "p2", Kind: SessionKindPTY}, w2, SplitRight)

	subID, events := m.Subscribe(32)
	defer m.Unsubscribe(subID)

	_, _, _, err := m.BreakPane(p1)
	if err != nil {
		t.Fatalf("BreakPane: %v", err)
	}

	var foundActivated, foundWindowUpdated bool
	timeout := time.After(2 * time.Second)
drainBreak:
	for {
		select {
		case evt := <-events:
			switch evt.Kind {
			case EventSessionActivated:
				foundActivated = true
			case EventWindowUpdated:
				foundWindowUpdated = true
			}
			if foundActivated && foundWindowUpdated {
				break drainBreak
			}
		case <-timeout:
			break drainBreak
		}
	}
	if !foundActivated {
		t.Errorf("missing EventSessionActivated after BreakPane")
	}
	if !foundWindowUpdated {
		t.Errorf("missing EventWindowUpdated after BreakPane")
	}

	foundActivated = false
	foundWindowUpdated = false
	timeout = time.After(2 * time.Second)

	_, _, err = m.JoinPane(p2, w1)
	if err != nil {
		t.Fatalf("JoinPane: %v", err)
	}

	if w1 == w2 {
		t.Fatal("w1 == w2, test setup broken")
	}
drainJoin:
	for {
		select {
		case evt := <-events:
			switch evt.Kind {
			case EventSessionActivated:
				foundActivated = true
			case EventWindowUpdated:
				foundWindowUpdated = true
			}
			if foundActivated && foundWindowUpdated {
				break drainJoin
			}
		case <-timeout:
			break drainJoin
		}
	}
	if !foundActivated {
		t.Errorf("missing EventSessionActivated after JoinPane")
	}
	if !foundWindowUpdated {
		t.Errorf("missing EventWindowUpdated after JoinPane")
	}
}
