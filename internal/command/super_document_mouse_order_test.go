package command

import (
	"bytes"
	"context"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestHandleMouse_CallsSetViewportContextBeforeHandleClickAtScreenCoords ensures
// that the JS `handleMouse` function calls `setViewportContext` before
// calling `handleClickAtScreenCoords` on the textarea object. This is a
// deterministic, single-threaded unit test implemented by injecting spy
// functions into a synthetic `s` object and invoking the real `handleMouse`.
func TestHandleMouse_CallsSetViewportContextBeforeHandleClickAtScreenCoords(t *testing.T) {
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

	// Test script: install spies and call handleMouse(msg, s)
	testScript := `
	var calls = [];
	var spySetViewportContext = function(cfg) { calls.push("setViewportContext"); };
	var spyHandleClickAtScreenCoords = function(x,y,th) { calls.push("handleClickAtScreenCoords"); return { hit: true, row: 123, col: 4 }; };

	var s = {
		mode: MODE_INPUT,
		inputFocus: FOCUS_CONTENT,
		textareaBounds: { contentTop: 0, contentLeft: 0, fieldWidth: 80 },
		inputVp: { yOffset: function(){ return 0; }, height: function(){ return 10; } },
		contentTextarea: {
			visualLineCount: function(){ return 2; },
			setViewportContext: spySetViewportContext,
			handleClickAtScreenCoords: spyHandleClickAtScreenCoords,
			focus: function() { calls.push('focus'); },
			blur: function() { calls.push('blur'); }
		},
		titleHeight: 1,
		width: 80
	};

	var msg = { type: "Mouse", action: "press", button: "left", x: 1, y: 2 };

	// Invoke the real, embedded handleMouse implementation
	handleMouse(msg, s);

	// Expose the call order for the Go test to inspect deterministically
	__calls_json = JSON.stringify(calls);
	`
	testScriptObj := engine.LoadScriptFromString("call-order-test", testScript)
	if err := engine.ExecuteScript(testScriptObj); err != nil {
		t.Fatalf("call-order test script execution failed: %v", err)
	}

	val := engine.GetGlobal("__calls_json")
	if val == nil {
		t.Fatalf("expected __calls_json to be set by test script")
	}

	callsJSON, ok := val.(string)
	if !ok {
		t.Fatalf("expected __calls_json to be a string, got %T", val)
	}

	expected := `["setViewportContext","handleClickAtScreenCoords"]`
	if callsJSON != expected {
		t.Fatalf("unexpected call order: got %s, want %s", callsJSON, expected)
	}
}
