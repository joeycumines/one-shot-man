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
