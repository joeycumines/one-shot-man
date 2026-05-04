package vt

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderRaster_BasicText(t *testing.T) {
	scr := NewScreen(2, 10) // 2 rows, 10 cols
	// Write "Hello" in the first row.
	for _, ch := range "Hello" {
		scr.PutChar(ch)
	}

	img := RenderRaster(scr, 8, 16)

	// Verify image dimensions: 10 cols * 8px = 80, 2 rows * 16px = 32.
	if img.Bounds().Dx() != 80 {
		t.Errorf("image width = %d, want 80", img.Bounds().Dx())
	}
	if img.Bounds().Dy() != 32 {
		t.Errorf("image height = %d, want 32", img.Bounds().Dy())
	}

	// Verify that the first 5 cells have content (non-background pixels).
	// Content cells should have white foreground in the upper 75% of the cell.
	for col := 0; col < 5; col++ {
		px := col*8 + 3 // center-x of the cell
		py := 8         // middle-y of the cell (within the upper 75%)
		r, g, b, _ := img.At(px, py).RGBA()
		// Default FG is white (255, 255, 255).
		if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
			t.Errorf("cell[%d] at (%d,%d): got rgba(%d,%d,%d), want white fg",
				col, px, py, r>>8, g>>8, b>>8)
		}
	}

	// Verify that cells 5-9 are blank (all black background).
	for col := 5; col < 10; col++ {
		px := col*8 + 3
		py := 8
		r, g, b, _ := img.At(px, py).RGBA()
		// Default BG is black (0, 0, 0).
		if r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
			t.Errorf("blank cell[%d] at (%d,%d): got rgba(%d,%d,%d), want black bg",
				col, px, py, r>>8, g>>8, b>>8)
		}
	}
}

func TestRenderRaster_DefaultDimensions(t *testing.T) {
	scr := NewScreen(24, 80) // 24 rows, 80 cols
	img := RenderRasterDefault(scr)

	// Default: 80 cols * 8px = 640, 24 rows * 16px = 384.
	if img.Bounds().Dx() != 640 {
		t.Errorf("image width = %d, want 640", img.Bounds().Dx())
	}
	if img.Bounds().Dy() != 384 {
		t.Errorf("image height = %d, want 384", img.Bounds().Dy())
	}
}

func TestRenderRaster_ANSIColors(t *testing.T) {
	scr := NewScreen(1, 10) // 1 row, 10 cols

	// Write 'R' with red foreground (SGR 31).
	scr.CurAttr = Attr{FG: color{kind: kind8, value: 1}} // red
	scr.PutChar('R')
	// Write 'G' with green foreground (SGR 32).
	scr.CurAttr = Attr{FG: color{kind: kind8, value: 2}} // green
	scr.PutChar('G')
	// Write 'B' with blue foreground (SGR 34).
	scr.CurAttr = Attr{FG: color{kind: kind8, value: 4}} // blue
	scr.PutChar('B')

	img := RenderRaster(scr, 8, 16)

	// Check red cell (column 0).
	px, py := 4, 8
	r, g, b, _ := img.At(px, py).RGBA()
	if r>>8 < 100 { // red channel should be significant
		t.Errorf("red cell: got rgba(%d,%d,%d), expected high red", r>>8, g>>8, b>>8)
	}

	// Check green cell (column 1).
	px = 8 + 4
	r, g, b, _ = img.At(px, py).RGBA()
	if g>>8 < 100 { // green channel should be significant
		t.Errorf("green cell: got rgba(%d,%d,%d), expected high green", r>>8, g>>8, b>>8)
	}

	// Check blue cell (column 2).
	px = 16 + 4
	r, g, b, _ = img.At(px, py).RGBA()
	if b>>8 < 100 { // blue channel should be significant
		t.Errorf("blue cell: got rgba(%d,%d,%d), expected high blue", r>>8, g>>8, b>>8)
	}
}

func TestRenderRaster_TrueColor(t *testing.T) {
	scr := NewScreen(1, 3) // 1 row, 3 cols

	// Write with truecolor (38;2;255;128;0) — orange.
	scr.CurAttr = Attr{FG: color{kind: kindRGB, value: 0xFF8000}}
	scr.PutChar('X')

	img := RenderRaster(scr, 8, 16)
	px, py := 4, 8
	r, g, b, _ := img.At(px, py).RGBA()
	if r>>8 != 255 || g>>8 != 128 || b>>8 != 0 {
		t.Errorf("truecolor cell: got rgba(%d,%d,%d), want rgba(255,128,0)",
			r>>8, g>>8, b>>8)
	}
}

