//go:build unix

package dashboard

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

func expectTitle(t *testing.T, ctx context.Context, cp *termtest.Console) {
	t.Helper()
	snap := cp.Snapshot()
	expect(t, ctx, cp, snap, "OSM Grand Interactive Dashboard", 15*time.Second)
}

func newTestProcessEnv(tb testing.TB) []string {
	tb.Helper()
	tmpDir := tb.TempDir()
	clipboardFile := filepath.Join(tmpDir, "clipboard.txt")
	return []string{
		"OSM_SESSION=dashboard-test",
		"OSM_STORE=memory",
		"OSM_CLIPBOARD=cat > " + clipboardFile,
	}
}

func resolveScriptPath(binaryPath string) string {
	root := testutil.RepoRootFromWD()
	candidate := filepath.Join(root, "scripts", "example-14-comprehensive-demo.js")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return "scripts/example-14-comprehensive-demo.js"
}

func skipIfNotUnix(t *testing.T) {
	t.Helper()
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}
	if testing.Short() {
		t.Skip("skipping slow integration test in short mode")
	}
}

func newDashboardConsole(t *testing.T, ctx context.Context) *termtest.Console {
	t.Helper()
	binaryPath := buildTestBinary(t)
	env := newTestProcessEnv(t)
	scriptPath := resolveScriptPath(binaryPath)
	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "script", scriptPath),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	require.NoError(t, err)
	return cp
}

