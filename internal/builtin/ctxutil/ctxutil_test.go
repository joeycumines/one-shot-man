package ctxutil

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func setupBuildContext(t *testing.T) (*goja.Runtime, *goeventloop.Loop) {
	t.Helper()

	runtime := goja.New()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	module := runtime.NewObject()
	exports := runtime.NewObject()
	_ = module.Set("exports", exports)

	loader := Require(context.Background(), adapter)
	loader(runtime, module)

	if err := runtime.Set("exports", exports); err != nil {
		t.Fatalf("failed to bind exports: %v", err)
	}

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go loop.Run(loopCtx)
	t.Cleanup(func() {
		loopCancel()
		loop.Shutdown(context.Background())
	})

	return runtime, loop
}

func runAsync(t *testing.T, runtime *goja.Runtime, loop *goeventloop.Loop, script string) {
	t.Helper()
	done := make(chan error, 1)

	err := loop.Submit(func() {
		_ = runtime.Set("__signalDone", func() {
			done <- nil
		})
		_, err := runtime.RunString(script)
		if err != nil {
			done <- err
		}
	})
	if err != nil {
		t.Fatalf("failed to submit script: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("script error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout waiting for async script")
	}
}

func setVarSync(t *testing.T, runtime *goja.Runtime, loop *goeventloop.Loop, name string, value any) {
	t.Helper()
	done := make(chan struct{})
	err := loop.Submit(func() {
		_ = runtime.Set(name, value)
		close(done)
	})
	if err != nil {
		t.Fatalf("failed to set %s: %v", name, err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout setting %s", name)
	}
}

func getVarSync(t *testing.T, runtime *goja.Runtime, loop *goeventloop.Loop, name string) string {
	t.Helper()
	ch := make(chan string, 1)
	err := loop.Submit(func() {
		val := runtime.Get(name)
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			ch <- ""
		} else {
			ch <- val.String()
		}
	})
	if err != nil {
		t.Fatalf("failed to get %s: %v", name, err)
	}
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout getting %s", name)
		return ""
	}
}

