package termmux

import (
	"context"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/builtin/bubbletea"
)

// TestNewKeymap_JSBinding verifies the termmux.newKeymap export: options are
// honored, handleKey runs the state machine, and mutations work from JS.
func TestNewKeymap_JSBinding(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		var km = tm.newKeymap({
			prefix: "ctrl+a",
			passthroughKey: "ctrl+]",
			keys: { "f1": "help" },
			chords: { "c": "new-window", "x": "close" }
		});
		var out = [];
		out.push(km.prefix() === "ctrl+a");
		out.push(km.passthroughKey() === "ctrl+]");
		// direct key
		var r = km.handleKey("f1");
		out.push(r.handled === true && r.action === "help" && r.armed === false);
		// passthrough key
		r = km.handleKey("ctrl+]");
		out.push(r.handled === true && r.action === "passthrough");
		// prefix arms
		r = km.handleKey("ctrl+a");
		out.push(r.handled === true && r.action === "" && r.armed === true);
		out.push(km.armed() === true);
		// chord dispatches
		r = km.handleKey("c");
		out.push(r.handled === true && r.action === "new-window" && r.armed === false);
		// prefix + esc cancels
		km.handleKey("ctrl+a");
		r = km.handleKey("esc");
		out.push(r.handled === true && r.action === "" && r.armed === false);
		// prefix + unbound passes through
		km.handleKey("ctrl+a");
		r = km.handleKey("q");
		out.push(r.handled === false && r.armed === false);
		// unbound normal key passes through
		r = km.handleKey("z");
		out.push(r.handled === false);
		// mutations
		km.bind("f2", "help2");
		r = km.handleKey("f2");
		out.push(r.handled && r.action === "help2");
		km.unbind("f2");
		out.push(km.handleKey("f2").handled === false);
		km.bindChord("s", "save");
		km.handleKey("ctrl+a");
		out.push(km.handleKey("s").action === "save");
		km.unbindChord("s");
		km.handleKey("ctrl+a");
		out.push(km.handleKey("s").handled === false);
		km.setPrefix("ctrl+x");
		out.push(km.prefix() === "ctrl+x");
		km.handleKey("ctrl+x");
		out.push(km.handleKey("c").action === "new-window");
		km.setPrefix("");
		out.push(km.handleKey("ctrl+x").handled === false);
		km.reset();
		// bindings + listKeys
		var b = km.bindings();
		out.push(Array.isArray(b) && b.length >= 3);
		var lk = km.listKeys();
		out.push(typeof lk === "string" && lk.indexOf("new-window") >= 0);
		var bad = out.findIndex(function(x){ return !x; });
		return bad < 0 ? true : "assertion #" + bad + " failed";
	})()`)
	if err != nil {
		t.Fatalf("newKeymap script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatalf("newKeymap assertions failed: %v", v)
	}
}

// TestNewKeymap_Empty verifies the default keymap is fully opt-in.
func TestNewKeymap_Empty(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		var km = tm.newKeymap();
		var r = km.handleKey("ctrl+a");
		return r.handled === false && km.armed() === false && km.listKeys() === "";
	})()`)
	if err != nil {
		t.Fatalf("empty keymap: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("empty keymap should handle nothing")
	}
}

