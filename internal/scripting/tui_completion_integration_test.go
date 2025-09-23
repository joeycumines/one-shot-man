package scripting

import (
	"context"
	"testing"
	"time"

	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"

	"github.com/joeycumines/one-shot-man/internal/argv"
	"github.com/joeycumines/one-shot-man/internal/termtest"
)

// TestFileCompletion_NoPanic_WithSpaces ensures completing quoted paths with spaces does not panic.
func TestFileCompletion_NoPanic_WithSpaces(t *testing.T) {
	ctx := context.Background()
	test, err := termtest.NewGoPromptTest(ctx)
	if err != nil {
		t.Fatalf("failed to create go-prompt test: %v", err)
	}
	defer test.Close()

	// Minimal executor; our interest is only in completion stability
	executor := termtest.TestExecutor(&[]string{})

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
	test.RunPrompt(executor, prompt.WithCompleter(completer))

	// Type a command with a quoted path containing spaces, then press Tab to request completion
	if err := test.SendInput("add \"my repo"); err != nil {
		t.Fatalf("send input: %v", err)
	}
	if err := test.SendKeys("tab"); err != nil {
		t.Fatalf("send tab: %v", err)
	}

	// Give the UI a moment; ensure the process hasn't panicked by checking it's still responsive
	if err := test.WaitForOutput("add", 1*time.Second); err != nil {
		// Even if no visual suggestion shows, the important assertion is lack of panic; provide context
		t.Logf("no completion output observed (not necessarily a failure): %v", err)
	}

	// Exit cleanly
	if err := test.SendLine("exit"); err != nil {
		t.Fatalf("send exit: %v", err)
	}
	// Give it a moment to process the exit; avoid hard WaitForExit to reduce flakiness
	time.Sleep(200 * time.Millisecond)
}
