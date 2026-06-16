package termmux

import (
	"strings"
	"testing"
)

func TestPrefixDispatcher_UnboundKey(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	res, err := d.Handle("q")
	if err != nil {
		t.Fatalf("Handle(q): %v", err)
	}
	if res.Consumed {
		t.Fatalf("expected unbound key to not be consumed")
	}
	if res.Action.Kind != PrefixActionNone {
		t.Fatalf("expected action None, got %v", res.Action.Kind)
	}
}

func TestPrefixDispatcher_NewWindow(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	res, err := d.Handle("c")
	if err != nil {
		t.Fatalf("Handle(c): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected consumed")
	}
	if res.Action.Kind != PrefixActionNewWindow {
		t.Fatalf("expected NewWindow, got %v", res.Action.Kind)
	}
	wid, ok := res.Result.(uint64)
	if !ok || wid == 0 {
		t.Fatalf("expected non-zero window ID, got %v", res.Result)
	}
	if m.ActiveWindowID() != WindowID(wid) {
		t.Fatalf("active window %d != %d", m.ActiveWindowID(), wid)
	}
	panes := m.Panes()
	if len(panes) != 1 {
		t.Fatalf("expected 1 pane, got %d", len(panes))
	}
}

func TestPrefixDispatcher_NextPrevWindow(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))

	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}

	first := m.ActiveWindowID()
	res, err := d.Handle("n")
	if err != nil {
		t.Fatalf("Handle(n): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected n consumed")
	}
	nextID, ok := res.Result.(uint64)
	if !ok || WindowID(nextID) == first {
		t.Fatalf("next window did not change: first=%d next=%d", first, nextID)
	}

	res, err = d.Handle("p")
	if err != nil {
		t.Fatalf("Handle(p): %v", err)
	}
	prevID, ok := res.Result.(uint64)
	if !ok || WindowID(prevID) != first {
		t.Fatalf("prev window did not return to first: got %d want %d", prevID, first)
	}
}

func TestPrefixDispatcher_RenameWindow(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}
	wid := m.ActiveWindowID()

	res, err := d.Handle(",")
	if err != nil {
		t.Fatalf("Handle(,): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected rename consumed")
	}
	found := false
	for _, w := range m.Windows() {
		if w.ID == wid {
			found = true
			if w.Name != "renamed" {
				t.Fatalf("expected window name %q, got %q", "renamed", w.Name)
			}
		}
	}
	if !found {
		t.Fatalf("window %d not found", wid)
	}
}

func TestPrefixDispatcher_ClosePane(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}
	if len(m.Panes()) != 1 {
		t.Fatalf("expected 1 pane before close")
	}

	res, err := d.Handle("x")
	if err != nil {
		t.Fatalf("Handle(x): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected close pane consumed")
	}
	if len(m.Panes()) != 0 {
		t.Fatalf("expected 0 panes after close, got %d", len(m.Panes()))
	}
}

func TestPrefixDispatcher_SplitPaneHorizontal(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}

	res, err := d.Handle("%")
	if err != nil {
		t.Fatalf("Handle(%%): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected split consumed")
	}
	pid, ok := res.Result.(uint64)
	if !ok || pid == 0 {
		t.Fatalf("expected non-zero pane ID, got %v", res.Result)
	}
	if len(m.Panes()) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(m.Panes()))
	}
}

func TestPrefixDispatcher_SplitPaneVertical(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}

	res, err := d.Handle("\"")
	if err != nil {
		t.Fatalf("Handle(\"): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected split consumed")
	}
	if res.Result == nil {
		t.Fatalf("expected pane ID")
	}
	if len(m.Panes()) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(m.Panes()))
	}
}

func TestPrefixDispatcher_EnterCopyMode(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}

	res, err := d.Handle("[")
	if err != nil {
		t.Fatalf("Handle([): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected copy mode consumed")
	}
	if !m.IsCopyModeActive(m.ActiveID()) {
		t.Fatalf("expected copy mode active")
	}
}

func TestPrefixDispatcher_ZoomPane(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	if _, err := d.Handle("c"); err != nil {
		t.Fatalf("new window: %v", err)
	}
	if _, err := d.Handle("%"); err != nil {
		t.Fatalf("split: %v", err)
	}

	res, err := d.Handle("z")
	if err != nil {
		t.Fatalf("Handle(z): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected zoom consumed")
	}
	if m.ZoomedPane() == 0 {
		t.Fatalf("expected a zoomed pane")
	}
}

func TestPrefixDispatcher_ListKeys(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	d := NewPrefixDispatcher(m, NewPrefixKeyHandler(""))
	res, err := d.Handle("?")
	if err != nil {
		t.Fatalf("Handle(?): %v", err)
	}
	if !res.Consumed {
		t.Fatalf("expected list keys consumed")
	}
	if res.ListKeys == "" {
		t.Fatalf("expected non-empty list keys")
	}
	if res.Action.Kind != PrefixActionListKeys {
		t.Fatalf("expected ListKeys action")
	}
	want := []string{"NewWindow", "NextWindow", "PrevWindow"}
	for _, w := range want {
		if !strings.Contains(res.ListKeys, w) {
			t.Fatalf("list keys missing %q: %s", w, res.ListKeys)
		}
	}
}
