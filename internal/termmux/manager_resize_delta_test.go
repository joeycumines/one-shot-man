package termmux

import (
	"testing"
)

func paneGeometryByID(panes []Pane, id PaneID) (PaneGeometry, bool) {
	for _, p := range panes {
		if p.ID == id {
			return p.Geometry, true
		}
	}
	return PaneGeometry{}, false
}

// newHalfHeightPane creates a root pane and then splits it horizontally,
// returning the bottom half pane and its controllable session. The resulting
// pane is 12 rows by 80 columns in a 24x80 terminal, matching the geometry
// assumptions in the ResizePaneDelta tests.
func newHalfHeightPane(t *testing.T, m *SessionManager) (*controllableSession, PaneID) {
	t.Helper()
	_, err := m.NewPane(newControllableSession(), SessionTarget{Name: "root", Kind: SessionKindPTY}, SplitRight)
	if err != nil {
		t.Fatalf("root pane: %v", err)
	}
	s := newControllableSession()
	p, err := m.NewPane(s, SessionTarget{Name: "p", Kind: SessionKindPTY}, SplitDown)
	if err != nil {
		t.Fatalf("NewPane: %v", err)
	}
	return s, p
}

func TestSessionManager_ResizePaneDelta_RightGrowsWidth(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s, p := newHalfHeightPane(t, m)

	if err := m.ResizePaneDelta(p, "right", 5); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	geo, ok := paneGeometryByID(m.Panes(), p)
	if !ok {
		t.Fatal("pane not found in Panes()")
	}
	if geo.Cols != 85 {
		t.Errorf("Cols=%d, want 85", geo.Cols)
	}
	if geo.Rows != 12 {
		t.Errorf("Rows=%d, want 12", geo.Rows)
	}

	resizes := s.Resizes()
	if len(resizes) == 0 {
		t.Fatal("expected session.Resize call")
	}
	last := resizes[len(resizes)-1]
	if last.cols != 85 || last.rows != 12 {
		t.Errorf("session resize rows=%d cols=%d, want 12,85", last.rows, last.cols)
	}
}

func TestSessionManager_ResizePaneDelta_LeftShrinksWidth(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s, p := newHalfHeightPane(t, m)

	if err := m.ResizePaneDelta(p, "left", 5); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	geo, ok := paneGeometryByID(m.Panes(), p)
	if !ok {
		t.Fatal("pane not found in Panes()")
	}
	if geo.Cols != 75 {
		t.Errorf("Cols=%d, want 75", geo.Cols)
	}

	resizes := s.Resizes()
	if len(resizes) == 0 {
		t.Fatal("expected session.Resize call")
	}
	last := resizes[len(resizes)-1]
	if last.cols != 75 {
		t.Errorf("session resize cols=%d, want 75", last.cols)
	}
}

func TestSessionManager_ResizePaneDelta_DownGrowsHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s, p := newHalfHeightPane(t, m)

	if err := m.ResizePaneDelta(p, "down", 5); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	geo, ok := paneGeometryByID(m.Panes(), p)
	if !ok {
		t.Fatal("pane not found in Panes()")
	}
	if geo.Rows != 17 {
		t.Errorf("Rows=%d, want 17", geo.Rows)
	}
	if geo.Cols != 80 {
		t.Errorf("Cols=%d, want 80", geo.Cols)
	}

	resizes := s.Resizes()
	if len(resizes) == 0 {
		t.Fatal("expected session.Resize call")
	}
	last := resizes[len(resizes)-1]
	if last.rows != 17 || last.cols != 80 {
		t.Errorf("session resize rows=%d cols=%d, want 17,80", last.rows, last.cols)
	}
}

func TestSessionManager_ResizePaneDelta_UpShrinksHeight(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	s, p := newHalfHeightPane(t, m)

	if err := m.ResizePaneDelta(p, "up", 5); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	geo, ok := paneGeometryByID(m.Panes(), p)
	if !ok {
		t.Fatal("pane not found in Panes()")
	}
	if geo.Rows != 7 {
		t.Errorf("Rows=%d, want 7", geo.Rows)
	}

	resizes := s.Resizes()
	if len(resizes) == 0 {
		t.Fatal("expected session.Resize call")
	}
	last := resizes[len(resizes)-1]
	if last.rows != 7 {
		t.Errorf("session resize rows=%d, want 7", last.rows)
	}
}

func TestSessionManager_ResizePaneDelta_ClampsAtMinimum(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	_, p := newHalfHeightPane(t, m)

	if err := m.ResizePaneDelta(p, "up", 100); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	geo, ok := paneGeometryByID(m.Panes(), p)
	if !ok {
		t.Fatal("pane not found in Panes()")
	}
	if geo.Rows != MinPaneRows {
		t.Errorf("Rows=%d, want min %d", geo.Rows, MinPaneRows)
	}

	if err := m.ResizePaneDelta(p, "left", 100); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	geo, ok = paneGeometryByID(m.Panes(), p)
	if !ok {
		t.Fatal("pane not found in Panes()")
	}
	if geo.Cols != MinPaneCols {
		t.Errorf("Cols=%d, want min %d", geo.Cols, MinPaneCols)
	}
}

func TestSessionManager_ResizePaneDelta_InvalidDirection(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	_, p := newHalfHeightPane(t, m)

	err := m.ResizePaneDelta(p, "diagonal", 5)
	if err == nil {
		t.Fatal("expected error for invalid direction")
	}
}

func TestSessionManager_ResizePaneDelta_InvalidPane(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	err := m.ResizePaneDelta(PaneID(999), "right", 5)
	if err == nil {
		t.Fatal("expected error for missing pane")
	}
}

func TestSessionManager_ResizePaneDelta_EmitsWindowUpdated(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	_, p := newHalfHeightPane(t, m)

	subID, events := m.Subscribe(16)
	defer m.Unsubscribe(subID)

	if err := m.ResizePaneDelta(p, "right", 5); err != nil {
		t.Fatalf("ResizePaneDelta: %v", err)
	}

	for evt := range events {
		if evt.Kind == EventWindowUpdated {
			return
		}
	}
	t.Error("missing EventWindowUpdated after ResizePaneDelta")
}
