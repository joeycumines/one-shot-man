package ctxutil

import (
	"context"
	"slices"
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
		copyArgs := slices.Clone(args)
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

	goItems := []map[string]any{
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
		copyArgs := slices.Clone(args)
		diffCalls = append(diffCalls, copyArgs)
		return "custom diff", "", false
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT"}
	}

	if err := runtime.Set("__payload", []any{"--stat", "--cached"}); err != nil {
		t.Fatalf("failed to set payload: %v", err)
	}
	if err := runtime.Set("__invalidPayload", []any{"--stat", 42}); err != nil {
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

	// A plain object {} is not an Array — ExportTo to []any should fail.
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
		capturedArgs = slices.Clone(args)
		return "go-string-slice diff", "", false
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT"}
	}

	// Set a Go []string (not []any) as payload to hit the `case []string:` path.
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

func TestBuildContext_TxtarMetadataOutsideFence(t *testing.T) {
	t.Parallel()
	runtime := setupBuildContext(t)

	t.Run("MetadataExtracted", func(t *testing.T) {
		// Simulate realistic txtar output with metadata in comment section.
		txtarContent := "context root: /Users/dev/project\ncommon path: src/pkg\ntracked directories: src/, tests/\n-- src/pkg/main.go --\npackage main\n"
		script := `
			globalThis.__result = exports.buildContext([], {
				toTxtar: () => ` + "`" + txtarContent + "`" + `
			});
		`
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed: %v", err)
		}

		text := runtime.Get("__result").String()

		// Context root should be OUTSIDE the code fence, with path in backticks.
		if !strings.Contains(text, "context root: `/Users/dev/project`") {
			t.Fatalf("expected context root outside fence with backticked path, got:\n%s", text)
		}
		if !strings.Contains(text, "common path: `src/pkg`") {
			t.Fatalf("expected common path outside fence with backticked value, got:\n%s", text)
		}
		if !strings.Contains(text, "tracked directories: `src/, tests/`") {
			t.Fatalf("expected tracked directories outside fence, got:\n%s", text)
		}

		// Metadata should NOT be inside the txtar code fence.
		fenceStart := strings.Index(text, "`````txtar\n")
		if fenceStart < 0 {
			t.Fatalf("expected txtar code fence, got:\n%s", text)
		}
		fencedContent := text[fenceStart:]
		if strings.Contains(fencedContent, "context root:") {
			t.Fatalf("context root should NOT be inside the code fence, got:\n%s", fencedContent)
		}

		// File entries should still be inside the fence.
		if !strings.Contains(fencedContent, "-- src/pkg/main.go --") {
			t.Fatalf("expected file entries inside fence, got:\n%s", fencedContent)
		}
	})

	t.Run("NoMetadata", func(t *testing.T) {
		// Txtar content WITHOUT metadata should work as before.
		script := `
			globalThis.__result = exports.buildContext([], {
				toTxtar: () => "-- file.go --\npackage main\n"
			});
		`
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed: %v", err)
		}

		text := runtime.Get("__result").String()
		// No metadata rendered above fence.
		if strings.Contains(text, "context root:") {
			t.Fatalf("expected no metadata for content without it, got:\n%s", text)
		}
		// File entry should be in fence.
		if !strings.Contains(text, "`````txtar\n-- file.go --") {
			t.Fatalf("expected file content in fence, got:\n%s", text)
		}
	})

	t.Run("MetadataOnly", func(t *testing.T) {
		// Edge case: only metadata, no file entries.
		script := `
			globalThis.__result = exports.buildContext([], {
				toTxtar: () => "context root: /tmp/test\n"
			});
		`
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed: %v", err)
		}

		text := runtime.Get("__result").String()
		if !strings.Contains(text, "context root: `/tmp/test`") {
			t.Fatalf("expected context root with backticked path, got:\n%s", text)
		}
	})

	t.Run("MetadataWithoutColonSpace", func(t *testing.T) {
		// Edge case: metadata-like line recognized by prefix but lacking ": " separator.
		// E.g. "context root:/no/space" has prefix "context root:" but no ": " — hits else branch.
		script := `
			globalThis.__result = exports.buildContext([], {
				toTxtar: () => "context root:/no/space\n-- file.go --\npackage main\n"
			});
		`
		if _, err := runtime.RunString(script); err != nil {
			t.Fatalf("failed: %v", err)
		}

		text := runtime.Get("__result").String()
		// Without ": " separator, the raw line is emitted as-is (no backtick wrapping).
		if !strings.Contains(text, "context root:/no/space") {
			t.Fatalf("expected raw metadata line without backtick wrapping, got:\n%s", text)
		}
		// Ensure no backtick-wrapped value for this malformed line.
		if strings.Contains(text, "context root: `") {
			t.Fatalf("should NOT backtick-wrap when no ': ' separator, got:\n%s", text)
		}
	})
}

