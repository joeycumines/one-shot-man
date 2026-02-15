package ctxutil

import (
	"context"
	"testing"

	"github.com/dop251/goja"
)

// FuzzBuildContext ensures buildContext does not panic on arbitrary item shapes.
// Each iteration creates a Goja runtime, constructs items from the fuzzed
// strings, and calls buildContext. The function must not panic regardless of
// what types, labels, or payloads are provided.
func FuzzBuildContext(f *testing.F) {
	// Seed with representative item JSON shapes
	f.Add("note", "label1", "payload1")
	f.Add("file", "/some/path", "")
	f.Add("diff", "git diff", "--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new")
	f.Add("lazy-diff", "git diff HEAD", "")
	f.Add("lazy-exec", "echo hello", "")
	f.Add("", "", "")
	f.Add("unknown-type", "", "some payload")
	f.Add("note", "", "")
	f.Add("diff", "", "")
	f.Add("file", "", "content with\nnewlines\nand ```backticks```")

	f.Fuzz(func(t *testing.T, itemType, label, payload string) {
		runtime := goja.New()
		module := runtime.NewObject()
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		// Stub the git diff function so lazy-diff items don't hit real git
		SetRunGitDiffFn(func(_ context.Context, args []string) (string, string, bool) {
			return "--- stub\n+++ stub\n@@ -1 +1 @@\n-old\n+new\n", "", true
		})
		defer SetRunGitDiffFn(nil)

		SetGetDefaultGitDiffArgsFn(func(_ context.Context) []string {
			return []string{"HEAD"}
		})
		defer SetGetDefaultGitDiffArgsFn(nil)

		loader := Require(context.Background())
		loader(runtime, module)

		if err := runtime.Set("exports", exports); err != nil {
			t.Skip("failed to set exports")
		}

		// Build an item array and call buildContext
		script := `
			const bc = exports.buildContext;
			const items = [{
				type: itemType,
				label: itemLabel,
				payload: itemPayload
			}];
			bc(items, {});
		`

		// Inject the fuzz values as JS variables
		_ = runtime.Set("itemType", itemType)
		_ = runtime.Set("itemLabel", label)
		_ = runtime.Set("itemPayload", payload)

		// Must not panic
		_, _ = runtime.RunString(script)
	})
}
