// Package list provides a vertical list rendering component with optional
// selection highlighting. List implements the component.Component interface
// and uses functional options for configuration.
package list

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
)

// Compile-time check that List satisfies component.Component.
var _ component.Component = List{}

// ListItem is a single entry in a List, with optional per-item styling.
type ListItem struct {
	Text  string
	Style lipgloss.Style
}

// List renders a vertical list of items, with optional selection highlighting.
type List struct {
	items         []ListItem
	selectedStyle lipgloss.Style
	selectedIndex int
}

// ListOption configures a List.
type ListOption func(*List)

// WithListItems sets the list entries.
func WithListItems(items []ListItem) ListOption { return func(l *List) { l.items = items } }

// WithListSelectedStyle sets the style applied to the currently selected item.
func WithListSelectedStyle(style lipgloss.Style) ListOption {
	return func(l *List) { l.selectedStyle = style }
}

// WithListSelectedIndex sets the index of the selected item.
func WithListSelectedIndex(index int) ListOption { return func(l *List) { l.selectedIndex = index } }

// NewList creates a List with optional configuration.
func NewList(opts ...ListOption) *List {
	l := &List{}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Render produces a vertical list of items fitting within bounds. Items beyond
// the available height are truncated. The selected item (if within range) is
// rendered with the selected style instead of its per-item style.
func (l List) Render(bounds coordinate.Rect) string {
	w, h := bounds.Size.Width, bounds.Size.Height
	if w <= 0 || h <= 0 || len(l.items) == 0 {
		return ""
	}
	maxItems := h
	if len(l.items) < maxItems {
		maxItems = len(l.items)
	}
	var lines []string
	for i := 0; i < maxItems; i++ {
		style := l.items[i].Style
		if i == l.selectedIndex {
			style = l.selectedStyle
		}
		line := label.NewLabel(l.items[i].Text, label.WithLabelStyle(style)).Render(
			coordinate.Rect{Size: coordinate.Size{Width: w, Height: 1}})
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
