// Package splitview provides a two-pane layout component for terminal UI.
// SplitView renders two components side by side (horizontal) or stacked
// (vertical), dividing the available space according to a configurable ratio.
// It implements the component.Component interface and uses functional options
// for configuration.
package splitview

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// Compile-time check that SplitView satisfies component.Component.
var _ component.Component = SplitView{}

// SplitView renders two components side by side (horizontal) or stacked
// (vertical), dividing the available space according to a configurable ratio.
type SplitView struct {
	Primary   component.Component
	Secondary component.Component
	Ratio     float64
	Direction layout.Direction
	Style     lipgloss.Style
}

// SplitViewOption configures a SplitView.
type SplitViewOption func(*SplitView)

// WithSplitViewPrimary sets the component rendered in the first (left or top)
// pane.
func WithSplitViewPrimary(c component.Component) SplitViewOption {
	return func(s *SplitView) { s.Primary = c }
}

// WithSplitViewSecondary sets the component rendered in the second (right or
// bottom) pane.
func WithSplitViewSecondary(c component.Component) SplitViewOption {
	return func(s *SplitView) { s.Secondary = c }
}

// WithSplitViewRatio sets the space allocation ratio (0.0–1.0). A ratio of 0.5
// gives equal space to both panes; 0 gives all space to the secondary pane; 1
// gives all to the primary. Default is 0.5.
func WithSplitViewRatio(r float64) SplitViewOption {
	return func(s *SplitView) { s.Ratio = r }
}

// WithSplitViewDirection sets the split axis. Default is layout.Horizontal
// (left/right).
func WithSplitViewDirection(d layout.Direction) SplitViewOption {
	return func(s *SplitView) { s.Direction = d }
}

// WithSplitViewStyle sets the lipgloss style applied to the split view.
func WithSplitViewStyle(style lipgloss.Style) SplitViewOption {
	return func(s *SplitView) { s.Style = style }
}

// NewSplitView creates a SplitView with optional configuration. The default
// ratio is 0.5 and the default direction is layout.Horizontal.
func NewSplitView(opts ...SplitViewOption) *SplitView {
	s := &SplitView{Ratio: 0.5, Direction: layout.Horizontal}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Render produces the two-pane layout within bounds. The bounds are split
// according to the configured ratio and direction, and each pane is rendered
// independently. Returns an empty string for zero or negative bounds.
func (sv SplitView) Render(bounds coordinate.Rect) string {
	w, h := bounds.Size.Width, bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	first, second := bounds.Split(sv.Ratio, sv.Direction == layout.Horizontal)

	var lines []string
	if sv.Primary != nil {
		lines = append(lines, sv.Primary.Render(first))
	}
	if sv.Secondary != nil {
		lines = append(lines, sv.Secondary.Render(second))
	}

	return strings.Join(lines, "\n")
}
