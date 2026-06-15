package termmux

import (
	"testing"

	"github.com/dop251/goja"
)

func setupControlRouter(t *testing.T) *goja.Runtime {
	t.Helper()
	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	_, err := runtime.RunString(`
		var router = exports.newControlRouter({
			keys: {
				'ctrl+p': 'pause',
				'ctrl+b': 'bigger',
				'ctrl+s': 'smaller',
				'q': 'quit'
			},
			chordMode: {
				prefix: 'ctrl+x',
				actions: {
					's': 'smaller',
					'b': 'bigger',
					'c': 'close'
				}
			}
		});
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return runtime
}

func TestControlRouter_HandleKnownKey(t *testing.T) {
	runtime := setupControlRouter(t)

	v, err := runtime.RunString(`router.handleKey('ctrl+p')`)
	if err != nil {
		t.Fatalf("handleKey: %v", err)
	}
	obj := v.(*goja.Object)
	if !obj.Get("handled").ToBoolean() {
		t.Error("expected handled=true for ctrl+p")
	}
	if obj.Get("action").String() != "pause" {
		t.Errorf("expected action='pause', got %q", obj.Get("action").String())
	}
}

func TestControlRouter_HandleUnknownKey(t *testing.T) {
	runtime := setupControlRouter(t)

	v, err := runtime.RunString(`router.handleKey('a')`)
	if err != nil {
		t.Fatalf("handleKey: %v", err)
	}
	obj := v.(*goja.Object)
	if obj.Get("handled").ToBoolean() {
		t.Error("expected handled=false for unknown key")
	}
}

func TestControlRouter_ChordModeEnter(t *testing.T) {
	runtime := setupControlRouter(t)

	v, err := runtime.RunString(`router.handleKey('ctrl+x')`)
	if err != nil {
		t.Fatalf("handleKey chord prefix: %v", err)
	}
	obj := v.(*goja.Object)
	if !obj.Get("handled").ToBoolean() {
		t.Error("expected handled=true for chord prefix")
	}

	inChord, err := runtime.RunString(`router.inChordMode()`)
	if err != nil {
		t.Fatalf("inChordMode: %v", err)
	}
	if !inChord.ToBoolean() {
		t.Error("expected inChordMode=true after chord prefix")
	}
}

func TestControlRouter_ChordResolve(t *testing.T) {
	runtime := setupControlRouter(t)

	_, err := runtime.RunString(`router.handleKey('ctrl+x')`)
	if err != nil {
		t.Fatalf("chord prefix: %v", err)
	}

	v, err := runtime.RunString(`router.handleKey('s')`)
	if err != nil {
		t.Fatalf("chord key: %v", err)
	}
	obj := v.(*goja.Object)
	if !obj.Get("handled").ToBoolean() {
		t.Error("expected handled=true for chord resolution")
	}
	if obj.Get("action").String() != "smaller" {
		t.Errorf("expected action='smaller', got %q", obj.Get("action").String())
	}

	inChord, _ := runtime.RunString(`router.inChordMode()`)
	if inChord.ToBoolean() {
		t.Error("expected inChordMode=false after chord resolution")
	}
}

func TestControlRouter_ChordCancel(t *testing.T) {
	runtime := setupControlRouter(t)

	_, err := runtime.RunString(`router.handleKey('ctrl+x')`)
	if err != nil {
		t.Fatalf("chord prefix: %v", err)
	}

	v, err := runtime.RunString(`router.handleKey('esc')`)
	if err != nil {
		t.Fatalf("chord cancel: %v", err)
	}
	obj := v.(*goja.Object)
	if !obj.Get("handled").ToBoolean() {
		t.Error("expected handled=true for chord cancel")
	}

	inChord, _ := runtime.RunString(`router.inChordMode()`)
	if inChord.ToBoolean() {
		t.Error("expected inChordMode=false after cancel")
	}
}

func TestControlRouter_ChordUnknownKey(t *testing.T) {
	runtime := setupControlRouter(t)

	_, err := runtime.RunString(`router.handleKey('ctrl+x')`)
	if err != nil {
		t.Fatalf("chord prefix: %v", err)
	}

	v, err := runtime.RunString(`router.handleKey('z')`)
	if err != nil {
		t.Fatalf("unknown chord key: %v", err)
	}
	obj := v.(*goja.Object)
	if obj.Get("handled").ToBoolean() {
		t.Error("expected handled=false for unknown chord key")
	}

	inChord, _ := runtime.RunString(`router.inChordMode()`)
	if inChord.ToBoolean() {
		t.Error("expected inChordMode=false after unknown chord key")
	}
}

func TestControlRouter_NoChordMode(t *testing.T) {
	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	v, err := runtime.RunString(`
		var router = exports.newControlRouter({
			keys: { 'ctrl+p': 'pause' }
		});
		router.handleKey('ctrl+x').handled;
	`)
	if err != nil {
		t.Fatalf("no chord mode: %v", err)
	}
	if v.ToBoolean() {
		t.Error("expected handled=false when no chord mode configured")
	}
}

func TestControlRouter_EmptyConfig(t *testing.T) {
	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	v, err := runtime.RunString(`
		var router = exports.newControlRouter();
		router.handleKey('a').handled;
	`)
	if err != nil {
		t.Fatalf("empty config: %v", err)
	}
	if v.ToBoolean() {
		t.Error("expected handled=false with empty config")
	}
}

func TestControlRouter_MultipleKeys(t *testing.T) {
	runtime := setupControlRouter(t)

	for _, tc := range []struct{ key, action string }{
		{"ctrl+b", "bigger"},
		{"ctrl+s", "smaller"},
		{"q", "quit"},
	} {
		v, err := runtime.RunString(`router.handleKey('` + tc.key + `')`)
		if err != nil {
			t.Fatalf("handleKey %q: %v", tc.key, err)
		}
		obj := v.(*goja.Object)
		if obj.Get("action").String() != tc.action {
			t.Errorf("key %q: expected action=%q, got %q", tc.key, tc.action, obj.Get("action").String())
		}
	}
}

func TestControlRouter_HandleKeyNoArgs(t *testing.T) {
	runtime := setupControlRouter(t)

	v, err := runtime.RunString(`router.handleKey().handled`)
	if err != nil {
		t.Fatalf("handleKey no args: %v", err)
	}
	if v.ToBoolean() {
		t.Error("expected handled=false when no key provided")
	}
}

func TestControlRouter_HandleChordNoArgs(t *testing.T) {
	runtime := setupControlRouter(t)

	v, err := runtime.RunString(`router.handleChord().handled`)
	if err != nil {
		t.Fatalf("handleChord no args: %v", err)
	}
	if v.ToBoolean() {
		t.Error("expected handled=false when no chord key provided")
	}
}

func TestControlRouter_HandleChordNotInChord(t *testing.T) {
	runtime := setupControlRouter(t)

	v, err := runtime.RunString(`router.handleChord('s').handled`)
	if err != nil {
		t.Fatalf("handleChord not in chord: %v", err)
	}
	if v.ToBoolean() {
		t.Error("expected handled=false when not in chord")
	}
}

func TestControlRouter_ChordModePrefixOnly(t *testing.T) {
	runtime, exp := testRequire(t)
	_ = runtime.Set("exports", exp)

	v, err := runtime.RunString(`
		var router = exports.newControlRouter({ chordMode: { prefix: 'ctrl+x' } });
		router.handleKey('ctrl+x').handled;
	`)
	if err != nil {
		t.Fatalf("chord prefix only: %v", err)
	}
	if !v.ToBoolean() {
		t.Error("expected handled=true for chord prefix even without actions")
	}
}
