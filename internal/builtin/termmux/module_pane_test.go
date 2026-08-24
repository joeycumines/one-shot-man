package termmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

func setupPaneMgr(t *testing.T) (*goja.Runtime, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter.Bind: %v", err)
	}

	tuiMux := WrapSessionManager(ctx, adapter, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	return runtime, func() {
		cancel()
		<-errCh
		_ = loop.Shutdown(context.Background())
	}
}

func setupTmuxModule(t *testing.T) (*goja.Runtime, func()) {
	t.Helper()
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager()
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	loop, err := goeventloop.New()
	if err != nil {
		t.Fatalf("create event loop: %v", err)
	}

	runtime := goja.New()
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatalf("adapter.Bind: %v", err)
	}

	tuiMux := WrapSessionManager(ctx, adapter, runtime, mgr, nil, nil, -1, "")

	// Set up the termmux module namespace so newBoundedSession etc. are available.
	exports := runtime.NewObject()
	_ = exports.Set("newSessionManager", func(call goja.FunctionCall) goja.Value {
		return newSessionManager(ctx, adapter, runtime, call)
	})
	_ = exports.Set("newBoundedSession", func(call goja.FunctionCall) goja.Value {
		return newBoundedSession(ctx, adapter, runtime, mgr, call)
	})
	_ = exports.Set("newCaptureSession", func(call goja.FunctionCall) goja.Value {
		return newCaptureSession(ctx, adapter, runtime, call)
	})
	_ = runtime.Set("termmux", exports)
	_ = runtime.Set("tuiMux", tuiMux)

	return runtime, func() {
		cancel()
		<-errCh
		_ = loop.Shutdown(context.Background())
	}
}

func TestPaneMethods_PanesEmpty(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	v, err := runtime.RunString(`JSON.stringify(tuiMux.panes())`)
	if err != nil {
		t.Fatalf("panes(): %v", err)
	}
	if v.String() != "[]" {
		t.Fatalf("panes() = %s, want []", v.String())
	}
}

func TestPaneMethods_ActivePaneIdZero(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	v, err := runtime.RunString(`tuiMux.activePaneId()`)
	if err != nil {
		t.Fatalf("activePaneId(): %v", err)
	}
	if v.ToInteger() != 0 {
		t.Fatalf("activePaneId() = %d, want 0", v.ToInteger())
	}
}

func TestPaneMethods_FocusPaneDirectionsNoPanes(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	for _, dir := range []string{"Up", "Down", "Left", "Right"} {
		v, err := runtime.RunString(fmt.Sprintf("tuiMux.focusPane%s()", dir))
		if err != nil {
			t.Fatalf("focusPane%s(): %v", dir, err)
		}
		if v.ToInteger() != 0 {
			t.Fatalf("focusPane%s() = %d, want 0 (no panes)", dir, v.ToInteger())
		}
	}
}

func TestPaneMethods_ClosePaneInvalid(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.closePane(999);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("closePane(999): %v", err)
	}
}

func TestPaneMethods_ResizePaneInvalid(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.resizePane(999, 0.5);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("resizePane(999, 0.5): %v", err)
	}
}

func TestPaneMethods_SplitHorizontalNoArgs(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.splitHorizontal();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("splitHorizontal(): %v", err)
	}
}

func TestPaneMethods_SplitVerticalNoArgs(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.splitVertical();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("splitVertical(): %v", err)
	}
}

func TestPaneMethods_ResizePaneArgCount(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.resizePane(1);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("resizePane(1): %v", err)
	}
}

