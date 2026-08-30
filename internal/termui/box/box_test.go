package box

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestBox_BasicRender(t *testing.T) {
	b := NewBox()
	got := b.Render(bounds(10, 3))
	if got == "" {
		t.Error("expected non-empty output")
	}
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	topRunes := []rune(lines[0])
	if topRunes[0] != '╭' || topRunes[len(topRunes)-1] != '╮' {
		t.Errorf("expected rounded top corners, got %q", lines[0])
	}
}

func TestBox_WithTitle(t *testing.T) {
	b := NewBox(WithBoxTitle("Test"))
	got := b.Render(bounds(20, 3))
	if !strings.Contains(got, "Test") {
		t.Errorf("expected output to contain 'Test', got %q", got)
	}
	topLine := strings.Split(got, "\n")[0]
	topRunes := []rune(topLine)
	if topRunes[0] != '╭' || topRunes[len(topRunes)-1] != '╮' {
		t.Errorf("expected rounded corners in titled top line, got %q", topLine)
	}
}

func TestBox_WithContent(t *testing.T) {
	content := label.NewLabel("hello")
	b := NewBox(WithBoxContent(content))
	got := b.Render(bounds(10, 3))
	if !strings.Contains(got, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", got)
	}
}

func TestBox_WithBorder(t *testing.T) {
	b := NewBox(WithBoxBorder(lipgloss.DoubleBorder()))
	got := b.Render(bounds(10, 3))
	topRunes := []rune(strings.Split(got, "\n")[0])
	if topRunes[0] != '╔' {
		t.Errorf("expected double border top-left corner '╔', got %c", topRunes[0])
	}
}

func TestBox_ZeroBounds(t *testing.T) {
	b := NewBox()
	tests := []struct {
		name string
		bd   coordinate.Rect
	}{
		{"zero", bounds(0, 0)},
		{"width 1", bounds(1, 5)},
		{"height 1", bounds(5, 1)},
		{"negative", bounds(-1, -1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.Render(tt.bd)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestBox_TooSmall(t *testing.T) {
	b := NewBox(WithBoxContent(label.NewLabel("hello")))
	got := b.Render(bounds(2, 2))
	if got == "" {
		t.Error("2x2 should be minimum for border, expected non-empty")
	}
}

func TestBox_NestedBox(t *testing.T) {
	inner := NewBox(WithBoxContent(label.NewLabel("inner")))
	outer := NewBox(WithBoxContent(inner))
	got := outer.Render(bounds(20, 10))
	if !strings.Contains(got, "inner") {
		t.Errorf("expected nested output to contain 'inner', got %q", got)
	}
	innerCount := strings.Count(got, "╭")
	if innerCount < 2 {
		t.Errorf("expected at least 2 top-left corners for nested boxes, got %d", innerCount)
	}
}

func TestBox_OptionsChaining(t *testing.T) {
	b := NewBox(
		WithBoxTitle("My Title"),
		WithBoxContent(component.Component(label.NewLabel("content"))),
		WithBoxBorder(lipgloss.DoubleBorder()),
	)
	if b.title != "My Title" {
		t.Errorf("expected title 'My Title', got %q", b.title)
	}
	if b.content == nil {
		t.Error("expected content to be set")
	}
	got := b.Render(bounds(20, 5))
	if !strings.Contains(got, "My Title") {
		t.Errorf("expected output to contain 'My Title'")
	}
	if !strings.Contains(got, "content") {
		t.Errorf("expected output to contain 'content'")
	}
}
