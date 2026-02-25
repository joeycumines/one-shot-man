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
