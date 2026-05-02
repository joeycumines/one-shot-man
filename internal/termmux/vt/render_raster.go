package vt

import (
	"image"
	icolor "image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
)

// rgba aliases image/color.RGBA to avoid collision with the vt.color type.
type rgba = icolor.RGBA

// DefaultCellW and DefaultCellH are the default pixel dimensions for a single
// terminal cell in the raster renderer. These match a typical 8x16 monospace
// font cell size.
const (
	DefaultCellW = 8
	DefaultCellH = 16
)

// standardColors maps the 16 standard ANSI colors (indices 0-15) to sRGB values.
// Indices 0-7 are the normal colors, 8-15 are the bright (bold) variants.
var standardColors = [16]rgba{
	{R: 0, G: 0, B: 0, A: 255},       // 0: Black
	{R: 170, G: 0, B: 0, A: 255},     // 1: Red
	{R: 0, G: 170, B: 0, A: 255},     // 2: Green
	{R: 170, G: 85, B: 0, A: 255},    // 3: Yellow/Brown
	{R: 0, G: 0, B: 170, A: 255},     // 4: Blue
	{R: 170, G: 0, B: 170, A: 255},   // 5: Magenta
	{R: 0, G: 170, B: 170, A: 255},   // 6: Cyan
	{R: 170, G: 170, B: 170, A: 255}, // 7: White (light gray)
	{R: 85, G: 85, B: 85, A: 255},    // 8: Bright Black (dark gray)
	{R: 255, G: 85, B: 85, A: 255},   // 9: Bright Red
	{R: 85, G: 255, B: 85, A: 255},   // 10: Bright Green
	{R: 255, G: 255, B: 85, A: 255},  // 11: Bright Yellow
	{R: 85, G: 85, B: 255, A: 255},   // 12: Bright Blue
	{R: 255, G: 85, B: 255, A: 255},  // 13: Bright Magenta
	{R: 85, G: 255, B: 255, A: 255},  // 14: Bright Cyan
	{R: 255, G: 255, B: 255, A: 255}, // 15: Bright White
}

// color256Table is the 256-color palette. Indices 0-15 match standardColors,
// 16-231 are a 6x6x6 color cube, 232-255 are a grayscale ramp.
// It is lazily initialized on first access via sync.Once.
var (
	color256Table    *[256]rgba
	color256InitOnce sync.Once
)

// getColor256 returns the RGBA color for the given 256-color palette index.
func getColor256(idx int) rgba {
	if idx < 0 || idx > 255 {
		return standardColors[0]
	}
	color256InitOnce.Do(func() {
		color256Table = &[256]rgba{}
		initColor256()
	})
	return color256Table[idx]
}

func initColor256() {
	t := color256Table
	// 0-15: standard colors.
	for i := 0; i < 16; i++ {
		t[i] = standardColors[i]
	}
	// 16-231: 6x6x6 color cube.
	for i := 0; i < 216; i++ {
		r := colorCubeScale(i / 36)
		g := colorCubeScale((i / 6) % 6)
		b := colorCubeScale(i % 6)
		t[16+i] = rgba{R: r, G: g, B: b, A: 255}
	}
	// 232-255: grayscale ramp from near-black to near-white.
	for i := 0; i < 24; i++ {
		v := uint8(8 + i*10)
		t[232+i] = rgba{R: v, G: v, B: v, A: 255}
	}
}

func colorCubeScale(v int) uint8 {
	switch v {
	case 0:
		return 0
	case 1:
		return 95
	case 2:
		return 135
	case 3:
		return 175
	case 4:
		return 215
	case 5:
		return 255
	}
	return 0
}

// resolveFG returns the foreground RGBA color for the given Attr.
// When Inverse is true, the background color attribute is resolved as the
// foreground color (colors are swapped at the attribute level).
func resolveFG(a Attr) rgba {
	if a.Inverse {
		return resolveColor(a.BG, false) // BG's color rendered as foreground
	}
	return resolveColor(a.FG, true)
}

