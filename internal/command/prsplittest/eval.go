package prsplittest

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

var evalJSCallID int64

func MakeEvalJS(t testing.TB, engine *scripting.Engine, timeout time.Duration) func(string) (any, error) {
	t.Helper()
	return makeEvalJS(t, engine, timeout)
}

func makeEvalJS(t testing.TB, engine *scripting.Engine, timeout time.Duration) func(string) (any, error) {
	t.Helper()

	return func(js string) (any, error) {
		done := make(chan struct{})
		var result any
		var resultErr error

		submitErr := engine.Loop().Submit(func() {
			vm := engine.Runtime()

			val, err := vm.RunString(js)
			if err != nil {
				errMsg := err.Error()
				if strings.Contains(errMsg, "await") || strings.Contains(errMsg, "Unexpected identifier") || strings.Contains(errMsg, "Unexpected token") {
					callID := atomic.AddInt64(&evalJSCallID, 1)
					resultVar := fmt.Sprintf("__evalResult_%d", callID)
					errorVar := fmt.Sprintf("__evalError_%d", callID)
					// cleanup removes the uniquely-named global callbacks
					// after they fire, so repeated EvalJS calls on the same
					// engine don't accumulate Go closures (and the local
					// state they capture) on the global object forever.
					cleanup := func() {
						vm.GlobalObject().Delete(resultVar)
						vm.GlobalObject().Delete(errorVar)
					}
					_ = vm.Set(resultVar, func(val any) {
						result = val
						cleanup()
						close(done)
					})
					_ = vm.Set(errorVar, func(msg string) {
						resultErr = errors.New(msg)
						cleanup()
						close(done)
					})
					wrapped := "(async function() {\n" + insertReturnBeforeLastExpr(js) + "\n})().then(function(v) {\n\t" + resultVar + "(v);\n}, function(e) {\n\t" + errorVar + "(e && e.message ? e.message : String(e));\n});"
					if _, runErr := vm.RunString(wrapped); runErr != nil {
						cleanup()
						resultErr = runErr
						close(done)
					}
					return
				}
				resultErr = err
				close(done)
				return
			}

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

func insertReturnBeforeLastExpr(js string) string {
	trimmed := strings.TrimRight(js, " \t\n\r;")

	// Convert leading IIFE (function(...) to (async function(...) so `await`
	// is valid inside. Only the leading IIFE — NOT callbacks like .map(function).
	if strings.Contains(js, "await ") {
		leadingTrimmed := strings.TrimLeft(trimmed, " \t\n\r")
		if strings.HasPrefix(leadingTrimmed, "(function") && !strings.HasPrefix(leadingTrimmed, "(async function") {
			idx := len(trimmed) - len(leadingTrimmed)
			trimmed = trimmed[:idx] + "(async function" + trimmed[idx+9:]
		}
	}

	depth := 0
	inStr := false
	strCh := byte(0)
	lastTopSemi := -1

	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		if inStr {
			if c == '\\' && i+1 < len(trimmed) {
				i++
				continue
			}
			if c == strCh {
				inStr = false
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			inStr = true
			strCh = c
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		case ';':
			if depth == 0 {
				lastTopSemi = i
			}
		}
	}

	if lastTopSemi >= 0 {
		return trimmed[:lastTopSemi+1] + " return (" + trimmed[lastTopSemi+1:] + ");"
	}
	return "return (" + trimmed + ");"
}
