// Package label provides a text rendering component for terminal UI. Label
// implements the component.Component interface and uses functional options for
// configuration.
package label

import (
	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Compile-time check that Label satisfies component.Component.
var _ component.Component = Label{}

// Label renders a text string within a bounding rectangle, optionally
// constrained by style and max dimension settings.
type Label struct {
	text      string
	style     lipgloss.Style
	maxWidth  int
	maxHeight int
}

// LabelOption configures a Label.
type LabelOption func(*Label)

// WithLabelStyle sets the lipgloss style applied to the label text.
func WithLabelStyle(style lipgloss.Style) LabelOption {
	return func(l *Label) { l.style = style }
}

// WithLabelMaxWidth sets an upper bound on the rendered width. Zero means
// no constraint beyond the bounds argument.
func WithLabelMaxWidth(w int) LabelOption {
	return func(l *Label) { l.maxWidth = w }
}

// WithLabelMaxHeight sets an upper bound on the rendered height. Zero means
// no constraint beyond the bounds argument.
func WithLabelMaxHeight(h int) LabelOption {
	return func(l *Label) { l.maxHeight = h }
}

// NewLabel creates a Label displaying the given text. Options may override
// style and max dimension constraints.
func NewLabel(text string, opts ...LabelOption) *Label {
	l := &Label{text: text}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Render produces the styled label text constrained by bounds and any
// maxWidth/maxHeight overrides. Returns an empty string for zero or negative
// bounds.
func (l Label) Render(bounds coordinate.Rect) string {
	w := bounds.Size.Width
	h := bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	if l.maxWidth > 0 && w > l.maxWidth {
		w = l.maxWidth
	}
	if l.maxHeight > 0 && h > l.maxHeight {
		h = l.maxHeight
	}

	return l.style.MaxWidth(w).MaxHeight(h).Render(l.text)
}
