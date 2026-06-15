package termmux

import (
	"testing"
)

func TestSessionTarget_Helpers(t *testing.T) {
	target := SessionTarget{}
	if !target.IsZero() {
		t.Error("zero target should be zero")
	}

	target = target.WithName("n").WithID("i").WithKind(SessionKindPTY)
	if target.Name != "n" || target.ID != "i" || target.Kind != SessionKindPTY {
		t.Errorf("target = %+v", target)
	}
	if target.IsZero() {
		t.Error("populated target should not be zero")
	}
	if got := target.String(); got != "n[pty:i]" {
		t.Errorf("String = %q, want n[pty:i]", got)
	}
}

func TestPrefixAction_String(t *testing.T) {
	cases := []struct {
		kind PrefixActionKind
		want string
	}{
		{PrefixActionNewWindow, "NewWindow"},
		{PrefixActionNextWindow, "NextWindow"},
		{PrefixActionPrevWindow, "PrevWindow"},
		{PrefixActionDetach, "Detach"},
		{PrefixActionZoomPane, "ZoomPane"},
		{PrefixActionClosePane, "ClosePane"},
		{PrefixActionSplitHorizontal, "SplitHorizontal"},
		{PrefixActionSplitVertical, "SplitVertical"},
		{PrefixActionCopyMode, "CopyMode"},
		{PrefixActionListKeys, "ListKeys"},
		{PrefixActionRenameWindow, "RenameWindow"},
		{PrefixActionCancel, "Cancel"},
		{PrefixActionKind(999), "None"},
	}
	for _, tc := range cases {
		got := PrefixAction{Kind: tc.kind}.String()
		if got != tc.want {
			t.Errorf("String(%v) = %q, want %q", tc.kind, got, tc.want)
		}
	}
}

func TestPaneManager_MiscHelpers(t *testing.T) {
	pm := newPaneManager(LayoutVertical, 80, 24)
	if pm.HasPanes() {
		t.Error("HasPanes should be false initially")
	}
	if pm.ActivePaneID() != 0 {
		t.Error("ActivePaneID should be zero initially")
	}
	if len(pm.AllSessionIDs()) != 0 {
		t.Error("AllSessionIDs should be empty initially")
	}

	p1 := pm.engine.Split(0, SplitDown)
	pm.panes[p1] = &PaneBinding{PaneID: p1, SessionID: SessionID(1), Title: "p1"}
	pm.paneOrder = []PaneID{p1}
	pm.activePaneID = p1

	if !pm.HasPanes() {
		t.Error("HasPanes should be true after adding pane")
	}
	if pm.ActivePaneID() != p1 {
		t.Errorf("ActivePaneID = %d, want %d", pm.ActivePaneID(), p1)
	}
	if ids := pm.AllSessionIDs(); len(ids) != 1 || ids[0] != SessionID(1) {
		t.Errorf("AllSessionIDs = %v, want [1]", ids)
	}
	if b := pm.Binding(p1); b == nil || b.SessionID != SessionID(1) {
		t.Error("Binding returned wrong value")
	}
	if pm.Binding(999) != nil {
		t.Error("Binding for unknown pane should be nil")
	}

	pm.MarkPaneExited(p1)
	if !pm.panes[p1].Exited {
		t.Error("MarkPaneExited did not set Exited")
	}

	pm.FocusRight()
	pm.FocusLeft()
	pm.FocusUp()
	pm.FocusDown()
	if pm.ActivePaneID() != p1 {
		t.Errorf("single pane focus should remain %d, got %d", p1, pm.ActivePaneID())
	}
}

func TestPaneManager_TransferPaneToWindow(t *testing.T) {
	src := newPaneManager(LayoutVertical, 80, 24)
	dst := newPaneManager(LayoutVertical, 80, 24)

	p1 := src.engine.Split(0, SplitDown)
	src.panes[p1] = &PaneBinding{PaneID: p1, SessionID: SessionID(1)}
	src.paneOrder = []PaneID{p1}
	src.activePaneID = p1

	newID := src.transferPaneToWindow(dst, SplitRight)
	if newID == 0 {
		t.Fatal("transferPaneToWindow returned zero")
	}
	if _, ok := dst.panes[newID]; !ok {
		t.Errorf("pane %d not in destination", newID)
	}
	if _, ok := src.panes[p1]; ok {
		t.Error("pane still in source")
	}
}

func TestLayoutEngine_ChromeRowsGetter(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	e.SetChromeRows(3)
	if e.ChromeRows() != 3 {
		t.Errorf("ChromeRows = %d, want 3", e.ChromeRows())
	}
}
