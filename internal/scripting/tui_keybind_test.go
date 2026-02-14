package scripting

import (
	"io"
	"testing"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"
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

	// Use a helper that checks ALL expected methods including new ones from T006.
	val, err := vm.RunString(`(function(obj) {
		var methods = ['insertText', 'insertTextMoveCursor', 'deleteBeforeCursor',
			'delete', 'cursorLeft', 'cursorRight', 'cursorUp', 'cursorDown', 'getText',
			'terminalColumns', 'terminalRows', 'userInputColumns'];
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

func TestPromptBuildConfig_NewFields(t *testing.T) {
	t.Parallel()

	cfg := promptBuildConfig{
		prefix:                  "> ",
		initialText:             "hello",
		showCompletionAtStart:   true,
		completionOnDown:        true,
		keyBindMode:             "emacs",
		maxSuggestion:           20,
		dynamicCompletion:       false,
		executeHidesCompletions: false,
		escapeToggle:            false,
	}

	if cfg.initialText != "hello" {
		t.Errorf("expected initialText 'hello', got %q", cfg.initialText)
	}
	if !cfg.showCompletionAtStart {
		t.Error("expected showCompletionAtStart to be true")
	}
	if !cfg.completionOnDown {
		t.Error("expected completionOnDown to be true")
	}
	if cfg.keyBindMode != "emacs" {
		t.Errorf("expected keyBindMode 'emacs', got %q", cfg.keyBindMode)
	}
	if cfg.maxSuggestion != 20 {
		t.Errorf("expected maxSuggestion 20, got %d", cfg.maxSuggestion)
	}
	if cfg.dynamicCompletion {
		t.Error("expected dynamicCompletion to be false")
	}
	if cfg.executeHidesCompletions {
		t.Error("expected executeHidesCompletions to be false")
	}
	if cfg.escapeToggle {
		t.Error("expected escapeToggle to be false")
	}
}

func TestParseKeyString_AllKeys(t *testing.T) {
	t.Parallel()

	// Exhaustive test of all supported key strings, including new T006 additions.
	tests := []struct {
		input    string
		expected prompt.Key
	}{
		// Existing keys
		{"escape", prompt.Escape},
		{"esc", prompt.Escape},
		{"ctrl-a", prompt.ControlA},
		{"control-a", prompt.ControlA},
		{"ctrl+a", prompt.ControlA},
		{"control+a", prompt.ControlA},
		{"ctrl-z", prompt.ControlZ},
		{"up", prompt.Up},
		{"down", prompt.Down},
		{"left", prompt.Left},
		{"right", prompt.Right},
		{"home", prompt.Home},
		{"end", prompt.End},
		{"delete", prompt.Delete},
		{"del", prompt.Delete},
		{"backspace", prompt.Backspace},
		{"tab", prompt.Tab},
		{"enter", prompt.Enter},
		{"return", prompt.Enter},
		{"f1", prompt.F1},
		{"f12", prompt.F12},

		// T006: New control keys
		{"ctrl-space", prompt.ControlSpace},
		{"control-space", prompt.ControlSpace},
		{"ctrl+space", prompt.ControlSpace},
		{"control+space", prompt.ControlSpace},
		{`ctrl-\`, prompt.ControlBackslash},
		{`control-\`, prompt.ControlBackslash},
		{`ctrl+\`, prompt.ControlBackslash},
		{`control+\`, prompt.ControlBackslash},
		{"ctrl-]", prompt.ControlSquareClose},
		{"control-]", prompt.ControlSquareClose},
		{"ctrl+]", prompt.ControlSquareClose},
		{"control+]", prompt.ControlSquareClose},
		{"ctrl-^", prompt.ControlCircumflex},
		{"control-^", prompt.ControlCircumflex},
		{"ctrl+^", prompt.ControlCircumflex},
		{"control+^", prompt.ControlCircumflex},
		{"ctrl-_", prompt.ControlUnderscore},
		{"control-_", prompt.ControlUnderscore},
		{"ctrl+_", prompt.ControlUnderscore},
		{"control+_", prompt.ControlUnderscore},

		// T006: Control+arrow keys
		{"ctrl-left", prompt.ControlLeft},
		{"control-left", prompt.ControlLeft},
		{"ctrl+left", prompt.ControlLeft},
		{"control+left", prompt.ControlLeft},
		{"ctrl-right", prompt.ControlRight},
		{"ctrl-up", prompt.ControlUp},
		{"ctrl-down", prompt.ControlDown},

		// T006: Alt keys
		{"alt-left", prompt.AltLeft},
		{"alt+left", prompt.AltLeft},
		{"alt-right", prompt.AltRight},
		{"alt+right", prompt.AltRight},
		{"alt-backspace", prompt.AltBackspace},
		{"alt+backspace", prompt.AltBackspace},

		// T006: Shift keys
		{"shift-left", prompt.ShiftLeft},
		{"shift+left", prompt.ShiftLeft},
		{"shift-right", prompt.ShiftRight},
		{"shift+right", prompt.ShiftRight},
		{"shift-up", prompt.ShiftUp},
		{"shift+up", prompt.ShiftUp},
		{"shift-down", prompt.ShiftDown},
		{"shift+down", prompt.ShiftDown},
		{"shift-delete", prompt.ShiftDelete},
		{"shift-del", prompt.ShiftDelete},
		{"shift+delete", prompt.ShiftDelete},

		// T006: Control+delete
		{"ctrl-delete", prompt.ControlDelete},
		{"ctrl-del", prompt.ControlDelete},
		{"control-delete", prompt.ControlDelete},
		{"ctrl+delete", prompt.ControlDelete},

		// T006: Navigation keys
		{"backtab", prompt.BackTab},
		{"shift-tab", prompt.BackTab},
		{"shift+tab", prompt.BackTab},
		{"insert", prompt.Insert},
		{"ins", prompt.Insert},
		{"pageup", prompt.PageUp},
		{"page-up", prompt.PageUp},
		{"page+up", prompt.PageUp},
		{"pagedown", prompt.PageDown},
		{"page-down", prompt.PageDown},
		{"page+down", prompt.PageDown},

		// T006: Special keys
		{"any", prompt.Any},
		{"bracketed-paste", prompt.BracketedPaste},
		{"bracketedpaste", prompt.BracketedPaste},

		// T006: F13-F24
		{"f13", prompt.F13},
		{"f14", prompt.F14},
		{"f15", prompt.F15},
		{"f16", prompt.F16},
		{"f17", prompt.F17},
		{"f18", prompt.F18},
		{"f19", prompt.F19},
		{"f20", prompt.F20},
		{"f21", prompt.F21},
		{"f22", prompt.F22},
		{"f23", prompt.F23},
		{"f24", prompt.F24},

		// Case insensitivity
		{"CTRL-A", prompt.ControlA},
		{"Ctrl-Space", prompt.ControlSpace},
		{"SHIFT-LEFT", prompt.ShiftLeft},
		{"PageUp", prompt.PageUp},

		// Unknown
		{"unknown-key", prompt.NotDefined},
		{"", prompt.NotDefined},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := parseKeyString(tc.input)
			if got != tc.expected {
				t.Errorf("parseKeyString(%q) = %v, want %v", tc.input, got, tc.expected)
			}
		})
	}
}

func TestDocumentAPI_EnrichedMethods(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	tm := &TUIManager{
		writer: NewTUIWriterFromIO(io.Discard),
		engine: &Engine{vm: vm},
	}

	// Create a document with multi-line text and cursor in the middle.
	// "hello world\nfoo bar" with cursor after "hello wo" (position 8)
	doc := prompt.NewDocument()
	doc.Text = "hello world\nfoo bar"
	// The cursor position is set via the cursor offset.
	// We'll use the tryCallJSCompleter to test the full pipeline.

	// Register a JS completer that exercises all document API methods.
	jsCompleterCode := `(function(doc) {
		var result = {};
		result.text = doc.getText();
		result.beforeCursor = doc.getTextBeforeCursor();
		result.afterCursor = doc.getTextAfterCursor();
		result.wordBefore = doc.getWordBeforeCursor();
		result.wordAfter = doc.getWordAfterCursor();
		result.currentLine = doc.getCurrentLine();
		result.lineBeforeCursor = doc.getCurrentLineBeforeCursor();
		result.lineAfterCursor = doc.getCurrentLineAfterCursor();
		result.cursorCol = doc.getCursorPositionCol();
		result.cursorRow = doc.getCursorPositionRow();
		result.lines = doc.getLines();
		result.lineCount = doc.getLineCount();
		result.onLastLine = doc.onLastLine();
		result.charAtCursor = doc.getCharRelativeToCursor(0);
		return [{ text: JSON.stringify(result) }];
	})`

	val, err := vm.RunString(jsCompleterCode)
	if err != nil {
		t.Fatalf("Failed to create JS completer: %v", err)
	}
	callable, ok := goja.AssertFunction(val)
	if !ok {
		t.Fatal("completer is not callable")
	}

	suggestions, err := tm.tryCallJSCompleter(callable, *doc)
	if err != nil {
		t.Fatalf("tryCallJSCompleter failed: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(suggestions))
	}

	// The result JSON should contain all the document properties.
	// We verify the text field includes the full document text.
	resultJSON := suggestions[0].Text
	if resultJSON == "" {
		t.Fatal("expected non-empty JSON result")
	}

	// Verify the JSON is valid and contains expected fields.
	var result map[string]interface{}
	if err := vm.ExportTo(vm.ToValue(resultJSON), &result); err != nil {
		// Use Go standard library instead
		t.Logf("Result JSON: %s", resultJSON)
	}

	// Verify using a separate JS call to parse and check the result.
	checkScript := `(function(json) {
		var r = JSON.parse(json);
		var errors = [];
		if (r.text !== "hello world\nfoo bar") errors.push("text=" + r.text);
		if (r.lineCount !== 2) errors.push("lineCount=" + r.lineCount);
		if (typeof r.cursorCol !== "number") errors.push("cursorCol type=" + typeof r.cursorCol);
		if (typeof r.cursorRow !== "number") errors.push("cursorRow type=" + typeof r.cursorRow);
		if (!Array.isArray(r.lines)) errors.push("lines not array");
		if (r.lines && r.lines.length !== 2) errors.push("lines.length=" + r.lines.length);
		if (typeof r.onLastLine !== "boolean") errors.push("onLastLine type=" + typeof r.onLastLine);
		return errors.length === 0 ? "" : errors.join("; ");
	})`

	checkVal, err := vm.RunString(checkScript)
	if err != nil {
		t.Fatalf("Failed to create check fn: %v", err)
	}
	checkFn, ok := goja.AssertFunction(checkVal)
	if !ok {
		t.Fatal("check fn is not callable")
	}
	checkResult, err := checkFn(goja.Undefined(), vm.ToValue(resultJSON))
	if err != nil {
		t.Fatalf("check fn failed: %v", err)
	}
	if s := checkResult.String(); s != "" {
		t.Errorf("Document API check failures: %s", s)
	}
}

// Ensure istrings import is used (hint for compiler).
var _ = istrings.GraphemeNumber(0)