func TestPaneMethods_ClosePaneArgCount(t *testing.T) {
	runtime, cleanup := setupPaneMgr(t)
	defer cleanup()

	_, err := runtime.RunString(`
		try {
			tuiMux.closePane();
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("closePane(): %v", err)
	}
}

func TestPaneMethods_FocusPaneAtAndResizePaneAt(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	mgr := parent.NewSessionManager(parent.WithTermSize(80, 24))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	defer func() {
		cancel()
		<-errCh
	}()

	sess := newTestSession(t, ctx)
	sess2 := newTestSession(t, ctx)

	if _, err := mgr.NewPane(sess, parent.SessionTarget{Kind: parent.SessionKindPTY}, parent.SplitDown); err != nil {
		t.Fatalf("first NewPane: %v", err)
	}
	if _, err := mgr.NewPane(sess2, parent.SessionTarget{Kind: parent.SessionKindPTY}, parent.SplitDown); err != nil {
		t.Fatalf("second NewPane: %v", err)
	}

	if id, err := mgr.FocusAt(15, 10); err != nil {
		t.Fatalf("FocusAt: %v", err)
	} else if id == 0 {
		t.Fatal("FocusAt returned zero pane")
	}

	geoms := mgr.Panes()
	if len(geoms) < 2 {
		t.Fatal("expected at least 2 panes")
	}
	dividerRow := geoms[0].Geometry.Row + geoms[0].Geometry.Rows

	if err := mgr.ResizePaneAt(dividerRow, 10, 0.7); err != nil {
		t.Fatalf("ResizePaneAt: %v", err)
	}
}

func TestBreakPane_CreatesNewWindow(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.register(sess.session, { name: "test" });
		var winId = tuiMux.newWindow("win1");
		tuiMux.addPaneToWindow(sess.session, { name: "test", windowId: winId });
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = runtime.RunString(`
		var newWin = tuiMux.breakPane(1);
		if (newWin <= 0) {
			throw new Error("breakPane should return new window ID > 0, got " + newWin);
		}
	`)
	if err != nil {
		t.Fatalf("breakPane: %v", err)
	}

	_, err = runtime.RunString(`
		var wins = tuiMux.windows();
		if (wins.length !== 2) {
			throw new Error("expected 2 windows, got " + wins.length);
		}
	`)
	if err != nil {
		t.Fatalf("window count: %v", err)
	}
}

func TestJoinPane_MovesPaneBetweenWindows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		var sess1 = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.register(sess1.session, { name: "test" });
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, err = runtime.RunString(`
		var winId = tuiMux.newWindow("win1");
		tuiMux.addPaneToWindow(sess1.session, { name: "test", windowId: winId });
	`)
	if err != nil {
		t.Fatalf("addPaneToWindow: %v", err)
	}

	_, err = runtime.RunString(`
		var newWin = tuiMux.breakPane(1);
		if (newWin.windowID !== 2) {
			throw new Error("expected window 2, got " + newWin.windowID);
		}
	`)
	if err != nil {
		t.Fatalf("breakPane: %v", err)
	}

	_, err = runtime.RunString(`
		var result = tuiMux.joinPane(1, 1);
		if (!result || !result.paneID) {
			throw new Error("joinPane failed: returned " + JSON.stringify(result));
		}
	`)
	if err != nil {
		t.Fatalf("joinPane: %v", err)
	}
}

func TestBreakPane_InvalidPaneId(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	// Break with a non-existent pane ID should error.
	_, err := runtime.RunString(`
		try {
			tuiMux.breakPane(999);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("breakPane(999): %v", err)
	}
}

func TestJoinPane_InvalidPaneId(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	// Join with a non-existent pane ID should error.
	_, err := runtime.RunString(`
		try {
			tuiMux.joinPane(999, 1);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("joinPane(999, 1): %v", err)
	}
}

func TestJoinPane_InvalidTargetWindow(t *testing.T) {

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	// Register a session and split to create panes.
	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.register(sess.session, { name: "test" });
		var sess2 = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.register(sess2.session, { name: "test2" });
		tuiMux.splitVertical({ session: sess2.session, target: { name: "test2" } });
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Join to a non-existent window should error.
	_, err = runtime.RunString(`
		try {
			tuiMux.joinPane(1, 999);
			throw new Error("expected error");
		} catch (e) {
			if (e.message === "expected error") throw e;
		}
	`)
	if err != nil {
		t.Fatalf("joinPane(1, 999): %v", err)
	}
}

func TestZoomSwap_StillWork(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	// Register sessions and create panes via splitVertical.
	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.splitVertical({ session: sess.session, target: { name: "test" } });
		var sess2 = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.splitVertical({ session: sess2.session, target: { name: "test2" } });
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Zoom should not error.
	_, err = runtime.RunString(`
		tuiMux.zoomPane(1);
	`)
	if err != nil {
		t.Fatalf("zoomPane: %v", err)
	}

	// Swap panes should not error.
	_, err = runtime.RunString(`
		tuiMux.swapPanes(1, 2);
	`)
	if err != nil {
		t.Fatalf("swapPanes: %v", err)
	}
}

func TestRespawnSession_StillWorks(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	// Register a session.
	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: idleBin });
		tuiMux.register(sess.session, { name: "test" });
	`)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Respawn should not error.
	_, err = runtime.RunString(`
		var newSid = tuiMux.respawnSession(1);
	`)
	if err != nil {
		t.Fatalf("respawnSession: %v", err)
	}
}

func TestPaneMethodBindings(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		function mkSession(name) {
			var s = termmux.newBoundedSession({ cmd: idleBin });
			tuiMux.register(s.session, { name: name });
			return s.session;
		}
		mkSession("test");
		var pid = tuiMux.splitHorizontal({ session: mkSession("sp"), target: { name: "sp" } });
		if (typeof pid !== "bigint" && typeof pid !== "number") { throw new Error("splitHorizontal returned non-numeric"); }
		pid = tuiMux.splitVertical({ session: mkSession("sv"), target: { name: "sv" } });
		if (typeof pid !== "bigint" && typeof pid !== "number") { throw new Error("splitVertical returned non-numeric"); }
		tuiMux.focusPaneUp();
		tuiMux.focusPaneDown();
		tuiMux.focusPaneLeft();
		tuiMux.focusPaneRight();
		var activeId = tuiMux.activePaneId();
		var paneList = tuiMux.panes();
		if (paneList.length === 0) { throw new Error("expected panes"); }
		tuiMux.resizePane(1, 0.7);
		var winId = tuiMux.newWindow("winAdded");
		tuiMux.addPaneToWindow(mkSession("added"), { target: { name: "added" }, windowId: winId });
		tuiMux.zoomPane(1);
		tuiMux.zoomPane(1);
		tuiMux.swapPanes(1, 2);
		tuiMux.closePane(2);
	`)
	if err != nil {
		t.Fatalf("pane method binding test: %v", err)
	}
}

func TestWindowSwitch_JSRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	_ = runtime.Set("idleBin", idleBin)

	_, err := runtime.RunString(`
		function mkSession(name) {
			// Use newCaptureSession to create a CaptureSession without
			// registering it with the SessionManager. addPaneToWindow will
			// register it. This avoids double-registration which creates
			// two per-session output goroutines that compete for the same
			// CaptureSession output channel.
			var s = termmux.newCaptureSession(idleBin, [], { name: name });
			s.start();
			return s;
		}

		var s1 = mkSession("s1");
		var w1 = tuiMux.newWindow("w1");
		var p1 = tuiMux.addPaneToWindow(s1, { windowId: w1, target: { name: "w1p1" } });
		if (p1 === 0) { throw new Error("expected valid pane id for s1"); }

		var s2 = mkSession("s2");
		var w2 = tuiMux.newWindow("w2");
		var p2 = tuiMux.addPaneToWindow(s2, { windowId: w2, target: { name: "w2p1" } });
		if (p2 === 0) { throw new Error("expected valid pane id for s2"); }

		var before = {
			activeWindow: tuiMux.activeWindowID(),
			activePane: tuiMux.activePaneId(),
			panes: tuiMux.panes().length
		};

		var next = tuiMux.nextWindow();
		if (next !== w2) { throw new Error("nextWindow = " + next + ", want " + w2); }

		var after = {
			activeWindow: tuiMux.activeWindowID(),
			activePane: tuiMux.activePaneId(),
			panes: tuiMux.panes().length
		};
		if (after.activeWindow !== w2) { throw new Error("activeWindow after switch = " + after.activeWindow + ", want " + w2); }
		if (after.activePane !== p2) { throw new Error("activePane after switch = " + after.activePane + ", want " + p2); }
		if (after.panes !== 1) { throw new Error("expected 1 pane after switch, got " + after.panes); }

		var sessions = tuiMux.sessions();
		var active = sessions.filter(function(s) { return s.isActive; })[0];
		if (!active) { throw new Error("no active session after switch"); }
		if (active.target.name !== "w2p1") { throw new Error("active session = " + active.target.name + ", want w2p1"); }

		// Drive input into the active session and verify it reaches the window.
		// Send "echo routed" + enter as individual keys to avoid any encoding
		// issues with cmd.exe on Windows (which requires \r not \n).
		var keys = ("echo routed").split("");
		keys.push("enter");
		tuiMux.sendKeys.apply(tuiMux, [active.id].concat(keys));
		__activeId = active.id;
	`)
	if err != nil {
		t.Fatalf("window switch JS routing: %v", err)
	}

	// Wait for cmd.exe (Windows) to fully initialize before polling.
	// On Windows, cmd.exe prints a startup banner that takes variable time.
	time.Sleep(1 * time.Second)

	// Poll for "routed" in the snapshot with Go-side delays.
	// The JS while-loop was a busy-loop that starved the SessionManager worker.
	activeIdVal := runtime.Get("__activeId")
	activeId := uint64(activeIdVal.ToInteger())
	_ = activeId // used in JS via __activeId variable
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		v, err := runtime.RunString(`
			var snap = tuiMux.snapshot(__activeId);
			snap && snap.plainText ? snap.plainText : ""
		`)
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if strings.Contains(v.String(), "routed") {
			return // SUCCESS
		}
		time.Sleep(50 * time.Millisecond)
	}

	snapVal, _ := runtime.RunString(`var snap = tuiMux.snapshot(__activeId); snap && snap.plainText ? snap.plainText : "(nil)"`)
	t.Fatalf("input did not reach active session; snapshot = %s", snapVal.String())
}

func newTestSession(t *testing.T, ctx context.Context) *parent.StringIOSession {
	t.Helper()
	sio := &dummyStringIO{doneCh: make(chan struct{})}
	sess := parent.NewStringIOSession(sio)
	sess.Start()
	t.Cleanup(func() { _ = sess.Close() })
	return sess
}

type dummyStringIO struct {
	mu     sync.Mutex
	doneCh chan struct{}
	closed bool
}

func (d *dummyStringIO) Send(string) error { return nil }

func (d *dummyStringIO) Receive() (string, error) {
	<-d.doneCh
	return "", io.EOF
}

func (d *dummyStringIO) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.closed {
		d.closed = true
		close(d.doneCh)
	}
	return nil
}

func buildExitProgram(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := `package main
import "fmt"
func main() { fmt.Println("hello respawn") }
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}

	binName := "exitprogram"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

// buildEchoProgram builds a cross-platform binary that prints the given text
// to stdout and exits immediately, replacing "echo text" or "/bin/echo text".
func buildEchoProgram(t *testing.T, text string) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := fmt.Sprintf(`package main

import "fmt"

func main() {
	fmt.Println(%q)
}
`, text)
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write echo helper source: %v", err)
	}

	binName := "echoprogram"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build echo helper: %v\n%s", err, stderr.String())
	}
	return bin
}

