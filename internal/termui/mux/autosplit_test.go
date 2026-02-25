package mux

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewAutoSplitModel_Defaults(t *testing.T) {
	m := NewAutoSplitModel()
	if m.width != 80 {
		t.Errorf("default width = %d, want 80", m.width)
	}
	if m.height != 24 {
		t.Errorf("default height = %d, want 24", m.height)
	}
	if m.maxLines != 500 {
		t.Errorf("default maxLines = %d, want 500", m.maxLines)
	}
	if len(m.steps) != 0 {
		t.Errorf("default steps = %d, want 0", len(m.steps))
	}
	if m.done || m.quitting {
		t.Error("model should not start as done/quitting")
	}
}

func TestAutoSplitModel_WithMaxLines(t *testing.T) {
	m := NewAutoSplitModel(WithAutoSplitMaxLines(50))
	if m.maxLines != 50 {
		t.Errorf("maxLines = %d, want 50", m.maxLines)
	}
	// Enforce minimum.
	m2 := NewAutoSplitModel(WithAutoSplitMaxLines(3))
	if m2.maxLines != 10 {
		t.Errorf("maxLines clamped = %d, want 10", m2.maxLines)
	}
}

func TestAutoSplitModel_Init(t *testing.T) {
	m := NewAutoSplitModel()
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init should return a tick command")
	}
}

func TestAutoSplitModel_StepStartMsg(t *testing.T) {
	m := NewAutoSplitModel()
	msg := AutoSplitStepStartMsg{Name: "Analyze diff"}
	next, _ := m.Update(msg)
	model := next.(*AutoSplitModel)

	if len(model.steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(model.steps))
	}
	s := model.steps[0]
	if s.Name != "Analyze diff" {
		t.Errorf("name = %q, want %q", s.Name, "Analyze diff")
	}
	if s.Status != StepRunning {
		t.Errorf("status = %d, want StepRunning(%d)", s.Status, StepRunning)
	}
	if s.StartedAt.IsZero() {
		t.Error("StartedAt should be set")
	}
}

func TestAutoSplitModel_StepDoneMsg_Success(t *testing.T) {
	m := NewAutoSplitModel()
	// Start step first.
	m.Update(AutoSplitStepStartMsg{Name: "Analyze diff"})
	// Finish it.
	next, _ := m.Update(AutoSplitStepDoneMsg{
		Name:    "Analyze diff",
		Err:     "",
		Elapsed: 150 * time.Millisecond,
	})
	model := next.(*AutoSplitModel)
	s := model.steps[0]
	if s.Status != StepDone {
		t.Errorf("status = %d, want StepDone(%d)", s.Status, StepDone)
	}
	if s.Elapsed != 150*time.Millisecond {
		t.Errorf("elapsed = %v, want 150ms", s.Elapsed)
	}
	if s.Error != "" {
		t.Errorf("error = %q, want empty", s.Error)
	}
}

func TestAutoSplitModel_StepDoneMsg_Failure(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitStepStartMsg{Name: "Spawn Claude"})
	next, _ := m.Update(AutoSplitStepDoneMsg{
		Name:    "Spawn Claude",
		Err:     "binary not found",
		Elapsed: 50 * time.Millisecond,
	})
	model := next.(*AutoSplitModel)
	s := model.steps[0]
	if s.Status != StepFailed {
		t.Errorf("status = %d, want StepFailed(%d)", s.Status, StepFailed)
	}
	if s.Error != "binary not found" {
		t.Errorf("error = %q, want %q", s.Error, "binary not found")
	}
}

func TestAutoSplitModel_OutputMsg(t *testing.T) {
	m := NewAutoSplitModel()
	next, _ := m.Update(AutoSplitOutputMsg{Text: "line 1\nline 2"})
	model := next.(*AutoSplitModel)
	if len(model.outputLines) != 2 {
		t.Fatalf("outputLines = %d, want 2", len(model.outputLines))
	}
	if model.outputLines[0] != "line 1" || model.outputLines[1] != "line 2" {
		t.Errorf("lines = %v, want [line 1, line 2]", model.outputLines)
	}
}

