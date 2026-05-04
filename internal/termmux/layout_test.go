package termmux

import (
	"testing"
)

func TestSplitLayout_Basic(t *testing.T) {
	l := SplitLayout{
		TotalChromeRows:      8, // topHeader(2) + bottomHeader(2) + external(4)
		TopPaneHeaderRows:    2, // title bar + divider
		DividerRows:          1, // pane divider (within viewport budget)
		BottomPaneHeaderRows: 2, // border top + title line
		LeftChromeCol:        1,
		MinPaneRows:          3,
	}
	// 40 rows total, 60% top ratio
	top, bottom := l.Compute(40, 80, 0.6)

	// Top pane starts at row 2 (after header chrome).
	if top.Row != 2 {
		t.Errorf("top.Row=%d, want 2", top.Row)
	}
	if top.Cols != 80 {
		t.Errorf("top.Cols=%d, want 80", top.Cols)
	}
	// vpHeight = 40-8=32; topH = floor(32*0.6) = 19
	if top.Rows != 19 {
		t.Errorf("top.Rows=%d, want 19", top.Rows)
	}

	// Bottom pane content starts after topHeader + topH + divider + bottomHeader.
	expectedBottomRow := 2 + 19 + 1 + 2
	if bottom.Row != expectedBottomRow {
		t.Errorf("bottom.Row=%d, want %d", bottom.Row, expectedBottomRow)
	}
	// Bottom content cols = 80 - 1 = 79
	if bottom.Cols != 79 {
		t.Errorf("bottom.Cols=%d, want 79", bottom.Cols)
	}
	// Bottom content height = 32 - 19 - 1 = 12
	if bottom.Rows != 12 {
		t.Errorf("bottom.Rows=%d, want 12", bottom.Rows)
	}
}

func TestSplitLayout_MinPaneEnforced(t *testing.T) {
	l := SplitLayout{
		TotalChromeRows:      8,
		TopPaneHeaderRows:    2,
		DividerRows:          1,
		BottomPaneHeaderRows: 2,
		LeftChromeCol:        1,
		MinPaneRows:          3,
	}
	// Very small terminal: 10 rows
	top, bottom := l.Compute(10, 40, 0.9)

	if top.Rows < 3 {
		t.Errorf("top.Rows=%d, should be >= minPaneRows (3)", top.Rows)
	}
	if bottom.Rows < 1 {
		t.Errorf("bottom.Rows=%d, should be >= 1", bottom.Rows)
	}
}

func TestSplitLayout_MatchesPrSplitOriginal(t *testing.T) {
	// Matches the original computeSplitPaneContentOffset logic.
	// CHROME_ESTIMATE = 8: title(1) + 2 dividers + nav(~3) + status(~2).
	// contentOffset = { row: 5 + wizardH, col: 1 }.
	l := SplitLayout{
		TotalChromeRows:      8,
		TopPaneHeaderRows:    2,
		DividerRows:          1,
		BottomPaneHeaderRows: 2,
		LeftChromeCol:        1,
		MinPaneRows:          3,
	}

	// Test with representative dimensions.
	tests := []struct {
		height int
		ratio  float64
	}{
		{40, 0.6},
		{24, 0.5},
		{80, 0.7},
	}

	for _, tt := range tests {
		_, bottom := l.Compute(tt.height, 80, tt.ratio)

		// The original JS: vpHeight = max(3, h - 8); wizardH = max(3, floor(vpHeight * ratio))
		// wizardH = min(wizardH, vpHeight - 3 - 1); offset = { row: 5 + wizardH, col: 1 }
		vpHeight := tt.height - 8
		if vpHeight < 3 {
			vpHeight = 3
		}
		wizardH := int(float64(vpHeight) * tt.ratio)
		if wizardH < 3 {
			wizardH = 3
		}
		maxWiz := vpHeight - 3 - 1
		if wizardH > maxWiz {
			wizardH = maxWiz
		}
		expectedRow := 5 + wizardH
		expectedCol := 1

		if bottom.Row != expectedRow {
			t.Errorf("h=%d ratio=%.1f: bottom.Row=%d, want %d (original offset)", tt.height, tt.ratio, bottom.Row, expectedRow)
		}
		if bottom.Col != expectedCol {
			t.Errorf("h=%d ratio=%.1f: bottom.Col=%d, want %d", tt.height, tt.ratio, bottom.Col, expectedCol)
		}
	}
}

func TestPaneGeometry_OffsetMouse(t *testing.T) {
	g := PaneGeometry{Row: 10, Col: 5, Rows: 20, Cols: 70}

	// Inside
	lr, lc, ok := g.OffsetMouse(15, 10)
	if !ok || lr != 5 || lc != 5 {
		t.Errorf("inside: lr=%d lc=%d ok=%v", lr, lc, ok)
	}

	// Top-left corner
	lr, lc, ok = g.OffsetMouse(10, 5)
	if !ok || lr != 0 || lc != 0 {
		t.Errorf("corner: lr=%d lc=%d ok=%v", lr, lc, ok)
	}

	// Outside (above)
	_, _, ok = g.OffsetMouse(5, 10)
	if ok {
		t.Error("expected !ok for above pane")
	}

	// Outside (left)
	_, _, ok = g.OffsetMouse(15, 3)
	if ok {
		t.Error("expected !ok for left of pane")
	}

	// Outside (below)
	_, _, ok = g.OffsetMouse(30, 10)
	if ok {
		t.Error("expected !ok for below pane")
	}

	// Outside (right)
	_, _, ok = g.OffsetMouse(15, 75)
	if ok {
		t.Error("expected !ok for right of pane")
	}
}
