package scripting

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// sentinelDrainTestEngine creates a test engine wired up for sentinel drain tests.
// It uses the same pattern as newTestEngine in engine_test.go.
func sentinelDrainTestEngine(t *testing.T, stdout, stderr *bytes.Buffer) *Engine {
	t.Helper()
	ctx := context.Background()
	engine, err := NewEngine(ctx, stdout, stderr, testutil.NewTestSessionID("", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	return engine
}

// TestSentinelDrain_SetTimeoutChains verifies that waitForAsyncWork blocks until
// a chain of three setTimeout callbacks completes. Each timer schedules the next,
// producing a serial chain that the sentinel drain must fully drain before returning.
//
// Without the sentinel drain, ExecuteScript would return after the synchronous
// portion finishes, and the setTimeout callbacks would never fire (or would fire
// on a shutting-down loop).
func TestSentinelDrain_SetTimeoutChains(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sentinel drain test in short mode")
	}
	t.Parallel()

	var stdout bytes.Buffer
	engine := sentinelDrainTestEngine(t, &stdout, &stdout)

	script := engine.LoadScriptFromString("settimeout-chains", `
		setTimeout(function() {
			output.print("tick-1");
			setTimeout(function() {
				output.print("tick-2");
				setTimeout(function() {
					output.print("tick-3");
				}, 0);
			}, 0);
		}, 0);
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	out := stdout.String()
	for _, expected := range []string{"tick-1", "tick-2", "tick-3"} {
		if !strings.Contains(out, expected) {
			t.Errorf("expected output to contain %q, got:\n%s", expected, out)
		}
	}

	// Verify ordering: tick-1 must appear before tick-2, tick-2 before tick-3.
	idx1 := strings.Index(out, "tick-1")
	idx2 := strings.Index(out, "tick-2")
	idx3 := strings.Index(out, "tick-3")
	if idx1 >= idx2 {
		t.Errorf("tick-1 (index %d) should appear before tick-2 (index %d)", idx1, idx2)
	}
	if idx2 >= idx3 {
		t.Errorf("tick-2 (index %d) should appear before tick-3 (index %d)", idx2, idx3)
	}
}

// TestSentinelDrain_PromiseAsync verifies that waitForAsyncWork drains async work
// initiated via new Promise + setTimeout (the Promisify-like pattern used by
// native modules). The script creates a Promise that resolves after a short delay,
// and the resolve handler writes output. The sentinel drain must wait for the
// full promise chain to settle.
func TestSentinelDrain_PromiseAsync(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sentinel drain test in short mode")
	}
	t.Parallel()

	var stdout bytes.Buffer
	engine := sentinelDrainTestEngine(t, &stdout, &stdout)

	script := engine.LoadScriptFromString("promise-async", `
		new Promise(function(resolve) {
			setTimeout(function() {
				resolve("async-value");
			}, 0);
		}).then(function(val) {
			output.print("resolved: " + val);
			return new Promise(function(resolve) {
				setTimeout(function() {
					resolve("chained");
				}, 0);
			});
		}).then(function(val) {
			output.print("chained: " + val);
		});
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "resolved: async-value") {
		t.Errorf("expected resolved promise output, got:\n%s", out)
	}
	if !strings.Contains(out, "chained: chained") {
		t.Errorf("expected chained promise output, got:\n%s", out)
	}
}

// TestSentinelDrain_NoAsyncWork verifies that waitForAsyncWork returns immediately
// when the script performs no async work. This exercises the early-exit path:
// if Alive() is false on the first check, the sentinel loop body never executes.
func TestSentinelDrain_NoAsyncWork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sentinel drain test in short mode")
	}
	t.Parallel()

	var stdout bytes.Buffer
	engine := sentinelDrainTestEngine(t, &stdout, &stdout)

	script := engine.LoadScriptFromString("no-async", `
		output.print("sync-only");
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "sync-only") {
		t.Errorf("expected sync output, got:\n%s", out)
	}
}

// TestSentinelDrain_RapidFireTimers stress-tests the sentinel drain with many
// concurrent setTimeout callbacks. All timers are registered simultaneously
// (no chaining), so they should all fire within the same event loop tick or
// across a few ticks. The sentinel drain must not return prematurely.
func TestSentinelDrain_RapidFireTimers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping sentinel drain test in short mode")
	}
	t.Parallel()

	var stdout bytes.Buffer
	engine := sentinelDrainTestEngine(t, &stdout, &stdout)

	const count = 50

	engine.SetGlobal("timerCount", int64(count))

	script := engine.LoadScriptFromString("rapid-fire", `
		for (var i = 0; i < timerCount; i++) {
			(function(idx) {
				setTimeout(function() {
					output.print("timer-" + idx);
				}, 0);
			})(i);
		}
	`)

	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript failed: %v", err)
	}

	out := stdout.String()

	// Verify all timers fired by checking the count of "timer-" occurrences.
	occurrences := strings.Count(out, "timer-")
	if occurrences != count {
		t.Errorf("expected %d timer outputs, got %d\noutput:\n%s", count, occurrences, out)
	}
}
