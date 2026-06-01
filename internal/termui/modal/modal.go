// Package modal provides a centered dialog box component for terminal UI.
// Modal renders a centered dialog over the available bounds, with optional
// border and explicit width/height constraints. It implements the
// component.Component interface and uses functional options for configuration.
package modal

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/box"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

// Compile-time check that Modal satisfies component.Component.
var _ component.Component = Modal{}

// Modal renders a centered dialog box over the available bounds, with optional
// border and explicit width/height constraints.
type Modal struct {
	Content component.Component
	Width   int
	Height  int
	Style   lipgloss.Style
	Border  lipgloss.Border
}

// ModalOption configures a Modal.
type ModalOption func(*Modal)

// WithModalContent sets the inner Component rendered inside the modal.
func WithModalContent(c component.Component) ModalOption {
	return func(m *Modal) { m.Content = c }
}

// WithModalWidth sets an explicit width for the modal. Zero means use the full
// bounds width.
func WithModalWidth(w int) ModalOption {
	return func(m *Modal) { m.Width = w }
}

// WithModalHeight sets an explicit height for the modal. Zero means use the
// full bounds height.
func WithModalHeight(h int) ModalOption {
	return func(m *Modal) { m.Height = h }
}

// WithModalStyle sets the lipgloss style applied to the modal.
func WithModalStyle(style lipgloss.Style) ModalOption {
	return func(m *Modal) { m.Style = style }
}

// WithModalBorder sets the border style. Default is lipgloss.RoundedBorder().
func WithModalBorder(border lipgloss.Border) ModalOption {
	return func(m *Modal) { m.Border = border }
}

// NewModal creates a Modal with optional configuration. The default border is
// lipgloss.RoundedBorder().
func NewModal(opts ...ModalOption) *Modal {
	m := &Modal{Border: lipgloss.RoundedBorder()}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Render produces a centered modal box within bounds. If width or height are
// configured and smaller than the bounds, the modal is centered with vertical
// padding. Returns an empty string for zero or negative bounds.
func (m Modal) Render(bounds coordinate.Rect) string {
	w, h := bounds.Size.Width, bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	// Use configured width/height if set, otherwise use bounds.
	mw, mh := w, h
	if m.Width > 0 && m.Width < w {
		mw = m.Width
	}
	if m.Height > 0 && m.Height < h {
		mh = m.Height
	}

	// Center the modal within bounds.
	offY := (h - mh) / 2

	b := box.NewBox(
		box.WithBoxContent(m.Content),
		box.WithBoxStyle(m.Style),
		box.WithBoxBorder(m.Border),
	)
	modalRect := coordinate.Rect{
		Position: coordinate.Position{X: (w - mw) / 2, Y: offY},
		Size:     coordinate.Size{Width: mw, Height: mh},
	}

	rendered := b.Render(modalRect)
	if offY > 0 {
		rendered = strings.Repeat("\n", offY) + rendered
	}
	return rendered
}
