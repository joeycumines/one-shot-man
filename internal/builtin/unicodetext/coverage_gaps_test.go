package unicodetext

import (
	"context"
	"testing"

	"github.com/dop251/goja"
)

// --- Require function edge cases ---

func TestRequire_ExportsUndefined(t *testing.T) {
	runtime := goja.New()
	module := runtime.NewObject()
	// Do NOT set "exports" — leave undefined.

	loader := Require(context.Background())
	loader(runtime, module)

	exports := module.Get("exports")
	if goja.IsUndefined(exports) || goja.IsNull(exports) {
		t.Fatal("expected exports to be created, got undefined/null")
	}

	obj := exports.ToObject(runtime)
	if goja.IsUndefined(obj.Get("width")) {
		t.Fatal("expected 'width' on exports")
	}
	if goja.IsUndefined(obj.Get("truncate")) {
		t.Fatal("expected 'truncate' on exports")
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
		t.Fatal("expected exports to be created from null, got undefined/null")
	}
}

// --- Width edge cases ---

func TestWidth_NoArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.width()`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.ToInteger() != 0 {
		t.Errorf("expected 0 for no args, got %d", val.ToInteger())
	}
}

func TestWidth_Tab(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.width("\t")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// Tab has width 0 in uniseg
	if val.ToInteger() < 0 {
		t.Errorf("expected non-negative width for tab, got %d", val.ToInteger())
	}
}

// --- Truncate edge cases ---

func TestTruncate_NotEnoughArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			exports.truncate("hello");
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

func TestTruncate_ZeroArgs(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`
		try {
			exports.truncate();
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

func TestTruncate_ZeroMaxWidth(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.truncate("hello", 0, "")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// maxWidth=0, tail="", tailWidth=0 <= 0, targetWidth=0
	// First cluster "h" (1) > 0, break immediately. Result = "" + "" = ""
	if val.String() != "" {
		t.Errorf("expected empty string for maxWidth=0 empty tail, got %q", val.String())
	}
}

func TestTruncate_TailExactlyMaxWidth(t *testing.T) {
	runtime, _ := setupModule(t)

	// tail "..." has width 3, maxWidth = 3
	// tailWidth (3) > maxWidth (3) is false
	// targetWidth = 3 - 3 = 0
	// First cluster exceeds targetWidth (0), break immediately
	// Result = "" + "..." = "..."
	val, err := runtime.RunString(`exports.truncate("hello", 3, "...")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	if val.String() != "..." {
		t.Errorf("expected '...' for tail=maxWidth, got %q", val.String())
	}
}

func TestTruncate_MaxWidthOne(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.truncate("hello", 1, ".")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// tailWidth=1, targetWidth=0, first cluster exceeds 0, result = "."
	if val.String() != "." {
		t.Errorf("expected '.', got %q", val.String())
	}
}

func TestTruncate_EmptyString(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.truncate("", 5, "...")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// Empty string has width 0 <= 5, fits entirely
	if val.String() != "" {
		t.Errorf("expected empty string, got %q", val.String())
	}
}

func TestTruncate_SingleCharExact(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.truncate("a", 1, ".")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// "a" has width 1 <= maxWidth 1, no truncation needed
	if val.String() != "a" {
		t.Errorf("expected 'a', got %q", val.String())
	}
}

func TestTruncate_OnlyWideChars(t *testing.T) {
	runtime, _ := setupModule(t)

	val, err := runtime.RunString(`exports.truncate("你好世界", 4, "")`)
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}
	// Each CJK char is width 2. Total width 8 > 4. No tail.
	// targetWidth = 4. "你"(2) + "好"(2) = 4. Next "世" would make 6>4. Break.
	// Result: "你好"
	if val.String() != "你好" {
		t.Errorf("expected '你好', got %q", val.String())
	}
}
