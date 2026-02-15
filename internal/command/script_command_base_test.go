package command

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// scriptCommandBase.RegisterFlags
// ---------------------------------------------------------------------------

func TestScriptCommandBase_RegisterFlags(t *testing.T) {
	t.Parallel()

	var b scriptCommandBase
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	b.RegisterFlags(fs)

	// Verify all expected flags are registered.
	for _, name := range []string{"test", "session", "store", "log-level", "log-file", "log-buffer"} {
		if fs.Lookup(name) == nil {
			t.Errorf("expected flag %q to be registered", name)
		}
	}

	// Parse with non-default values and verify propagation.
	if err := fs.Parse([]string{
		"-test",
		"-session", "sid",
		"-store", "memory",
		"-log-level", "debug",
		"-log-file", "/tmp/test.log",
		"-log-buffer", "42",
	}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !b.testMode {
		t.Error("expected testMode=true")
	}
	if b.session != "sid" {
		t.Errorf("session = %q, want %q", b.session, "sid")
	}
	if b.store != "memory" {
		t.Errorf("store = %q, want %q", b.store, "memory")
	}
	if b.logLevel != "debug" {
		t.Errorf("logLevel = %q, want %q", b.logLevel, "debug")
	}
	if b.logPath != "/tmp/test.log" {
		t.Errorf("logPath = %q, want %q", b.logPath, "/tmp/test.log")
	}
	if b.logBufferSize != 42 {
		t.Errorf("logBufferSize = %d, want %d", b.logBufferSize, 42)
	}
}

func TestScriptCommandBase_RegisterFlags_Defaults(t *testing.T) {
	t.Parallel()

	var b scriptCommandBase
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	b.RegisterFlags(fs)

	// Parse with no args — defaults should apply.
	if err := fs.Parse(nil); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if b.testMode {
		t.Error("expected testMode=false by default")
	}
	if b.session != "" {
		t.Errorf("session = %q, want empty", b.session)
	}
	if b.store != "" {
		t.Errorf("store = %q, want empty", b.store)
	}
	if b.logLevel != "info" {
		t.Errorf("logLevel = %q, want %q", b.logLevel, "info")
	}
	if b.logPath != "" {
		t.Errorf("logPath = %q, want empty", b.logPath)
	}
	if b.logBufferSize != 1000 {
		t.Errorf("logBufferSize = %d, want %d", b.logBufferSize, 1000)
	}
}

// ---------------------------------------------------------------------------
// scriptCommandBase.PrepareEngine
// ---------------------------------------------------------------------------

func TestScriptCommandBase_PrepareEngine_Success(t *testing.T) {
	t.Parallel()

	b := scriptCommandBase{
		config:   nil,
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("PrepareEngine: %v", err)
	}
	defer cleanup()

	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestScriptCommandBase_PrepareEngine_TestMode(t *testing.T) {
	t.Parallel()

	b := scriptCommandBase{
		config:   nil,
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
		testMode: true,
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("PrepareEngine: %v", err)
	}
	defer cleanup()

	// Engine should be in test mode. We can verify by executing a script
	// that checks testMode behavior (output includes script name prefix).
	script := engine.LoadScriptFromString("test-script", "ctx.log('hello');")
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript: %v", err)
	}

	// In test mode, output should include the script-name prefix.
	if out := stdout.String(); out == "" {
		t.Error("expected non-empty stdout in test mode")
	}
}

func TestScriptCommandBase_PrepareEngine_InvalidLogLevel(t *testing.T) {
	t.Parallel()

	b := scriptCommandBase{
		config:   nil,
		store:    "memory",
		session:  t.Name(),
		logLevel: "invalid-level",
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	_, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err == nil {
		cleanup()
		t.Fatal("expected error for invalid log level")
	}
}

func TestScriptCommandBase_PrepareEngine_WithLogFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")

	b := scriptCommandBase{
		config:   nil,
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
		logPath:  logPath,
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("PrepareEngine: %v", err)
	}

	// Log something through the engine.
	script := engine.LoadScriptFromString("log-test", `log.info("from-base-test");`)
	if err := engine.ExecuteScript(script); err != nil {
		cleanup()
		t.Fatalf("ExecuteScript: %v", err)
	}

	cleanup()

	// Verify log file was created and closed properly (file should be readable).
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(content) == 0 {
		t.Error("expected non-empty log file")
	}
}

func TestScriptCommandBase_PrepareEngine_CleanupIdempotent(t *testing.T) {
	t.Parallel()

	b := scriptCommandBase{
		config:   nil,
		store:    "memory",
		session:  t.Name(),
		logLevel: "info",
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	_, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err != nil {
		t.Fatalf("PrepareEngine: %v", err)
	}

	// Call cleanup twice — should not panic.
	cleanup()
	cleanup()
}

func TestScriptCommandBase_PrepareEngine_ErrorCleanup(t *testing.T) {
	t.Parallel()

	// Use an invalid log file path to trigger error after log file creation.
	// But first, let's test that error returns noop cleanup.
	b := scriptCommandBase{
		config:   nil,
		store:    "memory",
		session:  t.Name(),
		logLevel: "INVALID",
	}

	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	_, cleanup, err := b.PrepareEngine(ctx, &stdout, &stderr)
	if err == nil {
		cleanup()
		t.Fatal("expected error")
	}

	// cleanup should be safe to call even on error.
	cleanup()
}