// TestRunGitDiff_NilContext verifies the nil ctx guard in runGitDiff.
func TestRunGitDiff_NilContext(t *testing.T) {
	t.Parallel()
	// runGitDiff with nil context should NOT panic (nil → context.Background()).
	// The actual git command may or may not succeed depending on environment,
	// but what we're testing is that the nil guard prevents a nil-pointer panic.
	// Use typed nil to avoid SA1012 (staticcheck: do not pass a nil Context).
	var nilCtx context.Context
	_, _, _ = runGitDiff(nilCtx, []string{"--stat", "HEAD"})
	// If we reach here, the nil ctx guard worked.
}

// TestGetDefaultGitDiffArgs_NilContext verifies the nil ctx guard in getDefaultGitDiffArgs.
func TestGetDefaultGitDiffArgs_NilContext(t *testing.T) {
	t.Parallel()
	// getDefaultGitDiffArgs with nil context should NOT panic (nil → context.Background()).
	// Use typed nil to avoid SA1012.
	var nilCtx context.Context
	result := getDefaultGitDiffArgs(nilCtx)
	if len(result) == 0 {
		t.Fatal("expected non-empty default args")
	}
}

// TestRunExec_NilContext verifies the nil ctx guard in runExec.
func TestRunExec_NilContext(t *testing.T) {
	t.Parallel()
	// runExec with nil context should NOT panic (nil → context.Background()).
	// Use typed nil to avoid SA1012.
	var nilCtx context.Context
	_, _, _ = runExec(nilCtx, []string{"echo", "test"})
}

// TestRunExec_Basic verifies basic command execution.
func TestRunExec_Basic(t *testing.T) {
	t.Parallel()
	stdout, msg, hadErr := runExec(context.Background(), []string{"go", "version"})
	if hadErr {
		t.Fatalf("unexpected error: %s", msg)
	}
	if !strings.HasPrefix(stdout, "go version") {
		t.Fatalf("expected output starting with 'go version', got %q", stdout)
	}
}

// TestRunExec_CommandNotFound verifies error handling for missing command.
func TestRunExec_CommandNotFound(t *testing.T) {
	t.Parallel()
	_, msg, hadErr := runExec(context.Background(), []string{"nonexistent-command-xyz"})
	if !hadErr {
		t.Fatal("expected error for missing command")
	}
	if msg == "" {
		t.Fatal("expected error message")
	}
}

// TestRunExec_NoCommand verifies error handling for empty args.
func TestRunExec_NoCommand(t *testing.T) {
	t.Parallel()
	_, msg, hadErr := runExec(context.Background(), []string{})
	if !hadErr {
		t.Fatal("expected error for empty command")
	}
	if msg != "exec: no command specified" {
		t.Fatalf("expected 'exec: no command specified', got %q", msg)
	}
}