func TestRenderRaster_Inverse(t *testing.T) {
	scr := NewScreen(1, 3) // 1 row, 3 cols

	// Cell 0: inverse with default colors.
	scr.CurAttr = Attr{Inverse: true}
	scr.PutChar('I')

	// Cell 1: inverse with explicit FG=red, BG=blue.
	scr.CurAttr = Attr{Inverse: true, FG: color{kind: kind8, value: 1}, BG: color{kind: kind8, value: 4}} // red fg, blue bg
	scr.PutChar('X')

	// Cell 2: inverse with explicit FG=green, default BG.
	scr.CurAttr = Attr{Inverse: true, FG: color{kind: kind8, value: 2}} // green fg
	scr.PutChar('Y')

	img := RenderRaster(scr, 8, 16)

	// Cell 0: default inverse. FG color = black (BG default), BG color = white (FG default).
	px, py := 4, 8
	r, g, b, _ := img.At(px, py).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("cell 0 inverse fg: got rgba(%d,%d,%d), want black (swapped from white)",
			r>>8, g>>8, b>>8)
	}
	// Cell 1: inverse with explicit colors. FG = blue (swapped from BG), BG = red (swapped from FG).
	px = 8 + 4
	r, g, b, _ = img.At(px, py).RGBA()
	// Blue (0, 0, 170): blue channel should be dominant.
	if b>>8 < 100 || r>>8 > 30 {
		t.Errorf("cell 1 inverse fg: got rgba(%d,%d,%d), want blue (swapped from BG)",
			r>>8, g>>8, b>>8)
	}
	// Cell 2: inverse with explicit FG=green, default BG. FG = black (swapped from BG default), BG = green.
	px = 16 + 4
	r, g, b, _ = img.At(px, py).RGBA()
	// BG default is black.
	if r>>8 > 30 || g>>8 > 30 || b>>8 > 30 {
		t.Errorf("cell 2 inverse fg: got rgba(%d,%d,%d), want black (swapped from default BG)",
			r>>8, g>>8, b>>8)
	}
}

func TestRenderRaster_WideChar(t *testing.T) {
	scr := NewScreen(1, 5) // 1 row, 5 cols

	// Write a CJK character (width 2).
	scr.PutChar('中')

	img := RenderRaster(scr, 8, 16)

	// The wide character should occupy 2 cells (16px wide).
	// Check the middle of the first cell.
	px, py := 4, 8
	r, g, b, _ := img.At(px, py).RGBA()
	// Content should have white foreground.
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("wide char first cell: got rgba(%d,%d,%d), want white fg",
			r>>8, g>>8, b>>8)
	}

	// Check the middle of the second cell area (columns 1-2).
	px = 8 + 4
	r, g, b, _ = img.At(px, py).RGBA()
	// The wide char extends into this cell area, so it should also be foreground.
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("wide char second cell: got rgba(%d,%d,%d), want white fg (extended)",
			r>>8, g>>8, b>>8)
	}
}

func TestRenderRaster_256Color(t *testing.T) {
	scr := NewScreen(1, 2) // 1 row, 2 cols

	// Write with 256-color index 196 (bright red in the color cube).
	scr.CurAttr = Attr{FG: color{kind: kind256, value: 196}}
	scr.PutChar('X')

	img := RenderRaster(scr, 8, 16)
	px, py := 4, 8
	r, _, _, _ := img.At(px, py).RGBA()
	// Color 196 is in the 6x6x6 cube: index 196-16=180, r=180/36=5→255.
	// 196 = 16 + 5*36 + 0*6 + 0 → (255, 0, 0)
	if r>>8 != 255 {
		t.Errorf("256-color cell: got r=%d, want 255", r>>8)
	}
}

func TestRenderRaster_BackgroundColor(t *testing.T) {
	scr := NewScreen(1, 2) // 1 row, 2 cols

	// Write a space with red background (SGR 41).
	// Space characters with colored backgrounds render as pure background fill.
	scr.CurAttr = Attr{BG: color{kind: kind8, value: 1}} // red bg
	scr.PutChar(' ')

	img := RenderRaster(scr, 8, 16)

	// The entire cell should be red background.
	px, py := 4, 8
	r, g, b, _ := img.At(px, py).RGBA()
	// Red bg (170, 0, 0): red channel should be ~170, green/blue near zero.
	if r>>8 < 100 {
		t.Errorf("bg color cell: got rgba(%d,%d,%d), expected red bg", r>>8, g>>8, b>>8)
	}
	if g>>8 > 30 || b>>8 > 30 {
		t.Errorf("bg color cell: got rgba(%d,%d,%d), expected minimal green/blue for red bg", r>>8, g>>8, b>>8)
	}
}

