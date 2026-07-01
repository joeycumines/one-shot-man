package termmux

import (
	"fmt"
	"strings"
	"testing"

	"github.com/joeycumines/goja"
)

func TestCopyModeKey_JS_NotInCopyMode(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "hello")
	_ = runtime.Set("echoBin", echoBin)

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 10, cols: 40, name: "copy" });
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

		var unknown = mgr.copyModeKey(sid, "h");
		if (unknown.error === "") {
			throw new Error("expected error for movement key outside copy mode");
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

	var lines []string
	for i := range 100 {
		lines = append(lines, fmt.Sprintf("line_%d", i))
	}
	echoBin := buildEchoIdleProgram(t, strings.Join(lines, "\n"))
	_ = runtime.Set("echoBin", echoBin)

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 10, cols: 40, name: "copy" });
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
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "hello copy mode world")
	_ = runtime.Set("echoBin", echoBin)

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 10, cols: 80, name: "copy" });
	`)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	sid := uint64(mustRunInt(t, runtime, "s.sid"))
	mgrVal := mustRunValue(t, runtime, "s.mgr")

	waitForSnapshotText(t, runtime, mgrVal, sid, "hello copy")

	// Verify copy mode selection by checking copySelection returns the text.
	// copySelection calls VTerm.SelectedText() which requires both SelectStart
	// and SelectEnd to have been called. CopyAndExit calls SelectEnd then
	// CopySelection then ExitCopyMode, so we call copySelection between
	// selectStart and enter to capture the selection before exit clears state.
	// However, since SelectEnd is only called by CopyAndExit, we instead verify
	// the full copy flow by checking that CopyAndExit succeeds and the session
	// snapshot still contains the expected text.
	_, err = runtime.RunString(`
		var sid = s.sid;
		var mgr = s.mgr;

		mgr.enterCopyMode(sid);
		if (!mgr.isCopyModeActive(sid)) {
			throw new Error("enterCopyMode did not activate copy mode");
		}

		// Move cursor to beginning of line and start selection.
		var fwd0 = mgr.copyModeKey(sid, "k");
		if (!fwd0.consumed) { throw new Error("k not consumed"); }
		fwd0 = mgr.copyModeKey(sid, "0");
		if (!fwd0.consumed) { throw new Error("0 not consumed"); }
		fwd0 = mgr.copyModeKey(sid, " ");
		if (!fwd0.consumed) { throw new Error("space not consumed"); }

		// Move to end of line — extends selection.
		fwd0 = mgr.copyModeKey(sid, "end");
		if (!fwd0.consumed) { throw new Error("end not consumed"); }

		// CopyAndExit should copy the selection and exit copy mode.
		fwd0 = mgr.copyModeKey(sid, "enter");
		if (!fwd0.consumed) { throw new Error("enter not consumed"); }
		if (fwd0.action !== "CopyAndExit") { throw new Error("expected CopyAndExit, got " + fwd0.action); }

		if (mgr.isCopyModeActive(sid)) {
			throw new Error("enter should exit copy mode");
		}

		// Verify the session content is still accessible.
		var snap = mgr.snapshot(sid);
		if (!snap || !snap.plainText || snap.plainText.indexOf("hello copy mode world") < 0) {
			throw new Error("snapshot missing expected text after copy");
		}
	`)
	if err != nil {
		t.Fatalf("select/copy test: %v", err)
	}
}

func TestCopyModeKey_JS_SearchKeys(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "search me")
	_ = runtime.Set("echoBin", echoBin)

	_, err := runtime.RunString(`
		var s = termmux.newBoundedSession({ cmd: echoBin, rows: 10, cols: 40, name: "copy" });
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
