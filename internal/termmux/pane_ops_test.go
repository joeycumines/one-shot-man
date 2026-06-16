package termmux

import (
	"testing"
)

func TestLayoutEngine_Swap(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	id1 := e.Split(0, SplitRight)
	id2 := e.Split(id1, SplitRight)

	panesBefore := make([]PaneID, len(e.PaneIDs()))
	copy(panesBefore, e.PaneIDs())

	if !e.Swap(id1, id2) {
		t.Fatal("Swap should return true for existing panes")
	}

	panesAfter := e.PaneIDs()

	// Verify swap by checking that positions changed.
	beforeIdx1, beforeIdx2 := -1, -1
	for i, id := range panesBefore {
		if id == id1 {
			beforeIdx1 = i
		} else if id == id2 {
			beforeIdx2 = i
		}
	}

	afterIdx1, afterIdx2 := -1, -1
	for i, id := range panesAfter {
		if id == id1 {
			afterIdx1 = i
		} else if id == id2 {
			afterIdx2 = i
		}
	}

	if afterIdx1 != beforeIdx2 {
		t.Errorf("id1: before=%d, after=%d, want after=%d", beforeIdx1, afterIdx1, beforeIdx2)
	}
	if afterIdx2 != beforeIdx1 {
		t.Errorf("id2: before=%d, after=%d, want after=%d", beforeIdx2, afterIdx2, beforeIdx1)
	}
}

func TestLayoutEngine_Swap_NotFound(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	if e.Swap(99, 100) {
		t.Error("Swap should return false for nonexistent panes")
	}
}

func TestLayoutEngine_Zoom(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	id1 := e.Split(0, SplitRight)
	id2 := e.Split(id1, SplitRight)

	e.Zoom(id1)
	if e.ZoomedPane() != id1 {
		t.Error("ZoomedPane should be id1")
	}

	panes := []Pane{
		{ID: id1},
		{ID: id2},
	}
	geoms := e.Compute(panes)

	if geoms[0].Rows != 24 || geoms[0].Cols != 80 {
		t.Errorf("zoomed pane geometry = %dx%d, want 24x80", geoms[0].Rows, geoms[0].Cols)
	}
	if geoms[1].Rows != 0 || geoms[1].Cols != 0 {
		t.Errorf("non-zoomed pane geometry = %dx%d, want 0x0", geoms[1].Rows, geoms[1].Cols)
	}
}

func TestLayoutEngine_Unzoom(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	id1 := e.Split(0, SplitRight)

	e.Zoom(id1)
	if e.ZoomedPane() != id1 {
		t.Error("should be zoomed")
	}

	e.Unzoom()
	if e.ZoomedPane() != 0 {
		t.Error("should not be zoomed after Unzoom")
	}
}

func TestSessionManager_SwapPanes(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s1 := newControllableSession()
	sid1, _ := m.Register(s1, SessionTarget{Name: "one", Kind: SessionKindPTY})
	s2 := newControllableSession()
	sid2, _ := m.Register(s2, SessionTarget{Name: "two", Kind: SessionKindPTY})

	id1 := m.paneMgr.PaneIDForSession(sid1)
	id2 := m.paneMgr.PaneIDForSession(sid2)

	err := m.SwapPanes(id1, id2)
	if err != nil {
		t.Fatalf("SwapPanes: %v", err)
	}

	if m.paneMgr.panes[id1].SessionID != sid2 {
		t.Errorf("pane %d session = %d, want %d", id1, m.paneMgr.panes[id1].SessionID, sid2)
	}
	if m.paneMgr.panes[id2].SessionID != sid1 {
		t.Errorf("pane %d session = %d, want %d", id2, m.paneMgr.panes[id2].SessionID, sid1)
	}
}

func TestSessionManager_ZoomPane_Toggle(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, _ := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	paneID := m.paneMgr.PaneIDForSession(id)

	if m.ZoomedPane() != 0 {
		t.Error("no pane should be zoomed initially")
	}

	m.ZoomPane(paneID)
	if m.ZoomedPane() != paneID {
		t.Error("pane should be zoomed")
	}

	m.ZoomPane(paneID)
	if m.ZoomedPane() != 0 {
		t.Error("pane should be unzoomed after second toggle")
	}
}
