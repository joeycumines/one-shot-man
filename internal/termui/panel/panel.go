// Package panel provides a bordered container component with an optional title
// header and nested content. Panel implements the component.Component interface
// and uses functional options for configuration.
package panel

import (
	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/box"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
)

// Compile-time check that Panel satisfies component.Component.
var _ component.Component = Panel{}

// Panel renders a bordered container with an optional title header and nested
// content component. It delegates to box.Box for the border and composes a
// title Label + content inside.
type Panel struct {
	title   string
	content component.Component
	style   lipgloss.Style
	border  lipgloss.Border
}

// PanelOption configures a Panel.
type PanelOption func(*Panel)

// WithPanelTitle sets the title displayed as a header inside the panel.
func WithPanelTitle(title string) PanelOption { return func(p *Panel) { p.title = title } }

// WithPanelContent sets the inner Component rendered below the title.
func WithPanelContent(content component.Component) PanelOption {
	return func(p *Panel) { p.content = content }
}

// WithPanelStyle sets the lipgloss style applied to the panel.
func WithPanelStyle(style lipgloss.Style) PanelOption { return func(p *Panel) { p.style = style } }

// WithPanelBorder sets the border style. Default is lipgloss.RoundedBorder().
func WithPanelBorder(border lipgloss.Border) PanelOption {
	return func(p *Panel) { p.border = border }
}

// NewPanel creates a Panel with optional configuration. The default border is
// lipgloss.RoundedBorder().
func NewPanel(opts ...PanelOption) *Panel {
	p := &Panel{border: lipgloss.RoundedBorder()}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Render produces a bordered panel fitting within bounds. The inner area is
// split into a title Label (1 row) and the content component (remaining rows).
func (p Panel) Render(bounds coordinate.Rect) string {
	inner := panelInner{title: p.title, content: p.content}
	b := box.NewBox(
		box.WithBoxContent(inner),
		box.WithBoxStyle(p.style),
		box.WithBoxBorder(p.border),
	)
	return b.Render(bounds)
}

// panelInner is an adapter that renders a title header followed by content,
// implementing component.Component for use as Box content.
type panelInner struct {
	title   string
	content component.Component
}

// Compile-time check that panelInner satisfies component.Component.
var _ component.Component = panelInner{}

// Render produces the title header (1 row) followed by the content component
// (remaining rows), joined by a newline.
func (pi panelInner) Render(bounds coordinate.Rect) string {
	w, h := bounds.Size.Width, bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}
	header := label.NewLabel(pi.title).Render(coordinate.Rect{Size: coordinate.Size{Width: w, Height: 1}})
	if h <= 1 || pi.content == nil {
		return header
	}
	body := pi.content.Render(coordinate.Rect{Size: coordinate.Size{Width: w, Height: h - 1}})
	if body == "" {
		return header
	}
	return header + "\n" + body
}
