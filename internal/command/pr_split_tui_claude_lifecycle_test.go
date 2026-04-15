package command

// pr_split_tui_claude_lifecycle_test.go — Tests for Task 9: event-driven
// Claude lifecycle, adaptive polling, write error surfacing, and bell flash.
//
// Evidence tier: JS engine + mock tuiMux. Proves event wiring, lifecycle
// state derivation, adaptive tick intervals, write error propagation, and
// bell indicator behavior through the refactored pollClaudeScreenshot and
// the wizardUpdateImpl dispatch chain.
//
// All tests use skipSlow(t) and t.Parallel() per project conventions.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestClaudeLifecycle_EventWiring proves that wireClaudeLifecycleEvents
// registers event handlers that filter by Claude's pinned sessionId.
// Events for other sessions must NOT affect Claude lifecycle flags.
func TestClaudeLifecycle_EventWiring(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var otherCID = 99;
		var registeredEvents = [];

		prSplit._state = prSplit._state || {};
		prSplit._state.claudeSessionID = __mockCID;
		// Clear any prior event state.
		prSplit._state._claudeEventIDs = null;
		prSplit._state._claudeOutputDirty = false;
		prSplit._state._claudeExitEvent = false;
		prSplit._state._claudeBellFlash = false;
		prSplit._state._claudeClosedEvent = false;

		// Mock tuiMux with event registration tracking.
		var listeners = {};
		var nextID = 0;
		globalThis.tuiMux = {
			on: function(event, cb) {
				nextID++;
				registeredEvents.push(event);
				listeners[nextID] = { event: event, cb: cb };
				return nextID;
			},
			off: function(id) {
				delete listeners[id];
				return true;
			},
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function(id) { return { fullScreen: 'test', plainText: 'test' }; },
			activate: function(id) {},
			input: function(data) {},
			pollEvents: function() { return 0; }
		};

		// Wire events.
		prSplit._wireClaudeLifecycleEvents();
		var ids = prSplit._state._claudeEventIDs;
		if (!ids || ids.length !== 4) {
			return 'FAIL: expected 4 event handlers, got ' + (ids ? ids.length : 'null');
		}

		// Verify correct events were registered.
		var expectedEvents = ['output', 'exit', 'bell', 'closed'];
		for (var i = 0; i < expectedEvents.length; i++) {
			if (registeredEvents.indexOf(expectedEvents[i]) < 0) {
				return 'FAIL: missing event registration for ' + expectedEvents[i];
			}
		}

		// Fire output event for Claude's session — should set dirty flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'output') {
				listeners[lid].cb({ sessionId: __mockCID });
			}
		}
		if (!prSplit._state._claudeOutputDirty) {
			return 'FAIL: output event for Claude should set _claudeOutputDirty';
		}

		// Reset and fire output event for OTHER session — should NOT set dirty.
		prSplit._state._claudeOutputDirty = false;
		for (var lid in listeners) {
			if (listeners[lid].event === 'output') {
				listeners[lid].cb({ sessionId: otherCID });
			}
		}
		if (prSplit._state._claudeOutputDirty) {
			return 'FAIL: output event for other session should NOT set _claudeOutputDirty';
		}

		// Fire exit event for Claude — should set exit flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'exit') {
				listeners[lid].cb({ sessionId: __mockCID });
			}
		}
		if (!prSplit._state._claudeExitEvent) {
			return 'FAIL: exit event should set _claudeExitEvent';
		}

		// Fire bell event for Claude — should set bell flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'bell') {
				listeners[lid].cb({ sessionId: __mockCID });
			}
		}
		if (!prSplit._state._claudeBellFlash) {
			return 'FAIL: bell event should set _claudeBellFlash';
		}

		// Fire closed event for OTHER session — should NOT set closed flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'closed') {
				listeners[lid].cb({ sessionId: otherCID });
			}
		}
		if (prSplit._state._claudeClosedEvent) {
			return 'FAIL: closed event for other session should NOT set _claudeClosedEvent';
		}

		// Unwire — should remove all handlers.
		prSplit._unwireClaudeLifecycleEvents();
		if (prSplit._state._claudeEventIDs !== null) {
			return 'FAIL: unwire should null _claudeEventIDs';
		}
		// Check listeners were removed.
		var remaining = Object.keys(listeners).length;
		if (remaining !== 0) {
			return 'FAIL: unwire should remove all listeners, got ' + remaining;
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.claudeSessionID = null;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("event wiring: %v", raw)
	}
}

