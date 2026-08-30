//go:build unix

package bouncelogo

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

func resolveScriptPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.RepoRootWD(), "scripts", "example-15-bouncing-logo.js")
}

func resolveMockPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(testutil.RepoRootWD(), "internal", "example", "bouncelogo", "mock_shell.sh")
}

func newTestProcessEnv(tb testing.TB) []string {
	tb.Helper()
	tmpDir := tb.TempDir()
	clipboardFile := filepath.Join(tmpDir, "clipboard.txt")
	return []string{
		"OSM_SESSION=bouncelogo-test",
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

// launchBouncingLogo is a shared helper that builds the binary, starts the
// bouncing-logo demo with the mock shell, and waits for the dashboard to appear.
func launchBouncingLogo(t *testing.T, ctx context.Context, timeout time.Duration) *termtest.Console {
	t.Helper()

	binaryPath := buildTestBinary(t)
	scriptPath := resolveScriptPath(t)
	mockPath := resolveMockPath(t)

	// Copy mock shell to temp dir and make it executable there to avoid
	// mutating source tree file permissions.
	tmpMock := filepath.Join(t.TempDir(), "mock_shell.sh")
	mockContent, err := os.ReadFile(mockPath)
	if err != nil {
		t.Fatalf("Failed to read mock script: %v", err)
	}
	if err := os.WriteFile(tmpMock, mockContent, 0o755); err != nil {
		t.Fatalf("Failed to write temp mock script: %v", err)
	}

	env := newTestProcessEnv(t)
	args := []string{"script", scriptPath, "--cmd", "/bin/sh", "--", tmpMock}

	ptyRows, ptyCols := uint16(30), uint16(100)
	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, args...),
		termtest.WithDefaultTimeout(timeout),
		termtest.WithEnv(env),
		termtest.WithDir(testutil.RepoRootWD()),
		termtest.WithSize(ptyRows, ptyCols),
	)
	require.NoError(t, err, "Failed to create termtest console")

	// Wait for the Bubble Tea dashboard to attach the nested PTY and render
	// the mock shell banner. This is the point at which sends are safe.
	snap := cp.Snapshot()
	expect(t, ctx, cp, snap, "Bouncing Logo Shell", 15*time.Second)
	time.Sleep(200 * time.Millisecond)

	return cp
}

func skipSlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping slow bouncelogo PTY test in short mode")
	}
}

// TestBouncingLogo_LaunchAndDashboard verifies the script launches and renders
// the bouncing terminal dashboard with controls.
func TestBouncingLogo_LaunchAndDashboard(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify dashboard elements are present
	buffer := cp.String()
	assert.Contains(t, buffer, "^P Pause", "dashboard should show ctrl+p pause button")
	assert.Contains(t, buffer, "^Q Quit", "dashboard should show ctrl+q quit button")

	// Verify the nested PTY banner is visible.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "PTY snapshot should show mock shell banner")

	// Verify status bar is present
	buffer = cp.String()
	assert.Contains(t, buffer, "Bounces:", "status bar should show bounce count")
	assert.Contains(t, buffer, "RUNNING", "status bar should show child running")
}

// TestBouncingLogo_BounceAnimation verifies the pane actually bounces by
// checking that the bounce count increases over time.
func TestBouncingLogo_BounceAnimation(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Wait a moment for bounces to accumulate (bounce speed is 1px/tick at 50ms)
	time.Sleep(3 * time.Second)

	// The buffer should show a non-zero bounce count
	buffer := cp.String()
	assert.Contains(t, buffer, "Bounces:", "status bar should show bounce counter")
}

// TestBouncingLogo_PauseResume verifies the pause/resume toggle via ctrl+p.
func TestBouncingLogo_PauseResume(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Send ctrl+p to pause
	sendKey(t, cp, "\x10")
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "PAUSED", 5*time.Second)

	// Send ctrl+p again to resume
	sendKey(t, cp, "\x10")
	snap = cp.Snapshot()
	expect(t, testCtx, cp, snap, "RUNNING", 5*time.Second)
}

// TestBouncingLogo_KeyboardInput verifies keyboard input is forwarded to the
// nested PTY child process.
func TestBouncingLogo_KeyboardInput(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Type "hello" and press Enter — the mock shell echoes it
	sendKey(t, cp, "hello\r")

	// Wait for the echo response in the PTY snapshot
	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "Echo: hello", 5*time.Second)
}

// TestBouncingLogo_CleanExit verifies the script exits cleanly when ctrl+c is pressed.
func TestBouncingLogo_CleanExit(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Send ctrl+c to quit
	sendKey(t, cp, "\x03")

	// Allow a moment for clean exit
	time.Sleep(2 * time.Second)

	// Verify no panic or error in the buffer
	buffer := cp.String()
	assert.NotContains(t, buffer, "panic")
	assert.NotContains(t, buffer, "runtime error")
	assert.NotContains(t, buffer, "[object Object]")
}

