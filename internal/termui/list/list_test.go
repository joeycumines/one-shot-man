package list

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestList_EmptyList(t *testing.T) {
	l := NewList()
	got := l.Render(bounds(10, 5))
	if got != "" {
		t.Errorf("expected empty string for empty list, got %q", got)
	}
}

func TestList_SingleItem(t *testing.T) {
	l := NewList(WithListItems([]ListItem{{Text: "only"}}))
	got := l.Render(bounds(10, 5))
	if got != "only" {
		t.Errorf("expected %q, got %q", "only", got)
	}
}

func TestList_MultipleItems(t *testing.T) {
	items := []ListItem{
		{Text: "alpha"},
		{Text: "beta"},
		{Text: "gamma"},
	}
	l := NewList(WithListItems(items))
	got := l.Render(bounds(10, 3))
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "alpha" {
		t.Errorf("expected first line %q, got %q", "alpha", lines[0])
	}
	if lines[1] != "beta" {
		t.Errorf("expected second line %q, got %q", "beta", lines[1])
	}
	if lines[2] != "gamma" {
		t.Errorf("expected third line %q, got %q", "gamma", lines[2])
	}
}

func TestList_SelectedItemHighlight(t *testing.T) {
	items := []ListItem{
		{Text: "alpha"},
		{Text: "beta"},
		{Text: "gamma"},
	}
	selStyle := lipgloss.NewStyle().Bold(true)
	l := NewList(WithListItems(items), WithListSelectedStyle(selStyle), WithListSelectedIndex(1))
	got := l.Render(bounds(10, 3))
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// Selected item (index 1) should be rendered with bold style (contains ANSI).
	if lines[0] == "beta" {
		t.Error("expected first item to NOT be the selected text (should be 'alpha')")
	}
	if lines[1] == "beta" {
		t.Error("expected selected item to have ANSI escape codes from bold style, got plain text")
	}
	if !strings.Contains(lines[1], "beta") {
		t.Errorf("expected selected line to contain 'beta', got %q", lines[1])
	}
	// Non-selected items should be plain.
	if lines[0] != "alpha" {
		t.Errorf("expected unselected first item to be plain %q, got %q", "alpha", lines[0])
	}
}

func TestList_BoundsTruncation(t *testing.T) {
	items := []ListItem{
		{Text: "one"},
		{Text: "two"},
		{Text: "three"},
		{Text: "four"},
		{Text: "five"},
	}
	l := NewList(WithListItems(items))
	got := l.Render(bounds(10, 2))
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (truncated by bounds), got %d", len(lines))
	}
	if lines[0] != "one" {
		t.Errorf("expected first line %q, got %q", "one", lines[0])
	}
	if lines[1] != "two" {
		t.Errorf("expected second line %q, got %q", "two", lines[1])
	}
}

func TestList_ZeroBounds(t *testing.T) {
	items := []ListItem{{Text: "item"}}
	l := NewList(WithListItems(items))
	tests := []struct {
		name string
		b    coordinate.Rect
	}{
		{"zero width", bounds(0, 5)},
		{"zero height", bounds(5, 0)},
		{"negative width", bounds(-1, 5)},
		{"negative height", bounds(5, -1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := l.Render(tt.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestList_SelectedIndexOutOfRange(t *testing.T) {
	items := []ListItem{{Text: "a"}, {Text: "b"}}
	l := NewList(WithListItems(items), WithListSelectedIndex(99))
	got := l.Render(bounds(10, 2))
	lines := strings.Split(got, "\n")
	// Index 99 is out of range, so no item gets selected style — all plain.
	if lines[0] != "a" {
		t.Errorf("expected %q, got %q", "a", lines[0])
	}
	if lines[1] != "b" {
		t.Errorf("expected %q, got %q", "b", lines[1])
	}
}

func TestList_OptionsChaining(t *testing.T) {
	items := []ListItem{{Text: "x"}, {Text: "y"}}
	l := NewList(
		WithListItems(items),
		WithListSelectedIndex(0),
	)
	if len(l.items) != 2 {
		t.Errorf("expected 2 items, got %d", len(l.items))
	}
	if l.selectedIndex != 0 {
		t.Errorf("expected selectedIndex 0, got %d", l.selectedIndex)
	}
}
