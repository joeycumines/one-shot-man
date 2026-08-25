package scripting

import (
	"context"
	"time"

	"github.com/joeycumines/goja"
	"github.com/joeycumines/one-shot-man/internal/builtin/async"
	builtinos "github.com/joeycumines/one-shot-man/internal/builtin/os"
)

// JavaScript API functions for terminal output

// jsOutputPrint prints to terminal output.
func (e *Engine) jsOutputPrint(msg string) {
	e.logger.PrintToTUI(msg)
}

// jsOutputPrintf prints formatted text to terminal output.
func (e *Engine) jsOutputPrintf(format string, args ...any) {
	e.logger.PrintfToTUI(format, args...)
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
		tuiSink := func(s string) { e.logger.PrintToTUI(s) }
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
	clipCtx, cancel := context.WithTimeout(e.ctx, outputClipboardTimeout)
	defer cancel()
	return async.PromiseTracked(e.Adapter(), e.Loop(), clipCtx, fn, nil)
}