func TestBuildContextFormatting(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			const items = [
				{ type: "note", label: "Important", payload: "Remember" },
				{ type: "diff", payload: "+added" },
				{ type: "diff-error", payload: "error details" },
				{ type: "lazy-diff", label: "stats", payload: "--stat" },
				{ type: "lazy-diff", payload: [] }
			];
			globalThis.__buildResult = await exports.buildContext(items, { toTxtar: () => "content\nof\ntxtar" });
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__buildResult")
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
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			const items = [
				{ type: "lazy-diff", payload: ["valid", undefined] },
				{ type: "lazy-diff", payload: 123 },
				{ type: "lazy-diff" }
			];
			globalThis.__errorResult = await exports.buildContext(items);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__errorResult")
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

func TestBuildContextItemsSymbol(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `
		(async function() {
			const result = await exports.buildContext(Symbol("items"));
			globalThis.__symbolResult = result;
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	if got := getVarSync(t, runtime, loop, "__symbolResult"); got != "" {
		t.Fatalf("expected empty string result, got %q", got)
	}
}

func TestBuildContextWithGoSlice(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	goItems := []map[string]any{
		{
			"type":    "note",
			"label":   "from Go slice",
			"payload": "payload from Go slice",
		},
	}

	setVarSync(t, runtime, loop, "goItems", goItems)

	script := `
		(async function() {
			globalThis.__goSliceResult = await exports.buildContext(globalThis.goItems);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__goSliceResult")
	if !strings.Contains(text, "payload from Go slice") {
		t.Fatalf("expected payload to be present, got %q", text)
	}
	if !strings.Contains(text, "### Note: from Go slice") {
		t.Fatalf("expected label to be present, got %q", text)
	}
}

func TestBuildContextLabelToString(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `
		(async function() {
			const labelObj = {
				toString() { return "converted label"; }
			};
			globalThis.__labelResult = await exports.buildContext([
				{ type: "note", label: labelObj, payload: "payload" }
			]);
			__signalDone();
		})();
	`

	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__labelResult")
	if !strings.Contains(text, "### Note: converted label") {
		t.Fatalf("expected converted label in output, got %q", text)
	}
}

func TestBuildContextLazyDiffExportedSlice(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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

	setVarSync(t, runtime, loop, "__payload", []any{"--stat", "--cached"})
	setVarSync(t, runtime, loop, "__invalidPayload", []any{"--stat", 42})

	script := `
		(async function() {
			globalThis.__lazyOk = await exports.buildContext([
				{ type: "lazy-diff", payload: globalThis.__payload }
			]);
			globalThis.__lazyBad = await exports.buildContext([
				{ type: "lazy-diff", payload: globalThis.__invalidPayload }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	if len(diffCalls) != 1 {
		t.Fatalf("expected one git diff call, got %d", len(diffCalls))
	}
	if got := strings.Join(diffCalls[0], " "); got != "--stat --cached" {
		t.Fatalf("unexpected diff args: %q", got)
	}

	if text := getVarSync(t, runtime, loop, "__lazyOk"); !strings.Contains(text, "custom diff") {
		t.Fatalf("expected diff output to contain custom diff: %q", text)
	}

	if text := getVarSync(t, runtime, loop, "__lazyBad"); !strings.Contains(text, "Invalid payload: expected a string array, but found non-string element at index 1") {
		t.Fatalf("expected invalid payload error, got: %q", text)
	}
}

func TestBuildContext_NoArgs(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `(async function() { globalThis.__result = await exports.buildContext(); __signalDone(); })();`
	runAsync(t, runtime, loop, script)
	if got := getVarSync(t, runtime, loop, "__result"); got != "" {
		t.Fatalf("expected empty string for no-args buildContext, got %q", got)
	}
}

func TestBuildContext_NullUndefinedItems(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `
		(async function() {
			globalThis.__nullResult = await exports.buildContext(null);
			globalThis.__undefResult = await exports.buildContext(undefined);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)
	if got := getVarSync(t, runtime, loop, "__nullResult"); got != "" {
		t.Fatalf("expected empty string for null items, got %q", got)
	}
	if got := getVarSync(t, runtime, loop, "__undefResult"); got != "" {
		t.Fatalf("expected empty string for undefined items, got %q", got)
	}
}

func TestBuildContext_NonArrayObject(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `(async function() { globalThis.__result = await exports.buildContext({}); __signalDone(); })();`
	runAsync(t, runtime, loop, script)
	if got := getVarSync(t, runtime, loop, "__result"); got != "" {
		t.Fatalf("expected empty string for non-array object, got %q", got)
	}
}

func TestBuildContext_EdgeCases(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `
		(async function() {
			const items = [
				{ type: "note", payload: "unlabeled note" },
				{ type: null, payload: "null type" },
				{ payload: "no type at all" },
				null,
				undefined
			];
			globalThis.__result = await exports.buildContext(items);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
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
	runtime, loop := setupBuildContext(t)

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

	script := `
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-diff", payload: "" }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "default fallback diff") {
		t.Fatalf("expected default fallback diff for empty string payload, got:\n%s", text)
	}
	if !strings.Contains(text, "FALLBACK_ARG") {
		t.Fatalf("expected label to contain fallback arg, got:\n%s", text)
	}
}

func TestBuildContext_LazyDiffGoStringSlice(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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

	setVarSync(t, runtime, loop, "__goStringPayload", []string{"--stat", "HEAD"})

	script := `
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-diff", payload: globalThis.__goStringPayload }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "go-string-slice diff") {
		t.Fatalf("expected diff output, got:\n%s", text)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "--stat" || capturedArgs[1] != "HEAD" {
		t.Fatalf("expected captured args [--stat HEAD], got %v", capturedArgs)
	}
}

func TestBuildContext_LazyDiffNilInSlice(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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

	script := `
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-diff", payload: ["good", null] }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "non-string element at index 1") {
		t.Fatalf("expected error about non-string element at index 1, got:\n%s", text)
	}
}

func TestBuildContext_LazyDiffArrayNonString(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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

	script := `
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-diff", payload: [123] }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "non-string element at index 0") {
		t.Fatalf("expected error about non-string element at index 0, got:\n%s", text)
	}
}

func TestBuildContext_TxtarEdgeCases(t *testing.T) {
	t.Parallel()
	runtime, loop := setupBuildContext(t)

	script := `
		(async function() {
			globalThis.__emptyResult = await exports.buildContext(
				[{ type: "note", payload: "test" }],
				{ toTxtar: () => "" }
			);
			globalThis.__nonFnResult = await exports.buildContext(
				[{ type: "note", payload: "test2" }],
				{ toTxtar: "not a function" }
			);
			globalThis.__noTxtarResult = await exports.buildContext(
				[{ type: "note", payload: "test3" }],
				{ someOtherOption: true }
			);
			globalThis.__throwResult = await exports.buildContext(
				[{ type: "note", payload: "test4" }],
				{ toTxtar: () => { throw new Error("oops"); } }
			);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	for _, tc := range []struct {
		name    string
		varName string
	}{
		{"empty toTxtar", "__emptyResult"},
		{"non-function toTxtar", "__nonFnResult"},
		{"missing toTxtar", "__noTxtarResult"},
		{"throwing toTxtar", "__throwResult"},
	} {
		text := getVarSync(t, runtime, loop, tc.varName)
		if strings.Contains(text, "txtar") {
			t.Errorf("[%s] expected no txtar block, got:\n%s", tc.name, text)
		}
	}
}

func TestRequire_UndefinedExports(t *testing.T) {
	t.Parallel()
	runtime := goja.New()
	module := runtime.NewObject()

	loader := Require(context.Background(), nil)
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
		runtime, loop := setupBuildContext(t)

		originalRun := runGitDiffFn
		t.Cleanup(func() { runGitDiffFn = originalRun })

		runGitDiffFn = func(ctx context.Context, args []string) (string, string, bool) {
			return "diff content with ````` backticks", "", false
		}

		script := "(async function() { const items = [{ type: 'diff', payload: 'diff with ' + '`````' + ' backticks' }]; globalThis.__result = await exports.buildContext(items); __signalDone(); })();"
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")
		if !strings.Contains(text, "``````diff\n") {
			t.Fatalf("expected 6-backtick fence for escaping, got: %q", text)
		}
		if !strings.Contains(text, "\n``````\n") {
			t.Fatalf("expected closing 6-backtick fence, got: %q", text)
		}
	})

	t.Run("MinimumLength", func(t *testing.T) {
		runtime, loop := setupBuildContext(t)

		script := `
			(async function() {
				const items = [{ type: "diff", payload: "no backticks here" }];
				globalThis.__result = await exports.buildContext(items);
				__signalDone();
			})();
		`
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")
		if !strings.Contains(text, "`````diff\n") {
			t.Fatalf("expected 5-backtick fence (minimum), got: %q", text)
		}
		if !strings.Contains(text, "\n`````\n") {
			t.Fatalf("expected closing 5-backtick fence, got: %q", text)
		}
	})

	t.Run("Consistency", func(t *testing.T) {
		runtime, loop := setupBuildContext(t)

		script := "(async function() { const items = [" +
			"{ type: 'diff', label: 'first', payload: '```' + '`' }," +
			"{ type: 'diff', label: 'second', payload: '```' + '``' }" +
			"]; globalThis.__result = await exports.buildContext(items); __signalDone(); })();"
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")

		firstDiffStart := strings.Index(text, "### Diff: first")
		secondDiffStart := strings.Index(text, "### Diff: second")

		if firstDiffStart == -1 || secondDiffStart == -1 {
			t.Fatalf("missing diff sections in output: %q", text)
		}

		firstBlock := text[firstDiffStart:secondDiffStart]
		if !strings.Contains(firstBlock, "``````diff\n") {
			t.Fatalf("expected first block to use 6-backtick fence, got: %q", firstBlock)
		}

		if !strings.Contains(text[secondDiffStart:], "``````diff\n") {
			t.Fatalf("expected second block to use 6-backtick fence, got: %q", text[secondDiffStart:])
		}
	})

	t.Run("TxtarInfluence", func(t *testing.T) {
		runtime, loop := setupBuildContext(t)

		script := "(async function() { const items = [{ type: 'diff', payload: 'simple diff' }]; " +
			"globalThis.__result = await exports.buildContext(items, { " +
			"toTxtar: () => 'txtar with ' + '`````' + ' backticks' " +
			"}); __signalDone(); })();"
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")

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
	runtime, loop := setupBuildContext(t)

	t.Run("MetadataExtracted", func(t *testing.T) {
		txtarContent := "context root: /Users/dev/project\ncommon path: src/pkg\ntracked directories: src/, tests/\n-- src/pkg/main.go --\npackage main\n"
		script := `
			(async function() {
				globalThis.__result = await exports.buildContext([], {
					toTxtar: () => ` + "`" + txtarContent + "`" + `
				});
				__signalDone();
			})();
		`
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")

		if !strings.Contains(text, "context root: `/Users/dev/project`") {
			t.Fatalf("expected context root outside fence with backticked path, got:\n%s", text)
		}
		if !strings.Contains(text, "common path: `src/pkg`") {
			t.Fatalf("expected common path outside fence with backticked value, got:\n%s", text)
		}
		if !strings.Contains(text, "tracked directories: `src/, tests/`") {
			t.Fatalf("expected tracked directories outside fence, got:\n%s", text)
		}

		fenceStart := strings.Index(text, "`````txtar\n")
		if fenceStart < 0 {
			t.Fatalf("expected txtar code fence, got:\n%s", text)
		}
		fencedContent := text[fenceStart:]
		if strings.Contains(fencedContent, "context root:") {
			t.Fatalf("context root should NOT be inside the code fence, got:\n%s", fencedContent)
		}

		if !strings.Contains(fencedContent, "-- src/pkg/main.go --") {
			t.Fatalf("expected file entries inside fence, got:\n%s", fencedContent)
		}
	})

	t.Run("NoMetadata", func(t *testing.T) {
		script := `
			(async function() {
				globalThis.__result = await exports.buildContext([], {
					toTxtar: () => "-- file.go --\npackage main\n"
				});
				__signalDone();
			})();
		`
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")
		if strings.Contains(text, "context root:") {
			t.Fatalf("expected no metadata for content without it, got:\n%s", text)
		}
		if !strings.Contains(text, "`````txtar\n-- file.go --") {
			t.Fatalf("expected file content in fence, got:\n%s", text)
		}
	})

	t.Run("MetadataOnly", func(t *testing.T) {
		script := `
			(async function() {
				globalThis.__result = await exports.buildContext([], {
					toTxtar: () => "context root: /tmp/test\n"
				});
				__signalDone();
			})();
		`
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")
		if !strings.Contains(text, "context root: `/tmp/test`") {
			t.Fatalf("expected context root with backticked path, got:\n%s", text)
		}
	})

	t.Run("MetadataWithoutColonSpace", func(t *testing.T) {
		script := `
			(async function() {
				globalThis.__result = await exports.buildContext([], {
					toTxtar: () => "context root:/no/space\n-- file.go --\npackage main\n"
				});
				__signalDone();
			})();
		`
		runAsync(t, runtime, loop, script)

		text := getVarSync(t, runtime, loop, "__result")
		if !strings.Contains(text, "context root:/no/space") {
			t.Fatalf("expected raw metadata line without backtick wrapping, got:\n%s", text)
		}
		if strings.Contains(text, "context root: `") {
			t.Fatalf("should NOT backtick-wrap when no ': ' separator, got:\n%s", text)
		}
	})
}

func TestRunGitDiff_NilContext(t *testing.T) {
	t.Parallel()
	var nilCtx context.Context
	_, _, _ = runGitDiff(nilCtx, []string{"--stat", "HEAD"})
}

func TestGetDefaultGitDiffArgs_NilContext(t *testing.T) {
	t.Parallel()
	var nilCtx context.Context
	result := getDefaultGitDiffArgs(nilCtx)
	if len(result) == 0 {
		t.Fatal("expected non-empty default args")
	}
}

func TestRunExec_NilContext(t *testing.T) {
	t.Parallel()
	var nilCtx context.Context
	_, _, _ = runExec(nilCtx, []string{"echo", "test"})
}

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

func TestBuildContext_LazyExec(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			const items = [
				{ type: "lazy-exec", label: "greeting", payload: ["echo", "hello"] },
				{ type: "lazy-exec", payload: ["echo", "world"] }
			];
			globalThis.__buildResult = await exports.buildContext(items);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__buildResult")

	if !strings.Contains(text, "### Exec: greeting") || !strings.Contains(text, "hello") {
		t.Fatalf("missing exec section with label: %q", text)
	}
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

func TestBuildContext_LazyExecErrors(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			const items = [
				{ type: "lazy-exec", payload: ["nonexistent"] },
				{ type: "lazy-exec", payload: ["valid", undefined] },
				{ type: "lazy-exec", payload: 123 },
				{ type: "lazy-exec" }
			];
			globalThis.__errorResult = await exports.buildContext(items);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__errorResult")
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

func TestBuildContext_LazyExecExportedSlice(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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

	setVarSync(t, runtime, loop, "__payload", []any{"echo", "test"})
	setVarSync(t, runtime, loop, "__invalidPayload", []any{"echo", 42})

	script := `
		(async function() {
			globalThis.__lazyOk = await exports.buildContext([
				{ type: "lazy-exec", payload: globalThis.__payload }
			]);
			globalThis.__lazyBad = await exports.buildContext([
				{ type: "lazy-exec", payload: globalThis.__invalidPayload }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	if len(capturedArgs) != 2 || capturedArgs[0] != "echo" || capturedArgs[1] != "test" {
		t.Fatalf("expected captured args [echo test], got %v", capturedArgs)
	}

	if text := getVarSync(t, runtime, loop, "__lazyOk"); !strings.Contains(text, "custom exec output") {
		t.Fatalf("expected exec output to contain custom exec: %q", text)
	}

	if text := getVarSync(t, runtime, loop, "__lazyBad"); !strings.Contains(text, "Invalid payload: expected a string array, but found non-string element at index 1") {
		t.Fatalf("expected invalid payload error, got: %q", text)
	}
}

func TestBuildContext_LazyExecGoStringSlice(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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

	setVarSync(t, runtime, loop, "__goStringPayload", []string{"echo", "hello"})

	script := `
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", payload: globalThis.__goStringPayload }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "go-string-slice output") {
		t.Fatalf("expected exec output, got: %q", text)
	}
	if len(capturedArgs) != 2 || capturedArgs[0] != "echo" || capturedArgs[1] != "hello" {
		t.Fatalf("expected captured args [echo hello], got %v", capturedArgs)
	}
}

func TestBuildContext_LazyExecStringPayload(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", label: "test cmd", payload: "echo hello world" }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "### Exec: test cmd") {
		t.Fatalf("expected '### Exec: test cmd', got: %q", text)
	}
	if !strings.Contains(text, "string payload output") {
		t.Fatalf("expected exec output, got: %q", text)
	}

	if len(capturedArgs) != 3 || capturedArgs[0] != "echo" || capturedArgs[1] != "hello" || capturedArgs[2] != "world" {
		t.Fatalf("expected captured args [echo hello world], got %v", capturedArgs)
	}
}

func TestBuildContext_LazyExecNilInSlice(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", payload: ["echo", null] }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "non-string element at index 1") {
		t.Fatalf("expected error about non-string element at index 1, got: %q", text)
	}
}

func TestBuildContext_LazyExecArrayNonString(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", payload: [123] }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "non-string element at index 0") {
		t.Fatalf("expected error about non-string element at index 0, got: %q", text)
	}
}

func TestBuildContext_LazyExecEmptyLabel(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", label: "", payload: ["echo", "test"] },
				{ type: "lazy-exec", payload: ["echo", "test2"] }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "### Exec: echo test") {
		t.Fatalf("expected '### Exec: echo test', got: %q", text)
	}
	if !strings.Contains(text, "### Exec: echo test2") {
		t.Fatalf("expected '### Exec: echo test2', got: %q", text)
	}
}

func TestBuildContext_LazyExecStderrCapture(t *testing.T) {
	runtime, loop := setupBuildContext(t)

	originalRun := runGitDiffFn
	originalExec := runExecFn
	t.Cleanup(func() {
		runGitDiffFn = originalRun
		runExecFn = originalExec
	})

	runExecFn = func(ctx context.Context, args []string) (string, string, bool) {
		return "", "permission denied: ./script.sh", true
	}

	script := `
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", payload: ["./script.sh"] }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
	if !strings.Contains(text, "Exec Error") {
		t.Fatalf("expected 'Exec Error' section, got: %q", text)
	}
	if !strings.Contains(text, "Error executing command: permission denied: ./script.sh") {
		t.Fatalf("expected error message with stderr, got: %q", text)
	}
}

func TestBuildContext_LazyExecCombinedWithLazyDiff(t *testing.T) {
	runtime, loop := setupBuildContext(t)

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
		(async function() {
			globalThis.__result = await exports.buildContext([
				{ type: "lazy-exec", label: "my cmd", payload: ["echo", "hello"] },
				{ type: "lazy-diff", label: "my diff", payload: ["--stat"] },
				{ type: "note", label: "a note", payload: "some note content" }
			]);
			__signalDone();
		})();
	`
	runAsync(t, runtime, loop, script)

	text := getVarSync(t, runtime, loop, "__result")
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
