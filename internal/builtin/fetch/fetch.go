// Package fetch provides a Goja module wrapping Go's net/http client for JS scripts.
package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goja_nodejs/require"
)

// handleSettleErr handles settler/bridge settlement errors symmetrically.
// ErrLoopTerminated, ErrAdapterInvalid and ErrPromiseSettled are expected
// during shutdown/termination and are tolerated at debug level; other
// errors are unexpected and would be logged. Documented tolerance: settlement
// may be dropped if loop terminated before Submit, promise may remain pending
// only for hard Close (stranded is defined behavior, see track.go).
func handleSettleErr(err error) {
    if err == nil {
        return
    }
    if errors.Is(err, goeventloop.ErrLoopTerminated) || errors.Is(err, gojaeventloop.ErrAdapterInvalid) || errors.Is(err, gojaeventloop.ErrPromiseSettled) {
        return
    }
    _ = err
}


const defaultMaxResponseSize int64 = 10 << 20

func Require(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		if adapter != nil {
			_ = exports.Set("fetch", jsFetch(ctx, runtime, adapter, loop))
			_ = exports.Set("sseReader", jsSSEReader(ctx, runtime, adapter, loop))
		}
	}
}

func jsFetch(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()
		method, timeout, bodyReader, reqHeaders, signalVal, maxBody := parseOptions(call)
		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		for k, v := range reqHeaders {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
		reqCtx, cancel := context.WithTimeout(ctx, timeout)
		var abortCleanup func()
		if signalVal != nil && !goja.IsUndefined(signalVal) && !goja.IsNull(signalVal) {
			if cleanup, aborted, ok := adapter.TrackAbortSignal(signalVal, func() { cancel() }); ok {
				abortCleanup = cleanup
				if aborted {
					var reason any
					if sigObj, ok := signalVal.(*goja.Object); ok {
						if rv := sigObj.Get("reason"); rv != nil && !goja.IsUndefined(rv) && !goja.IsNull(rv) {
							reason = rv.Export()
						}
					}
					cancel()
					promise, settler := adapter.NewPromise()
					handleSettleErr(settler.Reject(func(rt *goja.Runtime) any {
						if reason != nil {
							return reason
						}
						return rt.NewGoError(fmt.Errorf("aborted"))
					}))
					return promise
				}
			}
		}
		req = req.WithContext(reqCtx)
		type fetchResult struct {
			resp *http.Response
			body []byte
		}
		baseCtx := ctx
		return adapter.TrackPromise(reqCtx, func(trackCtx context.Context, settle gojaeventloop.TrackedSettlement) {
			defer cancel()
			if abortCleanup != nil {
				defer abortCleanup()
			}
			client := &http.Client{}
			resp, doErr := client.Do(req)
			if doErr != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(doErr) })
				return
			}
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
			resp.Body.Close()
			if readErr != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(readErr) })
				return
			}
			if int64(len(body)) > maxBody {
				_ = settle.Settle(true, func(rt *goja.Runtime) any {
					return rt.NewGoError(fmt.Errorf("response body exceeds maximum size of %d bytes", maxBody))
				})
				return
			}
			fr := fetchResult{resp: resp, body: body}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				return buildResponse(baseCtx, rt, adapter, loop, fr.resp, fr.body)
			})
		})
	}
}

func jsSSEReader(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		bodyArg := call.Argument(0)
		if bodyArg == nil || goja.IsUndefined(bodyArg) || goja.IsNull(bodyArg) {
			panic(runtime.NewTypeError("sseReader requires a ReadableStream body argument"))
		}
		bodyObj, ok := bodyArg.(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("sseReader argument must be a ReadableStream object"))
		}
		goStreamVal := bodyObj.Get("_goStream")
		if goStreamVal == nil || goja.IsUndefined(goStreamVal) {
			panic(runtime.NewTypeError("body does not have a Go ReadableStream"))
		}
		goStream, ok := goStreamVal.Export().(*ReadableStream)
		if !ok {
			panic(runtime.NewTypeError("_goStream is not a *ReadableStream"))
		}
		reader, err := goStream.GetReader()
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		parser := NewSSEParser(reader)
		return wrapSSEParserJS(ctx, runtime, adapter, loop, parser)
	}
}