// buildIdleProgram builds a cross-platform binary that stays running indefinitely
// (reading from stdin), replacing "cat" or "sh" in tests that need a long-lived
// process attached to a PTY.
func buildIdleProgram(t *testing.T) string {
	t.Helper()
	// On Unix, use the system "cat" binary directly — it's universally available
	// and avoids the overhead of building a Go binary for every test invocation.
	if runtime.GOOS != "windows" {
		return "cat"
	}
	// On Windows, use cmd.exe — it correctly handles ConPTY console input.
	// Go's io.Copy(os.Stdout, os.Stdin) does not work because ReadConsole on
	// a ConPTY console input requires the console's line-input mode which
	// cmd.exe configures automatically.
	return "cmd.exe"
}

// buildEchoIdleProgram builds a cross-platform binary that prints the given text
// to stdout and then stays running indefinitely (reading from stdin), replacing
// "sh -c 'echo text; exec cat'" patterns in tests.
func buildEchoIdleProgram(t *testing.T, text string) string {
	t.Helper()

	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	prog := fmt.Sprintf(`package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
)

func main() {
	fmt.Println(%q)
	go io.Copy(os.Stdout, os.Stdin)
	c := make(chan os.Signal, 1)
	signal.Notify(c)
	<-c
}
`, text)
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write echo-idle helper source: %v", err)
	}

	binName := "echoidle"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)

	cmd := exec.Command("go", "build", "-o", bin, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build echo-idle helper: %v\n%s", err, stderr.String())
	}
	return bin
}

