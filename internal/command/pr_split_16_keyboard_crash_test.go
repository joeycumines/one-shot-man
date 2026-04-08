package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T24: Keyboard routing audit — help overlay content & context-awareness
// ---------------------------------------------------------------------------

func TestChunk16_HelpOverlay_ContainsAllSections(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// T065: Help overlay is now context-aware. Test each section
		// appears in the appropriate wizard state.

		// PLAN_EDITOR state: should show Plan Editor + global sections.
		var s = initState('PLAN_EDITOR');
		var view = globalThis.prSplit._viewHelpOverlay(s);

		// Global sections always present.
		if (view.indexOf('Navigation') < 0) errors.push('missing Navigation section');
		if (view.indexOf('Scrolling') < 0) errors.push('missing Scrolling section');
		if (view.indexOf('Claude') < 0) errors.push('missing Claude Integration section');

		// Plan Editor section only in PLAN_EDITOR/PLAN_REVIEW.
		if (view.indexOf('Plan Editor') < 0) errors.push('missing Plan Editor section in PLAN_EDITOR');

		// Specific key bindings.
		var keys = [
			['F1', 'help toggle'],
			['Tab', 'tab key'],
			['Shift+Tab', 'shift-tab'],
			['Enter', 'enter key'],
			['Esc', 'escape key'],
			['Ctrl+C', 'cancel'],
			['Ctrl+L', 'split view toggle'],
			['Ctrl+]', 'claude pane'],
			['Ctrl+=', 'resize split'],
			['Ctrl+-', 'resize split minus'],
			['PgUp', 'page up'],
			['PgDn', 'page down'],
			['Home', 'home key'],
			['End', 'end key'],
			['Space', 'checkbox toggle'],
			['Shift+', 'reorder files']
		];
		for (var i = 0; i < keys.length; i++) {
			if (view.indexOf(keys[i][0]) < 0) {
				errors.push('missing key ' + keys[i][1] + ' (' + keys[i][0] + ')');
			}
		}

		// CONFIG state should NOT show Plan Editor section.
		var s2 = initState('CONFIG');
		var v2 = globalThis.prSplit._viewHelpOverlay(s2);
		if (v2.indexOf('Plan Editor') >= 0) errors.push('CONFIG should not show Plan Editor section');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("help overlay content: %v", raw)
	}
}

func TestChunk16_JKContextAwareness_SplitNavigation(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// PLAN_REVIEW: j/k should navigate splits.
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.selectedSplitIdx = 0;
		var r = sendKey(s, 'j');
		if (r[0].selectedSplitIdx !== 1) errors.push('PLAN_REVIEW j did not move split down (got ' + r[0].selectedSplitIdx + ')');
		r = sendKey(r[0], 'k');
		if (r[0].selectedSplitIdx !== 0) errors.push('PLAN_REVIEW k did not move split up (got ' + r[0].selectedSplitIdx + ')');

		// PLAN_EDITOR: j/k should navigate files within split.
		setupPlanCache();
		s = initState('PLAN_EDITOR');
		s.selectedSplitIdx = 0; // api split has 2 files.
		s.selectedFileIdx = 0;
		r = sendKey(s, 'j');
		if (r[0].selectedFileIdx !== 1) errors.push('PLAN_EDITOR j did not move file down (got ' + r[0].selectedFileIdx + ')');
		r = sendKey(r[0], 'k');
		if (r[0].selectedFileIdx !== 0) errors.push('PLAN_EDITOR k did not move file up (got ' + r[0].selectedFileIdx + ')');

		// CONFIG: j/k should not change selectedSplitIdx (no list to navigate).
		s = initState('CONFIG');
		s.selectedSplitIdx = 0;
		r = sendKey(s, 'j');
		if (r[0].selectedSplitIdx !== 0) errors.push('CONFIG j should not change splitIdx');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("j/k context awareness: %v", raw)
	}
}

