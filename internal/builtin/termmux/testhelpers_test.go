package termmux

import (
	"context"
	"io"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// testEnv bundles the runtime, event loop, adapter and module exports for a
// single test.
type testEnv struct {
	ctx     context.Context
	cancel  context.CancelFunc
	loop    *goeventloop.Loop
	adapter *gojaeventloop.Adapter
	runtime *goja.Runtime
	exports *goja.Object
}

// stop cancels the context and shuts down the event loop.
func (e *testEnv) stop() {
	e.cancel()
	_ = e.loop.Shutdown(context.Background())
}

// newTestEnv creates a fresh Goja runtime with EventTarget/CustomEvent globals
// bound and the osm:termmux module loaded. It starts the event loop in a
// goroutine. Callers must run e.stop() before the test returns.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}

	runtime := goja.New()
	registry := require.NewRegistry()
	registry.Enable(runtime)

	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter.Bind: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go loop.Run(ctx)

	registry.RegisterNativeModule("osm:termmux", Require(ctx, adapter, nil, nil))

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}

	return &testEnv{
		ctx:     ctx,
		cancel:  cancel,
		loop:    loop,
		adapter: adapter,
		runtime: runtime,
		exports: v.(*goja.Object),
	}
}

// testRequire is retained for tests that only need module exports and do not
// create a SessionManager. It returns the runtime and module exports and starts
// an event loop which callers must stop by invoking e.stop() on the returned
// testEnv.
func testRequire(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	e := newTestEnv(t)
	t.Cleanup(e.stop)
	return e.runtime, e.exports
}

func newTestEnvCtx(t *testing.T, ctx context.Context) *testEnv {
	t.Helper()

	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}

	runtime := goja.New()
	registry := require.NewRegistry()
	registry.Enable(runtime)

	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter.Bind: %v", err)
	}

	go loop.Run(ctx)

	registry.RegisterNativeModule("osm:termmux", Require(ctx, adapter, nil, nil))

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}

	return &testEnv{
		ctx:     ctx,
		loop:    loop,
		adapter: adapter,
		runtime: runtime,
		exports: v.(*goja.Object),
	}
}

func testRequireCtx(t *testing.T, ctx context.Context) (*goja.Runtime, *goja.Object, *testEnv) {
	t.Helper()
	e := newTestEnvCtx(t, ctx)
	return e.runtime, e.exports, e
}

// wrapTestSessionManager creates a fresh event loop, binds EventTarget/CustomEvent
// to runtime, and wraps mgr with WrapSessionManager. It starts the loop with the
// provided context and registers a cleanup to stop it.
func wrapTestSessionManager(t *testing.T, ctx context.Context, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) goja.Value {
	t.Helper()

	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}

	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter.Bind: %v", err)
	}

	go loop.Run(ctx)
	t.Cleanup(func() { _ = loop.Shutdown(context.Background()) })

	return WrapSessionManager(ctx, adapter, runtime, mgr, stdin, stdout, termFd, title)
}
