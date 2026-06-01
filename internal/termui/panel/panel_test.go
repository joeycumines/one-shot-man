package panel

import (
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	labelmod "github.com/joeycumines/one-shot-man/internal/termui/label"
)

func bounds(w, h int) coordinate.Rect {
	return coordinate.Rect{Size: coordinate.Size{Width: w, Height: h}}
}

func TestPanel_BasicRender(t *testing.T) {
	p := NewPanel()
	got := p.Render(bounds(10, 5))
	if got == "" {
		t.Error("expected non-empty output from Panel")
	}
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Errorf("expected at least 3 lines (border), got %d", len(lines))
	}
	// Should have rounded border corners.
	topRunes := []rune(lines[0])
	if topRunes[0] != '╭' || topRunes[len(topRunes)-1] != '╮' {
		t.Errorf("expected rounded top corners, got %q", lines[0])
	}
}

func TestPanel_WithTitle(t *testing.T) {
	p := NewPanel(WithPanelTitle("My Panel"))
	got := p.Render(bounds(20, 5))
	if !strings.Contains(got, "My Panel") {
		t.Errorf("expected output to contain 'My Panel', got %q", got)
	}
}

func TestPanel_WithContent(t *testing.T) {
	var content component.Component = labelmod.NewLabel("hello world")
	p := NewPanel(WithPanelContent(content))
	got := p.Render(bounds(20, 5))
	if !strings.Contains(got, "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", got)
	}
}

func TestPanel_WithTitleAndContent(t *testing.T) {
	var content component.Component = labelmod.NewLabel("body text")
	p := NewPanel(WithPanelTitle("Title"), WithPanelContent(content))
	got := p.Render(bounds(30, 6))
	if !strings.Contains(got, "Title") {
		t.Error("expected output to contain 'Title'")
	}
	if !strings.Contains(got, "body text") {
		t.Error("expected output to contain 'body text'")
	}
}

func TestPanel_ZeroBounds(t *testing.T) {
	var content component.Component = labelmod.NewLabel("Y")
	p := NewPanel(WithPanelTitle("X"), WithPanelContent(content))
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
			got := p.Render(tt.b)
			if got != "" {
				t.Errorf("expected empty string, got %q", got)
			}
		})
	}
}

func TestPanel_WithStyle(t *testing.T) {
	style := lipgloss.NewStyle().Bold(true)
	p := NewPanel(WithPanelTitle("Styled"), WithPanelStyle(style))
	got := p.Render(bounds(20, 5))
	if !strings.Contains(got, "Styled") {
		t.Errorf("expected output to contain 'Styled', got %q", got)
	}
}

func TestPanel_ContentLargerThanBounds(t *testing.T) {
	var content component.Component = labelmod.NewLabel("this is a very long content string that exceeds bounds")
	p := NewPanel(WithPanelContent(content))
	got := p.Render(bounds(10, 3))
	if got == "" {
		t.Error("expected non-empty output")
	}
	// The inner area of a 10x3 box is 8x1, so content should be truncated.
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		visible := stripANSI(line)
		if utf8.RuneCountInString(visible) > 10 {
			t.Errorf("line exceeds bounds width: %q (visible runes: %d)", visible, utf8.RuneCountInString(visible))
		}
	}
}

func TestPanel_WithBorder(t *testing.T) {
	p := NewPanel(WithPanelBorder(lipgloss.DoubleBorder()))
	got := p.Render(bounds(10, 3))
	topRunes := []rune(strings.Split(got, "\n")[0])
	if topRunes[0] != '╔' {
		t.Errorf("expected double border top-left corner '╔', got %c", topRunes[0])
	}
}

func TestPanel_OptionsChaining(t *testing.T) {
	var content component.Component = labelmod.NewLabel("content")
	p := NewPanel(
		WithPanelTitle("Title"),
		WithPanelContent(content),
		WithPanelBorder(lipgloss.DoubleBorder()),
	)
	if p.title != "Title" {
		t.Errorf("expected title 'Title', got %q", p.title)
	}
	if p.content == nil {
		t.Error("expected content to be set")
	}
}

// stripANSI removes ANSI escape sequences from a string for measuring visible width.
func stripANSI(s string) string {
	var result []byte
	inEscape := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z') {
				inEscape = false
			}
			continue
		}
		result = append(result, s[i])
	}
	return string(result)
}