func TestAutoSplitModel_ErrorMsg(t *testing.T) {
	m := NewAutoSplitModel()
	next, _ := m.Update(AutoSplitErrorMsg{Text: "git failed"})
	model := next.(*AutoSplitModel)
	if len(model.outputLines) != 1 {
		t.Fatalf("outputLines = %d, want 1", len(model.outputLines))
	}
	if !strings.HasPrefix(model.outputLines[0], "ERROR:") {
		t.Errorf("output = %q, want ERROR: prefix", model.outputLines[0])
	}
}

func TestAutoSplitModel_DoneMsg(t *testing.T) {
	m := NewAutoSplitModel()
	next, _ := m.Update(AutoSplitDoneMsg{Summary: "3 splits created"})
	model := next.(*AutoSplitModel)
	if !model.done {
		t.Error("done should be true")
	}
	if model.doneSummary != "3 splits created" {
		t.Errorf("summary = %q, want %q", model.doneSummary, "3 splits created")
	}
}

func TestAutoSplitModel_WindowSizeMsg(t *testing.T) {
	m := NewAutoSplitModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := next.(*AutoSplitModel)
	if model.width != 120 || model.height != 40 {
		t.Errorf("dimensions = %dx%d, want 120x40", model.width, model.height)
	}
}

func TestAutoSplitModel_CtrlC_Quit(t *testing.T) {
	m := NewAutoSplitModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C should return tea.Quit command")
	}
	if !m.quitting {
		t.Error("quitting should be true after Ctrl+C")
	}
}

func TestAutoSplitModel_Q_Quit(t *testing.T) {
	m := NewAutoSplitModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("'q' should return tea.Quit command")
	}
	if !m.quitting {
		t.Error("quitting should be true after 'q'")
	}
}

func TestAutoSplitModel_View_Empty(t *testing.T) {
	m := NewAutoSplitModel()
	view := m.View()
	if !strings.Contains(view, "Auto-Split Pipeline") {
		t.Errorf("empty view should contain header, got:\n%s", view)
	}
}

func TestAutoSplitModel_View_WithSteps(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitStepStartMsg{Name: "Analyze diff"})
	m.Update(AutoSplitStepDoneMsg{Name: "Analyze diff", Elapsed: 200 * time.Millisecond})
	m.Update(AutoSplitStepStartMsg{Name: "Spawn Claude"})

	view := m.View()

	// Should contain step names.
	if !strings.Contains(view, "Analyze diff") {
		t.Errorf("view should contain 'Analyze diff', got:\n%s", view)
	}
	if !strings.Contains(view, "Spawn Claude") {
		t.Errorf("view should contain 'Spawn Claude', got:\n%s", view)
	}
	// Should contain done icon.
	if !strings.Contains(view, "✓") {
		t.Errorf("view should contain ✓ icon for completed step, got:\n%s", view)
	}
	// Running icon.
	if !strings.Contains(view, "◉") {
		t.Errorf("view should contain ◉ icon for running step, got:\n%s", view)
	}
}

func TestAutoSplitModel_View_WithFailure(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitStepStartMsg{Name: "Spawn Claude"})
	m.Update(AutoSplitStepDoneMsg{Name: "Spawn Claude", Err: "not found", Elapsed: 10 * time.Millisecond})

	view := m.View()
	if !strings.Contains(view, "✗") {
		t.Errorf("view should contain ✗ icon for failed step, got:\n%s", view)
	}
	if !strings.Contains(view, "not found") {
		t.Errorf("view should contain error text, got:\n%s", view)
	}
}

func TestAutoSplitModel_View_SeparatorShowsCurrent(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitStepStartMsg{Name: "Execute split"})

	view := m.View()
	if !strings.Contains(view, "Execute split") {
		t.Errorf("separator should show current step name, got:\n%s", view)
	}
}

func TestAutoSplitModel_View_DoneSeparator(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitDoneMsg{Summary: "done"})

	view := m.View()
	if !strings.Contains(view, "Complete") {
		t.Errorf("separator should show complete when done, got:\n%s", view)
	}
}

func TestAutoSplitModel_View_OutputLines(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitOutputMsg{Text: "creating branch split/01"})

	view := m.View()
	if !strings.Contains(view, "creating branch split/01") {
		t.Errorf("view should contain output line, got:\n%s", view)
	}
}

func TestAutoSplitModel_View_Quitting(t *testing.T) {
	m := NewAutoSplitModel()
	m.quitting = true
	view := m.View()
	if view != "" {
		t.Errorf("quitting view should be empty, got: %q", view)
	}
}

