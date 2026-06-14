package termmux

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func TestRenderBorders_NoneStyle(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 5, Cols: 10}},
	}
	grid := RenderBorders(panes, 20, 10, 1, BorderNone)
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x].Ch != 0 {
				t.Errorf("BorderNone: found non-zero cell at (%d,%d): %q", y, x, grid[y][x].Ch)
			}
		}
	}
}

func TestRenderBorders_EmptyPanes(t *testing.T) {
	grid := RenderBorders(nil, 20, 10, 0, BorderSimple)
	for y := range grid {
		for x := range grid[y] {
			if grid[y][x].Ch != 0 {
				t.Errorf("empty panes: found non-zero cell at (%d,%d): %q", y, x, grid[y][x].Ch)
			}
		}
	}
}

func TestRenderBorders_SinglePaneSimple(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
	}
	grid := RenderBorders(panes, 10, 8, 1, BorderSimple)

	// Top-left corner at (0,0)
	if grid[0][0].Ch != '┌' {
		t.Errorf("top-left: got %q, want ┌", grid[0][0].Ch)
	}
	// Top-right corner at (0,6)
	if grid[0][6].Ch != '┐' {
		t.Errorf("top-right: got %q, want ┐", grid[0][6].Ch)
	}
	// Bottom-left corner at (4,0)
	if grid[4][0].Ch != '└' {
		t.Errorf("bottom-left: got %q, want └", grid[4][0].Ch)
	}
	// Bottom-right corner at (4,6)
	if grid[4][6].Ch != '┘' {
		t.Errorf("bottom-right: got %q, want ┘", grid[4][6].Ch)
	}
	// Top edge horizontal
	if grid[0][3].Ch != '─' {
		t.Errorf("top edge: got %q, want ─", grid[0][3].Ch)
	}
	// Left edge vertical
	if grid[2][0].Ch != '│' {
		t.Errorf("left edge: got %q, want │", grid[2][0].Ch)
	}
	// Interior should be empty
	if grid[2][3].Ch != 0 {
		t.Errorf("interior: got %q, want 0", grid[2][3].Ch)
	}
}

func TestRenderBorders_FocusedVsUnfocused(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
		{ID: 2, Geometry: PaneGeometry{Row: 1, Col: 8, Rows: 3, Cols: 5}},
	}

	grid := RenderBorders(panes, 20, 8, 1, BorderSimple)

	// Pane 1 is focused — its border should have bold attr
	focusedCell := grid[0][0]
	if !focusedCell.Attr.Bold {
		t.Errorf("focused border: expected Bold=true, got Bold=%v", focusedCell.Attr.Bold)
	}

	// Pane 2 is unfocused — its border should have dim attr
	unfocusedCell := grid[0][7]
	if !unfocusedCell.Attr.Dim {
		t.Errorf("unfocused border: expected Dim=true, got Dim=%v", unfocusedCell.Attr.Dim)
	}
}

func TestRenderBorders_HeavyStyle(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
	}
	grid := RenderBorders(panes, 10, 8, 1, BorderHeavy)

	if grid[0][0].Ch != '╔' {
		t.Errorf("heavy top-left: got %q, want ╔", grid[0][0].Ch)
	}
	if grid[0][6].Ch != '╗' {
		t.Errorf("heavy top-right: got %q, want ╗", grid[0][6].Ch)
	}
	if grid[4][0].Ch != '╚' {
		t.Errorf("heavy bottom-left: got %q, want ╚", grid[4][0].Ch)
	}
	if grid[4][6].Ch != '╝' {
		t.Errorf("heavy bottom-right: got %q, want ╝", grid[4][6].Ch)
	}
	if grid[0][3].Ch != '═' {
		t.Errorf("heavy horizontal: got %q, want ═", grid[0][3].Ch)
	}
	if grid[2][0].Ch != '║' {
		t.Errorf("heavy vertical: got %q, want ║", grid[2][0].Ch)
	}
}

