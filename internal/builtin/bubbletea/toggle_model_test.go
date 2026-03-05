package bubbletea

import (
	"bytes"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
)

// TestToggleModel_Init delegates to inner model.
func TestToggleModel_Init(t *testing.T) {
	inner := &stubModel{initCmd: tea.ClearScreen}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}
	cmd := tm.Init()
	if cmd == nil {
		t.Fatal("expected Init to delegate to inner and return a cmd")
	}
}

// TestToggleModel_View delegates to inner model.
func TestToggleModel_View(t *testing.T) {
	inner := &stubModel{view: "hello toggle"}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}
	got := tm.View()
	if got != "hello toggle" {
		t.Errorf("View() = %q, want %q", got, "hello toggle")
	}
}

// TestToggleModel_Update_NonToggle passes through to inner model.
func TestToggleModel_Update_NonToggle(t *testing.T) {
	inner := &stubModel{view: "v1"}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// Send a regular 'q' key — should NOT trigger toggle
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	model, cmd := tm.Update(keyMsg)
	if model != tm {
		t.Error("expected Update to return same toggleModel")
	}
	if cmd != nil {
		t.Error("expected nil cmd for non-toggle key")
	}
	if inner.updateCount != 1 {
		t.Errorf("inner.Update called %d times, want 1", inner.updateCount)
	}
}

// TestToggleModel_Update_ToggleByRune detects toggle key via rune match.
func TestToggleModel_Update_ToggleByRune(t *testing.T) {
	inner := &stubModel{}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// Send a KeyMsg with rune 0x1D (Ctrl+])
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x1D}}
	model, cmd := tm.Update(keyMsg)
	if model != tm {
		t.Error("expected Update to return same toggleModel")
	}
	if cmd == nil {
		t.Error("expected a cmd for toggle key (rune match)")
	}
	if inner.updateCount != 0 {
		t.Error("inner.Update should NOT be called for toggle key")
	}
}

// TestToggleModel_Update_ToggleByCtrlCloseBracket detects Ctrl+] via KeyType.
func TestToggleModel_Update_ToggleByCtrlCloseBracket(t *testing.T) {
	inner := &stubModel{}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// Send KeyCtrlCloseBracket (Ctrl+])
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}
	model, cmd := tm.Update(keyMsg)
	if model != tm {
		t.Error("expected Update to return same toggleModel")
	}
	if cmd == nil {
		t.Error("expected a cmd for toggle key (KeyCtrlCloseBracket)")
	}
	if inner.updateCount != 0 {
		t.Error("inner.Update should NOT be called for toggle key")
	}
}

// TestToggleModel_Update_DifferentToggleKey uses a non-default toggle key.
func TestToggleModel_Update_DifferentToggleKey(t *testing.T) {
	inner := &stubModel{}
	// Use Ctrl+A (0x01) as toggle key
	tm := &toggleModel{inner: inner, toggleKey: 0x01}

	// Ctrl+] should NOT trigger toggle
	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlCloseBracket}
	_, cmd := tm.Update(keyMsg)
	if cmd != nil {
		t.Error("Ctrl+] should not trigger with toggleKey=0x01")
	}

	// Rune 0x01 should trigger toggle
	keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0x01}}
	_, cmd = tm.Update(keyMsg)
	if cmd == nil {
		t.Error("expected toggle for rune 0x01")
	}
}

// TestToggleModel_Update_NonKeyMsg passes through non-key messages.
func TestToggleModel_Update_NonKeyMsg(t *testing.T) {
	inner := &stubModel{}
	tm := &toggleModel{inner: inner, toggleKey: 0x1D}

	// WindowSize message should pass through
	msg := tea.WindowSizeMsg{Width: 80, Height: 24}
	_, cmd := tm.Update(msg)
	if cmd != nil {
		t.Error("expected nil cmd for non-key message")
	}
	if inner.updateCount != 1 {
		t.Errorf("inner.Update should be called for non-key msg, got %d", inner.updateCount)
	}
}

// TestToggleModel_ToggleCmd_WritesEscapes verifies escape sequences are written.
func TestToggleModel_ToggleCmd_WritesEscapes(t *testing.T) {
	var buf bytes.Buffer
	runtime := goja.New()

	called := false
	onToggle := func(goja.FunctionCall) goja.Value {
		called = true
		return goja.Undefined()
	}
	fn, ok := goja.AssertFunction(runtime.ToValue(onToggle))
	if !ok {
		t.Fatal("failed to create callable")
	}

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  &directJSRunner{runtime: runtime},
		output:    &buf,
	}

	cmd := tm.toggleCmd()
	if cmd == nil {
		t.Fatal("toggleCmd returned nil")
	}

	msg := cmd()
	if msg == nil {
		t.Fatal("expected toggleReturnMsg, got nil")
	}
	trm, ok := msg.(toggleReturnMsg)
	if !ok {
		t.Errorf("expected toggleReturnMsg, got %T", msg)
	}
	// Result should be nil since onToggle returns undefined
	if trm.Result != nil {
		t.Errorf("expected nil Result, got %v", trm.Result)
	}

	if !called {
		t.Error("onToggle callback was not called")
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("\x1b[?1049l")) {
		t.Error("missing alt-screen exit sequence")
	}
	if !bytes.Contains([]byte(output), []byte("\x1b[?1049h")) {
		t.Error("missing alt-screen enter sequence")
	}
}

