package termmux

import (
	"context"
	"testing"
	"time"

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
	// model has dimensions, then read the view content through the public
	// ov.view() accessor.
	v2, err := awaitJSValue(t, runtime, `
		var km = termmux.newKeymap({ prefix: "ctrl+a", chords: { "c": "new-window" }, keys: { "f1": "help" } });
		var ov = await termmux.keymapOverlay({ keymap: km, tea: tea });
		ov._update({type: 'WindowSize', width: 80, height: 24});
		var view = ov.view();
		var render = ov.render();
		return typeof view.content === "string"
			&& typeof render === "string"
			&& render === view.content
			&& view.content.indexOf("new-window") >= 0
			&& view.content.indexOf("help") >= 0
			&& view.content.indexOf("ctrl+a") >= 0
			&& view.content.indexOf("┌") >= 0;
	`)
	if err != nil {
		t.Fatalf("overlay view check: %v", err)
	}
	if !v2.ToBoolean() {
		t.Fatalf("overlay view content missing bindings or frame: %v", v2)
	}

	// An unrecognized key string (one bubbletea.ParseKey cannot resolve, e.g.
	// a modified non-letter) must still close the overlay — the "any key
	// closes" contract.
	v3, err := awaitJSValue(t, runtime, `
		var km = termmux.newKeymap({ keys: { "f1": "help" } });
		var ov = await termmux.keymapOverlay({ keymap: km, tea: tea });
		ov._update({type: 'WindowSize', width: 80, height: 24});
		var before = ov.visible();
		ov._update({type: 'Key', key: 'alt+,'});
		return before === true && ov.visible() === false && ov.render() === "";
	`)
	if err != nil {
		t.Fatalf("unrecognized-key close script: %v", err)
	}
	if !v3.ToBoolean() {
		t.Fatal("overlay must close on an unrecognized key string")
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

// TestKeymapOverlay_NewModelErrorObjectRejects verifies that when
// tea.newModel RETURNS an error object (its invalid-config path returns
// {error, errorCode} instead of throwing), keymapOverlay still REJECTS the
// promise rather than resolving with {model: {error, ...}}.
func TestKeymapOverlay_NewModelErrorObjectRejects(t *testing.T) {
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
	// tea.newModel returns an error object when config.update is missing.
	if err := awaitJSErr(t, runtime, `
		var teaBad = { newModel: function(cfg) { return { error: "invalid args: update must be a function", errorCode: "invalidArgs" }; } };
		await termmux.keymapOverlay({ keymap: globalThis.__km, tea: teaBad });
	`); err == nil {
		t.Fatal("expected keymapOverlay to REJECT when tea.newModel returns an error object, got resolution")
	}
	// And with the real tea module: verify the real newModel error-object
	// path by monkey-patching newModel to delegate to the real one with a
	// broken config (no update fn → {error, errorCode} object). The real
	// module's Require must run ON the event loop (the runtime is live), so
	// route it through setOnLoop's submit barrier.
	done := make(chan struct{})
	loop := loopForRuntime(t, runtime)
	if err := loop.Submit(func() {
		defer close(done)
		btMgr := bubbletea.NewManager(context.Background(), nil, nil, &bubbletea.SyncJSRunner{Runtime: runtime}, nil, nil)
		teaModuleObj := runtime.NewObject()
		bubbletea.Require(context.Background(), btMgr)(runtime, teaModuleObj)
		_ = runtime.Set("tea", teaModuleObj.Get("exports"))
	}); err != nil {
		t.Fatalf("submit tea module setup: %v", err)
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out setting up real tea module on loop")
	}
	if err := awaitJSErr(t, runtime, `
		// Real tea.newModel returns an error object for a config without
		// update — but keymapOverlay always supplies one, so instead assert
		// the error-object DETECTION works by monkey-patching newModel to
		// delegate to the real one with a broken config.
		var realNewModel = tea.newModel;
		tea.newModel = function(cfg) { return realNewModel({}); };
		await termmux.keymapOverlay({ keymap: globalThis.__km, tea: tea });
	`); err == nil {
		t.Fatal("expected keymapOverlay to REJECT via the real newModel error-object path")
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

// TestKeymapObject_NullArgHandling verifies the mutator contract:
// setPrefix/setPassthroughKey treat null/undefined as "" (disable), bind and
// bindChord skip a null/undefined action, and bind/unbind/bindChord/unbindChord
// reject a null/undefined KEY with a TypeError instead of coercing to "null".
func TestKeymapObject_NullArgHandling(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		var km = tm.newKeymap({ keys: { "f1": "help" } });
		var out = [];
		// null/undefined action on bind/bindChord: no-op, existing binding kept.
		km.bind("f1", null);
		out.push(km.handleKey("f1").action === "help");
		km.bind("f2", undefined);
		out.push(km.handleKey("f2").handled === false);
		km.bindChord("c", null);
		km.setPrefix("ctrl+a");
		km.handleKey("ctrl+a");
		out.push(km.handleKey("c").handled === false);
		// setPrefix(null) disables the prefix (clears it).
		km.setPrefix(null);
		out.push(km.prefix() === "");
		km.setPrefix("ctrl+a");
		km.setPassthroughKey(undefined);
		out.push(km.passthroughKey() === "");
		km.setPassthroughKey(null);
		out.push(km.passthroughKey() === "");
		// null/undefined KEY must throw TypeError.
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		out.push(expectTypeError(function(){ km.bind(null, "x"); }));
		out.push(expectTypeError(function(){ km.bind(undefined, "x"); }));
		out.push(expectTypeError(function(){ km.bindChord(null, "x"); }));
		out.push(expectTypeError(function(){ km.unbind(null); }));
		out.push(expectTypeError(function(){ km.unbindChord(undefined); }));
		// missing key arg also TypeError.
		out.push(expectTypeError(function(){ km.bind(); }));
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("null-arg script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("keymap null/undefined argument handling incorrect")
	}
}

// TestNewKeymap_NonStringActionRejects verifies a non-string, non-null action
// value in keys/chords (e.g. a number or object) panics with a TypeError
// rather than being coerced to a string.
func TestNewKeymap_NonStringActionRejects(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		var out = [];
		out.push(expectTypeError(function(){ tm.newKeymap({ keys: { "f1": 0 } }); }));
		out.push(expectTypeError(function(){ tm.newKeymap({ keys: { "f1": {} } }); }));
		out.push(expectTypeError(function(){ tm.newKeymap({ chords: { "c": true } }); }));
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("non-string action script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("non-string action values should throw TypeError")
	}
}

// TestKeymapBind_TypeValidation pins R3-12/R3-19: bind/bindChord require a
// string key AND a string action (matching newKeymap's checks), unbind/
// unbindChord require a string key, and newKeymap's keys/chords must be plain
// objects — arrays and primitives throw TypeError instead of binding numeric
// indices. bindings() uses numeric index strings for array-like action lists.
func TestKeymapBind_TypeValidation(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		var km = tm.newKeymap({});
		var out = [];
		// bind/bindChord: string key AND string action required.
		out.push(expectTypeError(function(){ km.bind("x", 5); }));
		out.push(expectTypeError(function(){ km.bind("x", {}); }));
		out.push(expectTypeError(function(){ km.bind(123, "a"); }));
		out.push(expectTypeError(function(){ km.bind(true, "a"); }));
		out.push(expectTypeError(function(){ km.bindChord("x", 5); }));
		out.push(expectTypeError(function(){ km.bindChord(123, "a"); }));
		// unbind/unbindChord: string key required.
		out.push(expectTypeError(function(){ km.unbind(123); }));
		out.push(expectTypeError(function(){ km.unbindChord({}); }));
		// Valid string/string still works.
		km.bind("x", "act");
		out.push(km.handleKey("x").action === "act");
		// newKeymap keys/chords must be plain objects.
		out.push(expectTypeError(function(){ tm.newKeymap({ keys: ["a"] }); }));
		out.push(expectTypeError(function(){ tm.newKeymap({ chords: 42 }); }));
		out.push(expectTypeError(function(){ tm.newKeymap({ keys: "ab" }); }));
		// bindings() reports numeric indices as string keys.
		var km2 = tm.newKeymap({ keys: { "0": "zero", "a": "alpha" } });
		var b = km2.bindings();
		var found0 = false;
		for (var i = 0; i < b.length; i++) {
			if (b[i].key === "0" && b[i].action === "zero") found0 = true;
		}
		out.push(found0);
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("type-validation script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("keymap bind type validation incorrect")
	}
}

// TestKeymap_StrictStringArg pins R4-6: every keymap method rejects
// non-string arguments (objects, symbols, numbers, booleans) with a TypeError
// BEFORE any .String() coercion — previously bind/unbind coerced objects via
// .String() to "[object Object]" before the type check, and setPrefix coerced
// numbers silently.
func TestKeymap_StrictStringArg(t *testing.T) {
	t.Parallel()

	runtime, exports := testRequire(t)
	setOnLoop(t, runtime, "tm", exports)

	v, err := sessionRun(t, runtime, `(function(){
		function expectTypeError(fn) {
			try { fn(); return false; }
			catch (e) { return e instanceof TypeError; }
		}
		var km = tm.newKeymap({});
		var out = [];
		// handleKey: object/symbol/number must throw, not coerce.
		out.push(expectTypeError(function(){ km.handleKey({}); }));
		out.push(expectTypeError(function(){ km.handleKey(Symbol("s")); }));
		out.push(expectTypeError(function(){ km.handleKey(42); }));
		// bind/bindChord key and action: non-strings throw.
		out.push(expectTypeError(function(){ km.bind({ toString: function(){ return "x"; } }, "a"); }));
		out.push(expectTypeError(function(){ km.bind("x", Symbol("a")); }));
		out.push(expectTypeError(function(){ km.bindChord(42, "a"); }));
		out.push(expectTypeError(function(){ km.bindChord("x", [1]); }));
		// unbind/unbindChord: non-string key throws.
		out.push(expectTypeError(function(){ km.unbind({}); }));
		out.push(expectTypeError(function(){ km.unbindChord(7); }));
		// setPrefix/setPassthroughKey: null/undefined = disable (no throw),
		// non-string = TypeError.
		km.setPrefix(null); km.setPassthroughKey(undefined);
		out.push(km.prefix() === "" && km.passthroughKey() === "");
		out.push(expectTypeError(function(){ km.setPrefix(42); }));
		out.push(expectTypeError(function(){ km.setPassthroughKey({}); }));
		// newKeymap opts: non-string prefix/passthroughKey throws.
		out.push(expectTypeError(function(){ tm.newKeymap({ prefix: 42 }); }));
		out.push(expectTypeError(function(){ tm.newKeymap({ passthroughKey: {} }); }));
		// String primitives still work everywhere.
		km.setPrefix("ctrl+a");
		out.push(km.prefix() === "ctrl+a");
		return out.every(function(x){ return x; });
	})()`)
	if err != nil {
		t.Fatalf("strict-string script: %v", err)
	}
	if !v.ToBoolean() {
		t.Fatal("keymap strict string validation incorrect")
	}
}
