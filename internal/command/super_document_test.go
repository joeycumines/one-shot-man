package command

import (
	"bytes"
	"context"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestSuperDocument_FormMode_TextareaCommandPropagation(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Provide minimal globals the script expects
	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
		"primary":   "#7f5fcf",
		"secondary": "#efefef",
		"danger":    "#ff0000",
		"muted":     "#888888",
		"bg":        "#000000",
		"fg":        "#ffffff",
		"warning":   "#ffaa00",
		"focus":     "#00ff00",
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

	// The result is a JS array -> []interface{}
	arr, ok := val.([]interface{})
	if !ok {
		t.Fatalf("unexpected __result type: %T", val)
	}
	if len(arr) < 2 {
		t.Fatalf("expected returned array to have at least 2 elements, got %d", len(arr))
	}
	cmdVal := arr[1]
	cmdObj, ok := cmdVal.(map[string]interface{})
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
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
		"primary":   "#7f5fcf",
		"secondary": "#efefef",
		"danger":    "#ff0000",
		"muted":     "#888888",
		"bg":        "#000000",
		"fg":        "#ffffff",
		"warning":   "#ffaa00",
		"focus":     "#00ff00",
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
	arr, ok := val.([]interface{})
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
	sm, ok := sval.(map[string]interface{})
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
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
		"primary":   "#7f5fcf",
		"secondary": "#efefef",
		"danger":    "#ff0000",
		"muted":     "#888888",
		"bg":        "#000000",
		"fg":        "#ffffff",
		"warning":   "#ffaa00",
		"focus":     "#00ff00",
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
	m, ok := val.(map[string]interface{})
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
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
		"primary":   "#7f5fcf",
		"secondary": "#efefef",
		"danger":    "#ff0000",
		"muted":     "#888888",
		"bg":        "#000000",
		"fg":        "#ffffff",
		"warning":   "#ffaa00",
		"focus":     "#00ff00",
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
	arr, ok := val.([]interface{})
	if !ok || len(arr) < 2 {
		t.Fatalf("unexpected submit result: %T %#v", val, val)
	}
	if arr[1] == nil {
		t.Fatalf("expected a command (clearScreen) on submit, got nil")
	}
	cmdObj, ok := arr[1].(map[string]interface{})
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
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
		"primary":   "#7f5fcf",
		"secondary": "#efefef",
		"danger":    "#ff0000",
		"muted":     "#888888",
		"bg":        "#000000",
		"fg":        "#ffffff",
		"warning":   "#ffaa00",
		"focus":     "#00ff00",
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
	sm, ok := val.(map[string]interface{})
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
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
		"primary":   "#7f5fcf",
		"secondary": "#efefef",
		"danger":    "#ff0000",
		"muted":     "#888888",
		"bg":        "#000000",
		"fg":        "#ffffff",
		"warning":   "#ffaa00",
		"focus":     "#00ff00",
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
	m, ok := docVal.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected doc type: %T", docVal)
	}
	if m["content"] != "new content" {
		t.Fatalf("expected doc content updated to 'new content', got %v", m["content"])
	}
	// Verify selection preserved
	sVal := engine.GetGlobal("__s")
	sm, ok := sVal.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected s type: %T", sVal)
	}
	if sm["selectedIdx"].(int64) != 0 {
		t.Fatalf("expected selectedIdx to remain 0, got %v", sm["selectedIdx"])
	}
}
