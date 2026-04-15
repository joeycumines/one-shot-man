package command

// pr_split_tui_persistence_truthful_test.go — Tests for Task 10: persistence
// and resume truthfulness. Proves that session status annotation is truthful
// about PID liveness, clean exit removes state files, resume metadata is
// surfaced in TUI model, and placeholder rendering reflects resume state.
//
// Evidence tier: JS engine coverage with both mock tuiMux unit tests and
// real termmux persistence bindings. The focused integration tests prove
// persisted-file startup loading and clean-exit cleanup on disk.

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	termmuxmod "github.com/joeycumines/one-shot-man/internal/builtin/termmux"
	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
	"github.com/joeycumines/one-shot-man/internal/termmux"
)

func newPersistenceIntegrationEval(t *testing.T, state *termmux.PersistedManagerState) (func(string) (any, error), string) {
	t.Helper()

	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "pr-split-mux.state.json")
	if state != nil {
		if err := termmux.SaveManagerState(statePath, state); err != nil {
			t.Fatalf("save manager state: %v", err)
		}
	}

	eng := prsplittest.NewEngine(t, map[string]any{
		"persistStatePath": statePath,
		"dir":              tempDir,
	})
	eng.LoadChunks(t, prsplittest.ChunkNamesThrough("12")...)

	evalJS := eng.EvalJS(t)
	if _, err := evalJS(prsplittest.SetupTUIMocks); err != nil {
		t.Fatalf("prsplittest: TUI mocks failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	mgr := termmux.NewSessionManager()
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()
	t.Cleanup(func() {
		cancel()
		<-errCh
	})

	eng.SetGlobal("tuiMux", termmuxmod.WrapSessionManager(
		ctx,
		eng.ScriptingEngine().Runtime(),
		mgr,
		nil,
		io.Discard,
		-1,
	))
	eng.LoadChunks(t, prsplittest.ChunkNamesAfter("12")...)
	if _, err := eng.EvalJS(t)(prsplittest.Chunk16Helpers); err != nil {
		t.Fatalf("prsplittest: chunk16 helpers failed: %v", err)
	}

	return eng.EvalJS(t), statePath
}

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

// TestPersistence_ResumeStateInTUIModel proves that the base wizard state
// exposes the Task 10 resume fields with truthful zero defaults.
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

// TestPersistence_ResumeStatePopulatedByModelInit proves the real startup
// path: a persisted file is loaded through the real termmux bindings,
// prSplit.previousState is populated during chunk load, and the production
// _initModelFn surfaces truthful resume state into the TUI model.
func TestPersistence_ResumeStatePopulatedByModelInit(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS, _ := newPersistenceIntegrationEval(t, &termmux.PersistedManagerState{
		Version:  "1",
		ActiveID: 1,
		Sessions: []termmux.PersistedSession{
			{
				SessionID: 1,
				PID:       os.Getpid(),
				Target:    termmux.SessionTarget{Name: "claude", Kind: termmux.SessionKindCapture},
			},
			{
				SessionID: 2,
				PID:       0,
				Target:    termmux.SessionTarget{Name: "agent"},
			},
		},
		TermRows: 24,
		TermCols: 80,
		SavedAt:  time.Now().Add(-48 * time.Hour),
	})

	raw, err := evalJS(`(function() {
		var errors = [];
		if (!prSplit.previousState) {
			errors.push('previousState should load from the real persisted file at startup');
		}
		if (typeof prSplit._wizardModelInit !== 'function') {
			errors.push('_wizardModelInit should be exported for production-path testing');
		}
		var initRes = prSplit._wizardModelInit();
		if (!initRes || initRes.length < 1) {
			errors.push('_wizardModelInit should return [state, cmd]');
		}
		var s = initRes && initRes.length > 0 ? initRes[0] : null;
		if (!s) {
			errors.push('model init should produce state');
		} else {
			if (!s.resumeFound) errors.push('resumeFound should be true');
			if (!s.resumeStale) errors.push('resumeStale should be true for a 48h-old file');
			if (!(s.resumeAgeMs > 86400000)) errors.push('resumeAgeMs should exceed 24h, got ' + s.resumeAgeMs);
			if (s.resumeSessions.length !== 2) errors.push('expected 2 resume sessions, got ' + s.resumeSessions.length);
			if (s.resumeSessions[0] && s.resumeSessions[0].status !== 'alive') {
				errors.push('session 1 status should be alive, got ' + s.resumeSessions[0].status);
			}
			if (s.resumeSessions[1] && s.resumeSessions[1].status !== 'unknown') {
				errors.push('session 2 status should be unknown, got ' + s.resumeSessions[1].status);
			}
			s.width = 80;
			var bar = prSplit._renderStatusBar(s);
			if (bar.indexOf('alive') < 0) errors.push('status bar should show alive count');
			if (bar.indexOf('unknown') < 0) errors.push('status bar should show unknown count');
			if (bar.toLowerCase().indexOf('stale') < 0) errors.push('status bar should label stale resumes');
		}
		if (prSplit.previousState && prSplit.previousState._resumeMeta) {
			var meta = prSplit.previousState._resumeMeta;
			if (meta.aliveCount !== 1) errors.push('meta aliveCount should be 1, got ' + meta.aliveCount);
			if (meta.deadCount !== 0) errors.push('meta deadCount should be 0, got ' + meta.deadCount);
			if (meta.unknownCount !== 1) errors.push('meta unknownCount should be 1, got ' + meta.unknownCount);
		} else {
			errors.push('previousState._resumeMeta should be populated');
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

// TestPersistence_ConfirmCancelRemovesRealStateFile proves the clean-exit
// path removes the actual persisted state file via the real termmux binding.
func TestPersistence_ConfirmCancelRemovesRealStateFile(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS, statePath := newPersistenceIntegrationEval(t, &termmux.PersistedManagerState{
		Version:  "1",
		ActiveID: 1,
		Sessions: []termmux.PersistedSession{{
			SessionID: 1,
			PID:       os.Getpid(),
			Target:    termmux.SessionTarget{Name: "claude", Kind: termmux.SessionKindCapture},
		}},
		TermRows: 24,
		TermCols: 80,
		SavedAt:  time.Now(),
	})

	raw, err := evalJS(`(function() {
		var origUnwire = prSplit._unwireClaudeLifecycleEvents;
		var unwireCalled = false;
		prSplit._unwireClaudeLifecycleEvents = function() { unwireCalled = true; };

		var s = initState('PLAN_REVIEW');
		s.showConfirmCancel = true;
		s.confirmCancelFocus = 0;

		var r = update({ type: 'Key', key: 'enter' }, s);
		s = r[0];
		prSplit._unwireClaudeLifecycleEvents = origUnwire;

		var errors = [];
		if (!unwireCalled) errors.push('unwireClaudeLifecycleEvents should be called on quit');
		if (s.wizardState !== 'CANCELLED') errors.push('wizardState should be CANCELLED, got ' + s.wizardState);
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Fatalf("confirmCancel real cleanup: %v", raw)
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("persisted state file should be removed on clean exit, stat err=%v", err)
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
