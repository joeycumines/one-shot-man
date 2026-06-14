package termmux

import (
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// BorderStyle determines the visual style of pane borders.
type BorderStyle int

const (
	// BorderNone disables pane borders.
	BorderNone BorderStyle = iota
	// BorderSimple uses single-line box drawing characters (─ │ ┌ ┐ └ ┘ ┼).
	BorderSimple
	// BorderHeavy uses double-line box drawing characters (═ ║ ╔ ╗ ╚ ╝ ╬).
	BorderHeavy
	// BorderRound uses single-line box drawing with rounded corners (╭ ╮ ╰ ╯).
	BorderRound
	// BorderSingle is an alias for BorderSimple.
	BorderSingle
)

type boxChars struct {
	H, V          rune
	TL, TR, BL, BR rune
	Cross         rune
	Left, Right   rune
	Top, Bottom   rune
}

var simpleChars = boxChars{
	H: '─', V: '│',
	TL: '┌', TR: '┐', BL: '└', BR: '┘',
	Cross: '┼', Left: '├', Right: '┤', Top: '┬', Bottom: '┴',
}

var heavyChars = boxChars{
	H: '═', V: '║',
	TL: '╔', TR: '╗', BL: '╚', BR: '╝',
	Cross: '╬', Left: '╠', Right: '╣', Top: '╦', Bottom: '╩',
}

var roundChars = boxChars{
	H: '─', V: '│',
	TL: '╭', TR: '╮', BL: '╰', BR: '╯',
	Cross: '┼', Left: '├', Right: '┤', Top: '┬', Bottom: '┴',
}

// charsForStyle returns the box-drawing character set for the given style.
func charsForStyle(style BorderStyle) boxChars {
	switch style {
	case BorderHeavy:
		return heavyChars
	case BorderRound:
		return roundChars
	default:
		// BorderSimple, BorderSingle, and anything else use simple chars.
		return simpleChars
	}
}

// focusedBorderAttr is the Attr for focused pane borders (bold + cyan).
var focusedBorderAttr = vt.ParseSGR([]int{1, 36}, vt.Attr{})

// unfocusedBorderAttr is the Attr for unfocused pane borders (dim + bright black).
var unfocusedBorderAttr = vt.ParseSGR([]int{2, 90}, vt.Attr{})

// RenderBorders draws pane borders onto a 2D cell grid overlay. The returned
// grid has dimensions [height][width]. Each cell is either a border cell (with
// the appropriate box-drawing character and attributes) or a zero-value Cell
// (Ch=0) indicating no border. The caller merges this overlay with pane content,
// writing border cells only where the overlay is non-zero.
//
// Border rendering does not corrupt pane content — it produces a separate
// overlay that the caller composites.
func RenderBorders(panes []Pane, width, height int, focused PaneID, style BorderStyle) [][]vt.Cell {
	grid := make([][]vt.Cell, height)
	for i := range grid {
		grid[i] = make([]vt.Cell, width)
	}

	if style == BorderNone || len(panes) == 0 {
		return grid
	}

	chars := charsForStyle(style)

	for _, pane := range panes {
		if !pane.IsValid() {
			continue
		}

		attr := unfocusedBorderAttr
		if pane.ID == focused {
			attr = focusedBorderAttr
		}

		g := pane.Geometry
		bx := g.Col - 1
		by := g.Row - 1
		bx2 := g.Col + g.Cols
		by2 := g.Row + g.Rows

		// Draw top edge.
		if by >= 0 && by < height {
			for x := max(bx, 0); x <= min(bx2, width-1); x++ {
				ch := chars.H
				if x == bx {
					ch = chars.TL
				} else if x == bx2 {
					ch = chars.TR
				}
				grid[by][x] = vt.Cell{Ch: ch, Attr: attr}
			}
		}

		// Draw bottom edge.
		if by2 >= 0 && by2 < height {
			for x := max(bx, 0); x <= min(bx2, width-1); x++ {
				ch := chars.H
				if x == bx {
					ch = chars.BL
				} else if x == bx2 {
					ch = chars.BR
				}
				grid[by2][x] = vt.Cell{Ch: ch, Attr: attr}
			}
		}

		if bx >= 0 && bx < width {
			for y := max(by+1, 0); y <= min(by2-1, height-1); y++ {
				grid[y][bx] = vt.Cell{Ch: chars.V, Attr: attr}
			}
		}

		if bx2 >= 0 && bx2 < width {
			for y := max(by+1, 0); y <= min(by2-1, height-1); y++ {
				grid[y][bx2] = vt.Cell{Ch: chars.V, Attr: attr}
			}
		}
	}

	resolveIntersections(grid, panes, width, height, chars)

	return grid
}

func resolveIntersections(grid [][]vt.Cell, panes []Pane, width, height int, chars boxChars) {
	type edgeInfo struct {
		hasTop, hasBottom, hasLeft, hasRight bool
		paneCount                            int
	}

	edges := make(map[[2]int]*edgeInfo)

	for _, pane := range panes {
		if !pane.IsValid() {
			continue
		}
		g := pane.Geometry
		bx := g.Col - 1
		by := g.Row - 1
		bx2 := g.Col + g.Cols
		by2 := g.Row + g.Rows

		contributed := make(map[[2]int]bool)

		if by >= 0 && by < height {
			for x := max(bx, 0); x < min(bx2, width); x++ {
				pos := [2]int{by, x}
				e := edges[pos]
				if e == nil {
					e = &edgeInfo{}
					edges[pos] = e
				}
				e.hasLeft = true
				e.hasRight = true
				contributed[pos] = true
			}
		}

		if by2 >= 0 && by2 < height {
			for x := max(bx, 0); x < min(bx2, width); x++ {
				pos := [2]int{by2, x}
				e := edges[pos]
				if e == nil {
					e = &edgeInfo{}
					edges[pos] = e
				}
				e.hasLeft = true
				e.hasRight = true
				contributed[pos] = true
			}
		}

		if bx >= 0 && bx < width {
			for y := max(by, 0); y < min(by2, height); y++ {
				pos := [2]int{y, bx}
				e := edges[pos]
				if e == nil {
					e = &edgeInfo{}
					edges[pos] = e
				}
				e.hasTop = true
				e.hasBottom = true
				contributed[pos] = true
			}
		}

		if bx2 >= 0 && bx2 < width {
			for y := max(by, 0); y < min(by2, height); y++ {
				pos := [2]int{y, bx2}
				e := edges[pos]
				if e == nil {
					e = &edgeInfo{}
					edges[pos] = e
				}
				e.hasTop = true
				e.hasBottom = true
				contributed[pos] = true
			}
		}

		for pos := range contributed {
			edges[pos].paneCount++
		}
	}

	for pos, e := range edges {
		y, x := pos[0], pos[1]
		if y < 0 || y >= height || x < 0 || x >= width {
			continue
		}
		cell := grid[y][x]
		if cell.Ch == 0 {
			continue
		}

		// Only resolve intersections where multiple panes overlap.
		if e.paneCount < 2 {
			continue
		}

		ch := junctionChar(e.hasLeft || e.hasRight, e.hasTop || e.hasBottom, chars)
		if ch != 0 {
			grid[y][x] = vt.Cell{Ch: ch, Attr: cell.Attr}
		}
	}
}

func junctionChar(hasH, hasV bool, chars boxChars) rune {
	switch {
	case hasH && hasV:
		return chars.Cross
	case hasH:
		return chars.H
	case hasV:
		return chars.V
	default:
		return 0
	}
}

// MergeOverlay composites a border overlay onto a content grid. Border cells
// (those with Ch != 0) from the overlay replace the corresponding cells in
// the content grid. Content cells are left unchanged where the overlay has
// zero-value cells.
func MergeOverlay(content [][]vt.Cell, overlay [][]vt.Cell) {
	if len(content) == 0 || len(overlay) == 0 {
		return
	}
	rows := min(len(content), len(overlay))
	for y := 0; y < rows; y++ {
		cols := min(len(content[y]), len(overlay[y]))
		for x := 0; x < cols; x++ {
			if overlay[y][x].Ch != 0 {
				content[y][x] = overlay[y][x]
			}
		}
	}
}
