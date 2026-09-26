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
		await tuiMux.setRemainOnExit(true);
		var sess = termmux.newCaptureSession(exitBin);
		await sess.start();
		var paneId = await tuiMux.splitHorizontal({ session: sess, target: { name: "exited-js", kind: "capture" } });
		if (paneId === 0) { throw new Error("expected valid pane id"); }

		async function waitExited(deadlineMs) {
			while (Date.now() <= deadlineMs) {
				var sessions = await tuiMux.sessions();
				for (var i = 0; i < sessions.length; i++) {
					if (sessions[i].state === "exited") return;
				}
				await new Promise(function(resolve) { setTimeout(resolve, 10); });
			}
			throw new Error("timeout waiting for session exit");
		}
		await waitExited(Date.now() + 5000);

		if (await tuiMux.paneExited(paneId) !== true) {
			throw new Error("expected paneExited=true before respawn, got " + await tuiMux.paneExited(paneId));
		}

		var panes = await tuiMux.panes();
		if (panes.length === 0) { throw new Error("expected panes"); }
		if (panes[0].exited !== true) {
			throw new Error("expected panes[0].exited=true after exit, got " + panes[0].exited);
		}

		var oldSid = 1;
		var newSid = await tuiMux.respawnSession(oldSid);
		if (newSid === 0 || newSid === oldSid) {
			throw new Error("expected valid new session id, got " + newSid);
		}

		panes = await tuiMux.panes();
		if (panes[0].sessionId !== newSid) {
			throw new Error("pane sessionId did not update after respawn: " + panes[0].sessionId);
		}
	`)
	if err != nil {
		t.Fatalf("windowPanes exited flag: %v", err)
	}
}