func TestAutoSplitModel_EnsureStep_Idempotent(t *testing.T) {
	m := NewAutoSplitModel()
	m.ensureStep("Analyze diff")
	m.ensureStep("Analyze diff") // duplicate
	if len(m.steps) != 1 {
		t.Errorf("steps = %d, want 1 (idempotent)", len(m.steps))
	}
}

func TestAutoSplitModel_StepDone_NewStep(t *testing.T) {
	// StepDone for a step that was never started should create it.
	m := NewAutoSplitModel()
	next, _ := m.Update(AutoSplitStepDoneMsg{
		Name:    "Unknown step",
		Err:     "",
		Elapsed: 100 * time.Millisecond,
	})
	model := next.(*AutoSplitModel)
	if len(model.steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(model.steps))
	}
	if model.steps[0].Status != StepDone {
		t.Errorf("status = %d, want StepDone(%d)", model.steps[0].Status, StepDone)
	}
}

func TestAutoSplitModel_OutputCapping(t *testing.T) {
	m := NewAutoSplitModel(WithAutoSplitMaxLines(10))
	// Add 15 lines.
	for i := 0; i < 15; i++ {
		m.Update(AutoSplitOutputMsg{Text: "line"})
	}
	if len(m.outputLines) > 10 {
		t.Errorf("outputLines = %d, want <= 10 (capped)", len(m.outputLines))
	}
}

func TestAutoSplitModel_TickMsg(t *testing.T) {
	m := NewAutoSplitModel()
	// Tick should return another tick command (not nil).
	_, cmd := m.Update(autoSplitTickMsg(time.Now()))
	if cmd == nil {
		t.Error("tick should return next tick command")
	}
	// When quitting, should not schedule another tick.
	m.quitting = true
	_, cmd = m.Update(autoSplitTickMsg(time.Now()))
	if cmd != nil {
		t.Error("tick should return nil when quitting")
	}
}

func TestAutoSplitModel_MultipleSteps_FullPipeline(t *testing.T) {
	m := NewAutoSplitModel()

	steps := []string{"Analyze diff", "Spawn Claude", "Classify", "Plan", "Execute", "Verify"}
	for _, name := range steps {
		m.Update(AutoSplitStepStartMsg{Name: name})
		m.Update(AutoSplitStepDoneMsg{Name: name, Elapsed: 100 * time.Millisecond})
	}
	m.Update(AutoSplitDoneMsg{Summary: "6 steps passed"})

	if len(m.steps) != 6 {
		t.Fatalf("steps = %d, want 6", len(m.steps))
	}
	for _, s := range m.steps {
		if s.Status != StepDone {
			t.Errorf("step %q status = %d, want StepDone", s.Name, s.Status)
		}
	}
	if !m.done {
		t.Error("pipeline should be done")
	}

	view := m.View()
	// All step names should be visible.
	for _, name := range steps {
		if !strings.Contains(view, name) {
			t.Errorf("view should contain step %q", name)
		}
	}
}

