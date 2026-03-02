package command

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Package-level integration test flags, parsed by TestMain.
//
// Usage:
//
//	go test -race -v -count=1 -integration \
//	  -ollama-command=ollama \
//	  ./internal/command/...
//
// Or with Claude Code:
//
//	go test -race -v -count=1 -integration \
//	  -claude-command=ollama -claude-arg=launch -claude-arg=claude \
//	  -claude-arg=--model=minimax-m2.5:cloud -claude-arg=-- \
//	  ./internal/command/... -run 'TestIntegration_.*Claude'
var (
	integrationEnabled bool
	ollamaCommand      string
	integrationModel   string

	// Claude Code test configuration — passed to auto-split integration tests.
	claudeTestCommand string          // path/name of the Claude binary
	claudeTestArgs    stringSliceFlag // additional CLI arguments (repeatable)
)

func TestMain(m *testing.M) {
	flag.BoolVar(&integrationEnabled, "integration", false,
		"enable integration tests that require real agent infrastructure")
	flag.StringVar(&ollamaCommand, "ollama-command", "",
		"path to ollama binary for integration tests (empty = skip ollama tests)")
	flag.StringVar(&integrationModel, "integration-model", "gpt-oss:20b-cloud",
		"model to use for integration tests")
	flag.StringVar(&claudeTestCommand, "claude-command", "",
		"path to Claude binary for pr-split integration tests (empty = skip Claude tests)")
	flag.Var(&claudeTestArgs, "claude-arg",
		"additional CLI argument for Claude binary (repeatable, e.g. -claude-arg=launch -claude-arg=claude)")
	flag.Parse()
	os.Exit(m.Run())
}

// skipIfNotIntegration skips the calling test if -integration was not passed.
func skipIfNotIntegration(t *testing.T) {
	t.Helper()
	if !integrationEnabled {
		t.Skip("integration tests disabled; use -integration flag to enable")
	}
}

// skipIfNoOllama skips the calling test if -ollama-command was not provided.
func skipIfNoOllama(t *testing.T) {
	t.Helper()
	skipIfNotIntegration(t)
	if ollamaCommand == "" {
		t.Skip("ollama integration tests disabled; use -ollama-command=<path> to enable")
	}
}

// skipIfNoClaude skips the calling test if -claude-command was not provided.
func skipIfNoClaude(t *testing.T) {
	t.Helper()
	skipIfNotIntegration(t)
	if claudeTestCommand == "" {
		t.Skip("Claude integration tests disabled; use -claude-command=<path> to enable")
	}
}

// verifyClaudeAuth runs a minimal Claude -p (print/headless) check to
// verify the configured Claude command is authenticated and functional.
// Skips the test if Claude cannot process a prompt (e.g., not logged in,
// no API key, model unavailable).
//
// This catches the common failure mode where Claude Code's interactive TUI
// shows "Not logged in · Run /login" — in TUI mode, authentication is
// required and prompts won't be processed without it.
func verifyClaudeAuth(t *testing.T) {
	t.Helper()

	args := []string{"-p", "Reply with exactly: AUTH_OK", "--max-turns", "1"}
	if integrationModel != "" {
		args = append(args, "--model", integrationModel)
	}
	// Copy any extra Claude args (but filter out --dangerously-skip-permissions
	// which is for interactive mode only).
	for _, a := range claudeTestArgs {
		if a != "--dangerously-skip-permissions" {
			args = append(args, a)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Logf("verifyClaudeAuth: running %s %s", claudeTestCommand, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, claudeTestCommand, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Claude auth check failed (run 'claude login' or set ANTHROPIC_API_KEY):\n  command: %s %s\n  error: %v\n  output: %s",
			claudeTestCommand, strings.Join(args, " "), err, string(out))
	}
	if !strings.Contains(string(out), "AUTH_OK") {
		t.Logf("verifyClaudeAuth: Claude responded but did not contain AUTH_OK: %s", string(out))
		// Still proceed — Claude is at least functional even if it didn't follow
		// the exact instruction. The important thing is that it responded at all.
	}
	t.Log("verifyClaudeAuth: Claude is authenticated and functional")
}
