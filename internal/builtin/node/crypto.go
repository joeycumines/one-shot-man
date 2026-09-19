package node

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// cryptoRequire returns the loader for require("crypto"). The subset is
// randomBytes(n): Node's async form resolves a Buffer-shaped Uint8Array of n
// cryptographically secure bytes; a negative size rejects with ERR_OUT_OF_RANGE.
func CryptoRequire(ctx context.Context, adapter *gojaeventloop.Adapter) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		_ = exports.Set("randomBytes", func(call goja.FunctionCall) goja.Value {
			size := 0
			if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
				size = int(call.Argument(0).ToInteger())
			}
			if size < 0 || size > 0x7FFFFFF0 {
				promise, settler := adapter.NewPromise()
				_ = settler.Reject(func(rt *goja.Runtime) any {
					obj := rt.NewGoError(errors.New("The \"size\" argument must be between 0 and 2147483647"))
					_ = obj.Set("code", "ERR_OUT_OF_RANGE")
					return obj
				})
				return promise
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				buf := make([]byte, size)
				if _, err := rand.Read(buf); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any { return bytesToUint8Array(rt, buf) })
			})
		})
	}
}

// bytesToUint8Array exposes bytes as a Uint8Array, which is the Buffer view
// scripts need for token material (length, byte indexing, hex conversion on
// the JS side). TypedArray constructors require `new` — invoking Uint8Array
// as a plain function throws in goja — so construction goes through rt.New.
// The plain-array fallback stays only for a genuinely absent global.
func bytesToUint8Array(rt *goja.Runtime, buf []byte) goja.Value {
	constructor := rt.GlobalObject().Get("Uint8Array")
	if constructor != nil && !goja.IsUndefined(constructor) {
		if obj, err := rt.New(constructor, rt.ToValue(len(buf))); err == nil {
			for i, b := range buf {
				_ = obj.Set(itoa(i), b)
			}
			return obj
		}
	}
	// Uint8Array unavailable: fall back to a plain byte array.
	arr := make([]any, len(buf))
	for i, b := range buf {
		arr[i] = b
	}
	return rt.ToValue(arr)
}
