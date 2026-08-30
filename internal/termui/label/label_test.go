package label

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestLabel_BasicRender(t *testing.T) {
	l := NewLabel("hello")
	got := l.Render(bounds(20, 1))
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestLabel_WithStyle(t *testing.T) {
	style := lipgloss.NewStyle().Bold(true)
	l := NewLabel("hello", WithLabelStyle(style))
	got := l.Render(bounds(20, 1))
	if !strings.Contains(got, "hello") {
		t.Errorf("expected output to contain %q, got %q", "hello", got)
	}
}

func TestLabel_Truncation(t *testing.T) {
	l := NewLabel("hello world", WithLabelMaxWidth(5))
	got := l.Render(bounds(20, 1))
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestLabel_ZeroBounds(t *testing.T) {
	l := NewLabel("hello")
	tests := []struct {
		name string
		b    coordinate.Rect
	}{
		{"zero width", bounds(0, 1)},
		{"zero height", bounds(1, 0)},
		{"negative width", bounds(-1, 1)},
		{"negative height", bounds(1, -1)},
		{"both zero", bounds(0, 0)},
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

func TestLabel_MaxConstraints(t *testing.T) {
	l := NewLabel("hello world", WithLabelMaxWidth(5), WithLabelMaxHeight(1))
	got := l.Render(bounds(20, 10))
	if got != "hello" {
		t.Errorf("expected %q, got %q", "hello", got)
	}
}

func TestLabel_BoundsSmallerThanMax(t *testing.T) {
	l := NewLabel("hello world", WithLabelMaxWidth(20))
	got := l.Render(bounds(3, 1))
	if got != "hel" {
		t.Errorf("expected %q, got %q", "hel", got)
	}
}

func TestLabel_OptionsChaining(t *testing.T) {
	l := NewLabel("hi",
		WithLabelMaxWidth(10),
		WithLabelMaxHeight(5),
	)
	if l.maxWidth != 10 {
		t.Errorf("expected maxWidth 10, got %d", l.maxWidth)
	}
	if l.maxHeight != 5 {
		t.Errorf("expected maxHeight 5, got %d", l.maxHeight)
	}
}
