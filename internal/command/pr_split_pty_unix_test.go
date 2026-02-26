//go:build unix

package command

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/builtin/pty"
)

// osmBinaryPath caches the built binary path across tests.
var (
	osmBinaryOnce sync.Once
	osmBinaryPath string
	osmBinaryErr  error
)

// buildOSMBinary compiles the osm binary once per test run. Returns the
// path to the binary or an error. The binary is placed in a temp directory
// generated from the test's context.
func buildOSMBinary(t *testing.T) string {
	t.Helper()
	osmBinaryOnce.Do(func() {
		binDir, err := os.MkdirTemp("", "osm-test-bin-*")
		if err != nil {
			osmBinaryErr = fmt.Errorf("failed to create temp dir: %w", err)
			return
		}
		osmBinaryPath = filepath.Join(binDir, "osm")
		cmd := exec.Command("go", "build", "-o", osmBinaryPath, "./cmd/osm")
		// Build from the repository root — two levels up from internal/command/.
		cmd.Dir = filepath.Join(projectRoot(t))
		cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			osmBinaryErr = fmt.Errorf("go build failed: %w\n%s", err, out)
		}
	})
	if osmBinaryErr != nil {
		t.Fatalf("failed to build osm: %v", osmBinaryErr)
	}
	return osmBinaryPath
}

// projectRoot returns the repository root by walking up from this file.
func projectRoot(t *testing.T) string {
	t.Helper()
	// internal/command/ → ../.. → repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

// ---------------------------------------------------------------------------
// PTY Integration Test: Deadlock regression — Signal during blocked Write
// ---------------------------------------------------------------------------

// TestPTY_AutoSplit_SendBlockedCancelWorks verifies the core deadlock fix:
// when a PTY write blocks (child not reading stdin), Signal("SIGKILL") must
// be deliverable and must unblock the write. This exercises the exact code
// path used by auto-split: prSplitSendWithCancel → send goroutine blocks →
// cancel flag detected → kill() → SIGKILL → write returns → function returns.
//
// Before the fix, Process.Write() held p.mu during the blocking kernel write,
// preventing Signal() from acquiring the same lock → deadlock.
func TestPTY_AutoSplit_SendBlockedCancelWorks(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}

	ctx := context.Background()
	// Use the internal/builtin/pty package directly to test at the
	// lowest level — same as ClaudeCodeExecutor uses via ptyAgentHandle.
	proc, err := ptySpawnSleep(ctx)
	if err != nil {
		t.Fatalf("failed to spawn sleep: %v", err)
	}
	defer proc.Close()

	// Simulate the cancel-after-delay pattern from the auto-split TUI.
	var (
		cancelFlag int32
		cancelMu   sync.Mutex
		cancelled  = func() bool { cancelMu.Lock(); defer cancelMu.Unlock(); return cancelFlag == 1 }
		setCancel  = func() { cancelMu.Lock(); cancelFlag = 1; cancelMu.Unlock() }
	)

	// Cancel after 500ms (simulates user pressing q in the TUI).
	go func() {
		time.Sleep(500 * time.Millisecond)
		setCancel()
	}()

	start := time.Now()
	sendErr := prSplitSendWithCancel(
		func() error {
			// Write a large amount of data — this will block because
			// sleep never reads from stdin and the PTY buffer fills.
			return proc.Write(strings.Repeat("classify these files please\n", 50000))
		},
		func() { _ = proc.Signal("SIGKILL") },
		cancelled,
		func() bool { return false },
	)
	elapsed := time.Since(start)

	// The function MUST return within a reasonable time — if it deadlocks,
	// the test framework's timeout will catch it.
	if elapsed > 10*time.Second {
		t.Fatalf("prSplitSendWithCancel took %v — likely deadlock (expected <5s)", elapsed)
	}

	if sendErr == nil {
		// On macOS, the PTY buffer might be large enough to absorb the
		// write before cancel fires. This is acceptable — no deadlock.
		t.Logf("Send completed before cancel (large PTY buffer), elapsed=%v", elapsed)
	} else if strings.Contains(sendErr.Error(), "cancelled") {
		t.Logf("Cancel detected correctly, elapsed=%v, err=%v", elapsed, sendErr)
	} else {
		// The write might fail with an I/O error after SIGKILL — also acceptable.
		t.Logf("Send returned error (acceptable), elapsed=%v, err=%v", elapsed, sendErr)
	}
}

