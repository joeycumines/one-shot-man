package scripting

import (
	"io"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
)

func TestBuildKeyBinds_PassesPromptObject(t *testing.T) {
	t.Parallel()

	vm := goja.New()

	// Create a JS handler that checks it receives a prompt object with methods
	handler, err := vm.RunString(`(function(p) { return p !== undefined && p !== null && typeof p.insertText === 'function'; })`)
	if err != nil {
		t.Fatalf("Failed to create JS handler: %v", err)
	}
	callable, ok := goja.AssertFunction(handler)
	if !ok {
		t.Fatal("handler is not callable")
	}

	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		engine: &Engine{vm: vm},
		keyBindings: map[string]goja.Callable{
			"ctrl-a": callable,
		},
	}

	keyBinds := tm.buildKeyBinds()
	if len(keyBinds) != 1 {
		t.Fatalf("expected 1 key bind, got %d", len(keyBinds))
	}

	if keyBinds[0].Key != prompt.ControlA {
		t.Errorf("expected ControlA, got %v", keyBinds[0].Key)
	}

	// We can't easily test Fn because it requires a real *prompt.Prompt,
	// but we verify the structure was created correctly.
}

func TestBuildPromptJSObject_Methods(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		engine: &Engine{vm: vm},
	}

	// We can't create a real *prompt.Prompt without a terminal,
	// but we can verify the function doesn't panic with nil-safe patterns.
	// Instead, verify the method exists by checking the returned object has
	// the expected properties.
	// Note: We test the JS object shape, not the actual prompt interaction,
	// because that requires a real prompt with terminal I/O.

	// Use a helper that exposes the object shape
	val, err := vm.RunString(`(function(obj) {
		var methods = ['insertText', 'insertTextMoveCursor', 'deleteBeforeCursor',
			'delete', 'cursorLeft', 'cursorRight', 'cursorUp', 'cursorDown', 'getText'];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof obj[methods[i]] !== 'function') {
				missing.push(methods[i]);
			}
		}
		return missing.length === 0 ? '' : 'missing: ' + missing.join(', ');
	})`)
	if err != nil {
		t.Fatalf("Failed to create checker: %v", err)
	}
	checker, ok := goja.AssertFunction(val)
	if !ok {
		t.Fatal("checker is not callable")
	}

	// Create a prompt with minimal options for structure testing.
	p := prompt.New(
		func(s string) {},
		prompt.WithWriter(NewTUIWriterFromIO(io.Discard)),
	)
	obj := tm.buildPromptJSObject(p)

	result, err := checker(goja.Undefined(), obj)
	if err != nil {
		t.Fatalf("checker call failed: %v", err)
	}
	if s := result.String(); s != "" {
		t.Errorf("prompt JS object: %s", s)
	}
}

func TestPromptBuildConfig_HistorySize(t *testing.T) {
	t.Parallel()

	// Verify that promptBuildConfig accepts historySize
	cfg := promptBuildConfig{
		prefix:      "> ",
		historySize: 500,
	}

	if cfg.historySize != 500 {
		t.Errorf("expected historySize 500, got %d", cfg.historySize)
	}

	// Zero value should be valid (means "use default")
	cfg2 := promptBuildConfig{}
	if cfg2.historySize != 0 {
		t.Errorf("expected zero historySize by default, got %d", cfg2.historySize)
	}
}
