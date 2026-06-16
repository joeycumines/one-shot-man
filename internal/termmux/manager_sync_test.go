package termmux

import (
	"testing"
)

func addPanesToWindow(t *testing.T, m *SessionManager, w WindowID, nameA, nameB string) (*controllableSession, *controllableSession) {
	t.Helper()
	s1 := newControllableSession()
	s2 := newControllableSession()

	if _, err := m.AddPaneToWindow(s1, SessionTarget{Name: nameA, Kind: SessionKindPTY}, w, SplitRight); err != nil {
		t.Fatalf("AddPaneToWindow %s: %v", nameA, err)
	}
	if _, err := m.AddPaneToWindow(s2, SessionTarget{Name: nameB, Kind: SessionKindPTY}, w, SplitRight); err != nil {
		t.Fatalf("AddPaneToWindow %s: %v", nameB, err)
	}

	return s1, s2
}

func TestSynchronizePanes_PerWindow_DefaultOff(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	wid, err := m.NewWindow("sync-default")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	addPanesToWindow(t, m, wid, "a1", "a2")

	m.NextWindow()
	if m.SynchronizePanes() {
		t.Error("SynchronizePanes = true, want false by default")
	}
}

func TestSynchronizePanes_PerWindow_BroadcastsOnlyActiveWindow(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	wA, err := m.NewWindow("sync-A")
	if err != nil {
		t.Fatalf("NewWindow A: %v", err)
	}
	wB, err := m.NewWindow("sync-B")
	if err != nil {
		t.Fatalf("NewWindow B: %v", err)
	}

	a1, a2 := addPanesToWindow(t, m, wA, "a1", "a2")
	b1, b2 := addPanesToWindow(t, m, wB, "b1", "b2")

	m.NextWindow()
	m.NextWindow() // return to window A as the SessionManager active window

	if err := m.SetSynchronizePanes(true); err != nil {
		t.Fatalf("SetSynchronizePanes(true): %v", err)
	}

	if err := m.Input([]byte("x")); err != nil {
		t.Fatalf("Input: %v", err)
	}

	if got := string(a1.Written()); got != "x" {
		t.Errorf("a1.Written = %q, want %q", got, "x")
	}
	if got := string(a2.Written()); got != "x" {
		t.Errorf("a2.Written = %q, want %q", got, "x")
	}
	if got := string(b1.Written()); got != "" {
		t.Errorf("b1.Written = %q, want empty", got)
	}
	if got := string(b2.Written()); got != "" {
		t.Errorf("b2.Written = %q, want empty", got)
	}
}

func TestSynchronizePanes_PerWindow_SwitchingChangesTarget(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	wA, err := m.NewWindow("switch-A")
	if err != nil {
		t.Fatalf("NewWindow A: %v", err)
	}
	wB, err := m.NewWindow("switch-B")
	if err != nil {
		t.Fatalf("NewWindow B: %v", err)
	}

	a1, a2 := addPanesToWindow(t, m, wA, "a1", "a2")
	b1, b2 := addPanesToWindow(t, m, wB, "b1", "b2")

	m.NextWindow()
	m.NextWindow() // return to window A as the SessionManager active window

	if err := m.SetSynchronizePanes(true); err != nil {
		t.Fatalf("SetSynchronizePanes(true): %v", err)
	}

	m.NextWindow()
	if err := m.SetSynchronizePanes(true); err != nil {
		t.Fatalf("SetSynchronizePanes(true) on B: %v", err)
	}

	if err := m.Input([]byte("after-switch")); err != nil {
		t.Fatalf("Input after switch: %v", err)
	}

	if got := string(b1.Written()); got != "after-switch" {
		t.Errorf("b1.Written = %q, want %q", got, "after-switch")
	}
	if got := string(b2.Written()); got != "after-switch" {
		t.Errorf("b2.Written = %q, want %q", got, "after-switch")
	}
	if got := string(a1.Written()); got != "" {
		t.Errorf("a1.Written = %q, want empty", got)
	}
	if got := string(a2.Written()); got != "" {
		t.Errorf("a2.Written = %q, want empty", got)
	}

	m.PrevWindow()
	if err := m.Input([]byte("back-to-a")); err != nil {
		t.Fatalf("Input back to A: %v", err)
	}

	if got := string(a1.Written()); got != "back-to-a" {
		t.Errorf("a1.Written = %q, want %q", got, "back-to-a")
	}
	if got := string(a2.Written()); got != "back-to-a" {
		t.Errorf("a2.Written = %q, want %q", got, "back-to-a")
	}
}