func TestStepIcon(t *testing.T) {
	tests := []struct {
		status StepStatus
		icon   string
	}{
		{StepPending, "○"},
		{StepRunning, "◉"},
		{StepDone, "✓"},
		{StepFailed, "✗"},
		{StepStatus(99), "?"},
	}
	for _, tt := range tests {
		got := stepIcon(tt.status)
		if got != tt.icon {
			t.Errorf("stepIcon(%d) = %q, want %q", tt.status, got, tt.icon)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{1500 * time.Millisecond, "1.5s"},
		{90 * time.Second, "1m30s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello world", 5) != "hello" {
		t.Errorf("truncate: got %q", truncate("hello world", 5))
	}
	if truncate("hi", 10) != "hi" {
		t.Error("truncate should not touch short strings")
	}
	if truncate("abc", 0) != "" {
		t.Error("truncate width=0 should return empty")
	}
}

func TestAutoSplitModel_SendMethods_NilProgram(t *testing.T) {
	// Send methods should be no-ops when program is nil (not started).
	m := NewAutoSplitModel()
	// These should not panic.
	m.SendStepStart("test")
	m.SendStepDone("test", "", time.Millisecond)
	m.SendOutput("text")
	m.SendError("err")
	m.SendDone("summary")
}

func TestAutoSplitModel_SeparatorProgress(t *testing.T) {
	m := NewAutoSplitModel()
	m.steps = []AutoSplitStep{
		{Name: "A", Status: StepDone},
		{Name: "B", Status: StepDone},
		{Name: "C", Status: StepFailed, Error: "bad"},
		{Name: "D", Status: StepRunning},
	}
	sep := m.renderSeparator(80)
	// Should show "3/4" (3 completed = 2 done + 1 failed out of 4 total).
	if !strings.Contains(sep, "3/4") {
		t.Errorf("separator should show 3/4 progress, got: %s", sep)
	}
	// Should show failure count.
	if !strings.Contains(sep, "1 failed") {
		t.Errorf("separator should show failure count, got: %s", sep)
	}
	// Should show current running step name.
	if !strings.Contains(sep, "D") {
		t.Errorf("separator should show running step 'D', got: %s", sep)
	}
}

// --- New feature tests: cancellation, scroll, toggle ---

func TestAutoSplitModel_Cancelled_InitiallyFalse(t *testing.T) {
	m := NewAutoSplitModel()
	if m.Cancelled() {
		t.Error("Cancelled() should be false initially")
	}
}

func TestAutoSplitModel_Cancelled_TrueAfterCtrlC(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.Cancelled() {
		t.Error("Cancelled() should be true after Ctrl+C")
	}
}

func TestAutoSplitModel_Cancelled_TrueAfterQ(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !m.Cancelled() {
		t.Error("Cancelled() should be true after 'q'")
	}
}

func TestAutoSplitModel_Quit_SetsCancelled(t *testing.T) {
	m := NewAutoSplitModel()
	m.Quit()
	if !m.Cancelled() {
		t.Error("Cancelled() should be true after Quit()")
	}
}

func TestAutoSplitModel_Quit_NilProgram(t *testing.T) {
	// Quit should not panic when program is nil.
	m := NewAutoSplitModel()
	m.Quit() // no panic
}

func TestAutoSplitModel_WithToggleKey(t *testing.T) {
	m := NewAutoSplitModel(WithAutoSplitToggleKey(0x1E))
	if m.toggleKey != 0x1E {
		t.Errorf("toggleKey = 0x%02X, want 0x1E", m.toggleKey)
	}
}

func TestAutoSplitModel_DefaultToggleKey(t *testing.T) {
	m := NewAutoSplitModel()
	if m.toggleKey != DefaultToggleKey {
		t.Errorf("toggleKey = 0x%02X, want DefaultToggleKey (0x%02X)", m.toggleKey, DefaultToggleKey)
	}
}

func TestAutoSplitModel_ToggleKey_InvokesCallback(t *testing.T) {
	var called bool
	m := NewAutoSplitModel(WithAutoSplitOnToggle(func() {
		called = true
	}))
	// Send Ctrl+] key.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if cmd == nil {
		t.Fatal("toggle key should produce a command")
	}
	// Execute the command to invoke the callback.
	msg := cmd()
	if !called {
		t.Error("toggle callback should have been invoked")
	}
	if _, ok := msg.(AutoSplitToggleMsg); !ok {
		t.Errorf("command should return AutoSplitToggleMsg, got %T", msg)
	}
}

func TestAutoSplitModel_ToggleKey_NilCallback(t *testing.T) {
	// No callback set — toggle key should be silently ignored.
	m := NewAutoSplitModel()
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if cmd != nil {
		t.Error("toggle without callback should not produce a command")
	}
}

func TestAutoSplitModel_ToggleMsg_RefreshesDisplay(t *testing.T) {
	m := NewAutoSplitModel()
	_, cmd := m.Update(AutoSplitToggleMsg{})
	if cmd != nil {
		t.Error("AutoSplitToggleMsg should return nil cmd (just refresh)")
	}
}

func TestAutoSplitModel_ScrollUp_Basic(t *testing.T) {
	m := NewAutoSplitModel()
	// Add 50 lines to output.
	for i := 0; i < 50; i++ {
		m.Update(AutoSplitOutputMsg{Text: "line"})
	}
	if m.scrollOffset != 0 {
		t.Fatalf("initial scrollOffset = %d, want 0", m.scrollOffset)
	}
	m.scrollUp(5)
	if m.scrollOffset != 5 {
		t.Errorf("scrollOffset after scrollUp(5) = %d, want 5", m.scrollOffset)
	}
}

func TestAutoSplitModel_ScrollDown_Basic(t *testing.T) {
	m := NewAutoSplitModel()
	m.scrollOffset = 10
	m.scrollDown(3)
	if m.scrollOffset != 7 {
		t.Errorf("scrollOffset after scrollDown(3) = %d, want 7", m.scrollOffset)
	}
}

func TestAutoSplitModel_ScrollDown_ClampsAtZero(t *testing.T) {
	m := NewAutoSplitModel()
	m.scrollOffset = 2
	m.scrollDown(5)
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset should clamp at 0, got %d", m.scrollOffset)
	}
}