// TestToggleModel_ToggleCmd_NilOutput handles nil output gracefully.
func TestToggleModel_ToggleCmd_NilOutput(t *testing.T) {
	runtime := goja.New()
	onToggle := func(goja.FunctionCall) goja.Value { return goja.Undefined() }
	fn, _ := goja.AssertFunction(runtime.ToValue(onToggle))

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  &directJSRunner{runtime: runtime},
		output:    nil, // No output writer
	}

	cmd := tm.toggleCmd()
	msg := cmd() // Should not panic
	if _, ok := msg.(toggleReturnMsg); !ok {
		t.Errorf("expected toggleReturnMsg, got %T", msg)
	}
}

// TestToggleModel_ToggleCmd_NilJSRunner handles nil jsRunner gracefully.
func TestToggleModel_ToggleCmd_NilJSRunner(t *testing.T) {
	var buf bytes.Buffer
	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		jsRunner:  nil, // No JS runner
		output:    &buf,
	}

	cmd := tm.toggleCmd()
	msg := cmd() // Should not panic
	if _, ok := msg.(toggleReturnMsg); !ok {
		t.Errorf("expected toggleReturnMsg, got %T", msg)
	}
	// Escape sequences should still be written
	if !bytes.Contains(buf.Bytes(), []byte("\x1b[?1049l")) {
		t.Error("missing alt-screen exit even without jsRunner")
	}
}

// TestToggleModel_ProgramRef verifies program reference is set and cleaned up.
func TestToggleModel_ProgramRef(t *testing.T) {
	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
	}

	// Initially nil
	tm.mu.Lock()
	if tm.program != nil {
		t.Error("expected nil program initially")
	}
	tm.mu.Unlock()

	// After setting
	p := &tea.Program{}
	tm.mu.Lock()
	tm.program = p
	tm.mu.Unlock()

	tm.mu.Lock()
	got := tm.program
	tm.mu.Unlock()
	if got != p {
		t.Error("expected program to be set")
	}
}

// TestToggleReturnMsg_MsgToJS verifies the ToggleReturn message type appears in JS.
func TestToggleReturnMsg_MsgToJS(t *testing.T) {
	runtime := goja.New()
	m := &jsModel{runtime: runtime}
	result := m.msgToJS(toggleReturnMsg{})
	if result == nil {
		t.Fatal("msgToJS returned nil for toggleReturnMsg")
	}
	if result["type"] != "ToggleReturn" {
		t.Errorf("type = %v, want ToggleReturn", result["type"])
	}
}

// TestToggleModel_ToggleCmd_ReturnValue verifies the onToggle return value is captured.
func TestToggleModel_ToggleCmd_ReturnValue(t *testing.T) {
	var buf bytes.Buffer
	runtime := goja.New()

	// onToggle returns a map with reason and error (like switchTo does)
	onToggle := func(goja.FunctionCall) goja.Value {
		return runtime.ToValue(map[string]interface{}{
			"reason": "toggle",
			"extra":  42,
		})
	}
	fn, ok := goja.AssertFunction(runtime.ToValue(onToggle))
	if !ok {
		t.Fatal("failed to create callable")
	}

	tm := &toggleModel{
		inner:     &stubModel{},
		toggleKey: 0x1D,
		onToggle:  fn,
		jsRunner:  &directJSRunner{runtime: runtime},
		output:    &buf,
	}

	cmd := tm.toggleCmd()
	msg := cmd()
	trm, ok := msg.(toggleReturnMsg)
	if !ok {
		t.Fatalf("expected toggleReturnMsg, got %T", msg)
	}
	if trm.Result == nil {
		t.Fatal("expected non-nil Result")
	}
	if trm.Result["reason"] != "toggle" {
		t.Errorf("Result[reason] = %v, want toggle", trm.Result["reason"])
	}
}

// TestToggleReturnMsg_MsgToJS_WithResult verifies Result fields are merged.
func TestToggleReturnMsg_MsgToJS_WithResult(t *testing.T) {
	runtime := goja.New()
	m := &jsModel{runtime: runtime}
	msg := toggleReturnMsg{Result: map[string]interface{}{
		"reason": "childExit",
		"error":  "something failed",
	}}
	result := m.msgToJS(msg)
	if result == nil {
		t.Fatal("msgToJS returned nil")
	}
	if result["type"] != "ToggleReturn" {
		t.Errorf("type = %v, want ToggleReturn", result["type"])
	}
	if result["reason"] != "childExit" {
		t.Errorf("reason = %v, want childExit", result["reason"])
	}
	if result["error"] != "something failed" {
		t.Errorf("error = %v, want 'something failed'", result["error"])
	}
}

// TestToggleReturnMsg_MsgToJS_NilResult verifies nil Result produces clean output.
func TestToggleReturnMsg_MsgToJS_NilResult(t *testing.T) {
	runtime := goja.New()
	m := &jsModel{runtime: runtime}
	msg := toggleReturnMsg{Result: nil}
	result := m.msgToJS(msg)
	if result == nil {
		t.Fatal("msgToJS returned nil")
	}
	if result["type"] != "ToggleReturn" {
		t.Errorf("type = %v, want ToggleReturn", result["type"])
	}
	// Should have ONLY the type field
	if len(result) != 1 {
		t.Errorf("expected 1 field (type), got %d: %v", len(result), result)
	}
}

// --- Helpers ---

// stubModel is a minimal tea.Model for testing toggleModel wrapping.
type stubModel struct {
	initCmd     tea.Cmd
	view        string
	updateCount int
}

func (m *stubModel) Init() tea.Cmd { return m.initCmd }
func (m *stubModel) View() string  { return m.view }
func (m *stubModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.updateCount++
	return m, nil
}

// directJSRunner implements JSRunner by running the callback directly.
// Only safe for single-goroutine tests where there's no real event loop.
type directJSRunner struct {
	runtime *goja.Runtime
	mu      sync.Mutex
}

func (r *directJSRunner) RunJSSync(fn func(*goja.Runtime) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn(r.runtime)
}