// TestBuildContext_LazyExec tests the lazy-exec handler with various payload types.
func TestBuildContext_LazyExec(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	var execCalls [][]string
	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		copyArgs := slices.Clone(args)
		execCalls = append(execCalls, copyArgs)
		switch strings.Join(args, " ") {
		case "echo hello":
			return "hello\n", "", false
		case "echo world":
			return "world\n", "", false
		default:
			return "default output\n", "", false
		}
	}

	script := `
		const items = [
			{ type: "lazy-exec", label: "greeting", payload: ["echo", "hello"] },
			{ type: "lazy-exec", payload: ["echo", "world"] }
		];
		globalThis.__buildResult = exports.buildContext(items);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	text := runtime.Get("__buildResult").String()

	// Check Exec output with label
	if !strings.Contains(text, "### Exec: greeting") || !strings.Contains(text, "hello") {
		t.Fatalf("missing exec section with label: %q", text)
	}
	// Check Exec output without label
	if !strings.Contains(text, "### Exec: echo world") || !strings.Contains(text, "world") {
		t.Fatalf("missing exec section without label: %q", text)
	}

	if len(execCalls) != 2 {
		t.Fatalf("expected two exec calls, got %d", len(execCalls))
	}
	if got := strings.Join(execCalls[0], " "); got != "echo hello" {
		t.Fatalf("unexpected first exec args: %q", got)
	}
	if got := strings.Join(execCalls[1], " "); got != "echo world" {
		t.Fatalf("unexpected second exec args: %q", got)
	}
}

// TestBuildContext_LazyExecErrors tests error handling for lazy-exec.
func TestBuildContext_LazyExecErrors(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "command not found", true
	}

	script := `
		const items = [
			{ type: "lazy-exec", payload: ["nonexistent"] },
			{ type: "lazy-exec", payload: ["valid", undefined] },
			{ type: "lazy-exec", payload: 123 },
			{ type: "lazy-exec" }
		];
		globalThis.__errorResult = exports.buildContext(items);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute error script: %v", err)
	}

	text := runtime.Get("__errorResult").String()
	if !strings.Contains(text, "Error executing command: command not found") {
		t.Fatalf("expected command error: %q", text)
	}
	if !strings.Contains(text, "Invalid payload: expected a string array, but found non-string element") {
		t.Fatalf("expected array error: %q", text)
	}
	if !strings.Contains(text, "Invalid payload: expected a string or string array, but got type") {
		t.Fatalf("expected type error: %q", text)
	}
	if !strings.Contains(text, "exec: no command specified") {
		t.Fatalf("expected no command error: %q", text)
	}
}

