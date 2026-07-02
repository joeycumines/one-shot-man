package jscompliance

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// harness.js is embedded and loaded before every spec.
//
//go:embed specs/harness.js
var harnessJS string

// specs embeds every file under specs/ so the suite is fully hermetic (no
// dependency on the working directory). Spec files are read via specs.ReadFile.
//
//go:embed all:specs
var specs embed.FS

// defaultEvalTimeout is the per-call cap for evalJS and the per-spec cap for
// runSpec. A never-settling promise MUST fail (never silently pass), so this
// is a hard deadline.
const defaultEvalTimeout = 10 * time.Second

// skipSlow skips slow-tier tests in -short mode (per CLAUDE.md: use
// testing.Short, never build tags).
func skipSlow(t testing.TB) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping slow compliance test in short mode")
	}
}

// newComplianceEngine builds the REAL production engine, mirroring
// internal/scripting.newTestEngine verbatim. The engine owns its own event
// loop; t.Cleanup closes it. The caller MUST run all JavaScript on the loop
// (engine.Loop().Submit / evalJS / runSpec).
func newComplianceEngine(t testing.TB, ctx context.Context) (*scripting.Engine, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine, &stdout, &stderr
}

// newComplianceEngineOpts is like newComplianceEngine but applies EngineOptions
// (e.g. WithModulePaths for bare-name/security resolution tests).
func newComplianceEngineOpts(t testing.TB, ctx context.Context, opts ...scripting.EngineOption) (*scripting.Engine, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("", t.Name()), "memory", nil, 0, slog.LevelInfo, opts...)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	return engine, &stdout, &stderr
}

