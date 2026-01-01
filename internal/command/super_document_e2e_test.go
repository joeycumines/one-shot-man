package command

import (
	"bytes"
	"context"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestSuperDocumentE2E_FullWorkflow simulates a user going: List -> Add(Form) -> Submit -> List -> Edit -> Submit
// and verifies that commands (clearScreen on submit) are returned and document content is persisted.
func TestSuperDocumentE2E_FullWorkflow(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("super-document-e2e", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	engine.SetGlobal("config", map[string]interface{}{"name": "super-document", "theme": map[string]interface{}{
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
__res = (function(){
    // Step 1: Start in list, press 'a' to enter add form
    var s = { mode: MODE_LIST, documents: getDocuments(), selectedIdx: -1 };
    handleKeys({ key: 'a' }, s);
    if (s.mode !== MODE_INPUT) throw new Error('expected input mode after a');

    // Step 2: Fill form and submit
    s.labelBuffer = 'doc1';
    s.contentTextarea = { value: function(){ return 'content1'; } };
    var r = handleKeys({ key: 'ctrl+enter' }, s);
    if (!r[1] || r[1]._cmdType !== 'clearScreen') throw new Error('expected clearScreen on submit');

    // Step 3: Verify we are back in list with one document
    if (s.mode !== MODE_LIST) throw new Error('expected mode list after submit');
    var docs = getDocuments();
    if (docs.length !== 1) throw new Error('expected 1 doc after submit');

    // Step 4: Edit the document
    s = { mode: MODE_LIST, documents: getDocuments(), selectedIdx: 0 };
    handleKeys({ key: 'e' }, s);
    if (s.mode !== MODE_INPUT) throw new Error('expected input mode on edit');
    s.contentTextarea = { value: function(){ return 'edited'; } };
    var r2 = handleKeys({ key: 'ctrl+enter' }, s);
    if (!r2[1] || r2[1]._cmdType !== 'clearScreen') throw new Error('expected clearScreen on edit submit');

    var d = getDocuments()[0];
    if (d.content !== 'edited') throw new Error('expected edited content persisted');

    return { success: true };
})();
`

	testObj := engine.LoadScriptFromString("super-document-e2e", testScript)
	if err := engine.ExecuteScript(testObj); err != nil {
		t.Fatalf("E2E script failed: %v", err)
	}
	res := engine.GetGlobal("__res")
	if res == nil {
		t.Fatalf("E2E returned nil")
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected E2E return type: %T", res)
	}
	if success, _ := m["success"].(bool); !success {
		t.Fatalf("E2E indicated failure: %#v", m)
	}
}
