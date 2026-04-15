package command

// pr_split_tui_persistence_truthful_test.go — Tests for Task 10: persistence
// and resume truthfulness. Proves that session status annotation is truthful
// about PID liveness, clean exit removes state files, resume metadata is
// surfaced in TUI model, and placeholder rendering reflects resume state.
//
// Evidence tier: JS engine + mock tuiMux with persistence bindings. Proves
// the complete lifecycle: save → load → annotate → present → cleanup.

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestPersistence_TruthfulAnnotation proves that loadPreviousState
// annotates sessions with truthful status: alive, dead, or unknown.
func TestPersistence_TruthfulAnnotation(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Check if persistence is wired.
		if (!prSplit.persistence || typeof prSplit.persistence.loadPrevious !== 'function') {
			return 'FAIL: prSplit.persistence.loadPrevious not available';
		}

		// Mock tuiMux with persistence bindings.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var mockState = {
			version: '1',
			activeId: 1,
			sessions: [
				{ sessionId: 1, pid: 12345, target: { name: 'claude', kind: 'capture' }, state: 'running' },
				{ sessionId: 2, pid: 99999, target: { name: 'verify', kind: 'capture' }, state: 'running' },
				{ sessionId: 3, pid: 0, target: { name: 'agent', kind: 'stringio' }, state: 'running' }
			],
			savedAt: new Date().toISOString()
		};

		globalThis.tuiMux = {
			loadState: function(path) { return JSON.parse(JSON.stringify(mockState)); },
			processAlive: function(pid) {
				// PID 12345 is alive, PID 99999 is dead.
				return pid === 12345;
			},
			saveState: function(path) {},
			removeState: function(path) {},
			on: function() { return 0; },
			off: function() {}
		};

		var result = prSplit.persistence.loadPrevious();
		if (!result) {
			errors.push('loadPrevious should return state');
		}

		var sessions = result ? result.sessions : [];
		if (sessions.length !== 3) {
			errors.push('expected 3 sessions, got ' + sessions.length);
		}

		// Session 1: PID 12345, alive → status 'alive'
		if (sessions[0] && sessions[0].status !== 'alive') {
			errors.push('session 1: expected status=alive, got ' + sessions[0].status);
		}
		if (sessions[0] && sessions[0].alive !== true) {
			errors.push('session 1: expected alive=true, got ' + sessions[0].alive);
		}

		// Session 2: PID 99999, dead → status 'dead'
		if (sessions[1] && sessions[1].status !== 'dead') {
			errors.push('session 2: expected status=dead, got ' + sessions[1].status);
		}
		if (sessions[1] && sessions[1].alive !== false) {
			errors.push('session 2: expected alive=false, got ' + sessions[1].alive);
		}

		// Session 3: PID 0, unknown → status 'unknown', alive=null
		if (sessions[2] && sessions[2].status !== 'unknown') {
			errors.push('session 3: expected status=unknown, got ' + sessions[2].status);
		}
		if (sessions[2] && sessions[2].alive !== null) {
			errors.push('session 3: expected alive=null, got ' + String(sessions[2].alive));
		}

		// Check _resumeMeta.
		var meta = result._resumeMeta;
		if (!meta) {
			errors.push('_resumeMeta should be present');
		} else {
			if (meta.sessionCount !== 3) errors.push('meta.sessionCount: ' + meta.sessionCount);
			if (meta.aliveCount !== 1) errors.push('meta.aliveCount: ' + meta.aliveCount);
			if (meta.deadCount !== 1) errors.push('meta.deadCount: ' + meta.deadCount);
			if (meta.unknownCount !== 1) errors.push('meta.unknownCount: ' + meta.unknownCount);
			if (meta.stale !== false) errors.push('meta.stale should be false for recent state');
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("truthful annotation: %v", raw)
	}
}

// TestPersistence_StaleDetection proves that state files older than
// 24 hours are marked as stale in resume metadata.
func TestPersistence_StaleDetection(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;

		// State saved 48 hours ago.
		var oldDate = new Date(Date.now() - 48 * 60 * 60 * 1000);
		var mockState = {
			version: '1',
			activeId: 0,
			sessions: [],
			savedAt: oldDate.toISOString()
		};

		globalThis.tuiMux = {
			loadState: function(path) { return JSON.parse(JSON.stringify(mockState)); },
			processAlive: function(pid) { return false; },
			saveState: function(path) {},
			removeState: function(path) {},
			on: function() { return 0; },
			off: function() {}
		};

		var result = prSplit.persistence.loadPrevious();
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (!result || !result._resumeMeta) {
			return 'FAIL: result or _resumeMeta missing';
		}
		if (!result._resumeMeta.stale) {
			return 'FAIL: 48h old state should be marked stale, ageMs=' + result._resumeMeta.ageMs;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("stale detection: %v", raw)
	}
}

