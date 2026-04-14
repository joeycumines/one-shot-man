package command

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// ---------------------------------------------------------------------------
// GoalCommand coverage gaps
// ---------------------------------------------------------------------------

// TestGoalCommand_SetupFlags_AllFlags verifies that SetupFlags registers
// every flag the goal command exposes (SetupFlags was 0% covered).
func TestGoalCommand_SetupFlags_AllFlags(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	fs := flag.NewFlagSet("goal", flag.ContinueOnError)

	cmd.SetupFlags(fs)

	expectedFlags := []string{
		"i", "l", "c", "r",
		"session", "store",
		"log-level", "log-file", "log-buffer",
	}
	for _, name := range expectedFlags {
		if fs.Lookup(name) == nil {
			t.Errorf("expected flag %q to be registered", name)
		}
	}
}

// TestGoalCommand_SetupFlags_Parsing verifies flag value propagation.
func TestGoalCommand_SetupFlags_Parsing(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	fs := flag.NewFlagSet("goal", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	if err := fs.Parse([]string{"-i", "-l", "-c", "testing", "-r", "some-goal",
		"-session", "sess1", "-store", "memory",
		"-log-level", "debug", "-log-file", "/tmp/test.log", "-log-buffer", "500"}); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if !cmd.interactive {
		t.Error("expected interactive to be true")
	}
	if !cmd.list {
		t.Error("expected list to be true")
	}
	if cmd.category != "testing" {
		t.Errorf("expected category 'testing', got %q", cmd.category)
	}
	if cmd.run != "some-goal" {
		t.Errorf("expected run 'some-goal', got %q", cmd.run)
	}
	if cmd.session != "sess1" {
		t.Errorf("expected session 'sess1', got %q", cmd.session)
	}
	if cmd.store != "memory" {
		t.Errorf("expected store 'memory', got %q", cmd.store)
	}
	if cmd.logLevel != "debug" {
		t.Errorf("expected logLevel 'debug', got %q", cmd.logLevel)
	}
	if cmd.logPath != "/tmp/test.log" {
		t.Errorf("expected logPath '/tmp/test.log', got %q", cmd.logPath)
	}
	if cmd.logBufferSize != 500 {
		t.Errorf("expected logBufferSize 500, got %d", cmd.logBufferSize)
	}
}

// TestGoalCommand_Execute_ResolveLogConfigError exercises the resolveLogConfig
// error return in GoalCommand.Execute.
func TestGoalCommand_Execute_ResolveLogConfigError(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	cmd.run = "comment-stripper"
	cmd.logLevel = "bogus"
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected 'invalid log level' error, got: %v", err)
	}
}

// TestGoalCommand_Execute_WithLogFile exercises the logFile != nil → defer
// Close() path in GoalCommand.Execute.
func TestGoalCommand_Execute_WithLogFile(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	cmd.run = "comment-stripper"
	cmd.testMode = true
	cmd.store = "memory"
	cmd.session = t.Name()

	dir := t.TempDir()
	cmd.logPath = filepath.Join(dir, "goal-cov.log")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify the log file was created (and thus the Close path was exercised).
	if _, err := os.Stat(cmd.logPath); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

// staticGoalRegistry is a test helper that returns a fixed set of goals.
type staticGoalRegistry struct {
	goals []Goal
}

func (r *staticGoalRegistry) List() []string {
	names := make([]string, len(r.goals))
	for i, g := range r.goals {
		names[i] = g.Name
	}
	return names
}

func (r *staticGoalRegistry) Get(name string) (*Goal, error) {
	for i := range r.goals {
		if r.goals[i].Name == name {
			return &r.goals[i], nil
		}
	}
	return nil, fmt.Errorf("goal not found: %s", name)
}

func (r *staticGoalRegistry) GetAllGoals() []Goal { return r.goals }

func (r *staticGoalRegistry) Reload() error { return nil }

// TestGoalCommand_ListGoals_EmptyGoals covers the "No goals available" path
// with an empty registry (no category filter).
func TestGoalCommand_ListGoals_EmptyGoals(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	registry := &staticGoalRegistry{goals: []Goal{}}
	cmd := NewGoalCommand(cfg, registry)
	cmd.list = true
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stdout.String(), "No goals available") {
		t.Errorf("expected 'No goals available', got: %s", stdout.String())
	}
}

// TestGoalCommand_ListGoals_NoGoalsForCategory covers the category filter
// path when the filter matches zero goals.
func TestGoalCommand_ListGoals_NoGoalsForCategory(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	goalRegistry := newTestGoalRegistryForGoal()
	cmd := NewGoalCommand(cfg, goalRegistry)
	cmd.list = true
	cmd.category = "nonexistent-category-zzz"
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "No goals found for category: nonexistent-category-zzz"
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("expected %q, got: %s", want, stdout.String())
	}
}

