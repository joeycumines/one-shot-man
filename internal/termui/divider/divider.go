// Package divider provides a line rendering component for terminal UI. Divider
// implements the component.Component interface and uses functional options for
// configuration.
package divider

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// Compile-time check that Divider satisfies component.Component.
var _ component.Component = Divider{}

// Direction is a type alias for layout.Direction, allowing divider consumers
// to specify layout axis without importing the layout package directly.
type Direction = layout.Direction

// Divider renders a horizontal or vertical line within a bounding rectangle.
type Divider struct {
	direction Direction
	style     lipgloss.Style
	char      rune
}

// DividerOption configures a Divider.
type DividerOption func(*Divider)

// WithDividerStyle sets the lipgloss style applied to the divider line.
func WithDividerStyle(style lipgloss.Style) DividerOption {
	return func(d *Divider) { d.style = style }
}

// WithDividerChar overrides the character used to draw the divider.
func WithDividerChar(char rune) DividerOption {
	return func(d *Divider) { d.char = char }
}

// NewDivider creates a Divider along the given direction. The default
// character is '─' for horizontal and '│' for vertical.
func NewDivider(direction Direction, opts ...DividerOption) *Divider {
	d := &Divider{direction: direction}
	switch direction {
	case layout.Horizontal:
		d.char = '─'
	case layout.Vertical:
		d.char = '│'
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// Render produces a styled divider line fitting within bounds. Horizontal
// dividers repeat the char across the width; vertical dividers repeat it
// down the height, joined by newlines. Returns an empty string for zero or
// negative bounds or an unknown direction.
func (d Divider) Render(bounds coordinate.Rect) string {
	w := bounds.Size.Width
	h := bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	switch d.direction {
	case layout.Horizontal:
		line := strings.Repeat(string(d.char), w)
		return d.style.MaxWidth(w).Render(line)
	case layout.Vertical:
		line := strings.Repeat(string(d.char)+"\n", h)
		line = strings.TrimRight(line, "\n")
		return d.style.MaxHeight(h).Render(line)
	default:
		return ""
	}
}