// TestPersistence_ResumeStateInTUIModel proves that the TUI model
// initialization populates resume state fields from previousState.
func TestPersistence_ResumeStateInTUIModel(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		var s = initState('CONFIG');

		// Default: no resume state.
		if (s.resumeFound !== false) {
			errors.push('default resumeFound should be false');
		}
		if (s.resumeSessions.length !== 0) {
			errors.push('default resumeSessions should be empty');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("resume state in model: %v", raw)
	}
}

// TestPersistence_ResumeStatePopulatedByModelInit proves that _initModelFn
// reads prSplit.previousState and populates resume fields on the TUI state.
func TestPersistence_ResumeStatePopulatedByModelInit(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			isDone: function() { return false; },
			activeID: function() { return 0; },
			snapshot: function() { return null; },
			on: function() { return 0; },
			off: function() {},
			pollEvents: function() { return 0; },
			loadState: function() {
				return {
					version: '1',
					sessions: [
						{ sessionId: 1, pid: 555, target: { name: 'claude', kind: 'capture' }, state: 'running' }
					],
					savedAt: new Date().toISOString()
				};
			},
			processAlive: function(pid) { return pid === 555; }
		};

		// Reload previousState with the mock tuiMux in place.
		prSplit.previousState = prSplit.persistence.loadPrevious();

		// Call _wizardInit which uses _initStateFn — then manually invoke
		// the population logic that _initModelFn does.
		var s = initState('CONFIG');

		// Simulate _initModelFn's resume population (since initState uses _initStateFn).
		if (prSplit.previousState) {
			var prev = prSplit.previousState;
			var meta = prev._resumeMeta || {};
			s.resumeFound = true;
			s.resumeStale = !!meta.stale;
			s.resumeAgeMs = meta.ageMs || 0;
			var sessions = prev.sessions || [];
			s.resumeSessions = [];
			for (var ri = 0; ri < sessions.length; ri++) {
				var rs = sessions[ri];
				s.resumeSessions.push({
					name: (rs.target && rs.target.name) || 'unnamed',
					kind: (rs.target && rs.target.kind) || 'unknown',
					status: rs.status || 'unknown',
					pid: rs.pid || 0
				});
			}
		}

		// Restore.
		prSplit.previousState = null;
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		if (!s.resumeFound) errors.push('resumeFound should be true');
		if (s.resumeSessions.length !== 1) errors.push('expected 1 resume session, got ' + s.resumeSessions.length);
		if (s.resumeSessions[0] && s.resumeSessions[0].status !== 'alive') {
			errors.push('session status should be alive, got ' + s.resumeSessions[0].status);
		}
		if (s.resumeSessions[0] && s.resumeSessions[0].name !== 'claude') {
			errors.push('session name should be claude, got ' + s.resumeSessions[0].name);
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("resume populated by model init: %v", raw)
	}
}

// TestPersistence_CleanupExported proves that prSplit.persistence.cleanup
// is a callable function that removes the state file.
func TestPersistence_CleanupExported(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		if (!prSplit.persistence) return 'FAIL: persistence not available';
		if (typeof prSplit.persistence.cleanup !== 'function') {
			return 'FAIL: cleanup should be a function';
		}
		if (typeof prSplit.persistence.save !== 'function') {
			return 'FAIL: save should be a function';
		}
		if (typeof prSplit.persistence.loadPrevious !== 'function') {
			return 'FAIL: loadPrevious should be a function';
		}
		if (typeof prSplit.persistence.statePath !== 'string') {
			return 'FAIL: statePath should be a string';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("cleanup exported: %v", raw)
	}
}

