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
// Or with Agent Code:
//
//	go test -race -v -count=1 -integration \
//	  -agent-command=ollama -agent-arg=launch -agent-arg=agent \
//	  -agent-arg=--model=minimax-m2.5:cloud -agent-arg=-- \
//	  ./internal/command/... -run 'TestIntegration_.*Agent'
var (
	integrationEnabled bool
	ollamaCommand      string
	integrationModel   string

	// Agent Code test configuration — passed to auto-split integration tests.
	agentTestCommand string          // path/name of the Agent binary
	agentTestArgs    stringSliceFlag // additional CLI arguments (repeatable)
)

func TestMain(m *testing.M) {
	flag.BoolVar(&integrationEnabled, "integration", false,
		"enable integration tests that require real agent infrastructure")
	flag.StringVar(&ollamaCommand, "ollama-command", "",
		"path to ollama binary for integration tests (empty = skip ollama tests)")
	flag.StringVar(&integrationModel, "integration-model", "minimax-m2.5:cloud",
		"model to use for integration tests")
	flag.StringVar(&agentTestCommand, "agent-command", "",
		"path to Agent binary for pr-split integration tests (empty = skip Agent tests)")
	flag.Var(&agentTestArgs, "agent-arg",
		"additional CLI argument for Agent binary (repeatable, e.g. -agent-arg=launch -agent-arg=agent)")
	flag.Parse()
	os.Exit(m.Run())
}

// skipSlow skips the calling test when -short mode is active. All slow
// integration and E2E tests call this at the top of the function body so
// that `go test -short` provides a fast feedback loop while `go test`
// (without -short) runs the full suite.
func skipSlow(tb testing.TB) {
	tb.Helper()
	if testing.Short() {
		tb.Skip("slow test skipped in -short mode")
	}
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

// skipIfNoAgent skips the calling test if -agent-command was not provided.
func skipIfNoAgent(t *testing.T) {
	t.Helper()
	skipIfNotIntegration(t)
	if agentTestCommand == "" {
		t.Skip("Agent integration tests disabled; use -agent-command=<path> to enable")
	}
}

// verifyAgentAuth runs a minimal Agent -p (print/headless) check to
// verify the configured Agent command is authenticated and functional.
// Skips the test if Agent cannot process a prompt (e.g., not logged in,
// no API key, model unavailable).
//
// This catches the common failure mode where Agent Code's interactive TUI
// shows "Not logged in · Run /login" — in TUI mode, authentication is
// required and prompts won't be processed without it.
func verifyAgentAuth(t *testing.T) {
	t.Helper()

	args := []string{"-p", "Reply with exactly: AUTH_OK", "--max-turns", "1"}
	if integrationModel != "" {
		args = append(args, "--model", integrationModel)
	}
	// Copy any extra Agent args (but filter out --dangerously-skip-permissions
	// which is for interactive mode only).
	for _, a := range agentTestArgs {
		if a != "--dangerously-skip-permissions" {
			args = append(args, a)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	t.Logf("verifyAgentAuth: running %s %s", agentTestCommand, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, agentTestCommand, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("Agent auth check failed (run 'agent login' or set ANTHROPIC_API_KEY):\n  command: %s %s\n  error: %v\n  output: %s",
			agentTestCommand, strings.Join(args, " "), err, string(out))
	}
	if !strings.Contains(string(out), "AUTH_OK") {
		t.Logf("verifyAgentAuth: Agent responded but did not contain AUTH_OK: %s", string(out))
		// Still proceed — Agent is at least functional even if it didn't follow
		// the exact instruction. The important thing is that it responded at all.
	}
	t.Log("verifyAgentAuth: Agent is authenticated and functional")
}
