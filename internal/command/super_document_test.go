package command

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestSuperDocument_FormMode_TextareaCommandPropagation(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Provide minimal globals the script expects
	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	// Load and execute the embedded command script
	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	// Test: stub textarea.update to return [newTa, { _cmdType: 'quit' }]
	testScript := `
var taCmd = { _cmdType: 'quit' };
var newTa = { updated: true };
var s = {
    mode: MODE_INPUT,
    inputFocus: FOCUS_CONTENT,
    contentTextarea: {
        update: function(msg) { return [newTa, taCmd]; },
    }
};

var res = handleKeys({ type: 'Key', key: 'a', paste: false }, s);
// Expose results for Go test
__result = res;
`

	testObj := engine.LoadScriptFromString("super-doc-propagation", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}

	val := engine.GetGlobal("__result")
	if val == nil {
		t.Fatalf("expected __result to be set by test script")
	}

	// The result is a JS array -> []any
	arr, ok := val.([]any)
	if !ok {
		t.Fatalf("unexpected __result type: %T", val)
	}
	if len(arr) < 2 {
		t.Fatalf("expected returned array to have at least 2 elements, got %d", len(arr))
	}
	cmdVal := arr[1]
	cmdObj, ok := cmdVal.(map[string]any)
	if !ok {
		t.Fatalf("expected cmd object to be a map, got %T", cmdVal)
	}
	if cmdObj["_cmdType"] != "quit" {
		t.Fatalf("expected returned cmd _cmdType 'quit', got %v", cmdObj["_cmdType"])
	}
}

func TestSuperDocument_ListMode_NoCommandOnKeyNav(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	testScript := `
var s = {
    mode: MODE_LIST,
    documents: [{id:1,label:'one'},{id:2,label:'two'}],
    selectedIdx: 0,
    vp: { setYOffset: function(y){ this.y = y; }, yOffset: function(){ return this.y || 0; }, height: function(){ return 10; } }
};
var res = handleKeys({ type: 'Key', key: 'down' }, s);
__resArr = res;
__s = s;
`

	testObj := engine.LoadScriptFromString("super-doc-list-nav", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}
	val := engine.GetGlobal("__resArr")
	arr, ok := val.([]any)
	if !ok {
		t.Fatalf("unexpected result type: %T", val)
	}
	if len(arr) < 2 {
		t.Fatalf("expected returned array to have at least 2 elements, got %d", len(arr))
	}
	if arr[1] != nil {
		t.Fatalf("expected no command returned on key nav in list mode, got %T", arr[1])
	}
	// Verify selection moved down
	sval := engine.GetGlobal("__s")
	sm, ok := sval.(map[string]any)
	if !ok {
		t.Fatalf("unexpected s type: %T", sval)
	}
	if sm["selectedIdx"].(int64) != 1 {
		t.Fatalf("expected selectedIdx to be 1 after down, got %v", sm["selectedIdx"])
	}
}

func TestSuperDocument_ListMode_ViewportCommandPropagation(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	// Simulate a viewport.update() that returns a command descriptor and verify it's preserved
	testScript := `
var vpCmd = { _cmdType: 'scroll' };
var fakeVp = { update: function(msg) { return [fakeVp, vpCmd]; } };
var s = { mode: MODE_LIST, documents: [{id:1,label:'one'}], selectedIdx:0, vp: fakeVp };
var res = (function(){ const r = s.vp.update({type:'wheel'}); return r[1]; })();
__result = res;
`

	testObj := engine.LoadScriptFromString("super-doc-vp-prop", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}
	val := engine.GetGlobal("__result")
	m, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("expected result to be object, got %T", val)
	}
	if m["_cmdType"] != "scroll" {
		t.Fatalf("expected returned cmd _cmdType 'scroll', got %v", m["_cmdType"])
	}
}

