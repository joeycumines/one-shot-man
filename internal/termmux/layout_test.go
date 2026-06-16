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

func TestLayoutEngine_MainHorizontal_2Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutMainHorizontal, 100, 50)
	p1, p2 := e.Split(0, SplitDown), e.Split(0, SplitDown)
	geoms := e.Compute([]Pane{{ID: p1}, {ID: p2}})

	if geoms[0].Rows != 30 {
		t.Errorf("main pane rows = %d, want 30 (60%% of 50)", geoms[0].Rows)
	}
	if geoms[1].Rows != 20 {
		t.Errorf("secondary pane rows = %d, want 20", geoms[1].Rows)
	}
}

func TestLayoutEngine_MainHorizontal_3Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutMainHorizontal, 100, 60)
	p1 := e.Split(0, SplitDown)
	p2 := e.Split(p1, SplitDown)
	p3 := e.Split(p2, SplitDown)
	geoms := e.Compute([]Pane{{ID: p1}, {ID: p2}, {ID: p3}})

	if geoms[0].Rows != 36 {
		t.Errorf("main pane rows = %d, want 36", geoms[0].Rows)
	}
	if geoms[1].Row != 36 {
		t.Errorf("pane 2 row = %d, want 36", geoms[1].Row)
	}
}

func TestLayoutEngine_MainVertical_2Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutMainVertical, 100, 50)
	p1, p2 := e.Split(0, SplitRight), e.Split(0, SplitRight)
	geoms := e.Compute([]Pane{{ID: p1}, {ID: p2}})

	if geoms[0].Cols != 60 {
		t.Errorf("main pane cols = %d, want 60 (60%% of 100)", geoms[0].Cols)
	}
	if geoms[1].Cols != 40 {
		t.Errorf("secondary pane cols = %d, want 40", geoms[1].Cols)
	}
}

func TestLayoutEngine_MainVertical_5Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutMainVertical, 100, 50)
	p1 := e.Split(0, SplitRight)
	p2 := e.Split(p1, SplitRight)
	p3 := e.Split(p2, SplitRight)
	p4 := e.Split(p3, SplitRight)
	p5 := e.Split(p4, SplitRight)
	geoms := e.Compute([]Pane{{ID: p1}, {ID: p2}, {ID: p3}, {ID: p4}, {ID: p5}})

	if geoms[0].Cols != 60 {
		t.Errorf("main pane cols = %d, want 60", geoms[0].Cols)
	}
	if geoms[1].Col != 60 {
		t.Errorf("pane 2 col = %d, want 60", geoms[1].Col)
	}
	totalSecondary := 0
	for i := 1; i < 5; i++ {
		totalSecondary += geoms[i].Cols
	}
	if totalSecondary != 40 {
		t.Errorf("total secondary cols = %d, want 40", totalSecondary)
	}
}

func TestLayoutEngine_MainHorizontal_CustomRatio(t *testing.T) {
	e := NewLayoutEngine(LayoutMainHorizontal, 100, 50)
	e.SetMainRatio(0.8)
	p1, p2 := e.Split(0, SplitDown), e.Split(0, SplitDown)
	geoms := e.Compute([]Pane{{ID: p1}, {ID: p2}})

	if geoms[0].Rows != 40 {
		t.Errorf("main pane rows = %d, want 40 (80%% of 50)", geoms[0].Rows)
	}
}

func TestLayoutEngine_MainVertical_CustomRatio(t *testing.T) {
	e := NewLayoutEngine(LayoutMainVertical, 100, 50)
	e.SetMainRatio(0.4)
	p1, p2 := e.Split(0, SplitRight), e.Split(0, SplitRight)
	geoms := e.Compute([]Pane{{ID: p1}, {ID: p2}})

	if geoms[0].Cols != 40 {
		t.Errorf("main pane cols = %d, want 40 (40%% of 100)", geoms[0].Cols)
	}
}

func TestLayoutEngine_MainHorizontal_SinglePane(t *testing.T) {
	e := NewLayoutEngine(LayoutMainHorizontal, 100, 50)
	p1 := e.Split(0, SplitDown)
	geoms := e.Compute([]Pane{{ID: p1}})

	if geoms[0].Rows != 30 {
		t.Errorf("single pane rows = %d, want 30", geoms[0].Rows)
	}
	if geoms[0].Cols != 100 {
		t.Errorf("single pane cols = %d, want 100", geoms[0].Cols)
	}
}

func TestLayoutEngine_MainVertical_8Panes(t *testing.T) {
	e := NewLayoutEngine(LayoutMainVertical, 120, 50)
	p1 := e.Split(0, SplitRight)
	ids := []PaneID{p1}
	for range 7 {
		ids = append(ids, e.Split(ids[len(ids)-1], SplitRight))
	}
	panes := make([]Pane, len(ids))
	for i, id := range ids {
		panes[i] = Pane{ID: id}
	}
	geoms := e.Compute(panes)

	if geoms[0].Cols != 72 {
		t.Errorf("main pane cols = %d, want 72 (60%% of 120)", geoms[0].Cols)
	}
	if geoms[1].Col != 72 {
		t.Errorf("pane 2 col = %d, want 72", geoms[1].Col)
	}
}

