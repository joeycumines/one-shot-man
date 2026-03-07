// Package fetch provides a Goja module wrapping Go's net/http client for JS scripts.
// It is registered as "osm:fetch" and provides a Promise-based fetch() function
// following the browser Fetch API.
//
// The module requires the goja-eventloop adapter for async Promise support.
// HTTP requests run in a dedicated goroutine and resolve on the event loop,
// ensuring thread-safe access to the goja runtime.
package fetch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
	goeventloop "github.com/joeycumines/go-eventloop"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// defaultMaxResponseSize is the maximum response body size (10 MiB).
// Callers can override via the maxResponseSize option.
const defaultMaxResponseSize int64 = 10 << 20

// Require returns a module loader for osm:fetch backed by the event loop adapter.
// The adapter is required for Promise-based async fetch operations.
func Require(adapter *gojaeventloop.Adapter) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		_ = exports.Set("fetch", jsFetch(runtime, adapter))
		_ = exports.Set("sseReader", jsSSEReader(runtime, adapter))
	}
}

// jsFetch implements the browser-compliant fetch(url, options?) function.
// It returns a Promise<Response> that resolves with a Response object
// once the HTTP request completes and the body is fully read.
//
// Options:
//
//	method  - HTTP method (default: "GET")
//	headers - object of header key/value pairs
//	body    - request body string
//	timeout - request timeout in seconds (default: 30)
//	signal  - AbortSignal for cancelling the request
//
// The returned Promise resolves with a Response object:
//
//	status     - HTTP status code (number)
//	ok         - true if status is 200-299 (boolean)
//	statusText - HTTP status line, e.g. "200 OK" (string)
//	url        - final URL after redirects (string)
//	headers    - Headers object with get/has/entries/keys/values/forEach
//	text()     - returns Promise<string> with body as string
//	json()     - returns Promise<any> with body parsed as JSON
func jsFetch(runtime *goja.Runtime, adapter *gojaeventloop.Adapter) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()
		method, timeout, bodyReader, reqHeaders, signal, maxBody := parseOptions(call)

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		for k, v := range reqHeaders {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}

		// Set up context for request cancellation.
		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		// Wire AbortSignal to cancel the request context.
		if signal != nil {
			if signal.Aborted() {
				cancel()
				promise, _, reject := adapter.JS().NewChainedPromise()
				reject(signal.Reason())
				return adapter.GojaWrapPromise(promise)
			}
			signal.OnAbort(func(reason any) {
				cancel()
			})
		}

		req = req.WithContext(ctx)
		promise, resolve, reject := adapter.JS().NewChainedPromise()

		go func() {
			defer cancel()
			client := &http.Client{}
			resp, doErr := client.Do(req)
			if doErr != nil {
				reject(doErr)
				return
			}

			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
			resp.Body.Close()
			if readErr != nil {
				reject(readErr)
				return
			}
			if int64(len(body)) > maxBody {
				reject(fmt.Errorf("response body exceeds maximum size of %d bytes", maxBody))
				return
			}

			if submitErr := adapter.Loop().Submit(func() {
				resolve(buildResponse(runtime, adapter, resp, body))
			}); submitErr != nil {
				reject(fmt.Errorf("event loop not running"))
			}
		}()

		return adapter.GojaWrapPromise(promise)
	}
}

// jsSSEReader returns a factory function: sseReader(body) → SSE reader object.
// body must be a ReadableStream JS object (response.body).  The returned reader
// has a read() method returning Promise<{value: {event, data, id}, done: boolean}>.
func jsSSEReader(runtime *goja.Runtime, adapter *gojaeventloop.Adapter) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		bodyArg := call.Argument(0)
		if bodyArg == nil || goja.IsUndefined(bodyArg) || goja.IsNull(bodyArg) {
			panic(runtime.NewTypeError("sseReader requires a ReadableStream body argument"))
		}
		bodyObj, ok := bodyArg.(*goja.Object)
		if !ok {
			panic(runtime.NewTypeError("sseReader argument must be a ReadableStream object"))
		}

		// Retrieve the Go ReadableStream stashed on the JS object.
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
		return wrapSSEParserJS(runtime, adapter, parser)
	}
}