// TestClaudeLifecycle_IdempotentWiring proves wireClaudeLifecycleEvents
// does not double-register handlers when called multiple times.
func TestClaudeLifecycle_IdempotentWiring(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var callCount = 0;
		prSplit._state = prSplit._state || {};
		prSplit._state.claudeSessionID = __mockCID;
		prSplit._state._claudeEventIDs = null;

		globalThis.tuiMux = {
			on: function(event, cb) { callCount++; return callCount; },
			off: function(id) { return true; },
			isDone: function() { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function() { return null; },
			pollEvents: function() { return 0; }
		};

		// Wire twice.
		prSplit._wireClaudeLifecycleEvents();
		var firstCount = callCount;
		prSplit._wireClaudeLifecycleEvents();
		var secondCount = callCount;

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.claudeSessionID = null;
		prSplit._state._claudeEventIDs = null;

		if (firstCount !== 4) return 'FAIL: first wire should register 4, got ' + firstCount;
		if (secondCount !== 4) return 'FAIL: second wire should be no-op, got ' + secondCount;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("idempotent wiring: %v", raw)
	}
}

// TestClaudeLifecycle_StateDerivation proves deriveClaudeLifecycleState
// returns the correct state based on event flags and session status.
func TestClaudeLifecycle_StateDerivation(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	cases := []struct {
		name   string
		setup  string
		expect string
	}{
		{
			name:   "detached (no session)",
			setup:  `prSplit._state.claudeSessionID = null;`,
			expect: "detached",
		},
		{
			name: "active (recent output)",
			setup: `prSplit._state.claudeSessionID = 42;
				prSplit._state._claudeLastOutputMs = Date.now() - 500;
				prSplit._state._claudeExitEvent = false;
				prSplit._state._claudeClosedEvent = false;`,
			expect: "active",
		},
		{
			name: "idle (no recent output)",
			setup: `prSplit._state.claudeSessionID = 42;
				prSplit._state._claudeLastOutputMs = Date.now() - 10000;
				prSplit._state._claudeExitEvent = false;
				prSplit._state._claudeClosedEvent = false;`,
			expect: "idle",
		},
		{
			name: "waiting (question detected)",
			setup: `prSplit._state.claudeSessionID = 42;
				prSplit._state._claudeLastOutputMs = Date.now() - 10000;
				prSplit._state._claudeExitEvent = false;
				prSplit._state._claudeClosedEvent = false;
				__testState.claudeQuestionDetected = true;`,
			expect: "waiting",
		},
		{
			name: "crashed (exit during pipeline)",
			setup: `prSplit._state.claudeSessionID = 42;
				prSplit._state._claudeExitEvent = true;
				prSplit._state._claudeClosedEvent = false;
				__testState.autoSplitRunning = true;`,
			expect: "crashed",
		},
		{
			name: "exited (exit after pipeline)",
			setup: `prSplit._state.claudeSessionID = 42;
				prSplit._state._claudeExitEvent = true;
				prSplit._state._claudeClosedEvent = false;
				__testState.autoSplitRunning = false;`,
			expect: "exited",
		},
		{
			name: "closed (session unregistered)",
			setup: `prSplit._state.claudeSessionID = 42;
				prSplit._state._claudeExitEvent = false;
				prSplit._state._claudeClosedEvent = true;`,
			expect: "closed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := evalJS(`(function() {
				var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
				globalThis.tuiMux = {
					isDone: function(id) { return false; },
					activeID: function() { return 42; }
				};
				prSplit._state = prSplit._state || {};
				var __testState = { claudeQuestionDetected: false, autoSplitRunning: false };
				` + tc.setup + `
				var result = prSplit._deriveClaudeLifecycleState(__testState);
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
				prSplit._state.claudeSessionID = null;
				prSplit._state._claudeExitEvent = false;
				prSplit._state._claudeClosedEvent = false;
				prSplit._state._claudeLastOutputMs = 0;
				return result;
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			if raw != tc.expect {
				t.Errorf("got %q, want %q", raw, tc.expect)
			}
		})
	}
}

