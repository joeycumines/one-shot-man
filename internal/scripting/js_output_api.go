package scripting

import (
	"context"
	"time"

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

// jsOutputToClipboard copies text to the system clipboard.
// Returns true on success, throws a JS error on failure.
// T088: Platform-specific clipboard support via pbcopy/xclip/clip.
func (e *Engine) jsOutputToClipboard(text string) {
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()

	tuiSink := func(s string) {
		e.logger.PrintToTUI(s)
	}

	if err := builtinos.ClipboardCopy(ctx, tuiSink, text); err != nil {
		panic(e.vm.NewGoError(err))
	}
}

// jsOutputFromClipboard reads text from the system clipboard.
// Returns the clipboard content as a string, throws a JS error on failure.
func (e *Engine) jsOutputFromClipboard() string {
	ctx, cancel := context.WithTimeout(e.ctx, 10*time.Second)
	defer cancel()

	text, err := builtinos.ClipboardPaste(ctx)
	if err != nil {
		panic(e.vm.NewGoError(err))
	}
	return text
}
