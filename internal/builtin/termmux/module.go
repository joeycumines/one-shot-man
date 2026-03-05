// Package termmux provides JavaScript bindings for the terminal multiplexer,
// registered as the "osm:termmux" native module. It wraps
// [github.com/joeycumines/one-shot-man/internal/termmux] to expose pane
// management, passthrough control, and configuration to Goja scripts.
package termmux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/dop251/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// Require returns a module loader for "osm:termmux" that exposes the terminal
// multiplexer to JavaScript. The input/output parameters are optional; when nil
// the module falls back to os.Stdin/os.Stdout.
func Require(ctx context.Context, input io.Reader, output io.Writer) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// ── Constants ────────────────────────────────────────
		_ = exports.Set("EXIT_TOGGLE", "toggle")
		_ = exports.Set("EXIT_CHILD_EXIT", "childExit")
		_ = exports.Set("EXIT_CONTEXT", "context")
		_ = exports.Set("EXIT_ERROR", "error")
		_ = exports.Set("SIDE_OSM", "osm")
		_ = exports.Set("SIDE_CLAUDE", "claude")
		_ = exports.Set("DEFAULT_TOGGLE_KEY", int(parent.DefaultToggleKey))

		// ── Constructor ──────────────────────────────────────
		_ = exports.Set("newMux", func(call goja.FunctionCall) goja.Value {
			return newMux(ctx, runtime, input, output, call)
		})
	}
}

// newMux creates a [parent.Mux] from an optional JS options object and returns
// a wrapped JS object exposing all multiplexer methods.
//
// JS signature:
//
//	termmux.newMux({ toggleKey?: number, statusEnabled?: boolean, initialStatus?: string })
func newMux(ctx context.Context, runtime *goja.Runtime, input io.Reader, output io.Writer, call goja.FunctionCall) goja.Value {
	var opts []parent.Option

	// Parse optional config object.
	if arg := call.Argument(0); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		obj := arg.ToObject(runtime)
		if v := obj.Get("toggleKey"); v != nil && !goja.IsUndefined(v) {
			key := byte(v.ToInteger())
			opts = append(opts, func(c *parent.Config) { c.ToggleKey = key })
		}
		if v := obj.Get("statusEnabled"); v != nil && !goja.IsUndefined(v) {
			enabled := v.ToBoolean()
			opts = append(opts, func(c *parent.Config) { c.StatusEnabled = enabled })
		}
		if v := obj.Get("initialStatus"); v != nil && !goja.IsUndefined(v) {
			status := v.String()
			opts = append(opts, func(c *parent.Config) { c.InitialStatus = status })
		}
	}

	// Resolve terminal I/O from provided args or OS defaults.
	stdin := input
	stdout := output
	termFd := int(os.Stdin.Fd())
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}

	mux := parent.New(stdin, stdout, termFd, opts...)
	return WrapMux(ctx, runtime, mux)
}

