//go:build unix

package autopilot

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveScriptPath returns the absolute path to the autopilot script.
func resolveScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.RepoRootFromWD(), "scripts", "example-15-claude-autopilot.js")
}

// resolveMockPath returns the absolute path to the mock claude script.
func resolveMockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.RepoRootFromWD(), "internal", "example", "autopilot", "mock_claude.sh")
}

func newTestProcessEnv(tb testing.TB) []string {
	tb.Helper()
	tmpDir := tb.TempDir()
	clipboardFile := filepath.Join(tmpDir, "clipboard.txt")
	return []string{
		"OSM_SESSION=autopilot-test",
		"OSM_STORE=memory",
		"OSM_CLIPBOARD=cat > " + clipboardFile,
	}
}

func sendKey(t *testing.T, cp *termtest.Console, key string) {
	t.Helper()
	if _, err := cp.WriteString(key); err != nil {
		t.Fatalf("Failed to send key %q: %v\nBuffer: %q", key, err, cp.String())
	}
}

func expect(t *testing.T, ctx context.Context, cp *termtest.Console, snap termtest.Snapshot, target string, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
		t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
	}
}

// launchAutopilot is a shared helper that builds the binary, starts the autopilot
// with the mock claude, and waits for the dashboard to appear.
func launchAutopilot(t *testing.T, ctx context.Context, timeout time.Duration) *termtest.Console {
	t.Helper()

	binaryPath := buildTestBinary(t)
	scriptPath := resolveScriptPath(t)
	mockPath := resolveMockPath(t)

	if err := os.Chmod(mockPath, 0o755); err != nil {
		t.Fatalf("Failed to chmod mock script: %v", err)
	}

	env := append(newTestProcessEnv(t), "OSM_SYNC_PROTOCOL=1")
	args := []string{"script", scriptPath, "--cmd", "/bin/bash", "--", mockPath}

	ptyRows, ptyCols := uint16(30), uint16(100)
	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, args...),
		termtest.WithDefaultTimeout(timeout),
		termtest.WithEnv(env),
		termtest.WithDir(testutil.RepoRootFromWD()),
		termtest.WithSize(ptyRows, ptyCols),
	)
	require.NoError(t, err, "Failed to create termtest console")

	// Wait for dashboard title to appear
	snap := cp.Snapshot()
	expect(t, ctx, cp, snap, "Claude Code Autopilot", 15*time.Second)

	return cp
}

// TestAutopilot_LaunchAndDashboard verifies the script launches, renders the dashboard,
// and detects the mock's prompt state.
func TestAutopilot_LaunchAndDashboard(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Verify dashboard elements are present
	buffer := cp.String()
	assert.Contains(t, buffer, "AP:", "dashboard should show autopilot status")
	assert.Contains(t, buffer, "OFF", "autopilot should be off by default")

	// Wait for snapshot to load and state detection to update (mock prints "> " prompt)
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "IDLE_PROMPT", 10*time.Second)

	// Verify status bar is present after first tick renders the chrome
	buffer = cp.String()
	assert.Contains(t, buffer, "[1]", "dashboard should have session indicator in status bar")
}

// TestAutopilot_AutopilotToggle verifies the autopilot can be toggled on/off.
func TestAutopilot_AutopilotToggle(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Send 'a' to toggle autopilot on
	sendKey(t, cp, "a")
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "AP:ON", 5*time.Second)

	// Send 'a' again to toggle off
	sendKey(t, cp, "a")
	snap = cp.Snapshot()
	expect(t, testCtx, cp, snap, "AP:OFF", 5*time.Second)
}

// TestAutopilot_ManualKick verifies the manual kick injects a message into the PTY.
func TestAutopilot_ManualKick(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Send 'k' for manual kick from dashboard
	sendKey(t, cp, "k")
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "MANUAL_KICK", 5*time.Second)
}

// TestAutopilot_InputMode verifies input mode sends text to the PTY child.
func TestAutopilot_InputMode(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Wait for snapshot to show the mock prompt
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "IDLE_PROMPT", 10*time.Second)

	// Enter input mode
	sendKey(t, cp, "i")
	snap = cp.Snapshot()
	expect(t, testCtx, cp, snap, "INPUT>", 5*time.Second)

	// Type "hello"
	sendKey(t, cp, "hello")
	snap = cp.Snapshot()
	expect(t, testCtx, cp, snap, "hello_", 5*time.Second)

	// Press Enter to send
	sendKey(t, cp, "\r")

	// Wait for the mock's echo response to appear in the snapshot preview
	snap = cp.Snapshot()
	expect(t, testCtx, cp, snap, "Echo: hello", 5*time.Second)
}

// TestAutopilot_CleanExit verifies the script exits cleanly when 'q' is pressed.
func TestAutopilot_CleanExit(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Send 'q' to quit
	sendKey(t, cp, "q")

	// Allow a moment for clean exit
	time.Sleep(2 * time.Second)

	// Verify no panic or error in the buffer
	buffer := cp.String()
	assert.NotContains(t, buffer, "panic")
	assert.NotContains(t, buffer, "runtime error")
	assert.NotContains(t, buffer, "[object Object]")
}

// TestAutopilot_ANSIPreview verifies the PTY snapshot preview contains ANSI
// color sequences (full fidelity rendering, not plain text).
func TestAutopilot_ANSIPreview(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Wait for snapshot to populate
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "IDLE_PROMPT", 10*time.Second)

	// The buffer should contain ANSI escape sequences from the child PTY output
	// (the snapshot.ansi field preserves SGR color codes)
	buffer := cp.String()
	assert.Contains(t, buffer, "\x1b[", "PTY preview should contain ANSI escape sequences for color fidelity")
}

// TestAutopilot_ButtonClick verifies mouse click on bubblezone-marked toggle button
// triggers the autopilot toggle. Uses SGR-1006 mouse encoding to send a click at
// the toggle button's position in the controls bar (row 28, columns 0–9).
func TestAutopilot_ButtonClick(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchAutopilot(t, testCtx, timeout)
	defer cp.Close()

	// Wait for snapshot to load and state detection
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "IDLE_PROMPT", 10*time.Second)

	// Verify autopilot is OFF initially
	buffer := cp.String()
	assert.Contains(t, buffer, "AP:OFF", "autopilot should start OFF")

	// Click on the toggle button via SGR mouse encoding (1-indexed).
	// The controls bar is at row H-2 = 28 (0-based) = row 29 (1-indexed SGR).
	// The toggle button "[○] Toggle" starts at column 0 (0-based) = column 1 (1-indexed).
	// Click at column 4, row 29 to hit the middle of the toggle button.
	mousePress := "\x1b[<0;4;29M"
	mouseRelease := "\x1b[<0;4;29m"
	sendKey(t, cp, mousePress)
	sendKey(t, cp, mouseRelease)

	// Wait for autopilot to toggle ON
	snap = cp.Snapshot()
	expect(t, testCtx, cp, snap, "AP:ON", 5*time.Second)
}

func skipSlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow autopilot PTY test in short mode")
	}
}
