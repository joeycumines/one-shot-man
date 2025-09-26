package argv

import (
	"errors"
	"testing"

	"github.com/dop251/goja"
)

func setupModule(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()

	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)
	return runtime, module
}

func TestParseArgv(t *testing.T) {
	runtime, module := setupModule(t)
	LoadModule(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	parseFn, ok := goja.AssertFunction(exports.Get("parseArgv"))
	if !ok {
		t.Fatalf("parseArgv export is not a function")
	}

	result, err := parseFn(goja.Undefined(), runtime.ToValue(`cmd "arg 1" arg2`))
	if err != nil {
		t.Fatalf("parseArgv returned error: %v", err)
	}

	var args []string
	if exportErr := runtime.ExportTo(result, &args); exportErr != nil {
		t.Fatalf("failed to export parseArgv result: %v", exportErr)
	}

	expected := []string{"cmd", "arg 1", "arg2"}
	if len(args) != len(expected) {
		t.Fatalf("expected %d args, got %d", len(expected), len(args))
	}
	for i, arg := range expected {
		if args[i] != arg {
			t.Fatalf("expected args[%d] = %q, got %q", i, arg, args[i])
		}
	}
}

func TestParseArgvTypeError(t *testing.T) {
	runtime, module := setupModule(t)
	LoadModule(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	parseFn, ok := goja.AssertFunction(exports.Get("parseArgv"))
	if !ok {
		t.Fatalf("parseArgv export is not a function")
	}

	_, err := parseFn(goja.Undefined(), runtime.ToValue(123))
	if err == nil {
		t.Fatalf("expected parseArgv to return error on non-string argument")
	}

	var ex *goja.Exception
	if !errors.As(err, &ex) {
		t.Fatalf("expected goja exception, got %T: %v", err, err)
	}

	if ex.Value() == nil || ex.Value().String() != "TypeError: parseArgv: argument must be a string" {
		t.Fatalf("unexpected error: %v", ex.Value())
	}
}

func TestFormatArgvQuoting(t *testing.T) {
	runtime, module := setupModule(t)
	LoadModule(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	input := runtime.ToValue([]string{"cmd", "arg with space", "plain"})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}

	var formatted string
	if exportErr := runtime.ExportTo(result, &formatted); exportErr != nil {
		t.Fatalf("failed to export formatArgv result: %v", exportErr)
	}

	expected := `cmd "arg with space" plain`
	if formatted != expected {
		t.Fatalf("expected %q, got %q", expected, formatted)
	}
}

func TestFormatArgvFallbacks(t *testing.T) {
	runtime, module := setupModule(t)
	LoadModule(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// No arguments defaults to empty string.
	noArgVal, err := formatFn(goja.Undefined())
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if noArgVal.String() != "" {
		t.Fatalf("expected empty string for no arguments, got %q", noArgVal.String())
	}

	// Mixed type arrays fall back to sprint conversion.
	mixedArray, evalErr := runtime.RunString(`[1, "two", true]`)
	if evalErr != nil {
		t.Fatalf("failed to create mixed array: %v", evalErr)
	}
	mixedVal, err := formatFn(goja.Undefined(), mixedArray)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if mixedVal.String() != "1 two true" {
		t.Fatalf("expected fallback formatting, got %q", mixedVal.String())
	}

	// Non-array arguments fall back to String() representation.
	nonArrayVal, err := formatFn(goja.Undefined(), runtime.ToValue(123))
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if nonArrayVal.String() != "123" {
		t.Fatalf("expected string conversion of number, got %q", nonArrayVal.String())
	}
}
