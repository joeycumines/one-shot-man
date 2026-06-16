package termmux

import (
	"testing"
	"time"
)

func TestSessionManager_SwapPanes_SameWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	sid1, err := m.Register(s1, SessionTarget{Name: "one", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register s1: %v", err)
	}

	s2 := newControllableSession()
	sid2, err := m.Register(s2, SessionTarget{Name: "two", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register s2: %v", err)
	}

	p1 := m.paneMgr.PaneIDForSession(sid1)
	p2 := m.paneMgr.PaneIDForSession(sid2)

	// Distinguish the bindings so swapping is observable.
	m.paneMgr.SetPaneRemainOnExit(p1, true)
	m.paneMgr.panes[p1].Title = "pane-a"
	m.paneMgr.panes[p2].Title = "pane-b"
	m.paneMgr.panes[p1].LastActive = time.Unix(100, 0)
	m.paneMgr.panes[p2].LastActive = time.Unix(200, 0)

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	if m.paneMgr.panes[p1].SessionID != sid2 {
		t.Errorf("pane %d SessionID = %d, want %d", p1, m.paneMgr.panes[p1].SessionID, sid2)
	}
	if m.paneMgr.panes[p2].SessionID != sid1 {
		t.Errorf("pane %d SessionID = %d, want %d", p2, m.paneMgr.panes[p2].SessionID, sid1)
	}
	if m.paneMgr.panes[p1].Title != "pane-b" {
		t.Errorf("pane %d Title = %q, want pane-b", p1, m.paneMgr.panes[p1].Title)
	}
	if m.paneMgr.panes[p2].Title != "pane-a" {
		t.Errorf("pane %d Title = %q, want pane-a", p2, m.paneMgr.panes[p2].Title)
	}
	if got, _ := m.PaneRemainOnExit(p1); got != false {
		t.Errorf("pane %d RemainOnExit = %v, want false", p1, got)
	}
	if got, _ := m.PaneRemainOnExit(p2); got != true {
		t.Errorf("pane %d RemainOnExit = %v, want true", p2, got)
	}

	// Pane IDs must remain stable.
	if m.paneMgr.PaneIDForSession(sid1) != p2 {
		t.Errorf("sid1 now lives in pane %d, want %d", m.paneMgr.PaneIDForSession(sid1), p2)
	}
	if m.paneMgr.PaneIDForSession(sid2) != p1 {
		t.Errorf("sid2 now lives in pane %d, want %d", m.paneMgr.PaneIDForSession(sid2), p1)
	}
}

func TestSessionManager_SwapPanes_PreservesActiveID(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	sid1, _ := m.Register(s1, SessionTarget{Name: "one", Kind: SessionKindPTY})
	s2 := newControllableSession()
	sid2, _ := m.Register(s2, SessionTarget{Name: "two", Kind: SessionKindPTY})

	p1 := m.paneMgr.PaneIDForSession(sid1)
	p2 := m.paneMgr.PaneIDForSession(sid2)

	if err := m.FocusPane(p2); err != nil {
		t.Fatalf("FocusPane: %v", err)
	}

	if m.ActivePaneID() != p2 {
		t.Fatalf("ActivePaneID = %d, want %d", m.ActivePaneID(), p2)
	}

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	if m.ActivePaneID() != p2 {
		t.Errorf("ActivePaneID = %d, want %d", m.ActivePaneID(), p2)
	}
	if m.ActiveID() != sid1 {
		t.Errorf("ActiveID = %d, want %d (session now in active pane)", m.ActiveID(), sid1)
	}
}

func TestSessionManager_SwapPanes_WindowPanesReflectsSwap(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "a", Kind: SessionKindPTY}, w1, SplitRight)

	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	p2, _ := m.AddPaneToWindow(s2, SessionTarget{Name: "b", Kind: SessionKindPTY}, w2, SplitRight)

	sid1 := windowPaneSession(m, w1, p1)
	sid2 := windowPaneSession(m, w2, p2)

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	wp := m.WindowPanes()
	if len(wp[w1]) != 1 {
		t.Fatalf("window %d pane count = %d, want 1", w1, len(wp[w1]))
	}
	if len(wp[w2]) != 1 {
		t.Fatalf("window %d pane count = %d, want 1", w2, len(wp[w2]))
	}
	if wp[w1][0].SessionID != sid2 {
		t.Errorf("window %d pane session = %d, want %d", w1, wp[w1][0].SessionID, sid2)
	}
	if wp[w2][0].SessionID != sid1 {
		t.Errorf("window %d pane session = %d, want %d", w2, wp[w2][0].SessionID, sid1)
	}
	if wp[w1][0].ID != p1 {
		t.Errorf("window %d pane ID changed to %d, want %d", w1, wp[w1][0].ID, p1)
	}
	if wp[w2][0].ID != p2 {
		t.Errorf("window %d pane ID changed to %d, want %d", w2, wp[w2][0].ID, p2)
	}
}