func TestAutoSplitModel_ScrollUp_ClampsAtMax(t *testing.T) {
	m := NewAutoSplitModel()
	// With default height=24, outputPaneHeight is around 13.
	// Add 20 lines — maxOffset = 20 - paneHeight.
	for i := 0; i < 20; i++ {
		m.Update(AutoSplitOutputMsg{Text: "line"})
	}
	paneHeight := m.outputPaneHeight()
	expectedMax := 20 - paneHeight
	if expectedMax < 0 {
		expectedMax = 0
	}
	m.scrollUp(999) // way over the limit
	if m.scrollOffset != expectedMax {
		t.Errorf("scrollOffset clamped = %d, want %d", m.scrollOffset, expectedMax)
	}
}

func TestAutoSplitModel_ScrollUp_NoContent(t *testing.T) {
	// With zero lines, scrollUp should stay at 0.
	m := NewAutoSplitModel()
	m.scrollUp(10)
	if m.scrollOffset != 0 {
		t.Errorf("scrollOffset with no content = %d, want 0", m.scrollOffset)
	}
}

func TestAutoSplitModel_ScrollKeys(t *testing.T) {
	m := NewAutoSplitModel()
	for i := 0; i < 50; i++ {
		m.Update(AutoSplitOutputMsg{Text: "line"})
	}

	// Up arrow.
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.scrollOffset != 1 {
		t.Errorf("after Up: scrollOffset = %d, want 1", m.scrollOffset)
	}

	// Down arrow.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.scrollOffset != 0 {
		t.Errorf("after Down: scrollOffset = %d, want 0", m.scrollOffset)
	}

	// PgUp.
	m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	expected := m.outputPaneHeight() / 2
	if m.scrollOffset != expected {
		t.Errorf("after PgUp: scrollOffset = %d, want %d", m.scrollOffset, expected)
	}

	// Home — scroll to top.
	m.Update(tea.KeyMsg{Type: tea.KeyHome})
	maxOffset := len(m.outputLines) - m.outputPaneHeight()
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.scrollOffset != maxOffset {
		t.Errorf("after Home: scrollOffset = %d, want %d (maxOffset)", m.scrollOffset, maxOffset)
	}

	// End — scroll to bottom.
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.scrollOffset != 0 {
		t.Errorf("after End: scrollOffset = %d, want 0", m.scrollOffset)
	}
}

func TestAutoSplitModel_View_WithScrollOffset(t *testing.T) {
	// Prove that scroll offset shows the correct window of lines.
	// Create a model with known dimensions and content.
	m := NewAutoSplitModel()
	m.width = 80
	m.height = 24

	// Add 50 numbered lines so we can identify them.
	for i := 0; i < 50; i++ {
		m.outputLines = append(m.outputLines, "LINE-"+strings.Repeat("0", 3-len(string(rune('0'+i%10))))+string(rune('0'+i/10))+string(rune('0'+i%10)))
	}

	// Compute pane height to know the visible window size.
	paneHeight := m.outputPaneHeight()

	// At scrollOffset = 0 (tail), last line should be visible.
	view0 := m.View()
	lastLine := m.outputLines[len(m.outputLines)-1]
	if !strings.Contains(view0, lastLine) {
		t.Errorf("at scrollOffset=0, view should contain last line %q", lastLine)
	}

	// Scroll up by 1 — the last line should no longer be visible.
	m.scrollOffset = 1
	view1 := m.View()
	if strings.Contains(view1, lastLine) {
		t.Error("at scrollOffset=1, the absolute last line should not be visible")
	}
	// But the second-to-last line should be visible.
	secondToLast := m.outputLines[len(m.outputLines)-2]
	if !strings.Contains(view1, secondToLast) {
		t.Errorf("at scrollOffset=1, second-to-last line should be visible, view:\n%s", view1)
	}

	// Scroll to top — the first lines should be visible.
	maxOffset := len(m.outputLines) - paneHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	m.scrollOffset = maxOffset
	viewTop := m.View()
	firstLine := m.outputLines[0]
	if !strings.Contains(viewTop, firstLine) {
		t.Errorf("at max scroll, first line %q should be visible, view:\n%s", firstLine, viewTop)
	}
}

