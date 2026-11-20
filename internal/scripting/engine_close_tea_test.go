package scripting

import (
	"bytes"
	"context"
	"log/slog"
	"runtime"
	"sync"
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

	parentCtx := context.Background()
	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("test", t.Name())
	engine, err := NewEngine(parentCtx, &stdout, &stderr, sessionID, "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	if engine.Ctx() == nil {
		t.Fatal("engine.Ctx() is nil; EventLoopProvider.Ctx() must be implemented")
	}

	teaReady := make(chan struct{})
	var readyOnce sync.Once
	engine.SetGlobal("__teaReady", func() {
		readyOnce.Do(func() { close(teaReady) })
	})

	script := engine.LoadScriptString("tea-idle.js", `
		var tea = require("osm:bubbletea");
		function init() {
			__teaReady();
			return [{ n: 0 }, null];
		}
		function update(msg, m) { return [m, null]; }
		function view(m) { return { content: "idle " + m.n }; }
		// Idle program: no quit command, waits for ctx cancellation.
		tea.run(tea.newModel({ init: init, update: update, view: view }));
	`)
	execDone := make(chan error, 1)
	go func() {
		execDone <- engine.ExecuteScript(script)
	}()

	// Wait deterministically for tea.run to enter Init() before calling Close.
	select {
	case err := <-execDone:
		t.Fatalf("script finished before Close (err=%v); the program was not idle — stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	case <-teaReady:
	case <-time.After(5 * time.Second):
		t.Fatalf("tea.run did not reach init() within 5s (stdout=%q stderr=%q)", stdout.String(), stderr.String())
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- engine.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("engine.Close() = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("engine.Close() did not return within 10s with an idle tea.run program running and an uncancelled parent ctx (stdout=%q stderr=%q)", stdout.String(), stderr.String())
	}

	if engine.Ctx().Err() == nil {
		t.Fatal("engine.Ctx().Err() = nil after Close(), want cancelled")
	}
	if parentCtx.Err() != nil {
		t.Fatalf("parentCtx.Err() = %v, want nil (Close must cancel only engine ctx)", parentCtx.Err())
	}

	// ExecuteScript must also return now that the loop is shut down.
	select {
	case err := <-execDone:
		t.Logf("ExecuteScript returned after Close: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteScript did not return after engine.Close()")
	}
}

// TestEngine_CloseWithPendingChildWaitReturnsBounded verifies that closing an
// engine while a script has an outstanding child.wait() tears down cleanly and
// promptly without hanging or triggering an unexpected fatal error.
func TestEngine_CloseWithPendingChildWaitReturnsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a child process and live event loop")
	}

	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	var stdout, stderr bytes.Buffer
	sessionID := testutil.NewTestSessionID("test", t.Name())
	engine, err := NewEngine(parentCtx, &stdout, &stderr, sessionID, "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	waitStarted := make(chan struct{})
	var startOnce sync.Once
	engine.SetGlobal("__waitStarted", func() {
		startOnce.Do(func() { close(waitStarted) })
	})

	cmdJS := `exec.spawn("sh", ["-c", "sleep 30"])`
	if runtime.GOOS == "windows" {
		cmdJS = `exec.spawn("cmd.exe", ["/d", "/c", "ping -n 30 127.0.0.1 >nul"])`
	}

	script := engine.LoadScriptString("pending-wait.js", `
		var exec = require("osm:exec");
		(async function() {
			var child = await `+cmdJS+`;
			var p = child.wait();
			__waitStarted();
			try {
				await p;
			} catch (e) {
				// Expected on engine shutdown/cancellation.
			}
		})();
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript: %v (stderr=%q)", err, stderr.String())
	}

	select {
	case <-waitStarted:
	case <-time.After(5 * time.Second):
		t.Fatalf("child.wait() did not start within 5s (stderr=%q)", stderr.String())
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- engine.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("engine.Close() = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("engine.Close() did not return within 10s with pending child.wait() (stderr=%q)", stderr.String())
	}
}
