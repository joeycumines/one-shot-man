package async

import (
	"context"
	"errors"
	"log/slog"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// PromiseTracked runs fn off the event loop via loop.Promisify and returns a
// native goja Promise bridged from the tracked settlement. Unlike an untracked
// goroutine, the work participates in loop liveness (auto-exit cannot fire
// mid-I/O), graceful Shutdown waits for it via promisifyWg, Goexit and panics
// are canonicalized, and settlement is guaranteed or rejected with
// ErrLoopTerminated. The adapter must be bound and PromiseTracked called on
// the runtime owner; fn receives baseCtx for cancellation. Errors from fn are
// rejected as GoErrors (so JS sees error.message). mapResult, when non-nil,
// runs on the runtime owner to convert the fulfilled Go result into the JS
// value; when nil, a nil result maps to undefined.
func PromiseTracked(
	adapter *gojaeventloop.Adapter,
	loop *goeventloop.Loop,
	baseCtx context.Context,
	fn func(ctx context.Context) (any, error),
	mapResult func(rt *goja.Runtime, result any) any,
) goja.Value {
	if adapter == nil {
		panic("async: adapter is nil")
	}
	if loop == nil {
		panic("async: loop is nil")
	}
	promise, settler := adapter.NewPromise()
	tracked := loop.Promisify(baseCtx, fn)
	go func() {
		result, ok := <-tracked.ToChannel()
		if !ok {
			return
		}
		if err, isErr := result.(error); isErr {
			if err := settler.Reject(func(rt *goja.Runtime) any {
				return rt.NewGoError(err)
			}); err != nil && !tolerateSettlementRace(err) {
				slog.Error("async: promise settlement failed", "error", err)
			}
			return
		}
		if err := settler.Resolve(func(rt *goja.Runtime) any {
			if mapResult != nil {
				return mapResult(rt, result)
			}
			if result == nil {
				return goja.Undefined()
			}
			return result
		}); err != nil && !tolerateSettlementRace(err) {
			slog.Error("async: promise settlement failed", "error", err)
		}
	}()
	return promise
}

// tolerateSettlementRace reports whether a failed settler attempt is an
// expected shutdown race (already settled, adapter invalid) rather than an
// unexpected settlement failure worth surfacing.
func tolerateSettlementRace(err error) bool {
	return errors.Is(err, gojaeventloop.ErrPromiseSettled) || errors.Is(err, gojaeventloop.ErrAdapterInvalid)
}
