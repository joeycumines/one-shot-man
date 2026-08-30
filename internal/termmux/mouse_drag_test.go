package termmux

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func setupMouseDragTwoPanes(t *testing.T) (*SessionManager, func()) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	m, cleanup := startManager(t, WithTermSize(10, 40))
	if err := m.Resize(10, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	s1 := newControllableSession()
	if _, err := m.NewPane(s1, SessionTarget{Name: "s1", Kind: SessionKindPTY}, SplitRight); err != nil {
		t.Fatalf("NewPane 1: %v", err)
	}
	s2 := newControllableSession()
	if _, err := m.NewPane(s2, SessionTarget{Name: "s2", Kind: SessionKindPTY}, SplitRight); err != nil {
		t.Fatalf("NewPane 2: %v", err)
	}
	return m, cleanup
}

func TestMouseDrag_DownOnDivider(t *testing.T) {
	m, cleanup := setupMouseDragTwoPanes(t)
	defer cleanup()

	d := NewMouseDrag()
	handled, _, _ := d.Handle(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft}, m)
	if !handled {
		t.Fatal("expected mouse down on divider to be handled")
	}

	// Confirm the drag is active by checking that a subsequent motion on the
	// divider is also consumed.
	handled, _, _ = d.Handle(tea.MouseMotionMsg{X: 5, Y: 6, Button: tea.MouseLeft}, m)
	if !handled {
		t.Fatal("expected motion during active drag to be handled")
	}
}

func TestMouseDrag_MotionResizes(t *testing.T) {
	m, cleanup := setupMouseDragTwoPanes(t)
	defer cleanup()

	panesBefore := m.Panes()
	var topBefore Pane
	for _, p := range panesBefore {
		if p.ID == 1 {
			topBefore = p
			break
		}
	}
	if topBefore.Geometry.Rows == 0 {
		t.Fatal("top pane has no geometry")
	}

	d := NewMouseDrag()
	d.Handle(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft}, m)
	handled, _, _ := d.Handle(tea.MouseMotionMsg{X: 5, Y: 6, Button: tea.MouseLeft}, m)
	if !handled {
		t.Fatal("expected motion during drag to be handled")
	}

	panesAfter := m.Panes()
	var topAfter Pane
	for _, p := range panesAfter {
		if p.ID == 1 {
			topAfter = p
			break
		}
	}
	if topAfter.Geometry.Rows >= topBefore.Geometry.Rows {
		t.Errorf("expected top pane height to shrink, before=%d after=%d", topBefore.Geometry.Rows, topAfter.Geometry.Rows)
	}
}

func TestMouseDrag_ReleaseEnds(t *testing.T) {
	m, cleanup := setupMouseDragTwoPanes(t)
	defer cleanup()

	d := NewMouseDrag()
	d.Handle(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft}, m)
	handled, _, _ := d.Handle(tea.MouseReleaseMsg{X: 5, Y: 6, Button: tea.MouseLeft}, m)
	if !handled {
		t.Error("expected release during drag to be handled")
	}

	// After release a new motion should be ignored.
	handled, _, _ = d.Handle(tea.MouseMotionMsg{X: 5, Y: 6, Button: tea.MouseLeft}, m)
	if handled {
		t.Error("expected motion after release to be ignored")
	}
}

func TestMouseDrag_NonDividerIgnored(t *testing.T) {
	m, cleanup := setupMouseDragTwoPanes(t)
	defer cleanup()

	d := NewMouseDrag()
	handled, _, _ := d.Handle(tea.MouseClickMsg{X: 5, Y: 2, Button: tea.MouseLeft}, m)
	if handled {
		t.Error("expected click not on a divider to be ignored")
	}
}

func TestMouseDrag_WrongButtonIgnored(t *testing.T) {
	m, cleanup := setupMouseDragTwoPanes(t)
	defer cleanup()

	d := NewMouseDrag()
	handled, _, _ := d.Handle(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseRight}, m)
	if handled {
		t.Error("expected non-left click on divider to be ignored")
	}
}

func TestMouseDrag_MotionWithoutDragIgnored(t *testing.T) {
	m, cleanup := setupMouseDragTwoPanes(t)
	defer cleanup()

	d := NewMouseDrag()
	handled, _, _ := d.Handle(tea.MouseMotionMsg{X: 5, Y: 5, Button: tea.MouseLeft}, m)
	if handled {
		t.Error("expected motion without active drag to be ignored")
	}
}