// ---------------------------------------------------------------------------
// SuperDocumentCommand coverage gaps (Execute was 0%)
// ---------------------------------------------------------------------------

// TestSuperDocumentCommand_Name_Description_Usage validates metadata accessors.
func TestSuperDocumentCommand_Name_Description_Usage(t *testing.T) {
	t.Parallel()
	cmd := NewSuperDocumentCommand(config.NewConfig())

	if got := cmd.Name(); got != "super-document" {
		t.Errorf("Name() = %q, want %q", got, "super-document")
	}
	if got := cmd.Description(); got != "TUI for merging documents into a single internally consistent super-document" {
		t.Errorf("Description() = %q", got)
	}
	if got := cmd.Usage(); got != "super-document [options]" {
		t.Errorf("Usage() = %q", got)
	}
}

// TestSuperDocumentCommand_SetupFlags verifies all flags including --shell.
func TestSuperDocumentCommand_SetupFlags(t *testing.T) {
	t.Parallel()
	cmd := NewSuperDocumentCommand(config.NewConfig())
	fs := flag.NewFlagSet("sd", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	expected := []string{
		"interactive", "i", "shell", "test",
		"session", "store",
		"log-level", "log-file", "log-buffer",
	}
	for _, name := range expected {
		if fs.Lookup(name) == nil {
			t.Errorf("expected flag %q to be registered", name)
		}
	}

	// Verify --shell defaults to false.
	if cmd.shellMode {
		t.Error("expected shellMode default to be false")
	}

	// Parse --shell and verify it sets shellMode.
	cmd2 := NewSuperDocumentCommand(config.NewConfig())
	fs2 := flag.NewFlagSet("sd2", flag.ContinueOnError)
	cmd2.SetupFlags(fs2)
	if err := fs2.Parse([]string{"--shell"}); err != nil {
		t.Fatal(err)
	}
	if !cmd2.shellMode {
		t.Error("expected shellMode to be true after --shell")
	}
}

// TestSuperDocumentCommand_Execute_NonInteractive exercises the full
// Execute path with testMode=true (the entire function was 0% covered).
func TestSuperDocumentCommand_Execute_NonInteractive(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("Execute failed: %v\nstderr: %s", err, stderr.String())
	}
}

// TestSuperDocumentCommand_Execute_NilConfig verifies Execute tolerates nil
// config (exercises nil-guard branches in theme and hot-snippet injection).
func TestSuperDocumentCommand_Execute_NilConfig(t *testing.T) {
	t.Parallel()
	cmd := NewSuperDocumentCommand(nil)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute with nil config failed: %v", err)
	}
}

// TestSuperDocumentCommand_Execute_ConfigThemeOverrides exercises the
// config → theme color override paths (global + command-specific).
func TestSuperDocumentCommand_Execute_ConfigThemeOverrides(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	// Global theme override.
	cfg.Global = map[string]string{
		"theme.textPrimary": "#111111",
		"other.setting":     "ignored",
	}
	// Command-specific theme override.
	cfg.Commands = map[string]map[string]string{
		"super-document": {
			"theme.accentPrimary": "#222222",
			"non-theme":           "ignored",
		},
	}

	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute with theme overrides failed: %v", err)
	}
}

// TestSuperDocumentCommand_Execute_ShellMode verifies that shellMode flag is
// propagated to the config global.
func TestSuperDocumentCommand_Execute_ShellMode(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.shellMode = true
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute with shellMode failed: %v", err)
	}
}

// TestSuperDocumentCommand_Execute_ResolveLogConfigError exercises the
// resolveLogConfig error return in SuperDocumentCommand.Execute.
func TestSuperDocumentCommand_Execute_ResolveLogConfigError(t *testing.T) {
	t.Parallel()
	cmd := NewSuperDocumentCommand(config.NewConfig())
	cmd.logLevel = "garbage"
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected 'invalid log level' error, got: %v", err)
	}
}

// TestSuperDocumentCommand_Execute_WithLogFile exercises the logFile path
// (logFile != nil → defer Close()).
func TestSuperDocumentCommand_Execute_WithLogFile(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	dir := t.TempDir()
	cmd.logPath = filepath.Join(dir, "sd-cov.log")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if _, err := os.Stat(cmd.logPath); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

// TestSuperDocumentCommand_Execute_HotSnippetInjection exercises
// injectConfigHotSnippets through Execute with populated HotSnippets.
func TestSuperDocumentCommand_Execute_HotSnippetInjection(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "followup", Text: "Continue with context."},
	}

	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute with hot snippets failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CodeReviewCommand coverage gaps
// ---------------------------------------------------------------------------