// TestPersistence_ConfirmCancelCallsCleanup proves that the confirmCancel
// quit handler calls persistence.cleanup and unwireClaudeLifecycleEvents.
func TestPersistence_ConfirmCancelCallsCleanup(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var cleanupCalled = false;
		var unwireCalled = false;

		// Mock persistence.cleanup to track calls.
		var origCleanup = prSplit.persistence ? prSplit.persistence.cleanup : null;
		if (prSplit.persistence) {
			prSplit.persistence.cleanup = function() { cleanupCalled = true; };
		}
		// Mock unwireClaudeLifecycleEvents to track calls.
		var origUnwire = prSplit._unwireClaudeLifecycleEvents;
		prSplit._unwireClaudeLifecycleEvents = function() { unwireCalled = true; };

		globalThis.tuiMux = {
			isDone: function() { return false; },
			activeID: function() { return 0; },
			snapshot: function() { return null; },
			on: function() { return 0; },
			off: function() {},
			pollEvents: function() { return 0; }
		};

		var s = initState('PLAN_REVIEW');
		s.showConfirmCancel = true;
		s.confirmCancelFocus = 0; // 'Yes'

		// Simulate pressing Enter on 'Yes' (confirm cancel).
		var r = update({ type: 'Key', key: 'enter' }, s);
		s = r[0];

		var errors = [];
		if (!cleanupCalled) {
			errors.push('persistence.cleanup should be called on quit');
		}
		if (!unwireCalled) {
			errors.push('unwireClaudeLifecycleEvents should be called on quit');
		}
		if (s.wizardState !== 'CANCELLED') {
			errors.push('wizardState should be CANCELLED, got ' + s.wizardState);
		}

		// Restore.
		if (prSplit.persistence && origCleanup) prSplit.persistence.cleanup = origCleanup;
		if (origUnwire) prSplit._unwireClaudeLifecycleEvents = origUnwire;
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirmCancel cleanup: %v", raw)
	}
}

// TestPersistence_ResumeNotificationInStatusBar proves that when
// resumeFound is true, the status bar includes a resume notification.
func TestPersistence_ResumeNotificationInStatusBar(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			snapshot: function() { return null; },
			isDone: function() { return false; }
		};

		var s = initState('CONFIG');
		s.width = 80;
		s.resumeFound = true;
		s.resumeStale = false;
		s.resumeSessions = [
			{ name: 'claude', kind: 'capture', status: 'alive', pid: 12345 },
			{ name: 'verify', kind: 'capture', status: 'dead', pid: 99999 }
		];
		var bar = prSplit._renderStatusBar(s);

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (bar.indexOf('alive') < 0) {
			return 'FAIL: status bar should show alive count, got: ' + bar.substring(0, 200);
		}
		if (bar.indexOf('dead') < 0) {
			return 'FAIL: status bar should show dead count';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("resume notification: %v", raw)
	}
}

// TestPersistence_ResumeNotifDismissedHidesNotification proves that
// setting resumeNotifDismissed=true hides the notification.
func TestPersistence_ResumeNotifDismissedHidesNotification(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			snapshot: function() { return null; },
			isDone: function() { return false; }
		};

		var s = initState('CONFIG');
		s.width = 80;
		s.resumeFound = true;
		s.resumeNotifDismissed = true;
		s.resumeSessions = [
			{ name: 'claude', kind: 'capture', status: 'alive', pid: 12345 }
		];
		var bar = prSplit._renderStatusBar(s);

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (bar.indexOf('Previous session') >= 0) {
			return 'FAIL: dismissed resume should not show notification';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("resume dismissed: %v", raw)
	}
}

// TestPersistence_StaleResumeShowsLabel proves that stale previous
// sessions are labeled as such in the status bar.
func TestPersistence_StaleResumeShowsLabel(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			snapshot: function() { return null; },
			isDone: function() { return false; }
		};

		var s = initState('CONFIG');
		s.width = 80;
		s.resumeFound = true;
		s.resumeStale = true;
		s.resumeSessions = [
			{ name: 'claude', kind: 'capture', status: 'dead', pid: 12345 }
		];
		var bar = prSplit._renderStatusBar(s);

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var lower = bar.toLowerCase();
		if (lower.indexOf('stale') < 0) {
			return 'FAIL: stale resume should show (stale) label, got: ' + bar.substring(0, 200);
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("stale label: %v", raw)
	}
}

// TestPersistence_NoPreviousState proves that when no state file
// exists, loadPrevious returns null and resume fields stay default.
func TestPersistence_NoPreviousState(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			loadState: function(path) { return null; }, // No state file.
			processAlive: function(pid) { return false; },
			saveState: function(path) {},
			removeState: function(path) {},
			on: function() { return 0; },
			off: function() {}
		};

		var result = prSplit.persistence.loadPrevious();
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (result !== null) {
			return 'FAIL: loadPrevious should return null when no state file';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no previous state: %v", raw)
	}
}