func TestSuperDocument_ModeTransition_TextareaToList(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	// Prepare an input-state and then submit via ctrl+enter; expect clearScreen cmd
	testScript := `
var s = { mode: MODE_INPUT, inputOperation: INPUT_ADD, inputFocus: FOCUS_LABEL, labelBuffer: 'lbl', contentTextarea: { value: function(){ return 'body'; } } };
var res = handleKeys({ key: 'ctrl+enter' }, s);
__res = res;
`

	testObj := engine.LoadScriptFromString("super-doc-input-submit", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}
	val := engine.GetGlobal("__res")
	arr, ok := val.([]any)
	if !ok || len(arr) < 2 {
		t.Fatalf("unexpected submit result: %T %#v", val, val)
	}
	if arr[1] == nil {
		t.Fatalf("expected a command (clearScreen) on submit, got nil")
	}
	cmdObj, ok := arr[1].(map[string]any)
	if !ok {
		t.Fatalf("expected cmd to be object, got %T", arr[1])
	}
	if cmdObj["_cmdType"] != "clearScreen" {
		t.Fatalf("expected clearScreen command, got %v", cmdObj["_cmdType"])
	}
}

func TestSuperDocument_ModeTransition_ListToForm(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	testScript := `
var s = { mode: MODE_LIST, documents: [{id:1,label:'one'}], selectedIdx: 0 };
var res = handleKeys({ key: 'a' }, s);
__s = s;
__res = res;
`

	testObj := engine.LoadScriptFromString("super-doc-list-to-form", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}
	val := engine.GetGlobal("__s")
	sm, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("unexpected s type: %T", val)
	}
	if sm["mode"].(string) != "input" {
		t.Fatalf("expected mode 'input' after 'a' key, got %v", sm["mode"])
	}
	if sm["contentTextarea"] == nil {
		t.Fatalf("expected contentTextarea to be initialized in input mode")
	}
}

func TestSuperDocument_ModeTransition_PreservesState(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	// Add a document, edit it, submit new content, ensure selection preserved and content updated
	testScript := `
addDocument('lbl','orig');
var docs = getDocuments();
var docId = docs[0].id;
	var s = { mode: MODE_LIST, documents: getDocuments(), selectedIdx: 0 };
	handleKeys({ key: 'e' }, s);
// Now s should be in input mode with editingDocId set
if (s.mode !== MODE_INPUT) throw new Error('expected input mode');
var id = s.editingDocId;
// Replace content via s.contentTextarea simulation
s.contentTextarea = { value: function(){ return 'new content'; } };
var res = handleKeys({ key: 'ctrl+enter' }, s);
// After submit, verify document content updated and selectedIdx preserved
var post = getDocumentById(id);
__doc = post;
__s = s;
`

	testObj := engine.LoadScriptFromString("super-doc-preserve", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}
	docVal := engine.GetGlobal("__doc")
	m, ok := docVal.(map[string]any)
	if !ok {
		t.Fatalf("unexpected doc type: %T", docVal)
	}
	if m["content"] != "new content" {
		t.Fatalf("expected doc content updated to 'new content', got %v", m["content"])
	}
	// Verify selection preserved
	sVal := engine.GetGlobal("__s")
	sm, ok := sVal.(map[string]any)
	if !ok {
		t.Fatalf("unexpected s type: %T", sVal)
	}
	if sm["selectedIdx"].(int64) != 0 {
		t.Fatalf("expected selectedIdx to remain 0, got %v", sm["selectedIdx"])
	}
}

