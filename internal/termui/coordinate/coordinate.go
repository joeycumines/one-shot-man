// Package coordinate provides value types for 2D terminal coordinate geometry.
//
// All types (Position, Size, Rect, Layer) are plain structs with value
// receivers — zero heap allocation in hot render paths.
package coordinate

import (
	"fmt"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// Position is a 2D point in cell coordinates. X is the column (horizontal),
// Y is the row (vertical). Both are 0-based.
type Position struct {
	X int
	Y int
}

// Add returns the sum of two positions.
func (p Position) Add(q Position) Position {
	return Position{X: p.X + q.X, Y: p.Y + q.Y}
}

// Sub returns the difference p - q.
func (p Position) Sub(q Position) Position {
	return Position{X: p.X - q.X, Y: p.Y - q.Y}
}

// In reports whether the position lies within the given Rect (inclusive of
// the top-left corner, exclusive of the bottom-right corner).
func (p Position) In(r Rect) bool {
	return p.X >= r.Position.X && p.X < r.Position.X+r.Size.Width &&
		p.Y >= r.Position.Y && p.Y < r.Position.Y+r.Size.Height
}

// String returns a human-readable representation like "(3,5)".
func (p Position) String() string {
	return fmt.Sprintf("(%d,%d)", p.X, p.Y)
}

// Size describes 2D dimensions in terminal cells.
type Size struct {
	Width  int
	Height int
}

// Area returns the total number of cells (Width * Height).
func (s Size) Area() int {
	return s.Width * s.Height
}

// Contains reports whether this Size is large enough to fully contain the
// other Size (both dimensions must be >= the other's).
func (s Size) Contains(other Size) bool {
	return s.Width >= other.Width && s.Height >= other.Height
}

// String returns a human-readable representation like "24x80".
func (s Size) String() string {
	return fmt.Sprintf("%dx%d", s.Width, s.Height)
}

// Rect is an axis-aligned rectangle defined by a Position (top-left corner)
// and a Size.
type Rect struct {
	Position Position
	Size     Size
}

// Contains reports whether the position lies within the Rect (inclusive of
// the top-left corner, exclusive of the bottom-right corner).
func (r Rect) Contains(p Position) bool {
	return p.In(r)
}

// Overlaps reports whether two rectangles share at least one cell.
func (r Rect) Overlaps(other Rect) bool {
	if r.Size.Width <= 0 || r.Size.Height <= 0 || other.Size.Width <= 0 || other.Size.Height <= 0 {
		return false
	}
	return r.Position.X < other.Position.X+other.Size.Width &&
		r.Position.X+r.Size.Width > other.Position.X &&
		r.Position.Y < other.Position.Y+other.Size.Height &&
		r.Position.Y+r.Size.Height > other.Position.Y
}

// Inset shrinks the Rect by the given Size on all sides (Width from left and
// right, Height from top and bottom). If the inset is larger than the Rect
// in either dimension, that dimension collapses to zero.
func (r Rect) Inset(s Size) Rect {
	newX := r.Position.X + s.Width
	newY := r.Position.Y + s.Height
	newW := r.Size.Width - 2*s.Width
	newH := r.Size.Height - 2*s.Height
	if newW < 0 {
		newW = 0
	}
	if newH < 0 {
		newH = 0
	}
	return Rect{
		Position: Position{X: newX, Y: newY},
		Size:     Size{Width: newW, Height: newH},
	}
}

// Split divides the Rect into two sub-rects along the given ratio (0.0–1.0).
// If horizontal is true, the split is left/right; otherwise top/bottom.
// A ratio of 0 gives all space to the second rect; 1 gives all to the first.
func (r Rect) Split(ratio float64, horizontal bool) (first, second Rect) {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}

	if horizontal {
		splitW := int(float64(r.Size.Width) * ratio)
		first = Rect{
			Position: r.Position,
			Size:     Size{Width: splitW, Height: r.Size.Height},
		}
		second = Rect{
			Position: Position{X: r.Position.X + splitW, Y: r.Position.Y},
			Size:     Size{Width: r.Size.Width - splitW, Height: r.Size.Height},
		}
	} else {
		splitH := int(float64(r.Size.Height) * ratio)
		first = Rect{
			Position: r.Position,
			Size:     Size{Width: r.Size.Width, Height: splitH},
		}
		second = Rect{
			Position: Position{X: r.Position.X, Y: r.Position.Y + splitH},
			Size:     Size{Width: r.Size.Width, Height: r.Size.Height - splitH},
		}
	}

	return first, second
}