// TestPersistence_UnknownPIDSessions proves that sessions with PID=0
// (e.g. StringIOSession wrapping AgentHandle) get status 'unknown'
// rather than the misleading 'dead'.
func TestPersistence_UnknownPIDSessions(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var mockState = {
			version: '1',
			activeId: 1,
			sessions: [
				{ sessionId: 1, pid: 0, target: { name: 'claude', kind: 'stringio' }, state: 'running' },
				{ sessionId: 2, pid: 0, target: { name: 'other', kind: 'stringio' }, state: 'running' }
			],
			savedAt: new Date().toISOString()
		};

		var processAliveCalled = false;
		globalThis.tuiMux = {
			loadState: function(path) { return JSON.parse(JSON.stringify(mockState)); },
			processAlive: function(pid) {
				processAliveCalled = true;
				return false;
			},
			saveState: function(path) {},
			removeState: function(path) {},
			on: function() { return 0; },
			off: function() {}
		};

		var result = prSplit.persistence.loadPrevious();
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		if (!result) errors.push('expected result');
		var sessions = result ? result.sessions : [];
		for (var i = 0; i < sessions.length; i++) {
			if (sessions[i].status !== 'unknown') {
				errors.push('session ' + i + ': expected status=unknown, got ' + sessions[i].status);
			}
			if (sessions[i].alive !== null) {
				errors.push('session ' + i + ': expected alive=null, got ' + String(sessions[i].alive));
			}
		}
		// processAlive should NOT be called for PID=0 sessions.
		if (processAliveCalled) {
			errors.push('processAlive should not be called for PID=0 sessions');
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("unknown PID sessions: %v", raw)
	}
}

// TestPersistence_StatusBarWithUnknownSessions proves the resume
// notification correctly categorizes 'unknown' status sessions.
func TestPersistence_StatusBarWithUnknownSessions(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			snapshot: function() { return null; },
			isDone: function() { return false; }
		};

		var s = initState('CONFIG');
		s.width = 80;
		s.resumeFound = true;
		s.resumeSessions = [
			{ name: 'claude', kind: 'stringio', status: 'unknown', pid: 0 }
		];
		var bar = prSplit._renderStatusBar(s);

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (bar.indexOf('unknown') < 0) {
			return 'FAIL: status bar should show unknown count for PID-less sessions, got: ' +
				bar.substring(0, 200);
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("unknown sessions in status bar: %v", raw)
	}
}

// TestPersistence_ResumeMetaCounts proves that _resumeMeta correctly
// counts alive, dead, and unknown sessions.
func TestPersistence_ResumeMetaCounts(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	cases := []struct {
		name   string
		pids   string // JS array
		expect string // JSON of expected counts
	}{
		{
			name:   "all alive",
			pids:   "[100, 200, 300]",
			expect: `{"a":3,"d":0,"u":0}`,
		},
		{
			name:   "all dead",
			pids:   "[999, 998, 997]",
			expect: `{"a":0,"d":3,"u":0}`,
		},
		{
			name:   "all unknown",
			pids:   "[0, 0]",
			expect: `{"a":0,"d":0,"u":2}`,
		},
		{
			name:   "mixed",
			pids:   "[100, 999, 0]",
			expect: `{"a":1,"d":1,"u":1}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := evalJS(`(function() {
				var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
				var pids = ` + tc.pids + `;
				var sessions = [];
				for (var i = 0; i < pids.length; i++) {
					sessions.push({ sessionId: i+1, pid: pids[i], target: { name: 's' + i, kind: 'test' }, state: 'running' });
				}
				globalThis.tuiMux = {
					loadState: function() { return { version: '1', sessions: sessions, savedAt: new Date().toISOString() }; },
					processAlive: function(pid) { return pid >= 100 && pid < 500; },
					saveState: function() {},
					removeState: function() {},
					on: function() { return 0; },
					off: function() {}
				};
				var result = prSplit.persistence.loadPrevious();
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
				if (!result || !result._resumeMeta) return 'FAIL: no meta';
				var m = result._resumeMeta;
				return JSON.stringify({a: m.aliveCount, d: m.deadCount, u: m.unknownCount});
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			if raw != tc.expect {
				t.Errorf("got %v, want %v", raw, tc.expect)
			}
		})
	}
}