// evalJS evaluates a JavaScript expression on the engine's event loop and
// returns its result. It is await-aware (mirrors
// internal/command/prsplittest.makeEvalJS): for top-level-await expressions it
// wraps the source in an async IIFE and attaches Go-side onFulfilled/
// onRejected handlers to the resulting Promise, so the loop is NEVER blocked
// by a Promise expression. A timeout fails (returns an error) — it never
// silently passes a never-settling promise.
func evalJS(t testing.TB, engine *scripting.Engine, js string, timeout time.Duration) (any, error) {
	t.Helper()
	if timeout <= 0 {
		timeout = defaultEvalTimeout
	}

	// Hoist the await-path callback names so the timeout branch (off-loop) can
	// neutralize them — porting the prsplittest/eval.go lesson. Without this,
	// a timed-out await leaks the __evResult_N/__evError_N global closures
	// (and the Go state they capture: vm, result, done) for the engine's life.
	callID := evalCallID.Add(1)
	resultVar := fmt.Sprintf("__evResult_%d", callID)
	errorVar := fmt.Sprintf("__evError_%d", callID)

	var (
		result    any
		resultErr error
		done      = make(chan struct{})
		doneOnce  sync.Once
		closeDone = func() { doneOnce.Do(func() { close(done) }) }
	)

	submitErr := engine.Loop().Submit(func() {
		vm := engine.Runtime()

		val, runErr := vm.RunString(js)
		if runErr != nil {
			errMsg := runErr.Error()
			// Detect top-level await: wrap in an async IIFE that settles via
			// uniquely-named global callbacks, exactly like makeEvalJS.
			if strings.Contains(errMsg, "await") || strings.Contains(errMsg, "Unexpected identifier") || strings.Contains(errMsg, "Unexpected token") {
				cleanup := func() {
					vm.GlobalObject().Delete(resultVar)
					vm.GlobalObject().Delete(errorVar)
				}
				_ = vm.Set(resultVar, func(v any) { result = v; cleanup(); closeDone() })
				_ = vm.Set(errorVar, func(msg string) { resultErr = errors.New(msg); cleanup(); closeDone() })
				wrapped := "(async function() {\n" + insertReturnBeforeLastExpr(js) + "\n})().then(function(v){ " + resultVar + "(v); }, function(e){ " + errorVar + "((e && e.message) ? e.message : String(e)); });"
				if _, e2 := vm.RunString(wrapped); e2 != nil {
					cleanup()
					resultErr = e2
					closeDone()
				}
				return
			}
			resultErr = runErr
			closeDone()
			return
		}

		// If the value is a thenable, attach Go-side handlers.
		if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
			obj := val.ToObject(vm)
			if obj != nil {
				if thenProp := obj.Get("then"); thenProp != nil && !goja.IsUndefined(thenProp) {
					if thenFn, ok := goja.AssertFunction(thenProp); ok {
						onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
							result = call.Argument(0).Export()
							closeDone()
							return goja.Undefined()
						})
						onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
							resultErr = fmt.Errorf("promise rejected: %v", call.Argument(0).Export())
							closeDone()
							return goja.Undefined()
						})
						// p.then(onFulfilled).catch(onRejected): catches both a
						// rejection of p AND a throw inside onFulfilled.
						thenResult, thenErr := thenFn(val, onFulfilled)
						if thenErr != nil {
							resultErr = thenErr
							closeDone()
							return
						}
						if thenObj := thenResult.ToObject(vm); thenObj != nil {
							if catchProp := thenObj.Get("catch"); catchProp != nil && !goja.IsUndefined(catchProp) {
								if catchFn, ok := goja.AssertFunction(catchProp); ok {
									if _, catchErr := catchFn(thenResult, onRejected); catchErr != nil {
										resultErr = catchErr
										closeDone()
									}
								}
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
		closeDone()
	})
	if submitErr != nil {
		return nil, fmt.Errorf("evalJS: event loop not running: %w", submitErr)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return result, resultErr
	case <-timer.C:
		// Neutralize the await-path global callbacks with no-ops (NOT delete) so
		// a late settlement calls a no-op rather than dereferencing a removed
		// global, and the original closures (capturing vm/result/done) are
		// released. Runs on the loop goroutine, serialized with any settlement.
		_ = engine.Loop().Submit(func() {
			vm := engine.Runtime()
			_ = vm.Set(resultVar, func(any) {})
			_ = vm.Set(errorVar, func(string) {})
		})
		return nil, fmt.Errorf("evalJS timed out after %s (a never-settling promise fails, never passes)", timeout)
	}
}

// specResult is a single assertion outcome produced by harness.js.
type specResult struct {
	Name  string `json:"name"`
	Pass  bool   `json:"pass"`
	Error string `json:"error"`
}

// runSpec loads harness.js + the named spec (e.g. "specs/core_promises.spec.js")
// on the engine, awaits __done via Go-side handlers, then maps each recorded
// result to a t.Run subtest. A spec that registers zero tests FAILS (it would
// otherwise be a silent no-op / false-confidence trap).
func runSpec(t *testing.T, engine *scripting.Engine, specPath string, timeout time.Duration) {
	t.Helper()
	specSrc, err := specs.ReadFile(specPath)
	if err != nil {
		t.Fatalf("runSpec: cannot read embedded spec %q: %v", specPath, err)
	}
	runSpecSource(t, engine, specPath, string(specSrc), timeout)
}

// runSpecSource runs an inline spec source (labeled by label) through the same
// harness + __done mechanic as runSpec. Used by specs whose source is built
// inline (e.g. data-driven cases) and by the harness self-tests.
func runSpecSource(t *testing.T, engine *scripting.Engine, label, specSrc string, timeout time.Duration) {
	t.Helper()
	results, err := collectSpecResults(t, engine, label, specSrc, timeout)
	if err != nil {
		t.Fatalf("runSpec %s: %v", label, err)
		return
	}
	if len(results) == 0 {
		t.Fatalf("runSpec %s registered zero tests — a spec that asserts nothing is a false-confidence trap", label)
		return
	}
	for _, r := range results {
		t.Run(r.Name, func(t *testing.T) {
			if !r.Pass {
				t.Errorf("%s", r.Error)
			}
		})
	}
}

// collectSpecResults loads harness.js + specSrc on the engine, awaits __done
// via Go-side handlers, and returns the recorded results. It has no test
// side-effects, so callers can assert on the data directly (used by the
// harness self-tests and data-driven specs).
func collectSpecResults(t *testing.T, engine *scripting.Engine, label, specSrc string, timeout time.Duration) ([]specResult, error) {
	t.Helper()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	var (
		results   []specResult
		specErr   error
		done      = make(chan struct{})
		doneOnce  sync.Once
		closeDone = func() { doneOnce.Do(func() { close(done) }) }
	)

	submitErr := engine.Loop().Submit(func() {
		vm := engine.Runtime()

		// 1. Load the assertion runtime (defines test/assert/__done).
		if _, e := vm.RunString(harnessJS); e != nil {
			specErr = fmt.Errorf("harness.js failed to load: %w", e)
			closeDone()
			return
		}
		// 2. Mirror production: args global (DRIFT-3 — bound by the command
		//    layer in production; here []).
		_ = vm.Set("args", []any{})

		// 3. Run the spec body (registers tests via test()).
		if _, e := vm.RunString(specSrc); e != nil {
			specErr = fmt.Errorf("spec %s failed to load: %w", label, e)
			closeDone()
			return
		}

		// 4. Close registration (resolves __done now if no tests pending).
		if _, e := vm.RunString("__finishRegistration();"); e != nil {
			specErr = fmt.Errorf("__finishRegistration failed: %w", e)
			closeDone()
			return
		}

		// 5. Attach Go-side handlers to __done (never block on a Promise).
		doneVal := vm.Get("__done")
		if doneVal == nil || goja.IsUndefined(doneVal) || goja.IsNull(doneVal) {
			specErr = errors.New("harness did not expose __done")
			closeDone()
			return
		}
		doneObj := doneVal.ToObject(vm)
		thenProp := doneObj.Get("then")
		thenFn, ok := goja.AssertFunction(thenProp)
		if !ok {
			specErr = errors.New("__done is not a thenable")
			closeDone()
			return
		}
		onFulfilled := vm.ToValue(func(call goja.FunctionCall) goja.Value {
			results = exportResults(vm.Get("__results"))
			closeDone()
			return goja.Undefined()
		})
		onRejected := vm.ToValue(func(call goja.FunctionCall) goja.Value {
			specErr = fmt.Errorf("__done rejected: %v", call.Argument(0).Export())
			closeDone()
			return goja.Undefined()
		})
		if _, e := thenFn(doneVal, onFulfilled, onRejected); e != nil {
			specErr = fmt.Errorf("attaching __done handlers failed: %w", e)
			closeDone()
			return
		}
	})
	if submitErr != nil {
		return nil, fmt.Errorf("event loop not running: %w", submitErr)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		return nil, fmt.Errorf("timed out after %s (a never-settling spec fails)", timeout)
	}
	return results, specErr
}

// exportResults converts the JS __results array to []specResult.
func exportResults(v goja.Value) []specResult {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	raw, ok := v.Export().([]any)
	if !ok {
		return nil
	}
	out := make([]specResult, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		pass, _ := m["pass"].(bool)
		errMsg, _ := m["error"].(string)
		out = append(out, specResult{Name: name, Pass: pass, Error: errMsg})
	}
	return out
}

// evalCallID uniquely namespaces the global callbacks used by the await path
// of evalJS (so concurrent/repeated calls don't collide). Atomic because
// parallel subtests may eval concurrently on separate engines.
var evalCallID atomic.Int64

// insertReturnBeforeLastExpr rewrites a JS snippet so its final top-level
// expression is returned, and converts a leading `(function` IIFE to
// `(async function` so top-level await is legal inside. Ported from
// internal/command/prsplittest.insertReturnBeforeLastExpr.
func insertReturnBeforeLastExpr(js string) string {
	trimmed := strings.TrimRight(js, " \t\n\r;")

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