// TestKeymapOverlay_JSBinding verifies termmux.keymapOverlay renders the
// bindings table in a bubbletea model and closes on any key.
func TestKeymapOverlay_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: needs bubbletea module + event loop")
	}

	runtime, termmux := testRequire(t)
	setOnLoop(t, runtime, "termmux", termmux)

	btMgr := bubbletea.NewManager(context.Background(), nil, nil, &bubbletea.SyncJSRunner{Runtime: runtime}, nil, nil)
	teaModuleObj := runtime.NewObject()
	bubbletea.Require(context.Background(), btMgr)(runtime, teaModuleObj)
	setOnLoop(t, runtime, "tea", teaModuleObj.Get("exports"))

	v, err := awaitJSValue(t, runtime, `
		var km = termmux.newKeymap({
			prefix: "ctrl+a",
			chords: { "c": "new-window", "x": "close-pane" },
			keys: { "f1": "help" }
		});
		var closed = false;
		var ov = await termmux.keymapOverlay({
			keymap: km,
			tea: tea,
			onClose: function() { closed = true; }
		});
		var ok = true;
		ok = ok && ov && ov.model && typeof ov.visible === "function" && ov.visible() === true;
		// Drive a WindowSize then a Key through _update; then render via view.
		ov._update({type: 'WindowSize', width: 80, height: 24});
		ov._update({type: 'Key', key: 'esc'});
		ok = ok && ov.visible() === false && closed === true;
		return ok;
	`)
	if err != nil {
		t.Fatalf("keymapOverlay script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("keymapOverlay did not behave as expected")
	}

	// The overlay's VIEW (what the user actually sees) contains the rendered
	// bindings table — not merely km.listKeys(). Drive a WindowSize so the
	// model has dimensions, then read the view content through the model.
	v2, err := awaitJSValue(t, runtime, `
		var km = termmux.newKeymap({ prefix: "ctrl+a", chords: { "c": "new-window" }, keys: { "f1": "help" } });
		var ov = await termmux.keymapOverlay({ keymap: km, tea: tea });
		ov._update({type: 'WindowSize', width: 80, height: 24});
		var goModel = ov.model._getModel();
		var view = goModel.View();
		return typeof view.Content === "string"
			&& view.Content.indexOf("new-window") >= 0
			&& view.Content.indexOf("help") >= 0
			&& view.Content.indexOf("ctrl+a") >= 0
			&& view.Content.indexOf("┌") >= 0;
	`)
	if err != nil {
		t.Fatalf("overlay view check: %v", err)
	}
	if !v2.ToBoolean() {
		t.Fatalf("overlay view content missing bindings or frame: %v", v2)
	}
}

// TestKeymapOverlay_NewModelErrorRejects verifies that when tea.newModel
// fails, keymapOverlay's promise REJECTS (not resolves with an error object),
// so `await` throws instead of handing back a bogus value.
func TestKeymapOverlay_NewModelErrorRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: needs bubbletea module + event loop")
	}

	runtime, termmux := testRequire(t)
	setOnLoop(t, runtime, "termmux", termmux)

	if _, err := awaitJSValue(t, runtime, `
		globalThis.__km = termmux.newKeymap({ keys: { "f1": "help" } });
		return true;
	`); err != nil {
		t.Fatalf("setup: %v", err)
	}
	// Drive keymapOverlay with a tea object whose newModel throws.
	if err := awaitJSErr(t, runtime, `
		var teaBad = { newModel: function(cfg) { throw new Error("boom"); } };
		await termmux.keymapOverlay({ keymap: globalThis.__km, tea: teaBad });
	`); err == nil {
		t.Fatal("expected keymapOverlay to REJECT when tea.newModel throws, got resolution")
	}
}

// TestNewKeymap_SkipsNullActionValues verifies null/undefined action values in
// keys/chords maps are skipped (not bound as "null"/"undefined" strings).
func TestNewKeymap_SkipsNullActionValues(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		var km = tm.newKeymap({
			keys: { "f1": "help", "f2": null, "f3": undefined },
			chords: { "c": "new-window", "x": null }
		});
		var out = [];
		out.push(km.handleKey("f1").action === "help");
		out.push(km.handleKey("f2").handled === false);
		out.push(km.handleKey("f3").handled === false);
		km.setPrefix("ctrl+a");
		km.handleKey("ctrl+a");
		out.push(km.handleKey("c").action === "new-window");
		km.handleKey("ctrl+a");
		out.push(km.handleKey("x").handled === false);
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("null-skip script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("null/undefined action values should be skipped")
	}
}

// TestNewKeymap_HandleKeyNullRejects verifies handleKey(null) throws a
// TypeError instead of coercing to the string "null".
func TestNewKeymap_HandleKeyNullRejects(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		var km = tm.newKeymap();
		try {
			km.handleKey(null);
			return "no-throw";
		} catch (e) {
			return e instanceof TypeError ? "typeerror" : ("other: " + e);
		}
	})()`)
	if err != nil {
		t.Fatalf("handleKey(null) script: %v", err)
	}
	if v.String() != "typeerror" {
		t.Fatalf("handleKey(null) = %q, want TypeError", v.String())
	}
}
