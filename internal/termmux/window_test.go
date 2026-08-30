package termmux

import (
	"testing"
	"time"
)

func TestWindowManager_NewWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id := wm.NewWindow("shell")
	if id == 0 {
		t.Error("NewWindow returned 0, want non-zero")
	}
	if wm.ActiveWindowID() != id {
		t.Errorf("ActiveWindowID = %d, want %d (first window should be active)", wm.ActiveWindowID(), id)
	}

	w := wm.Window(id)
	if w == nil {
		t.Fatalf("Window(%d) returned nil", id)
	}
	if w.Name != "shell" {
		t.Errorf("Window.Name = %q, want %q", w.Name, "shell")
	}
	if w.paneMgr == nil {
		t.Error("Window.paneMgr is nil")
	}
}

func TestWindowManager_NewWindow_DefaultName(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id := wm.NewWindow("")
	w := wm.Window(id)
	if w == nil {
		t.Fatal("Window is nil")
	}
	if w.Name != "window-1" {
		t.Errorf("Window.Name = %q, want %q", w.Name, "window-1")
	}
}

func TestWindowManager_MultipleWindows(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("first")
	id2 := wm.NewWindow("second")
	id3 := wm.NewWindow("third")

	if id1 == id2 || id2 == id3 || id1 == id3 {
		t.Errorf("window IDs should be unique: %d, %d, %d", id1, id2, id3)
	}

	windows := wm.Windows()
	if len(windows) != 3 {
		t.Fatalf("len(Windows) = %d, want 3", len(windows))
	}

	if wm.ActiveWindowID() != id1 {
		t.Errorf("ActiveWindowID = %d, want %d (first window stays active)", wm.ActiveWindowID(), id1)
	}
}

func TestWindowManager_NextWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("first")
	id2 := wm.NewWindow("second")
	id3 := wm.NewWindow("third")

	if wm.ActiveWindowID() != id1 {
		t.Fatalf("initial active = %d, want %d", wm.ActiveWindowID(), id1)
	}

	next := wm.NextWindow()
	if next != id2 {
		t.Errorf("NextWindow = %d, want %d", next, id2)
	}

	next = wm.NextWindow()
	if next != id3 {
		t.Errorf("NextWindow = %d, want %d", next, id3)
	}

	next = wm.NextWindow()
	if next != id1 {
		t.Errorf("NextWindow should wrap, got %d, want %d", next, id1)
	}
}

func TestWindowManager_PrevWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("first")
	id2 := wm.NewWindow("second")

	if wm.ActiveWindowID() != id1 {
		t.Fatalf("initial active = %d, want %d", wm.ActiveWindowID(), id1)
	}

	prev := wm.PrevWindow()
	if prev != id2 {
		t.Errorf("PrevWindow should wrap backwards, got %d, want %d", prev, id2)
	}

	prev = wm.PrevWindow()
	if prev != id1 {
		t.Errorf("PrevWindow = %d, want %d", prev, id1)
	}
}

func TestWindowManager_NextWindow_SingleWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id := wm.NewWindow("only")
	next := wm.NextWindow()
	if next != id {
		t.Errorf("NextWindow with single window = %d, want %d", next, id)
	}
}

func TestWindowManager_RenameWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id := wm.NewWindow("old")
	if err := wm.RenameWindow(id, "new"); err != nil {
		t.Fatalf("RenameWindow error: %v", err)
	}

	w := wm.Window(id)
	if w.Name != "new" {
		t.Errorf("Window.Name = %q, want %q", w.Name, "new")
	}
}

func TestWindowManager_RenameWindow_NotFound(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	if err := wm.RenameWindow(999, "new"); err == nil {
		t.Error("RenameWindow should return error for non-existent window")
	}
}

func TestWindowManager_CloseWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("first")
	id2 := wm.NewWindow("second")

	wm.NextWindow()

	if err := wm.CloseWindow(id2); err != nil {
		t.Fatalf("CloseWindow error: %v", err)
	}

	if wm.Window(id2) != nil {
		t.Error("Window should be nil after closing")
	}

	if wm.ActiveWindowID() != id1 {
		t.Errorf("ActiveWindowID = %d, want %d (fell back to first)", wm.ActiveWindowID(), id1)
	}

	windows := wm.Windows()
	if len(windows) != 1 {
		t.Errorf("len(Windows) = %d, want 1", len(windows))
	}
}

func TestWindowManager_CloseWindow_LastWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	wm.NewWindow("only")

	if err := wm.CloseWindow(1); err == nil {
		t.Error("CloseWindow should return error for last window")
	}
}

func TestWindowManager_CloseWindow_NotFound(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)
	wm.NewWindow("only")

	if err := wm.CloseWindow(999); err == nil {
		t.Error("CloseWindow should return error for non-existent window")
	}
}

func TestWindowManager_ActivePaneManager(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	if pm := wm.ActivePaneManager(); pm != nil {
		t.Error("ActivePaneManager should be nil when no windows exist")
	}

	id := wm.NewWindow("test")
	pm := wm.ActivePaneManager()
	if pm == nil {
		t.Fatal("ActivePaneManager is nil after creating window")
	}
	_ = id
}

