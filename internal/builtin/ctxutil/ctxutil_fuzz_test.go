package ctxutil

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func FuzzBuildContext(f *testing.F) {
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

		loop, err := goeventloop.New(
			goeventloop.WithStrictMicrotaskOrdering(true),
		)
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

		SetRunGitDiffFn(func(_ context.Context, args []string) (string, string, bool) {
			return "--- stub\n+++ stub\n@@ -1 +1 @@\n-old\n+new\n", "", true
		})
		defer SetRunGitDiffFn(nil)

		SetGetDefaultGitDiffArgsFn(func(_ context.Context) []string {
			return []string{"HEAD"}
		})
		defer SetGetDefaultGitDiffArgsFn(nil)

		module := runtime.NewObject()
		exports := runtime.NewObject()
		_ = module.Set("exports", exports)

		loader := Require(context.Background(), adapter)
		loader(runtime, module)

		if err := runtime.Set("exports", exports); err != nil {
			t.Skip("failed to set exports")
		}

		_ = runtime.Set("itemType", itemType)
		_ = runtime.Set("itemLabel", label)
		_ = runtime.Set("itemPayload", payload)

		script := `
			(async function() {
				const bc = exports.buildContext;
				const items = [{
					type: itemType,
					label: itemLabel,
					payload: itemPayload
				}];
				await bc(items, {});
				__signalDone();
			})();
		`

		loopCtx, loopCancel := context.WithCancel(context.Background())
		go loop.Run(loopCtx)
		defer func() {
			loopCancel()
			loop.Shutdown(context.Background())
		}()

		done := make(chan error, 1)
		err = loop.Submit(func() {
			_ = runtime.Set("__signalDone", func() {
				done <- nil
			})
			_, runErr := runtime.RunString(script)
			if runErr != nil {
				done <- runErr
			}
		})
		if err != nil {
			t.Fatalf("failed to submit script: %v", err)
		}
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatalf("timeout waiting for async script")
		}
	})
}
