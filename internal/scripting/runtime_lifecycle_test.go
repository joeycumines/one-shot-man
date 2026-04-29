package scripting

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// TestRuntime_PersistentLoop_SurvivesMultipleExecuteScript verifies that the
// persistent Runtime loop stays alive across multiple sequential ExecuteScript
// calls. This is a regression test for the startup race introduced by
// WithAutoExit(true): with auto-exit, the loop could terminate after the first
// quiescence, causing subsequent submissions to fail with ErrLoopTerminated.
func TestRuntime_PersistentLoop_SurvivesMultipleExecuteScript(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	ctx := context.Background()
	engine := newTestEngine(t, ctx, &stdout, &stdout)

	for i := 0; i < 3; i++ {
		script := engine.LoadScriptFromString("persistent-test",
			`output.print("exec-ok");`)
		if err := engine.ExecuteScript(script); err != nil {
			t.Fatalf("ExecuteScript call %d failed: %v", i+1, err)
		}
	}

	// All three scripts must have produced output.
	occurrences := strings.Count(stdout.String(), "exec-ok")
	if occurrences != 3 {
		t.Errorf("expected 3 'exec-ok' outputs, got %d\nfull output:\n%s", occurrences, stdout.String())
	}
}

// TestRuntime_Close_TerminatesLoop verifies that Close() makes the loop
// unavailable for further work. After Close(), RunOnLoopSync must return an
// error rather than blocking forever.
func TestRuntime_Close_TerminatesLoop(t *testing.T) {
	t.Parallel()

	rt, err := NewRuntime(context.Background())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}

	// Verify it works before Close.
	if runErr := rt.RunOnLoopSync(func(_ *goja.Runtime) error {
		return nil
	}); runErr != nil {
		t.Fatalf("RunOnLoopSync before Close failed: %v", runErr)
	}

	if closeErr := rt.Close(); closeErr != nil {
		t.Fatalf("Close failed: %v", closeErr)
	}

	// After Close the runtime must report not running.
	if rt.IsRunning() {
		t.Error("IsRunning() returned true after Close()")
	}

	// RunOnLoopSync must fail, not block.
	done := make(chan error, 1)
	go func() {
		done <- rt.RunOnLoopSync(func(_ *goja.Runtime) error {
			return nil
		})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("RunOnLoopSync after Close() unexpectedly succeeded")
		}
	case <-time.After(2 * time.Second):
		t.Error("RunOnLoopSync blocked after Close() — loop did not terminate")
	}
}

// TestRuntime_NoAutoExit_IdleLoop verifies that an idle Runtime loop stays
// alive without any timers or work queued. Under the old WithAutoExit(true)
// design, an idle loop could exit prematurely and reject subsequent submissions.
func TestRuntime_NoAutoExit_IdleLoop(t *testing.T) {
	t.Parallel()

	rt, err := NewRuntime(context.Background())
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	t.Cleanup(func() { _ = rt.Close() })

	// Wait briefly to give the loop an opportunity to auto-exit (if it were
	// mistakenly configured with WithAutoExit(true) and no keepalive work).
	time.Sleep(50 * time.Millisecond)

	// The loop must still be alive and accept work.
	if err := rt.RunOnLoopSync(func(_ *goja.Runtime) error {
		return nil
	}); err != nil {
		t.Errorf("RunOnLoopSync on idle loop failed: %v (loop may have auto-exited)", err)
	}
}
