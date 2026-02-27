package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewSplitView_Defaults(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	if sv.width != 80 {
		t.Errorf("default width = %d, want 80", sv.width)
	}
	if sv.height != 24 {
		t.Errorf("default height = %d, want 24", sv.height)
	}
	if sv.activePane != PaneOsm {
		t.Errorf("default activePane = %d, want PaneOsm", sv.activePane)
	}
	if sv.maxLines != 500 {
		t.Errorf("default maxLines = %d, want 500", sv.maxLines)
	}
	if sv.splitRatio != 0.5 {
		t.Errorf("default splitRatio = %f, want 0.5", sv.splitRatio)
	}
	if sv.claudeStatus != "idle" {
		t.Errorf("default claudeStatus = %q, want idle", sv.claudeStatus)
	}
}

func TestNewSplitView_Options(t *testing.T) {
	t.Parallel()
	sv := NewSplitView(
		WithSplitRatio(0.7),
		WithMaxLines(100),
		WithToggleKey(0x1C), // Ctrl+\
	)
	if sv.splitRatio != 0.7 {
		t.Errorf("splitRatio = %f, want 0.7", sv.splitRatio)
	}
	if sv.maxLines != 100 {
		t.Errorf("maxLines = %d, want 100", sv.maxLines)
	}
	if sv.toggleKey != 0x1C {
		t.Errorf("toggleKey = %x, want 1C", sv.toggleKey)
	}
}

func TestNewSplitView_RatioClamp(t *testing.T) {
	t.Parallel()
	// Below minimum
	sv := NewSplitView(WithSplitRatio(0.01))
	if sv.splitRatio != 0.1 {
		t.Errorf("splitRatio = %f, want 0.1 (clamped)", sv.splitRatio)
	}
	// Above maximum
	sv = NewSplitView(WithSplitRatio(0.99))
	if sv.splitRatio != 0.9 {
		t.Errorf("splitRatio = %f, want 0.9 (clamped)", sv.splitRatio)
	}
}

func TestNewSplitView_MaxLinesClamp(t *testing.T) {
	t.Parallel()
	sv := NewSplitView(WithMaxLines(1))
	if sv.maxLines != 10 {
		t.Errorf("maxLines = %d, want 10 (clamped)", sv.maxLines)
	}
}

func TestSplitView_AppendOsmOutput(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.AppendOsmOutput("line1\nline2\nline3")
	if len(sv.osmLines) != 3 {
		t.Errorf("osmLines count = %d, want 3", len(sv.osmLines))
	}
	if sv.osmLines[0] != "line1" || sv.osmLines[2] != "line3" {
		t.Errorf("osmLines = %v, want [line1, line2, line3]", sv.osmLines)
	}
}

func TestSplitView_AppendClaudeOutput(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.AppendClaudeOutput("response A")
	sv.AppendClaudeOutput("response B")
	if len(sv.claudeLines) != 2 {
		t.Errorf("claudeLines count = %d, want 2", len(sv.claudeLines))
	}
}

func TestSplitView_AppendCapping(t *testing.T) {
	t.Parallel()
	sv := NewSplitView(WithMaxLines(10))
	for i := 0; i < 20; i++ {
		sv.AppendOsmOutput("line")
	}
	if len(sv.osmLines) != 10 {
		t.Errorf("osmLines count = %d after capping, want 10", len(sv.osmLines))
	}
}

func TestSplitView_SetClaudeStatus(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.SetClaudeStatus("thinking")
	if sv.claudeStatus != "thinking" {
		t.Errorf("claudeStatus = %q, want thinking", sv.claudeStatus)
	}
	// Verify the status appears in the rendered separator bar.
	sep := sv.renderSeparator(80)
	if !strings.Contains(sep, "thinking") {
		t.Errorf("separator should contain status 'thinking', got: %s", sep)
	}
}

func TestSplitView_SetSplitRatio(t *testing.T) {
	t.Parallel()
	sv := NewSplitView(WithSplitRatio(0.5))
	sv.SetSplitRatio(0.7)
	sv.mu.Lock()
	ratio := sv.splitRatio
	sv.mu.Unlock()
	if ratio != 0.7 {
		t.Errorf("splitRatio = %f, want 0.7", ratio)
	}
	// Test clamping.
	sv.SetSplitRatio(0.01)
	sv.mu.Lock()
	ratio = sv.splitRatio
	sv.mu.Unlock()
	if ratio != 0.1 {
		t.Errorf("splitRatio = %f after low clamp, want 0.1", ratio)
	}
	sv.SetSplitRatio(0.99)
	sv.mu.Lock()
	ratio = sv.splitRatio
	sv.mu.Unlock()
	if ratio != 0.9 {
		t.Errorf("splitRatio = %f after high clamp, want 0.9", ratio)
	}
}

