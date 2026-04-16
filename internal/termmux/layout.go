package termmux

// PaneGeometry describes a rectangular region within a terminal screen.
type PaneGeometry struct {
	Row  int // 0-based top row of content area
	Col  int // 0-based left column of content area
	Rows int // height of content area
	Cols int // width of content area
}

// OffsetMouse transforms screen-space mouse coordinates to pane-local
// coordinates by subtracting the pane origin. Returns the local
// coordinates and true if they fall within the pane, or (-1,-1) and
// false if outside.
func (g PaneGeometry) OffsetMouse(screenRow, screenCol int) (localRow, localCol int, inside bool) {
	lr := screenRow - g.Row
	lc := screenCol - g.Col
	if lr < 0 || lc < 0 || lr >= g.Rows || lc >= g.Cols {
		return -1, -1, false
	}
	return lr, lc, true
}

// SplitLayout describes a vertical split: a top pane and a bottom pane
// separated by a divider, with configurable chrome regions.
//
// The layout model:
//
//	[TopPaneHeaderRows]   ← chrome above top pane (e.g., title bar, divider)
//	[top pane content]    ← sized by ratio
//	[DividerRows]         ← separator (counted within viewport budget)
//	[BottomPaneHeaderRows]← chrome inside bottom pane before content
//	[bottom pane content] ← remaining viewport space
//	[external chrome]     ← rows outside the split (nav, status, etc.)
//
// TotalChromeRows is the sum of ALL non-pane-content, non-divider rows
// deducted from terminal height before splitting. It equals
// TopPaneHeaderRows + BottomPaneHeaderRows + external chrome rows.
// The DividerRows is NOT included in TotalChromeRows because the
// divider is counted as part of the viewport that gets split.
type SplitLayout struct {
	// TotalChromeRows is all non-content rows deducted from terminal
	// height before computing the viewport available for pane content
	// plus divider. Equals TopPaneHeaderRows + BottomPaneHeaderRows +
	// any external chrome (nav, status bars, etc.).
	TotalChromeRows int

	// TopPaneHeaderRows is the number of rows above the top pane content
	// (e.g., title bar + divider: 2 in pr-split).
	TopPaneHeaderRows int

	// DividerRows is the number of rows between the two panes. This is
	// counted within the viewport budget (not in TotalChromeRows).
	DividerRows int

	// BottomPaneHeaderRows is the number of chrome rows at the start of
	// the bottom pane before its content begins (e.g., border top +
	// title line: 2).
	BottomPaneHeaderRows int

	// LeftChromeCol is the number of chrome columns before bottom pane
	// content starts (e.g., border left: 1).
	LeftChromeCol int

	// MinPaneRows is the minimum content height for either pane.
	MinPaneRows int
}

// Compute calculates the geometry of both panes given the total screen
// dimensions and the top pane's share of the available viewport height
// (0.0–1.0). The ratio controls the split between the two panes.
func (l SplitLayout) Compute(totalRows, totalCols int, topRatio float64) (top, bottom PaneGeometry) {
	minPane := l.MinPaneRows
	if minPane < 1 {
		minPane = 1
	}

	// Available height for both panes + divider.
	viewport := totalRows - l.TotalChromeRows
	if viewport < minPane {
		viewport = minPane
	}

	// Top pane height (content).
	topH := int(float64(viewport) * topRatio)
	if topH < minPane {
		topH = minPane
	}
	maxTop := viewport - l.DividerRows - minPane
	if maxTop < minPane {
		maxTop = minPane
	}
	if topH > maxTop {
		topH = maxTop
	}

	// Bottom pane content height.
	bottomContentH := viewport - topH - l.DividerRows
	if bottomContentH < 1 {
		bottomContentH = 1
	}

	contentCols := totalCols - l.LeftChromeCol
	if contentCols < 1 {
		contentCols = 1
	}

	top = PaneGeometry{
		Row:  l.TopPaneHeaderRows,
		Col:  0,
		Rows: topH,
		Cols: totalCols,
	}

	bottomContentRow := l.TopPaneHeaderRows + topH + l.DividerRows + l.BottomPaneHeaderRows
	bottom = PaneGeometry{
		Row:  bottomContentRow,
		Col:  l.LeftChromeCol,
		Rows: bottomContentH,
		Cols: contentCols,
	}

	return top, bottom
}
