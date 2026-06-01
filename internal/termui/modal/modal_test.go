package modal

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestModal_CenteredRender(t *testing.T) {
	content := label.NewLabel("modal content")
	m := NewModal(WithModalContent(content))
	got := m.Render(bounds(40, 10))
	if !strings.Contains(got, "modal content") {
		t.Error("expected output to contain 'modal content'")
	}
}

func TestModal_WithBorder(t *testing.T) {
	content := label.NewLabel("hello")
	m := NewModal(
		WithModalContent(content),
		WithModalBorder(lipgloss.DoubleBorder()),
	)
	got := m.Render(bounds(30, 8))
	if !strings.Contains(got, "hello") {
		t.Error("expected output to contain 'hello'")
	}
	topRunes := []rune(strings.Split(got, "\n")[0])
	if topRunes[0] != '╔' {
		t.Errorf("expected double border top-left corner '╔', got %c", topRunes[0])
	}
}

func TestModal_WithWidthHeight(t *testing.T) {
	content := label.NewLabel("small")
	m := NewModal(
		WithModalContent(content),
		WithModalWidth(10),
		WithModalHeight(3),
	)
	got := m.Render(bounds(40, 10))
	if !strings.Contains(got, "small") {
		t.Error("expected output to contain 'small'")
	}
	// Modal should be vertically centered, so there should be padding lines above.
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines (modal height), got %d", len(lines))
	}
}

func TestModal_ZeroBounds(t *testing.T) {
	content := label.NewLabel("content")
	m := NewModal(WithModalContent(content))
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
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := m.Render(tt.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestModal_WidthLargerThanBounds(t *testing.T) {
	content := label.NewLabel("hi")
	m := NewModal(WithModalContent(content), WithModalWidth(100))
	got := m.Render(bounds(20, 5))
	if !strings.Contains(got, "hi") {
		t.Error("expected output to contain 'hi'")
	}
}

func TestModal_OptionsChaining(t *testing.T) {
	m := NewModal(
		WithModalContent(label.NewLabel("body")),
		WithModalWidth(20),
		WithModalHeight(5),
		WithModalBorder(lipgloss.DoubleBorder()),
	)
	if m.Content == nil {
		t.Error("expected content to be set")
	}
	if m.Width != 20 {
		t.Errorf("expected width 20, got %d", m.Width)
	}
	if m.Height != 5 {
		t.Errorf("expected height 5, got %d", m.Height)
	}
}