func TestWindowManager_SetSize(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	wid := wm.NewWindow("test")
	pm := wm.Window(wid).paneMgr

	sid := SessionID(1)
	pid, _ := pm.Create(sid, SplitRight)
	_ = pid

	panes := pm.Panes()
	if len(panes) == 0 {
		t.Fatal("no panes after Create")
	}
	initialWidth := panes[0].Geometry.Cols
	initialHeight := panes[0].Geometry.Rows

	wm.SetSize(120, 40)

	panes2 := pm.Panes()
	if len(panes2) == 0 {
		t.Fatal("no panes after SetSize")
	}
	newWidth := panes2[0].Geometry.Cols
	newHeight := panes2[0].Geometry.Rows

	if newWidth <= initialWidth && newHeight <= initialHeight {
		t.Errorf("geometry did not grow: %dx%d -> %dx%d", initialWidth, initialHeight, newWidth, newHeight)
	}
}

func TestWindowManager_WindowOrder(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("a")
	id2 := wm.NewWindow("b")
	id3 := wm.NewWindow("c")

	windows := wm.Windows()
	if len(windows) != 3 {
		t.Fatalf("len(Windows) = %d, want 3", len(windows))
	}
	if windows[0].ID != id1 {
		t.Errorf("Windows[0].ID = %d, want %d", windows[0].ID, id1)
	}
	if windows[1].ID != id2 {
		t.Errorf("Windows[1].ID = %d, want %d", windows[1].ID, id2)
	}
	if windows[2].ID != id3 {
		t.Errorf("Windows[2].ID = %d, want %d", windows[2].ID, id3)
	}
}

func TestWindow_Created(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	before := time.Now()
	id := wm.NewWindow("test")
	after := time.Now()

	w := wm.Window(id)
	if w.created.Before(before) || w.created.After(after) {
		t.Errorf("Window.created = %v, want between %v and %v", w.created, before, after)
	}
}

func TestWindowManager_MoveWindow(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("a")
	id2 := wm.NewWindow("b")
	id3 := wm.NewWindow("c")

	if err := wm.MoveWindow(id3, 0); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	windows := wm.Windows()
	if windows[0].ID != id3 || windows[1].ID != id1 || windows[2].ID != id2 {
		t.Errorf("MoveWindow to 0: got %v, want [%d %d %d]", windowIDs(windows), id3, id1, id2)
	}

	if err := wm.MoveWindow(id3, 1); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	windows = wm.Windows()
	if windows[0].ID != id1 || windows[1].ID != id3 || windows[2].ID != id2 {
		t.Errorf("MoveWindow to 1: got %v, want [%d %d %d]", windowIDs(windows), id1, id3, id2)
	}

	if err := wm.MoveWindow(id3, 2); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	windows = wm.Windows()
	if windows[0].ID != id1 || windows[1].ID != id2 || windows[2].ID != id3 {
		t.Errorf("MoveWindow to end: got %v, want [%d %d %d]", windowIDs(windows), id1, id2, id3)
	}

	if err := wm.MoveWindow(id1, 0); err != nil {
		t.Errorf("MoveWindow same index returned error: %v", err)
	}

	if err := wm.MoveWindow(999, 0); err == nil {
		t.Error("MoveWindow: expected error for missing window")
	}
	if err := wm.MoveWindow(id1, -1); err == nil {
		t.Error("MoveWindow: expected error for negative index")
	}
	if err := wm.MoveWindow(id1, 3); err == nil {
		t.Error("MoveWindow: expected error for out-of-range index")
	}

	if wm.ActiveWindowID() != id1 {
		t.Errorf("ActiveWindowID = %d, want %d", wm.ActiveWindowID(), id1)
	}
}

func TestWindowManager_SwapWindows(t *testing.T) {
	wm := NewWindowManager(LayoutTiled, 80, 24)

	id1 := wm.NewWindow("a")
	id2 := wm.NewWindow("b")
	id3 := wm.NewWindow("c")

	if err := wm.SwapWindows(id1, id3); err != nil {
		t.Fatalf("SwapWindows: %v", err)
	}
	windows := wm.Windows()
	if windows[0].ID != id3 || windows[1].ID != id2 || windows[2].ID != id1 {
		t.Errorf("SwapWindows: got %v, want [%d %d %d]", windowIDs(windows), id3, id2, id1)
	}

	if err := wm.SwapWindows(id2, id2); err != nil {
		t.Errorf("SwapWindows same ID returned error: %v", err)
	}
	windows = wm.Windows()
	if windows[0].ID != id3 || windows[1].ID != id2 || windows[2].ID != id1 {
		t.Errorf("SwapWindows same ID changed order: got %v", windowIDs(windows))
	}

	if err := wm.SwapWindows(999, id1); err == nil {
		t.Error("SwapWindows: expected error for missing first window")
	}
	if err := wm.SwapWindows(id1, 999); err == nil {
		t.Error("SwapWindows: expected error for missing second window")
	}

	if wm.ActiveWindowID() != id1 {
		t.Errorf("ActiveWindowID = %d, want %d after swap", wm.ActiveWindowID(), id1)
	}
}

func windowIDs(windows []*Window) []WindowID {
	ids := make([]WindowID, len(windows))
	for i, w := range windows {
		ids[i] = w.ID
	}
	return ids
}
