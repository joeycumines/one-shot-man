package termmux

import (
	"context"
	"fmt"
	"os"
	"strings"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
	"github.com/joeycumines/one-shot-man/internal/termmux/ptyio"
)

func trackSessionOperation(ctx context.Context, adapter *gojaeventloop.Adapter, runtime *goja.Runtime, operation func() (any, error)) goja.Value {
	if adapter == nil {
		panic(runtime.NewGoError(fmt.Errorf("session operation: event loop adapter is required")))
	}
	return adapter.TrackPromise(ctx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
		value, err := operation()
		if err != nil {
			_ = settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(err) })
			return
		}
		_ = settle.Settle(false, func(owner *goja.Runtime) any {
			if value == nil {
				return goja.Undefined()
			}
			return owner.ToValue(value)
		})
	})
}

// newCaptureSession creates a [parent.CaptureSession] from JS arguments and
// returns a wrapped JS object.
//
// JS signature:
//
//	termmux.newCaptureSession(command, args?, { dir?, rows?, cols?, env?, envReplace? }?)
func newCaptureSession(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(runtime.NewTypeError("newCaptureSession: command argument is required"))
	}

	cmd := call.Argument(0).String()
	if cmd == "" {
		panic(runtime.NewTypeError("newCaptureSession: command must be a non-empty string"))
	}

	cfg := parent.CaptureConfig{
		Command: cmd,
	}

	// Parse optional args array (second argument).
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		argsObj := call.Argument(1).ToObject(runtime)
		if lenVal := argsObj.Get("length"); lenVal != nil && !goja.IsUndefined(lenVal) {
			arrLen := lenVal.ToInteger()
			for i := range arrLen {
				v := argsObj.Get(fmt.Sprintf("%d", i))
				if v != nil && !goja.IsUndefined(v) {
					cfg.Args = append(cfg.Args, v.String())
				}
			}
		}
	}

	// Parse optional options object (third argument).
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
		optObj := call.Argument(2).ToObject(runtime)
		if v := optObj.Get("dir"); v != nil && !goja.IsUndefined(v) {
			cfg.Dir = v.String()
		}
		if v := optObj.Get("name"); v != nil && !goja.IsUndefined(v) {
			cfg.Name = v.String()
		}
		if v := optObj.Get("kind"); v != nil && !goja.IsUndefined(v) {
			cfg.Kind = parent.SessionKind(v.String())
		}
		if v := optObj.Get("rows"); v != nil && !goja.IsUndefined(v) {
			cfg.Rows = int(v.ToInteger())
		}
		if v := optObj.Get("cols"); v != nil && !goja.IsUndefined(v) {
			cfg.Cols = int(v.ToInteger())
		}
		if v := optObj.Get("env"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			envObj := v.ToObject(runtime)
			cfg.Env = make(map[string]string)
			for _, key := range envObj.Keys() {
				val := envObj.Get(key)
				if val != nil && !goja.IsUndefined(val) && !goja.IsNull(val) {
					cfg.Env[key] = val.String()
				}
			}
		}
		if v := optObj.Get("envReplace"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			cfg.EnvReplace = v.ToBoolean()
		}
	}

	cs := parent.NewCaptureSession(cfg)
	return WrapCaptureSession(ctx, adapter, loop, runtime, cs)
}

