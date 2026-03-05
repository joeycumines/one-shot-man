package termmux

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// testRequire sets up a goja.Runtime with the osm:termmux module registered
// (using nil TerminalOpsProvider so it falls back to os.Stdin/os.Stdout).
func testRequire(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	runtime := goja.New()
	registry := require.NewRegistry()

	registry.RegisterNativeModule("osm:termmux", Require(context.Background(), nil, nil))
	registry.Enable(runtime)

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}
	obj := v.(*goja.Object)
	return runtime, obj
}

func TestModule_Constants(t *testing.T) {
	_, exports := testRequire(t)

	tests := []struct {
		name string
		want interface{}
	}{
		{"EXIT_TOGGLE", "toggle"},
		{"EXIT_CHILD_EXIT", "childExit"},
		{"EXIT_CONTEXT", "context"},
		{"EXIT_ERROR", "error"},
		{"SIDE_OSM", "osm"},
		{"SIDE_CLAUDE", "claude"},
		{"DEFAULT_TOGGLE_KEY", int64(0x1D)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := exports.Get(tt.name).Export()
			if got != tt.want {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestModule_NewMux_ReturnsObject(t *testing.T) {
	runtime, _ := testRequire(t)

	// newMux() with no args should return an object with all expected methods.
	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		var methods = ['attach', 'detach', 'hasChild', 'switchTo', 'activeSide',
			'setStatus', 'setToggleKey', 'setStatusEnabled', 'setResizeFunc', 'screenshot'];
		var missing = [];
		for (var i = 0; i < methods.length; i++) {
			if (typeof m[methods[i]] !== 'function') missing.push(methods[i]);
		}
		missing;
	`)
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}

	missing := v.Export()
	if arr, ok := missing.([]interface{}); ok && len(arr) > 0 {
		t.Errorf("missing methods on mux: %v", arr)
	}
}

func TestModule_NewMux_WithOptions(t *testing.T) {
	runtime, _ := testRequire(t)

	// Should accept options without error.
	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux({
			toggleKey: 0x1C,
			statusEnabled: false,
			initialStatus: 'test'
		});
	`)
	if err != nil {
		t.Fatalf("newMux with options: %v", err)
	}
}

func TestModule_NewMux_HasChild_InitialFalse(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.hasChild();
	`)
	if err != nil {
		t.Fatalf("hasChild: %v", err)
	}
	if v.ToBoolean() {
		t.Error("hasChild() should be false before attach")
	}
}

func TestModule_NewMux_ActiveSide_InitialOsm(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.activeSide();
	`)
	if err != nil {
		t.Fatalf("activeSide: %v", err)
	}
	if v.String() != "osm" {
		t.Errorf("activeSide = %q, want %q", v.String(), "osm")
	}
}

func TestModule_NewMux_DetachIdempotent(t *testing.T) {
	runtime, _ := testRequire(t)

	// detach() on fresh mux should not panic.
	_, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.detach();
	`)
	if err != nil {
		t.Fatalf("detach on fresh mux should be idempotent: %v", err)
	}
}

func TestModule_NewMux_ScreenshotEmpty(t *testing.T) {
	runtime, _ := testRequire(t)

	v, err := runtime.RunString(`
		var m = require('osm:termmux').newMux();
		m.screenshot();
	`)
	if err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	if v.String() != "" {
		t.Errorf("screenshot = %q, want empty string", v.String())
	}
}

func TestExitReasonString(t *testing.T) {
	tests := []struct {
		input parent.ExitReason
		want  string
	}{
		{parent.ExitToggle, "toggle"},
		{parent.ExitChildExit, "childExit"},
		{parent.ExitContext, "context"},
		{parent.ExitError, "error"},
		{parent.ExitReason(99), "unknown(99)"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := exitReasonString(tt.input)
			if got != tt.want {
				t.Errorf("exitReasonString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveChild_NilError(t *testing.T) {
	_, err := resolveChild(nil)
	if err == nil {
		t.Error("resolveChild(nil) should return error")
	}
}

func TestResolveChild_InvalidTypeError(t *testing.T) {
	_, err := resolveChild("not a handle")
	if err == nil {
		t.Error("resolveChild(string) should return error")
	}
}