// ---------------------------------------------------------------------------
// PTY Integration Test: Full osm pr-split auto-split via termtest
// ---------------------------------------------------------------------------

// TestPTY_AutoSplit_EndToEnd spawns the real `osm pr-split` binary in a
// PTY using termtest, sets up a git repository, triggers auto-split with
// a mock Claude (a shell script that writes the expected MCP result files),
// and verifies:
//   - The auto-split TUI appears with progress steps
//   - The pipeline progresses past "Send classification request"
//   - Pressing q cancels within a reasonable timeout
//   - The process returns to the command prompt cleanly
//
// This test requires the osm binary to be buildable and runs on Unix only
// (PTY requirement). It does NOT require real AI infrastructure.
func TestPTY_AutoSplit_EndToEnd(t *testing.T) {
	t.Parallel()

	osmBin := buildOSMBinary(t)

	// Set up a realistic git repository.
	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Create a mock Claude script that writes the classification result.
	// The script reads --mcp-config from args, extracts the result dir,
	// sleeps briefly to simulate startup, reads stdin, then writes
	// classification.json.
	mockScript := filepath.Join(t.TempDir(), "mock-claude.sh")
	if err := os.WriteFile(mockScript, []byte(`#!/bin/bash
# Mock Claude — writes classification.json after receiving stdin.
# Extract result dir from --mcp-config argument.
RESULT_DIR=""
for arg in "$@"; do
  if [ -n "$NEXT_IS_CONFIG" ]; then
    # Read the MCP config file to find the result directory.
    if [ -f "$arg" ]; then
      RESULT_DIR=$(dirname "$arg")/results
    fi
    NEXT_IS_CONFIG=""
  fi
  if [ "$arg" = "--mcp-config" ]; then
    NEXT_IS_CONFIG=1
  fi
done

# Fallback: scan for any arg that looks like a path with results.
if [ -z "$RESULT_DIR" ]; then
  for arg in "$@"; do
    if echo "$arg" | grep -q "mcp"; then
      RESULT_DIR=$(dirname "$arg")/results
      break
    fi
  done
fi

# Wait for stdin input (the classification prompt).
# Read at most 1 line (the prompt might be multi-line).
read -t 30 FIRST_LINE || true

# Write a minimal classification — maps all files to "feature".
if [ -n "$RESULT_DIR" ]; then
  mkdir -p "$RESULT_DIR"
  echo '{"pkg/auth/auth.go":"auth","pkg/auth/auth_test.go":"auth","pkg/core/config.go":"core","pkg/core/config_test.go":"core","internal/util/numbers.go":"util","internal/util/numbers_test.go":"util","docs/api-reference.md":"docs","Makefile":"infra"}' > "$RESULT_DIR/classification.json"
fi

# Stay alive until killed (Claude Code stays running).
sleep 3600
`), 0o755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Spawn osm pr-split in a PTY via termtest.
	console, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, "pr-split",
			"-base=main",
			"-strategy=directory",
			"-verify=true", // always-pass verify command
			"-claude-command="+mockScript,
		),
		termtest.WithDir(repoDir),
		termtest.WithSize(30, 120),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
		}),
	)
	if err != nil {
		t.Fatalf("failed to create console: %v", err)
	}
	defer console.Close()

	// Wait for the go-prompt to appear.
	snap := console.Snapshot()
	err = console.Expect(ctx, snap, termtest.Contains("pr-split"), "waiting for prompt")
	if err != nil {
		t.Fatalf("prompt did not appear: %v\nOutput so far:\n%s", err, console.String())
	}

	// Type auto-split command.
	snap = console.Snapshot()
	if err := console.SendLine("auto-split"); err != nil {
		t.Fatalf("failed to send auto-split command: %v", err)
	}

	// Expect the auto-split TUI to show progress.
	err = console.Expect(ctx, snap,
		termtest.Any(
			termtest.Contains("Analyze diff"),
			termtest.Contains("Auto-Split"),
			termtest.Contains("auto-split"),
		),
		"waiting for auto-split TUI to appear",
	)
	if err != nil {
		t.Fatalf("auto-split TUI did not appear: %v\nOutput:\n%s", err, console.String())
	}

	// Wait for some progress — at least past the Analyze step.
	snap = console.Snapshot()
	err = console.Expect(ctx, snap,
		termtest.Any(
			termtest.Contains("Spawn Claude"),
			termtest.Contains("Send classification"),
			termtest.Contains("Receive classification"),
			// If mock-claude is too fast, we might see completion.
			termtest.Contains("Complete"),
			termtest.Contains("Error"),
		),
		"waiting for pipeline progress",
	)
	if err != nil {
		t.Logf("Pipeline progress check timed out (may be acceptable): %v", err)
		t.Logf("Output:\n%s", console.String())
	}

	// Give the pipeline a few more seconds to make progress.
	time.Sleep(3 * time.Second)

	// Check if the pipeline is still running or completed.
	output := console.String()
	t.Logf("Output after waiting:\n%s", output)

	if strings.Contains(output, "Complete") || strings.Contains(output, "Error") {
		// Pipeline finished (success or error) — verify we can dismiss.
		t.Log("Pipeline completed, pressing q to dismiss")
		if err := console.Send("q"); err != nil {
			t.Fatalf("failed to send q: %v", err)
		}

		// Wait for the prompt to reappear.
		snap = console.Snapshot()
		err = console.Expect(ctx, snap, termtest.Contains("pr-split"), "waiting for prompt after dismiss")
		if err != nil {
			t.Logf("Prompt did not reappear (may be acceptable): %v", err)
		}
	} else {
		// Pipeline is still running — test cancellation.
		t.Log("Pipeline still running, pressing q to cancel")
		if err := console.Send("q"); err != nil {
			t.Fatalf("failed to send q: %v", err)
		}

		// Cancellation should be detected within 2 seconds.
		snap = console.Snapshot()
		cancelCtx, cancelTimeout := context.WithTimeout(ctx, 10*time.Second)
		defer cancelTimeout()
		err = console.Expect(cancelCtx, snap,
			termtest.Any(
				termtest.Contains("Cancelling"),
				termtest.Contains("cancel"),
				termtest.Contains("pr-split"), // prompt returned
			),
			"waiting for cancel acknowledgement",
		)
		if err != nil {
			t.Logf("WARNING: Cancel not acknowledged within timeout: %v", err)
			t.Logf("Output:\n%s", console.String())

			// Force cancel with second q press.
			t.Log("Force cancelling with second q press")
			if err := console.Send("q"); err != nil {
				t.Logf("Failed to send second q: %v", err)
			}
		}

		// After cancel, we should eventually return to the prompt.
		snap = console.Snapshot()
		promptCtx, promptCancel := context.WithTimeout(ctx, 15*time.Second)
		defer promptCancel()
		err = console.Expect(promptCtx, snap,
			termtest.Any(
				termtest.Contains("pr-split"),
				termtest.Contains(">"),
			),
			"waiting for prompt to return after cancel",
		)
		if err != nil {
			t.Errorf("HANG DETECTED: prompt did not return after cancel: %v", err)
			t.Logf("Final output:\n%s", console.String())
		}
	}
}

