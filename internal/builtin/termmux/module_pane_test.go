package termmux

import (
	"context"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/dop251/goja"

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

	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("tuiMux", tuiMux)

	return runtime, func() {
		cancel()
		<-errCh
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

	runtime := goja.New()
	tuiMux := WrapSessionManager(ctx, runtime, mgr, nil, nil, -1, "")

	// Set up the termmux module namespace so newBoundedSession etc. are available.
	exports := runtime.NewObject()
	_ = exports.Set("newSessionManager", func(call goja.FunctionCall) goja.Value {
		return newSessionManager(ctx, runtime, call)
	})
	_ = exports.Set("newBoundedSession", func(call goja.FunctionCall) goja.Value {
		return newBoundedSession(ctx, runtime, call)
	})
	_ = runtime.Set("termmux", exports)
	_ = runtime.Set("tuiMux", tuiMux)

	return runtime, func() {
		cancel()
		<-errCh
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
