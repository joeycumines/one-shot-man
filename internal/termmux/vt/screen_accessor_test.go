package vt

import "testing"

func TestRowCount(t *testing.T) {
	s := NewScreen(24, 80)
	if got := s.RowCount(); got != 24 {
		t.Errorf("RowCount() = %d, want 24", got)
	}
}

func TestColCount(t *testing.T) {
	s := NewScreen(24, 80)
	if got := s.ColCount(); got != 80 {
		t.Errorf("ColCount() = %d, want 80", got)
	}
}

func TestCursorPosition(t *testing.T) {
	s := NewScreen(24, 80)
	row, col := s.CursorPosition()
	if row != 0 || col != 0 {
		t.Errorf("CursorPosition() = %d,%d, want 0,0", row, col)
	}
	s.CurRow = 5
	s.CurCol = 10
	row, col = s.CursorPosition()
	if row != 5 || col != 10 {
		t.Errorf("CursorPosition() = %d,%d, want 5,10", row, col)
	}
}

func TestSetCursor_clampsNegative(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetCursor(-5, -3)
	row, col := s.CursorPosition()
	if row != 0 || col != 0 {
		t.Errorf("SetCursor(-5,-3) = %d,%d, want 0,0", row, col)
	}
}

func TestSetCursor_clampsOverflow(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetCursor(100, 200)
	row, col := s.CursorPosition()
	if row != 23 || col != 79 {
		t.Errorf("SetCursor(100,200) = %d,%d, want 23,79", row, col)
	}
}

func TestSetCursor_clearsPendingWrap(t *testing.T) {
	s := NewScreen(24, 80)
	s.PendingWrap = true
	s.SetCursor(0, 0)
	if s.PendingWrap {
		t.Error("SetCursor should clear PendingWrap")
	}
}

func TestSetCursor_validPosition(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetCursor(10, 40)
	row, col := s.CursorPosition()
	if row != 10 || col != 40 {
		t.Errorf("SetCursor(10,40) = %d,%d, want 10,40", row, col)
	}
}

func TestSetScrollRegion_valid(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(5, 20)
	if s.ScrollTop != 5 || s.ScrollBot != 20 {
		t.Errorf("SetScrollRegion(5,20) = %d,%d, want 5,20", s.ScrollTop, s.ScrollBot)
	}
}

func TestSetScrollRegion_topEqualsBottom(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(10, 10)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("SetScrollRegion(10,10) = %d,%d, want 0,0 (reset)", s.ScrollTop, s.ScrollBot)
	}
}

func TestSetScrollRegion_topGreaterThanBottom(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(20, 10)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("SetScrollRegion(20,10) = %d,%d, want 0,0 (reset)", s.ScrollTop, s.ScrollBot)
	}
}

func TestSetScrollRegion_topLessThanOne(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(0, 20)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("SetScrollRegion(0,20) = %d,%d, want 0,0 (reset)", s.ScrollTop, s.ScrollBot)
	}
}

func TestSetScrollRegion_bottomExceedsRows(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(1, 30)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Errorf("SetScrollRegion(1,30) = %d,%d, want 0,0 (reset)", s.ScrollTop, s.ScrollBot)
	}
}

func TestMouseTrackingMode(t *testing.T) {
	s := NewScreen(24, 80)
	if got := s.MouseTrackingMode(); got != MouseTrackingNone {
		t.Errorf("MouseTrackingMode() = %d, want None", got)
	}
	s.MouseTracking = MouseTrackingBasic
	if got := s.MouseTrackingMode(); got != MouseTrackingBasic {
		t.Errorf("MouseTrackingMode() = %d, want Basic", got)
	}
}

func TestSetMouseTracking_validValues(t *testing.T) {
	s := NewScreen(24, 80)
	for _, m := range []MouseTrackingMode{
		MouseTrackingNone,
		MouseTrackingBasic,
		MouseTrackingButtonEvent,
		MouseTrackingAnyEvent,
	} {
		s.SetMouseTracking(m)
		if got := s.MouseTrackingMode(); got != m {
			t.Errorf("SetMouseTracking(%d): got %d", m, got)
		}
	}
}

