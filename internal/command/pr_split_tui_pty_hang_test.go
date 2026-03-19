//go:build unix && prsplit_slow

package command

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// ---------------------------------------------------------------------------
// PTY Integration Tests for TUI Hang Fix
//
// These tests exercise the actual `osm pr-split` binary in interactive mode
// through a PTY, verifying that the TUI progresses past the "Processing..."
// screen. This is the definitive end-to-end test for the TUI hang fix.
// ---------------------------------------------------------------------------

// threadSafeBuffer is a bytes.Buffer with mutex protection for concurrent
// reader (PTY pump goroutine) and poller (test goroutine) access.
type threadSafeBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *threadSafeBuffer) Write(p []byte) {
	b.mu.Lock()
	b.data = append(b.data, p...)
	b.mu.Unlock()
}

func (b *threadSafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}

// TestTUIHang_BinaryPTY_Interactive builds the actual osm binary, creates a
// test git repo, runs `osm pr-split` through a PTY (interactive mode),
// navigates to "Start Analysis", and verifies the TUI progresses past the
// CONFIG screen within a reasonable timeout.
func TestTUIHang_BinaryPTY_Interactive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping PTY integration test in short mode")
	}

	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Launch osm pr-split in interactive mode via PTY.
	// -strategy controls GROUPING (directory, extension, etc.).
	// -claude-command forces a nonexistent path so the auto-detect check
	// determines Claude is unavailable, forcing the heuristic pipeline.
	cmd := exec.CommandContext(ctx, osmBin,
		"pr-split",
		"-base=main",
		"-strategy=directory",
		"-claude-command=/nonexistent/claude",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
	)
	cmd.Dir = repoDir
	logFile := filepath.Join(t.TempDir(), "osm-debug.log")
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"OSM_CONFIG=",
		"TERM=xterm-256color",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"GIT_TERMINAL_PROMPT=0", // Prevent git credential prompts
		"GIT_PAGER=cat",         // Prevent git pager
		"NO_COLOR=1",            // Disable colors in git output
		"OSM_LOG_LEVEL=debug",
		"OSM_LOG_FILE="+logFile,
	)
	// Dump the debug log on test failure (or always for diagnosis).
	t.Cleanup(func() {
		if data, err := os.ReadFile(logFile); err == nil && len(data) > 0 {
			const maxLogDump = 8000
			s := string(data)
			if len(s) > maxLogDump {
				s = "...(truncated)...\n" + s[len(s)-maxLogDump:]
			}
			t.Logf("=== OSM DEBUG LOG ===\n%s\n=== END LOG ===", s)
		} else {
			t.Logf("No debug log found at %s (err=%v)", logFile, err)
		}
	})

	// Start with PTY (24 rows × 80 cols).
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("failed to start pty: %v", err)
	}
	defer func() {
		_ = ptmx.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Thread-safe buffer to collect output (concurrent reader/poller).
	var outputBuf threadSafeBuffer
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				outputBuf.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for initial CONFIG screen to render. BubbleTea enters alt screen
	// and draws the wizard. We look for characteristic CONFIG screen text.
	if !waitForPTYOutput(t, &outputBuf, "Start Analysis", 15*time.Second) {
		sendCtrlC(ptmx)
		t.Fatalf("CONFIG screen did not render within timeout.\nOutput so far:\n%s",
			sanitizePTYOutput(outputBuf.String()))
	}
	t.Logf("CONFIG screen rendered successfully")

	// Wait for Claude auto-detect to settle (fires 1ms after WindowSize,
	// takes a few hundred ms to run `which claude` and fail). We wait until
	// the screen stabilises.
	waitForScreenChange(t, &outputBuf, outputBuf.String(), 5*time.Second)

	// DIAGNOSTIC: First verify that keypresses reach BubbleTea at all.
	// Send '?' which should toggle the help overlay.
	_, _ = ptmx.Write([]byte("?"))
	if waitForPTYOutput(t, &outputBuf, "Help", 5*time.Second) {
		t.Logf("DIAGNOSTIC: '?' keypress reached BubbleTea (help overlay appeared)")
		// Close help: press '?' or 'esc' again
		snap := outputBuf.String()
		_, _ = ptmx.Write([]byte{0x1b}) // Escape
		waitForScreenChange(t, &outputBuf, snap, 3*time.Second)
	} else {
		t.Logf("WARNING: '?' keypress might not have reached BubbleTea")
	}

	// Navigate to "Start Analysis" (nav-next) button. Use Shift+Tab×2
	// from index 0 (the screen transition always resets focusIndex to 0),
	// which wraps backward to nav-next (always second-to-last element).
	// This is robust regardless of CONFIG's element count.
	focusNavNext(t, ptmx, &outputBuf)

	// Snapshot output to check focus state.
	preEnterOutput := outputBuf.String()
	t.Logf("Pre-Enter output tail (last 1000 chars):\n%s",
		sanitizePTYTail(preEnterOutput, 1000))

	// Press Enter to trigger "Start Analysis".
	_, _ = ptmx.Write([]byte{'\r'})
	t.Logf("Sent Shift+Tab×2 + Enter to trigger Start Analysis")

	// First check: see if "Processing..." appears in the nav bar.
	// This would confirm that startAnalysis was triggered.
	if waitForPTYOutput(t, &outputBuf, "Processing", 5*time.Second) {
		t.Logf("startAnalysis confirmed: 'Processing...' visible in nav bar")
	} else {
		// Tab navigation may have failed. Dump output for diagnosis.
		snap := outputBuf.String()
		sendCtrlC(ptmx)
		waitForScreenChange(t, &outputBuf, snap, 3*time.Second)
		// Try to find what the nav button text is now
		out := outputBuf.String()
		t.Logf("Navigation may have failed. Checking for 'Start Analysis' text still present: %v",
			strings.Contains(out, "Start Analysis"))
		t.Fatalf("Tab+Enter did not trigger startAnalysis — 'Processing...' never appeared.\n"+
			"This is a PTY test navigation issue, not the actual bug.\n"+
			"Final output tail (1500 chars):\n%s", sanitizePTYTail(out, 1500))
	}

	// startAnalysis was triggered. Now wait for the async pipeline
	// to complete and transition to PLAN_REVIEW.
	if waitForPTYOutput(t, &outputBuf, "Plan Review", 30*time.Second) {
		t.Logf("SUCCESS: TUI reached PLAN_REVIEW — async pipeline completed!")
	} else if waitForPTYOutput(t, &outputBuf, "Execute Plan", 10*time.Second) {
		t.Logf("SUCCESS: TUI reached PLAN_REVIEW (found 'Execute Plan')")
	} else {
		snap := outputBuf.String()
		sendCtrlC(ptmx)
		waitForScreenChange(t, &outputBuf, snap, 3*time.Second)
		t.Fatalf("Analysis started but TUI never reached PLAN_REVIEW.\n"+
			"This IS the async pipeline hang bug.\n"+
			"Final output:\n%s", sanitizePTYOutput(outputBuf.String()))
	}

	// Clean exit: send Ctrl+C then confirm.
	sendCtrlC(ptmx)
	waitForPTYOutput(t, &outputBuf, "Cancel", 3*time.Second)
	snap := outputBuf.String()
	_, _ = ptmx.Write([]byte("y"))
	waitForScreenChange(t, &outputBuf, snap, 3*time.Second)
}

