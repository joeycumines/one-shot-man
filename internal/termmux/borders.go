package termmux

import (
	"fmt"
	"strings"
)

const (
	borderTopLeft     = '┌'
	borderTopRight    = '┐'
	borderBottomLeft  = '└'
	borderBottomRight = '┘'
	borderHorizontal  = '─'
	borderVertical    = '│'
)

// RenderPaneBorders returns an ANSI/UTF-8 string that draws a border around
// every pane in the slice for a terminal of the given width and height.
//
// Each pane's border is drawn on its outer boundary (Row..Row+Rows-1 and
// Col..Col+Cols-1). Pane content outside these lines is left untouched. The
// top border of each pane displays a short "<index>:<title>" label.
//
// Pane indexes are 1-based and correspond to the position of the pane in the
// supplied slice. Coordinates outside the terminal are silently clipped.
func RenderPaneBorders(width, height int, panes []Pane) string {
	if width <= 0 || height <= 0 {
		return ""
	}

	grid := make([][]rune, height)
	for i := range grid {
		grid[i] = make([]rune, width)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	set := func(row, col int, r rune) {
		if row < 0 || row >= height || col < 0 || col >= width {
			return
		}
		grid[row][col] = r
	}

	hline := func(row, col1, col2 int) {
		for c := col1; c <= col2; c++ {
			set(row, c, borderHorizontal)
		}
	}

	vline := func(col, row1, row2 int) {
		for r := row1; r <= row2; r++ {
			set(r, col, borderVertical)
		}
	}

	for i, p := range panes {
		idx := i + 1
		g := p.Geometry
		if g.Rows <= 0 || g.Cols <= 0 {
			continue
		}

		top := g.Row
		if top >= height {
			continue
		}
		bottom := min(g.Row+g.Rows-1, height-1)
		left := g.Col
		if left >= width {
			continue
		}
		right := min(g.Col+g.Cols-1, width-1)

		// Horizontal borders.
		hline(top, left, right)
		hline(bottom, left, right)

		// Vertical borders.
		vline(left, top, bottom)
		vline(right, top, bottom)

		// Corners.
		set(top, left, borderTopLeft)
		set(top, right, borderTopRight)
		set(bottom, left, borderBottomLeft)
		set(bottom, right, borderBottomRight)

		// Title label on the top border.
		label := fmt.Sprintf("%d:%s", idx, p.Title)
		if p.Title == "" {
			label = ""
		}
		if len(label) > g.Cols-2 {
			label = label[:max(g.Cols-2, 0)]
		}
		startCol := g.Col + 1
		for j, r := range label {
			set(g.Row, startCol+j, r)
		}

		// Ensure corners are visible even if the label overlapped them.
		set(top, left, borderTopLeft)
		set(top, right, borderTopRight)
		set(bottom, left, borderBottomLeft)
		set(bottom, right, borderBottomRight)
	}

	var b strings.Builder
	for i, line := range grid {
		b.WriteString(string(line))
		if i < len(grid)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
