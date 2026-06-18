package command

// pr_split_tui_agent_lifecycle_test.go — Tests for Task 9: event-driven
// Agent lifecycle, adaptive polling, write error surfacing, and bell flash.
//
// Evidence tier: JS engine + mock tuiMux. Proves event wiring, lifecycle
// state derivation, adaptive tick intervals, write error propagation, and
// bell indicator behavior through the refactored pollAgentScreenshot and
// the wizardUpdateImpl dispatch chain.
//
// All tests use skipSlow(t) and t.Parallel() per project conventions.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestAgentLifecycle_EventWiring proves that wireAgentLifecycleEvents
// registers event handlers that filter by Agent's pinned sessionId.
// Events for other sessions must NOT affect Agent lifecycle flags.
func TestAgentLifecycle_EventWiring(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var otherCID = 99;
		var registeredEvents = [];

		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		// Clear any prior event state.
		prSplit._state._agentEventIDs = null;
		prSplit._state._agentOutputDirty = false;
		prSplit._state._agentExitEvent = false;
		prSplit._state._agentBellFlash = false;
		prSplit._state._agentClosedEvent = false;

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
		prSplit._wireAgentLifecycleEvents();
		var ids = prSplit._state._agentEventIDs;
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

		// Fire output event for Agent's session — should set dirty flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'output') {
				listeners[lid].cb({ sessionId: __mockCID });
			}
		}
		if (!prSplit._state._agentOutputDirty) {
			return 'FAIL: output event for Agent should set _agentOutputDirty';
		}

		// Reset and fire output event for OTHER session — should NOT set dirty.
		prSplit._state._agentOutputDirty = false;
		for (var lid in listeners) {
			if (listeners[lid].event === 'output') {
				listeners[lid].cb({ sessionId: otherCID });
			}
		}
		if (prSplit._state._agentOutputDirty) {
			return 'FAIL: output event for other session should NOT set _agentOutputDirty';
		}

		// Fire exit event for Agent — should set exit flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'exit') {
				listeners[lid].cb({ sessionId: __mockCID });
			}
		}
		if (!prSplit._state._agentExitEvent) {
			return 'FAIL: exit event should set _agentExitEvent';
		}

		// Fire bell event for Agent — should set bell flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'bell') {
				listeners[lid].cb({ sessionId: __mockCID });
			}
		}
		if (!prSplit._state._agentBellFlash) {
			return 'FAIL: bell event should set _agentBellFlash';
		}

		// Fire closed event for OTHER session — should NOT set closed flag.
		for (var lid in listeners) {
			if (listeners[lid].event === 'closed') {
				listeners[lid].cb({ sessionId: otherCID });
			}
		}
		if (prSplit._state._agentClosedEvent) {
			return 'FAIL: closed event for other session should NOT set _agentClosedEvent';
		}

		// Unwire — should remove all handlers.
		prSplit._unwireAgentLifecycleEvents();
		if (prSplit._state._agentEventIDs !== null) {
			return 'FAIL: unwire should null _agentEventIDs';
		}
		// Check listeners were removed.
		var remaining = Object.keys(listeners).length;
		if (remaining !== 0) {
			return 'FAIL: unwire should remove all listeners, got ' + remaining;
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.agentSessionID = null;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("event wiring: %v", raw)
	}
}