// TestTUIHang_BinaryBatch_ProvesHeuristicWorks confirms that the heuristic
// batch pipeline works end-to-end with the strict microtask ordering. This
// is a correctness gate: if batch mode works, the analysis logic is sound
// and any remaining TUI issues are isolated to the BubbleTea bridge.
func TestTUIHang_BinaryBatch_ProvesHeuristicWorks(t *testing.T) {
	osmBin := buildOSMBinary(t)
	repoDir := setupBinaryTestRepo(t)

	stdout, stderr, err := runBinary(t, osmBin, repoDir,
		"pr-split",
		"-interactive=false",
		"-base=main",
		"-strategy=directory",
		"-verify=true",
		"--store=memory",
		"--session="+t.Name(),
		"run",
	)

	if err != nil {
		t.Fatalf("batch mode failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	if !strings.Contains(stdout, "Split executed") {
		t.Fatalf("batch mode did not complete successfully.\nstdout:\n%s", stdout)
	}

	branchCount := countSplitBranches(t, repoDir)
	if branchCount == 0 {
		t.Fatal("no split branches created in batch mode")
	}
	t.Logf("Batch mode verified: %d split branches created", branchCount)
}

// waitForPTYOutput polls the thread-safe buffer for a substring.
func waitForPTYOutput(t *testing.T, buf *threadSafeBuffer, substr string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), substr) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// sendCtrlC sends Ctrl+C to the PTY.
func sendCtrlC(ptmx *os.File) {
	_, _ = ptmx.Write([]byte{0x03})
}

// sanitizePTYOutput strips ANSI escape sequences for readable test output.
func sanitizePTYOutput(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\x1b' && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		if s[i] >= 32 || s[i] == '\n' || s[i] == '\r' || s[i] == '\t' {
			result.WriteByte(s[i])
		}
		i++
	}
	out := result.String()
	if len(out) > 3000 {
		out = "...(truncated)..." + out[len(out)-3000:]
	}
	return out
}

// sanitizePTYTail returns the last n characters of sanitized PTY output.
func sanitizePTYTail(s string, n int) string {
	clean := sanitizePTYOutput(s)
	if len(clean) > n {
		return clean[len(clean)-n:]
	}
	return clean
}
