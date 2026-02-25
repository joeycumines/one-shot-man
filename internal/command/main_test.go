package command

import (
	"flag"
	"os"
	"testing"
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