func stripANSI(s string) string {
	result := make([]byte, 0, len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && !((s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z')) {
				i++
			}
			if i < len(s) {
				i++
			}
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return string(result)
}

func assertContains(t *testing.T, buf string, substr string, msgAndArgs ...any) {
	t.Helper()
	if termtest.Contains(substr)(buf) {
		return
	}
	assert.Contains(t, stripANSI(buf), substr, msgAndArgs...)
}

var viewMarkers = map[string]string{
	"Overview": "System Overview",
	"Showcase": "Box Demo",
	"Compose":  "Composition Editor",
	"Agents":   "Planner",
	"Builder":  "Layout Builder",
	"Log":      "Live Log Viewer",
	"Editor":   "Text Editor",
	"Help":     "Keyboard Shortcuts",
}

func switchTab(t *testing.T, ctx context.Context, cp *termtest.Console, viewName string) {
	t.Helper()
	marker := viewMarkers[viewName]
	for range 12 {
		if termtest.Contains(marker)(cp.String()) {
			return
		}
		sendKey(t, cp, "\t")
		time.Sleep(400 * time.Millisecond)
	}
	if termtest.Contains(marker)(cp.String()) {
		return
	}
	t.Fatalf("Failed to switch to view %q after 12 tabs (marker=%q)\nBuffer: %q", viewName, marker, cp.String())
}

// ============================================================================
// PTY INTEGRATION TESTS
// ============================================================================

func TestDashboard_InitialRender(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	buf := cp.String()
	assertContains(t, buf, "DARK")
	assertContains(t, buf, "View: Overview")
	assertContains(t, buf, "CPU")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_OverviewMetrics(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	buf := cp.String()
	assertContains(t, buf, "CPU")
	assertContains(t, buf, "MEM")
	assertContains(t, buf, "NET")
	assertContains(t, buf, "Processes")
	assertContains(t, buf, "\xe2\x96\x81")
	assertContains(t, buf, "PID")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_ShowcaseNavigation(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Showcase")
	time.Sleep(300 * time.Millisecond)

	buf := cp.String()
	assertContains(t, buf, "Box")
	assertContains(t, buf, "Label")
	assertContains(t, buf, "Divider")
	assertContains(t, buf, "List")
	assertContains(t, buf, "Viewport")
	assertContains(t, buf, "Textarea")

	for range 5 {
		sendKey(t, cp, "\x1b[B")
		time.Sleep(150 * time.Millisecond)
	}
	for range 5 {
		sendKey(t, cp, "\x1b[A")
		time.Sleep(150 * time.Millisecond)
	}

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_TabSwitching(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	views := []struct {
		name    string
		content string
	}{
		{"Showcase", "Box"},
		{"Compose", "Composition Editor"},
		{"Agents", "Planner"},
		{"Builder", "Layout Builder"},
		{"Log", "Live Log Viewer"},
		{"Editor", "Text Editor"},
		{"Help", "Keyboard Shortcuts"},
	}

	for _, v := range views {
		switchTab(t, ctx, cp, v.name)
		time.Sleep(300 * time.Millisecond)
		buf := cp.String()
		assertContains(t, buf, v.content, "view %s should show %s", v.name, v.content)
	}

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_ComposeInteract(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Compose")
	time.Sleep(300 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Composition Editor")
	assertContains(t, buf, "Main")

	sendKey(t, cp, "n")
	time.Sleep(300 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "P3")

	sendKey(t, cp, "m")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "\x1b[B")
	time.Sleep(200 * time.Millisecond)

	sendKey(t, cp, "r")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "\x1b[C")
	time.Sleep(200 * time.Millisecond)

	sendKey(t, cp, "d")
	time.Sleep(300 * time.Millisecond)

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_AgentsInteract(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Agents")
	time.Sleep(300 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Planner")
	assertContains(t, buf, "Coder")
	assertContains(t, buf, "Reviewer")
	assertContains(t, buf, "Tester")

	sendKey(t, cp, "\x1b[B")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "\x1b[A")
	time.Sleep(200 * time.Millisecond)

	sendKey(t, cp, "\r")
	time.Sleep(500 * time.Millisecond)

	buf = cp.String()
	assertContains(t, buf, "Planner")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_BuilderInteract(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Builder")
	time.Sleep(300 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Layout Builder")
	assertContains(t, buf, "split")

	sendKey(t, cp, "g")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "grid")

	sendKey(t, cp, "t")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "stack")

	sendKey(t, cp, "h")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "horizo")

	sendKey(t, cp, "v")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "vertica")

	sendKey(t, cp, "+")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "C")

	sendKey(t, cp, "-")
	time.Sleep(200 * time.Millisecond)

	sendKey(t, cp, "\x1b[B")
	time.Sleep(100 * time.Millisecond)

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_LogViewer(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Log")
	time.Sleep(300 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Live Log Viewer")
	assertContains(t, buf, "osm-engine")

	sendKey(t, cp, "\x1b[B")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "\x1b[B")
	time.Sleep(200 * time.Millisecond)

	sendKey(t, cp, "2")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "Filter")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_EditorInteract(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Editor")
	time.Sleep(300 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Text Editor")
	assertContains(t, buf, "Chars:")

	sendKey(t, cp, "f")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "FOCUSED")

	sendKey(t, cp, "h")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "i")
	time.Sleep(100 * time.Millisecond)

	sendKey(t, cp, "\x1b")
	time.Sleep(200 * time.Millisecond)
	buf = cp.String()
	assertContains(t, buf, "BLURRED")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_HelpView(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Help")
	time.Sleep(300 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Keyboard Shortcuts")
	assertContains(t, buf, "Tab")
	assertContains(t, buf, "Compose")
	assertContains(t, buf, "Agents")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_ModalDemo(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Showcase")
	time.Sleep(300 * time.Millisecond)

	for range 6 {
		sendKey(t, cp, "\x1b[B")
		time.Sleep(150 * time.Millisecond)
	}

	sendKey(t, cp, "\r")
	time.Sleep(500 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Modal dialog")

	sendKey(t, cp, "\r")
	time.Sleep(300 * time.Millisecond)

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_ToastDemo(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	switchTab(t, ctx, cp, "Showcase")
	time.Sleep(300 * time.Millisecond)

	sendKey(t, cp, "\r")
	time.Sleep(500 * time.Millisecond)
	buf := cp.String()
	assertContains(t, buf, "Box demo activated")

	sendKey(t, cp, "q")
	cp.WaitExit(ctx)
}

func TestDashboard_CtrlCQuit(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)
	sendKey(t, cp, "\x03")
	cp.WaitExit(ctx)
}

func TestDashboard_NoErrorsAfterFullCycle(t *testing.T) {
	skipIfNotUnix(t)
	ctx := t.Context()
	cp := newDashboardConsole(t, ctx)
	defer cp.Close()

	expectTitle(t, ctx, cp)

	time.Sleep(300 * time.Millisecond)

	switchTab(t, ctx, cp, "Showcase")
	time.Sleep(200 * time.Millisecond)
	for range 15 {
		sendKey(t, cp, "\x1b[B")
		time.Sleep(80 * time.Millisecond)
	}
	for range 15 {
		sendKey(t, cp, "\x1b[A")
		time.Sleep(80 * time.Millisecond)
	}

	switchTab(t, ctx, cp, "Compose")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "n")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "m")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "\x1b[B")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "r")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "\x1b[C")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "d")
	time.Sleep(200 * time.Millisecond)

	switchTab(t, ctx, cp, "Agents")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "\r")
	time.Sleep(300 * time.Millisecond)

	switchTab(t, ctx, cp, "Builder")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "g")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "t")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "+")
	time.Sleep(100 * time.Millisecond)

	switchTab(t, ctx, cp, "Log")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "\x1b[B")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "2")
	time.Sleep(100 * time.Millisecond)

	switchTab(t, ctx, cp, "Editor")
	time.Sleep(200 * time.Millisecond)
	sendKey(t, cp, "f")
	time.Sleep(100 * time.Millisecond)
	sendKey(t, cp, "x")
	time.Sleep(100 * time.Millisecond)

	switchTab(t, ctx, cp, "Help")
	time.Sleep(200 * time.Millisecond)

	switchTab(t, ctx, cp, "Overview")
	time.Sleep(200 * time.Millisecond)

	buf := cp.String()
	assert.NotContains(t, buf, "ERROR:")
	assert.NotContains(t, buf, "FATAL ERROR")
	assert.NotContains(t, buf, "event loop not running")
	assert.NotContains(t, buf, "TypeError")
	assert.NotContains(t, buf, "panic")

	sendKey(t, cp, "q")
	time.Sleep(500 * time.Millisecond)
	cp.WaitExit(ctx)
}

func TestDashboard_ScriptPath(t *testing.T) {
	t.Parallel()
	root := testutil.RepoRootFromWD()
	p := filepath.Join(root, "scripts", "example-14-comprehensive-demo.js")
	_, err := os.Stat(p)
	assert.NoError(t, err, "script must exist at %s", p)
}
