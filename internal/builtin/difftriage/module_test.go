package difftriage

import (
	"context"
	"fmt"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

func asyncEnvDifftriage(t *testing.T) (*goja.Runtime, func(string) (goja.Value, error)) {
	t.Helper()
	if testing.Short() {
		t.Skip("skip slow")
	}
	rt := goja.New()
	loop, err := goeventloop.New()
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, rt)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}
	mod := rt.NewObject()
	exports := rt.NewObject()
	_ = mod.Set("exports", exports)
	Require(context.Background(), adapter)(rt, mod)
	_ = rt.Set("difftriage", mod.Get("exports"))
	resultCh := make(chan goja.Value, 1)
	errCh := make(chan error, 1)
	_ = rt.Set("__collect", func(call goja.FunctionCall) goja.Value {
		resultCh <- call.Argument(0)
		return goja.Undefined()
	})
	_ = rt.Set("__collectErr", func(call goja.FunctionCall) goja.Value {
		errCh <- fmt.Errorf("%s", call.Argument(0).String())
		return goja.Undefined()
	})
	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)
	t.Cleanup(func() {
		cancel()
		loop.Shutdown(context.Background())
	})
	runJS := func(script string) (goja.Value, error) {
		if err := loop.Submit(func() {
			if _, err := rt.RunString(script); err != nil {
				errCh <- err
			}
		}); err != nil {
			return goja.Undefined(), err
		}
		select {
		case v := <-resultCh:
			return v, nil
		case e := <-errCh:
			return goja.Undefined(), e
		case <-time.After(10 * time.Second):
			return goja.Undefined(), fmt.Errorf("timeout")
		}
	}
	return rt, runJS
}

func TestDifftriageTriageReturnsPromise(t *testing.T) {
	_, runJS := asyncEnvDifftriage(t)
	v, err := runJS(`difftriage.triage("diff --git a/foo.go b/foo.go\n--- a/foo.go\n+++ b/foo.go\n@@ -1 +1 @@\n-foo\n+bar\n").then(function(x){ __collect(JSON.stringify(x)); });`)
	if err != nil {
		t.Fatalf("triage: %v", err)
	}
	if v.String() == "" {
		t.Fatalf("expected json")
	}
}
