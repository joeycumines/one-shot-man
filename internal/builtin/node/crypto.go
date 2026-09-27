package node

import (
	"context"
	"crypto/rand"
	"math"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// cryptoRequire returns the loader for require("crypto"). The subset is
// randomBytes(n): Node's async form resolves a Buffer-shaped Uint8Array of n
// cryptographically secure bytes.
const maxRandomBytes = 0x7FFFFFF0

func CryptoRequire(ctx context.Context, adapter *gojaeventloop.Adapter) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		_ = exports.Set("randomBytes", func(call goja.FunctionCall) goja.Value {
			sizeArg := call.Argument(0)
			var sizeNumber float64
			switch number := sizeArg.Export().(type) {
			case int64:
				sizeNumber = float64(number)
			case float64:
				sizeNumber = number
			default:
				return rejectTypeErrorCode(adapter, "randomBytes", "ERR_INVALID_ARG_TYPE", "The \"size\" argument must be of type number")
			}
			if math.IsNaN(sizeNumber) || math.IsInf(sizeNumber, 0) || sizeNumber < 0 || sizeNumber > maxRandomBytes {
				return rejectRangeErrorCode(adapter, "randomBytes", "ERR_OUT_OF_RANGE", "The \"size\" argument must be between 0 and 2147483632")
			}
			size := int(sizeNumber)
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

func rejectRangeErrorCode(adapter *gojaeventloop.Adapter, _, code, message string) goja.Value {
	promise, settler := adapter.NewPromise()
	_ = settler.Reject(func(rt *goja.Runtime) any {
		rangeError, err := rt.New(rt.Get("RangeError"), rt.ToValue(message))
		if err != nil {
			return rt.NewGoError(err)
		}
		_ = rangeError.Set("code", code)
		return rangeError
	})
	return promise
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
