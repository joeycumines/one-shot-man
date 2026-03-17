package prsplittest

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// MakeEvalJS creates an evalJS function from a [scripting.Engine] with a
// custom timeout. This is the exported version of makeEvalJS for tests that
// need direct access to the raw engine (e.g., pr_split_tui_hang_test.go).
func MakeEvalJS(t testing.TB, engine *scripting.Engine, timeout time.Duration) func(string) (any, error) {
	t.Helper()
	return makeEvalJS(t, engine, timeout)
}

// makeEvalJS creates an evalJS function from a [scripting.Engine].
// This is a direct port of the makeEvalJS helper from pr_split_00_core_test.go.
//
// The returned function:
//   - Detects async JS (contains "await ") and wraps in async IIFE
//   - Handles Promise results via .then/.catch chaining
//   - Times out after the specified duration
func makeEvalJS(t testing.TB, engine *scripting.Engine, timeout time.Duration) func(string) (any, error) {
	t.Helper()

	return func(js string) (any, error) {
		done := make(chan struct{})
		var result any
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			// Async path: if JS contains 'await', wrap in async IIFE.
			if strings.Contains(js, "await ") {
				_ = vm.Set("__evalResult", func(val any) {
					result = val
					close(done)
				})
				_ = vm.Set("__evalError", func(msg string) {
					resultErr = errors.New(msg)
					close(done)
				})
				wrapped := "(async function() {\n\ttry {\n\t\tvar __res = " + js + ";\n\t\tif (__res && typeof __res.then === 'function') { __res = await __res; }\n\t\t__evalResult(__res);\n\t} catch(e) {\n\t\t__evalError(e.message || String(e));\n\t}\n})();"
				if _, runErr := vm.RunString(wrapped); runErr != nil {
					resultErr = runErr
					close(done)
				}
				return
			}

			// Synchronous: run directly.
			val, err := vm.RunString(js)
			if err != nil {
				resultErr = err
				close(done)
				return
			}

			// Check if result is a Promise (duck-type via .then).
			if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
				obj := val.ToObject(vm)
				if obj != nil {
					thenProp := obj.Get("then")
					if thenProp != nil && !goja.IsUndefined(thenProp) {
						if thenFn, ok := goja.AssertFunction(thenProp); ok {
							onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								result = call.Argument(0).Export()
								close(done)
								return goja.Undefined()
							})
							onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
								resultErr = fmt.Errorf("promise rejected: %v", call.Argument(0).Export())
								close(done)
								return goja.Undefined()
							})
							thenResult, thenErr := thenFn(val, onFulfilled)
							if thenErr != nil {
								resultErr = thenErr
								close(done)
								return
							}
							thenObj := thenResult.ToObject(vm)
							catchProp := thenObj.Get("catch")
							if catchFn, ok := goja.AssertFunction(catchProp); ok {
								if _, catchErr := catchFn(thenResult, onRejected); catchErr != nil {
									resultErr = catchErr
									close(done)
								}
							}
							return
						}
					}
				}
			}

			if val != nil {
				result = val.Export()
			}
			close(done)
		})
		if submitErr != nil {
			return nil, submitErr
		}

		select {
		case <-done:
			return result, resultErr
		case <-time.After(timeout):
			return nil, fmt.Errorf("evalJS timed out after %s", timeout)
		}
	}
}
