// Package toast provides a short notification component for terminal UI.
// Toast renders a styled message positioned at the bottom of the available
// bounds. It implements the component.Component interface and uses functional
// options for configuration.
package toast

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
)

// Compile-time check that Toast satisfies component.Component.
var _ component.Component = Toast{}

// Toast renders a short notification message positioned at the bottom of the
// available bounds.
type Toast struct {
	Message string
	Style   lipgloss.Style
	Width   int
}

// ToastOption configures a Toast.
type ToastOption func(*Toast)

// WithToastMessage sets the notification text.
func WithToastMessage(msg string) ToastOption {
	return func(t *Toast) { t.Message = msg }
}

// WithToastStyle sets the lipgloss style applied to the toast text.
func WithToastStyle(style lipgloss.Style) ToastOption {
	return func(t *Toast) { t.Style = style }
}

// WithToastWidth sets an explicit width for the toast. Zero means use the full
// bounds width.
func WithToastWidth(w int) ToastOption {
	return func(t *Toast) { t.Width = w }
}

// NewToast creates a Toast with optional configuration.
func NewToast(opts ...ToastOption) *Toast {
	t := &Toast{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Render produces a styled message positioned at the bottom of bounds. If the
// toast width is configured and smaller than the bounds, the message is
// constrained to that width. Returns an empty string for zero or negative
// bounds.
func (t Toast) Render(bounds coordinate.Rect) string {
	w, h := bounds.Size.Width, bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	tw := w
	if t.Width > 0 && t.Width < w {
		tw = t.Width
	}

	// Render as a styled Label, positioned at bottom of bounds.
	l := label.NewLabel(t.Message, label.WithLabelStyle(t.Style), label.WithLabelMaxWidth(tw))
	result := l.Render(coordinate.Rect{Size: coordinate.Size{Width: tw, Height: 1}})

	// Pad to bottom of bounds.
	if h > 1 {
		result = strings.Repeat("\n", h-1) + result
	}
	return result
}