// parseOptions extracts HTTP request parameters from the optional second argument.
func parseOptions(call goja.FunctionCall) (method string, timeout time.Duration, bodyReader io.Reader, reqHeaders map[string]any, signal *goeventloop.AbortSignal, maxResponseSize int64) {
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

	// Extract AbortSignal from the options object via the raw goja value.
	// The signal is stored as a native Go *goeventloop.AbortSignal on the
	// JS object's "_signal" property, set by the goja-eventloop adapter.
	if len(call.Arguments) > 1 {
		if argObj, ok := call.Arguments[1].(*goja.Object); ok {
			if signalVal := argObj.Get("signal"); signalVal != nil && !goja.IsUndefined(signalVal) && !goja.IsNull(signalVal) {
				if signalObj, ok := signalVal.(*goja.Object); ok {
					if nativeVal := signalObj.Get("_signal"); nativeVal != nil && !goja.IsUndefined(nativeVal) {
						if s, ok := nativeVal.Export().(*goeventloop.AbortSignal); ok {
							signal = s
						}
					}
				}
			}
		}
	}

	return
}

// buildResponse constructs the JS Response object with the full body buffered.
// Must be called on the event loop goroutine.
func buildResponse(runtime *goja.Runtime, adapter *gojaeventloop.Adapter, resp *http.Response, body []byte) *goja.Object {
	result := runtime.NewObject()
	_ = result.Set("status", resp.StatusCode)
	_ = result.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
	_ = result.Set("statusText", resp.Status)
	_ = result.Set("url", resp.Request.URL.String())
	_ = result.Set("headers", buildHeaders(runtime, resp.Header))

	// body — ReadableStream backed by the already-buffered bytes.
	// reader.read() returns Promise<{value: string, done: boolean}>.
	stream := NewReadableStream(io.NopCloser(bytes.NewReader(body)))
	_ = result.Set("body", wrapReadableStreamJS(runtime, adapter, stream))

	// text() returns a Promise<string> that resolves with the body as a string.
	// Since the body is fully buffered, the Promise resolves immediately.
	_ = result.Set("text", func(goja.FunctionCall) goja.Value {
		p, resolve, _ := runtime.NewPromise()
		resolve(string(body))
		return runtime.ToValue(p)
	})

	// json() returns a Promise<any> that resolves with the parsed JSON.
	// Since the body is fully buffered, the Promise resolves immediately.
	// Rejects if the body is not valid JSON.
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

	return result
}

// buildHeaders constructs a Headers object implementing the browser Headers API.
// Must be called on the event loop goroutine.
func buildHeaders(runtime *goja.Runtime, h http.Header) *goja.Object {
	obj := runtime.NewObject()

	// get(name) returns the header value (joined with ", ") or null if not present.
	_ = obj.Set("get", func(name string) goja.Value {
		canonical := http.CanonicalHeaderKey(name)
		values, exists := h[canonical]
		if !exists {
			return goja.Null()
		}
		return runtime.ToValue(strings.Join(values, ", "))
	})

	// has(name) returns true if the header exists.
	_ = obj.Set("has", func(name string) bool {
		_, exists := h[http.CanonicalHeaderKey(name)]
		return exists
	})

	// entries() returns an array of [name, value] pairs (lowercased keys).
	_ = obj.Set("entries", func() goja.Value {
		var entries []any
		for k, v := range h {
			entries = append(entries, []any{strings.ToLower(k), strings.Join(v, ", ")})
		}
		return runtime.ToValue(entries)
	})

	// keys() returns an array of lowercased header names.
	_ = obj.Set("keys", func() goja.Value {
		var keys []string
		for k := range h {
			keys = append(keys, strings.ToLower(k))
		}
		return runtime.ToValue(keys)
	})

	// values() returns an array of header values.
	_ = obj.Set("values", func() goja.Value {
		var values []string
		for _, v := range h {
			values = append(values, strings.Join(v, ", "))
		}
		return runtime.ToValue(values)
	})

	// forEach(callback) calls callback(value, name, headers) for each header.
	_ = obj.Set("forEach", func(call goja.FunctionCall) goja.Value {
		fn, ok := goja.AssertFunction(call.Argument(0))
		if !ok {
			panic(runtime.NewTypeError("forEach requires a function argument"))
		}
		for k, v := range h {
			val := strings.Join(v, ", ")
			if _, err := fn(goja.Undefined(),
				runtime.ToValue(val),
				runtime.ToValue(strings.ToLower(k)),
				obj,
			); err != nil {
				panic(err)
			}
		}
		return goja.Undefined()
	})

	return obj
}