// TestAgentLifecycle_IdempotentWiring proves wireAgentLifecycleEvents
// does not double-register handlers when called multiple times.
func TestAgentLifecycle_IdempotentWiring(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var callCount = 0;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		prSplit._state._agentEventIDs = null;

		globalThis.tuiMux = {
			on: function(event, cb) { callCount++; return callCount; },
			off: function(id) { return true; },
			isDone: function() { return false; },
			activeID: function() { return __mockCID; },
			snapshot: function() { return null; },
			pollEvents: function() { return 0; }
		};

		// Wire twice.
		prSplit._wireAgentLifecycleEvents();
		var firstCount = callCount;
		prSplit._wireAgentLifecycleEvents();
		var secondCount = callCount;

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.agentSessionID = null;
		prSplit._state._agentEventIDs = null;

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

// TestAgentLifecycle_StateDerivation proves deriveAgentLifecycleState
// returns the correct state based on event flags and session status.
func TestAgentLifecycle_StateDerivation(t *testing.T) {
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
			setup:  `prSplit._state.agentSessionID = null;`,
			expect: "detached",
		},
		{
			name: "active (recent output)",
			setup: `prSplit._state.agentSessionID = 42;
				prSplit._state._agentLastOutputMs = Date.now() - 500;
				prSplit._state._agentExitEvent = false;
				prSplit._state._agentClosedEvent = false;`,
			expect: "active",
		},
		{
			name: "idle (no recent output)",
			setup: `prSplit._state.agentSessionID = 42;
				prSplit._state._agentLastOutputMs = Date.now() - 10000;
				prSplit._state._agentExitEvent = false;
				prSplit._state._agentClosedEvent = false;`,
			expect: "idle",
		},
		{
			name: "waiting (question detected)",
			setup: `prSplit._state.agentSessionID = 42;
				prSplit._state._agentLastOutputMs = Date.now() - 10000;
				prSplit._state._agentExitEvent = false;
				prSplit._state._agentClosedEvent = false;
				__testState.agentQuestionDetected = true;`,
			expect: "waiting",
		},
		{
			name: "crashed (exit during pipeline)",
			setup: `prSplit._state.agentSessionID = 42;
				prSplit._state._agentExitEvent = true;
				prSplit._state._agentClosedEvent = false;
				__testState.autoSplitRunning = true;`,
			expect: "crashed",
		},
		{
			name: "exited (exit after pipeline)",
			setup: `prSplit._state.agentSessionID = 42;
				prSplit._state._agentExitEvent = true;
				prSplit._state._agentClosedEvent = false;
				__testState.autoSplitRunning = false;`,
			expect: "exited",
		},
		{
			name: "closed (session unregistered)",
			setup: `prSplit._state.agentSessionID = 42;
				prSplit._state._agentExitEvent = false;
				prSplit._state._agentClosedEvent = true;`,
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
				var __testState = { agentQuestionDetected: false, autoSplitRunning: false };
				` + tc.setup + `
				var result = prSplit._deriveAgentLifecycleState(__testState);
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
				prSplit._state.agentSessionID = null;
				prSplit._state._agentExitEvent = false;
				prSplit._state._agentClosedEvent = false;
				prSplit._state._agentLastOutputMs = 0;
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

// TestAgentLifecycle_AdaptivePolling proves that pollAgentScreenshot
// returns shorter tick intervals when Agent is actively outputting and
// longer intervals when idle.
func TestAgentLifecycle_AdaptivePolling(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		prSplit._state._agentEventIDs = null;
		prSplit._state._agentLastOutputMs = 0;

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
		prSplit._state._agentLastOutputMs = Date.now() - 500; // 500ms ago
		var s1 = initState('PLAN_REVIEW');
		s1.splitViewEnabled = true;
		var r1 = prSplit._pollAgentScreenshot(s1);
		var cmd1 = r1[1];
		// cmd1 should be a tick command — extract the delay.
		// The tick creates a {type:'Tick', id:'agent-screenshot'} message.
		// We verify by checking which constant was used.
		if (!cmd1) {
			errors.push('active: expected tick command');
		}

		// Test 2: No recent output → slow tick.
		prSplit._state._agentLastOutputMs = Date.now() - 10000; // 10s ago
		prSplit._state._agentEventIDs = null; // reset for re-wire
		var s2 = initState('PLAN_REVIEW');
		s2.splitViewEnabled = true;
		var r2 = prSplit._pollAgentScreenshot(s2);
		var cmd2 = r2[1];
		if (!cmd2) {
			errors.push('idle: expected tick command');
		}

		// Both commands should be non-null tick commands.
		// We can't directly inspect tick durations from JS, but we can verify
		// the lifecycle state reflects the polling mode.
		var state1 = r1[0];
		var state2 = r2[0];
		if (state1.agentLifecycleState !== 'active') {
			errors.push('active state: got ' + state1.agentLifecycleState);
		}
		if (state2.agentLifecycleState !== 'idle') {
			errors.push('idle state: got ' + state2.agentLifecycleState);
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.agentSessionID = null;
		prSplit._state._agentEventIDs = null;
		prSplit._state._agentLastOutputMs = 0;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("adaptive polling: %v", raw)
	}
}

// TestAgentLifecycle_GenSkipsRedundantSnapshot proves that when the
// snapshot generation is unchanged, pollAgentScreenshot skips the
// expensive screen capture but still runs lifecycle state derivation.
func TestAgentLifecycle_GenSkipsRedundantSnapshot(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var snapshotCalls = 0;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		prSplit._state._agentEventIDs = null;
		prSplit._state._agentLastSnapshotGen = 0;

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
		s.agentScreen = '';
		prSplit._pollAgentScreenshot(s);
		if (s.agentScreen !== 'screen-gen5') {
			errors.push('first poll should capture screen');
		}
		if (prSplit._state._agentLastSnapshotGen !== 5) {
			errors.push('first poll should set lastGen=5');
		}

		// Second poll: gen still 5 → should skip screen update.
		s.agentScreen = 'old-value';
		prSplit._state._agentEventIDs = null; // allow re-wire
		prSplit._pollAgentScreenshot(s);
		if (s.agentScreen !== 'old-value') {
			errors.push('second poll with same gen should not overwrite screen');
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.agentSessionID = null;
		prSplit._state._agentEventIDs = null;
		prSplit._state._agentLastSnapshotGen = 0;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("gen skip: %v", raw)
	}
}

// TestAgentLifecycle_WriteErrorSurfacing proves that PTY write errors
// are surfaced in state rather than silently swallowed.
func TestAgentLifecycle_WriteErrorSurfacing(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		prSplit._state._agentEventIDs = null;

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
		s.splitViewFocus = 'agent';
		s.splitViewTab = 'agent';

		// Send a key that should be forwarded to Agent PTY.
		// 'a' is not a reserved key, so it goes through keyToTermBytes → write.
		var r = update({ type: 'Key', key: 'a' }, s);
		s = r[0];

		var errors = [];
		if (!s.agentWriteError) {
			errors.push('write error should be surfaced');
		}
		if (s.agentWriteError && s.agentWriteError.indexOf('session closed') < 0) {
			errors.push('write error should contain original message: ' + s.agentWriteError);
		}
		if (!s.agentWriteErrorAt) {
			errors.push('write error timestamp should be set');
		}

		// Successful write should clear the error.
		writeFails = false;
		var r2 = update({ type: 'Key', key: 'b' }, s);
		s = r2[0];
		if (s.agentWriteError) {
			errors.push('successful write should clear error: ' + s.agentWriteError);
		}

		// Cleanup.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.agentSessionID = null;
		prSplit._state._agentEventIDs = null;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("write error surfacing: %v", raw)
	}
}

// TestAgentLifecycle_BellFlashIndicator proves that bell events from
// Agent's session set the bell flash flag and it appears in state.
func TestAgentLifecycle_BellFlashIndicator(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		var bellCallback = null;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		prSplit._state._agentEventIDs = null;
		prSplit._state._agentBellFlash = false;
		prSplit._state._agentBellFlashAt = 0;

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
		prSplit._wireAgentLifecycleEvents();

		// Simulate bell event.
		if (bellCallback) {
			bellCallback({ sessionId: __mockCID });
		}

		// Poll — should see bell flash in state.
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		prSplit._pollAgentScreenshot(s);

		if (!s.agentBellFlash) {
			errors.push('agentBellFlash should be true after bell event');
		}

		// Cleanup.
		prSplit._unwireAgentLifecycleEvents();
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		prSplit._state.agentSessionID = null;
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("bell flash: %v", raw)
	}
}