func parseOptions(call goja.FunctionCall) (method string, timeout time.Duration, bodyReader io.Reader, reqHeaders map[string]any, signalVal goja.Value, maxResponseSize int64) {
	method = "GET"
	timeout = 30 * time.Second
	maxResponseSize = defaultMaxResponseSize
	if len(call.Arguments) <= 1 {
		return
	}
	arg := call.Arguments[1]
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return
	}
	opts, ok := arg.Export().(map[string]any)
	if !ok {
		return
	}
	if m, ok := opts["method"]; ok {
		if s, ok := m.(string); ok {
			method = strings.ToUpper(s)
		}
	}
	if t, ok := opts["timeout"]; ok {
		switch v := t.(type) {
		case int64:
			timeout = time.Duration(v) * time.Second
		case float64:
			timeout = time.Duration(v * float64(time.Second))
		}
	}
	if b, ok := opts["body"]; ok {
		if s, ok := b.(string); ok {
			bodyReader = strings.NewReader(s)
		}
	}
	if m, ok := opts["maxResponseSize"]; ok {
		switch v := m.(type) {
		case int64:
			maxResponseSize = v
		case float64:
			maxResponseSize = int64(v)
		}
	}
	if h, ok := opts["headers"]; ok {
		if m, ok := h.(map[string]any); ok {
			reqHeaders = m
		}
	}
	if len(call.Arguments) > 1 {
		if argObj, ok := call.Arguments[1].(*goja.Object); ok {
			if sv := argObj.Get("signal"); sv != nil && !goja.IsUndefined(sv) && !goja.IsNull(sv) {
				signalVal = sv
			}
		}
	}
	return
}

func buildResponse(ctx context.Context, runtime *goja.Runtime, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, resp *http.Response, body []byte) *goja.Object {
	result := runtime.NewObject()
	_ = result.Set("status", resp.StatusCode)
	_ = result.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
	_ = result.Set("statusText", resp.Status)
	_ = result.Set("url", resp.Request.URL.String())
	_ = result.Set("headers", buildHeaders(runtime, resp.Header))
	_ = result.Set("text", func(goja.FunctionCall) goja.Value {
		p, resolve, _ := runtime.NewPromise()
		resolve(string(body))
		return runtime.ToValue(p)
	})
	_ = result.Set("json", func(goja.FunctionCall) goja.Value {
		var parsed any
		if err := json.Unmarshal(body, &parsed); err != nil {
			p, _, reject := runtime.NewPromise()
			reject(runtime.NewGoError(err))
			return runtime.ToValue(p)
		}
		p, resolve, _ := runtime.NewPromise()
		resolve(runtime.ToValue(parsed))
		return runtime.ToValue(p)
	})
	stream := NewReadableStream(ctx, io.NopCloser(bytes.NewReader(body)))
	_ = result.Set("body", wrapReadableStreamJS(ctx, runtime, adapter, stream, loop))
	return result
}

func buildHeaders(runtime *goja.Runtime, h http.Header) *goja.Object {
	obj := runtime.NewObject()
	_ = obj.Set("get", func(name string) goja.Value {
		canonical := http.CanonicalHeaderKey(name)
		values, exists := h[canonical]
		if !exists {
			return goja.Null()
		}
		return runtime.ToValue(strings.Join(values, ", "))
	})
	_ = obj.Set("has", func(name string) bool {
		_, exists := h[http.CanonicalHeaderKey(name)]
		return exists
	})
	_ = obj.Set("entries", func() goja.Value {
		var entries []any
		for k, v := range h {
			entries = append(entries, []any{strings.ToLower(k), strings.Join(v, ", ")})
		}
		return runtime.ToValue(entries)
	})
	_ = obj.Set("keys", func() goja.Value {
		var keys []string
		for k := range h {
			keys = append(keys, strings.ToLower(k))
		}
		return runtime.ToValue(keys)
	})
	_ = obj.Set("values", func() goja.Value {
		var values []string
		for _, v := range h {
			values = append(values, strings.Join(v, ", "))
		}
		return runtime.ToValue(values)
	})
	_ = obj.Set("forEach", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("forEach requires a function argument"))
		}
		for k, v := range h {
			val := strings.Join(v, ", ")
			if _, err := fn(goja.Undefined(), runtime.ToValue(val), runtime.ToValue(strings.ToLower(k)), obj); err != nil {
				panic(err)
			}
		}
		return goja.Undefined()
	})
	return obj
}