func TestSetMouseTracking_clampsNegative(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetMouseTracking(-1)
	if got := s.MouseTrackingMode(); got != MouseTrackingNone {
		t.Errorf("SetMouseTracking(-1) = %d, want None (0)", got)
	}
}

func TestSetMouseTracking_clampsOverflow(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetMouseTracking(100)
	if got := s.MouseTrackingMode(); got != MouseTrackingAnyEvent {
		t.Errorf("SetMouseTracking(100) = %d, want AnyEvent (3)", got)
	}
}

func TestInScrollRegion(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(5, 20)
	tests := []struct {
		row  int
		want bool
	}{
		{3, false},
		{4, true},
		{10, true},
		{19, true},
		{20, false},
		{23, false},
	}
	for _, tt := range tests {
		if got := s.InScrollRegion(tt.row); got != tt.want {
			t.Errorf("InScrollRegion(%d) = %v, want %v", tt.row, got, tt.want)
		}
	}
}

func TestInScrollRegion_noRegionSet(t *testing.T) {
	s := NewScreen(24, 80)
	if !s.InScrollRegion(0) {
		t.Error("InScrollRegion(0) with no region should be true (full screen)")
	}
	if !s.InScrollRegion(23) {
		t.Error("InScrollRegion(23) with no region should be true (full screen)")
	}
}

func TestCellAt_valid(t *testing.T) {
	s := NewScreen(24, 80)
	s.Cells[5][10] = Cell{Ch: 'X'}
	cell := s.CellAt(5, 10)
	if cell.Ch != 'X' {
		t.Errorf("CellAt(5,10).Ch = %q, want 'X'", cell.Ch)
	}
}

func TestCellAt_outOfBounds(t *testing.T) {
	s := NewScreen(24, 80)
	cell := s.CellAt(-1, 0)
	if cell != DefaultCell() {
		t.Error("CellAt(-1,0) should return DefaultCell()")
	}
	cell = s.CellAt(0, -1)
	if cell != DefaultCell() {
		t.Error("CellAt(0,-1) should return DefaultCell()")
	}
	cell = s.CellAt(24, 0)
	if cell != DefaultCell() {
		t.Error("CellAt(24,0) should return DefaultCell()")
	}
	cell = s.CellAt(0, 80)
	if cell != DefaultCell() {
		t.Error("CellAt(0,80) should return DefaultCell()")
	}
}

func TestSetCell_valid(t *testing.T) {
	s := NewScreen(24, 80)
	c := Cell{Ch: 'Z'}
	s.SetCell(5, 10, c)
	if s.Cells[5][10].Ch != 'Z' {
		t.Errorf("SetCell(5,10,'Z'): Cells[5][10].Ch = %q", s.Cells[5][10].Ch)
	}
}

func TestSetCell_outOfBounds(t *testing.T) {
	s := NewScreen(24, 80)
	c := Cell{Ch: 'Z'}
	s.SetCell(-1, 0, c)
	s.SetCell(0, -1, c)
	s.SetCell(24, 0, c)
	s.SetCell(0, 80, c)
}

func TestSetCell_marksDirty(t *testing.T) {
	s := NewScreen(24, 80)
	s.ClearDirty()
	s.SetCell(5, 10, Cell{Ch: 'Z'})
	min, max := s.DirtyRange()
	if min != 5 || max != 5 {
		t.Errorf("SetCell should mark row dirty: got [%d,%d], want [5,5]", min, max)
	}
}

func TestTabStopAt_valid(t *testing.T) {
	s := NewScreen(24, 80)
	if !s.TabStopAt(0) {
		t.Error("TabStopAt(0) should be true (default tab stop)")
	}
	if !s.TabStopAt(8) {
		t.Error("TabStopAt(8) should be true (default tab stop)")
	}
	if s.TabStopAt(1) {
		t.Error("TabStopAt(1) should be false (not a default tab stop)")
	}
}

func TestTabStopAt_outOfBounds(t *testing.T) {
	s := NewScreen(24, 80)
	if s.TabStopAt(-1) {
		t.Error("TabStopAt(-1) should be false")
	}
	if s.TabStopAt(80) {
		t.Error("TabStopAt(80) should be false")
	}
}
