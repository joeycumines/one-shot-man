package jsbridge

import (
	"context"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/one-shot-man/internal/builtin/async"
)

// Async runs fn off the event loop and returns a JS Promise via adapter.
// Unified helper for all Go->JS async bridges (termmux, aimux, mcp, mcpcallback, pabt, etc.).
// Collapses split asyncHandleValue / jsPromise wrappers into one.
func Async(adapter *gojaeventloop.Adapter, ctx context.Context, fn func(context.Context) (any, error)) goja.Value {
	return async.Promise(adapter, ctx, fn)
}

// MustAdapter panics if adapter is nil, with clear message for footgun.
func MustAdapter(adapter *gojaeventloop.Adapter) *gojaeventloop.Adapter {
	if adapter == nil {
		panic("jsbridge: adapter is nil — SetAdapter/NewBridge with adapter required before JS promise creation")
	}
	return adapter
}

var _ = MustAdapter
var _ = func() goja.Value { return goja.Undefined() }