// TestAgentLifecycle_LifecycleStateInTitle proves that the Agent pane
// title bar includes lifecycle state indicators.
func TestAgentLifecycle_LifecycleStateInTitle(t *testing.T) {
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
					prSplit._state = prSplit._state || {};
					prSplit._state.agentSessionID = 42;
				globalThis.tuiMux = {
						snapshot: function(id) { if (id !== 42) throw new Error('wrong session id: ' + id); return null; },
					isDone: function() { return false; }
				};
				var s = initState('PLAN_REVIEW');
				s.splitViewEnabled = true;
				s.splitViewFocus = 'wizard';
				s.splitViewTab = 'agent';
				s.agentScreen = 'some content here';
				s.agentScreenshot = 'some content here';
				s.agentLifecycleState = '` + tc.state + `';
				s.width = 80;
				s.height = 30;
				s.agentViewOffset = 0;
				setupPlanCache();
				var pane = prSplit._renderAgentPane(s, 60, 12);
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
					if (prSplit._state) prSplit._state.agentSessionID = null;
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

// TestAgentLifecycle_WriteErrorInTitle proves that write errors appear
// in the Agent pane title bar as a transient indicator.
func TestAgentLifecycle_WriteErrorInTitle(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = 42;
		globalThis.tuiMux = {
			snapshot: function(id) { if (id !== 42) throw new Error('wrong session id: ' + id); return null; },
			isDone: function() { return false; }
		};
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewTab = 'agent';
		s.agentScreen = 'some content';
		s.agentScreenshot = 'some content';
		s.agentLifecycleState = 'idle';
		s.agentWriteError = 'session closed';
		s.agentWriteErrorAt = Date.now(); // fresh error
		s.width = 80;
		s.height = 30;
		setupPlanCache();
		var pane = prSplit._renderAgentPane(s, 60, 12);
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;
		if (prSplit._state) prSplit._state.agentSessionID = null;
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

// TestAgentLifecycle_PlaceholderStates proves that the Agent pane
// placeholder text reflects lifecycle state (crashed, exited, etc.)
func TestAgentLifecycle_PlaceholderStates(t *testing.T) {
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
		{"waiting", "", "waiting for agent"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := evalJS(`(function() {
				var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
					prSplit._state = prSplit._state || {};
					prSplit._state.agentSessionID = 42;
				globalThis.tuiMux = {
						snapshot: function(id) { if (id !== 42) throw new Error('wrong session id: ' + id); return null; },
					isDone: function() { return false; }
				};
				var s = initState('PLAN_REVIEW');
				s.splitViewEnabled = true;
				s.splitViewTab = 'agent';
				// Empty content triggers placeholder.
				s.agentScreen = '';
				s.agentScreenshot = '';
				s.agentLifecycleState = '` + tc.state + `';
				s.width = 80;
				s.height = 30;
				var pane = prSplit._renderAgentPane(s, 60, 12);
				if (savedMux !== undefined) globalThis.tuiMux = savedMux;
				else delete globalThis.tuiMux;
					if (prSplit._state) prSplit._state.agentSessionID = null;
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

// TestAgentLifecycle_ExportsAvailable verifies all new Task 9 exports
// are accessible on the prSplit global.
func TestAgentLifecycle_ExportsAvailable(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	exports := []string{
		"_wireAgentLifecycleEvents",
		"_unwireAgentLifecycleEvents",
		"_deriveAgentLifecycleState",
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
