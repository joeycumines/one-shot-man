package ctxutil

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func setupBuildContext(t *testing.T) *goja.Runtime {
	t.Helper()

	runtime := goja.New()
	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	loader := Require(context.Background())
	loader(runtime, module)

	if err := runtime.Set("exports", exports); err != nil {
		t.Fatalf("failed to bind exports: %v", err)
	}

	return runtime
}

func TestBuildContextFormatting(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	var diffCalls [][]string
	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		copyArgs := append([]string(nil), args...)
		diffCalls = append(diffCalls, copyArgs)
		switch strings.Join(args, " ") {
		case "--stat":
			return "diff --stat", "", false
		case "HEAD~1":
			return "fallback diff", "", false
		default:
			return "default diff", "", false
		}
	}

	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"BASE", "HEAD"}
	}

	script := `
		const items = [
			{ type: "note", label: "Important", payload: "Remember" },
			{ type: "diff", payload: "+added" },
			{ type: "diff-error", payload: "error details" },
			{ type: "lazy-diff", label: "stats", payload: "--stat" },
			{ type: "lazy-diff", payload: [] }
		];
		globalThis.__buildResult = exports.buildContext(items, { toTxtar: () => "content\nof\ntxtar" });
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	text := runtime.Get("__buildResult").String()
	if !strings.Contains(text, "### Note: Important") || !strings.Contains(text, "Remember") {
		t.Fatalf("missing note section: %q", text)
	}
	if !strings.Contains(text, "### Diff: git diff") || !strings.Contains(text, "`````diff\n+added") {
		t.Fatalf("missing diff section: %q", text)
	}
	if !strings.Contains(text, "### Diff Error: git diff") || !strings.Contains(text, "error details") {
		t.Fatalf("missing diff error section: %q", text)
	}
	if !strings.Contains(text, "### Diff: stats") || !strings.Contains(text, "diff --stat") {
		t.Fatalf("missing lazy diff section: %q", text)
	}
	if !strings.Contains(text, "### Diff: git diff BASE HEAD") || !strings.Contains(text, "default diff") {
		t.Fatalf("expected fallback diff output to appear: %q", text)
	}
	if !strings.Contains(text, "`````txtar\ncontent\nof\ntxtar\n`````") {
		t.Fatalf("missing txtar block: %q", text)
	}

	if len(diffCalls) != 2 {
		t.Fatalf("expected two git diff calls, got %d", len(diffCalls))
	}
	if got := strings.Join(diffCalls[0], " "); got != "--stat" {
		t.Fatalf("unexpected first diff args: %q", got)
	}
	if got := strings.Join(diffCalls[1], " "); got != "BASE HEAD" {
		t.Fatalf("unexpected fallback diff args: %q", got)
	}
}

func TestBuildContextLazyDiffErrors(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "unexpected", true
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string { return []string{"BASE"} }

	script := `
		const items = [
			{ type: "lazy-diff", payload: ["valid", undefined] },
			{ type: "lazy-diff", payload: 123 },
			{ type: "lazy-diff" }
		];
		globalThis.__errorResult = exports.buildContext(items);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute error script: %v", err)
	}

	text := runtime.Get("__errorResult").String()
	if !strings.Contains(text, "Invalid payload: expected a string array, but found non-string element") {
		t.Fatalf("expected array error: %q", text)
	}
	if !strings.Contains(text, "Invalid payload: expected a string or string array, but got type") {
		t.Fatalf("expected type error: %q", text)
	}
	if !strings.Contains(text, "Error executing git diff: unexpected") {
		t.Fatalf("expected git error message: %q", text)
	}
}

func TestSafeGetString(t *testing.T) {
	t.Parallel()
	runtime := goja.New()
	obj := runtime.NewObject()
	_ = obj.Set("value", "text")
	_ = obj.Set("nullish", goja.Null())

	if got := safeGetString(nil, "any"); got != "" {
		t.Fatalf("expected empty string for nil object, got %q", got)
	}

	if got := safeGetString(obj, "value"); got != "text" {
		t.Fatalf("expected text, got %q", got)
	}
	if got := safeGetString(obj, "missing"); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
	if got := safeGetString(obj, "nullish"); got != "" {
		t.Fatalf("expected empty string for null value, got %q", got)
	}
}

func TestBuildContextItemsSymbol(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	script := `
		const result = exports.buildContext(Symbol("items"));
		globalThis.__symbolResult = result;
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute symbol script: %v", err)
	}

	if got := runtime.Get("__symbolResult").String(); got != "" {
		t.Fatalf("expected empty string result, got %q", got)
	}
}