func TestAutoSplitModel_SeparatorScrollIndicator(t *testing.T) {
	m := NewAutoSplitModel()
	// No scroll — no indicator.
	sep0 := m.renderSeparator(80)
	if strings.Contains(sep0, "▲") {
		t.Error("separator should not show scroll indicator when scrollOffset=0")
	}

	// With scroll — indicator.
	m.scrollOffset = 5
	sep5 := m.renderSeparator(80)
	if !strings.Contains(sep5, "▲5") {
		t.Errorf("separator should show ▲5, got: %s", sep5)
	}
}

func TestAutoSplitModel_OutputPaneHeight(t *testing.T) {
	m := NewAutoSplitModel()
	// Default: 80x24.
	h := m.outputPaneHeight()
	if h < 1 {
		t.Errorf("outputPaneHeight should be at least 1, got %d", h)
	}
	// Tiny terminal.
	m.height = 4
	h = m.outputPaneHeight()
	if h < 1 {
		t.Errorf("outputPaneHeight at tiny size should be at least 1, got %d", h)
	}
}

func TestAutoSplitModel_StepDetailMsg(t *testing.T) {
	m := NewAutoSplitModel()
	// Start a step.
	m.Update(AutoSplitStepStartMsg{Name: "Classify"})
	// Send detail update.
	m.Update(AutoSplitStepDetailMsg{Name: "Classify", Detail: "15/42 files"})
	// Verify detail is stored.
	if len(m.steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(m.steps))
	}
	if m.steps[0].Detail != "15/42 files" {
		t.Errorf("step detail = %q, want %q", m.steps[0].Detail, "15/42 files")
	}
}

func TestAutoSplitModel_StepDetailMsg_ClearedOnDone(t *testing.T) {
	m := NewAutoSplitModel()
	m.Update(AutoSplitStepStartMsg{Name: "Build"})
	m.Update(AutoSplitStepDetailMsg{Name: "Build", Detail: "compiling..."})
	if m.steps[0].Detail != "compiling..." {
		t.Fatalf("detail not set")
	}
	m.Update(AutoSplitStepDoneMsg{Name: "Build", Elapsed: 100})
	if m.steps[0].Detail != "" {
		t.Errorf("detail should be cleared after done, got %q", m.steps[0].Detail)
	}
}

func TestAutoSplitModel_StepDetail_VisibleInView(t *testing.T) {
	m := NewAutoSplitModel()
	m.width = 120
	m.height = 24
	m.Update(AutoSplitStepStartMsg{Name: "Analyze"})
	m.Update(AutoSplitStepDetailMsg{Name: "Analyze", Detail: "42 files found"})
	view := m.View()
	if !strings.Contains(view, "42 files found") {
		t.Errorf("view should contain detail '42 files found', got:\n%s", view)
	}
}

func TestAutoSplitModel_StepDetail_HiddenWhenNotRunning(t *testing.T) {
	m := NewAutoSplitModel()
	m.width = 120
	m.height = 24
	m.Update(AutoSplitStepStartMsg{Name: "Done Step"})
	m.Update(AutoSplitStepDetailMsg{Name: "Done Step", Detail: "should hide"})
	m.Update(AutoSplitStepDoneMsg{Name: "Done Step", Elapsed: 50})
	view := m.View()
	if strings.Contains(view, "should hide") {
		t.Errorf("completed step should not show detail, got:\n%s", view)
	}
}

func TestAutoSplitModel_SendStepDetail_NilProgram(t *testing.T) {
	m := NewAutoSplitModel()
	m.SendStepDetail("test", "detail") // should not panic
}