func TestSaveRasterPNG(t *testing.T) {
	scr := NewScreen(2, 10) // 2 rows, 10 cols
	for _, ch := range "Hello" {
		scr.PutChar(ch)
	}

	img := RenderRaster(scr, 8, 16)

	dir := t.TempDir()
	path := filepath.Join(dir, "test.png")

	if err := SaveRasterPNG(img, path); err != nil {
		t.Fatalf("SaveRasterPNG: %v", err)
	}

	// Verify file exists and has content.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Size() == 0 {
		t.Error("PNG file is empty")
	}
}

func TestResolveFG_BG_DefaultColors(t *testing.T) {
	// Default Attr: FG = white, BG = black.
	a := Attr{}
	fg := resolveFG(a)
	bg := resolveBG(a)

	if fg.R != 255 || fg.G != 255 || fg.B != 255 {
		t.Errorf("default fg: got rgba(%d,%d,%d), want white", fg.R, fg.G, fg.B)
	}
	if bg.R != 0 || bg.G != 0 || bg.B != 0 {
		t.Errorf("default bg: got rgba(%d,%d,%d), want black", bg.R, bg.G, bg.B)
	}
}

func TestResolveFG_BG_Inverse(t *testing.T) {
	// Default inverse: FG and BG swap defaults.
	a := Attr{Inverse: true}
	fg := resolveFG(a)
	bg := resolveBG(a)

	// Default FG is white, default BG is black. With inverse, FG gets BG's default (black).
	if fg.R != 0 || fg.G != 0 || fg.B != 0 {
		t.Errorf("inverse default fg: got rgba(%d,%d,%d), want black", fg.R, fg.G, fg.B)
	}
	if bg.R != 255 || bg.G != 255 || bg.B != 255 {
		t.Errorf("inverse default bg: got rgba(%d,%d,%d), want white", bg.R, bg.G, bg.B)
	}

	// Explicit inverse: FG=red(1) + BG=blue(4) + Inverse=true.
	// After swap: FG resolves BG (blue), BG resolves FG (red).
	a2 := Attr{Inverse: true, FG: color{kind: kind8, value: 1}, BG: color{kind: kind8, value: 4}}
	fg2 := resolveFG(a2)
	bg2 := resolveBG(a2)
	// Blue = (0, 0, 170)
	if fg2.R != 0 || fg2.G != 0 || fg2.B != 170 {
		t.Errorf("inverse explicit fg: got rgba(%d,%d,%d), want blue(0,0,170)", fg2.R, fg2.G, fg2.B)
	}
	// Red = (170, 0, 0)
	if bg2.R != 170 || bg2.G != 0 || bg2.B != 0 {
		t.Errorf("inverse explicit bg: got rgba(%d,%d,%d), want red(170,0,0)", bg2.R, bg2.G, bg2.B)
	}
}

func TestGetColor256_AllIndices(t *testing.T) {
	// Verify all 256 indices return without panic and produce valid RGBA.
	for i := 0; i <= 255; i++ {
		c := getColor256(i)
		_ = c // just verify no panic
	}

	// Out of range returns black.
	c := getColor256(-1)
	if c.R != 0 || c.G != 0 || c.B != 0 {
		t.Errorf("getColor256(-1): got rgba(%d,%d,%d), want black", c.R, c.G, c.B)
	}
	c = getColor256(256)
	if c.R != 0 || c.G != 0 || c.B != 0 {
		t.Errorf("getColor256(256): got rgba(%d,%d,%d), want black", c.R, c.G, c.B)
	}
}

func TestColorCubeScale(t *testing.T) {
	tests := []struct {
		v    int
		want uint8
	}{
		{0, 0},
		{1, 95},
		{2, 135},
		{3, 175},
		{4, 215},
		{5, 255},
	}
	for _, tt := range tests {
		got := colorCubeScale(tt.v)
		if got != tt.want {
			t.Errorf("colorCubeScale(%d) = %d, want %d", tt.v, got, tt.want)
		}
	}
}

func TestRenderRaster_CustomCellSize(t *testing.T) {
	scr := NewScreen(1, 5) // 1 row, 5 cols
	scr.PutChar('A')

	img := RenderRaster(scr, 4, 8)
	if img.Bounds().Dx() != 20 { // 5 * 4
		t.Errorf("width = %d, want 20", img.Bounds().Dx())
	}
	if img.Bounds().Dy() != 8 { // 1 * 8
		t.Errorf("height = %d, want 8", img.Bounds().Dy())
	}

	// Content pixel should be at (2, 4) — center of first cell.
	r, g, b, _ := img.At(2, 4).RGBA()
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("custom cell size content: got rgba(%d,%d,%d), want white",
			r>>8, g>>8, b>>8)
	}
}

