package command

import (
	"bytes"
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// ─────────────────────────────────────────────────────────────────────
// T073: Strict argument validation tests
//
// These tests simulate the dispatch flow in cmd/osm/main.go:
//   cmd.SetupFlags(fs) → fs.Parse(args) → cmd.Execute(fs.Args(), ...)
// ─────────────────────────────────────────────────────────────────────

// executeWithFlags simulates the real dispatch: SetupFlags + Parse + Execute.
func executeWithFlags(t *testing.T, cmd interface {
	SetupFlags(*flag.FlagSet)
	Execute([]string, io.Writer, io.Writer) error
}, args []string) (stdout, stderr bytes.Buffer, err error) {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&stderr)
	cmd.SetupFlags(fs)
	if parseErr := fs.Parse(args); parseErr != nil {
		return stdout, stderr, parseErr
	}
	err = cmd.Execute(fs.Args(), &stdout, &stderr)
	return
}

// ── GoalCommand ─────────────────────────────────────────────────────

func newArgValidationGoalRegistry() GoalRegistry {
	return &staticGoalRegistry{goals: []Goal{
		{Name: "test-goal", Description: "A test goal"},
	}}
}

func TestGoalCommand_ListRejectsExtraArgs(t *testing.T) {
	cmd := NewGoalCommand(nil, newArgValidationGoalRegistry())
	_, _, err := executeWithFlags(t, cmd, []string{"-l", "extra-arg"})
	if err == nil {
		t.Fatal("expected error for extra args with -l")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGoalCommand_RunRejectsExtraArgs(t *testing.T) {
	cmd := NewGoalCommand(nil, newArgValidationGoalRegistry())
	_, _, err := executeWithFlags(t, cmd, []string{"-r", "some-goal", "extra-arg"})
	if err == nil {
		t.Fatal("expected error for extra args with -r")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestGoalCommand_PositionalRejectsExtraArgs(t *testing.T) {
	cmd := NewGoalCommand(nil, newArgValidationGoalRegistry())
	_, _, err := executeWithFlags(t, cmd, []string{"my-goal", "extra-arg"})
	if err == nil {
		t.Fatal("expected error for extra positional args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── HelpCommand ─────────────────────────────────────────────────────

func TestHelpCommand_RejectsExtraArgs(t *testing.T) {
	reg := NewRegistryWithConfig(config.NewConfig())
	cmd := NewHelpCommand(reg)
	var stdout, stderr bytes.Buffer
	// HelpCommand.Execute receives args directly (no SetupFlags).
	err := cmd.Execute([]string{"version", "extra-arg"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── ConfigCommand ───────────────────────────────────────────────────

func TestConfigCommand_ValidateRejectsExtraArgs(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, "")
	var stdout, stderr bytes.Buffer
	// ConfigCommand receives subcommand + args directly.
	err := cmd.Execute([]string{"validate", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestConfigCommand_SchemaRejectsExtraArgs(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, "")
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"schema", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── SessionCommand ──────────────────────────────────────────────────

func newTestSessionCommandForValidation(t *testing.T) *SessionCommand {
	t.Helper()
	tmpDir := t.TempDir()
	storage.SetTestPaths(tmpDir)
	t.Cleanup(storage.ResetPaths)
	cmd := NewSessionCommand(nil)
	cmd.stdin = strings.NewReader("")
	return cmd
}

func TestSessionCommand_IdRejectsExtraArgs(t *testing.T) {
	cmd := newTestSessionCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"id", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "session id: unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSessionCommand_ListRejectsExtraArgs(t *testing.T) {
	cmd := newTestSessionCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"list", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "session list: unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSessionCommand_CleanRejectsExtraArgs(t *testing.T) {
	cmd := newTestSessionCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"clean", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "session clean: unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSessionCommand_PurgeRejectsExtraArgs(t *testing.T) {
	cmd := newTestSessionCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"purge", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "session purge: unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSessionCommand_InfoRejectsExtraArgs(t *testing.T) {
	cmd := newTestSessionCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"info", "some-id", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args after session id")
	}
	if !strings.Contains(err.Error(), "unexpected arguments after session id") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSessionCommand_PathRejectsExtraArgs(t *testing.T) {
	cmd := newTestSessionCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"path", "some-id", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments for path") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── SyncCommand ─────────────────────────────────────────────────────

func newTestSyncCommandForValidation(t *testing.T) *SyncCommand {
	t.Helper()
	return NewSyncCommand(nil, t.TempDir())
}

func TestSyncCommand_SaveRejectsExtraArgs(t *testing.T) {
	cmd := newTestSyncCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"save", "--title", "t", "--body", "b", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments for save") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSyncCommand_ListRejectsExtraArgs(t *testing.T) {
	cmd := newTestSyncCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"list", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments for list") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSyncCommand_InitRejectsExtraArgs(t *testing.T) {
	cmd := newTestSyncCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"init", "https://example.com/repo.git", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments for init") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSyncCommand_PushRejectsExtraArgs(t *testing.T) {
	cmd := newTestSyncCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"push", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments for push") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestSyncCommand_PullRejectsExtraArgs(t *testing.T) {
	cmd := newTestSyncCommandForValidation(t)
	_, _, err := executeWithFlags(t, cmd, []string{"pull", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments for pull") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── VersionCommand (already validated, verify) ──────────────────────

func TestVersionCommand_RejectsArgs(t *testing.T) {
	cmd := NewVersionCommand("1.0.0")
	_, _, err := executeWithFlags(t, cmd, []string{"extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── CompletionCommand (already validated, verify) ───────────────────

func TestCompletionCommand_RejectsExtraArgs(t *testing.T) {
	reg := NewRegistryWithConfig(config.NewConfig())
	cmd := NewCompletionCommand(reg, nil)
	_, _, err := executeWithFlags(t, cmd, []string{"bash", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !strings.Contains(err.Error(), "too many arguments") {
		t.Fatalf("wrong error: %v", err)
	}
}

// ── LogCommand (already validated, verify) ──────────────────────────

func TestLogCommand_RejectsUnknownSubcommand(t *testing.T) {
	cmd := NewLogCommand(nil)
	_, _, err := executeWithFlags(t, cmd, []string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestLogCommand_TailRejectsExtraArgs(t *testing.T) {
	cmd := NewLogCommand(nil)
	_, _, err := executeWithFlags(t, cmd, []string{"tail", "extra"})
	if err == nil {
		t.Fatal("expected error for extra args after tail")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("wrong error: %v", err)
	}
}