func TestLayoutEngine_SetMainRatio_Clamped(t *testing.T) {
	e := NewLayoutEngine(LayoutMainHorizontal, 100, 50)
	e.SetMainRatio(0.05)
	if e.MainRatio() != 0.1 {
		t.Errorf("ratio below min = %f, want 0.1", e.MainRatio())
	}
	e.SetMainRatio(0.99)
	if e.MainRatio() != 0.9 {
		t.Errorf("ratio above max = %f, want 0.9", e.MainRatio())
	}
}

func TestLayoutEngine_Mode(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)
	if e.Mode() != LayoutTiled {
		t.Errorf("Mode() = %v, want LayoutTiled", e.Mode())
	}
	e.SetMode(LayoutMainVertical)
	if e.Mode() != LayoutMainVertical {
		t.Errorf("Mode() = %v, want LayoutMainVertical after SetMode", e.Mode())
	}
}

func TestLayoutEngine_Size(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)
	w, h := e.Size()
	if w != 80 || h != 24 {
		t.Errorf("Size() = %d,%d, want 80,24", w, h)
	}
	e.SetSize(120, 40)
	w, h = e.Size()
	if w != 120 || h != 40 {
		t.Errorf("Size() after SetSize = %d,%d, want 120,40", w, h)
	}
}

func TestLayoutEngine_AllocID(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)

	ids := make(map[PaneID]bool)
	for i := range 10 {
		id := e.AllocID()
		if ids[id] {
			t.Errorf("AllocID() returned duplicate ID %d on iteration %d", id, i)
		}
		ids[id] = true
	}

	if e.AllocID() != PaneID(len(ids)+1) {
		t.Errorf("AllocID() sequence broken after %d allocations", len(ids))
	}
}

func TestLayoutEngine_Resize_Clamped(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)
	p1 := e.Split(0, SplitRight)

	if ok := e.Resize(p1, 0.05); !ok {
		t.Error("Resize should succeed for existing pane")
	}
	sg := e.splits[p1]
	if sg.Ratio < 0.1 {
		t.Errorf("Resize ratio = %f, should be clamped to >= 0.1", sg.Ratio)
	}

	if ok := e.Resize(p1, 0.95); !ok {
		t.Error("Resize should succeed for existing pane")
	}
	sg = e.splits[p1]
	if sg.Ratio > 0.9 {
		t.Errorf("Resize ratio = %f, should be clamped to <= 0.9", sg.Ratio)
	}
}

func TestLayoutEngine_Resize_NotFound(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)
	if ok := e.Resize(999, 0.5); ok {
		t.Error("Resize should return false for non-existent pane")
	}
}

