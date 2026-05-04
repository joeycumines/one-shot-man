package scripting

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestInitCleanStdout verifies that engine initialization writes zero bytes to
// the stdout writer. Any stdout pollution during init would corrupt MCP stdio
// transport (JSON-RPC over stdin/stdout). This is a regression guard for the
// fix that moved the state-persistence-failure warning to stderr (T10–T12).
func TestInitCleanStdout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("init-clean-stdout", t.Name())

	engine, err := NewEngine(ctx, &stdout, &stderr, sessionID, "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig: %v", err)
	}
	t.Cleanup(func() { _ = engine.Close() })

	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty after init, got %d bytes: %q", stdout.Len(), stdout.String())
	}
}

// TestInitCleanStdout_FallbackBackend verifies that when the primary storage
// backend fails, the fallback-warning goes to stderr (not stdout). This tests
// the fix where tui_manager.go:114 was changed from fmt.Fprintf(output, ...)
// to fmt.Fprintf(os.Stderr, ...).
func TestInitCleanStdout_FallbackBackend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	var stdout bytes.Buffer
	engine := mustNewEngine(t, ctx, &stdout, &bytes.Buffer{})

	// Create a TUI manager with a bogus backend name to trigger the fallback path.
	// The "nonexistent" backend will fail in initializeStateManager, triggering
	// the warning path at tui_manager.go:114 which should go to os.Stderr.
	sessionID := testutil.NewTestSessionID("fallback-backend", t.Name())
	_ = NewTUIManagerWithConfig(ctx, engine, nil, &stdout, sessionID, "nonexistent")

	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty even when backend falls back, got %d bytes: %q",
			stdout.Len(), stdout.String())
	}
}
