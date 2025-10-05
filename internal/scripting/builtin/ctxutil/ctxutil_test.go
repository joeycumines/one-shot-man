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
			{ type: "lazy-diff", payload: ["HEAD~1"] }
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
	if !strings.Contains(text, "### Diff: git diff") || !strings.Contains(text, "```diff\n+added") {
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
	if !strings.Contains(text, "```\ncontent\nof\ntxtar\n```") {
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
