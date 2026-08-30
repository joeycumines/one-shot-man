package termmux

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goja_nodejs/require"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

var testLoops sync.Map

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
	if e == nil {
		return
	}
	if e.cancel != nil {
		e.cancel()
	}
	if e.loop != nil {
		_ = e.loop.Shutdown(context.Background())
	}
}

// newTestEnv creates a fresh Goja runtime with EventTarget/CustomEvent globals
// bound and the osm:termmux module loaded. The event loop runs so Promises
// settle; scripts must execute via runJS/awaitJS helpers (or runOnEnvLoop).
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	loop, err := goeventloop.New()
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

	registry.RegisterNativeModule("osm:termmux", Require(ctx, adapter, loop, nil, nil))

	v, err := runtime.RunString(`require('osm:termmux')`)
	if err != nil {
		t.Fatalf("require osm:termmux: %v", err)
	}

	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()
	testLoops.Store(runtime, loop)

	t.Cleanup(func() {
		testLoops.Delete(runtime)
		cancel()
		_ = loop.Shutdown(context.Background())
		<-loopDone
	})

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

	loop, err := goeventloop.New()
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

	registry.RegisterNativeModule("osm:termmux", Require(ctx, adapter, loop, nil, nil))

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

// testRequireLooped is testRequire with a running event loop registered in
// testLoops, for tests whose scripts must await promises.
func testRequireLooped(t *testing.T) (*goja.Runtime, *goja.Object) {
	t.Helper()
	e := newTestEnv(t)
	loopDone := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer close(loopDone)
		_ = e.loop.Run(ctx)
	}()
	testLoops.Store(e.runtime, e.loop)
	t.Cleanup(func() {
		cancel()
		testLoops.Delete(e.runtime)
		e.stop()
		<-loopDone
	})
	return e.runtime, e.exports
}

// wrapTestSessionManager creates a fresh event loop, binds EventTarget/CustomEvent
// to runtime, and wraps mgr with WrapSessionManager. The event loop is NOT
// started: tests call runtime.RunString() directly on the test goroutine.
// Starting the loop would cause a data race because the event bridge goroutine
// dispatches SessionManager events to the loop, accessing the Goja runtime
// concurrently with runtime.RunString(). The loop is still created so
// adapter Submit calls queue without panicking; they are drained on
// shutdown.
func wrapTestSessionManager(t *testing.T, ctx context.Context, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) goja.Value {
	t.Helper()

	loop, err := goeventloop.New()
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

	wrapper := WrapSessionManager(ctx, adapter, loop, runtime, mgr, stdin, stdout, termFd, title)

	t.Cleanup(func() {
		_ = loop.Shutdown(context.Background())
	})

	return wrapper
}

// wrapTestSessionManagerWithLoop is like wrapTestSessionManager but also starts
// the event loop goroutine and registers the loop in testLoops so runJS can
// dispatch scripts onto it. Use this for tests that need the event bridge to
// deliver SessionManager events to JS listeners. All runtime.RunString calls
// in such tests MUST go through runJS to avoid data races.
func wrapTestSessionManagerWithLoop(t *testing.T, ctx context.Context, runtime *goja.Runtime, mgr *parent.SessionManager, stdin io.Reader, stdout io.Writer, termFd int, title string) goja.Value {
	t.Helper()

	loop, err := goeventloop.New()
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

	wrapper := WrapSessionManager(ctx, adapter, loop, runtime, mgr, stdin, stdout, termFd, title)

	testLoops.Store(runtime, loop)
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		_ = loop.Run(ctx)
	}()
	t.Cleanup(func() {
		testLoops.Delete(runtime)
		_ = loop.Shutdown(context.Background())
		<-loopDone
	})

	return wrapper
}

