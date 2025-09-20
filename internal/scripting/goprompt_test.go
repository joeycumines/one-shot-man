package scripting

import (
	"testing"
	"context"
	"time"
	"os"
	"io"
	"strings"
	"bytes"

	"github.com/creack/pty"
	"github.com/elk-language/go-prompt"
	istrings "github.com/elk-language/go-prompt/strings"
)

// TestGoPromptExitChecker tests that go-prompt exit mechanism works
func TestGoPromptExitChecker(t *testing.T) {
	// Create a PTY pair for testing
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Fatalf("failed to open pty: %v", err)
	}
	defer pts.Close() 
	defer ptm.Close()

	// Track execution
	executorCallCount := 0
	var receivedCommands []string

	// Simple executor that tracks commands
	executor := func(input string) {
		executorCallCount++
		trimmed := strings.TrimSpace(input)
		receivedCommands = append(receivedCommands, trimmed)
		t.Logf("Executor received: %q", trimmed)
	}

	// Simple completer
	completer := func(d prompt.Document) ([]prompt.Suggest, istrings.RuneNumber, istrings.RuneNumber) {
		word := d.GetWordBeforeCursor()
		startChar := istrings.RuneNumber(len(d.TextBeforeCursor()) - len(word))
		endChar := istrings.RuneNumber(len(d.TextBeforeCursor()))
		return []prompt.Suggest{
			{Text: "help", Description: "Show help"},
			{Text: "exit", Description: "Exit"},
		}, startChar, endChar
	}

	// Channel to signal when prompt exits
	promptDone := make(chan bool, 1)

	// Run prompt in goroutine with PTY
	go func() {
		defer func() {
			t.Logf("Prompt goroutine finishing")
			promptDone <- true
		}()

		// Redirect stdin/stdout to PTY slave
		oldStdin := os.Stdin
		oldStdout := os.Stdout
		os.Stdin = pts
		os.Stdout = pts

		defer func() {
			os.Stdin = oldStdin
			os.Stdout = oldStdout
		}()

		// Create prompt with exit checker
		p := prompt.New(
			executor,
			prompt.WithPrefix(">>> "),
			prompt.WithCompleter(completer),
			prompt.WithExitChecker(func(in string, breakline bool) bool {
				shouldExit := strings.TrimSpace(in) == "exit"
				t.Logf("ExitChecker called with %q, shouldExit: %v", in, shouldExit)
				return shouldExit
			}),
			prompt.WithExecuteOnEnterCallback(func(prompt *prompt.Prompt, indentSize int) (int, bool) {
				return 0, true
			}),
		)
		
		t.Logf("Starting prompt.Run()")
		p.Run()
		t.Logf("prompt.Run() finished")
	}()

	// Give prompt time to initialize
	time.Sleep(200 * time.Millisecond)

	// Read any initial output
	var initialBuf bytes.Buffer
	ptm.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	io.Copy(&initialBuf, ptm)
	t.Logf("Initial output: %q", initialBuf.String())

	// Send help command
	t.Logf("Sending 'help' command")
	_, err = ptm.Write([]byte("help\r"))
	if err != nil {
		t.Fatalf("failed to write help: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	// Read output after help
	var helpBuf bytes.Buffer
	ptm.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	io.Copy(&helpBuf, ptm)
	t.Logf("Output after help: %q", helpBuf.String())

	// Send exit command
	t.Logf("Sending 'exit' command")
	_, err = ptm.Write([]byte("exit\r"))
	if err != nil {
		t.Fatalf("failed to write exit: %v", err)
	}

	// Wait for prompt to exit with timeout
	select {
	case <-promptDone:
		t.Logf("Prompt exited successfully")
	case <-time.After(5 * time.Second):
		t.Fatal("Prompt did not exit within 5 seconds")
	}

	// Read any remaining output
	var buf bytes.Buffer
	ptm.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	io.Copy(&buf, ptm)

	output := buf.String()
	t.Logf("Final output:\n%s", output)

	// Verify executor was called
	if executorCallCount == 0 {
		t.Error("Executor was never called")
	}

	// Should have received at least the help command
	if len(receivedCommands) == 0 {
		t.Error("No commands were received by executor")
	} else {
		t.Logf("Received commands: %v", receivedCommands)
	}
}

// TestTUIManagerGoPromptIntegration tests TUIManager with PTY
func TestTUIManagerGoPromptIntegration(t *testing.T) {
	// Create PTY
	ptm, pts, err := pty.Open()
	if err != nil {
		t.Fatalf("failed to open pty: %v", err)
	}
	defer pts.Close()
	defer ptm.Close()

	// Create engine
	ctx := context.Background()
	engine := NewEngine(ctx, pts, pts)
	defer engine.Close()

	// Channel to track when TUI finishes
	tuiDone := make(chan bool, 1)

	// Run TUI in goroutine
	go func() {
		defer func() {
			tuiDone <- true
		}()

		// Redirect stdin to PTY
		oldStdin := os.Stdin
		os.Stdin = pts
		defer func() {
			os.Stdin = oldStdin
		}()

		// Run the TUI
		tuiManager := engine.GetTUIManager()
		tuiManager.Run()
	}()

	// Give TUI time to start
	time.Sleep(300 * time.Millisecond)

	// Send help command
	_, err = ptm.Write([]byte("help\r\n"))
	if err != nil {
		t.Fatalf("failed to write help: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Send exit command
	_, err = ptm.Write([]byte("exit\r\n"))
	if err != nil {
		t.Fatalf("failed to write exit: %v", err)
	}

	// Wait for TUI to exit
	select {
	case <-tuiDone:
		t.Logf("TUI exited successfully")
	case <-time.After(10 * time.Second):
		t.Fatal("TUI did not exit within 10 seconds")
	}

	// Read output
	var buf bytes.Buffer
	ptm.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	io.Copy(&buf, ptm)

	output := buf.String()
	t.Logf("TUI output:\n%s", output)

	// Verify we see expected TUI elements
	if !strings.Contains(output, "one-shot-man Rich TUI Terminal") {
		t.Error("Expected to see TUI terminal header")
	}
}