func TestBuildContextWithGoSlice(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	goItems := []map[string]interface{}{
		{
			"type":    "note",
			"label":   "from Go slice",
			"payload": "payload from Go slice",
		},
	}

	if err := runtime.Set("goItems", goItems); err != nil {
		t.Fatalf("failed to set goItems: %v", err)
	}

	script := `
		globalThis.__goSliceResult = exports.buildContext(globalThis.goItems);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute go slice script: %v", err)
	}

	text := runtime.Get("__goSliceResult").String()
	if !strings.Contains(text, "payload from Go slice") {
		t.Fatalf("expected payload to be present, got %q", text)
	}
	if !strings.Contains(text, "### Note: from Go slice") {
		t.Fatalf("expected label to be present, got %q", text)
	}
}

func TestBuildContextLabelToString(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	script := `
		const labelObj = {
			toString() { return "converted label"; }
		};
		globalThis.__labelResult = exports.buildContext([
			{ type: "note", label: labelObj, payload: "payload" }
		]);
	`

	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute label script: %v", err)
	}

	text := runtime.Get("__labelResult").String()
	if !strings.Contains(text, "### Note: converted label") {
		t.Fatalf("expected converted label in output, got %q", text)
	}
}

func TestBuildContextLazyDiffExportedSlice(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	var diffCalls [][]string
	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		copyArgs := append([]string(nil), args...)
		diffCalls = append(diffCalls, copyArgs)
		return "custom diff", "", false
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT"}
	}

	if err := runtime.Set("__payload", []interface{}{"--stat", "--cached"}); err != nil {
		t.Fatalf("failed to set payload: %v", err)
	}
	if err := runtime.Set("__invalidPayload", []interface{}{"--stat", 42}); err != nil {
		t.Fatalf("failed to set invalid payload: %v", err)
	}

	script := `
		globalThis.__lazyOk = exports.buildContext([
			{ type: "lazy-diff", payload: globalThis.__payload }
		]);
		globalThis.__lazyBad = exports.buildContext([
			{ type: "lazy-diff", payload: globalThis.__invalidPayload }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute lazy-diff script: %v", err)
	}

	if len(diffCalls) != 1 {
		t.Fatalf("expected one git diff call, got %d", len(diffCalls))
	}
	if got := strings.Join(diffCalls[0], " "); got != "--stat --cached" {
		t.Fatalf("unexpected diff args: %q", got)
	}

	if text := runtime.Get("__lazyOk").String(); !strings.Contains(text, "custom diff") {
		t.Fatalf("expected diff output to contain custom diff: %q", text)
	}

	if text := runtime.Get("__lazyBad").String(); !strings.Contains(text, "Invalid payload: expected a string array, but found non-string element at index 1") {
		t.Fatalf("expected invalid payload error, got: %q", text)
	}
}

func TestBuildContext_NoArgs(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	script := `globalThis.__result = exports.buildContext();`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if got := runtime.Get("__result").String(); got != "" {
		t.Fatalf("expected empty string for no-args buildContext, got %q", got)
	}
}

func TestBuildContext_NullUndefinedItems(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	script := `
		globalThis.__nullResult = exports.buildContext(null);
		globalThis.__undefResult = exports.buildContext(undefined);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if got := runtime.Get("__nullResult").String(); got != "" {
		t.Fatalf("expected empty string for null items, got %q", got)
	}
	if got := runtime.Get("__undefResult").String(); got != "" {
		t.Fatalf("expected empty string for undefined items, got %q", got)
	}
}

func TestBuildContext_NonArrayObject(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	// A plain object {} is not an Array — ExportTo to []interface{} should fail.
	script := `globalThis.__result = exports.buildContext({});`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if got := runtime.Get("__result").String(); got != "" {
		t.Fatalf("expected empty string for non-array object, got %q", got)
	}
}

func TestBuildContext_EdgeCases(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	script := `
		const items = [
			// Note without label -> uses default "note" title
			{ type: "note", payload: "unlabeled note" },
			// Item with null type -> skipped
			{ type: null, payload: "null type" },
			// Item with missing type -> skipped
			{ payload: "no type at all" },
			// Null and undefined elements -> skipped
			null,
			undefined
		];
		globalThis.__result = exports.buildContext(items);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "### Note: note") {
		t.Fatalf("expected '### Note: note' for unlabeled note, got:\n%s", text)
	}
	if !strings.Contains(text, "unlabeled note") {
		t.Fatalf("expected unlabeled note content, got:\n%s", text)
	}
	if strings.Contains(text, "null type") || strings.Contains(text, "no type at all") {
		t.Fatalf("expected null/missing type items to be skipped, got:\n%s", text)
	}
}

