package unicodetext

import (
	"context"
	"testing"

	"github.com/dop251/goja"
)

func setupModule(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()

	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	loader := Require(context.Background())
	loader(runtime, module)

	// Make exports available in the runtime
	_ = runtime.Set("exports", module.Get("exports"))

	return runtime, exports
}

func TestWidthFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		const cases = [
			{in: "", want: 0, name: "empty string"},
			{in: "abc", want: 3, name: "basic ascii"},
			{in: "你好", want: 4, name: "CJK characters"}, // 2 chars * 2 width
			{in: "e\u0301", want: 1, name: "combining accent"}, // 'e' + combining acute accent = 1 grapheme
			{in: "a\u200bb", want: 2, name: "zero width space"}, // 'a' + ZWSP + 'b', ZWSP is width 0
			{in: "🏳️‍🌈", want: 2, name: "complex emoji"}, // Rainbow flag usually 2 in uniseg
			{in: "😀", want: 2, name: "simple emoji"},
		];

		const errors = [];
		for (const c of cases) {
			const got = exports.width(c.in);
			if (got !== c.want) {
				errors.push("Case '" + c.name + "': expected " + c.want + ", got " + got);
			}
		}

		errors.join("\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	if result := val.String(); result != "" {
		t.Errorf("exports.width failures:\n%s", result)
	}
}

func TestTruncateFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		const cases = [
			// Basic ASCII cases
			{s: "hello world", max: 5, tail: "...", want: "he...", name: "standard truncate"},
			{s: "hello", max: 5, tail: "...", want: "hello", name: "exact fit"},
			{s: "hello", max: 10, tail: "...", want: "hello", name: "no truncation needed"},
			{s: "hello", max: 4, tail: ".", want: "hel.", name: "custom tail single char"},
			{s: "hello", max: 4, tail: "", want: "hell", name: "empty tail"},

			// Edge cases: Tail wider than maxWidth
			// If tail (3) > max (2), code returns tail "..."
			{s: "abc", max: 2, tail: "...", want: "...", name: "tail wider than max"},

			// Edge cases: Unicode / CJK
			// "你好世界" -> Each char is width 2.
			// Max 3, tail ".". Target = 3-1 = 2.
			// First char "你" (2). Current=2. Matches target.
			// Next char "好" (2). 2+2 > 2. Break.
			// Result "你" + "." = "你."
			{s: "你好世界", max: 3, tail: ".", want: "你.", name: "CJK truncate split"},

			// "你好世界", max 5, tail ".". Target 4.
			// "你"(2) + "好"(2) = 4. Matches target.
			// Result "你好" + "." = "你好." (Total width 5)
			{s: "你好世界", max: 5, tail: ".", want: "你好.", name: "CJK exact grapheme fit"},

			// Complex Graphemes
			// "e\u0301" is width 1.
			{s: "e\u0301abcd", max: 2, tail: ".", want: "e\u0301.", name: "combining char preservation"},

			// Emoji
			// "😀" is width 2.
			// "😀bc", max 2, tail ".". Target 1.
			// "😀" (2) > 1. Break immediately.
			// Result "" + "." = "."
			{s: "😀bc", max: 2, tail: ".", want: ".", name: "emoji exceeds target"},

			// "abc😀", max 4, tail ".". Target 3.
			// "abc" (3) fits. "😀" (2) -> 5 > 3. Break.
			// Result "abc."
			{s: "abc😀", max: 4, tail: ".", want: "abc.", name: "emoji split at boundary"},
		];

		const errors = [];
		for (const c of cases) {
			// truncate(s, maxWidth, tail)
			const got = exports.truncate(c.s, c.max, c.tail);
			if (got !== c.want) {
				errors.push("Case '" + c.name + "': input='" + c.s + "', max=" + c.max + ", tail='" + c.tail + "'. Expected '" + c.want + "', got '" + got + "'");
			}
		}

		// Verify default tail behavior (optional argument)
		const defaultTail = exports.truncate("hello world", 5);
		if (defaultTail !== "he...") {
			errors.push("Default tail: expected 'he...', got '" + defaultTail + "'");
		}

		errors.join("\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}

	if result := val.String(); result != "" {
		t.Errorf("exports.truncate failures:\n%s", result)
	}
}

func TestPadRightFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		var errors = [];
		var r = exports.padRight("hello", 10);
		if (r !== "hello     ") errors.push("padRight('hello',10) = '" + r + "'");
		if (exports.padRight("hello", 5) !== "hello") errors.push("padRight no-op failed");
		if (exports.padRight("hello", 3) !== "hello") errors.push("padRight shorter failed");
		errors.join("\\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}
	if result := val.String(); result != "" {
		t.Errorf("padRight failures:\n%s", result)
	}
}

func TestPadLeftFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		var errors = [];
		var r = exports.padLeft("hello", 10);
		if (r !== "     hello") errors.push("padLeft('hello',10) = '" + r + "'");
		if (exports.padLeft("hello", 5) !== "hello") errors.push("padLeft no-op failed");
		errors.join("\\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}
	if result := val.String(); result != "" {
		t.Errorf("padLeft failures:\n%s", result)
	}
}

func TestPadCenterFunction(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		var errors = [];
		var r = exports.padCenter("hello", 10);
		if (r !== "  hello   ") errors.push("padCenter('hello',10) = '" + r + "'");
		if (exports.padCenter("hi", 10) !== "    hi    ") errors.push("padCenter('hi',10) = '" + exports.padCenter("hi", 10) + "'");
		if (exports.padCenter("hello", 5) !== "hello") errors.push("padCenter no-op failed");
		errors.join("\\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}
	if result := val.String(); result != "" {
		t.Errorf("padCenter failures:\n%s", result)
	}
}

func TestPadUnicodeAware(t *testing.T) {
	runtime, _ := setupModule(t)

	script := `
		var errors = [];
		var r = exports.padRight("\u4e16\u754c", 6);
		if (r !== "\u4e16\u754c  ") errors.push("CJK padRight = '" + r + "'");
		var l = exports.padLeft("\u4e16\u754c", 6);
		if (l !== "  \u4e16\u754c") errors.push("CJK padLeft = '" + l + "'");
		errors.join("\\n");
	`

	val, err := runtime.RunString(script)
	if err != nil {
		t.Fatalf("script execution failed: %v", err)
	}
	if result := val.String(); result != "" {
		t.Errorf("unicode pad failures:\n%s", result)
	}
}
