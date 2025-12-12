//go:build unix

package scripting

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt"
	istrings "github.com/joeycumines/go-prompt/strings"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/argv"
)

// TestFileCompletion_NoPanic_WithSpaces ensures completing quoted paths with spaces does not panic.
func TestFileCompletion_NoPanic_WithSpaces(t *testing.T) {
	ctx := context.Background()
	h, err := termtest.NewHarness(ctx)
	if err != nil {
		t.Fatalf("failed to create harness: %v", err)
	}
	defer h.Close()

	// Use a minimal TUIManager solely for completion logic
	tm := &TUIManager{commands: map[string]Command{}, commandOrder: []string{}, modes: map[string]*ScriptMode{}}
	tm.RegisterCommand(Command{Name: "add", Description: "Add", ArgCompleters: []string{"file"}, IsGoCommand: true, Handler: func([]string) error { return nil }})

	// Bridge our completer into go-prompt
	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		sugg := tm.getDefaultCompletionSuggestions(d)
		before := d.TextBeforeCursor()
		_, cur := argv.BeforeCursor(before)
		return sugg, istrings.RuneNumber(cur.Start), istrings.RuneNumber(cur.End)
	}

	// Start the prompt with our completer
	h.RunPrompt(h.Executor, prompt.WithCompleter(completer))

	// Type a command with a quoted path containing spaces, then press Tab to request completion
	{
		ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := h.Console().WriteSync(ctx, "add \"my repo"); err != nil {
			t.Fatalf("send input: %v", err)
		}
	}

	// Capture snapshot BEFORE sending tab
	snap := h.Console().Snapshot()
	{
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		if err := h.Console().SendSync(ctx, "tab"); err != nil {
			t.Fatalf("send tab: %v", err)
		}
	}

	// Give the UI a moment; ensure the process hasn't panicked by checking it's still responsive
	{
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()
		if err := h.Console().Expect(ctx, snap, termtest.Contains("add"), "checking completion non-panic"); err != nil {
			// Even if no visual suggestion shows, the important assertion is lack of panic; provide context
			t.Logf("no completion output observed (not necessarily a failure): %v", err)
		}
	}

	// Exit cleanly - clear current input, then send a fresh 'exit' line
	{
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := h.Console().SendSync(ctx, "ctrl+u"); err != nil {
			t.Fatalf("clear input ctrl+u: %v", err)
		}
		if err := h.Console().WriteSync(ctx, "exit"); err != nil {
			t.Fatalf("write exit: %v", err) // Fixed typo in log message here as well
		}
		if err := h.Console().SendSync(ctx, "enter"); err != nil {
			t.Fatalf("send exit enter: %v", err)
		}
	}

	// final enter just in case it showed the completion menu
	_ = h.Console().Send("enter")

	{
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		if err := h.WaitExit(ctx); err != nil {
			t.Errorf("wait for prompt exit: %v\nOUTPUT: %q", err, h.Console())
		}
	}
}