func TestBuildContext_LazyDiffEmptyStringPayload(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "default fallback diff", "", false
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"FALLBACK_ARG"}
	}

	// Empty string payload → ParseSlice("") → [] → len(args)==0 → default fallback.
	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-diff", payload: "" }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "default fallback diff") {
		t.Fatalf("expected default fallback diff for empty string payload, got:\n%s", text)
	}
	if !strings.Contains(text, "FALLBACK_ARG") {
		t.Fatalf("expected label to contain fallback arg, got:\n%s", text)
	}
}

func TestBuildContext_LazyDiffGoStringSlice(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	var capturedArgs []string
	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		capturedArgs = append([]string(nil), args...)
		return "go-string-slice diff", "", false
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT"}
	}

	// Set a Go []string (not []interface{}) as payload to hit the `case []string:` path.
	if err := runtime.Set("__goStringPayload", []string{"--stat", "HEAD"}); err != nil {
		t.Fatalf("failed to set payload: %v", err)
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-diff", payload: globalThis.__goStringPayload }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "go-string-slice diff") {
		t.Fatalf("expected diff output, got:\n%s", text)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "--stat" || capturedArgs[1] != "HEAD" {
		t.Fatalf("expected captured args [--stat HEAD], got %v", capturedArgs)
	}
}

func TestBuildContext_LazyDiffNilInSlice(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "should not be called", true
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT"}
	}

	// JS array with null element — covers the `goja.IsNull(itemVal)` path in the
	// Array iteration branch of lazy-diff processing.
	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-diff", payload: ["good", null] }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "non-string element at index 1") {
		t.Fatalf("expected error about non-string element at index 1, got:\n%s", text)
	}
}

func TestBuildContext_LazyDiffArrayNonString(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
	})

	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "should not be called", true
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT"}
	}

	// JS array with non-string element — covers the `!ok` branch with `exported != nil`
	// in the arr != nil (Array) iteration path.
	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-diff", payload: [123] }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "non-string element at index 0") {
		t.Fatalf("expected error about non-string element at index 0, got:\n%s", text)
	}
}

