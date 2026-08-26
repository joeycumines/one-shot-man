package termmux

import (
	"testing"
)

func TestWindowPanes_ExitedFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	exitBin := buildExitProgram(t)
	setOnLoop(t, runtime, "exitBin", exitBin)

	err := awaitJSErr(t, runtime, `
		tuiMux.setRemainOnExit(true);
		var sess = termmux.newCaptureSession(exitBin);
		await sess.start();
		var paneId = tuiMux.splitHorizontal({ session: sess, target: { name: "exited-js", kind: "capture" } });
		if (paneId === 0) { throw new Error("expected valid pane id"); }

		function waitExited(deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					var sessions = tuiMux.sessions();
					for (var i = 0; i < sessions.length; i++) {
						if (sessions[i].state === "exited") return resolve();
					}
					if (Date.now() > deadlineMs) return reject(new Error("timeout waiting for session exit"));
					setTimeout(poll, 10);
				})();
			});
		}
		await waitExited(Date.now() + 5000);

		if (tuiMux.paneExited(paneId) !== true) {
			throw new Error("expected paneExited=true before respawn, got " + tuiMux.paneExited(paneId));
		}

		var panes = tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected panes"); }
		if (panes[0].exited !== true) {
			throw new Error("expected panes[0].exited=true after exit, got " + panes[0].exited);
		}

		var oldSid = 1;
		var newSid = tuiMux.respawnSession(oldSid);
		if (newSid === 0 || newSid === oldSid) {
			throw new Error("expected valid new session id, got " + newSid);
		}

		panes = tuiMux.panes();
		if (panes[0].sessionId !== newSid) {
			throw new Error("pane sessionId did not update after respawn: " + panes[0].sessionId);
		}
	`)
	if err != nil {
		t.Fatalf("windowPanes exited flag: %v", err)
	}
}
