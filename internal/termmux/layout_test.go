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
		vpHeight := max(tt.height-8, 3)
		wizardH := max(int(float64(vpHeight)*tt.ratio), 3)
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

func makePanes(n int) []Pane {
	panes := make([]Pane, n)
	for i := range panes {
		panes[i] = Pane{ID: PaneID(i + 1)}
	}
	return panes
}

func TestLayoutEngine_Vertical2Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	geoms := e.Compute(makePanes(2))
	if len(geoms) != 2 {
		t.Fatalf("expected 2 geometries, got %d", len(geoms))
	}
	if geoms[0].Row != 0 || geoms[0].Rows != 12 || geoms[0].Cols != 80 {
		t.Errorf("pane[0] = %+v, want Row=0 Rows=12 Cols=80", geoms[0])
	}
	if geoms[1].Row != 12 || geoms[1].Rows != 12 || geoms[1].Cols != 80 {
		t.Errorf("pane[1] = %+v, want Row=12 Rows=12 Cols=80", geoms[1])
	}
}

func TestLayoutEngine_Vertical3Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	geoms := e.Compute(makePanes(3))
	if len(geoms) != 3 {
		t.Fatalf("expected 3 geometries, got %d", len(geoms))
	}
	totalRows := 0
	for _, g := range geoms {
		totalRows += g.Rows
		if g.Cols != 80 {
			t.Errorf("Cols=%d, want 80", g.Cols)
		}
	}
	if totalRows != 24 {
		t.Errorf("total Rows=%d, want 24", totalRows)
	}
}

func TestLayoutEngine_Horizontal5Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutHorizontal, 100, 24)
	geoms := e.Compute(makePanes(5))
	if len(geoms) != 5 {
		t.Fatalf("expected 5 geometries, got %d", len(geoms))
	}
	totalCols := 0
	for _, g := range geoms {
		totalCols += g.Cols
		if g.Rows != 24 {
			t.Errorf("Rows=%d, want 24", g.Rows)
		}
	}
	if totalCols != 100 {
		t.Errorf("total Cols=%d, want 100", totalCols)
	}
}

func TestLayoutEngine_Tiled8Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 120, 40)
	geoms := e.Compute(makePanes(8))
	if len(geoms) != 8 {
		t.Fatalf("expected 8 geometries, got %d", len(geoms))
	}
	// ceilSqrt(8) = 3 cols, rows = ceil(8/3) = 3
	for i, g := range geoms {
		if g.Rows < 1 || g.Cols < 1 {
			t.Errorf("pane[%d] has zero dimension: %+v", i, g)
		}
	}
	seen := make(map[[2]int]bool)
	for _, g := range geoms {
		for r := g.Row; r < g.Row+g.Rows; r++ {
			for c := g.Col; c < g.Col+g.Cols; c++ {
				key := [2]int{r, c}
				if seen[key] {
					t.Errorf("cell (%d,%d) covered by multiple panes", r, c)
				}
				seen[key] = true
			}
		}
	}
}

func TestLayoutEngine_StackedSameAsVertical(t *testing.T) {
	ev := NewLayoutEngine(LayoutVertical, 80, 24)
	es := NewLayoutEngine(LayoutStacked, 80, 24)
	panes := makePanes(3)
	gv := ev.Compute(panes)
	gs := es.Compute(panes)
	for i := range gv {
		if gv[i] != gs[i] {
			t.Errorf("pane[%d]: vertical=%+v stacked=%+v", i, gv[i], gs[i])
		}
	}
}

func TestLayoutEngine_ChromeRows(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	e.SetChromeRows(4)
	geoms := e.Compute(makePanes(2))
	totalRows := 0
	for _, g := range geoms {
		totalRows += g.Rows
	}
	if totalRows != 20 {
		t.Errorf("total Rows=%d, want 20 (24-4 chrome)", totalRows)
	}
}

func TestLayoutEngine_SplitAndRemove(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	p1 := PaneID(1)
	e.panes = []PaneID{p1}

	p2 := e.Split(p1, SplitDown)
	if len(e.panes) != 2 {
		t.Fatalf("expected 2 panes after split, got %d", len(e.panes))
	}
	if e.panes[0] != p1 || e.panes[1] != p2 {
		t.Errorf("pane order: %v, want [%d, %d]", e.panes, p1, p2)
	}

	e.Remove(p1)
	if len(e.panes) != 1 {
		t.Fatalf("expected 1 pane after remove, got %d", len(e.panes))
	}
	if e.panes[0] != p2 {
		t.Errorf("remaining pane=%d, want %d", e.panes[0], p2)
	}
}