func TestSplitView_ActivePane(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	if sv.ActivePane() != PaneOsm {
		t.Fatal("initial ActivePane should be PaneOsm")
	}
}

func TestSplitView_Init(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	cmd := sv.Init()
	if cmd != nil {
		t.Error("Init should return nil")
	}
}

func TestSplitView_WindowSizeMsg(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	m, _ := sv.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated := m.(*SplitView)
	if updated.width != 120 || updated.height != 40 {
		t.Errorf("size = %dx%d, want 120x40", updated.width, updated.height)
	}
}

func TestSplitView_TogglePaneSwitch(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()

	// Verify initial is osm.
	if sv.ActivePane() != PaneOsm {
		t.Fatal("expected initial pane = PaneOsm")
	}

	// Toggle to Claude.
	m, _ := sv.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	updated := m.(*SplitView)
	if updated.ActivePane() != PaneClaude {
		t.Error("expected pane = PaneClaude after toggle")
	}

	// Toggle back to osm.
	m, _ = updated.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	updated = m.(*SplitView)
	if updated.ActivePane() != PaneOsm {
		t.Error("expected pane = PaneOsm after second toggle")
	}
}

func TestSplitView_CtrlCQuits(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	_, cmd := sv.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected quit command from Ctrl+C")
	}
	if !sv.quitting {
		t.Error("expected quitting = true")
	}
}

func TestSplitView_ClaudeInputForwarding(t *testing.T) {
	t.Parallel()
	var received []byte
	sv := NewSplitView(WithClaudeWriter(func(data []byte) error {
		received = append(received, data...)
		return nil
	}))

	// Switch to Claude pane.
	sv.activePane = PaneClaude

	// Send a key.
	sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'i'}})
	if string(received) != "hi" {
		t.Errorf("received = %q, want 'hi'", received)
	}
}

func TestSplitView_OsmPaneNoForwarding(t *testing.T) {
	t.Parallel()
	called := false
	sv := NewSplitView(WithClaudeWriter(func(data []byte) error {
		called = true
		return nil
	}))
	// Default pane is osm — input should NOT forward.
	sv.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if called {
		t.Error("claudeWriter should not be called when pane is osm")
	}
}

func TestSplitView_ViewRenders(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.width = 40
	sv.height = 12
	sv.AppendOsmOutput("osm> analyze")
	sv.AppendClaudeOutput("Claude: processing...")
	sv.SetClaudeStatus("thinking")

	view := sv.View()
	if !strings.Contains(view, "analyze") {
		t.Error("view should contain osm output 'analyze'")
	}
	if !strings.Contains(view, "processing") {
		t.Error("view should contain Claude output 'processing'")
	}
	if !strings.Contains(view, "thinking") {
		t.Error("view should contain Claude status 'thinking'")
	}
	if !strings.Contains(view, "toggle") {
		t.Error("view should contain toggle key hint")
	}
}

func TestSplitView_ViewEmpty(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.width = 40
	sv.height = 12
	view := sv.View()
	if len(view) == 0 {
		t.Error("view should not be empty with no content")
	}
}

func TestSplitView_ViewQuitting(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.quitting = true
	view := sv.View()
	if view != "" {
		t.Errorf("view when quitting should be empty, got %q", view)
	}
}

func TestSplitView_SeparatorShowsActivePane(t *testing.T) {
	t.Parallel()
	sv := NewSplitView()
	sv.width = 50
	sv.height = 12

	// Default: osm active
	sep := sv.renderSeparator(50)
	if !strings.Contains(sep, "[osm]") {
		t.Error("separator should show [osm] when osm is active")
	}

	// Switch to Claude
	sv.activePane = PaneClaude
	sep = sv.renderSeparator(50)
	if !strings.Contains(sep, "[Claude]") {
		t.Error("separator should show [Claude] when Claude is active")
	}
}

// --- Direct unit tests for renderPane and appendCapped ---

func TestRenderPane_EmptyLines(t *testing.T) {
	t.Parallel()
	// Empty lines with height=3 → should return 2 newlines (height-1 padding).
	result := renderPane(nil, 3, 80)
	if result != "\n\n" {
		t.Errorf("renderPane(nil, 3, 80) = %q, want %q", result, "\n\n")
	}
}

func TestRenderPane_SingleLine(t *testing.T) {
	t.Parallel()
	// One line, height=1 → just the line, no newlines.
	result := renderPane([]string{"hello"}, 1, 80)
	if result != "hello" {
		t.Errorf("renderPane 1 line, height 1 = %q, want %q", result, "hello")
	}
}

