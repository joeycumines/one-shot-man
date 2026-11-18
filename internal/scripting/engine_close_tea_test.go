package scripting

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// TestEngine_CloseWithIdleTeaProgramReturnsBounded pins V7: the bubbletea
// manager must be constructed with the ENGINE's lifecycle context
// (EventLoopProvider.Ctx()), not the per-call registration context. With an
// uncancelled parent ctx, an idle tea.run program is kept alive by the
// manager's promisify token; engine.Close() cancels the engine ctx (which the
// manager now derives from), the program's tea.WithContext(ctx) fires, and
// Close returns promptly instead of hanging until the parent ctx is cancelled
// by test cleanup.
func TestEngine_CloseWithIdleTeaProgramReturnsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a live event loop and bubbletea program")
	}

	// Parent ctx is intentionally NEVER cancelled by this test: if the
	// manager were built on the Register() call-site ctx (== parent here) it
	// would behave the same, but the regression is that the manager must use
	// the ENGINE lifecycle ctx so Close() — which cancels that ctx via
	// runtime.Close() — unblocks the program.
	parentCtx := context.Background()
	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("test", t.Name())
	engine, err := NewEngine(parentCtx, &stdout, &stderr, sessionID, "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	// Sanity: the manager's context must be the engine's lifecycle ctx, i.e.
	// cancelling the engine (Close) cancels it — not the (here identical)
	// parent. We assert identity through behavior below; this check just
	// guarantees the provider wired Ctx() through.
	if engine.Ctx() == nil {
		t.Fatal("engine.Ctx() is nil; EventLoopProvider.Ctx() must be implemented")
	}

	script := engine.LoadScriptString("tea-idle.js", `
		var tea = require("osm:bubbletea");
		function init() { return [{ n: 0 }, null]; }
		function update(msg, m) { return [m, null]; }
		function view(m) { return { content: "idle " + m.n }; }
		// Idle program: no quit command, waits for ctx cancellation.
		tea.run(tea.newModel({ init: init, update: update, view: view }));
	`)
	execDone := make(chan error, 1)
	go func() {
		execDone <- engine.ExecuteScript(script)
	}()

	// Give the script a moment to reach tea.run and park in WaitForProgram.
	select {
	case err := <-execDone:
		t.Fatalf("script finished before Close (err=%v); the program was not idle — stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	case <-time.After(500 * time.Millisecond):
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- engine.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("engine.Close() = %v", err)
		}
	case <-time.After(10 * time.Second):
		// The engine's lifecycle context must be independent of the parent:
		// cancelling it via Close (runtime.Close -> rt.cancel) must unblock
		// both the tea program and the parked WaitForProgram, without the
		// parent ctx ever being cancelled. Prove the parent is still live,
		// then cancel the engine ctx explicitly as the fallback the old code
		// effectively required (parent cancellation) to pass this test.
		if parentCtx.Err() != nil {
			t.Fatal("parent ctx was cancelled; test setup is wrong")
		}
		t.Fatalf("engine.Close() did not return within 10s with an idle tea.run program running and an uncancelled parent ctx (stdout=%q stderr=%q)", stdout.String(), stderr.String())
	}

	// ExecuteScript must also return now that the loop is shut down.
	select {
	case err := <-execDone:
		// Any error here is acceptable — the loop is closed; what matters is
		// that it RETURNED. Log it for diagnostics.
		t.Logf("ExecuteScript returned after Close: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteScript did not return after engine.Close()")
	}

	// Keep the unused-import guard honest if io becomes unused.
	var _ io.Writer = &stdout
}