// TestBuildContext_LazyExecExportedSlice tests lazy-exec with exported Go slices.
func TestBuildContext_LazyExecExportedSlice(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	var capturedArgs []string
	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		capturedArgs = slices.Clone(args)
		return "custom exec output\n", "", false
	}

	if err := runtime.Set("__payload", []any{"echo", "test"}); err != nil {
		t.Fatalf("failed to set payload: %v", err)
	}
	if err := runtime.Set("__invalidPayload", []any{"echo", 42}); err != nil {
		t.Fatalf("failed to set invalid payload: %v", err)
	}

	script := `
		globalThis.__lazyOk = exports.buildContext([
			{ type: "lazy-exec", payload: globalThis.__payload }
		]);
		globalThis.__lazyBad = exports.buildContext([
			{ type: "lazy-exec", payload: globalThis.__invalidPayload }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute lazy-exec script: %v", err)
	}

	if len(capturedArgs) != 2 || capturedArgs[0] != "echo" || capturedArgs[1] != "test" {
		t.Fatalf("expected captured args [echo test], got %v", capturedArgs)
	}

	if text := runtime.Get("__lazyOk").String(); !strings.Contains(text, "custom exec output") {
		t.Fatalf("expected exec output to contain custom exec: %q", text)
	}

	if text := runtime.Get("__lazyBad").String(); !strings.Contains(text, "Invalid payload: expected a string array, but found non-string element at index 1") {
		t.Fatalf("expected invalid payload error, got: %q", text)
	}
}

// TestBuildContext_LazyExecGoStringSlice tests lazy-exec with Go []string.
func TestBuildContext_LazyExecGoStringSlice(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	var capturedArgs []string
	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		capturedArgs = slices.Clone(args)
		return "go-string-slice output\n", "", false
	}

	// Set a Go []string (not []any) as payload to hit the `case []string:` path.
	if err := runtime.Set("__goStringPayload", []string{"echo", "hello"}); err != nil {
		t.Fatalf("failed to set payload: %v", err)
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", payload: globalThis.__goStringPayload }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "go-string-slice output") {
		t.Fatalf("expected exec output, got: %q", text)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "echo" || capturedArgs[1] != "hello" {
		t.Fatalf("expected captured args [echo hello], got %v", capturedArgs)
	}
}

// TestBuildContext_LazyExecStringPayload tests lazy-exec with a string payload (shell-like parsing).
func TestBuildContext_LazyExecStringPayload(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	var capturedArgs []string
	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		capturedArgs = slices.Clone(args)
		return "string payload output\n", "", false
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", label: "test cmd", payload: "echo hello world" }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed to execute script: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "### Exec: test cmd") {
		t.Fatalf("expected '### Exec: test cmd', got: %q", text)
	}
	if !strings.Contains(text, "string payload output") {
		t.Fatalf("expected exec output, got: %q", text)
	}

	// Shell-like parsing should split "echo hello world" into ["echo", "hello", "world"]
	if len(capturedArgs) != 3 || capturedArgs[0] != "echo" || capturedArgs[1] != "hello" || capturedArgs[2] != "world" {
		t.Fatalf("expected captured args [echo hello world], got %v", capturedArgs)
	}
}

// TestBuildContext_LazyExecNilInSlice tests error handling for null elements in JS arrays.
func TestBuildContext_LazyExecNilInSlice(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "should not be called", true
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", payload: ["echo", null] }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "non-string element at index 1") {
		t.Fatalf("expected error about non-string element at index 1, got: %q", text)
	}
}

// TestBuildContext_LazyExecArrayNonString tests error handling for non-string numbers in JS arrays.
func TestBuildContext_LazyExecArrayNonString(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "should not be called", true
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", payload: [123] }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "non-string element at index 0") {
		t.Fatalf("expected error about non-string element at index 0, got: %q", text)
	}
}

// TestBuildContext_LazyExecEmptyLabel uses default label when label is empty.
func TestBuildContext_LazyExecEmptyLabel(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "output\n", "", false
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", label: "", payload: ["echo", "test"] },
			{ type: "lazy-exec", payload: ["echo", "test2"] }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	// When label is empty, should use the command joined with spaces
	if !strings.Contains(text, "### Exec: echo test") {
		t.Fatalf("expected '### Exec: echo test', got: %q", text)
	}
	if !strings.Contains(text, "### Exec: echo test2") {
		t.Fatalf("expected '### Exec: echo test2', got: %q", text)
	}
}

// TestBuildContext_LazyExecStderrCapture verifies stderr is included in error messages.
func TestBuildContext_LazyExecStderrCapture(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		// Simulate a command that fails with stderr
		return "", "permission denied: ./script.sh", true
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", payload: ["./script.sh"] }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "Exec Error") {
		t.Fatalf("expected 'Exec Error' section, got: %q", text)
	}
	if !strings.Contains(text, "Error executing command: permission denied: ./script.sh") {
		t.Fatalf("expected error message with stderr, got: %q", text)
	}
}

// TestBuildContext_LazyExecCombinedWithLazyDiff verifies lazy-exec and lazy-diff can coexist.
func TestBuildContext_LazyExecCombinedWithLazyDiff(t *testing.T) {
	runtime := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalDefault := getDefaultGitDiffArgsFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		getDefaultGitDiffArgsFn = originalDefault
		runExecFn = originalExec
	})

	var diffCalls [][]string
	var execCalls [][]string

	runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
		diffCalls = append(diffCalls, slices.Clone(args))
		return "diff output\n", "", false
	}
	getDefaultGitDiffArgsFn = func(ctx context.Context) []string {
		return []string{"DEFAULT_DIFF"}
	}
	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		execCalls = append(execCalls, slices.Clone(args))
		return "exec output\n", "", false
	}

	script := `
		globalThis.__result = exports.buildContext([
			{ type: "lazy-exec", label: "my cmd", payload: ["echo", "hello"] },
			{ type: "lazy-diff", label: "my diff", payload: ["--stat"] },
			{ type: "note", label: "a note", payload: "some note content" }
		]);
	`
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("failed: %v", err)
	}

	text := runtime.Get("__result").String()
	if !strings.Contains(text, "### Exec: my cmd") || !strings.Contains(text, "exec output") {
		t.Fatalf("missing exec section: %q", text)
	}
	if !strings.Contains(text, "### Diff: my diff") || !strings.Contains(text, "diff output") {
		t.Fatalf("missing diff section: %q", text)
	}
	if !strings.Contains(text, "### Note: a note") || !strings.Contains(text, "some note content") {
		t.Fatalf("missing note section: %q", text)
	}

	if len(execCalls) != 1 {
		t.Fatalf("expected 1 exec call, got %d", len(execCalls))
	}
	if len(diffCalls) != 1 {
		t.Fatalf("expected 1 diff call, got %d", len(diffCalls))
	}
}