// WrapMux wraps a [parent.Mux] into a Goja object with JavaScript-callable methods.
// This is exported so callers (e.g., pr_split.go) can create a Go-side Mux and
// expose it to JS through the same standardized interface that newMux() provides.
func WrapMux(ctx context.Context, runtime *goja.Runtime, mux *parent.Mux) goja.Value {
	obj := runtime.NewObject()

	// ── attach(handle) ───────────────────────────────────
	// Accepts AgentHandle (map with _goHandle), StringIO, or io.ReadWriteCloser.
	// On ErrAlreadyAttached, detaches first then retries once.
	_ = obj.Set("attach", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) == 0 {
			panic(runtime.NewTypeError("attach: handle argument is required"))
		}
		raw := call.Argument(0).Export()
		child, err := resolveChild(raw)
		if err != nil {
			panic(runtime.NewTypeError(fmt.Sprintf("attach: %v", err)))
		}
		if err := attachWithRetry(mux, child); err != nil {
			panic(runtime.NewGoError(err))
		}
		return goja.Undefined()
	})

	// ── detach() ─────────────────────────────────────────
	// Idempotent — safe to call with no child attached.
	_ = obj.Set("detach", func() {
		if err := mux.Detach(); err != nil {
			panic(runtime.NewGoError(err))
		}
	})

	// ── hasChild() → boolean ─────────────────────────────
	_ = obj.Set("hasChild", func() bool {
		return mux.HasChild()
	})

	// ── switchTo() → { reason, error?, childOutput? } ────
	// BLOCKING: enters raw passthrough mode, returns when user toggles or child exits.
	_ = obj.Set("switchTo", func() map[string]interface{} {
		reason, err := mux.RunPassthrough(ctx)
		result := map[string]interface{}{
			"reason": exitReasonString(reason),
		}
		if err != nil {
			result["error"] = err.Error()
		}
		if reason == parent.ExitChildExit {
			if output := mux.ChildExitOutput(); output != "" {
				result["childOutput"] = output
			}
		}
		return result
	})

	// ── activeSide() → "osm" | "claude" ─────────────────
	_ = obj.Set("activeSide", func() string {
		if mux.ActiveSide() == parent.SideClaude {
			return "claude"
		}
		return "osm"
	})

	// ── setStatus(text) ──────────────────────────────────
	_ = obj.Set("setStatus", func(s string) {
		mux.SetClaudeStatus(s)
	})

	// ── setToggleKey(keyByte) ────────────────────────────
	_ = obj.Set("setToggleKey", func(k int) {
		mux.SetToggleKey(byte(k))
	})

	// ── setStatusEnabled(bool) ───────────────────────────
	_ = obj.Set("setStatusEnabled", func(b bool) {
		mux.SetStatusEnabled(b)
	})

	// ── setResizeFunc(fn) ────────────────────────────────
	_ = obj.Set("setResizeFunc", func(fn func(int, int)) {
		mux.SetResizeFunc(func(rows, cols uint16) error {
			fn(int(rows), int(cols))
			return nil
		})
	})

	// ── screenshot() → string ────────────────────────────
	// Returns plain-text VTerm buffer content for diagnostics/test assertions.
	_ = obj.Set("screenshot", func() string {
		return mux.ChildExitOutput()
	})

	return obj
}

// resolveChild converts a JS-exported value into an [io.ReadWriteCloser]
// suitable for [parent.Mux.Attach]. Supports:
//   - [parent.StringIO] (Send/Receive/Close)
//   - map with "_goHandle" key containing StringIO or io.ReadWriteCloser
//   - [io.ReadWriteCloser] directly
func resolveChild(raw interface{}) (io.ReadWriteCloser, error) {
	// Case 1: Direct StringIO (Go test callers).
	if sio, ok := raw.(parent.StringIO); ok {
		return parent.WrapStringIO(sio), nil
	}

	// Case 2: Goja-wrapped AgentHandle — map with _goHandle.
	if m, ok := raw.(map[string]interface{}); ok {
		if goHandle, exists := m["_goHandle"]; exists && goHandle != nil {
			if sio, ok := goHandle.(parent.StringIO); ok {
				return parent.WrapStringIO(sio), nil
			}
			if rwc, ok := goHandle.(io.ReadWriteCloser); ok {
				return rwc, nil
			}
		}
	}

	// Case 3: Direct io.ReadWriteCloser.
	if rwc, ok := raw.(io.ReadWriteCloser); ok {
		return rwc, nil
	}

	return nil, errors.New("argument must implement Send/Receive/Close (or be a wrapped AgentHandle with _goHandle)")
}

// attachWithRetry calls [parent.Mux.Attach] with automatic detach-and-retry
// on [parent.ErrAlreadyAttached].
func attachWithRetry(mux *parent.Mux, child io.ReadWriteCloser) error {
	err := mux.Attach(child)
	if err != nil && errors.Is(err, parent.ErrAlreadyAttached) {
		if detachErr := mux.Detach(); detachErr != nil {
			return fmt.Errorf("detach before reattach: %w", detachErr)
		}
		err = mux.Attach(child)
	}
	return err
}

// exitReasonString maps a [parent.ExitReason] to its JS string constant.
func exitReasonString(r parent.ExitReason) string {
	switch r {
	case parent.ExitToggle:
		return "toggle"
	case parent.ExitChildExit:
		return "childExit"
	case parent.ExitContext:
		return "context"
	case parent.ExitError:
		return "error"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}