func TestChunk16_ArrowContextAwareness_VerifySession(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Verify session active: up/down should scroll output, not navigate lists.
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.activeVerifySession = {
			interrupt: function() {},
			kill: function() {}
		};
		s.verifyViewportOffset = 0;
		s.verifyAutoScroll = true;

		var r = sendKey(s, 'up');
		if ((r[0].verifyViewportOffset || 0) <= 0) errors.push('up in verify session did not scroll (got ' + r[0].verifyViewportOffset + ')');
		if (r[0].verifyAutoScroll !== false) errors.push('up in verify session did not disable auto-scroll');

		r = sendKey(r[0], 'end');
		if (r[0].verifyViewportOffset !== 0) errors.push('end in verify session did not jump to bottom');
		if (r[0].verifyAutoScroll !== true) errors.push('end in verify session did not enable auto-scroll');

		r = sendKey(r[0], 'home');
		if (r[0].verifyViewportOffset < 999) errors.push('home in verify session did not jump to top');
		if (r[0].verifyAutoScroll !== false) errors.push('home in verify session did not disable auto-scroll');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("arrow context in verify session: %v", raw)
	}
}

func TestChunk16_TabBehaviorInSplitView(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Normal mode: Tab cycles focus.
		var s = initState('CONFIG');
		s.splitViewEnabled = false;
		s.focusIndex = 0;
		var r = sendKey(s, 'tab');
		if (r[0].focusIndex === 0) errors.push('normal tab did not cycle focus');

		// Split-view mode: Ctrl+Tab cycles pane targets (T61).
		s = initState('CONFIG');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		r = sendKey(s, 'ctrl+tab');
		if (r[0].splitViewFocus !== 'claude') errors.push('split-view ctrl+tab did not switch to claude');
		r = sendKey(r[0], 'ctrl+tab');
		if (r[0].splitViewTab !== 'output') errors.push('split-view ctrl+tab did not advance to output');
		r = sendKey(r[0], 'ctrl+tab');
		if (r[0].splitViewFocus !== 'wizard') errors.push('split-view ctrl+tab did not wrap back to wizard');

		// Split-view + verify session: T380 removed the guard, Ctrl+Tab now switches panes.
		s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.activeVerifySession = {interrupt: function(){}, kill: function(){}};
		r = sendKey(s, 'ctrl+tab');
		// T380: Ctrl+Tab works during verify — switches to pane.
		if (r[0].splitViewFocus !== 'claude') errors.push('split-view+verify ctrl+tab should switch to claude');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("tab behavior in split view: %v", raw)
	}
}

func TestChunk16_PlanReviewE_EntersEditor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// 'e' in PLAN_REVIEW (not processing) should enter editor.
		setupPlanCache();
		var s = initState('PLAN_REVIEW');
		s.isProcessing = false;
		var r = sendKey(s, 'e');
		if (r[0].wizardState !== 'PLAN_EDITOR') errors.push('e did not enter editor (got ' + r[0].wizardState + ')');

		// 'e' when processing should NOT enter editor.
		setupPlanCache();
		s = initState('PLAN_REVIEW');
		s.isProcessing = true;
		r = sendKey(s, 'e');
		if (r[0].wizardState === 'PLAN_EDITOR') errors.push('e during processing should not enter editor');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan review e key: %v", raw)
	}
}