func TestSessionManager_SwapPanes_EmitsWindowUpdated(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, _ := m.NewWindow("w1")
	s1 := newControllableSession()
	p1, _ := m.AddPaneToWindow(s1, SessionTarget{Name: "a", Kind: SessionKindPTY}, w1, SplitRight)
	w2, _ := m.NewWindow("w2")
	s2 := newControllableSession()
	p2, _ := m.AddPaneToWindow(s2, SessionTarget{Name: "b", Kind: SessionKindPTY}, w2, SplitRight)

	subID, events := m.Subscribe(32)
	defer m.Unsubscribe(subID)

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	var found int
	timeout := time.After(2 * time.Second)
drain:
	for {
		select {
		case evt := <-events:
			if evt.Kind == EventWindowUpdated {
				found++
			}
			if found >= 1 {
				break drain
			}
		case <-timeout:
			break drain
		}
	}
	if found < 1 {
		t.Errorf("missing EventWindowUpdated after SwapPanes")
	}
}

func TestSessionManager_SwapPanes_ExitedState(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	sid1, _ := m.Register(s1, SessionTarget{Name: "one", Kind: SessionKindPTY})
	s2 := newControllableSession()
	sid2, _ := m.Register(s2, SessionTarget{Name: "two", Kind: SessionKindPTY})

	p1 := m.paneMgr.PaneIDForSession(sid1)
	p2 := m.paneMgr.PaneIDForSession(sid2)

	m.paneMgr.MarkPaneExited(p2)
	m.paneMgr.SetPaneRemainOnExit(p1, true)

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	if m.PaneExited(p1) != true {
		t.Errorf("pane %d Exited = %v, want true", p1, m.PaneExited(p1))
	}
	if m.PaneExited(p2) != false {
		t.Errorf("pane %d Exited = %v, want false", p2, m.PaneExited(p2))
	}
	if got, _ := m.PaneRemainOnExit(p2); got != true {
		t.Errorf("pane %d RemainOnExit = %v, want true", p2, got)
	}
}

func TestSessionManager_SwapPanes_VTermReference(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	sid1, _ := m.Register(s1, SessionTarget{Name: "one", Kind: SessionKindPTY})
	s2 := newControllableSession()
	sid2, _ := m.Register(s2, SessionTarget{Name: "two", Kind: SessionKindPTY})

	p1 := m.paneMgr.PaneIDForSession(sid1)
	p2 := m.paneMgr.PaneIDForSession(sid2)

	v1 := m.paneMgr.panes[p1].VTerm
	v2 := m.paneMgr.panes[p2].VTerm

	if err := m.SwapPanes(p1, p2); err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	if m.paneMgr.panes[p1].VTerm != v2 {
		t.Errorf("pane %d VTerm not swapped", p1)
	}
	if m.paneMgr.panes[p2].VTerm != v1 {
		t.Errorf("pane %d VTerm not swapped", p2)
	}
}

func TestSessionManager_SwapPanes_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	if err := m.SwapPanes(1, 2); err == nil {
		t.Error("SwapPanes with missing panes: expected error")
	}
}

func windowPaneSession(m *SessionManager, wid WindowID, pid PaneID) SessionID {
	for _, p := range m.WindowPanes()[wid] {
		if p.ID == pid {
			return p.SessionID
		}
	}
	return 0
}
