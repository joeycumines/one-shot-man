package async

import (
	"context"
	"errors"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// Promise runs fn off the event loop and returns a native goja Promise that
// settles via the owner-safe PromiseSettler. The adapter must be bound and
// called on the owner; fn is executed in a goroutine with baseCtx. context
// cancellation is propagated via baseCtx. Errors from fn are rejected as
// GoErrors (so JS sees error.message). Settler races (ErrPromiseSettled) are
// tolerated — they occur when shutdown wins the single admission slot.
func Promise(adapter *gojaeventloop.Adapter, baseCtx context.Context, fn func(ctx context.Context) (any, error)) goja.Value {
	if adapter == nil {
		panic("async: adapter is nil")
	}
	promise, settler := adapter.NewPromise()
	go func() {
		result, err := fn(baseCtx)
		if err != nil {
			_ = settler.Reject(func(rt *goja.Runtime) any {
				return rt.NewGoError(err)
			})
			return
		}
		if err := settler.Resolve(func(rt *goja.Runtime) any {
			if result == nil {
				return goja.Undefined()
			}
			return result
		}); err != nil && !errors.Is(err, gojaeventloop.ErrPromiseSettled) && !errors.Is(err, gojaeventloop.ErrAdapterInvalid) {
			// Terminal or double-settle after shutdown — ignore; promise stays pending
			// and will be rejected via terminal cleanup. Log via loop diagnostics if needed.
		}
	}()
	return promise
}
