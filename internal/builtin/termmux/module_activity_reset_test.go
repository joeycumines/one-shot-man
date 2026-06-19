package termmux

import (
	"testing"
)

func TestActivityReset_JSBinding(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}

	runtime, cleanup := setupTmuxModule(t)
	defer cleanup()

	_, err := runtime.RunString(`
		var s1 = termmux.newBoundedSession({ cmd: "cat" });
		var s2 = termmux.newBoundedSession({ cmd: "cat" });
		var mgr = s1.mgr;
		var sid = s1.sid;

		mgr.activate(s2.sid);
		mgr.setMonitorConfig(sid, { activity: true, activityThreshold: 0, activityResetThreshold: 0.05 });

		var sub = mgr.subscribe(1024);

		function waitOutput(text, deadline) {
			while (Date.now() < deadline) {
				var snap = mgr.snapshot(sid);
				if (snap && snap.plainText && snap.plainText.indexOf(text) >= 0) {
					return true;
				}
			}
			return false;
		}
		function countActivity() {
			var found = 0;
			var evts = sub.pollEvents();
			for (var j = 0; j < evts.length; j++) {
				if (evts[j].kind === "activity" && Number(evts[j].sessionId) === Number(sid)) {
					found++;
				}
			}
			return found;
		}
		function waitActivity(deadline) {
			var saw = 0;
			while (Date.now() < deadline) {
				var n = countActivity();
				saw += n;
				if (n >= 1) {
					return;
				}
			}
			throw new Error("timeout; activity count=" + saw);
		}

		s1.session.write("hello\n");
		s1.session.write("hello2\n");
		if (!waitOutput("hello2", Date.now() + 3000)) {
			throw new Error("expected output from s1");
		}
		waitActivity(Date.now() + 3000);

		mgr.resetActivity(sid);
		s1.session.write("again\n");
		s1.session.write("again2\n");
		if (!waitOutput("again2", Date.now() + 3000)) {
			throw new Error("expected output from s1 after reset");
		}
		waitActivity(Date.now() + 3000);
	`)
	if err != nil {
		t.Fatalf("activity reset script: %v", err)
	}
}