// resolveBG returns the background RGBA color for the given Attr.
// When Inverse is true, the foreground color attribute is resolved as the
// background color (colors are swapped at the attribute level).
func resolveBG(a Attr) rgba {
	if a.Inverse {
		return resolveColor(a.FG, true) // FG's color rendered as background
	}
	return resolveColor(a.BG, false)
}

func resolveColor(c color, isFG bool) rgba {
	var result rgba
	switch c.kind {
	case kindDefault:
		if isFG {
			result = standardColors[15] // default fg = white
		} else {
			result = standardColors[0] // default bg = black
		}
	case kind8:
		idx := int(c.value)
		if idx < 0 || idx > 15 {
			idx = 0
		}
		result = standardColors[idx]
	case kind256:
		result = getColor256(int(c.value))
	case kindRGB:
		r := uint8((c.value >> 16) & 0xFF)
		g := uint8((c.value >> 8) & 0xFF)
		b := uint8(c.value & 0xFF)
		result = rgba{R: r, G: g, B: b, A: 255}
	default:
		result = standardColors[0]
	}
	return result
}

// RenderRaster converts a Screen to an RGBA image. Each terminal cell is
// rendered as a cellW x cellH pixel block with the appropriate foreground
// and background colors. Non-space characters are drawn as a filled rectangle
// in the upper 75% of the cell (to indicate content), while space characters
// are rendered as entirely background fill (to show background color).
// Wide characters (Ch == 0 placeholder) are skipped — their foreground is
// rendered by the preceding cell.
//
// This renderer is designed for automated testing and visual verification,
// not for pixel-perfect terminal reproduction. For human-readable text
// rendering, use RenderFullScreen or ContentANSI instead.
func RenderRaster(scr *Screen, cellW, cellH int) *image.RGBA {
	if cellW < 1 {
		cellW = DefaultCellW
	}
	if cellH < 1 {
		cellH = DefaultCellH
	}

	imgW := scr.Cols * cellW
	imgH := scr.Rows * cellH
	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))

	for row := 0; row < scr.Rows; row++ {
		for col := 0; col < scr.Cols; col++ {
			cell := scr.Cells[row][col]

			// Skip NUL placeholder cells (second half of wide char).
			// The wide char's first cell already covers the full width.
			if cell.Ch == 0 {
				continue
			}

			bg := resolveBG(cell.Attr)
			fg := resolveFG(cell.Attr)

			// Determine if this cell has visible content.
			// Only non-space characters get the FG block treatment;
			// space characters with colored backgrounds render as pure background fill.
			hasContent := cell.Ch != ' '

			// Determine cell width in columns.
			cellCols := 1
			// Check if next cell is a NUL placeholder (wide char).
			if col+1 < scr.Cols && scr.Cells[row][col+1].Ch == 0 {
				cellCols = 2
			}

			// Fill the cell's pixel block.
			px0 := col * cellW
			py0 := row * cellH

			for dy := 0; dy < cellH; dy++ {
				for dx := 0; dx < cellW*cellCols; dx++ {
					px := px0 + dx
					py := py0 + dy
					if px >= imgW || py >= imgH {
						continue
					}

					if hasContent {
						// Draw content: upper 75% is foreground, lower 25% is background.
						// This creates a visible block that distinguishes content from blank.
						if dy < cellH*3/4 {
							img.SetRGBA(px, py, fg)
						} else {
							img.SetRGBA(px, py, bg)
						}
					} else {
						// Blank cell: entire block is background.
						img.SetRGBA(px, py, bg)
					}
				}
			}

			// If this is a wide character, skip the next column (NUL placeholder).
			if cellCols == 2 {
				col++ // skip the placeholder on next iteration
			}
		}
	}

	return img
}

// RenderRasterDefault renders the screen using default cell dimensions (8x16).
func RenderRasterDefault(scr *Screen) *image.RGBA {
	return RenderRaster(scr, DefaultCellW, DefaultCellH)
}

// SaveRasterPNG writes the raster image to a PNG file at the given path.
// Parent directories are created if they do not exist. Returns nil on success.
func SaveRasterPNG(img *image.RGBA, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