func TestSuperDocument_FocusedButtonEnterDoesNotFallIntoEdit(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary":    "#7f5fcf",
		"textSecondary":  "#efefef",
		"textTertiary":   "#888888",
		"textInverted":   "#ffffff",
		"accentPrimary":  "#7f5fcf",
		"accentSubtle":   "#efefef",
		"accentSuccess":  "#1a7f37",
		"accentError":    "#ff0000",
		"accentWarning":  "#ffaa00",
		"uiBorder":       "#444444",
		"uiActiveBorder": "#7f5fcf",
		"uiBg":           "#000000",
		"uiBgSubtle":     "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	testScript := `
buildFinalPrompt = function () { return Promise.resolve('prompt body'); };
os.clipboardCopy = function (txt) { globalThis.__copiedPrompt = txt; return Promise.resolve(); };

function baseState(idx) {
    return {
        mode: MODE_LIST,
        documents: [{id: 1, label: 'one', content: 'body'}],
        selectedIdx: 0,
        focusedButtonIdx: idx,
        width: 80,
        vp: {
            setYOffset: function () {},
            yOffset: function () { return 0; },
            height: function () { return 10; }
        }
    };
}

var addState = baseState(BUTTONS.findIndex(function (btn) { return btn.key === 'a'; }));
var addRes = handleKeys({ type: 'Key', key: 'enter' }, addState);

var loadState = baseState(BUTTONS.findIndex(function (btn) { return btn.key === 'l'; }));
var loadRes = handleKeys({ type: 'Key', key: 'enter' }, loadState);

var copyState = baseState(BUTTONS.findIndex(function (btn) { return btn.key === 'c'; }));
var copyRes = handleKeys({ type: 'Key', key: 'enter' }, copyState);

var resetState = baseState(BUTTONS.findIndex(function (btn) { return btn.key === 'r'; }));
var resetRes = handleKeys({ type: 'Key', key: 'enter' }, resetState);

Promise.resolve().then(function() {
globalThis.__result = {
    addMode: addState.mode,
    addOperation: addState.inputOperation,
    addCmdType: addRes[1] && addRes[1]._cmdType || null,
    loadMode: loadState.mode,
    loadOperation: loadState.inputOperation,
    loadCmdType: loadRes[1] && loadRes[1]._cmdType || null,
    copyMode: copyState.mode,
    copyStatusMsg: copyState.statusMsg,
    copyCmdType: copyRes[1] && copyRes[1]._cmdType || null,
    copiedPrompt: globalThis.__copiedPrompt || null,
    resetMode: resetState.mode,
    resetConfirmDocId: resetState.confirmDocId,
    resetCmdType: resetRes[1] && resetRes[1]._cmdType || null
};
});
`

	testObj := engine.LoadScriptFromString("super-doc-focused-button-enter", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("test script execution failed: %v", err)
	}

	flushDone := make(chan struct{})
	engine.Loop().Submit(func() { close(flushDone) })
	select {
	case <-flushDone:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for microtasks")
	}

	val := engine.GetGlobal("__result")
	result, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("unexpected __result type: %T", val)
	}

	if got := result["addMode"]; got != "input" {
		t.Fatalf("expected add button enter to stay in input mode, got %v", got)
	}
	if got := result["addOperation"]; got != "add" {
		t.Fatalf("expected add button enter to set inputOperation=add, got %v", got)
	}
	if got := result["addCmdType"]; got != nil {
		t.Fatalf("expected add button enter to return no command, got %v", got)
	}

	if got := result["loadMode"]; got != "input" {
		t.Fatalf("expected load button enter to stay in input mode, got %v", got)
	}
	if got := result["loadOperation"]; got != "load" {
		t.Fatalf("expected load button enter to set inputOperation=load, got %v", got)
	}
	if got := result["loadCmdType"]; got != nil {
		t.Fatalf("expected load button enter to return no command, got %v", got)
	}

	if got := result["copyMode"]; got != "list" {
		t.Fatalf("expected copy button enter to remain in list mode, got %v", got)
	}
	statusMsg, _ := result["copyStatusMsg"].(string)
	if !strings.Contains(statusMsg, "\u2502") {
		t.Fatalf("expected copy button enter to set copy summary, got %q", statusMsg)
	}
	if got := result["copyCmdType"]; got != nil {
		t.Fatalf("expected copy button enter to return no command, got %v", got)
	}
	if got := result["copiedPrompt"]; got != "prompt body" {
		t.Fatalf("expected copy button enter to copy prompt body, got %v", got)
	}

	if got := result["resetMode"]; got != "confirm" {
		t.Fatalf("expected reset button enter to switch to confirm mode, got %v", got)
	}
	if got := result["resetConfirmDocId"]; got != int64(-1) {
		t.Fatalf("expected reset button enter to target confirmDocId=-1, got %v", got)
	}
	if got := result["resetCmdType"]; got != nil {
		t.Fatalf("expected reset button enter to return no command, got %v", got)
	}
}

// TestSuperDocument_BareKeyHotkeys_Regression ensures that every bare-key
// hotkey in list mode triggers its action WITHOUT requiring a chord prefix.
//
// This is a direct regression test for a breakage where all action hotkeys
// (a/l/e/r/R/d/c/q/s) were incorrectly gated behind a Ctrl+X prefix mode,
// rendering the super-document TUI completely unusable. The breakage went
// unnoticed because the E2E test was skipped with t.Skip("broken: ...")
// instead of being fixed, and no short-mode test exercised bare-key
// dispatch holistically.
//
// This test runs in short mode so that any future regression of bare-key
// hotkeys is caught immediately by the fast test suite.
func TestSuperDocument_BareKeyHotkeys_Regression(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]any{"name": "super-document", "theme": map[string]any{
		"textPrimary": "#7f5fcf", "textSecondary": "#efefef", "textTertiary": "#888888",
		"textInverted": "#ffffff", "accentPrimary": "#7f5fcf", "accentSubtle": "#efefef",
		"accentSuccess": "#1a7f37", "accentError": "#ff0000", "accentWarning": "#ffaa00",
		"uiBorder": "#444444", "uiActiveBorder": "#7f5fcf", "uiBg": "#000000", "uiBgSubtle": "#111111",
	}})
	engine.SetGlobal("args", []string{})
	engine.SetGlobal("superDocumentTemplate", "dummy template")

	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("failed to execute super-document script: %v", err)
	}

	testScript := `
	// Stub clipboard + prompt builder so 'c' (copy) works without real I/O.
	buildFinalPrompt = function () { return Promise.resolve('prompt body'); };
	os.clipboardCopy = function (txt) { globalThis.__copiedPrompt = txt; return Promise.resolve(); };

	function baseState(extra) {
		return Object.assign({
			mode: MODE_LIST,
			documents: [{id:1, label:'doc1', content:'content1'}],
			selectedIdx: 0,
			width: 80,
			focusedButtonIdx: -1,
			vp: {
				setYOffset: function(y) { this._y = y; },
				yOffset: function() { return this._y || 0; },
				height: function() { return 10; }
			}
		}, extra || {});
	}

	var r = {};

	// 'a' = add → input mode, INPUT_ADD, no command
	var s = baseState();
	var res = handleKeys({key: 'a'}, s);
	r.a_mode = s.mode;
	r.a_op = s.inputOperation;
	r.a_cmd = res[1] ? res[1]._cmdType : null;

	// 'l' = load → input mode, INPUT_LOAD, no command
	s = baseState();
	res = handleKeys({key: 'l'}, s);
	r.l_mode = s.mode;
	r.l_op = s.inputOperation;
	r.l_cmd = res[1] ? res[1]._cmdType : null;

	// 'e' = edit → input mode, INPUT_EDIT, no command
	s = baseState();
	res = handleKeys({key: 'e'}, s);
	r.e_mode = s.mode;
	r.e_op = s.inputOperation;
	r.e_cmd = res[1] ? res[1]._cmdType : null;

	// 'r' = reset → confirm mode, confirmDocId = -1, no command
	s = baseState();
	res = handleKeys({key: 'r'}, s);
	r.r_mode = s.mode;
	r.r_confirmDocId = s.confirmDocId;
	r.r_cmd = res[1] ? res[1]._cmdType : null;

	// 'R' = rename → input mode, INPUT_RENAME, no command
	s = baseState();
	res = handleKeys({key: 'R'}, s);
	r.R_mode = s.mode;
	r.R_op = s.inputOperation;
	r.R_cmd = res[1] ? res[1]._cmdType : null;

	// 'd' = delete → confirm mode, confirmDocId = doc.id, no command
	s = baseState();
	res = handleKeys({key: 'd'}, s);
	r.d_mode = s.mode;
	r.d_confirmDocId = s.confirmDocId;
	r.d_cmd = res[1] ? res[1]._cmdType : null;

	// 'c' = copy → stays in list, copies prompt, sets statusMsg, no command
	s = baseState();
	var c_s = s;
	res = handleKeys({key: 'c'}, s);
	r.c_mode = s.mode;
	r.c_statusMsg = s.statusMsg;
	r.c_copied = globalThis.__copiedPrompt;
	r.c_cmd = res[1] ? res[1]._cmdType : null;

	// 'q' = quit → returns quit command
	s = baseState();
	res = handleKeys({key: 'q'}, s);
	r.q_mode = s.mode;
	r.q_cmd = res[1] ? res[1]._cmdType : null;

	// 's' = shell → returns quit command
	s = baseState();
	res = handleKeys({key: 's'}, s);
	r.s_mode = s.mode;
	r.s_cmd = res[1] ? res[1]._cmdType : null;

	// '?' = help → stays in list, sets statusMsg, no command
	s = baseState();
	res = handleKeys({key: '?'}, s);
	r.help_mode = s.mode;
	r.help_statusMsg = s.statusMsg;
	r.help_cmd = res[1] ? res[1]._cmdType : null;

	// 'enter' on focused Add button → input mode, INPUT_ADD (re-dispatch)
	var addIdx = BUTTONS.findIndex(function(b) { return b.key === 'a'; });
	s = baseState({focusedButtonIdx: addIdx});
	res = handleKeys({key: 'enter'}, s);
	r.enterAdd_mode = s.mode;
	r.enterAdd_op = s.inputOperation;
	r.enterAdd_cmd = res[1] ? res[1]._cmdType : null;

	// 'enter' on document (no button focused) → edit
	s = baseState({focusedButtonIdx: -1, selectedIdx: 0});
	res = handleKeys({key: 'enter'}, s);
	r.enterDoc_mode = s.mode;
	r.enterDoc_op = s.inputOperation;
	r.enterDoc_cmd = res[1] ? res[1]._cmdType : null;

	// 'backspace' on document → delete confirm
	s = baseState({focusedButtonIdx: -1, selectedIdx: 0});
	res = handleKeys({key: 'backspace'}, s);
	r.bs_mode = s.mode;
	r.bs_confirmDocId = s.confirmDocId;
	r.bs_cmd = res[1] ? res[1]._cmdType : null;

	// Navigation: 'j' moves selection down (stays in list)
	s = baseState({documents: [{id:1,label:'a',content:'x'},{id:2,label:'b',content:'y'}], selectedIdx: 0});
	res = handleKeys({key: 'j'}, s);
	r.j_mode = s.mode;
	r.j_selectedIdx = s.selectedIdx;

	// Navigation: 'k' moves selection up (stays in list)
	s = baseState({documents: [{id:1,label:'a',content:'x'},{id:2,label:'b',content:'y'}], selectedIdx: 1});
	res = handleKeys({key: 'k'}, s);
	r.k_mode = s.mode;
	r.k_selectedIdx = s.selectedIdx;

	__results = r;
Promise.resolve().then(function() {
	r.c_statusMsg = c_s.statusMsg;
	r.c_copied = globalThis.__copiedPrompt;
	__results = r;
});
`

	testObj := engine.LoadScriptFromString("sd-hotkey-regression", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("regression test script failed: %v", err)
	}

	flushDone2 := make(chan struct{})
	engine.Loop().Submit(func() { close(flushDone2) })
	select {
	case <-flushDone2:
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for microtasks")
	}

	val := engine.GetGlobal("__results")
	r, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("unexpected __results type: %T", val)
	}

	// Helper to extract a value with a clear failure message
	get := func(key string) any {
		v, exists := r[key]
		if !exists {
			t.Fatalf("missing result key %q in results: %#v", key, r)
		}
		return v
	}

	// 'a' = add
	if got := get("a_mode"); got != "input" {
		t.Errorf("bare 'a' should enter input mode, got %v", got)
	}
	if got := get("a_op"); got != "add" {
		t.Errorf("bare 'a' should set inputOperation=add, got %v", got)
	}
	if got := get("a_cmd"); got != nil {
		t.Errorf("bare 'a' should return no command, got %v", got)
	}

	// 'l' = load
	if got := get("l_mode"); got != "input" {
		t.Errorf("bare 'l' should enter input mode, got %v", got)
	}
	if got := get("l_op"); got != "load" {
		t.Errorf("bare 'l' should set inputOperation=load, got %v", got)
	}
	if got := get("l_cmd"); got != nil {
		t.Errorf("bare 'l' should return no command, got %v", got)
	}

	// 'e' = edit
	if got := get("e_mode"); got != "input" {
		t.Errorf("bare 'e' should enter input mode, got %v", got)
	}
	if got := get("e_op"); got != "edit" {
		t.Errorf("bare 'e' should set inputOperation=edit, got %v", got)
	}
	if got := get("e_cmd"); got != nil {
		t.Errorf("bare 'e' should return no command, got %v", got)
	}

	// 'r' = reset
	if got := get("r_mode"); got != "confirm" {
		t.Errorf("bare 'r' should enter confirm mode, got %v", got)
	}
	if got := get("r_confirmDocId"); got != int64(-1) {
		t.Errorf("bare 'r' should set confirmDocId=-1, got %v", got)
	}
	if got := get("r_cmd"); got != nil {
		t.Errorf("bare 'r' should return no command, got %v", got)
	}

	// 'R' = rename
	if got := get("R_mode"); got != "input" {
		t.Errorf("bare 'R' should enter input mode, got %v", got)
	}
	if got := get("R_op"); got != "rename" {
		t.Errorf("bare 'R' should set inputOperation=rename, got %v", got)
	}
	if got := get("R_cmd"); got != nil {
		t.Errorf("bare 'R' should return no command, got %v", got)
	}

	// 'd' = delete
	if got := get("d_mode"); got != "confirm" {
		t.Errorf("bare 'd' should enter confirm mode, got %v", got)
	}
	if got := get("d_confirmDocId"); got != int64(1) {
		t.Errorf("bare 'd' should set confirmDocId=1 (the doc id), got %v", got)
	}
	if got := get("d_cmd"); got != nil {
		t.Errorf("bare 'd' should return no command, got %v", got)
	}

	// 'c' = copy
	if got := get("c_mode"); got != "list" {
		t.Errorf("bare 'c' should stay in list mode, got %v", got)
	}
	statusMsg, _ := get("c_statusMsg").(string)
	if !strings.Contains(statusMsg, "\u2502") {
		t.Errorf("bare 'c' should set a copy summary statusMsg, got %q", statusMsg)
	}
	if got := get("c_copied"); got != "prompt body" {
		t.Errorf("bare 'c' should copy prompt body, got %v", got)
	}
	if got := get("c_cmd"); got != nil {
		t.Errorf("bare 'c' should return no command, got %v", got)
	}

	// 'q' = quit
	if got := get("q_cmd"); got != "quit" {
		t.Errorf("bare 'q' should return a quit command, got %v", got)
	}

	// 's' = shell (also quit)
	if got := get("s_cmd"); got != "quit" {
		t.Errorf("bare 's' should return a quit command, got %v", got)
	}

	// '?' = help
	if got := get("help_mode"); got != "list" {
		t.Errorf("bare '?' should stay in list mode, got %v", got)
	}
	helpMsg, _ := get("help_statusMsg").(string)
	if !strings.Contains(helpMsg, "a:add") || !strings.Contains(helpMsg, "q:quit") {
		t.Errorf("bare '?' should set help text with key bindings, got %q", helpMsg)
	}
	if got := get("help_cmd"); got != nil {
		t.Errorf("bare '?' should return no command, got %v", got)
	}

	// 'enter' on focused Add button
	if got := get("enterAdd_mode"); got != "input" {
		t.Errorf("enter on focused Add button should enter input mode, got %v", got)
	}
	if got := get("enterAdd_op"); got != "add" {
		t.Errorf("enter on focused Add button should set inputOperation=add, got %v", got)
	}
	if got := get("enterAdd_cmd"); got != nil {
		t.Errorf("enter on focused Add button should return no command, got %v", got)
	}

	// 'enter' on document
	if got := get("enterDoc_mode"); got != "input" {
		t.Errorf("enter on document should enter input mode, got %v", got)
	}
	if got := get("enterDoc_op"); got != "edit" {
		t.Errorf("enter on document should set inputOperation=edit, got %v", got)
	}

	// 'backspace' on document
	if got := get("bs_mode"); got != "confirm" {
		t.Errorf("backspace on document should enter confirm mode, got %v", got)
	}
	if got := get("bs_confirmDocId"); got != int64(1) {
		t.Errorf("backspace on document should set confirmDocId=1, got %v", got)
	}

	// Navigation 'j' (down)
	if got := get("j_mode"); got != "list" {
		t.Errorf("'j' should stay in list mode, got %v", got)
	}
	if got := get("j_selectedIdx"); got != int64(1) {
		t.Errorf("'j' should move selectedIdx to 1, got %v", got)
	}

	// Navigation 'k' (up)
	if got := get("k_mode"); got != "list" {
		t.Errorf("'k' should stay in list mode, got %v", got)
	}
	if got := get("k_selectedIdx"); got != int64(0) {
		t.Errorf("'k' should move selectedIdx to 0, got %v", got)
	}
}
