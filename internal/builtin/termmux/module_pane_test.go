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
	"sync"
	"testing"

	"github.com/dop251/goja"
	goeventloop "github.com/joeycumines/go-eventloop"
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

	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
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

	go loop.Run(ctx)

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

	loop, err := goeventloop.New(goeventloop.WithStrictMicrotaskOrdering(true))
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

	go loop.Run(ctx)

	tuiMux := WrapSessionManager(ctx, adapter, runtime, mgr, nil, nil, -1, "")

	// Set up the termmux module namespace so newBoundedSession etc. are available.
	exports := runtime.NewObject()
	_ = exports.Set("newSessionManager", func(call goja.FunctionCall) goja.Value {
		return newSessionManager(ctx, adapter, runtime, call)
	})
	_ = exports.Set("newBoundedSession", func(call goja.FunctionCall) goja.Value {
		return newBoundedSession(ctx, adapter, runtime, call)
	})
	_ = exports.Set("newCaptureSession", func(call goja.FunctionCall) goja.Value {
		return newCaptureSession(ctx, runtime, call)
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

	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: "sh" });
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
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var sess1 = termmux.newBoundedSession({ cmd: "sh" });
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
		if (newWin !== 2) {
			throw new Error("expected window 2, got " + newWin);
		}
	`)
	if err != nil {
		t.Fatalf("breakPane: %v", err)
	}

	_, err = runtime.RunString(`
		var err = tuiMux.joinPane(1, 1);
		if (err) {
			throw new Error("joinPane failed: " + err.message);
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

	// Register a session and split to create panes.
	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(sess.session, { name: "test" });
		var sess2 = termmux.newBoundedSession({ cmd: "sh" });
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
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	// Register a session and split to create a second pane.
	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(sess.session, { name: "test" });
		var sess2 = termmux.newBoundedSession({ cmd: "sh" });
		tuiMux.register(sess2.session, { name: "test2" });
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

	// Register a session.
	_, err := runtime.RunString(`
		var sess = termmux.newBoundedSession({ cmd: "sh" });
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

	_, err := runtime.RunString(`
		function mkSession(name) {
			var s = termmux.newBoundedSession({ cmd: "sh" });
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

	_, err := runtime.RunString(`
		function mkSession(name) {
			var s = termmux.newCaptureSession("sh");
			tuiMux.register(s, { name: name });
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
		if (active.name !== "s2") { throw new Error("active session = " + active.name + ", want s2"); }

		// Drive input into the active session and verify it reaches the window.
		tuiMux.input("echo routed\n");
		var deadline = Date.now() + 3000;
		var found = false;
		while (Date.now() < deadline) {
			var snap = tuiMux.snapshot(active.id);
			if (snap && snap.plainText && snap.plainText.indexOf("routed") >= 0) {
				found = true;
				break;
			}
		}
		if (!found) {
			var snap = tuiMux.snapshot(active.id);
			throw new Error("input did not reach active session; snapshot = " + (snap && snap.plainText));
		}

		// Add the active session id as a read-only variable for Go-side assertions.
		__activeSessionId = active.id;
	`)
	if err != nil {
		t.Fatalf("window switch JS routing: %v", err)
	}
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