// WrapCaptureSession wraps a [parent.CaptureSession] into a Goja object with
// JavaScript-callable methods. Exported so callers (e.g., pr_split.go) can
// create a Go-side CaptureSession and expose it through the same interface.
//
// AUDIT (T004/T059/T10/T49/T56): All 17 methods verified present and type-correct:
//
//	start, interrupt, kill, pause, resume, isPaused,
//	resize, wait, write, sendEOF, close, pid, exitCode, isDone,
//	passthrough, readAvailable.
//
// Task 56: target, setTarget, isRunning removed — all JS call sites
// use SessionManager wrappers (tuiMux.session()) instead.
//
// The 4 methods called by runVerifyBranch/pollVerifySession (isDone,
// exitCode, close, interrupt) are confirmed bound with correct signatures
// via module_capture_test.go. Screen reads go through SessionManager
// snapshots via the _buildVerifyProxy in JS (Task 48).
func WrapCaptureSession(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop, runtime *goja.Runtime, cs *parent.CaptureSession) goja.Value {
	obj := wrapInteractiveSession(ctx, adapter, runtime, cs, parent.SessionKindCapture).ToObject(runtime)

	// ── CaptureSession-specific methods (not part of InteractiveSession) ──

	// Task 49: Output() and Screen() removed from CaptureSession.
	// Screen reads now go through SessionManager snapshots via the
	// _buildVerifyProxy in JS (Task 48).
	//
	// Task 56: target(), setTarget(), isRunning() removed from
	// CaptureSession wrapper. All JS call sites use SessionManager
	// wrappers (tuiMux.session()) for these operations.

	// ── start() → Promise<void> ───────────────────────────
	// PTY/process startup can block, so it must never execute on the Goja
	// event-loop goroutine. Track the worker so shutdown joins it and settles
	// the promise on the owning runtime.
	_ = obj.Set("start", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(ctx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
			err := cs.Start(workerCtx)
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
		})
	})

	// ── interrupt() ──────────────────────────────────────
	_ = obj.Set("interrupt", func() goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, cs.Interrupt()
		})
	})

	// ── kill() ───────────────────────────────────────────
	_ = obj.Set("kill", func() goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, cs.Kill()
		})
	})

	// ── pause() ─────────────────────────────────────────
	// T059: Send SIGSTOP to suspend the child process.
	_ = obj.Set("pause", func() goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, cs.Pause()
		})
	})

	// ── resume() ────────────────────────────────────────
	// T059: Send SIGCONT to resume a paused child process.
	_ = obj.Set("resume", func() goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, cs.Resume()
		})
	})

	// ── isPaused() → boolean ────────────────────────────
	// T059: Check if the child process is currently paused.
	_ = obj.Set("isPaused", func() bool {
		return cs.IsPaused()
	})

	// ── resize(rows, cols) ───────────────────────────────
	_ = obj.Set("resize", func(rows, cols int) goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, cs.Resize(rows, cols)
		})
	})

	// ── wait() → Promise<{ code, error? }> ─────────────────
	// Async per JS Binding Contract: waits until child process exits and output is drained.
	_ = obj.Set("wait", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(ctx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
			stop := context.AfterFunc(workerCtx, func() { _ = cs.Kill() })
			defer stop()
			res, err := func(ctx context.Context) (any, error) {
				code, err := cs.WaitContext(workerCtx)
				result := map[string]any{"code": code}
				if err != nil {
					result["error"] = err.Error()
				}
				return result, nil
			}(workerCtx)
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				if res == nil {
					return goja.Undefined()
				}
				return res
			})
		})
	})

	// ── sendEOF() ────────────────────────────────────────
	_ = obj.Set("sendEOF", func() goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, cs.SendEOF()
		})
	})

	// ── close() → Promise<void> ───────────────────────────
	// Closing a PTY may signal a process and wait for reader shutdown, so keep
	// it off the Goja event-loop goroutine and join it through the adapter.
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		return adapter.TrackPromise(ctx, func(workerCtx context.Context, settle gojaeventloop.TrackedSettlement) {
			err := cs.Close()
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
		})
	})

	// ── pid() → number ──────────────────────────────────
	_ = obj.Set("pid", func() int {
		return cs.Pid()
	})

	// ── exitCode() → number ──────────────────────────────
	_ = obj.Set("exitCode", func() int {
		return cs.ExitCode()
	})

	// ── isDone() → boolean ───────────────────────────────
	// Non-blocking check: true if the child has exited and output is drained.
	_ = obj.Set("isDone", func() bool {
		select {
		case <-cs.Done():
			return true
		default:
			return false
		}
	})

	// ── passthrough(toggleKey?) → Promise<{ reason, error? }> ────
	// Async: enters raw passthrough mode, resolves when user toggles or child exits.
	// Uses os.Stdin/os.Stdout and the real terminal state. The caller (BubbleTea's
	// toggleModel) must have already released the terminal before calling this.
	_ = obj.Set("passthrough", func(call goja.FunctionCall) goja.Value {
		toggleKey := byte(parent.DefaultToggleKey)
		if len(call.Arguments) > 0 && !goja.IsUndefined(call.Argument(0)) && !goja.IsNull(call.Argument(0)) {
			cfgObj := call.Argument(0).ToObject(runtime)
			if cfgObj != nil {
				if v := cfgObj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
					toggleKey = byte(v.ToInteger())
				}
			}
		}

		return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
			res, err := func(ctx context.Context) (any, error) {
				termFd := int(os.Stdin.Fd())
				reason, err := cs.Passthrough(ctx, parent.PassthroughConfig{
					Stdin:         os.Stdin,
					Stdout:        os.Stdout,
					TermFd:        termFd,
					BlockingGuard: parent.DefaultBlockingGuard(),
					ToggleKey:     toggleKey,
					TermState:     ptyio.RealTermState{},
					// Host signals received while the terminal is handed over
					// (SIGINT/SIGQUIT/SIGTSTP) reach the child instead of being
					// absorbed by the notify channel — tmux-class passthrough
					// behavior; without this, watchSignals swallows them.
					SignalChild: func(sig string) error { return cs.Signal(sig) },
				})
				result := map[string]any{
					"reason": exitReasonString(reason),
				}
				if err != nil {
					result["error"] = err.Error()
				}
				return result, nil
			}(ctx)
			if err != nil {
				_ = settle.Settle(true, func(rt *goja.Runtime) any { return rt.NewGoError(err) })
				return
			}
			_ = settle.Settle(false, func(rt *goja.Runtime) any {
				if res == nil {
					return goja.Undefined()
				}
				return res
			})
		})
	})

	return obj
}

