package toast

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestToast_BottomRender(t *testing.T) {
	tt := NewToast(WithToastMessage("saved"))
	got := tt.Render(bounds(20, 5))
	if !strings.Contains(got, "saved") {
		t.Error("expected output to contain 'saved'")
	}
	// Message should be at the bottom: 4 newlines then the text.
	lines := strings.Split(got, "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines (4 padding + 1 message), got %d", len(lines))
	}
	if lines[0] != "" {
		t.Errorf("expected first line to be empty (padding), got %q", lines[0])
	}
	if lines[4] != "saved" {
		t.Errorf("expected last line to be 'saved', got %q", lines[4])
	}
}

func TestToast_WithStyle(t *testing.T) {
	style := lipgloss.NewStyle().Bold(true)
	tt := NewToast(WithToastMessage("alert"), WithToastStyle(style))
	got := tt.Render(bounds(20, 3))
	if !strings.Contains(got, "alert") {
		t.Error("expected output to contain 'alert'")
	}
	// Styled text should contain ANSI escape codes.
	lastLine := strings.Split(got, "\n")[2]
	if lastLine == "alert" {
		t.Error("expected styled text to contain ANSI escape codes, got plain text")
	}
}

func TestToast_WithWidth(t *testing.T) {
	tt := NewToast(WithToastMessage("short"), WithToastWidth(5))
	got := tt.Render(bounds(20, 3))
	if !strings.Contains(got, "short") {
		t.Error("expected output to contain 'short'")
	}
}

func TestToast_ZeroBounds(t *testing.T) {
	tt := NewToast(WithToastMessage("msg"))
	tests := []struct {
		name string
		b    coordinate.Rect
	}{
		{"zero width", bounds(0, 5)},
		{"zero height", bounds(5, 0)},
		{"negative width", bounds(-1, 5)},
		{"negative height", bounds(5, -1)},
		{"both zero", bounds(0, 0)},
	}
	for _, tt2 := range tests {
		t.Run(tt2.name, func(t *testing.T) {
			got := tt.Render(tt2.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestToast_WidthLargerThanBounds(t *testing.T) {
	tt := NewToast(WithToastMessage("hi"), WithToastWidth(100))
	got := tt.Render(bounds(20, 3))
	if !strings.Contains(got, "hi") {
		t.Error("expected output to contain 'hi'")
	}
}

func TestToast_OptionsChaining(t *testing.T) {
	tt := NewToast(
		WithToastMessage("hello"),
		WithToastWidth(30),
	)
	if tt.Message != "hello" {
		t.Errorf("expected message 'hello', got %q", tt.Message)
	}
	if tt.Width != 30 {
		t.Errorf("expected width 30, got %d", tt.Width)
	}
}
