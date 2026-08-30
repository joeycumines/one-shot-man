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

	idleBin := buildIdleProgram(t)
	setOnLoop(t, runtime, "idleBin", idleBin)

	_, err := awaitJSValue(t, runtime, `
		var s1 = await termmux.newBoundedSession({ cmd: idleBin });
		var s2 = await termmux.newBoundedSession({ cmd: idleBin });
		var mgr = s1.mgr;
		var sid = s1.sid;

		mgr.activate(s2.sid);
		mgr.setMonitorConfig(sid, { activity: true, activityThreshold: 0, activityResetThreshold: 0.05 });

		var sub = mgr.subscribe(1024);

		function waitOutput(text, deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					var snap = mgr.snapshot(sid);
					if (snap && snap.plainText && snap.plainText.indexOf(text) >= 0) return resolve();
					if (Date.now() > deadlineMs) return reject(new Error('timeout waiting output ' + text));
					setTimeout(poll, 10);
				})();
			});
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
		function waitActivity(deadlineMs) {
			return new Promise(function(resolve, reject) {
				(function poll() {
					if (countActivity() >= 1) return resolve();
					if (Date.now() > deadlineMs) return reject(new Error('timeout; activity count=0'));
					setTimeout(poll, 10);
				})();
			});
		}

		s1.session.write("hello\n");
		s1.session.write("hello2\n");
		await waitOutput("hello2", Date.now() + 3000);
		await waitActivity(Date.now() + 3000);

		mgr.resetActivity(sid);
		s1.session.write("again\n");
		s1.session.write("again2\n");
		await waitOutput("again2", Date.now() + 3000);
		await waitActivity(Date.now() + 3000);
	`)
	if err != nil {
		t.Fatalf("activity reset script: %v", err)
	}
}
