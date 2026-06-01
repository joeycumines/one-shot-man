//go:build unix

package vtassert

import (
	"reflect"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

// Color represents a terminal color for visual regression assertions.
// It stores SGR parameters that reconstruct the color via vt.ParseSGR.
type Color struct {
	sgr []int
}

// Color8 returns a standard 8/16-color (SGR codes 30-37 for basic, 90-97 for bright).
func Color8(n int) Color {
	if n < 0 {
		n = 0
	}
	if n > 15 {
		n = 15
	}
	if n < 8 {
		return Color{sgr: []int{30 + n}}
	}
	return Color{sgr: []int{90 + n - 8}}
}

// Color256 returns a 256-color palette entry.
func Color256(n int) Color {
	return Color{sgr: []int{38, 5, n}}
}

// ColorRGB returns a truecolor (24-bit RGB) color.
func ColorRGB(r, g, b int) Color {
	return Color{sgr: []int{38, 2, r, g, b}}
}

func (c Color) asFG() vt.Attr {
	return vt.ParseSGR(c.sgr, vt.Attr{})
}

func (c Color) asBG() vt.Attr {
	bg := make([]int, len(c.sgr))
	copy(bg, c.sgr)
	for i, p := range bg {
		switch {
		case p == 38:
			bg[i] = 48
		case p >= 30 && p <= 37:
			bg[i] = p + 10
		case p >= 90 && p <= 97:
			bg[i] = p + 10
		}
	}
	return vt.ParseSGR(bg, vt.Attr{})
}

// CellOpt configures cell assertion strictness.
type CellOpt interface {
	applyCell(*cellConfig)
}

type cellConfig struct {
	fg   *Color
	bg   *Color
	bold bool
}

type foregroundOpt struct {
	fg Color
}

func (o *foregroundOpt) applyCell(cfg *cellConfig) {
	cfg.fg = &o.fg
}

// WithForeground asserts the cell has the given foreground color.
func WithForeground(fg Color) *foregroundOpt {
	return &foregroundOpt{fg: fg}
}

type backgroundOpt struct {
	bg Color
}

func (o *backgroundOpt) applyCell(cfg *cellConfig) {
	cfg.bg = &o.bg
}

// WithBackground asserts the cell has the given background color.
func WithBackground(bg Color) *backgroundOpt {
	return &backgroundOpt{bg: bg}
}

type boldOpt struct{}

func (o *boldOpt) applyCell(cfg *cellConfig) {
	cfg.bold = true
}

// WithBold asserts the cell is bold.
func WithBold() *boldOpt {
	return &boldOpt{}
}

var (
	_ CellOpt = (*foregroundOpt)(nil)
	_ CellOpt = (*backgroundOpt)(nil)
	_ CellOpt = (*boldOpt)(nil)
)

// RegionMatcher is a predicate for region content.
type RegionMatcher func(cells [][]vt.Cell) error

// AssertCell verifies a specific cell's rune and optionally its attributes.
func AssertCell(t *testing.T, screen *vt.Screen, row, col int, expectedRune rune, opts ...CellOpt) {
	t.Helper()
	if row < 0 || row >= screen.Rows || col < 0 || col >= screen.Cols {
		t.Fatalf("vtassert: AssertCell position (%d,%d) out of bounds (screen %dx%d)", row, col, screen.Rows, screen.Cols)
	}
	cell := screen.Cells[row][col]
	if cell.Ch != expectedRune {
		t.Fatalf("vtassert: AssertCell (%d,%d) expected rune %q, got %q", row, col, expectedRune, cell.Ch)
	}
	cfg := cellConfig{}
	for _, opt := range opts {
		opt.applyCell(&cfg)
	}
	if cfg.fg != nil {
		refAttr := cfg.fg.asFG()
		if !reflect.DeepEqual(cell.Attr.FG, refAttr.FG) {
			t.Fatalf("vtassert: AssertCell (%d,%d) foreground color mismatch", row, col)
		}
	}
	if cfg.bg != nil {
		refAttr := cfg.bg.asBG()
		if !reflect.DeepEqual(cell.Attr.BG, refAttr.BG) {
			t.Fatalf("vtassert: AssertCell (%d,%d) background color mismatch", row, col)
		}
	}
	if cfg.bold && !cell.Attr.Bold {
		t.Fatalf("vtassert: AssertCell (%d,%d) expected bold attribute", row, col)
	}
}

// AssertRegion verifies a rectangular region matches a predicate.
func AssertRegion(t *testing.T, screen *vt.Screen, bounds coordinate.Rect, matcher RegionMatcher) {
	t.Helper()
	x0 := bounds.Position.X
	y0 := bounds.Position.Y
	x1 := x0 + bounds.Size.Width
	y1 := y0 + bounds.Size.Height
	if x0 < 0 || y0 < 0 || x1 > screen.Cols || y1 > screen.Rows {
		t.Fatalf("vtassert: AssertRegion bounds %s exceed screen %dx%d", bounds, screen.Cols, screen.Rows)
	}
	region := make([][]vt.Cell, y1-y0)
	for r := y0; r < y1; r++ {
		region[r-y0] = screen.Cells[r][x0:x1]
	}
	if err := matcher(region); err != nil {
		t.Fatalf("vtassert: AssertRegion %s: %v", bounds, err)
	}
}

// AssertContainsText searches all cells for a string.
func AssertContainsText(t *testing.T, screen *vt.Screen, text string) {
	t.Helper()
	if text == "" {
		return
	}
	runes := []rune(text)
	for r := 0; r < screen.Rows; r++ {
		for c := 0; c <= screen.Cols-len(runes); c++ {
			match := true
			for i, ru := range runes {
				if screen.Cells[r][c+i].Ch != ru {
					match = false
					break
				}
			}
			if match {
				return
			}
		}
	}
	t.Fatalf("vtassert: AssertContainsText %q not found on screen", text)
}

// AssertBorder verifies a horizontal or vertical border line with consistent
// SGR attributes.
func AssertBorder(t *testing.T, screen *vt.Screen, bounds coordinate.Rect, direction layout.Direction, opts ...CellOpt) {
	t.Helper()
	cfg := cellConfig{}
	for _, opt := range opts {
		opt.applyCell(&cfg)
	}
	switch direction {
	case layout.Horizontal:
		y := bounds.Position.Y
		if y < 0 || y >= screen.Rows {
			t.Fatalf("vtassert: AssertBorder row %d out of bounds", y)
		}
		for x := bounds.Position.X; x < bounds.Position.X+bounds.Size.Width; x++ {
			if x < 0 || x >= screen.Cols {
				t.Fatalf("vtassert: AssertBorder col %d out of bounds", x)
			}
			cell := screen.Cells[y][x]
			if cell.Ch == ' ' || cell.Ch == 0 {
				t.Fatalf("vtassert: AssertBorder (%d,%d) expected border rune, got %q", y, x, cell.Ch)
			}
			assertCellAttrs(t, cell, cfg, y, x)
		}
	case layout.Vertical:
		x := bounds.Position.X
		if x < 0 || x >= screen.Cols {
			t.Fatalf("vtassert: AssertBorder col %d out of bounds", x)
		}
		for y := bounds.Position.Y; y < bounds.Position.Y+bounds.Size.Height; y++ {
			if y < 0 || y >= screen.Rows {
				t.Fatalf("vtassert: AssertBorder row %d out of bounds", y)
			}
			cell := screen.Cells[y][x]
			if cell.Ch == ' ' || cell.Ch == 0 {
				t.Fatalf("vtassert: AssertBorder (%d,%d) expected border rune, got %q", y, x, cell.Ch)
			}
			assertCellAttrs(t, cell, cfg, y, x)
		}
	default:
		t.Fatalf("vtassert: AssertBorder unknown direction %d", direction)
	}
}

func assertCellAttrs(t *testing.T, cell vt.Cell, cfg cellConfig, row, col int) {
	t.Helper()
	if cfg.fg != nil {
		refAttr := cfg.fg.asFG()
		if !reflect.DeepEqual(cell.Attr.FG, refAttr.FG) {
			t.Fatalf("vtassert: (%d,%d) foreground color mismatch", row, col)
		}
	}
	if cfg.bg != nil {
		refAttr := cfg.bg.asBG()
		if !reflect.DeepEqual(cell.Attr.BG, refAttr.BG) {
			t.Fatalf("vtassert: (%d,%d) background color mismatch", row, col)
		}
	}
	if cfg.bold && !cell.Attr.Bold {
		t.Fatalf("vtassert: (%d,%d) expected bold attribute", row, col)
	}
}