func TestRenderPane_FewerLinesThanHeight(t *testing.T) {
	t.Parallel()
	// 2 lines but height=5 → the 2 lines joined with newline, then 3 padding newlines.
	result := renderPane([]string{"A", "B"}, 5, 80)
	// Expected: "A\nB" + "\n\n\n" = "A\nB\n\n\n"
	expected := "A\nB\n\n\n"
	if result != expected {
		t.Errorf("renderPane(2 lines, height 5) = %q, want %q", result, expected)
	}
}

func TestRenderPane_MoreLinesThanHeight(t *testing.T) {
	t.Parallel()
	// 5 lines, height=2 → should show only last 2.
	lines := []string{"A", "B", "C", "D", "E"}
	result := renderPane(lines, 2, 80)
	expected := "D\nE"
	if result != expected {
		t.Errorf("renderPane(5 lines, height 2) = %q, want %q", result, expected)
	}
}

func TestRenderPane_TruncatesWidth(t *testing.T) {
	t.Parallel()
	lines := []string{"hello world this is long"}
	result := renderPane(lines, 1, 5)
	if result != "hello" {
		t.Errorf("renderPane truncate = %q, want %q", result, "hello")
	}
}

func TestRenderPane_ZeroWidth(t *testing.T) {
	t.Parallel()
	// Width=0: lines shorter than 0 never truncate (len("x") > 0 → true),
	// but line[:0] = "". Should not panic.
	lines := []string{"abc"}
	result := renderPane(lines, 1, 0)
	if result != "" {
		t.Errorf("renderPane width=0 = %q, want empty", result)
	}
}

func TestRenderPane_EmptyStringLines(t *testing.T) {
	t.Parallel()
	lines := []string{"", "content", ""}
	result := renderPane(lines, 3, 80)
	expected := "\ncontent\n"
	if result != expected {
		t.Errorf("renderPane with empty strings = %q, want %q", result, expected)
	}
}

func TestAppendCapped_UnderCap(t *testing.T) {
	t.Parallel()
	result := appendCapped([]string{"a"}, []string{"b"}, 10)
	if len(result) != 2 || result[0] != "a" || result[1] != "b" {
		t.Errorf("appendCapped under cap = %v, want [a b]", result)
	}
}

func TestAppendCapped_OverCap(t *testing.T) {
	t.Parallel()
	existing := []string{"a", "b", "c"}
	result := appendCapped(existing, []string{"d", "e"}, 3)
	// Total 5, capped to 3 → [c, d, e]
	if len(result) != 3 || result[0] != "c" || result[1] != "d" || result[2] != "e" {
		t.Errorf("appendCapped over cap = %v, want [c d e]", result)
	}
}

func TestAppendCapped_ExactlyCap(t *testing.T) {
	t.Parallel()
	result := appendCapped([]string{"a", "b"}, []string{"c"}, 3)
	if len(result) != 3 {
		t.Errorf("appendCapped at cap = %v, want 3 items", result)
	}
}

func TestAppendCapped_NewLinesLargerThanMax(t *testing.T) {
	t.Parallel()
	// Append more new lines than max — should keep only last max.
	result := appendCapped(nil, []string{"a", "b", "c", "d", "e"}, 2)
	if len(result) != 2 || result[0] != "d" || result[1] != "e" {
		t.Errorf("appendCapped newLines > max = %v, want [d e]", result)
	}
}

func TestKeyMsgToBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  tea.KeyMsg
		want []byte
	}{
		{"runes", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}}, []byte{'A'}},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, []byte{'\r'}},
		{"backspace", tea.KeyMsg{Type: tea.KeyBackspace}, []byte{0x7f}},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, []byte{'\t'}},
		{"escape", tea.KeyMsg{Type: tea.KeyEscape}, []byte{0x1b}},
		{"space", tea.KeyMsg{Type: tea.KeySpace}, []byte{' '}},
		{"up", tea.KeyMsg{Type: tea.KeyUp}, []byte{0x1b, '[', 'A'}},
		{"down", tea.KeyMsg{Type: tea.KeyDown}, []byte{0x1b, '[', 'B'}},
		{"right", tea.KeyMsg{Type: tea.KeyRight}, []byte{0x1b, '[', 'C'}},
		{"left", tea.KeyMsg{Type: tea.KeyLeft}, []byte{0x1b, '[', 'D'}},
		{"ctrl-a", tea.KeyMsg{Type: tea.KeyCtrlA}, []byte{0x01}},
		{"ctrl-d", tea.KeyMsg{Type: tea.KeyCtrlD}, []byte{0x04}},
		{"unknown", tea.KeyMsg{Type: tea.KeyF1}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := keyMsgToBytes(tt.msg)
			if tt.want == nil {
				if got != nil {
					t.Errorf("got %v, want nil", got)
				}
				return
			}
			if string(got) != string(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