func TestSynchronizePanes_PerWindow_DisableReturnsToSinglePane(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	wA, err := m.NewWindow("disable-A")
	if err != nil {
		t.Fatalf("NewWindow A: %v", err)
	}
	wB, err := m.NewWindow("disable-B")
	if err != nil {
		t.Fatalf("NewWindow B: %v", err)
	}

	a1, a2 := addPanesToWindow(t, m, wA, "a1", "a2")
	_, _ = addPanesToWindow(t, m, wB, "b1", "b2")

	m.NextWindow()
	m.NextWindow() // return to window A as the SessionManager active window

	if err := m.SetSynchronizePanes(true); err != nil {
		t.Fatalf("SetSynchronizePanes(true): %v", err)
	}

	if err := m.Input([]byte("broadcast")); err != nil {
		t.Fatalf("Input broadcast: %v", err)
	}
	if got := string(a1.Written()); got != "broadcast" {
		t.Errorf("a1.Written = %q, want %q", got, "broadcast")
	}
	if got := string(a2.Written()); got != "broadcast" {
		t.Errorf("a2.Written = %q, want %q", got, "broadcast")
	}

	if err := m.SetSynchronizePanes(false); err != nil {
		t.Fatalf("SetSynchronizePanes(false): %v", err)
	}
	if err := m.Input([]byte("single")); err != nil {
		t.Fatalf("Input single: %v", err)
	}

	activeA := m.ActiveID()
	var activeWritten, inactiveWritten []byte
	if activeA == 1 {
		activeWritten = a1.Written()
		inactiveWritten = a2.Written()
	} else {
		activeWritten = a2.Written()
		inactiveWritten = a1.Written()
	}
	if !endsWith(activeWritten, "single") && !endsWith(activeWritten, "broadcastsingle") {
		t.Errorf("active pane %d missing single input, got %q", activeA, activeWritten)
	}
	if endsWith(inactiveWritten, "single") {
		t.Errorf("inactive pane should not receive single input, got %q", inactiveWritten)
	}
}

func endsWith(data []byte, sub string) bool {
	if len(data) < len(sub) {
		return false
	}
	return string(data[len(data)-len(sub):]) == sub
}

func TestSynchronizePanes_PerWindow_IndependentAcrossWindows(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	wA, err := m.NewWindow("indep-A")
	if err != nil {
		t.Fatalf("NewWindow A: %v", err)
	}
	wB, err := m.NewWindow("indep-B")
	if err != nil {
		t.Fatalf("NewWindow B: %v", err)
	}

	_, _ = addPanesToWindow(t, m, wA, "a1", "a2")
	b1, b2 := addPanesToWindow(t, m, wB, "b1", "b2")

	m.NextWindow()
	m.NextWindow() // return to window A as the SessionManager active window

	if err := m.SetSynchronizePanes(true); err != nil {
		t.Fatalf("SetSynchronizePanes(true) on A: %v", err)
	}

	m.NextWindow()
	if m.SynchronizePanes() {
		t.Error("window B should not inherit window A synchronize state")
	}

	if err := m.Input([]byte("only-b1")); err != nil {
		t.Fatalf("Input: %v", err)
	}
	if got := string(b1.Written()); got != "only-b1" {
		t.Errorf("b1.Written = %q, want %q", got, "only-b1")
	}
	if got := string(b2.Written()); got != "" {
		t.Errorf("b2.Written = %q, want empty (sync off on B)", got)
	}

	m.PrevWindow() // back to wA
	if !m.SynchronizePanes() {
		t.Error("window A should still have synchronize enabled after returning")
	}
}

func TestSynchronizePanes_WindowManagerActiveAccessor(t *testing.T) {
	t.Parallel()

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	wid, err := m.NewWindow("accessor")
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	_ = wid

	wm := m.windowMgr
	if wm.Active() == nil {
		t.Fatal("Active() returned nil for the first window")
	}
	if wm.Active().Synchronize() {
		t.Error("new window should have synchronization disabled")
	}

	wm.Active().SetSynchronize(true)
	if !wm.Active().Synchronize() {
		t.Error("Window.SetSynchronize(true) did not take effect")
	}
}
