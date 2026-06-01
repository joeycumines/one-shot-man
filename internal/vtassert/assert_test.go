//go:build unix

package vtassert

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func TestAssertCell(t *testing.T) {
	screen := vt.NewScreen(5, 10)
	screen.Cells[0][3] = vt.Cell{Ch: 'A'}
	screen.Cells[1][5] = vt.Cell{Ch: 'B', Attr: vt.Attr{Bold: true}}

	AssertCell(t, screen, 0, 3, 'A')
	AssertCell(t, screen, 1, 5, 'B', WithBold())
}

func TestAssertCellForeground(t *testing.T) {
	vterm := vt.NewVTerm(3, 10)
	vterm.Write([]byte("\x1b[31mX\x1b[0m"))
	scr := vterm.ActiveScreen()

	AssertCell(t, scr, 0, 0, 'X', WithForeground(Color8(1)))
}

func TestAssertCellBackground(t *testing.T) {
	vterm := vt.NewVTerm(3, 10)
	vterm.Write([]byte("\x1b[44mY\x1b[0m"))
	scr := vterm.ActiveScreen()

	AssertCell(t, scr, 0, 0, 'Y', WithBackground(Color8(4)))
}

func TestAssertContainsText(t *testing.T) {
	screen := vt.NewScreen(5, 20)
	for i, ch := range "hello" {
		screen.Cells[2][5+i] = vt.Cell{Ch: ch}
	}

	AssertContainsText(t, screen, "hello")
}

func TestAssertContainsTextEmpty(t *testing.T) {
	screen := vt.NewScreen(5, 20)
	AssertContainsText(t, screen, "")
}

func TestAssertBorderHorizontal(t *testing.T) {
	screen := vt.NewScreen(5, 20)
	for c := 2; c <= 6; c++ {
		screen.Cells[1][c] = vt.Cell{Ch: '─'}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 2, Y: 1},
		Size:     coordinate.Size{Width: 5, Height: 1},
	}
	AssertBorder(t, screen, bounds, layout.Horizontal)
}

func TestAssertBorderVertical(t *testing.T) {
	screen := vt.NewScreen(10, 20)
	for r := 1; r <= 4; r++ {
		screen.Cells[r][3] = vt.Cell{Ch: '│'}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 3, Y: 1},
		Size:     coordinate.Size{Width: 1, Height: 4},
	}
	AssertBorder(t, screen, bounds, layout.Vertical)
}

func TestAssertBorderWithBold(t *testing.T) {
	screen := vt.NewScreen(5, 20)
	for c := 0; c < 5; c++ {
		screen.Cells[0][c] = vt.Cell{Ch: '━', Attr: vt.Attr{Bold: true}}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 0, Y: 0},
		Size:     coordinate.Size{Width: 5, Height: 1},
	}
	AssertBorder(t, screen, bounds, layout.Horizontal, WithBold())
}

func TestAssertRegion(t *testing.T) {
	screen := vt.NewScreen(5, 10)
	for r := 1; r <= 2; r++ {
		for c := 2; c <= 4; c++ {
			screen.Cells[r][c] = vt.Cell{Ch: 'X'}
		}
	}

	bounds := coordinate.Rect{
		Position: coordinate.Position{X: 2, Y: 1},
		Size:     coordinate.Size{Width: 3, Height: 2},
	}
	AssertRegion(t, screen, bounds, func(cells [][]vt.Cell) error {
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