func TestLayoutEngine_SplitLeft(t *testing.T) {
	e := NewLayoutEngine(LayoutHorizontal, 80, 24)
	p1 := PaneID(1)
	e.panes = []PaneID{p1}

	p2 := e.Split(p1, SplitLeft)
	if len(e.panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(e.panes))
	}
	if e.panes[0] != p2 {
		t.Errorf("left split: pane[0]=%d, want %d (new pane before pivot)", e.panes[0], p2)
	}
	if e.panes[1] != p1 {
		t.Errorf("left split: pane[1]=%d, want %d (pivot)", e.panes[1], p1)
	}
}

func TestLayoutEngine_Resize(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	p1 := PaneID(1)
	e.panes = []PaneID{p1}
	e.splits[p1] = SplitGroup{Direction: SplitDown, Ratio: 0.5}

	if !e.Resize(p1, 0.7) {
		t.Error("Resize returned false, want true")
	}
	if e.splits[p1].Ratio != 0.7 {
		t.Errorf("ratio=%f, want 0.7", e.splits[p1].Ratio)
	}

	e.Resize(p1, 0.05)
	if e.splits[p1].Ratio != 0.1 {
		t.Errorf("clamped ratio=%f, want 0.1", e.splits[p1].Ratio)
	}

	e.Resize(p1, 0.99)
	if e.splits[p1].Ratio != 0.9 {
		t.Errorf("clamped ratio=%f, want 0.9", e.splits[p1].Ratio)
	}
}

func TestLayoutEngine_FocusNext(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	e.panes = []PaneID{1, 2, 3}

	if next := e.FocusNext(1, NavNext); next != 2 {
		t.Errorf("NavNext from 1 = %d, want 2", next)
	}
	if next := e.FocusNext(2, NavNext); next != 3 {
		t.Errorf("NavNext from 2 = %d, want 3", next)
	}
	if next := e.FocusNext(3, NavNext); next != 1 {
		t.Errorf("NavNext from 3 (wrap) = %d, want 1", next)
	}
	if next := e.FocusNext(1, NavPrev); next != 3 {
		t.Errorf("NavPrev from 1 (wrap) = %d, want 3", next)
	}
	if next := e.FocusNext(2, NavLeft); next != 1 {
		t.Errorf("NavLeft from 2 = %d, want 1", next)
	}
	if next := e.FocusNext(1, NavLeft); next != 1 {
		t.Errorf("NavLeft from 1 (edge) = %d, want 1", next)
	}
	if next := e.FocusNext(3, NavRight); next != 3 {
		t.Errorf("NavRight from 3 (edge) = %d, want 3", next)
	}
	if next := e.FocusNext(99, NavNext); next != 0 {
		t.Errorf("NavNext from missing = %d, want 0", next)
	}
}

func TestLayoutEngine_SetModeAndSize(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	geoms := e.Compute(makePanes(2))
	if geoms[0].Rows != 12 {
		t.Errorf("vertical: Rows=%d, want 12", geoms[0].Rows)
	}

	e.SetMode(LayoutHorizontal)
	geoms = e.Compute(makePanes(2))
	if geoms[0].Cols != 40 {
		t.Errorf("horizontal: Cols=%d, want 40", geoms[0].Cols)
	}

	e.SetSize(160, 48)
	geoms = e.Compute(makePanes(2))
	if geoms[0].Cols != 80 {
		t.Errorf("horizontal 160w: Cols=%d, want 80", geoms[0].Cols)
	}
}

func TestLayoutEngine_Tiled2Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)
	geoms := e.Compute(makePanes(2))
	if len(geoms) != 2 {
		t.Fatalf("expected 2, got %d", len(geoms))
	}
	// ceilSqrt(2) = 2 cols, 1 row → 2 cells side by side
	if geoms[0].Col != 0 || geoms[0].Cols != 40 {
		t.Errorf("pane[0] = %+v, want Col=0 Cols=40", geoms[0])
	}
	if geoms[1].Col != 40 || geoms[1].Cols != 40 {
		t.Errorf("pane[1] = %+v, want Col=40 Cols=40", geoms[1])
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
