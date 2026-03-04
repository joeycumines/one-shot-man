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

	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
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