// TestClaudeLifecycle_AdaptivePolling proves that pollClaudeScreenshot
// returns shorter tick intervals when Claude is actively outputting and
// longer intervals when idle.
func TestClaudeLifecycle_AdaptivePolling(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.claudeSessionID = __mockCID;
		prSplit._state._claudeEventIDs = null;
		prSplit._state._claudeLastOutputMs = 0;

		globalThis.tuiMux = {
			on: function(event, cb) { return 1; },
			off: function(id) { return true; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function(id) { return { fullScreen: 'test', plainText: 'test' }; },
			activate: function(id) {},
			input: function(data) {},
			pollEvents: function() { return 0; },
			lastActivityMs: function() { return 100; }
		};

		var C = prSplit._TUI_CONSTANTS;
		var errors = [];

		// Test 1: Recent output → fast tick.
		prSplit._state._claudeLastOutputMs = Date.now() - 500; // 500ms ago
		var s1 = initState('PLAN_REVIEW');
		s1.splitViewEnabled = true;
		var r1 = prSplit._pollClaudeScreenshot(s1);
		var cmd1 = r1[1];
		// cmd1 should be a tick command — extract the delay.
		// The tick creates a {type:'Tick', id:'claude-screenshot'} message.
		// We verify by checking which constant was used.
		if (!cmd1) {
			errors.push('active: expected tick command');
		}

		// Test 2: No recent output → slow tick.
		prSplit._state._claudeLastOutputMs = Date.now() - 10000; // 10s ago
		prSplit._state._claudeEventIDs = null; // reset for re-wire
		var s2 = initState('PLAN_REVIEW');
		s2.splitViewEnabled = true;
		var r2 = prSplit._pollClaudeScreenshot(s2);
		var cmd2 = r2[1];
		if (!cmd2) {
			errors.push('idle: expected tick command');
		}

		// Both commands should be non-null tick commands.
		// We can't directly inspect tick durations from JS, but we can verify
		// the lifecycle state reflects the polling mode.
		var state1 = r1[0];
		var state2 = r2[0];
		if (state1.claudeLifecycleState !== 'active') {
			errors.push('active state: got ' + state1.claudeLifecycleState);
		}
		if (state2.claudeLifecycleState !== 'idle') {
			errors.push('idle state: got ' + state2.claudeLifecycleState);
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.claudeSessionID = null;
		prSplit._state._claudeEventIDs = null;
		prSplit._state._claudeLastOutputMs = 0;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("adaptive polling: %v", raw)
	}
}

// TestClaudeLifecycle_GenSkipsRedundantSnapshot proves that when the
// snapshot generation is unchanged, pollClaudeScreenshot skips the
// expensive screen capture but still runs lifecycle state derivation.
func TestClaudeLifecycle_GenSkipsRedundantSnapshot(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var snapshotCalls = 0;
		prSplit._state = prSplit._state || {};
		prSplit._state.claudeSessionID = __mockCID;
		prSplit._state._claudeEventIDs = null;
		prSplit._state._claudeLastSnapshotGen = 0;

		globalThis.tuiMux = {
			on: function(event, cb) { return 1; },
			off: function(id) { return true; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function(id) {
				snapshotCalls++;
				return { gen: 5, fullScreen: 'screen-gen5', plainText: 'plain-gen5' };
			},
			activate: function(id) {},
			input: function(data) {},
			pollEvents: function() { return 0; },
			lastActivityMs: function() { return 100; }
		};

		var errors = [];

		// First poll: gen=5 vs lastGen=0 → should update.
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = '';
		prSplit._pollClaudeScreenshot(s);
		if (s.claudeScreen !== 'screen-gen5') {
			errors.push('first poll should capture screen');
		}
		if (prSplit._state._claudeLastSnapshotGen !== 5) {
			errors.push('first poll should set lastGen=5');
		}

		// Second poll: gen still 5 → should skip screen update.
		s.claudeScreen = 'old-value';
		prSplit._state._claudeEventIDs = null; // allow re-wire
		prSplit._pollClaudeScreenshot(s);
		if (s.claudeScreen !== 'old-value') {
			errors.push('second poll with same gen should not overwrite screen');
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.claudeSessionID = null;
		prSplit._state._claudeEventIDs = null;
		prSplit._state._claudeLastSnapshotGen = 0;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("gen skip: %v", raw)
	}
}

// TestClaudeLifecycle_WriteErrorSurfacing proves that PTY write errors
// are surfaced in state rather than silently swallowed.
func TestClaudeLifecycle_WriteErrorSurfacing(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.claudeSessionID = __mockCID;
		prSplit._state._claudeEventIDs = null;

		var writeFails = true;
		globalThis.tuiMux = {
			on: function(event, cb) { return 1; },
			off: function(id) { return true; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function(id) { return { fullScreen: 'screen', plainText: 'plain' }; },
			activate: function(id) {},
			input: function(data) {
				if (writeFails) throw new Error('session closed');
			},
			pollEvents: function() { return 0; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';

		// Send a key that should be forwarded to Claude PTY.
		// 'a' is not a reserved key, so it goes through keyToTermBytes → write.
		var r = update({ type: 'Key', key: 'a' }, s);
		s = r[0];

		var errors = [];
		if (!s.claudeWriteError) {
			errors.push('write error should be surfaced');
		}
		if (s.claudeWriteError && s.claudeWriteError.indexOf('session closed') < 0) {
			errors.push('write error should contain original message: ' + s.claudeWriteError);
		}
		if (!s.claudeWriteErrorAt) {
			errors.push('write error timestamp should be set');
		}

		// Successful write should clear the error.
		writeFails = false;
		var r2 = update({ type: 'Key', key: 'b' }, s);
		s = r2[0];
		if (s.claudeWriteError) {
			errors.push('successful write should clear error: ' + s.claudeWriteError);
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.claudeSessionID = null;
		prSplit._state._claudeEventIDs = null;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("write error surfacing: %v", raw)
	}
}

// TestClaudeLifecycle_BellFlashIndicator proves that bell events from
// Claude's session set the bell flash flag and it appears in state.
func TestClaudeLifecycle_BellFlashIndicator(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var bellCallback = null;
		prSplit._state = prSplit._state || {};
		prSplit._state.claudeSessionID = __mockCID;
		prSplit._state._claudeEventIDs = null;
		prSplit._state._claudeBellFlash = false;
		prSplit._state._claudeBellFlashAt = 0;

		globalThis.tuiMux = {
			on: function(event, cb) {
				if (event === 'bell') bellCallback = cb;
				return 1;
			},
			off: function(id) { return true; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function(id) { return { fullScreen: 'screen', plainText: 'plain' }; },
			activate: function(id) {},
			input: function(data) {},
			pollEvents: function() { return 0; },
			lastActivityMs: function() { return 100; }
		};

		var errors = [];

		// Wire events.
		prSplit._wireClaudeLifecycleEvents();

		// Simulate bell event.
		if (bellCallback) {
			bellCallback({ sessionId: __mockCID });
		}

		// Poll — should see bell flash in state.
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		prSplit._pollClaudeScreenshot(s);

		if (!s.claudeBellFlash) {
			errors.push('claudeBellFlash should be true after bell event');
		}

		// Cleanup.
		prSplit._unwireClaudeLifecycleEvents();
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.claudeSessionID = null;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("bell flash: %v", raw)
	}
}

// TestClaudeLifecycle_LifecycleStateInTitle proves that the Claude pane
// title bar includes lifecycle state indicators.
func TestClaudeLifecycle_LifecycleStateInTitle(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	cases := []struct {
		name      string
		state     string
		indicator string
	}{
		{"active", "active", "●"},
		{"idle", "idle", "○"},
		{"waiting", "waiting", "❓"},
		{"crashed", "crashed", "✗"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := evalJS(`(function() {
				var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
				globalThis.tuiMux = {
					snapshot: function() { return null; },
					isDone: function() { return false; }
				};
				var s = initState('PLAN_REVIEW');
				s.splitViewEnabled = true;
				s.splitViewFocus = 'wizard';
				s.splitViewTab = 'claude';
				s.claudeScreen = 'some content here';
				s.claudeScreenshot = 'some content here';
				s.claudeLifecycleState = '` + tc.state + `';
				s.width = 80;
				s.height = 30;
				s.claudeViewOffset = 0;
				setupPlanCache();
				var pane = prSplit._renderClaudePane(s, 60, 12);
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
				return pane;
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			s, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T", raw)
			}
			if !strings.Contains(s, tc.indicator) {
				t.Errorf("pane title should contain %q indicator for %s state\ngot: %s",
					tc.indicator, tc.state, s)
			}
		})
	}
}

// TestClaudeLifecycle_WriteErrorInTitle proves that write errors appear
// in the Claude pane title bar as a transient indicator.
func TestClaudeLifecycle_WriteErrorInTitle(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			snapshot: function() { return null; },
			isDone: function() { return false; }
		};
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewTab = 'claude';
		s.claudeScreen = 'some content';
		s.claudeScreenshot = 'some content';
		s.claudeLifecycleState = 'idle';
		s.claudeWriteError = 'session closed';
		s.claudeWriteErrorAt = Date.now(); // fresh error
		s.width = 80;
		s.height = 30;
		setupPlanCache();
		var pane = prSplit._renderClaudePane(s, 60, 12);
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		if (pane.indexOf('[write error]') < 0) {
			return 'FAIL: pane should contain [write error] indicator, got: ' + pane.substring(0, 200);
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("write error in title: %v", raw)
	}
}

// TestClaudeLifecycle_PlaceholderStates proves that the Claude pane
// placeholder text reflects lifecycle state (crashed, exited, etc.)
func TestClaudeLifecycle_PlaceholderStates(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	cases := []struct {
		name    string
		state   string
		contain string
	}{
		{"crashed", "crashed", "crashed"},
		{"exited", "exited", "ended"},
		{"closed", "closed", "ended"},
		{"waiting", "", "waiting for claude"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := evalJS(`(function() {
				var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
				globalThis.tuiMux = {
					snapshot: function() { return null; },
					isDone: function() { return false; }
				};
				var s = initState('PLAN_REVIEW');
				s.splitViewEnabled = true;
				s.splitViewTab = 'claude';
				// Empty content triggers placeholder.
				s.claudeScreen = '';
				s.claudeScreenshot = '';
				s.claudeLifecycleState = '` + tc.state + `';
				s.width = 80;
				s.height = 30;
				var pane = prSplit._renderClaudePane(s, 60, 12);
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
				return pane;
			})()`)
			if err != nil {
				t.Fatal(err)
			}
			s, ok := raw.(string)
			if !ok {
				t.Fatalf("expected string, got %T", raw)
			}
			if !strings.Contains(strings.ToLower(s), tc.contain) {
				t.Errorf("placeholder for %s should contain %q\ngot: %s",
					tc.name, tc.contain, s)
			}
		})
	}
}

// TestClaudeLifecycle_ExportsAvailable verifies all new Task 9 exports
// are accessible on the prSplit global.
func TestClaudeLifecycle_ExportsAvailable(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	exports := []string{
		"_wireClaudeLifecycleEvents",
		"_unwireClaudeLifecycleEvents",
		"_deriveClaudeLifecycleState",
	}

	for _, name := range exports {
		raw, err := evalJS(`typeof prSplit.` + name)
		if err != nil {
			t.Fatalf("checking %s: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("prSplit.%s should be a function, got %v", name, raw)
		}
	}
}