func TestAutoSplitModel_CustomToggleKey_Runes(t *testing.T) {
	// Custom toggle key (e.g. 'T') should trigger on matching rune.
	var called bool
	m := NewAutoSplitModel(
		WithAutoSplitToggleKey('T'),
		WithAutoSplitOnToggle(func() { called = true }),
	)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'T'}})
	if cmd == nil {
		t.Fatal("custom toggle key 'T' should produce a command")
	}
	cmd()
	if !called {
		t.Error("callback should have been invoked")
	}
}

func TestAutoSplitModel_CustomToggleKey_CtrlBracketIgnored(t *testing.T) {
	// When toggleKey is custom, Ctrl+] should NOT trigger toggle.
	var called bool
	m := NewAutoSplitModel(
		WithAutoSplitToggleKey('X'),
		WithAutoSplitOnToggle(func() { called = true }),
	)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlCloseBracket})
	if cmd != nil {
		t.Error("Ctrl+] should not trigger when toggleKey is custom ('X')")
	}
	if called {
		t.Error("callback should not have been invoked")
	}
}

func TestAutoSplitModel_StepDetail_PendingStepHidden(t *testing.T) {
	// Detail on a Pending step should not render (only Running steps show detail).
	m := NewAutoSplitModel()
	m.width = 120
	m.height = 24
	// ensureStep creates a Pending step.
	m.Update(AutoSplitStepDetailMsg{Name: "Pending Step", Detail: "invisible"})
	view := m.View()
	if strings.Contains(view, "invisible") {
		t.Error("detail on Pending step should not be visible in View()")
	}
}

func TestAutoSplitModel_MixedStepStatuses(t *testing.T) {
	// Render with realistic mixed statuses: Done, Failed, Running, Pending.
	m := NewAutoSplitModel()
	m.width = 120
	m.height = 24

	m.Update(AutoSplitStepStartMsg{Name: "Analyze"})
	m.Update(AutoSplitStepDoneMsg{Name: "Analyze", Elapsed: 500})

	m.Update(AutoSplitStepStartMsg{Name: "Classify"})
	m.Update(AutoSplitStepDoneMsg{Name: "Classify", Err: "timeout", Elapsed: 3000})

	m.Update(AutoSplitStepStartMsg{Name: "Execute"})
	m.Update(AutoSplitStepDetailMsg{Name: "Execute", Detail: "3 branches"})

	m.steps = append(m.steps, AutoSplitStep{Name: "Verify", Status: StepPending})

	view := m.View()
	if !strings.Contains(view, "✓") {
		t.Error("view should contain done icon")
	}
	if !strings.Contains(view, "✗") {
		t.Error("view should contain failed icon")
	}
	if !strings.Contains(view, "◉") {
		t.Error("view should contain running icon")
	}
	if !strings.Contains(view, "○") {
		t.Error("view should contain pending icon")
	}
	if !strings.Contains(view, "3 branches") {
		t.Error("view should contain detail for running step")
	}
	if !strings.Contains(view, "timeout") {
		t.Error("view should contain error message for failed step")
	}
}

func TestAutoSplitModel_NarrowTerminal(t *testing.T) {
	// View should not panic with very narrow terminal.
	m := NewAutoSplitModel()
	m.width = 10
	m.height = 6
	m.Update(AutoSplitStepStartMsg{Name: "Test"})
	m.Update(AutoSplitOutputMsg{Text: "some output"})
	// Should not panic.
	view := m.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}

func TestAutoSplitModel_StepOverflow(t *testing.T) {
	// When more steps than fit in the top pane, renderSteps should show
	// only the most recent ones (windowing).
	m := NewAutoSplitModel()
	m.width = 80
	m.height = 12 // small terminal

	// Add many steps.
	for i := 0; i < 15; i++ {
		name := "Step-" + string(rune('A'+i))
		m.Update(AutoSplitStepStartMsg{Name: name})
		m.Update(AutoSplitStepDoneMsg{Name: name, Elapsed: 100})
	}

	view := m.View()
	// The last step should be visible.
	if !strings.Contains(view, "Step-O") { // 'A'+14 = 'O'
		t.Errorf("last step 'Step-O' should be visible, view:\n%s", view)
	}
	// The first step may be scrolled out of view (depends on topMax).
	// With height=12, topMax = 12*2/5 = 4, slotsForSteps = 3.
	// So only the last 3 steps should be visible.
	if strings.Contains(view, "Step-A") {
		t.Error("first step should be scrolled out of view")
	}
}
