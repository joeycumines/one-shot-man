package template

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// --- Require function edge cases ---

func TestRequire_ExportsUndefined(t *testing.T) {
	// When module.exports is undefined, Require should create a new exports object.
	runtime := goja.New()
	module := runtime.NewObject()
	// Do NOT set "exports" — leave it undefined.

	loader := Require(context.Background())
	loader(runtime, module)

	exports := module.Get("exports")
	if goja.IsUndefined(exports) || goja.IsNull(exports) {
		t.Fatal("expected exports to be created, got undefined/null")
	}

	obj := exports.ToObject(runtime)
	newFn := obj.Get("new")
	if goja.IsUndefined(newFn) {
		t.Fatal("expected 'new' function on exports")
	}
	execFn := obj.Get("execute")
	if goja.IsUndefined(execFn) {
		t.Fatal("expected 'execute' function on exports")
	}
}

func TestRequire_ExportsNull(t *testing.T) {
	runtime := goja.New()
	module := runtime.NewObject()
	_ = module.Set("exports", goja.Null())

	loader := Require(context.Background())
	loader(runtime, module)

	exports := module.Get("exports")
	if goja.IsUndefined(exports) || goja.IsNull(exports) {
		t.Fatal("expected exports to be created, got undefined/null")
	}
}

// --- Quick execute error paths ---

func TestQuickExecute_NoArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	_, err := runtime.RunString(`
		try {
			exports.execute();
			"no error";
		} catch (e) {
			e.message || e.toString();
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
}

func TestQuickExecute_InvalidTemplate(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			exports.execute("{{.name", {});
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

func TestQuickExecute_ExecutionError(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			exports.execute("{{call .fn}}", {fn: "not-a-function"});
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

func TestQuickExecute_NoData(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.execute("hello");`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "hello" {
		t.Errorf("expected 'hello', got %q", val.String())
	}
}

// --- Template wrapper: parse error paths ---

func TestParse_NoArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			const tmpl = exports.new("t");
			tmpl.parse();
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

// --- Template wrapper: funcs error paths ---

func TestFuncs_NoArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			const tmpl = exports.new("t");
			tmpl.funcs();
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

func TestFuncs_NonFunctionValues(t *testing.T) {
	// Go's template.Funcs() panics on non-function values in the FuncMap.
	// The convertFuncMap code exports non-function values, but template.Funcs()
	// rejects them with a Go panic (not goja exception). This test verifies
	// the panic occurs and is recoverable.
	runtime, _ := setupModule(t)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic from non-function value in funcMap")
		}
		// Verify it's the expected panic message
		msg := fmt.Sprintf("%v", r)
		if !strings.Contains(msg, "not a function") {
			t.Errorf("unexpected panic message: %s", msg)
		}
	}()

	// This will panic because Go's template.Funcs rejects non-function values
	_, _ = runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.funcs({ myConst: "hello" });
	`)
	t.Fatal("should not reach here")
}

// --- Template wrapper: delims error paths ---

func TestDelims_NotEnoughArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			const tmpl = exports.new("t");
			tmpl.delims("<<");
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

// --- Template wrapper: option ---

func TestOption_MissingOption(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.option("missingkey=zero");
		tmpl.parse("{{.missing}}");
		tmpl.execute({});
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "<no value>" {
		t.Errorf("expected '<no value>', got %q", val.String())
	}
}

func TestOption_MissingKeyZero(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.option("missingkey=zero");
		tmpl.parse("{{.count}}");
		tmpl.execute({});
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// With missingkey=zero, missing keys render as zero value
	if val.String() != "<no value>" {
		t.Errorf("expected '<no value>', got %q", val.String())
	}
}

func TestOption_ErrorOnMissing(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			const tmpl = exports.new("t");
			tmpl.option("missingkey=error");
			tmpl.parse("{{.missing}}");
			tmpl.execute({});
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

func TestOption_NoArgs(t *testing.T) {
	// option() with no arguments should not panic.
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.option();
		tmpl.parse("hello");
		tmpl.execute({});
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "hello" {
		t.Errorf("expected 'hello', got %q", val.String())
	}
}

// --- Template new with no name ---

func TestNew_NoName(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new();
		tmpl.name();
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "" {
		t.Errorf("expected empty name, got %q", val.String())
	}
}

// --- Execute without data ---

func TestExecute_NoData(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.parse("static");
		tmpl.execute();
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "static" {
		t.Errorf("expected 'static', got %q", val.String())
	}
}

// --- wrapJSFunction non-error panic recovery ---

func TestWrapJSFunction_NonErrorPanic(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.funcs({
			crasher: function() {
				throw 42;  // Non-Error throw — triggers fmt.Errorf("%v", r) path
			}
		});
		tmpl.parse("{{crasher}}");
		try {
			tmpl.execute({});
			"no error";
		} catch (e) {
			"error caught";
		}
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "error caught" {
		t.Errorf("expected error caught, got %q", val.String())
	}
}

// --- convertFuncMap with nil object ---

// convertFuncMap: obj == nil guard after ToObject is defensive dead code.
// ToObject panics on null/undefined before returning nil,
// so this path cannot be reached in production or tests.

// --- Multiple funcs calls ---

func TestFuncs_MultipleCalls(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.funcs({ upper: function(s) { return s.toUpperCase(); } });
		tmpl.funcs({ lower: function(s) { return s.toLowerCase(); } });
		tmpl.parse("{{upper .a}} {{lower .b}}");
		tmpl.execute({a: "hello", b: "WORLD"});
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "HELLO world" {
		t.Errorf("expected 'HELLO world', got %q", val.String())
	}
}

// --- wrapJSFunction with multiple args ---

func TestWrapJSFunction_ZeroArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		const tmpl = exports.new("t");
		tmpl.funcs({ now: function() { return "today"; } });
		tmpl.parse("{{now}}");
		tmpl.execute({});
	`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "today" {
		t.Errorf("expected 'today', got %q", val.String())
	}
}
