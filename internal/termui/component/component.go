// Package component defines the shared Component interface implemented by
// all termui rendering components. Each component lives in its own package
// (e.g. termui/label, termui/box, termui/panel) and imports this package
// to satisfy the interface.
package component

import (
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Component is the interface implemented by all terminal UI rendering
// components. Render produces styled terminal content that fits within
// the given bounding rectangle.
type Component interface {
	Render(bounds coordinate.Rect) string
}