// TestRaster_NULByteSentinel verifies that a literal NUL byte (Ch == 0) in a
// screen cell does NOT trigger wide-character rendering for its predecessor.
// Without the SecondHalf field, writing a NUL to a cell would cause the
// preceding normal character to be rendered as 2 cells wide (corruption).
func TestRaster_NULByteSentinel(t *testing.T) {
	scr := NewScreen(1, 3) // 1 row, 3 cols
	// Write 'A' in col 0. Cursor advances to col 1.
	scr.PutChar('A')
	// Manually place a NUL byte (Ch=0) at col 1 — NOT a SecondHalf placeholder.
	scr.Cells[0][1] = Cell{Ch: 0}
	// Manually advance cursor to col 2 so PutChar writes 'B' at col 2.
	scr.CurCol = 2
	scr.PutChar('B')

	img := RenderRaster(scr, 8, 16)

	// Cell 0 ('A'): should be 1 cell wide (8px), white foreground.
	px := 4 // center of cell 0
	py := 8
	r, g, b, _ := img.At(px, py).RGBA()
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("cell 0 ('A') at (%d,%d): got rgba(%d,%d,%d), want white fg", px, py, r>>8, g>>8, b>>8)
	}

	// Cell 1 (NUL): NUL bytes render as blank background (no content).
	// Ch==0 is treated as no-content, so the pixel should be black background,
	// NOT white (which would mean 'A' was extended as a wide char).
	px = 12 // center of col 1 area
	r, g, b, _ = img.At(px, py).RGBA()
	if r>>8 > 100 || g>>8 > 100 || b>>8 > 100 {
		t.Errorf("cell 1 (NUL) at (%d,%d): got rgba(%d,%d,%d), want dark (background, not A-extended)",
			px, py, r>>8, g>>8, b>>8)
	}

	// Cell 2 ('B'): should be white foreground at correct position.
	px = 20 // center of col 2 area
	r, g, b, _ = img.At(px, py).RGBA()
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("cell 2 ('B') at (%d,%d): got rgba(%d,%d,%d), want white fg", px, py, r>>8, g>>8, b>>8)
	}
}

// TestRaster_CellH1_ForegroundVisible verifies that content cells render
// visible foreground pixels even with cellH=1. The original code used
// cellH*3/4 as the foreground threshold, which evaluated to 0 when cellH=1,
// making content cells render entirely as background (invisible foreground).
func TestRaster_CellH1_ForegroundVisible(t *testing.T) {
	scr := NewScreen(1, 1) // 1 row, 1 col
	scr.PutChar('X')

	// cellH=1: the only pixel row should be foreground (not background).
	img := RenderRaster(scr, 8, 1)

	if img.Bounds().Dy() != 1 {
		t.Fatalf("height = %d, want 1", img.Bounds().Dy())
	}

	// The single pixel at (4, 0) should be white foreground, not black background.
	px, py := 4, 0
	r, g, b, _ := img.At(px, py).RGBA()
	if r>>8 != 255 || g>>8 != 255 || b>>8 != 255 {
		t.Errorf("cellH=1 content: got rgba(%d,%d,%d), want white fg (not black bg)",
			r>>8, g>>8, b>>8)
	}
}

// TestRaster_CellH1_BlankBackground verifies that blank cells with cellH=1
// correctly render background (not foreground). This is the counterpart to
// TestRaster_CellH1_ForegroundVisible — content cells get foreground,
// blank cells get background.
func TestRaster_CellH1_BlankBackground(t *testing.T) {
	scr := NewScreen(1, 1) // 1 row, 1 col
	// Don't put any character — the cell defaults to space (blank).

	img := RenderRaster(scr, 8, 1)

	if img.Bounds().Dy() != 1 {
		t.Fatalf("height = %d, want 1", img.Bounds().Dy())
	}

	// The single pixel should be black background (the cell is blank).
	px, py := 4, 0
	r, g, b, _ := img.At(px, py).RGBA()
	if r>>8 != 0 || g>>8 != 0 || b>>8 != 0 {
		t.Errorf("cellH=1 blank: got rgba(%d,%d,%d), want black bg",
			r>>8, g>>8, b>>8)
	}
}
