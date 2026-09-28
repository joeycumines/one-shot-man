package termmux

import (
	"strings"
	"testing"
)

func TestSendKeys_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	snapVal, snapErr := awaitJSValue(t, runtime, `
		var s = await termmux.newBoundedSession({ cmd: idleBin });
		try {
			await tuiMux.activate(s.sid);

			await tuiMux.sendKeys(s.sid, "h", "e", "l", "l", "o", "enter");

			if (typeof s.session.sendKeys !== "function") {
				throw new Error("session wrapper missing sendKeys method");
			}
			await s.session.sendKeys("w", "o", "r", "l", "d", "enter");

			return await new Promise(function(resolve, reject) {
				(function poll() {
					var snap = tuiMux.capture(s.sid);
					var text = snap && snap.plain ? snap.plain : "";
					if (text.indexOf("hello") >= 0 && text.indexOf("world") >= 0) return resolve(text);
					if (Date.now() > (poll.deadline || (poll.deadline = Date.now() + 5000))) return reject(new Error("timeout waiting for output"));
					setTimeout(poll, 50);
				})();
			});
		} finally {
			await s.session.close();
		}
	`)
	if snapErr != nil {
		t.Fatalf("sendKeys script: %v", snapErr)
	}
	snapText := snapVal.String()

	if !strings.Contains(snapText, "world") {
		t.Errorf("output missing world: %q", snapText)
	}
}