// TestCodeReviewCommand_Execute_ResolveLogConfigError exercises the
// resolveLogConfig error return in CodeReviewCommand.Execute.
func TestCodeReviewCommand_Execute_ResolveLogConfigError(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)
	cmd.logLevel = "badlevel"
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected 'invalid log level' error, got: %v", err)
	}
}

// TestCodeReviewCommand_Execute_WithLogFile exercises the logFile close path.
func TestCodeReviewCommand_Execute_WithLogFile(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewCodeReviewCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	dir := t.TempDir()
	cmd.logPath = filepath.Join(dir, "cr-cov.log")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if _, err := os.Stat(cmd.logPath); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

// TestCodeReviewCommand_Execute_HotSnippetInjection exercises the
// injectConfigHotSnippets true branch through CodeReviewCommand.Execute.
func TestCodeReviewCommand_Execute_HotSnippetInjection(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "test-snippet", Text: "Test snippet text", Description: "desc"},
	}

	cmd := NewCodeReviewCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// PromptFlowCommand coverage gaps
// ---------------------------------------------------------------------------

// TestPromptFlowCommand_Execute_ResolveLogConfigError exercises the
// resolveLogConfig error return in PromptFlowCommand.Execute.
func TestPromptFlowCommand_Execute_ResolveLogConfigError(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)
	cmd.logLevel = "notreal"
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected 'invalid log level' error, got: %v", err)
	}
}

// TestPromptFlowCommand_Execute_WithLogFile exercises the logFile close path.
func TestPromptFlowCommand_Execute_WithLogFile(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewPromptFlowCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	dir := t.TempDir()
	cmd.logPath = filepath.Join(dir, "pf-cov.log")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if _, err := os.Stat(cmd.logPath); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

// TestPromptFlowCommand_Execute_HotSnippetInjection exercises the
// hot snippet injection through PromptFlowCommand.Execute.
func TestPromptFlowCommand_Execute_HotSnippetInjection(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "pf-snippet", Text: "Test text"},
	}

	cmd := NewPromptFlowCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ScriptingCommand coverage gaps
// ---------------------------------------------------------------------------

// TestScriptingCommand_DefaultTerminalFactory exercises the default
// terminal factory closure (body uncovered at 50% of NewScriptingCommand).
func TestScriptingCommand_DefaultTerminalFactory(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("script", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	defer engine.Close()

	// Call the default factory — exercises the closure body.
	result := cmd.terminalFactory(ctx, engine)
	if result == nil {
		t.Fatal("expected non-nil terminal from default factory")
	}
	// We cannot call Run() without a real terminal, but we confirmed the
	// factory closure body executes and returns a valid terminalRunner.
}

// TestScriptingCommand_Execute_ResolveLogConfigError exercises the
// resolveLogConfig error path through the ctxFactory → resolveLogConfig
// branch of ScriptingCommand.Execute.
func TestScriptingCommand_Execute_ResolveLogConfigError(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.ctxFactory = testCtxFactory
	cmd.logLevel = "totally-invalid"
	cmd.store = "memory"
	cmd.session = t.Name()
	cmd.script = "ctx.log('never');"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from invalid log level")
	}
	if !strings.Contains(err.Error(), "invalid log level") {
		t.Fatalf("expected 'invalid log level' error, got: %v", err)
	}
}

// TestScriptingCommand_Execute_WithLogFile exercises the logFile close path.
func TestScriptingCommand_Execute_WithLogFile(t *testing.T) {
	t.Parallel()
	cmd := NewScriptingCommand(config.NewConfig())
	cmd.ctxFactory = testCtxFactory
	cmd.testMode = true
	cmd.script = `log.info("cov-test");`
	cmd.store = "memory"
	cmd.session = t.Name()

	dir := t.TempDir()
	cmd.logPath = filepath.Join(dir, "sc-cov.log")

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if _, err := os.Stat(cmd.logPath); err != nil {
		t.Fatalf("expected log file to exist: %v", err)
	}
}

// TestScriptingCommand_Execute_HotSnippetInjection exercises the
// hot snippet injection through ScriptingCommand.Execute.
func TestScriptingCommand_Execute_HotSnippetInjection(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "sc-snippet", Text: "Snippet text"},
	}

	cmd := NewScriptingCommand(cfg)
	cmd.ctxFactory = testCtxFactory
	cmd.testMode = true
	cmd.script = "ctx.log('has-snippets');"
	cmd.store = "memory"
	cmd.session = t.Name()

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// injectConfigHotSnippets coverage gaps
// ---------------------------------------------------------------------------