func TestRenderBorders_RoundStyle(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
	}
	grid := RenderBorders(panes, 10, 8, 1, BorderRound)

	if grid[0][0].Ch != '╭' {
		t.Errorf("round top-left: got %q, want ╭", grid[0][0].Ch)
	}
	if grid[0][6].Ch != '╮' {
		t.Errorf("round top-right: got %q, want ╮", grid[0][6].Ch)
	}
	if grid[4][0].Ch != '╰' {
		t.Errorf("round bottom-left: got %q, want ╰", grid[4][0].Ch)
	}
	if grid[4][6].Ch != '╯' {
		t.Errorf("round bottom-right: got %q, want ╯", grid[4][6].Ch)
	}
}

func TestRenderBorders_SingleStyleMatchesSimple(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
	}
	gridSimple := RenderBorders(panes, 10, 8, 1, BorderSimple)
	gridSingle := RenderBorders(panes, 10, 8, 1, BorderSingle)

	for y := range gridSimple {
		for x := range gridSimple[y] {
			if gridSimple[y][x].Ch != gridSingle[y][x].Ch {
				t.Errorf("BorderSingle != BorderSimple at (%d,%d): %q vs %q", y, x, gridSimple[y][x].Ch, gridSingle[y][x].Ch)
			}
		}
	}
}

func TestRenderBorders_AdjacentPanesIntersection(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
		{ID: 2, Geometry: PaneGeometry{Row: 1, Col: 7, Rows: 3, Cols: 5}},
	}
	grid := RenderBorders(panes, 20, 8, 1, BorderSimple)

	// The border between the two panes should be a cross (┼) where
	// the right border of pane 1 meets the left border of pane 2.
	// Pane 1 right border is at col 6, pane 2 left border is at col 6.
	// They share the same column, so intersections should be resolved.
	// At row 0 (top edge of both), col 6 should be a cross.
	if grid[0][6].Ch != '┼' {
		t.Errorf("intersection top: got %q, want ┼", grid[0][6].Ch)
	}
}

func TestMergeOverlay(t *testing.T) {
	content := [][]vt.Cell{
		{{Ch: 'A'}, {Ch: 'B'}, {Ch: 'C'}},
		{{Ch: 'D'}, {Ch: 'E'}, {Ch: 'F'}},
	}
	overlay := [][]vt.Cell{
		{{Ch: 0}, {Ch: '│'}, {Ch: 0}},
		{{Ch: '─'}, {Ch: 0}, {Ch: 0}},
	}

	MergeOverlay(content, overlay)

	if content[0][0].Ch != 'A' {
		t.Errorf("content[0][0]: got %q, want A", content[0][0].Ch)
	}
	if content[0][1].Ch != '│' {
		t.Errorf("content[0][1]: got %q, want │", content[0][1].Ch)
	}
	if content[1][0].Ch != '─' {
		t.Errorf("content[1][0]: got %q, want ─", content[1][0].Ch)
	}
	if content[1][1].Ch != 'E' {
		t.Errorf("content[1][1]: got %q, want E", content[1][1].Ch)
	}
}

func TestMergeOverlay_Empty(t *testing.T) {
	MergeOverlay(nil, nil)
	MergeOverlay([][]vt.Cell{}, [][]vt.Cell{})
}

func TestRenderBorders_DoesNotCorruptContent(t *testing.T) {
	panes := []Pane{
		{ID: 1, Geometry: PaneGeometry{Row: 1, Col: 1, Rows: 3, Cols: 5}},
	}
	grid := RenderBorders(panes, 10, 8, 1, BorderSimple)

	// Interior cells (inside the border frame) should be zero-value
	for y := 1; y <= 3; y++ {
		for x := 1; x <= 5; x++ {
			if grid[y][x].Ch != 0 {
				t.Errorf("interior cell (%d,%d): got %q, want 0", y, x, grid[y][x].Ch)
			}
		}
	}
}
