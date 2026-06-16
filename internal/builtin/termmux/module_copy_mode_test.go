package termmux

import (
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestCopyModeKey_JS_NotInCopyMode(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "/bin/echo", args: ["hello"], rows: 10, cols: 40, name: "copy" });
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(mustRunInt(t, runtime, "s.sid"))
	mgrVal := mustRunValue(t, runtime, "s.mgr")

	waitForSnapshotText(t, runtime, mgrVal, sid, "hello")

	_, err = runtime.RunString(`
		var sid = s.sid;
		var mgr = s.mgr;

		var unknown = mgr.copyModeKey(sid, "x");
		if (unknown.error === "") {
			throw new Error("expected error for key outside copy mode");
		}

		var esc = mgr.copyModeKey(sid, "esc");
		if (esc.error !== "") {
			throw new Error("esc outside copy mode should be a no-op: " + esc.error);
		}

		var colon = mgr.copyModeKey(sid, ":");
		if (colon.error !== "") {
			throw new Error("colon failed: " + colon.error);
		}
		if (!mgr.isCopyModeActive(sid)) {
			throw new Error("colon should enter copy mode");
		}

		var q = mgr.copyModeKey(sid, "q");
		if (q.error !== "") {
			throw new Error("q failed: " + q.error);
		}
		if (mgr.isCopyModeActive(sid)) {
			throw new Error("q should exit copy mode");
		}
	`)
	if err != nil {
		t.Fatalf("copy-mode key test: %v", err)
	}
}

func TestCopyModeKey_JS_ScrollMovement(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var args = [];
		for (var i = 0; i < 100; i++) { args.push("line_" + i); }
		var s = termmux.newBoundedSession({ cmd: "/bin/echo", args: args, rows: 10, cols: 40, name: "copy" });
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(mustRunInt(t, runtime, "s.sid"))
	mgrVal := mustRunValue(t, runtime, "s.mgr")

	waitForSnapshotText(t, runtime, mgrVal, sid, "line_")

	_, err = runtime.RunString(`
		var sid = s.sid;
		var mgr = s.mgr;
		mgr.enterCopyMode(sid);

		var g = mgr.copyModeKey(sid, "g");
		if (g.error !== "") throw new Error("g: " + g.error);

		var k = mgr.copyModeKey(sid, "k");
		if (k.error !== "") throw new Error("k: " + k.error);
		if (k.action !== "MoveUp(1)") throw new Error("unexpected action for k: " + k.action);

		var j = mgr.copyModeKey(sid, "j");
		if (j.error !== "") throw new Error("j: " + j.error);
		if (j.action !== "MoveDown(1)") throw new Error("unexpected action for j: " + j.action);

		var pg = mgr.copyModeKey(sid, "pageUp");
		if (pg.error !== "") throw new Error("pageUp: " + pg.error);
		if (pg.action !== "PageUp") throw new Error("unexpected action for pageUp: " + pg.action);

		var pd = mgr.copyModeKey(sid, "pageDown");
		if (pd.error !== "") throw new Error("pageDown: " + pd.error);
		if (pd.action !== "PageDown") throw new Error("unexpected action for pageDown: " + pd.action);

		if (!mgr.isCopyModeActive(sid)) throw new Error("copy mode should remain active");
	`)
	if err != nil {
		t.Fatalf("scroll test: %v", err)
	}
}

func TestCopyModeKey_JS_SelectAndCopy(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "/bin/echo", args: ["hello copy mode world"], rows: 10, cols: 80, name: "copy" });
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(mustRunInt(t, runtime, "s.sid"))
	mgrVal := mustRunValue(t, runtime, "s.mgr")

	waitForSnapshotText(t, runtime, mgrVal, sid, "hello copy")

	events := make(chan goja.Value, 1)
	_ = runtime.Set("__copyEvent", func(v goja.Value) { events <- v })

	_, err = runtime.RunString(`
		var sid = s.sid;
		var mgr = s.mgr;

		mgr.addEventListener("clipboard", function(e) {
			__copyEvent(e.detail && e.detail.data ? e.detail.data : "");
		});

		mgr.enterCopyMode(sid);
		mgr.copyModeKey(sid, "0");
		mgr.copyModeKey(sid, " ");
		mgr.copyModeKey(sid, "end");
		mgr.copyModeKey(sid, "enter");

		if (mgr.isCopyModeActive(sid)) {
			throw new Error("enter should exit copy mode");
		}
	`)
	if err != nil {
		t.Fatalf("select/copy test: %v", err)
	}

	select {
	case data := <-events:
		text := data.String()
		if !strings.Contains(text, "hello copy mode world") {
			t.Fatalf("clipboard missing expected text: %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for clipboard event")
	}
}

func TestCopyModeKey_JS_SearchKeys(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: "/bin/echo", args: ["search me"], rows: 10, cols: 40, name: "copy" });
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(mustRunInt(t, runtime, "s.sid"))
	mgrVal := mustRunValue(t, runtime, "s.mgr")

	waitForSnapshotText(t, runtime, mgrVal, sid, "search")

	_, err = runtime.RunString(`
		var sid = s.sid;
		var mgr = s.mgr;
		mgr.enterCopyMode(sid);

		var fwd = mgr.copyModeKey(sid, "/");
		if (fwd.error !== "") throw new Error("/: " + fwd.error);
		if (fwd.action !== "SearchForward") throw new Error("unexpected action: " + fwd.action);

		var bwd = mgr.copyModeKey(sid, "?");
		if (bwd.error !== "") throw new Error("?: " + bwd.error);
		if (bwd.action !== "SearchBackward") throw new Error("unexpected action: " + bwd.action);

		var n = mgr.copyModeKey(sid, "n");
		if (n.error !== "") throw new Error("n: " + n.error);
	`)
	if err != nil {
		t.Fatalf("search key test: %v", err)
	}
}

func mustRunInt(t *testing.T, runtime *goja.Runtime, expr string) int64 {
	t.Helper()
	v, err := runtime.RunString(expr)
	if err != nil {
		t.Fatalf("run %q: %v", expr, err)
	}
	return v.ToInteger()
}

func mustRunValue(t *testing.T, runtime *goja.Runtime, expr string) goja.Value {
	t.Helper()
	v, err := runtime.RunString(expr)
	if err != nil {
		t.Fatalf("run %q: %v", expr, err)
	}
	return v
}
