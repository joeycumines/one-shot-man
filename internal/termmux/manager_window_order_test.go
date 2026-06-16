package termmux

import (
	"testing"
)

func TestSessionManager_MoveWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, err := m.NewWindow("first")
	if err != nil {
		t.Fatalf("NewWindow 1: %v", err)
	}
	w2, err := m.NewWindow("second")
	if err != nil {
		t.Fatalf("NewWindow 2: %v", err)
	}
	w3, err := m.NewWindow("third")
	if err != nil {
		t.Fatalf("NewWindow 3: %v", err)
	}

	if err := m.MoveWindow(w3, 0); err != nil {
		t.Fatalf("MoveWindow: %v", err)
	}
	windows := m.Windows()
	if len(windows) != 3 || windows[0].ID != w3 || windows[1].ID != w1 || windows[2].ID != w2 {
		t.Errorf("MoveWindow order = %v, want [%d %d %d]", managerWindowIDs(m), w3, w1, w2)
	}

	if m.ActiveWindowID() != w1 {
		t.Errorf("ActiveWindowID = %d, want %d", m.ActiveWindowID(), w1)
	}

	if err := m.MoveWindow(w3, 2); err != nil {
		t.Fatalf("MoveWindow back: %v", err)
	}
	windows = m.Windows()
	if windows[0].ID != w1 || windows[1].ID != w2 || windows[2].ID != w3 {
		t.Errorf("MoveWindow back order = %v, want [%d %d %d]", managerWindowIDs(m), w1, w2, w3)
	}

	if err := m.MoveWindow(w1, -1); err == nil {
		t.Error("MoveWindow: expected error for negative index")
	}
	if err := m.MoveWindow(w1, 3); err == nil {
		t.Error("MoveWindow: expected error for out-of-range index")
	}
	if err := m.MoveWindow(999, 0); err == nil {
		t.Error("MoveWindow: expected error for missing window")
	}
}

func TestSessionManager_SwapWindows(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	w1, err := m.NewWindow("first")
	if err != nil {
		t.Fatalf("NewWindow 1: %v", err)
	}
	w2, err := m.NewWindow("second")
	if err != nil {
		t.Fatalf("NewWindow 2: %v", err)
	}
	w3, err := m.NewWindow("third")
	if err != nil {
		t.Fatalf("NewWindow 3: %v", err)
	}

	if err := m.SwapWindows(w1, w3); err != nil {
		t.Fatalf("SwapWindows: %v", err)
	}
	windows := m.Windows()
	if len(windows) != 3 || windows[0].ID != w3 || windows[1].ID != w2 || windows[2].ID != w1 {
		t.Errorf("SwapWindows order = %v, want [%d %d %d]", managerWindowIDs(m), w3, w2, w1)
	}

	if m.ActiveWindowID() != w1 {
		t.Errorf("ActiveWindowID = %d, want %d", m.ActiveWindowID(), w1)
	}

	if err := m.SwapWindows(999, w1); err == nil {
		t.Error("SwapWindows: expected error for missing window")
	}
}

func managerWindowIDs(m *SessionManager) []WindowID {
	ws := m.Windows()
	ids := make([]WindowID, len(ws))
	for i, w := range ws {
		ids[i] = w.ID
	}
	return ids
}
