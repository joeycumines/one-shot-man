// Package layout provides pure functions for splitting, gridding, and stacking
// coordinate.Rect values. All functions are side-effect-free and produce
// []coordinate.Rect results from coordinate math alone.
package layout

import (
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Direction specifies the axis along which a layout operation is applied.
type Direction int

const (
	Horizontal Direction = iota // Left-to-right split or stack.
	Vertical                    // Top-to-bottom split or stack.
)

// Split divides rect into len(ratios) sub-rects along the given direction.
// Ratios are normalized so they sum to 1.0. Each sub-rect receives
// floor(ratio * available) cells in the split dimension; any remainder from
// integer truncation is given to the last sub-rect. The non-split dimension
// is shared fully by every sub-rect.
//
// If ratios is empty, Split returns a single-element slice containing rect
// unchanged.
func Split(rect coordinate.Rect, direction Direction, ratios []float64) []coordinate.Rect {
	if len(ratios) == 0 {
		return []coordinate.Rect{rect}
	}

	// Normalize ratios to sum to 1.0.
	sum := 0.0
	for _, r := range ratios {
		sum += r
	}
	normalized := make([]float64, len(ratios))
	if sum > 0 {
		for i, r := range ratios {
			normalized[i] = r / sum
		}
	}

	result := make([]coordinate.Rect, len(normalized))

	switch direction {
	case Horizontal:
		available := rect.Size.Width
		allocated := 0
		for i, ratio := range normalized {
			w := int(float64(available) * ratio)
			if i == len(normalized)-1 {
				w = available - allocated // remainder to last
			}
			if w < 0 {
				w = 0
			}
			result[i] = coordinate.Rect{
				Position: coordinate.Position{X: rect.Position.X + allocated, Y: rect.Position.Y},
				Size:     coordinate.Size{Width: w, Height: rect.Size.Height},
			}
			allocated += w
		}
	case Vertical:
		available := rect.Size.Height
		allocated := 0
		for i, ratio := range normalized {
			h := int(float64(available) * ratio)
			if i == len(normalized)-1 {
				h = available - allocated // remainder to last
			}
			if h < 0 {
				h = 0
			}
			result[i] = coordinate.Rect{
				Position: coordinate.Position{X: rect.Position.X, Y: rect.Position.Y + allocated},
				Size:     coordinate.Size{Width: rect.Size.Width, Height: h},
			}
			allocated += h
		}
	}

	return result
}

// Grid divides rect into a uniform grid of columns x rows cells, returned in
// row-major order (left-to-right within each row, top-to-bottom across rows).
// Remainder cells in either dimension are given to the last column or row.
//
// If columns or rows is <= 0, Grid returns an empty slice.
func Grid(rect coordinate.Rect, columns, rows int) []coordinate.Rect {
	if columns <= 0 || rows <= 0 {
		return nil
	}

	cellW := rect.Size.Width / columns
	cellH := rect.Size.Height / rows

	result := make([]coordinate.Rect, 0, columns*rows)

	for r := 0; r < rows; r++ {
		for c := 0; c < columns; c++ {
			x := rect.Position.X + c*cellW
			y := rect.Position.Y + r*cellH

			w := cellW
			if c == columns-1 {
				w = rect.Position.X + rect.Size.Width - x // remainder to last column
			}

			h := cellH
			if r == rows-1 {
				h = rect.Position.Y + rect.Size.Height - y // remainder to last row
			}

			if w < 0 {
				w = 0
			}
			if h < 0 {
				h = 0
			}

			result = append(result, coordinate.Rect{
				Position: coordinate.Position{X: x, Y: y},
				Size:     coordinate.Size{Width: w, Height: h},
			})
		}
	}

	return result
}

// Stack arranges items with the given sizes sequentially within rect along the
// specified direction. Each item receives its requested Size; items that would
// exceed the remaining space are clamped to what remains.
//
// If sizes is empty, Stack returns an empty slice.
func Stack(rect coordinate.Rect, direction Direction, sizes []coordinate.Size) []coordinate.Rect {
	if len(sizes) == 0 {
		return nil
	}

	result := make([]coordinate.Rect, len(sizes))

	switch direction {
	case Horizontal:
		remaining := rect.Size.Width
		offset := 0
		for i, sz := range sizes {
			w := sz.Width
			if w > remaining {
				w = remaining
			}
			if w < 0 {
				w = 0
			}
			h := sz.Height
			if h > rect.Size.Height {
				h = rect.Size.Height
			}
			if h < 0 {
				h = 0
			}
			result[i] = coordinate.Rect{
				Position: coordinate.Position{X: rect.Position.X + offset, Y: rect.Position.Y},
				Size:     coordinate.Size{Width: w, Height: h},
			}
			offset += w
			remaining -= w
		}
	case Vertical:
		remaining := rect.Size.Height
		offset := 0
		for i, sz := range sizes {
			h := sz.Height
			if h > remaining {
				h = remaining
			}
			if h < 0 {
				h = 0
			}
			w := sz.Width
			if w > rect.Size.Width {
				w = rect.Size.Width
			}
			if w < 0 {
				w = 0
			}
			result[i] = coordinate.Rect{
				Position: coordinate.Position{X: rect.Position.X, Y: rect.Position.Y + offset},
				Size:     coordinate.Size{Width: w, Height: h},
			}
			offset += h
			remaining -= h
		}
	}

	return result
}