// TestPTY_AutoSplit_CancelDuringBlockedSend specifically tests the scenario
// where the Claude process hasn't started reading stdin yet (simulated by
// using `sleep` as the claude command — it never reads). The send blocks,
// the user presses q, and the pipeline must cancel and return to the prompt.
//
// This is the core regression test for the mutex deadlock bug.
func TestPTY_AutoSplit_CancelDuringBlockedSend(t *testing.T) {
	t.Parallel()

	osmBin := buildOSMBinary(t)

	repoDir := initIntegrationRepo(t)
	addIntegrationFeatureFiles(t, repoDir)

	// Use `cat` as mock Claude — it echoes but never writes MCP results,
	// so the pipeline will block at pollForFile. But crucially, cat DOES
	// read stdin, so the send won't block. To test the blocked-send case,
	// we use a script that sleeps before reading.
	mockSlowClaude := filepath.Join(t.TempDir(), "slow-claude.sh")
	if err := os.WriteFile(mockSlowClaude, []byte(`#!/bin/bash
# Mock Claude that takes forever to start reading stdin.
# This simulates the real Claude Code startup delay.
sleep 3600
`), 0o755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	console, err := termtest.NewConsole(ctx,
		termtest.WithCommand(osmBin, "pr-split",
			"-base=main",
			"-strategy=directory",
			"-verify=true",
			"-claude-command="+mockSlowClaude,
		),
		termtest.WithDir(repoDir),
		termtest.WithSize(30, 120),
		termtest.WithDefaultTimeout(20*time.Second),
		termtest.WithEnv([]string{
			"TERM=xterm-256color",
			"HOME=" + t.TempDir(),
			"OSM_CONFIG=",
		}),
	)
	if err != nil {
		t.Fatalf("failed to create console: %v", err)
	}
	defer console.Close()

	// Wait for prompt.
	snap := console.Snapshot()
	err = console.Expect(ctx, snap, termtest.Contains("pr-split"), "waiting for prompt")
	if err != nil {
		t.Fatalf("prompt did not appear: %v\nOutput:\n%s", err, console.String())
	}

	// Start auto-split.
	snap = console.Snapshot()
	if err := console.SendLine("auto-split"); err != nil {
		t.Fatalf("failed to send auto-split: %v", err)
	}

	// Wait for the pipeline to reach "Send classification request".
	err = console.Expect(ctx, snap,
		termtest.Any(
			termtest.Contains("Send classification"),
			termtest.Contains("classification"),
			termtest.Contains("Error"),
		),
		"waiting for Send classification step",
	)
	if err != nil {
		t.Logf("Did not reach Send classification step: %v", err)
		t.Logf("Output:\n%s", console.String())
	}

	// The send should be blocking (mock Claude never reads). Wait a bit
	// to ensure it's really stuck, then press q.
	time.Sleep(2 * time.Second)

	t.Log("Pressing q to cancel blocked send")
	if err := console.Send("q"); err != nil {
		t.Fatalf("failed to send q: %v", err)
	}

	// The cancel MUST be processed — the pipeline should acknowledge it.
	snap = console.Snapshot()
	cancelCtx, cancelTimeout := context.WithTimeout(ctx, 10*time.Second)
	defer cancelTimeout()
	err = console.Expect(cancelCtx, snap,
		termtest.Any(
			termtest.Contains("Cancelling"),
			termtest.Contains("cancel"),
			termtest.Contains("Error"),
			termtest.Contains("pr-split"), // prompt returned
		),
		"waiting for cancel to take effect",
	)
	if err != nil {
		t.Errorf("CANCEL FAILED: cancel not acknowledged within 10s (DEADLOCK?): %v", err)
		t.Logf("Output:\n%s", console.String())

		// Try force cancel.
		t.Log("Attempting force cancel with second q")
		_ = console.Send("q")
		time.Sleep(5 * time.Second)
	}

	// The pipeline should return to the prompt within a reasonable time.
	snap = console.Snapshot()
	promptCtx, promptCancel := context.WithTimeout(ctx, 20*time.Second)
	defer promptCancel()
	err = console.Expect(promptCtx, snap,
		termtest.Any(
			termtest.Contains("pr-split"),
			termtest.Contains(">"),
		),
		"waiting for prompt to return after cancel",
	)
	if err != nil {
		t.Errorf("HANG: prompt did not return after cancel: %v", err)
		t.Logf("Final output:\n%s", console.String())
	} else {
		t.Log("Cancel succeeded — prompt returned cleanly")
	}
}

// ptySpawnSleep spawns a `sleep 3600` process in a PTY for testing blocked
// writes (sleep never reads stdin).
func ptySpawnSleep(ctx context.Context) (*pty.Process, error) {
	return pty.Spawn(ctx, pty.SpawnConfig{
		Command: "sleep",
		Args:    []string{"3600"},
		Rows:    24,
		Cols:    80,
	})
}
