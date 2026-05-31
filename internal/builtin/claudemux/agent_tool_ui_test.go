package claudemux

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/termmux/vt"
)

func TestAgentToolUI_New(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}

	a := NewAgentToolUI(d, 80, 24)
	if a == nil {
		t.Fatal("NewAgentToolUI returned nil")
	}
	if a.width != 80 {
		t.Errorf("width = %d, want 80", a.width)
	}
	if a.height != 24 {
		t.Errorf("height = %d, want 24", a.height)
	}
	if a.detector != d {
		t.Error("detector mismatch")
	}
}

func TestAgentToolUI_ProcessRaw(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	a.ProcessRaw([]byte("hello"))
	render := a.Render()
	if !strings.Contains(render, "hello") {
		t.Errorf("Render() = %q, want to contain 'hello'", render)
	}
}

func TestAgentToolUI_Render(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	// Simple text with CR+LF for clean line breaks.
	a.ProcessRaw([]byte("hello\r\nworld"))
	render := a.Render()
	if !strings.Contains(render, "hello") {
		t.Errorf("Render() = %q, want to contain 'hello'", render)
	}
	if !strings.Contains(render, "world") {
		t.Errorf("Render() = %q, want to contain 'world'", render)
	}

	// ANSI colored text.
	a2, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	ui2 := NewAgentToolUI(a2, 80, 24)
	colored := "\x1b[32mgreen\x1b[0m"
	ui2.ProcessRaw([]byte(colored))
	render2 := ui2.Render()
	if !strings.Contains(render2, "green") {
		t.Errorf("Render() with ANSI = %q, want to contain 'green'", render2)
	}
	if strings.Contains(render2, "\x1b[") {
		t.Errorf("Render() contains ANSI codes: %q", render2)
	}
}

func TestAgentToolUI_Resize(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	a.Resize(40, 10)
	if a.width != 40 {
		t.Errorf("width = %d, want 40", a.width)
	}
	if a.height != 10 {
		t.Errorf("height = %d, want 10", a.height)
	}

	row, col := a.GetCursor()
	if row != 0 || col != 0 {
		t.Errorf("cursor after resize = (%d, %d), want (0, 0)", row, col)
	}
}

func TestAgentToolUI_GetCursor(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	row, col := a.GetCursor()
	if row != 0 || col != 0 {
		t.Errorf("initial cursor = (%d, %d), want (0, 0)", row, col)
	}

	// Move cursor with text.
	a.ProcessRaw([]byte("hi"))
	row, col = a.GetCursor()
	if row != 0 {
		t.Errorf("cursor row after 'hi' = %d, want 0", row)
	}
	if col < 2 {
		t.Errorf("cursor col after 'hi' = %d, want >= 2", col)
	}
}

func TestAgentToolUI_Clear(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	a.ProcessRaw([]byte("some content"))
	render := a.Render()
	if !strings.Contains(render, "some content") {
		t.Fatalf("Render before clear missing content: %q", render)
	}

	a.Clear()
	render = a.Render()
	if strings.Contains(render, "some content") {
		t.Errorf("Render after clear still has content: %q", render)
	}
}

func TestAgentToolUI_RenderStyled(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	// Bold text via ANSI.
	a.ProcessRaw([]byte("\x1b[1mbold text\x1b[0m"))
	render := a.RenderStyled()
	// Should contain the bold characters.
	if !strings.Contains(render, "bold text") {
		t.Errorf("RenderStyled() = %q, want to contain 'bold text'", render)
	}
	// Should contain bold SGR (0;1 or 1).
	if !strings.Contains(render, ";1m") && !strings.Contains(render, "\x1b[1m") {
		t.Errorf("RenderStyled() = %q, want to contain bold SGR", render)
	}

	// Italic text.
	a2, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	ui2 := NewAgentToolUI(a2, 80, 24)
	ui2.ProcessRaw([]byte("\x1b[3mitalic\x1b[0m"))
	render2 := ui2.RenderStyled()
	if !strings.Contains(render2, "\x1b[3m") && !strings.Contains(render2, ";3m") {
		t.Errorf("RenderStyled() = %q, want to contain italic SGR", render2)
	}
}

func TestAgentToolUI_MultiLine(t *testing.T) {
	d, err := NewVTStateDetector(DefaultClaudeCodeTUIStateConfig())
	if err != nil {
		t.Fatalf("NewVTStateDetector() error = %v", err)
	}
	a := NewAgentToolUI(d, 80, 24)

	// Use \r\n for clean line breaks (CR resets column, LF moves down).
	a.ProcessRaw([]byte("line1\r\nline2\r\nline3"))
	render := a.Render()
	lines := strings.Split(render, "\n")
	if len(lines) < 3 {
		t.Fatalf("Render() has %d lines, want >= 3: %q", len(lines), render)
	}
	// Trim trailing spaces for comparison.
	cmp := func(s string) string {
		return strings.TrimRight(s, " ")
	}
	if got := cmp(lines[0]); got != "line1" {
		t.Errorf("line[0] = %q, want %q", got, "line1")
	}
	if got := cmp(lines[1]); got != "line2" {
		t.Errorf("line[1] = %q, want %q", got, "line2")
	}
	if got := cmp(lines[2]); got != "line3" {
		t.Errorf("line[2] = %q, want %q", got, "line3")
	}
}

func TestAgentToolUI_SGRDiff(t *testing.T) {
	tests := []struct {
		name  string
		attr  vt.Attr
		check func(t *testing.T, sgr string)
	}{
		{
			name: "reset",
			attr: vt.Attr{},
			check: func(t *testing.T, sgr string) {
				if sgr != "" {
					t.Errorf("SGR() = %q, want empty for zero Attr", sgr)
				}
			},
		},
		{
			name: "bold",
			attr: vt.Attr{Bold: true},
			check: func(t *testing.T, sgr string) {
				// SGRDiff includes reset (0) when setting first attribute.
				if !strings.Contains(sgr, "\x1b[") {
					t.Errorf("SGR() = %q, want non-empty SGR for bold", sgr)
				}
			},
		},
		{
			name: "italic",
			attr: vt.Attr{Italic: true},
			check: func(t *testing.T, sgr string) {
				if !strings.Contains(sgr, "\x1b[") {
					t.Errorf("SGR() = %q, want non-empty SGR for italic", sgr)
				}
			},
		},
		{
			name: "underline",
			attr: vt.Attr{Under: true},
			check: func(t *testing.T, sgr string) {
				if !strings.Contains(sgr, "\x1b[") {
					t.Errorf("SGR() = %q, want non-empty SGR for underline", sgr)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sgr := tt.attr.SGR()
			tt.check(t, sgr)
		})
	}
}