func TestBuildContext_TxtarEdgeCases(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	// toTxtar returns empty string → no txtar block should appear
	script := `
		globalThis.__emptyResult = exports.buildContext(
			[{ type: "note", payload: "test" }],
			{ toTxtar: () => "" }
		);
		// toTxtar is not a function → silently ignored
		globalThis.__nonFnResult = exports.buildContext(
			[{ type: "note", payload: "test2" }],
			{ toTxtar: "not a function" }
		);
		// options without toTxtar property → no txtar
		globalThis.__noTxtarResult = exports.buildContext(
			[{ type: "note", payload: "test3" }],
			{ someOtherOption: true }
		);
		// toTxtar throws → silently ignored
		globalThis.__throwResult = exports.buildContext(
			[{ type: "note", payload: "test4" }],
			{ toTxtar: () => { throw new Error("oops"); } }
		);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	for _, tc := range []struct {
		name    string
		varName string
	}{
		{"empty toTxtar", "__emptyResult"},
		{"non-function toTxtar", "__nonFnResult"},
		{"missing toTxtar", "__noTxtarResult"},
		{"throwing toTxtar", "__throwResult"},
	} {
		text := runtime.Get(tc.varName).String()
		if strings.Contains(text, "txtar") {
			t.Errorf("[%s] expected no txtar block, got:\n%s", tc.name, text)
		}
	}
}

func TestRequire_UndefinedExports(t *testing.T) {
	t.Parallel()
	runtime := goja.New()
	module := runtime.NewObject()
	// Intentionally do NOT set "exports" on module — the Require function should
	// create exports internally when it detects undefined.

	loader := Require(context.Background())
	loader(runtime, module)

	exportsVal := module.Get("exports")
	if goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
		t.Fatal("expected exports to be created by Require")
	}

	exports := exportsVal.ToObject(runtime)
	if fn := exports.Get("buildContext"); goja.IsUndefined(fn) || goja.IsNull(fn) {
		t.Fatal("expected buildContext to be defined on auto-created exports")
	}
	if fn := exports.Get("contextManager"); goja.IsUndefined(fn) || goja.IsNull(fn) {
		t.Fatal("expected contextManager to be defined on auto-created exports")
	}
}

func TestBuildContextDynamicFence(t *testing.T) {
	t.Parallel()

	t.Run("Escaping", func(t *testing.T) {
		runtime := setupBuildContext(t)

		originalRun := runGitDiffFn
		t.Cleanup(func() { runGitDiffFn = originalRun })

		runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
			return "diff content with ````` backticks", "", false
		}

		// Content with 5 backticks should result in 6-backtick fence
		script := "const items = [{ type: 'diff', payload: 'diff with ' + '`````' + ' backticks' }]; globalThis.__result = exports.buildContext(items);"
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed to execute script: %v", err)
		}

		text := runtime.Get("__result").String()
		// With 5 backticks in content, fence should be 6
		if !strings.Contains(text, "``````diff\n") {
			t.Fatalf("expected 6-backtick fence for escaping, got: %q", text)
		}
		if !strings.Contains(text, "\n``````\n") {
			t.Fatalf("expected closing 6-backtick fence, got: %q", text)
		}
	})

	t.Run("MinimumLength", func(t *testing.T) {
		runtime := setupBuildContext(t)

		script := `
			const items = [{ type: "diff", payload: "no backticks here" }];
			globalThis.__result = exports.buildContext(items);
		`
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed to execute script: %v", err)
		}

		text := runtime.Get("__result").String()
		if !strings.Contains(text, "`````diff\n") {
			t.Fatalf("expected 5-backtick fence (minimum), got: %q", text)
		}
		if !strings.Contains(text, "\n`````\n") {
			t.Fatalf("expected closing 5-backtick fence, got: %q", text)
		}
	})

	t.Run("Consistency", func(t *testing.T) {
		runtime := setupBuildContext(t)

		// Use string concatenation to create backticks: 4 and 5 backticks respectively
		script := "const items = [" +
			"{ type: 'diff', label: 'first', payload: '```' + '`' }," +
			"{ type: 'diff', label: 'second', payload: '```' + '``' }" +
			"]; globalThis.__result = exports.buildContext(items);"
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed to execute script: %v", err)
		}

		text := runtime.Get("__result").String()

		// Both blocks should use 6-backtick fence
		firstDiffStart := strings.Index(text, "### Diff: first")
		secondDiffStart := strings.Index(text, "### Diff: second")

		if firstDiffStart == -1 || secondDiffStart == -1 {
			t.Fatalf("missing diff sections in output: %q", text)
		}

		// Check first block uses 6 backticks
		firstBlock := text[firstDiffStart:secondDiffStart]
		if !strings.Contains(firstBlock, "``````diff\n") {
			t.Fatalf("expected first block to use 6-backtick fence, got: %q", firstBlock)
		}

		// Check second block uses 6 backticks
		if !strings.Contains(text[secondDiffStart:], "``````diff\n") {
			t.Fatalf("expected second block to use 6-backtick fence, got: %q", text[secondDiffStart:])
		}
	})

	t.Run("TxtarInfluence", func(t *testing.T) {
		runtime := setupBuildContext(t)

		// Use string concatenation to include backticks in toTxtar return value
		script := "const items = [{ type: 'diff', payload: 'simple diff' }]; " +
			"globalThis.__result = exports.buildContext(items, { " +
			"toTxtar: () => 'txtar with ' + '`````' + ' backticks' " +
			"});"
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed to execute script: %v", err)
		}

		text := runtime.Get("__result").String()

		// Both diff and txtar blocks should use 6-backtick fence
		if !strings.Contains(text, "``````diff\n") {
			t.Fatalf("expected diff block to use 6-backtick fence, got: %q", text)
		}
		if !strings.Contains(text, "``````txtar\n") {
			t.Fatalf("expected txtar block to use 6-backtick fence and be labeled, got: %q", text)
		}
	})
}
