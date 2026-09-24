package scripting

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/joeycumines/goja"
	builtinos "github.com/joeycumines/one-shot-man/internal/builtin/os"
)

// JavaScript API functions for terminal output

func (e *Engine) jsSetTUIOutputActive(active bool) {
	e.tuiOutputActive.Store(active)
}

// jsOutputPrint prints to terminal output.
func (e *Engine) jsOutputPrint(msg string) {
	if e.routeActiveTUIOutput(msg) {
		return
	}
	e.logger.PrintToTUI(msg)
}

// jsOutputPrintf prints formatted text to terminal output.
func (e *Engine) jsOutputPrintf(format string, args ...any) {
	msg := fmt.Sprintf(jsNormalizePrintfFormat(format), args...)
	if e.routeActiveTUIOutput(msg) {
		return
	}
	e.logger.PrintToTUI(msg)
}

// routeActiveTUIOutput hands user-visible output to the active pr-split
// model. The binding is normally called by Goja on the event-loop owner;
// direct Go-level tests and non-Goja callers retain the logger fallback.
func (e *Engine) routeActiveTUIOutput(msg string) bool {
	if !e.tuiOutputActive.Load() || e.vm == nil {
		return false
	}

	root := e.vm.Get("prSplit")
	if root == nil || goja.IsUndefined(root) || goja.IsNull(root) {
		return false
	}
	obj := root.ToObject(e.vm)
	if !obj.Get("_tuiOutputActive").ToBoolean() {
		return false
	}

	route, ok := goja.AssertFunction(obj.Get("_routeTuiOutput"))
	if !ok {
		e.logger.Error("tui output route unavailable")
		return true
	}
	if _, err := route(goja.Undefined(), e.vm.ToValue(msg)); err != nil {
		e.logger.Error("tui output route failed", slog.String("error", err.Error()))
	}
	return true
}

// outputClipboardTimeout caps a clipboard subprocess (pbcopy/xclip/clip).
const outputClipboardTimeout = 10 * time.Second

// jsOutputToClipboard copies text to the system clipboard.
//
// Returns a Promise<void> that resolves on success / rejects on failure. This
// is ASYNC per the JS Binding Contract (CLAUDE.md): clipboard is subprocess
// I/O and must run off the event loop via Promisify — the previous synchronous
// form monopolized the loop for up to 10s. Mirrors osm:os.clipboardCopy.
//
// NOTE: callers in sync (Elm-style) update handlers must fire-and-forget the
// returned Promise and handle the flash in a .then/.catch (see the pr-split
// TUI handlers), since the handler cannot await.
func (e *Engine) jsOutputToClipboard(text string) goja.Value {
	return e.clipboardPromise(func(ctx context.Context) (any, error) {
		tuiSink := func(msg string) {
			if e.logger != nil {
				e.logger.PrintToTUI(msg)
			}
		}
		if err := builtinos.ClipboardCopy(ctx, tuiSink, text); err != nil {
			return nil, err
		}
		return nil, nil
	})
}

// jsOutputFromClipboard reads text from the system clipboard.
//
// Returns a Promise<string> (async per the JS Binding Contract; was sync).
// Callers that need the value must await it (or consume it in a .then).
func (e *Engine) jsOutputFromClipboard() goja.Value {
	return e.clipboardPromise(func(ctx context.Context) (any, error) {
		text, err := builtinos.ClipboardPaste(ctx)
		if err != nil {
			return nil, err
		}
		return text, nil
	})
}

func (e *Engine) clipboardPromise(fn func(ctx context.Context) (any, error)) goja.Value {
	return e.Adapter().Promisify(e.ctx, func(ctx context.Context) (any, error) {
		clipCtx, cancel := context.WithTimeout(ctx, outputClipboardTimeout)
		defer cancel()
		return fn(clipCtx)
	})
}
