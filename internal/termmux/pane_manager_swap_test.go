package termmux

import (
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func TestPaneManager_Swap_SameManager(t *testing.T) {
	pm := newPaneManager(LayoutVertical, 80, 24)

	id1, err := pm.Create(1, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 1: %v", err)
	}
	id2, err := pm.Create(2, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 2: %v", err)
	}

	pm.panes[id1].Title = "one"
	pm.panes[id1].RemainOnExit = true
	pm.panes[id1].Exited = true
	pm.panes[id1].LastActive = time.Unix(1, 0)
	pm.panes[id1].VTerm = &vt.VTerm{}

	pm.panes[id2].Title = "two"
	pm.panes[id2].RemainOnExit = false
	pm.panes[id2].Exited = false
	pm.panes[id2].LastActive = time.Unix(2, 0)
	pm.panes[id2].VTerm = &vt.VTerm{}

	if err := pm.Swap(id1, id2); err != nil {
		t.Fatalf("Swap: %v", err)
	}

	if pm.panes[id1].SessionID != 2 {
		t.Errorf("pane %d SessionID = %d, want 2", id1, pm.panes[id1].SessionID)
	}
	if pm.panes[id1].Title != "two" {
		t.Errorf("pane %d Title = %q, want two", id1, pm.panes[id1].Title)
	}
	if pm.panes[id1].RemainOnExit != false {
		t.Errorf("pane %d RemainOnExit = %v, want false", id1, pm.panes[id1].RemainOnExit)
	}
	if pm.panes[id1].Exited != false {
		t.Errorf("pane %d Exited = %v, want false", id1, pm.panes[id1].Exited)
	}
	if pm.panes[id1].LastActive.Unix() != 2 {
		t.Errorf("pane %d LastActive = %v, want 2", id1, pm.panes[id1].LastActive)
	}

	if pm.panes[id2].SessionID != 1 {
		t.Errorf("pane %d SessionID = %d, want 1", id2, pm.panes[id2].SessionID)
	}
	if pm.panes[id2].Title != "one" {
		t.Errorf("pane %d Title = %q, want one", id2, pm.panes[id2].Title)
	}
	if pm.panes[id2].RemainOnExit != true {
		t.Errorf("pane %d RemainOnExit = %v, want true", id2, pm.panes[id2].RemainOnExit)
	}
	if pm.panes[id2].Exited != true {
		t.Errorf("pane %d Exited = %v, want true", id2, pm.panes[id2].Exited)
	}
	if pm.panes[id2].LastActive.Unix() != 1 {
		t.Errorf("pane %d LastActive = %v, want 1", id2, pm.panes[id2].LastActive)
	}

	if pm.panes[id1].PaneID != id1 {
		t.Errorf("pane %d PaneID changed to %d", id1, pm.panes[id1].PaneID)
	}
	if pm.panes[id2].PaneID != id2 {
		t.Errorf("pane %d PaneID changed to %d", id2, pm.panes[id2].PaneID)
	}
}

func TestPaneManager_Swap_PreservesActivePaneID(t *testing.T) {
	pm := newPaneManager(LayoutVertical, 80, 24)

	id1, err := pm.Create(1, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 1: %v", err)
	}
	id2, err := pm.Create(2, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 2: %v", err)
	}
	pm.activePaneID = id2

	if err := pm.Swap(id1, id2); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if pm.activePaneID != id2 {
		t.Errorf("activePaneID = %d, want %d", pm.activePaneID, id2)
	}
}

func TestPaneManager_Swap_MissingPane(t *testing.T) {
	pm := newPaneManager(LayoutVertical, 80, 24)
	id1, err := pm.Create(1, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 1: %v", err)
	}

	if err := pm.Swap(id1, 999); err == nil {
		t.Error("Swap missing pane: expected error, got nil")
	}
}

func TestPaneManager_Swap_DoubleSwapRestoresOriginal(t *testing.T) {
	pm := newPaneManager(LayoutVertical, 80, 24)

	id1, err := pm.Create(10, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 10: %v", err)
	}
	id2, err := pm.Create(20, SplitRight)
	if err != nil {
		t.Fatalf("Create pane 20: %v", err)
	}

	if err := pm.Swap(id1, id2); err != nil {
		t.Fatalf("first Swap: %v", err)
	}
	if err := pm.Swap(id1, id2); err != nil {
		t.Fatalf("second Swap: %v", err)
	}

	if pm.panes[id1].SessionID != 10 {
		t.Errorf("pane %d SessionID = %d, want 10", id1, pm.panes[id1].SessionID)
	}
	if pm.panes[id2].SessionID != 20 {
		t.Errorf("pane %d SessionID = %d, want 20", id2, pm.panes[id2].SessionID)
	}
}