// runJS executes a JS string on the event loop goroutine associated with the
// given runtime, waiting for the result. This avoids data races between
// runtime.RunString on the test goroutine and event dispatch on the event loop
// goroutine. The runtime must have been registered via wrapTestSessionManagerWithLoop.
func runJS(t *testing.T, runtime *goja.Runtime, script string) (goja.Value, error) {
	t.Helper()
	loop := loopForRuntime(t, runtime)
	type result struct {
		v   goja.Value
		err error
	}
	ch := make(chan result, 1)
	if err := loop.Submit(func() {
		v, err := runtime.RunString(script)
		ch <- result{v, err}
	}); err != nil {
		t.Fatalf("submit script to event loop: %v", err)
	}
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(30 * time.Second):
		t.Fatalf("runJS timed out")
		return nil, nil
	}
}

// awaitJS runs an async JS snippet (may use await) on the event loop
// goroutine associated with the given runtime and waits for it to settle.
// The runtime must have been registered via wrapTestSessionManagerWithLoop.
func awaitJS(t *testing.T, runtime *goja.Runtime, script string) {
	t.Helper()
	loop := loopForRuntime(t, runtime)
	errCh := make(chan error, 1)
	if err := loop.Submit(func() {
		_ = runtime.Set("__awaitJSDone", func() { errCh <- nil })
		_ = runtime.Set("__awaitJSFail", func(msg string) { errCh <- errors.New(msg) })
		wrapped := `(async function() { ` + script + ` })()
		.then(function() { __awaitJSDone(); })
		.catch(function(e) { __awaitJSFail(e && e.message ? e.message : String(e)); });`
		if _, runErr := runtime.RunString(wrapped); runErr != nil {
			errCh <- runErr
		}
	}); err != nil {
		t.Fatalf("submit script to event loop: %v", err)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("awaitJS: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("awaitJS timed out")
	}
}

// loopForRuntime returns the event loop registered for the given runtime.
// The runtime must have been registered via wrapTestSessionManagerWithLoop.
func loopForRuntime(t *testing.T, runtime *goja.Runtime) *goeventloop.Loop {
	t.Helper()
	loopVal, ok := testLoops.Load(runtime)
	if !ok {
		t.Fatalf("no event loop found for runtime")
	}
	return loopVal.(*goeventloop.Loop)
}

// awaitJSValue runs an async JS snippet (may use await; its returned value is
// captured) on the event loop goroutine for the given runtime and waits for
// settlement. The runtime must have been registered via
// wrapTestSessionManagerWithLoop or setupTmuxModule.
func awaitJSValue(t *testing.T, runtime *goja.Runtime, script string) (goja.Value, error) {
	t.Helper()
	loop := loopForRuntime(t, runtime)
	type result struct {
		v   goja.Value
		err error
	}
	ch := make(chan result, 1)
	if err := loop.Submit(func() {
		_ = runtime.Set("__awaitJSVal", func(v goja.Value) { ch <- result{v: v} })
		_ = runtime.Set("__awaitJSFail", func(msg string) { ch <- result{err: errors.New(msg)} })
		wrapped := `(async function() { ` + script + ` })()
		.then(function(v) { __awaitJSVal(v); }, function(e) { __awaitJSFail(e && e.message ? e.message : String(e)); });`
		if _, runErr := runtime.RunString(wrapped); runErr != nil {
			ch <- result{err: runErr}
		}
	}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(30 * time.Second):
		return nil, errors.New("awaitJSValue timed out")
	}
}

// awaitJSErr runs an async JS snippet on the event loop goroutine for the
// given runtime and returns any settlement error.
func awaitJSErr(t *testing.T, runtime *goja.Runtime, script string) error {
	_, err := awaitJSValue(t, runtime, script)
	return err
}

// setOnLoop sets a global on the event loop goroutine. Required whenever the
// runtime's loop is running: direct runtime access from the test goroutine
// races with bridge-dispatched event callbacks.
func setOnLoop(t *testing.T, runtime *goja.Runtime, name string, value any) {
	t.Helper()
	loop := loopForRuntime(t, runtime)
	done := make(chan struct{})
	if err := loop.Submit(func() {
		defer close(done)
		_ = runtime.Set(name, value)
	}); err != nil {
		t.Fatalf("submit set to event loop: %v", err)
	}
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("setOnLoop(%s) timed out", name)
	}
}