// Intersect returns the largest Rect contained by both r and other. If they
// do not overlap, the result is a zero Rect.
func (r Rect) Intersect(other Rect) Rect {
	if !r.Overlaps(other) {
		return Rect{}
	}

	x := max(r.Position.X, other.Position.X)
	y := max(r.Position.Y, other.Position.Y)
	right := min(r.Position.X+r.Size.Width, other.Position.X+other.Size.Width)
	bottom := min(r.Position.Y+r.Size.Height, other.Position.Y+other.Size.Height)

	return Rect{
		Position: Position{X: x, Y: y},
		Size:     Size{Width: right - x, Height: bottom - y},
	}
}

// Union returns the smallest Rect that contains both r and other.
func (r Rect) Union(other Rect) Rect {
	if r.Size.Width <= 0 || r.Size.Height <= 0 {
		return other
	}
	if other.Size.Width <= 0 || other.Size.Height <= 0 {
		return r
	}

	x := min(r.Position.X, other.Position.X)
	y := min(r.Position.Y, other.Position.Y)
	right := max(r.Position.X+r.Size.Width, other.Position.X+other.Size.Width)
	bottom := max(r.Position.Y+r.Size.Height, other.Position.Y+other.Size.Height)

	return Rect{
		Position: Position{X: x, Y: y},
		Size:     Size{Width: right - x, Height: bottom - y},
	}
}

// String returns a human-readable representation like "(3,5) 24x80".
func (r Rect) String() string {
	return fmt.Sprintf("%s %s", r.Position, r.Size)
}

// AsPaneGeometry converts the Rect to a termmux.PaneGeometry.
// Position.X maps to Col, Position.Y maps to Row,
// Size.Width maps to Cols, Size.Height maps to Rows.
func (r Rect) AsPaneGeometry() termmux.PaneGeometry {
	return termmux.PaneGeometry{
		Row:  r.Position.Y,
		Col:  r.Position.X,
		Rows: r.Size.Height,
		Cols: r.Size.Width,
	}
}

// FromPaneGeometry converts a termmux.PaneGeometry to a Rect.
func FromPaneGeometry(pg termmux.PaneGeometry) Rect {
	return Rect{
		Position: Position{X: pg.Col, Y: pg.Row},
		Size:     Size{Width: pg.Cols, Height: pg.Rows},
	}
}

// Layer is a positioned rectangle with a Z-order, designed for interop with
// lipgloss v2's compositing system.
type Layer struct {
	Rect Rect
	Z    int
}

// AsLayer converts the coordinate Layer to a lipgloss.Layer, preserving
// position and Z-order. The resulting layer has empty content and zero
// dimensions (set content separately before rendering).
func (l Layer) AsLayer() *lipgloss.Layer {
	return lipgloss.NewLayer("").
		X(l.Rect.Position.X).
		Y(l.Rect.Position.Y).
		Z(l.Z)
}

// FromLayer converts a lipgloss.Layer to a coordinate Layer, extracting
// position, dimensions, and Z-order.
func FromLayer(ll *lipgloss.Layer) Layer {
	return Layer{
		Rect: Rect{
			Position: Position{X: ll.GetX(), Y: ll.GetY()},
			Size:     Size{Width: ll.Width(), Height: ll.Height()},
		},
		Z: ll.GetZ(),
	}
}

// String returns a human-readable representation like "(3,5) 24x80 z:2".
func (l Layer) String() string {
	return fmt.Sprintf("%s z:%d", l.Rect, l.Z)
}
