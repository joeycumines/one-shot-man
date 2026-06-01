package divider

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestDivider_Horizontal(t *testing.T) {
	d := NewDivider(layout.Horizontal)
	got := d.Render(bounds(5, 1))
	expected := "─────"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestDivider_Vertical(t *testing.T) {
	d := NewDivider(layout.Vertical)
	got := d.Render(bounds(1, 3))
	expected := "│\n│\n│"
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestDivider_CustomChar(t *testing.T) {
	d := NewDivider(layout.Horizontal, WithDividerChar('='))
	got := d.Render(bounds(3, 1))
	expected := "==="
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestDivider_WithStyle(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	d := NewDivider(layout.Horizontal, WithDividerStyle(style))
	got := d.Render(bounds(5, 1))
	if !strings.Contains(got, "─") {
		t.Errorf("expected output to contain '─', got %q", got)
	}
}

func TestDivider_ZeroBounds(t *testing.T) {
	d := NewDivider(layout.Horizontal)
	tests := []struct {
		name string
		b    coordinate.Rect
	}{
		{"zero width", bounds(0, 1)},
		{"zero height", bounds(1, 0)},
		{"negative width", bounds(-1, 1)},
		{"negative height", bounds(1, -1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := d.Render(tt.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestDivider_UnknownDirection(t *testing.T) {
	d := NewDivider(Direction(99))
	got := d.Render(bounds(5, 5))
	if got != "" {
		t.Errorf("expected empty string for unknown direction, got %q", got)
	}
}

func TestDivider_OptionsChaining(t *testing.T) {
	d := NewDivider(layout.Horizontal,
		WithDividerChar('*'),
	)
	if d.char != '*' {
		t.Errorf("expected char '*', got %c", d.char)
	}
	got := d.Render(bounds(3, 1))
	if got != "***" {
		t.Errorf("expected %q, got %q", "***", got)
	}
}