func TestLayoutModeFromString(t *testing.T) {
	tests := []struct {
		input string
		want  LayoutMode
		ok    bool
	}{
		{"tiled", LayoutTiled, true},
		{"stacked", LayoutStacked, true},
		{"horizontal", LayoutHorizontal, true},
		{"vertical", LayoutVertical, true},
		{"main-horizontal", LayoutMainHorizontal, true},
		{"main-vertical", LayoutMainVertical, true},
		{"unknown", LayoutTiled, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := LayoutModeFromString(tt.input)
			if ok != tt.ok {
				t.Errorf("LayoutModeFromString(%q) ok = %v, want %v", tt.input, ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Errorf("LayoutModeFromString(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestLayoutMode_String(t *testing.T) {
	tests := []struct {
		mode LayoutMode
		want string
	}{
		{LayoutTiled, "tiled"},
		{LayoutStacked, "stacked"},
		{LayoutHorizontal, "horizontal"},
		{LayoutVertical, "vertical"},
		{LayoutMainHorizontal, "main-horizontal"},
		{LayoutMainVertical, "main-vertical"},
		{LayoutMode(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWindowManager_SetLayoutMode(t *testing.T) {
	wm := NewWindowManager(LayoutVertical, 80, 24)
	id := wm.NewWindow("w1")

	if err := wm.SetLayoutMode(id, LayoutMainHorizontal); err != nil {
		t.Fatalf("SetLayoutMode: %v", err)
	}

	mode, err := wm.LayoutMode(id)
	if err != nil {
		t.Fatalf("LayoutMode: %v", err)
	}
	if mode != LayoutMainHorizontal {
		t.Errorf("LayoutMode = %v, want LayoutMainHorizontal", mode)
	}

	w := wm.Window(id)
	if w.Layout != LayoutMainHorizontal {
		t.Errorf("Window.Layout = %v, want LayoutMainHorizontal", w.Layout)
	}

	if err := wm.SetLayoutMode(WindowID(999), LayoutTiled); err == nil {
		t.Error("expected error for unknown window")
	}
}

func TestLayoutEngine_PaneIDs(t *testing.T) {
	e := NewLayoutEngine(LayoutTiled, 80, 24)
	p1 := e.Split(0, SplitRight)
	p2 := e.Split(p1, SplitRight)

	ids := e.PaneIDs()
	if len(ids) != 2 {
		t.Errorf("PaneIDs() len = %d, want 2", len(ids))
	}

	found := map[PaneID]bool{}
	for _, id := range ids {
		found[id] = true
	}
	if !found[p1] || !found[p2] {
		t.Errorf("PaneIDs() = %v, expected to contain %d and %d", ids, p1, p2)
	}
}

func TestLayoutEngine_PaneAt(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	p1 := e.Split(0, SplitDown)
	p2 := e.Split(p1, SplitDown)
	panes := []Pane{{ID: p1}, {ID: p2}}

	if id, ok := e.PaneAt(panes, 5, 10); !ok || id != p1 {
		t.Errorf("PaneAt(5,10) = %d,%v, want %d,true", id, ok, p1)
	}
	if id, ok := e.PaneAt(panes, 15, 10); !ok || id != p2 {
		t.Errorf("PaneAt(15,10) = %d,%v, want %d,true", id, ok, p2)
	}
	if _, ok := e.PaneAt(panes, 50, 10); ok {
		t.Error("PaneAt(50,10) should be outside")
	}
}

func TestLayoutEngine_DividerAt_Vertical(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	p1 := e.Split(0, SplitDown)
	p2 := e.Split(p1, SplitDown)
	panes := []Pane{{ID: p1}, {ID: p2}}

	geoms := e.Compute(panes)
	dividerRow := geoms[0].Row + geoms[0].Rows

	if id, ok := e.DividerAt(panes, dividerRow, 10); !ok || id != p2 {
		t.Errorf("DividerAt(divider) = %d,%v, want %d,true", id, ok, p2)
	}
	if _, ok := e.DividerAt(panes, dividerRow+2, 10); ok {
		t.Error("DividerAt(non-divider row) should fail")
	}
}

func TestLayoutEngine_DividerAt_Horizontal(t *testing.T) {
	e := NewLayoutEngine(LayoutHorizontal, 80, 24)
	p1 := e.Split(0, SplitRight)
	p2 := e.Split(p1, SplitRight)
	panes := []Pane{{ID: p1}, {ID: p2}}

	geoms := e.Compute(panes)
	dividerCol := geoms[0].Col + geoms[0].Cols

	if id, ok := e.DividerAt(panes, 5, dividerCol); !ok || id != p2 {
		t.Errorf("DividerAt(divider) = %d,%v, want %d,true", id, ok, p2)
	}
	if _, ok := e.DividerAt(panes, 5, dividerCol+2); ok {
		t.Error("DividerAt(non-divider col) should fail")
	}
}

func TestLayoutEngine_DividerAt_Horizontal_OutsideContent(t *testing.T) {
	e := NewLayoutEngine(LayoutHorizontal, 80, 24)
	e.SetChromeRows(4)
	p1 := e.Split(0, SplitRight)
	p2 := e.Split(p1, SplitRight)
	panes := []Pane{{ID: p1}, {ID: p2}}

	geoms := e.Compute(panes)
	contentH := max(e.height-e.chromeRows, 1)
	dividerCol := geoms[0].Col + geoms[0].Cols

	if id, ok := e.DividerAt(panes, 5, dividerCol); !ok || id != p2 {
		t.Errorf("DividerAt(content area) = %d,%v, want %d,true", id, ok, p2)
	}
	if _, ok := e.DividerAt(panes, contentH, dividerCol); ok {
		t.Error("DividerAt should not report a divider outside the content area")
	}
}

func TestLayoutEngine_ResizeGeometry(t *testing.T) {
	e := NewLayoutEngine(LayoutVertical, 80, 24)
	p1 := e.Split(0, SplitDown)
	p2 := e.Split(p1, SplitDown)
	panes := []Pane{{ID: p1}, {ID: p2}}

	before := e.Compute(panes)
	if before[0].Rows != before[1].Rows {
		t.Fatalf("expected equal initial heights, got %v", before)
	}

	e.Resize(p2, 0.7)
	after := e.Compute(panes)
	if after[1].Rows <= before[1].Rows {
		t.Errorf("p2 height did not increase: before=%d after=%d", before[1].Rows, after[1].Rows)
	}
	if after[0].Rows >= before[0].Rows {
		t.Errorf("p1 height did not decrease: before=%d after=%d", before[0].Rows, after[0].Rows)
	}

	e.Resize(p2, 0.05)
	clamped := e.Compute(panes)
	if clamped[1].Rows < 2 {
		t.Errorf("p2 height %d below minimum", clamped[1].Rows)
	}
}
