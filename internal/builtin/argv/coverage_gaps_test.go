package argv

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// --- formatArgv edge cases ---

func TestFormatArgv_Null(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	result, err := formatFn(goja.Undefined(), goja.Null())
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if result.String() != "" {
		t.Errorf("expected empty string for null, got %q", result.String())
	}
}

func TestFormatArgv_Undefined(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	result, err := formatFn(goja.Undefined(), goja.Undefined())
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if result.String() != "" {
		t.Errorf("expected empty string for undefined, got %q", result.String())
	}
}

func TestFormatArgv_EmptyStringInArray(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// Empty string should be quoted
	input := runtime.ToValue([]string{"cmd", "", "arg"})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	expected := `cmd '' arg`
	if result.String() != expected {
		t.Errorf("expected %q, got %q", expected, result.String())
	}
}

func TestFormatArgv_EmbeddedQuotes(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// Args with spaces AND embedded quotes should be single-quoted
	input := runtime.ToValue([]string{`say "hello world"`})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	expected := `'say "hello world"'`
	if result.String() != expected {
		t.Errorf("expected %q, got %q", expected, result.String())
	}
}

func TestFormatArgv_TabCharacter(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// Tab character should trigger quoting
	input := runtime.ToValue([]string{"cmd", "arg\twith\ttabs"})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	expected := "cmd 'arg\twith\ttabs'"
	if result.String() != expected {
		t.Errorf("expected %q, got %q", expected, result.String())
	}
}

func TestFormatArgv_NewlineCharacter(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	input := runtime.ToValue([]string{"echo", "hello\nworld"})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	expected := "echo 'hello\nworld'"
	if result.String() != expected {
		t.Errorf("expected %q, got %q", expected, result.String())
	}
}

func TestFormatArgv_EmptyArray(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	input := runtime.ToValue([]string{})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if result.String() != "" {
		t.Errorf("expected empty string for empty array, got %q", result.String())
	}
}

func TestFormatArgv_SingleArg(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	input := runtime.ToValue([]string{"cmd"})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if result.String() != "cmd" {
		t.Errorf("expected 'cmd', got %q", result.String())
	}
}

func TestFormatArgv_UnicodeWhitespace(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// \u00A0 is non-breaking space (NBSP), matched by ShellQuote as unsafe
	input := runtime.ToValue([]string{"cmd", "arg\u00A0val"})
	result, err := formatFn(goja.Undefined(), input)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	expected := "cmd 'arg\u00A0val'"
	if result.String() != expected {
		t.Errorf("expected %q, got %q", expected, result.String())
	}
}

func TestFormatArgv_BooleanArg(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// Single non-array value (boolean) should fall through to String()
	result, err := formatFn(goja.Undefined(), runtime.ToValue(true))
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	if result.String() != "true" {
		t.Errorf("expected 'true', got %q", result.String())
	}
}

func TestParseArgv_Empty(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	parseFn, ok := goja.AssertFunction(exports.Get("parseArgv"))
	if !ok {
		t.Fatalf("parseArgv export is not a function")
	}

	result, err := parseFn(goja.Undefined(), runtime.ToValue(""))
	if err != nil {
		t.Fatalf("parseArgv returned error: %v", err)
	}

	var args []string
	if exportErr := runtime.ExportTo(result, &args); exportErr != nil {
		t.Fatalf("failed to export: %v", exportErr)
	}
	if len(args) != 0 {
		t.Errorf("expected empty array for empty string, got %v", args)
	}
}

func TestParseArgv_SingleQuotes(t *testing.T) {
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	parseFn, ok := goja.AssertFunction(exports.Get("parseArgv"))
	if !ok {
		t.Fatalf("parseArgv export is not a function")
	}

	result, err := parseFn(goja.Undefined(), runtime.ToValue(`echo 'hello world'`))
	if err != nil {
		t.Fatalf("parseArgv returned error: %v", err)
	}

	var args []string
	if exportErr := runtime.ExportTo(result, &args); exportErr != nil {
		t.Fatalf("failed to export: %v", exportErr)
	}
	if len(args) < 2 {
		t.Fatalf("expected at least 2 args, got %v", args)
	}
}

func TestFormatArgv_ObjectArg(t *testing.T) {
	// A plain object (not array-like) should fall through to String()
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	obj := runtime.NewObject()
	_ = obj.Set("key", "value")
	result, err := formatFn(goja.Undefined(), runtime.ToValue(obj))
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}
	// Object falls through both ExportTo calls (not []string, not []interface{})
	// Ultimate fallback: arg.String()
	if result.String() == "" {
		t.Error("expected non-empty string representation of object")
	}
}

func TestFormatArgv_ArrayWithObjects(t *testing.T) {
	// Test formatArgv with a JS array containing mixed types including objects.
	// goja's ExportTo to []string succeeds even for objects (using toString conversion),
	// so this exercises the main []string path with object stringification.
	runtime, module := setupModule(t)
	Require(runtime, module)

	exports := module.Get("exports").(*goja.Object)
	formatFn, ok := goja.AssertFunction(exports.Get("formatArgv"))
	if !ok {
		t.Fatalf("formatArgv export is not a function")
	}

	// Create a JS array containing an object — goja converts to string via toString
	arr, err := runtime.RunString(`([1, {name: "test"}, "hello"])`)
	if err != nil {
		t.Fatalf("failed to create JS array: %v", err)
	}

	result, err := formatFn(goja.Undefined(), arr)
	if err != nil {
		t.Fatalf("formatArgv returned error: %v", err)
	}

	// Validates that formatArgv handles mixed-type arrays correctly
	got := result.String()
	if got == "" {
		t.Error("expected non-empty result from mixed-type array")
	}
	// Should contain "hello" and the number 1 somewhere in the output
	if !strings.Contains(got, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", got)
	}
	if !strings.Contains(got, "1") {
		t.Errorf("expected output to contain '1', got %q", got)
	}
}