func TestChunk16_Keyboard_AllBindingsConsistency(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// Comprehensive test: verify EVERY key binding documented in the help overlay
	// actually does something (returns a changed state or a command).
	raw, err := evalJS(`(function() {
		var errors = [];

		function stateSnapshot(s) {
			return JSON.stringify({
				showHelp: s.showHelp,
				showConfirmCancel: s.showConfirmCancel,
				splitViewEnabled: s.splitViewEnabled,
				wizardState: s.wizardState,
				selectedSplitIdx: s.selectedSplitIdx,
				selectedFileIdx: s.selectedFileIdx,
				focusIndex: s.focusIndex,
				editorTitleEditing: s.editorTitleEditing,
				editorCheckedFiles: s.editorCheckedFiles || {},
				vpYOffset: s.vp ? s.vp.yOffset() : 0
			});
		}

		// Injects tall content into the viewport so PgDn/Up/Home/End are observable.
		// Pre-scrolls to mid-page so pgup and home have effect from non-top.
		function fillViewport(s) {
			if (s.vp) {
				var lines = [];
				for (var i = 0; i < 200; i++) lines.push('line ' + i);
				s.vp.setContent(lines.join('\n'));
				s.vp.setHeight(10);
				s.vp.halfPageDown();
				s.vp.halfPageDown();
			}
		}

		function testKeyChanges(key, state, label, opts) {
			setupPlanCache();
			var s = initState(state, opts);
			fillViewport(s);
			var before = stateSnapshot(s);
			var r = update({type: 'Key', key: key}, s);
			var after = stateSnapshot(r[0]);
			if (before === after && r[1] === null) {
				errors.push(label + ': no change on key=' + key);
			}
		}

		// Global navigation.
		testKeyChanges('?', 'CONFIG', 'help-?');
		testKeyChanges('f1', 'CONFIG', 'help-f1');
		testKeyChanges('ctrl+c', 'CONFIG', 'cancel');
		testKeyChanges('ctrl+l', 'CONFIG', 'split-view');
		testKeyChanges('enter', 'CONFIG', 'enter-config');
		testKeyChanges('pgdown', 'CONFIG', 'pgdown');
		testKeyChanges('pgup', 'CONFIG', 'pgup');
		testKeyChanges('home', 'CONFIG', 'home');
		testKeyChanges('end', 'CONFIG', 'end');
		testKeyChanges('tab', 'CONFIG', 'tab');
		testKeyChanges('shift+tab', 'CONFIG', 'shift-tab', {focusIndex: 1});

		// Plan review navigation.
		testKeyChanges('j', 'PLAN_REVIEW', 'j-review', {selectedSplitIdx: 0});
		testKeyChanges('k', 'PLAN_REVIEW', 'k-review', {selectedSplitIdx: 1});
		testKeyChanges('down', 'PLAN_REVIEW', 'down-review', {selectedSplitIdx: 0});
		testKeyChanges('up', 'PLAN_REVIEW', 'up-review', {selectedSplitIdx: 1});
		testKeyChanges('e', 'PLAN_REVIEW', 'e-review');

		// Plan editor specific.
		testKeyChanges('e', 'PLAN_EDITOR', 'e-editor', {selectedSplitIdx: 0});
		testKeyChanges(' ', 'PLAN_EDITOR', 'space-editor', {selectedSplitIdx: 0, selectedFileIdx: 0});
		testKeyChanges('j', 'PLAN_EDITOR', 'j-editor', {selectedSplitIdx: 0, selectedFileIdx: 0});
		testKeyChanges('k', 'PLAN_EDITOR', 'k-editor', {selectedSplitIdx: 0, selectedFileIdx: 1});
		testKeyChanges('shift+down', 'PLAN_EDITOR', 'shift-down', {selectedSplitIdx: 0, selectedFileIdx: 0});
		testKeyChanges('shift+up', 'PLAN_EDITOR', 'shift-up', {selectedSplitIdx: 0, selectedFileIdx: 1});

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("keyboard consistency check: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T025: Claude Crash Detection and Recovery
// ---------------------------------------------------------------------------

// TestChunk16_CrashDetection_AutoPoll verifies that the auto-poll tick handler
// detects a dead Claude process and transitions to ERROR_RESOLUTION with the
// claudeCrashDetected flag set.
func TestChunk16_CrashDetection_AutoPoll(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock ClaudeCodeExecutor with a dead handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return false; },
				receive: function() { return 'segfault at 0x0'; }
			},
			captureDiagnostic: function() { return 'last output: segfault at 0x0'; }
		};

		// Set up state as BRANCH_BUILDING with autoSplitRunning.
		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0; // Force immediate health check.

		// Send auto-poll tick.
		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		var errors = [];
		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			errors.push('wizardState: got ' + newState.wizardState + ', want ERROR_RESOLUTION');
		}
		if (!newState.claudeCrashDetected) {
			errors.push('claudeCrashDetected should be true');
		}
		if (newState.autoSplitRunning) {
			errors.push('autoSplitRunning should be false');
		}
		if (newState.isProcessing) {
			errors.push('isProcessing should be false');
		}
		if (!newState.errorDetails || newState.errorDetails.indexOf('crashed') < 0) {
			errors.push('errorDetails should mention crash, got: ' + newState.errorDetails);
		}
		if (newState.errorDetails.indexOf('segfault') < 0) {
			errors.push('errorDetails should contain diagnostic, got: ' + newState.errorDetails);
		}
		// Shared state flag should NOT be set (crash is view-state only).
		if (globalThis.prSplit._state.claudeCrashDetected) {
			errors.push('shared state claudeCrashDetected should NOT be true');
		}
		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash detection auto-poll: %v", raw)
	}
}

// TestChunk16_CrashDetection_SessionModel verifies that the session model's
// isDone() drives crash detection when tuiMux is present. Also verifies that
// isDone()=true from a never-attached session (no executor) does NOT trigger
// false crash detection (pre-closed sentinel channel guard).
func TestChunk16_CrashDetection_SessionModel(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// --- Subtest 1: isDone()=true WITH executor → crash detected ---
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			captureDiagnostic: function() { return 'session-model crash'; }
		};
		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];
		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			errors.push('subtest1: want ERROR_RESOLUTION, got ' + newState.wizardState);
		}
		if (!newState.claudeCrashDetected) {
			errors.push('subtest1: claudeCrashDetected should be true');
		}
		if (newState.errorDetails.indexOf('session-model crash') < 0) {
			errors.push('subtest1: errorDetails should contain diagnostic');
		}

		// --- Subtest 2: isDone()=true WITHOUT executor → no false positive ---
		globalThis.prSplit._state.claudeExecutor = null;
		var s2 = initState('BRANCH_BUILDING');
		s2.autoSplitRunning = true;
		s2.isProcessing = true;
		s2.lastClaudeHealthCheckMs = 0;

		var r2 = update({type: 'Tick', id: 'auto-poll'}, s2);
		var newState2 = r2[0];
		if (newState2.wizardState === 'ERROR_RESOLUTION') {
			errors.push('subtest2: should NOT transition to ERROR_RESOLUTION without executor');
		}
		if (newState2.claudeCrashDetected) {
			errors.push('subtest2: claudeCrashDetected should be false (no executor)');
		}

		// Cleanup.
		if (savedMux !== undefined) { globalThis.tuiMux = savedMux; } else { delete globalThis.tuiMux; }

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("session model crash detection: %v", raw)
	}
}

// TestChunk16_CrashDetection_AliveSkipsCheck verifies that a healthy Claude
// process does NOT trigger crash detection.
func TestChunk16_CrashDetection_AliveSkipsCheck(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock ClaudeCodeExecutor with a LIVE handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return true; }
			}
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		// Should still be running — no crash detected.
		if (newState.wizardState !== 'BRANCH_BUILDING') {
			return 'FAIL: wizardState changed to ' + newState.wizardState;
		}
		if (newState.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be false';
		}
		if (!newState.autoSplitRunning) {
			return 'FAIL: autoSplitRunning should still be true';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash detection alive: %v", raw)
	}
}

// TestChunk16_CrashDetection_HealthPollThrottled verifies that the health
// check is throttled (only fires every claudeHealthPollMs).
func TestChunk16_CrashDetection_HealthPollThrottled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var checkCount = 0;
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { checkCount++; return true; }
			}
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		// Set last check to NOW — should skip the immediate check.
		s.lastClaudeHealthCheckMs = Date.now();

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		// isAlive should NOT have been called (throttled).
		if (checkCount !== 0) {
			return 'FAIL: health check should be throttled, but isAlive called ' + checkCount + ' times';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash detection throttle: %v", raw)
	}
}

// TestChunk16_CrashRecovery_ClickZones verifies that crash-specific zone
// clicks dispatch to the correct recovery handlers.
func TestChunk16_CrashRecovery_ClickZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Mock a minimal ClaudeCodeExecutor for restart.
		// restart() returns a Promise (async, matching the real implementation).
		var restartCalled = false;
		var mockExecutor = {
			handle: { isAlive: function() { return true; } },
			resolve: function() { return { error: null }; },
			spawn: function() { return Promise.resolve({ error: null, sessionId: 'restarted-session' }); },
			close: function() {},
			restart: function() {
				restartCalled = true;
				return Promise.resolve({ error: null, sessionId: 'restarted-session' });
			}
		};
		globalThis.prSplit._state.claudeExecutor = mockExecutor;

		// Mock handleConfigState and heuristicSplit to prevent real work.
		var origConfigState = globalThis.prSplit._handleConfigState;
		globalThis.prSplit._handleConfigState = function() {
			return { error: null, analysis: { baseBranch: 'main', files: {} } };
		};

		// Test 1: restart-claude zone click.
		// After click, restart is async — the model enters a "restarting" state.
		// claudeCrashDetected is NOT immediately cleared; it's handled by the
		// restart-claude-poll tick handler after the Promise resolves.
		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		var restore = mockZoneHit('resolve-restart-claude');
		try {
			restartCalled = false;
			var r = sendClick(s);
			if (!restartCalled) {
				errors.push('restart: executor.restart() not called');
			}
			// Async restart: model should be in restarting state.
			if (!r[0].claudeRestarting) {
				errors.push('restart: claudeRestarting should be true while async restart is in progress');
			}
			// claudeCrashDetected is still true — cleared by poll handler later.
			if (!r[0].claudeCrashDetected) {
				errors.push('restart: claudeCrashDetected should still be true during async restart');
			}
			if (r[0].errorDetails !== 'Restarting Claude...') {
				errors.push('restart: errorDetails should show restarting message, got: ' + r[0].errorDetails);
			}
			// Should have returned a tick command for polling.
			if (!r[1]) {
				errors.push('restart: expected a tick command (non-null) for restart polling');
			}
		} finally { restore(); }

		// Test 2: fallback-heuristic zone click.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		restore = mockZoneHit('resolve-fallback-heuristic');
		try {
			var r2 = sendClick(s);
			if (r2[0].claudeCrashDetected) {
				errors.push('fallback: claudeCrashDetected should be cleared');
			}
			if (globalThis.prSplit.runtime.mode !== 'heuristic') {
				errors.push('fallback: mode should be heuristic, got ' + globalThis.prSplit.runtime.mode);
			}
		} finally { restore(); }

		// Test 3: abort zone click during crash.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		restore = mockZoneHit('resolve-abort');
		try {
			var r3 = sendClick(s);
			// Abort bypasses crash recovery — goes to handleErrorResolutionState('abort').
			// Should result in CANCELLED state.
			if (r3[0].wizardState !== 'CANCELLED') {
				errors.push('abort: wizardState should be CANCELLED, got ' + r3[0].wizardState);
			}
		} finally { restore(); }

		// Restore.
		globalThis.prSplit._handleConfigState = origConfigState;

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash recovery click zones: %v", raw)
	}
}

// TestChunk16_GetFocusElements_CrashMode verifies that getFocusElements
// returns crash-specific buttons when claudeCrashDetected is set.
func TestChunk16_GetFocusElements_CrashMode(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Standard ERROR_RESOLUTION (no crash) — should have 5+ normal buttons.
		globalThis.prSplit._state.claudeExecutor = {};
		var s = initState('ERROR_RESOLUTION');
		var elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length < 5) {
			errors.push('normal: expected >= 5 elements, got ' + elems.length);
		}
		// Should include resolve-auto, resolve-manual, etc.
		var hasAuto = false;
		for (var i = 0; i < elems.length; i++) {
			if (elems[i].id === 'resolve-auto') hasAuto = true;
		}
		if (!hasAuto) errors.push('normal: missing resolve-auto button');

		// Crash mode — should have exactly 5 elements (3 crash buttons + nav-next + nav-cancel).
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		elems = globalThis.prSplit._getFocusElements(s);
		if (elems.length !== 5) {
			errors.push('crash: expected 5 elements, got ' + elems.length);
		}
		var crashIds = {};
		for (var j = 0; j < elems.length; j++) {
			crashIds[elems[j].id] = true;
		}
		if (!crashIds['resolve-restart-claude']) errors.push('crash: missing restart button');
		if (!crashIds['resolve-fallback-heuristic']) errors.push('crash: missing fallback button');
		if (!crashIds['resolve-abort']) errors.push('crash: missing abort button');
		// Should NOT have normal buttons.
		if (crashIds['resolve-auto']) errors.push('crash: should not have resolve-auto');
		if (crashIds['error-ask-claude']) errors.push('crash: should not have ask-claude');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("getFocusElements crash mode: %v", raw)
	}
}

// TestChunk16_FocusActivate_CrashButtons verifies keyboard Enter activation
// dispatches crash-specific buttons correctly via handleFocusActivate.
func TestChunk16_FocusActivate_CrashButtons(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Mock executor for restart (returns Promise, matching async impl).
		var restartCalled = false;
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return true; } },
			restart: function() {
				restartCalled = true;
				return Promise.resolve({ error: null, sessionId: 'restarted' });
			}
		};

		// Mock handleConfigState to prevent real work.
		var origConfigState = globalThis.prSplit._handleConfigState;
		globalThis.prSplit._handleConfigState = function() {
			return { error: null, analysis: { baseBranch: 'main', files: {} } };
		};

		// Test: Enter with focusIndex=0 → restart-claude.
		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		s.focusIndex = 0; // First button = resolve-restart-claude.
		restartCalled = false;
		var r = sendKey(s, 'enter');
		if (!restartCalled) {
			errors.push('enter-restart: executor.restart() not called');
		}

		// Test: Enter with focusIndex=1 → fallback-heuristic.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		s.focusIndex = 1; // Second button = resolve-fallback-heuristic.
		globalThis.prSplit.runtime.mode = 'auto'; // Reset to non-heuristic.
		r = sendKey(s, 'enter');
		if (globalThis.prSplit.runtime.mode !== 'heuristic') {
			errors.push('enter-fallback: mode should be heuristic, got ' + globalThis.prSplit.runtime.mode);
		}

		// Test: Enter with focusIndex=2 → abort.
		s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;
		s.focusIndex = 2; // Third button = resolve-abort.
		r = sendKey(s, 'enter');
		// Abort goes through standard path → CANCELLED.
		if (r[0].wizardState !== 'CANCELLED') {
			errors.push('enter-abort: wizardState should be CANCELLED, got ' + r[0].wizardState);
		}

		// Restore.
		globalThis.prSplit._handleConfigState = origConfigState;

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus activate crash buttons: %v", raw)
	}
}

// TestChunk16_CrashDetection_InitState verifies that createWizardModel
// initializes crash-related fields to their default values.
func TestChunk16_CrashDetection_InitState(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		var errors = [];
		if (s.claudeCrashDetected !== false) {
			errors.push('claudeCrashDetected should be false, got ' + s.claudeCrashDetected);
		}
		if (s.lastClaudeHealthCheckMs !== 0) {
			errors.push('lastClaudeHealthCheckMs should be 0, got ' + s.lastClaudeHealthCheckMs);
		}
		if (s.autoSplitRunning !== false) {
			errors.push('autoSplitRunning should be false, got ' + s.autoSplitRunning);
		}
		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash init state: %v", raw)
	}
}

// TestChunk16_CrashDetection_PlanGenerationTransition verifies crash detection
// works during PLAN_GENERATION state (not just BRANCH_BUILDING).
func TestChunk16_CrashDetection_PlanGenerationTransition(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock dead Claude handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			captureDiagnostic: function() { return ''; }
		};

		// Set wizard to PLAN_GENERATION by manually transitioning.
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.width = 80;
		s.height = 24;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.wizardState = 'PLAN_GENERATION';
		s._prevWizardState = 'PLAN_GENERATION';
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should be ERROR_RESOLUTION, got ' + newState.wizardState;
		}
		if (!newState.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be true';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash during plan generation: %v", raw)
	}
}

// TestChunk16_CrashDetection_ConfigStateTransition verifies crash detection
// works during CONFIG state, which is the actual wizard state during auto-split
// pipeline execution (the wizard stays at CONFIG while the async pipeline runs).
func TestChunk16_CrashDetection_ConfigStateTransition(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock dead Claude handle.
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			captureDiagnostic: function() { return 'segfault at 0x0'; }
		};

		// Set wizard to CONFIG (auto-split scenario: wizard stays at CONFIG).
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.width = 80;
		s.height = 24;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s._prevWizardState = 'CONFIG';
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.lastClaudeHealthCheckMs = 0;

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var newState = r[0];

		if (newState.wizardState !== 'ERROR_RESOLUTION') {
			return 'FAIL: wizardState should be ERROR_RESOLUTION, got ' + newState.wizardState;
		}
		if (!newState.claudeCrashDetected) {
			return 'FAIL: claudeCrashDetected should be true';
		}
		if (!newState.errorDetails || newState.errorDetails.indexOf('segfault') < 0) {
			return 'FAIL: errorDetails should contain diagnostic, got: ' + newState.errorDetails;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash during config (auto-split): %v", raw)
	}
}

// TestChunk16_CrashRecovery_RestartFailure verifies that a failed restart
// attempt shows an error in the UI.
func TestChunk16_CrashRecovery_RestartFailure(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock executor that fails on restart (returns Promise with error).
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return false; } },
			restart: function() {
				return Promise.resolve({ error: 'Claude binary not found' });
			}
		};

		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;

		// Click restart button.
		// The restart is now async: the model enters a "restarting" state.
		// The actual error is surfaced by the restart-claude-poll tick handler.
		var restore = mockZoneHit('resolve-restart-claude');
		try {
			var r = sendClick(s);
			// Model should be in restarting state immediately.
			if (!r[0].claudeRestarting) {
				return 'FAIL: claudeRestarting should be true during async restart, got false';
			}
			if (r[0].errorDetails !== 'Restarting Claude...') {
				return 'FAIL: errorDetails should be "Restarting Claude..." during async restart, got: ' + r[0].errorDetails;
			}
			// Crash flag must remain true (not cleared until poll handler runs).
			if (!r[0].claudeCrashDetected) {
				return 'FAIL: claudeCrashDetected should remain true during async restart';
			}
			// Should have a tick command for polling.
			if (!r[1]) {
				return 'FAIL: expected a tick command for restart polling';
			}
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash recovery restart failure: %v", raw)
	}
}

// TestChunk16_CrashRecovery_NoExecutor verifies that restart-claude without
// an executor shows an appropriate error.
func TestChunk16_CrashRecovery_NoExecutor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// No executor set.
		globalThis.prSplit._state.claudeExecutor = null;

		var s = initState('ERROR_RESOLUTION');
		s.claudeCrashDetected = true;

		var restore = mockZoneHit('resolve-restart-claude');
		try {
			var r = sendClick(s);
			if (!r[0].errorDetails || r[0].errorDetails.indexOf('No Claude executor') < 0) {
				return 'FAIL: missing no-executor error, got: ' + r[0].errorDetails;
			}
		} finally { restore(); }
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash recovery no executor: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T33: tuiMux bootstrap — hasChild guards and pollClaudeScreenshot
// ---------------------------------------------------------------------------

// TestChunk16_PollClaudeScreenshot_NoMux verifies that pollClaudeScreenshot
// clears screen state and continues polling when tuiMux is undefined.
func TestChunk16_PollClaudeScreenshot_NoMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Remove tuiMux to simulate headless/test context.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = undefined;

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = 'stale-ansi-data';
		s.claudeScreenshot = 'stale-plain-data';

		// Send claude-screenshot tick.
		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var state = result[0];
		var cmd = result[1];

		// Restore tuiMux.
		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		// Verify state was cleared and polling continues.
		var cleared = (state.claudeScreen === '' && state.claudeScreenshot === '');
		var polls = (cmd !== null); // Should return a tick command to continue polling.

		if (!cleared) return 'FAIL: screen not cleared, claudeScreen=' + JSON.stringify(state.claudeScreen) +
			', claudeScreenshot=' + JSON.stringify(state.claudeScreenshot);
		if (!polls) return 'FAIL: expected polling to continue (non-null cmd)';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot no mux: %v", raw)
	}
}

// TestChunk16_PollClaudeScreenshot_NoChild verifies that pollClaudeScreenshot
// clears screen state when tuiMux exists but hasChild() returns false.
func TestChunk16_PollClaudeScreenshot_NoChild(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock tuiMux with hasChild() returning false.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			screenshot: function() { return 'should-not-be-called'; },
			childScreen: function() { return 'should-not-be-called'; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = 'stale-ansi';
		s.claudeScreenshot = 'stale-plain';

		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var state = result[0];
		var cmd = result[1];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var cleared = (state.claudeScreen === '' && state.claudeScreenshot === '');
		var polls = (cmd !== null);

		if (!cleared) return 'FAIL: screen not cleared';
		if (!polls) return 'FAIL: expected polling to continue';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot no child: %v", raw)
	}
}

// TestChunk16_PollClaudeScreenshot_WithChild verifies that pollClaudeScreenshot
// captures screen data when tuiMux has an attached child.
func TestChunk16_PollClaudeScreenshot_WithChild(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return 'ansi-content-here'; },
			screenshot: function() { return 'plain-content-here'; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeScreen = '';
		s.claudeScreenshot = '';

		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var state = result[0];
		var cmd = result[1];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (state.claudeScreen !== 'ansi-content-here')
			return 'FAIL: claudeScreen=' + JSON.stringify(state.claudeScreen);
		if (state.claudeScreenshot !== 'plain-content-here')
			return 'FAIL: claudeScreenshot=' + JSON.stringify(state.claudeScreenshot);
		if (!cmd) return 'FAIL: expected polling to continue';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot with child: %v", raw)
	}
}

// TestChunk16_PollClaudeScreenshot_SplitViewDisabled verifies that
// pollClaudeScreenshot stops polling when split view is disabled.
func TestChunk16_PollClaudeScreenshot_SplitViewDisabled(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;

		var result = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var cmd = result[1];

		if (cmd !== null) return 'FAIL: expected null cmd when split view disabled, got: ' + JSON.stringify(cmd);
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pollClaudeScreenshot split view disabled: %v", raw)
	}
}

// TestChunk16_SwitchTo_NoChild verifies _onToggle does NOT call switchTo
// when tuiMux.hasChild() returns false. T394 moved Ctrl+] handling from
// JS update to Go toggleModel — this test exercises the callback directly.
func TestChunk16_SwitchTo_NoChild(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var switchCalled = false;
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			switchTo: function() { switchCalled = true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};

		// T394: Call _onToggle directly.
		var result = globalThis.prSplit._onToggle();

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (switchCalled) return 'FAIL: switchTo called despite no child';
		if (!result.skipped) return 'FAIL: should be skipped when no child';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("switchTo no child: %v", raw)
	}
}

// TestChunk16_SwitchTo_WithChild verifies the _onToggle callback calls
// switchTo when tuiMux.hasChild() returns true. T394 moved Ctrl+] handling
// from JS update to Go toggleModel — this test exercises the callback directly.
func TestChunk16_SwitchTo_WithChild(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var switchCalled = false;
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			switchTo: function() { switchCalled = true; return {reason: 'toggle'}; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};

		// T394: Call _onToggle directly (Ctrl+] is intercepted by Go toggleModel).
		var result = globalThis.prSplit._onToggle();

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (!switchCalled) return 'FAIL: switchTo not called despite child attached';
		if (result.skipped) return 'FAIL: should not be skipped';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("switchTo with child: %v", raw)
	}
}
