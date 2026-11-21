package bubbletea

import (
	"context"
	"errors"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/stretchr/testify/require"
)

// TestExport_WaitForProgram_RequiresAdapter pins the unset-adapter contract:
// calling the export on a Manager that never received the engine adapter
// surfaces a TypeError instead of blocking the event-loop goroutine in
// WaitForProgram (which would deadlock a program that needs RunSync).
func TestExport_WaitForProgram_RequiresAdapter(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))
	Require(context.Background(), manager)(vm, module)

	exports := module.Get("exports").ToObject(vm)
	fn, ok := goja.AssertFunction(exports.Get("waitForProgram"))
	require.True(t, ok, "waitForProgram must be exported as a function")
	// goja converts the NewTypeError panic into an exception on the call's
	// error return rather than letting it unwind through AssertFunction.
	_, callErr := fn(goja.Undefined())
	require.Error(t, callErr)
	require.Contains(t, callErr.Error(), "waitForProgram: engine adapter is not configured")
}

// TestExport_WaitForProgram_ResolvesWithoutProgram proves the JS-promise
// completion signal end-to-end at the binding level: with the adapter
// configured and no program running, waitForProgram() settles "resolved"
// (the same immediate settlement as Manager.WaitForProgram with a nil slot).
//
// Observation is channel-based, not a poll-after-Submit: Loop.Submit only
// ENQUEUES the callback (go-eventloop schedule.go), so reading a flag on the
// test goroutine right after Submit races the loop and yields nil — a poll
// written that way times out against a product that settles in
// milliseconds. The JS .then therefore calls straight back into Go
// (__wpNotify), which fires on the loop goroutine exactly when the promise
// settles, and the test blocks on that channel.
func TestExport_WaitForProgram_ResolvesWithoutProgram(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	loop, err := goeventloop.New()
	require.NoError(t, err)
	vm := goja.New()
	adapter, err := gojaeventloop.New(loop, vm)
	require.NoError(t, err)
	require.NoError(t, adapter.Bind())
	go loop.Run(ctx)
	defer loop.Shutdown(context.Background())

	manager := newTestManager(ctx, vm)
	manager.SetAdapter(adapter)

	settled := make(chan string, 1)
	require.NoError(t, loop.Submit(func() {
		_ = vm.Set("__wpNotify", func(s string) {
			select {
			case settled <- s:
			default:
			}
		})
		module := vm.NewObject()
		if err := module.Set("exports", vm.NewObject()); err != nil {
			select {
			case settled <- "setup:" + err.Error():
			default:
			}
			return
		}
		Require(ctx, manager)(vm, module)
		exports := module.Get("exports").ToObject(vm)
		exportFn, ok := goja.AssertFunction(exports.Get("waitForProgram"))
		if !ok {
			select {
			case settled <- "setup:waitForProgram is not a function":
			default:
			}
			return
		}
		promiseVal, callErr := exportFn(goja.Undefined())
		if callErr != nil {
			select {
			case settled <- "setup:" + callErr.Error():
			default:
			}
			return
		}
		_ = vm.Set("__wpPromise", promiseVal)
		if _, runErr := vm.RunString(`__wpPromise.then(
			function () { __wpNotify("resolved"); },
			function (e) { __wpNotify("rejected:" + e); }
		);`); runErr != nil {
			select {
			case settled <- "setup:" + runErr.Error():
			default:
			}
		}
	}))

	select {
	case state := <-settled:
		require.Equal(t, "resolved", state)
	case <-time.After(5 * time.Second):
		t.Fatal("waitForProgram did not settle within 5s")
	}
}

// TestWaitForProgram_MultipleConsumersObserveCompletion proves the engine's
// post-script wait and the JS Promise wait can both observe one program result.
func TestWaitForProgram_MultipleConsumersObserveCompletion(t *testing.T) {
	t.Parallel()

	vm := goja.New()
	manager := newTestManager(context.Background(), vm)
	completion := newProgramCompletion()
	manager.mu.Lock()
	manager.programDone = completion
	manager.mu.Unlock()

	wantErr := errors.New("program failed")
	results := make(chan error, 2)
	go func() { results <- manager.WaitForProgram() }()
	go func() { results <- manager.WaitForProgram() }()
	completion.complete(wantErr)

	for range 2 {
		select {
		case err := <-results:
			require.ErrorIs(t, err, wantErr)
		case <-time.After(5 * time.Second):
			t.Fatal("a concurrent WaitForProgram caller did not observe completion")
		}
	}

	require.ErrorIs(t, manager.WaitForProgram(), wantErr, "late callers should observe the same completed run")
}

// TestProgramCompletion_WaitCtxCompletedWinsOverCancelledCtx pins the V5
// fix: when the completion has already fired, waitCtx must return the
// recorded result even if ctx is already cancelled — select would otherwise
// pick the cancelled case ~50% of the time and report context.Canceled for
// a program that already finished.
func TestProgramCompletion_WaitCtxCompletedWinsOverCancelledCtx(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("program exited with failure")
	// Loop many times: the bug is a race in select, so a single iteration
	// would pass even with the old code.
	for i := range 1000 {
		c := newProgramCompletion()
		c.complete(wantErr)
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // already cancelled
		if err := c.waitCtx(ctx); !errors.Is(err, wantErr) {
			t.Fatalf("iteration %d: waitCtx = %v, want the recorded completion error %v", i, err, wantErr)
		}
	}

	// Success case: recorded nil must also win.
	for i := range 1000 {
		c := newProgramCompletion()
		c.complete(nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := c.waitCtx(ctx); err != nil {
			t.Fatalf("iteration %d: waitCtx = %v, want nil for a successful completion", i, err)
		}
	}
}

// TestProgramCompletion_WaitCtxPendingInterruptedByCancel pins the other
// side of the contract: a completion that never fires must still be
// interruptible by ctx cancellation.
func TestProgramCompletion_WaitCtxPendingInterruptedByCancel(t *testing.T) {
	t.Parallel()

	c := newProgramCompletion()
	ctx, cancel := context.WithCancel(context.Background())
	res := make(chan error, 1)
	go func() { res <- c.waitCtx(ctx) }()
	cancel()
	select {
	case err := <-res:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waitCtx = %v, want context.Canceled for a pending completion", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("pending waitCtx was not interrupted by cancellation")
	}
	// The completion must still be observable afterwards.
	wantErr := errors.New("late result")
	c.complete(wantErr)
	if err := c.wait(); !errors.Is(err, wantErr) {
		t.Fatalf("wait after cancel = %v, want %v", err, wantErr)
	}
}