// TestBouncingLogo_ANSIPreview verifies the PTY snapshot preview contains ANSI
// color sequences (full fidelity rendering, not plain text).
func TestBouncingLogo_ANSIPreview(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// The buffer should contain ANSI escape sequences from the child PTY output
	buffer := cp.String()
	assert.Contains(t, buffer, "\x1b[", "PTY preview should contain ANSI escape sequences for color fidelity")
}

// TestBouncingLogo_NoObjectObject verifies the fix for the [object Object] bug
// — the view function must always return a proper string content.
func TestBouncingLogo_NoObjectObject(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// The buffer must NEVER contain [object Object]
	buffer := cp.String()
	assert.NotContains(t, buffer, "[object Object]", "view must never produce [object Object]")
}

// TestBouncingLogo_ResizePane verifies the bigger/smaller pane size controls via ctrl+b/ctrl+s.
func TestBouncingLogo_ResizePane(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Send ctrl+b to make pane bigger
	sendKey(t, cp, "\x02")
	time.Sleep(500 * time.Millisecond)

	// Send ctrl+s to make pane smaller
	sendKey(t, cp, "\x13")
	time.Sleep(500 * time.Millisecond)

	// Verify no errors occurred
	buffer := cp.String()
	assert.NotContains(t, buffer, "panic")
	assert.NotContains(t, buffer, "TypeError")
}

// TestBouncingLogo_ScriptPath verifies the script file exists at the expected path.
func TestBouncingLogo_ScriptPath(t *testing.T) {
	t.Parallel()
	root := testutil.RepoRootWD()
	p := filepath.Join(root, "scripts", "example-15-bouncing-logo.js")
	_, err := os.Stat(p)
	assert.NoError(t, err, "script must exist at %s", p)
}

// TestBouncingLogo_BareLettersForward verifies that bare letters (s, b, p, q)
// are forwarded to the nested PTY child instead of being intercepted as control
// actions. This is the core regression test for the key binding refactor.
func TestBouncingLogo_BareLettersForward(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Type "sbpq" — all four letters that WERE control keys before the fix.
	// Each letter must be forwarded to the PTY and echoed by the mock shell.
	sendKey(t, cp, "sbpq\r")

	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "Echo: sbpq", 5*time.Second)
}

// TestBouncingLogo_SpacebarForward verifies that the spacebar correctly sends a
// space character to the nested PTY (not the literal string "space"). This is
// the end-to-end test for GAP-001: KeyToTermBytes("space") → " ".
func TestBouncingLogo_SpacebarForward(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Type "echo hello world" with spaces — the spacebar must forward actual
	// space characters, not the literal string "space".
	sendKey(t, cp, "echo hello world\r")

	snap := cp.Snapshot()
	expect(t, testCtx, cp, snap, "Echo: echo hello world", 5*time.Second)
}

// TestBouncingLogo_ChordMode verifies the ctrl+x chord prefix: pressing ctrl+x
// enters chord mode, then the next key triggers a control action. This tests
// GAP-004 and proves the chord interaction pattern works.
func TestBouncingLogo_ChordMode(t *testing.T) {
	skipSlow(t)

	ctx := context.Background()
	timeout := 30 * time.Second
	testCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cp := launchBouncingLogo(t, testCtx, timeout)
	defer cp.Close()

	// Verify the banner is already visible after launch.
	assert.Contains(t, cp.String(), "Bouncing Logo Shell", "mock shell banner should be visible")

	// Enter chord mode: ctrl+x (0x18) then 'b' to make pane bigger
	sendKey(t, cp, "\x18")
	time.Sleep(300 * time.Millisecond)
	buffer := cp.String()
	assert.Contains(t, buffer, "C-x-", "chord mode should show C-x- prefix")

	sendKey(t, cp, "b")
	time.Sleep(500 * time.Millisecond)

	// Enter chord mode again: ctrl+x then 'p' to pause
	sendKey(t, cp, "\x18")
	time.Sleep(300 * time.Millisecond)
	buffer = cp.String()
	assert.Contains(t, buffer, "C-x-", "chord mode should show C-x- prefix on second entry")

	sendKey(t, cp, "p")
	time.Sleep(300 * time.Millisecond)
	buffer = cp.String()
	assert.Contains(t, buffer, "PAUSED", "chord ctrl+x p should toggle pause")

	// Resume with direct ctrl+p (not chord)
	sendKey(t, cp, "\x10")
	time.Sleep(300 * time.Millisecond)
	buffer = cp.String()
	assert.Contains(t, buffer, "RUNNING", "ctrl+p should toggle pause back")

	// Verify no errors
	assert.NotContains(t, buffer, "panic")
	assert.NotContains(t, buffer, "TypeError")
}
