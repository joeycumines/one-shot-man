package termmux

import (
	"fmt"
	"strings"
	"testing"
)

// copyModeWait is shared JS: defines s (bounded session) and a promise-based
// snapshot poll that cooperates with the event loop instead of busy-spinning.
const copyModeWait = `
		var s = await termmux.newBoundedSession({ cmd: echoBin, name: "copy" });
		function waitSnapshot(substr, deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					var snap = s.mgr.snapshot(s.sid);
					if (snap && snap.plainText && snap.plainText.indexOf(substr) >= 0) return resolve();
					if (Date.now() > deadlineMs) return reject(new Error('timeout waiting for ' + substr));
					setTimeout(poll, 10);
				})();
			});
		}
`

func TestCopyModeKey_JS_NotInCopyMode(t *testing.T) {
	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	echoBin := buildEchoIdleProgram(t, "hello")
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, copyModeWait+`
		await waitSnapshot("hello", Date.now() + 5000);

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
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, copyModeWait+`
		await waitSnapshot("line_", Date.now() + 5000);

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
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, copyModeWait+`
		await waitSnapshot("hello copy", Date.now() + 5000);

		var sid = s.sid;
		var mgr = s.mgr;

		mgr.enterCopyMode(sid);
		if (!mgr.isCopyModeActive(sid)) {
			throw new Error("enterCopyMode did not activate copy mode");
		}

		var fwd0 = mgr.copyModeKey(sid, "k");
		if (!fwd0.consumed) { throw new Error("k not consumed"); }
		fwd0 = mgr.copyModeKey(sid, "0");
		if (!fwd0.consumed) { throw new Error("0 not consumed"); }
		fwd0 = mgr.copyModeKey(sid, " ");
		if (!fwd0.consumed) { throw new Error("space not consumed"); }

		fwd0 = mgr.copyModeKey(sid, "end");
		if (!fwd0.consumed) { throw new Error("end not consumed"); }

		fwd0 = mgr.copyModeKey(sid, "enter");
		if (!fwd0.consumed) { throw new Error("enter not consumed"); }
		if (fwd0.action !== "CopyAndExit") { throw new Error("expected CopyAndExit, got " + fwd0.action); }

		if (mgr.isCopyModeActive(sid)) {
			throw new Error("enter should exit copy mode");
		}

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
	setOnLoop(t, runtime, "echoBin", echoBin)

	err := awaitJSErr(t, runtime, copyModeWait+`
		await waitSnapshot("search", Date.now() + 5000);

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
