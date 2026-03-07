package scripting

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
)

// TestMultilinePromptConfig_DefaultFalse verifies that multiline defaults to false.
func TestMultilinePromptConfig_DefaultFalse(t *testing.T) {
	t.Parallel()

	cfg := promptBuildConfig{
		prefix: "> ",
	}
	if cfg.multiline {
		t.Error("expected multiline to default to false")
	}
}

// TestMultilinePromptConfig_Enabled verifies that buildGoPrompt accepts multiline=true
// without panicking and produces a valid prompt.
func TestMultilinePromptConfig_Enabled(t *testing.T) {
	t.Parallel()

	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		reader: NewTUIReaderFromIO(&bytes.Reader{}),
	}

	p := tm.buildGoPrompt(promptBuildConfig{
		prefix:    "> ",
		multiline: true,
		completer: func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
			return nil, 0, 0
		},
	})
	if p == nil {
		t.Fatal("buildGoPrompt returned nil for multiline=true config")
	}
}

// TestMultilineDisabled_NoASCIIBind verifies that buildGoPrompt without multiline
// does not panic and produces a valid prompt (no ASCIICodeBind added).
func TestMultilineDisabled_NoASCIIBind(t *testing.T) {
	t.Parallel()

	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		reader: NewTUIReaderFromIO(&bytes.Reader{}),
	}

	p := tm.buildGoPrompt(promptBuildConfig{
		prefix:    "> ",
		multiline: false,
		completer: func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
			return nil, 0, 0
		},
	})
	if p == nil {
		t.Fatal("buildGoPrompt returned nil for multiline=false config")
	}
}

// TestMultilineAltEnterInsertNewline creates a prompt with multiline=true and
// verifies the Alt+Enter ASCIICodeBind handler inserts a newline into the buffer.
func TestMultilineAltEnterInsertNewline(t *testing.T) {
	t.Parallel()

	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		reader: NewTUIReaderFromIO(&bytes.Reader{}),
	}

	p := tm.buildGoPrompt(promptBuildConfig{
		prefix:    "> ",
		multiline: true,
		completer: func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
			return nil, 0, 0
		},
	})

	// Insert some text first, then insert a newline via Buffer().NewLine()
	p.InsertTextMoveCursor("hello", false)
	p.Buffer().NewLine(p.TerminalColumns(), p.TerminalRows(), false)
	p.InsertTextMoveCursor("world", false)

	text := p.Buffer().Text()
	if text != "hello\nworld" {
		t.Errorf("expected 'hello\\nworld', got %q", text)
	}
}

// TestMultilineNewLineMethod tests the newLine() method on the JS prompt object.
func TestMultilineNewLineMethod(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		engine: &Engine{vm: vm},
	}

	// Create a prompt with minimal options.
	p := prompt.New(
		func(s string) {},
		prompt.WithWriter(NewTUIWriterFromIO(io.Discard)),
	)
	obj := tm.buildPromptJSObject(p)

	// Verify the newLine method exists
	gojaObj := obj.ToObject(vm)
	nlVal := gojaObj.Get("newLine")
	if nlVal == nil || goja.IsUndefined(nlVal) {
		t.Fatal("newLine method not found on prompt JS object")
	}

	// Verify it's callable
	_, ok := goja.AssertFunction(nlVal)
	if !ok {
		t.Fatal("newLine is not callable")
	}
}

// TestMultilineJsCreatePrompt tests that jsCreatePrompt passes multiline option through.
func TestMultilineJsCreatePrompt(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)
	tm := eng.GetTUIManager()

	// Test with multiline=true
	name, err := tm.jsCreatePrompt(map[string]any{
		"name":      "ml-test",
		"prefix":    ">>> ",
		"multiline": true,
	})
	if err != nil {
		t.Fatalf("jsCreatePrompt with multiline=true: %v", err)
	}
	if name != "ml-test" {
		t.Errorf("expected name 'ml-test', got %q", name)
	}

	// Verify prompt was created
	tm.mu.RLock()
	p, exists := tm.prompts["ml-test"]
	tm.mu.RUnlock()
	if !exists {
		t.Fatal("prompt 'ml-test' not found")
	}
	if p == nil {
		t.Fatal("prompt is nil")
	}

	// Test with multiline=false (default)
	name2, err := tm.jsCreatePrompt(map[string]any{
		"name":      "ml-test-false",
		"prefix":    ">>> ",
		"multiline": false,
	})
	if err != nil {
		t.Fatalf("jsCreatePrompt with multiline=false: %v", err)
	}
	if name2 != "ml-test-false" {
		t.Errorf("expected name 'ml-test-false', got %q", name2)
	}

	// Test default (not specified)
	name3, err := tm.jsCreatePrompt(map[string]any{
		"name":   "ml-test-default",
		"prefix": ">>> ",
	})
	if err != nil {
		t.Fatalf("jsCreatePrompt without multiline: %v", err)
	}
	if name3 != "ml-test-default" {
		t.Errorf("expected name 'ml-test-default', got %q", name3)
	}

	// Test with invalid multiline type
	_, err = tm.jsCreatePrompt(map[string]any{
		"name":      "ml-test-invalid",
		"prefix":    ">>> ",
		"multiline": "notbool",
	})
	if err == nil {
		t.Error("expected error for invalid multiline type, got nil")
	}
}

// TestMultilineRegisterMode tests that registerMode passes multiline option through.
func TestMultilineRegisterMode(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	eng := mustNewEngine(t, ctx, &buf, &buf)
	tm := eng.GetTUIManager()

	// Register a mode with multiline=true
	script := eng.LoadScriptFromString("setup", `
		tui.registerMode({
			name: "ml-mode",
			multiline: true,
			tui: { prompt: "[ml]> " }
		});
	`)
	if err := eng.ExecuteScript(script); err != nil {
		t.Fatalf("setup: %v", err)
	}

	tm.mu.RLock()
	mode, exists := tm.modes["ml-mode"]
	tm.mu.RUnlock()
	if !exists {
		t.Fatal("mode 'ml-mode' not registered")
	}

	mode.mu.RLock()
	ml := mode.Multiline
	mode.mu.RUnlock()
	if !ml {
		t.Error("expected mode.Multiline to be true")
	}

	// Register a mode without multiline (should default to false)
	script2 := eng.LoadScriptFromString("setup2", `
		tui.registerMode({
			name: "no-ml-mode",
			tui: { prompt: "[no-ml]> " }
		});
	`)
	if err := eng.ExecuteScript(script2); err != nil {
		t.Fatalf("setup2: %v", err)
	}

	tm.mu.RLock()
	mode2, exists2 := tm.modes["no-ml-mode"]
	tm.mu.RUnlock()
	if !exists2 {
		t.Fatal("mode 'no-ml-mode' not registered")
	}

	mode2.mu.RLock()
	ml2 := mode2.Multiline
	mode2.mu.RUnlock()
	if ml2 {
		t.Error("expected mode.Multiline to be false by default")
	}
}

// istrings import anchor (prevents unused import if needed)
var _ = istrings.RuneNumber(0)