func TestRespawnSession_JSBinding_RebindsPane(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	exitBin := buildExitProgram(t)
	_ = runtime.Set("exitBin", exitBin)

	_, err := runtime.RunString(`
		var sess = termmux.newCaptureSession(exitBin);
		sess.start();
		tuiMux.setRemainOnExit(true);
		var paneId = tuiMux.splitHorizontal({ session: sess, target: { name: "respawn-js", kind: "capture" } });
		if (paneId === 0) { throw new Error("expected valid pane id"); }

		var sid = 1;
		var deadline = Date.now() + 5000;
		var exited = false;
		while (Date.now() < deadline) {
			var list = tuiMux.sessions();
			for (var i = 0; i < list.length; i++) {
				if (list[i].state === "exited") {
					exited = true;
					break;
				}
			}
			if (exited) break;
		}
		if (!exited) { throw new Error("timeout waiting for session exit"); }

		var newSid = tuiMux.respawnSession(sid);
		if (newSid === 0 || newSid === sid) {
			throw new Error("expected valid new session id, got " + newSid);
		}

		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected at least one pane"); }
		if (panes[0].sessionId !== newSid) {
			throw new Error("pane sessionId = " + panes[0].sessionId + ", want " + newSid);
		}
	`)
	if err != nil {
		t.Fatalf("respawnSession JS end-to-end: %v", err)
	}
}
