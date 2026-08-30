package termmux

import (
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func TestPaneIDZeroIsSentinel(t *testing.T) {
	var id PaneID
	if id != 0 {
		t.Fatalf("zero-value PaneID should be 0, got %d", id)
	}
}

func TestPaneIDNonZeroIsValid(t *testing.T) {
	id := PaneID(42)
	if id == 0 {
		t.Fatal("non-zero PaneID should not equal 0")
	}
}

func TestPaneZeroValueIsInvalid(t *testing.T) {
	var p Pane
	if p.IsValid() {
		t.Fatal("zero-value Pane should not be valid")
	}
}

func TestPaneWithIDIsValid(t *testing.T) {
	p := Pane{ID: PaneID(1)}
	if !p.IsValid() {
		t.Fatal("Pane with non-zero ID should be valid")
	}
}

func TestPaneFields(t *testing.T) {
	now := time.Now()
	term := vt.NewVTerm(24, 80)
	p := Pane{
		ID:          PaneID(7),
		SessionID:   SessionID(3),
		Geometry:    PaneGeometry{Row: 0, Col: 0, Rows: 24, Cols: 80},
		Title:       "main",
		Focus:       true,
		BorderStyle: "single",
		VTerm:       term,
		LastActive:  now,
	}

	if p.ID != 7 {
		t.Errorf("ID = %d, want 7", p.ID)
	}
	if p.SessionID != 3 {
		t.Errorf("SessionID = %d, want 3", p.SessionID)
	}
	if p.Geometry.Rows != 24 || p.Geometry.Cols != 80 {
		t.Errorf("Geometry = %+v, want Rows=24 Cols=80", p.Geometry)
	}
	if p.Title != "main" {
		t.Errorf("Title = %q, want %q", p.Title, "main")
	}
	if !p.Focus {
		t.Error("Focus = false, want true")
	}
	if p.BorderStyle != "single" {
		t.Errorf("BorderStyle = %q, want %q", p.BorderStyle, "single")
	}
	if p.VTerm != term {
		t.Error("VTerm pointer mismatch")
	}
	if !p.LastActive.Equal(now) {
		t.Errorf("LastActive = %v, want %v", p.LastActive, now)
	}
	if !p.IsValid() {
		t.Error("Pane with non-zero ID should be valid")
	}
}

func TestSplitDirectionValues(t *testing.T) {
	if SplitRight != 0 {
		t.Errorf("SplitRight = %d, want 0", SplitRight)
	}
	if SplitDown != 1 {
		t.Errorf("SplitDown = %d, want 1", SplitDown)
	}
	if SplitLeft != 2 {
		t.Errorf("SplitLeft = %d, want 2", SplitLeft)
	}
	if SplitUp != 3 {
		t.Errorf("SplitUp = %d, want 3", SplitUp)
	}
}

func TestNavigationDirectionValues(t *testing.T) {
	if NavNext != 0 {
		t.Errorf("NavNext = %d, want 0", NavNext)
	}
	if NavPrev != 1 {
		t.Errorf("NavPrev = %d, want 1", NavPrev)
	}
	if NavUp != 2 {
		t.Errorf("NavUp = %d, want 2", NavUp)
	}
	if NavDown != 3 {
		t.Errorf("NavDown = %d, want 3", NavDown)
	}
	if NavLeft != 4 {
		t.Errorf("NavLeft = %d, want 4", NavLeft)
	}
	if NavRight != 5 {
		t.Errorf("NavRight = %d, want 5", NavRight)
	}
}