// wrapInteractiveSession wraps a [parent.InteractiveSession] into a Goja
// object with JavaScript-callable methods. This is the shared base for both
// [SessionManager] session wrappers (via [WrapSessionManager].session())
// and [CaptureSession] wrappers (via [WrapCaptureSession]).
//
// Exported methods (6 total, matching the trimmed InteractiveSession interface):
//
//	resize, write, close, isDone, readAvailable.
//
// CaptureSession wrappers add concrete-type-specific methods
// (start, interrupt, kill, pause, resume, isPaused, wait, sendEOF,
// pid, exitCode, passthrough).
func wrapInteractiveSession(ctx context.Context, adapter *gojaeventloop.Adapter, runtime *goja.Runtime, session parent.InteractiveSession, defaultKind parent.SessionKind) goja.Value {
	obj := runtime.NewObject()

	// Store the Go session for later retrieval by unwrapInteractiveSession.
	// Non-enumerable so it doesn't appear in Object.keys().
	_ = obj.DefineDataProperty("_goSession", runtime.ToValue(session),
		goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)

	_ = obj.Set("resize", func(rows, cols int) goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, session.Resize(rows, cols)
		})
	})

	_ = obj.Set("write", func(data string) goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			_, err := session.Write([]byte(data))
			return nil, err
		})
	})

	_ = obj.Set("sendKeys", func(keys ...string) goja.Value {
		var buf strings.Builder
		for _, key := range keys {
			seq, ok := parent.KeyToTermBytes(key, false, false)
			if !ok {
				panic(runtime.NewTypeError("sendKeys: unrecognized key " + key))
			}
			buf.WriteString(seq)
		}
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			_, err := session.Write([]byte(buf.String()))
			return nil, err
		})
	})

	_ = obj.Set("close", func() goja.Value {
		return trackSessionOperation(ctx, adapter, runtime, func() (any, error) {
			return nil, session.Close()
		})
	})

	_ = obj.Set("isDone", func() bool {
		select {
		case <-session.Done():
			return true
		default:
			return false
		}
	})

	// readAvailable() drains all currently-buffered chunks from the Reader()
	// channel without blocking. Returns an empty string when nothing is
	// buffered and null when the channel is closed. Useful for polling loops
	// in synchronous JS contexts (Goja has no setTimeout).
	_ = obj.Set("readAvailable", func() goja.Value {
		ch := session.Reader()
		if ch == nil {
			return goja.Null()
		}
		var buf []byte
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					if len(buf) > 0 {
						return runtime.ToValue(string(buf))
					}
					return goja.Null()
				}
				buf = append(buf, data...)
			default:
				return runtime.ToValue(string(buf))
			}
		}
	})

	return obj
}

// unwrapInteractiveSession retrieves the Go InteractiveSession stored on a
// JS wrapper object by wrapInteractiveSession. Returns nil if the object
// does not contain a _goSession property.
func unwrapInteractiveSession(obj *goja.Object) parent.InteractiveSession {
	v := obj.Get("_goSession")
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	session, _ := v.Export().(parent.InteractiveSession)
	return session
}
