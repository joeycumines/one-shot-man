//go:build unix

package splitlayout

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
	"github.com/joeycumines/one-shot-man/internal/vtassert"
)

// TestVtAssert_FocusedPaneHasHighlightedBorder creates a screen simulating
// a split-pane layout where the focused pane's border has a different
// foreground color than the unfocused border, then asserts the difference.
func TestVtAssert_FocusedPaneHasHighlightedBorder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	// Simulate a 24x80 screen with a vertical divider at col 40.
	// Focused border (col 40) uses bright blue (color 12), unfocused uses grey (color 8).
	screen := vt.NewScreen(24, 80)

	// Focused border: bright blue vertical line at col 40.
	focusedFG := vt.ParseSGR([]int{38, 5, 12}, vt.Attr{})
	for r := 0; r < 24; r++ {
		screen.Cells[r][40] = vt.Cell{Ch: '│', Attr: focusedFG}
	}

	// Assert the focused border cells have the expected foreground color.
	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 40, Y: 0},
		Size:     coordinate.Size{Width: 1, Height: 24},
	}
	vtassert.AssertBorder(t, screen, bounds, layout.Vertical, vtassert.WithForeground(vtassert.Color256(12)))

	// Now create a second screen with dim border (grey, color 8).
	dimScreen := vt.NewScreen(24, 80)
	dimFG := vt.ParseSGR([]int{38, 5, 8}, vt.Attr{})
	for r := 0; r < 24; r++ {
		dimScreen.Cells[r][40] = vt.Cell{Ch: '│', Attr: dimFG}
	}

	// Assert the dim border cells have the dim foreground color.
	vtassert.AssertBorder(t, dimScreen, bounds, layout.Vertical, vtassert.WithForeground(vtassert.Color256(8)))
}

// TestVtAssert_CompositorOutputContainsPaneLabels parses compositor-style
// ANSI output through VTerm and asserts that pane labels are present.
func TestVtAssert_CompositorOutputContainsPaneLabels(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	// Simulate compositor output: two panes with labels rendered via ANSI.
	vterm := vt.NewVTerm(10, 60)

	// Pane 1 label at top-left.
	vterm.Write([]byte("\x1b[1;1H")) // cursor home
	vterm.Write([]byte("Pane 1"))

	// Pane 2 label at top of right half (col 31).
	vterm.Write([]byte("\x1b[1;31H"))
	vterm.Write([]byte("Pane 2"))

	scr := vterm.ActiveScreen()

	vtassert.AssertContainsText(t, scr, "Pane 1")
	vtassert.AssertContainsText(t, scr, "Pane 2")

	// Verify individual cells for "Pane 1" at row 0, cols 0-5.
	vtassert.AssertCell(t, scr, 0, 0, 'P')
	vtassert.AssertCell(t, scr, 0, 1, 'a')
	vtassert.AssertCell(t, scr, 0, 2, 'n')
	vtassert.AssertCell(t, scr, 0, 3, 'e')
	vtassert.AssertCell(t, scr, 0, 5, '1')
}

// TestVtAssert_VerticalDividerPresent creates a screen with │ characters
// and uses AssertBorder with layout.Vertical to verify the divider.
func TestVtAssert_VerticalDividerPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	screen := vt.NewScreen(24, 80)

	// Place a vertical divider at col 40, rows 0-23.
	for r := 0; r < 24; r++ {
		screen.Cells[r][40] = vt.Cell{Ch: '│'}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 40, Y: 0},
		Size:     coordinate.Size{Width: 1, Height: 24},
	}
	vtassert.AssertBorder(t, screen, bounds, layout.Vertical)
}

// TestVtAssert_HorizontalDividerPresent creates a screen with ─ characters
// and uses AssertBorder with layout.Horizontal to verify the divider.
func TestVtAssert_HorizontalDividerPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	screen := vt.NewScreen(24, 80)

	// Place a horizontal divider at row 12, cols 0-79.
	for c := 0; c < 80; c++ {
		screen.Cells[12][c] = vt.Cell{Ch: '─'}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 12},
		Size:     coordinate.Size{Width: 80, Height: 1},
	}
	vtassert.AssertBorder(t, screen, bounds, layout.Horizontal)
}

// TestVtAssert_RegionAllX verifies AssertRegion on a rectangular region
// of cells matching a specific pattern.
func TestVtAssert_RegionAllX(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	screen := vt.NewScreen(10, 40)

	// Fill a 3x5 region with 'X' starting at (5, 2).
	for r := 2; r < 5; r++ {
		for c := 5; c < 10; c++ {
			screen.Cells[r][c] = vt.Cell{Ch: 'X'}
		}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 5, Y: 2},
		Size:     coordinate.Size{Width: 5, Height: 3},
	}
	vtassert.AssertRegion(t, screen, bounds, func(cells [][]vt.Cell) error {
		for ri, row := range cells {
			for ci, cell := range row {
				if cell.Ch != 'X' {
					return fmt.Errorf("cell [%d][%d]: expected 'X', got %q", ri, ci, cell.Ch)
				}
			}
		}
		return nil
	})
}

// TestVtAssert_BoldBorder verifies that a border rendered with bold
// attribute is correctly asserted.
func TestVtAssert_BoldBorder(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	screen := vt.NewScreen(5, 20)

	// Bold horizontal border at row 0.
	for c := 0; c < 20; c++ {
		screen.Cells[0][c] = vt.Cell{Ch: '━', Attr: vt.Attr{Bold: true}}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 20, Height: 1},
	}
	vtassert.AssertBorder(t, screen, bounds, layout.Horizontal, vtassert.WithBold())
}

// TestVtAssert_ColoredCellWith8Color verifies AssertCell with 8-color
// foreground and background.
func TestVtAssert_ColoredCellWith8Color(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	vterm := vt.NewVTerm(5, 20)
	// Red text on blue background: ESC[31;44m
	vterm.Write([]byte("\x1b[31;44mZ\x1b[0m"))
	scr := vterm.ActiveScreen()

	vtassert.AssertCell(t, scr, 0, 0, 'Z',
		vtassert.WithForeground(vtassert.Color8(1)),
		vtassert.WithBackground(vtassert.Color8(4)),
	)
}

// TestVtAssert_RGBColorCell verifies AssertCell with truecolor (RGB).
func TestVtAssert_RGBColorCell(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	vterm := vt.NewVTerm(5, 20)
	// Truecolor foreground: ESC[38;2;255;100;0m
	vterm.Write([]byte("\x1b[38;2;255;100;0mR\x1b[0m"))
	scr := vterm.ActiveScreen()

	vtassert.AssertCell(t, scr, 0, 0, 'R',
		vtassert.WithForeground(vtassert.ColorRGB(255, 100, 0)),
	)
}