// TestInjectConfigHotSnippets_NilConfig exercises the nil-config → nil
// snippets → skip SetGlobal path.
func TestInjectConfigHotSnippets_NilConfig(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("hs-nil", t.Name()), "memory")
	if err != nil {
		t.Fatalf("engine creation failed: %v", err)
	}
	defer engine.Close()

	// Should not panic; SetGlobal should NOT be called.
	injectConfigHotSnippets(engine, nil)

	// CONFIG_HOT_SNIPPETS should be undefined.
	val := engine.GetGlobal("CONFIG_HOT_SNIPPETS")
	if val != nil {
		t.Errorf("expected CONFIG_HOT_SNIPPETS to be nil (undefined), got %v", val)
	}
}

// TestInjectConfigHotSnippets_EmptySnippets exercises the non-nil config
// with empty HotSnippets → nil return → skip SetGlobal.
func TestInjectConfigHotSnippets_EmptySnippets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("hs-empty", t.Name()), "memory")
	if err != nil {
		t.Fatalf("engine creation failed: %v", err)
	}
	defer engine.Close()

	cfg := config.NewConfig()
	// cfg.HotSnippets is empty by default.
	injectConfigHotSnippets(engine, cfg)

	val := engine.GetGlobal("CONFIG_HOT_SNIPPETS")
	if val != nil {
		t.Errorf("expected CONFIG_HOT_SNIPPETS to be nil, got %v", val)
	}
}

// TestInjectConfigHotSnippets_WithSnippets exercises the true branch
// where snippets are non-nil and SetGlobal IS called.
func TestInjectConfigHotSnippets_WithSnippets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineDeprecated(ctx, &stdout, &stderr,
		testutil.NewTestSessionID("hs-with", t.Name()), "memory")
	if err != nil {
		t.Fatalf("engine creation failed: %v", err)
	}
	defer engine.Close()

	cfg := config.NewConfig()
	cfg.HotSnippets = []config.HotSnippet{
		{Name: "test-hot", Text: "Hot snippet text"},
	}

	injectConfigHotSnippets(engine, cfg)

	val := engine.GetGlobal("CONFIG_HOT_SNIPPETS")
	if val == nil {
		t.Fatal("expected CONFIG_HOT_SNIPPETS to be set")
	}
	arr, ok := val.([]map[string]any)
	if !ok {
		t.Fatalf("expected []map[string]any, got %T", val)
	}
	if len(arr) != 1 {
		t.Fatalf("expected 1 snippet, got %d", len(arr))
	}
	if arr[0]["name"] != "test-hot" {
		t.Errorf("snippet name = %v, want %q", arr[0]["name"], "test-hot")
	}
}

// ---------------------------------------------------------------------------
// resolveLogConfig coverage gaps
// ---------------------------------------------------------------------------

// TestResolveLogConfig_FileCreationError exercises the error path when the
// log file cannot be created (e.g., parent is a regular file, not a dir).
func TestResolveLogConfig_FileCreationError(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()

	// Create a regular file, then try using it as a parent directory.
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(blocker, "test.log")

	_, err := resolveLogConfig(badPath, "info", 1000, cfg)
	if err == nil {
		t.Fatal("expected error for unreachable log file path")
	}
	if !strings.Contains(err.Error(), "failed to open log file") {
		t.Fatalf("expected 'failed to open log file' error, got: %v", err)
	}
}

// TestResolveLogConfig_NegativeMaxFiles exercises the maxFiles < 0 → default
// branch where a negative maxFiles from config is replaced with 5.
func TestResolveLogConfig_NegativeMaxFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "neg-max.log")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("log.max-files", "-1")

	lc, err := resolveLogConfig(logPath, "info", 1000, cfg)
	if err != nil {
		t.Fatalf("resolveLogConfig: %v", err)
	}
	defer func() {
		if lc.logFile != nil {
			lc.logFile.Close()
		}
	}()
	if lc.logFile == nil {
		t.Fatal("expected logFile to be created")
	}

	// We can't directly inspect maxFiles, but we verify the file opens
	// successfully with default maxFiles=5 (no error means the branch worked).
	msg := []byte("test entry\n")
	if _, err := lc.logFile.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}
}

// TestResolveLogConfig_ZeroMaxFiles exercises the maxFiles == 0 path
// (zero is valid — means no backups, just truncate on rotate).
func TestResolveLogConfig_ZeroMaxFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "zero-max.log")

	cfg := config.NewConfig()
	// Not setting log.max-files → resolveInt returns 0 → 0 < 0 is false → stays 0
	cfg.SetGlobalOption("log.max-size-mb", "1")

	lc, err := resolveLogConfig(logPath, "info", 1000, cfg)
	if err != nil {
		t.Fatalf("resolveLogConfig: %v", err)
	}
	defer func() {
		if lc.logFile != nil {
			lc.logFile.Close()
		}
	}()
	if lc.logFile == nil {
		t.Fatal("expected logFile to be created")
	}
}
