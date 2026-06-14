package termmux

import (
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

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
	minPane := max(l.MinPaneRows, 1)

	// Available height for both panes + divider.
	viewport := max(totalRows-l.TotalChromeRows, minPane)

	// Top pane height (content).
	topH := max(int(float64(viewport)*topRatio), minPane)
	maxTop := max(viewport-l.DividerRows-minPane, minPane)
	if topH > maxTop {
		topH = maxTop
	}

	// Bottom pane content height.
	bottomContentH := max(viewport-topH-l.DividerRows, 1)

	contentCols := max(totalCols-l.LeftChromeCol, 1)

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

// PaneID uniquely identifies a pane within a LayoutEngine.
type PaneID uint64

// LayoutMode determines how panes are arranged within the available space.
type LayoutMode int

const (
	LayoutTiled LayoutMode = iota
	LayoutStacked
	LayoutHorizontal
	LayoutVertical
	LayoutMainHorizontal
	LayoutMainVertical
)

// SplitDirection indicates the direction in which a new pane is inserted
// relative to the pivot pane.
type SplitDirection int

const (
	SplitRight SplitDirection = iota
	SplitDown
	SplitLeft
	SplitUp
)

// NavigationDirection indicates the direction of focus movement.
type NavigationDirection int

const (
	NavNext NavigationDirection = iota
	NavPrev
	NavUp
	NavDown
	NavLeft
	NavRight
)

// SplitGroup tracks the split relationship for a pane: which group it belongs
// to and its ratio within that group. Panes in the same group share the same
// parent space and are split along the group's direction.
type SplitGroup struct {
	Direction SplitDirection
	Ratio     float64 // this pane's share of the parent space (0.0–1.0)
}

// Pane represents a single terminal pane within a termmux session. It holds
// the virtual terminal, layout geometry, and visual metadata. A zero-value
// Pane is invalid — callers should check IsValid() before use.
type Pane struct {
	ID          PaneID
	SessionID   SessionID
	Geometry    PaneGeometry
	Title       string
	Focus       bool
	BorderStyle string
	VTerm       *vt.VTerm
	LastActive  time.Time
}

// IsValid reports whether the Pane represents a valid, allocated pane.
// A zero-value Pane (with ID 0) is not valid.
func (p Pane) IsValid() bool {
	return p.ID != 0
}

// PaneManager manages the lifecycle and layout of panes within a termmux
// session. Implementations handle pane creation, removal, focus management,
// resizing, and keyboard-driven navigation.
type PaneManager interface {
	Create(sessionID SessionID, direction SplitDirection) (PaneID, error)
	Remove(id PaneID) error
	Focus(id PaneID) error
	Resize(id PaneID, ratio float64) error
	Panes() []Pane
	ActivePaneID() PaneID
	FocusNext(direction NavigationDirection) PaneID
	HasPanes() bool
	FocusLeft()
	FocusRight()
	FocusDown()
	FocusUp()
	Close() error
}

// LayoutEngine computes PaneGeometry for N panes across four layout modes.
// It supports dynamic splitting, removal, resizing, and focus navigation.
type LayoutEngine struct {
	panes      []PaneID
	mode       LayoutMode
	splits     map[PaneID]SplitGroup
	chromeRows int
	width      int
	height     int
	nextID     PaneID
	mainRatio  float64
	zoomedPane PaneID
}

// NewLayoutEngine creates a LayoutEngine with the given mode and screen
// dimensions. Chrome rows are deducted from the available height before
// computing pane geometries.
func NewLayoutEngine(mode LayoutMode, width, height int) *LayoutEngine {
	return &LayoutEngine{
		mode:       mode,
		width:      width,
		height:     height,
		splits:     make(map[PaneID]SplitGroup),
		chromeRows: 0,
		nextID:     1,
		mainRatio:  0.6,
	}
}

// SetMode changes the layout mode. Pane geometries will reflect the new mode
// on the next Compute call.
func (e *LayoutEngine) SetMode(mode LayoutMode) {
	e.mode = mode
}

// SetSize updates the screen dimensions used for geometry computation.
func (e *LayoutEngine) SetSize(width, height int) {
	e.width = width
	e.height = height
}

// SetChromeRows sets the number of rows consumed by chrome (status bars,
// borders, etc.) that are deducted from available height before computing
// pane geometries.
func (e *LayoutEngine) SetChromeRows(rows int) {
	e.chromeRows = rows
}

func (e *LayoutEngine) SetMainRatio(ratio float64) {
	e.mainRatio = clamp(ratio, 0.1, 0.9)
}

func (e *LayoutEngine) MainRatio() float64 {
	return e.mainRatio
}

// ChromeRows returns the current chrome rows setting.
func (e *LayoutEngine) ChromeRows() int {
	return e.chromeRows
}

// Mode returns the current layout mode.
func (e *LayoutEngine) Mode() LayoutMode {
	return e.mode
}

// Size returns the current screen dimensions (width, height).
func (e *LayoutEngine) Size() (int, int) {
	return e.width, e.height
}

// PaneIDs returns the ordered list of pane IDs.
func (e *LayoutEngine) PaneIDs() []PaneID {
	out := make([]PaneID, len(e.panes))
	copy(out, e.panes)
	return out
}

// allocID returns the next available PaneID and advances the counter.
func (e *LayoutEngine) allocID() PaneID {
	id := e.nextID
	e.nextID++
	return id
}

// AllocID returns the next available PaneID and advances the counter.
// This is the package-private allocID, exposed for use by external
// types in the same package (e.g., WindowManager methods).
func (e *LayoutEngine) AllocID() PaneID {
	id := e.nextID
	e.nextID++
	return id
}

// Split inserts a new pane adjacent to the pivot pane in the given direction.
// The new pane takes half of the pivot's current space. Returns the new pane's
// ID. If pivot is 0 or not found, the pane is appended.
func (e *LayoutEngine) Split(pivot PaneID, direction SplitDirection) PaneID {
	newID := e.allocID()

	idx := -1
	for i, id := range e.panes {
		if id == pivot {
			idx = i
			break
		}
	}

	if idx < 0 {
		e.panes = append(e.panes, newID)
		e.splits[newID] = SplitGroup{Direction: direction, Ratio: 1.0}
		return newID
	}

	insertIdx := idx + 1
	if direction == SplitLeft || direction == SplitUp {
		insertIdx = idx
	}

	e.panes = append(e.panes, 0)
	copy(e.panes[insertIdx+1:], e.panes[insertIdx:])
	e.panes[insertIdx] = newID

	e.splits[newID] = SplitGroup{Direction: direction, Ratio: 0.5}

	return newID
}

// Remove removes the pane with the given ID and redistributes its space
// equally among remaining panes. If the ID is not found, Remove is a no-op.
func (e *LayoutEngine) Remove(id PaneID) {
	idx := -1
	for i, pid := range e.panes {
		if pid == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}

	e.panes = append(e.panes[:idx], e.panes[idx+1:]...)
	delete(e.splits, id)
}

// Resize adjusts the split ratio for the given pane. The ratio is clamped to
// [0.1, 0.9] to prevent panes from becoming too small. Returns false if the
// pane ID is not found.
func (e *LayoutEngine) Resize(id PaneID, ratio float64) bool {
	if _, ok := e.splits[id]; !ok {
		found := false
		for _, pid := range e.panes {
			if pid == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	ratio = clamp(ratio, 0.1, 0.9)
	e.splits[id] = SplitGroup{
		Direction: e.splits[id].Direction,
		Ratio:     ratio,
	}
	return true
}

// FocusNext returns the PaneID of the adjacent pane in the given navigation
// direction. If no adjacent pane exists (e.g., at the edge), the current
// PaneID is returned. If current is not found, 0 is returned.
func (e *LayoutEngine) FocusNext(current PaneID, direction NavigationDirection) PaneID {
	idx := -1
	for i, id := range e.panes {
		if id == current {
			idx = i
			break
		}
	}
	if idx < 0 {
		return 0
	}

	n := len(e.panes)
	if n == 0 {
		return current
	}

	switch direction {
	case NavNext:
		if idx+1 < n {
			return e.panes[idx+1]
		}
		return e.panes[0]
	case NavPrev:
		if idx-1 >= 0 {
			return e.panes[idx-1]
		}
		return e.panes[n-1]
	case NavRight, NavDown:
		if idx+1 < n {
			return e.panes[idx+1]
		}
		return current
	case NavLeft, NavUp:
		if idx-1 >= 0 {
			return e.panes[idx-1]
		}
		return current
	default:
		return current
	}
}

func (e *LayoutEngine) Swap(a, b PaneID) bool {
	idxA, idxB := -1, -1
	for i, id := range e.panes {
		if id == a {
			idxA = i
		} else if id == b {
			idxB = i
		}
	}
	if idxA < 0 || idxB < 0 {
		return false
	}
	e.panes[idxA], e.panes[idxB] = e.panes[idxB], e.panes[idxA]
	return true
}

func (e *LayoutEngine) Zoom(id PaneID) {
	e.zoomedPane = id
}

func (e *LayoutEngine) Unzoom() {
	e.zoomedPane = 0
}

func (e *LayoutEngine) ZoomedPane() PaneID {
	return e.zoomedPane
}

// Compute returns the PaneGeometry for each pane in the engine's current
// configuration. The returned slice is ordered the same as PaneIDs().
// Chrome rows are deducted from the available height before computing.
func (e *LayoutEngine) Compute(panes []Pane) []PaneGeometry {
	n := len(panes)
	if n == 0 {
		return nil
	}

	availW := e.width
	availH := max(e.height-e.chromeRows, 1)

	geoms := make([]PaneGeometry, n)

	if e.zoomedPane != 0 {
		for i, p := range panes {
			if p.ID == e.zoomedPane {
				geoms[i] = PaneGeometry{Row: 0, Col: 0, Rows: availH, Cols: availW}
			} else {
				geoms[i] = PaneGeometry{Row: 0, Col: 0, Rows: 0, Cols: 0}
			}
		}
		return geoms
	}

	switch e.mode {
	case LayoutVertical, LayoutStacked:
		e.computeVertical(geoms, availW, availH)
	case LayoutHorizontal:
		e.computeHorizontal(geoms, availW, availH)
	case LayoutTiled:
		e.computeTiled(geoms, availW, availH)
	case LayoutMainHorizontal:
		e.computeMainHorizontal(geoms, availW, availH)
	case LayoutMainVertical:
		e.computeMainVertical(geoms, availW, availH)
	default:
		e.computeVertical(geoms, availW, availH)
	}

	return geoms
}

// computeVertical stacks panes top to bottom, each getting an equal share
// of the available height. Remainder rows go to the last pane.
func (e *LayoutEngine) computeVertical(geoms []PaneGeometry, availW, availH int) {
	n := len(geoms)
	baseH := availH / n
	allocated := 0

	for i := range geoms {
		h := baseH
		if i == n-1 {
			h = availH - allocated
		}
		if h < 1 {
			h = 1
		}
		geoms[i] = PaneGeometry{
			Row:  allocated,
			Col:  0,
			Rows: h,
			Cols: availW,
		}
		allocated += h
	}
}

// computeHorizontal arranges panes side by side, each getting an equal share
// of the available width. Remainder columns go to the last pane.
func (e *LayoutEngine) computeHorizontal(geoms []PaneGeometry, availW, availH int) {
	n := len(geoms)
	baseW := availW / n
	allocated := 0

	for i := range geoms {
		w := baseW
		if i == n-1 {
			w = availW - allocated
		}
		if w < 1 {
			w = 1
		}
		geoms[i] = PaneGeometry{
			Row:  0,
			Col:  allocated,
			Rows: availH,
			Cols: w,
		}
		allocated += w
	}
}

// computeTiled arranges panes in a grid that approximates a square.
// For N panes, it chooses cols = ceil(sqrt(N)) and rows = ceil(N/cols).
// Remainder cells in the last row are distributed evenly.
func (e *LayoutEngine) computeTiled(geoms []PaneGeometry, availW, availH int) {
	n := len(geoms)
	cols := ceilSqrt(n)
	rows := (n + cols - 1) / cols

	cellW := availW / cols
	cellH := availH / rows

	for i := range geoms {
		r := i / cols
		c := i % cols

		x := c * cellW
		y := r * cellH

		w := cellW
		if c == cols-1 {
			w = availW - x
		}
		if w < 1 {
			w = 1
		}

		h := cellH
		if r == rows-1 {
			h = availH - y
		}
		if h < 1 {
			h = 1
		}

		geoms[i] = PaneGeometry{
			Row:  y,
			Col:  x,
			Rows: h,
			Cols: w,
		}
	}
}

// ceilSqrt returns ceil(sqrt(n)).
func (e *LayoutEngine) computeMainHorizontal(geoms []PaneGeometry, availW, availH int) {
	n := len(geoms)
	mainH := max(int(float64(availH)*e.mainRatio), 1)
	geoms[0] = PaneGeometry{Row: 0, Col: 0, Rows: mainH, Cols: availW}

	if n == 1 {
		return
	}

	remaining := max(availH-mainH, 1)
	baseH := remaining / (n - 1)
	allocated := 0

	for i := 1; i < n; i++ {
		h := baseH
		if i == n-1 {
			h = remaining - allocated
		}
		if h < 1 {
			h = 1
		}
		geoms[i] = PaneGeometry{Row: mainH + allocated, Col: 0, Rows: h, Cols: availW}
		allocated += h
	}
}

func (e *LayoutEngine) computeMainVertical(geoms []PaneGeometry, availW, availH int) {
	n := len(geoms)
	mainW := max(int(float64(availW)*e.mainRatio), 1)
	geoms[0] = PaneGeometry{Row: 0, Col: 0, Rows: availH, Cols: mainW}

	if n == 1 {
		return
	}

	remaining := max(availW-mainW, 1)
	baseW := remaining / (n - 1)
	allocated := 0

	for i := 1; i < n; i++ {
		w := baseW
		if i == n-1 {
			w = remaining - allocated
		}
		if w < 1 {
			w = 1
		}
		geoms[i] = PaneGeometry{Row: 0, Col: mainW + allocated, Rows: availH, Cols: w}
		allocated += w
	}
}

func ceilSqrt(n int) int {
	if n <= 0 {
		return 1
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	if x*x == n {
		return x
	}
	return x + 1
}

// clamp restricts v to [lo, hi].
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
