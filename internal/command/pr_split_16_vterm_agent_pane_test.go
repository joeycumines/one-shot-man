package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// VTerm Integration Tests: Agent Pane Rendering
//
// These tests verify the Agent pane rendering pipeline:
//   tuiMux.snapshot(agentSessionID) → pollAgentScreenshot → state fields →
//   renderAgentPane → _wizardView (split-view layout)
//
// All tests use mock tuiMux objects at the JS layer — no real process, no
// real VTerm. They prove that ANSI content, plain text fallback, scroll
// indicators, focus styling, and placeholder states render correctly.
// ---------------------------------------------------------------------------

// -- renderAgentPane tests -------------------------------------------------

// vtermMuxSetup returns JS code to save/set a mock tuiMux with pinned SessionID.
const vtermMuxSetup = `
var __savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
var __mockCID = 42;
prSplit._state = prSplit._state || {};
prSplit._state.agentSessionID = __mockCID;
globalThis.tuiMux = {
	snapshot: function(id) { return { fullScreen: '', plainText: '' }; },
	isDone: function(id) { return false; },
	activeID: function() { return __mockCID; },
	activate: function(id) {},
	input: function(data) {},
	switchTo: function() { return { reason: 'toggle' }; },
	lastActivityMs: function() { return 100; }
};
`

// vtermMuxRestore returns JS code to restore the original tuiMux.
const vtermMuxRestore = `
if (__savedMux !== undefined) globalThis.tuiMux = __savedMux;
else delete globalThis.tuiMux;
if (prSplit._state) prSplit._state.agentSessionID = null;
`

func TestChunk16_VTerm_RenderAgentPane_ANSIContent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + vtermMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		// 20 lines of ANSI content to overflow default viewH (height-3=9).
		var lines = [];
		for (var i = 0; i < 20; i++) lines.push('\x1b[31mRed line ' + i + '\x1b[0m');
		lines.push('\x1b[1;32mGreen bold\x1b[0m');
		s.agentScreen = lines.join('\n');
		s.agentScreenshot = '';
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.agentViewOffset = 0;

		var view = globalThis.prSplit._renderAgentPane(s, 60, 12);
		var errors = [];

		// Should NOT show [plain] tag (we have ANSI content in agentScreen).
		if (view.indexOf('[plain]') >= 0) errors.push('should not show [plain] tag for ANSI content');
		// Should show some content lines.
		if (view.indexOf('Red line') < 0) errors.push('ANSI pane should contain "Red line"');
		// Should show [live] indicator (offset=0, totalLines > viewH).
		if (view.indexOf('[live]') < 0) errors.push('should show [live] indicator, got: ' + view.substring(0, 300));
		// Should NOT show INPUT tag (wizard focused, not agent).
		if (view.indexOf('INPUT') >= 0) errors.push('should not show INPUT when wizard focused');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + vtermMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ANSI content rendering: %v", raw)
	}
}

func TestChunk16_VTerm_RenderAgentPane_PlainFallback(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + vtermMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		// agentScreen empty → isANSI=false → [plain] tag.
		// agentScreenshot has content → used as fallback.
		s.agentScreen = '';
		s.agentScreenshot = 'Plain text output from Agent\nLine 2\nLine 3';
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.agentViewOffset = 0;

		var view = globalThis.prSplit._renderAgentPane(s, 60, 12);
		var errors = [];

		// MUST show [plain] tag (agentScreen empty → not ANSI).
		if (view.indexOf('[plain]') < 0) errors.push('should show [plain] tag for non-ANSI content, got: ' + view.substring(0, 300));
		// Should show plain text content.
		if (view.indexOf('Plain text output') < 0) errors.push('should show plain text');
		// Should show Agent pane header.
		if (view.indexOf('Agent') < 0) errors.push('should show Agent in title');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + vtermMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plain fallback: %v", raw)
	}
}

func TestChunk16_VTerm_RenderAgentPane_Placeholder_NoMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Remove tuiMux entirely.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		delete globalThis.tuiMux;
		try {
			var s = initState('PLAN_REVIEW');
			s.agentScreen = '';
			s.agentScreenshot = '';
			s.splitViewEnabled = true;
			s.splitViewFocus = 'wizard';
			s.agentViewOffset = 0;

			var view = globalThis.prSplit._renderAgentPane(s, 60, 12);
			var errors = [];

			// Should show placeholder message.
			if (view.indexOf('No Agent') < 0 && view.indexOf('Waiting') < 0) {
				errors.push('should show placeholder when no mux attached, got: ' + view.substring(0, 200));
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no-mux placeholder: %v", raw)
	}
}

func TestChunk16_VTerm_RenderAgentPane_Placeholder_EmptyContent(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mux exists but no content yet.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; }
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.agentScreen = '';
			s.agentScreenshot = '';
			s.splitViewEnabled = true;
			s.splitViewFocus = 'wizard';
			s.agentViewOffset = 0;

			var view = globalThis.prSplit._renderAgentPane(s, 60, 12);
			var errors = [];

			// Should show waiting/placeholder.
			if (view.indexOf('Waiting') < 0 && view.indexOf('No Agent') < 0) {
				errors.push('should show waiting when mux has child but no content');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("empty content placeholder: %v", raw)
	}
}

func TestChunk16_VTerm_RenderAgentPane_FocusIndicator(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + vtermMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.agentScreen = 'test content\nline 2';
		s.agentScreenshot = '';
		s.splitViewEnabled = true;
		s.agentViewOffset = 0;

		// Render unfocused.
		s.splitViewFocus = 'wizard';
		var unfocused = globalThis.prSplit._renderAgentPane(s, 60, 12);

		// Render focused.
		s.splitViewFocus = 'agent';
		var focused = globalThis.prSplit._renderAgentPane(s, 60, 12);

		var errors = [];

		// Focused pane should show INPUT tag.
		if (focused.indexOf('INPUT') < 0) errors.push('focused pane should show INPUT tag, got: ' + focused.substring(0, 200));
		// Unfocused should NOT show INPUT tag.
		if (unfocused.indexOf('INPUT') >= 0) errors.push('unfocused pane should not show INPUT tag');
		// Both should render (non-empty).
		if (focused.length === 0) errors.push('focused render is empty');
		if (unfocused.length === 0) errors.push('unfocused render is empty');
		// Focused and unfocused should DIFFER (different border color / INPUT tag).
		if (focused === unfocused) errors.push('focused and unfocused should differ');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + vtermMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("focus indicator: %v", raw)
	}
}

func TestChunk16_VTerm_RenderAgentPane_LiveIndicator(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + vtermMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		// 40 lines — overflows viewH (height=12 → viewH=9), enabling scroll indicator.
		var lines = [];
		for (var i = 0; i < 40; i++) lines.push('Line ' + i);
		s.agentScreen = lines.join('\n');
		s.agentScreenshot = '';
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';

		// offset=0 → [live] (tailing)
		s.agentViewOffset = 0;
		var liveView = globalThis.prSplit._renderAgentPane(s, 60, 12);

		// offset=10 → should show percentage
		s.agentViewOffset = 10;
		var scrolledView = globalThis.prSplit._renderAgentPane(s, 60, 12);

		var errors = [];

		if (liveView.indexOf('[live]') < 0) errors.push('[live] indicator missing when offset=0, got: ' + liveView.substring(0, 300));
		// When scrolled, should show percentage.
		if (scrolledView.indexOf('[live]') >= 0) errors.push('should NOT show [live] when scrolled up');
		if (scrolledView.indexOf('%') < 0) errors.push('should show percentage indicator when scrolled');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + vtermMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("live/scroll indicator: %v", raw)
	}
}

func TestChunk16_VTerm_RenderAgentPane_TinyTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// Set up tuiMux so we exercise the content-rendering code path (not just placeholder).
	raw, err := evalJS(`(function() {
		` + vtermMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.agentScreen = 'some content\nline 2\nline 3';
		s.agentScreenshot = '';
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.agentViewOffset = 0;

		// Tiny terminal — should not crash in content-rendering path.
		var view = globalThis.prSplit._renderAgentPane(s, 10, 5);
		if (typeof view !== 'string') return 'FAIL: render did not return string';
		if (view.length === 0) return 'FAIL: render returned empty string';
		return 'OK';
		} finally { ` + vtermMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("tiny terminal: %v", raw)
	}
}

func TestChunk16_VTerm_PollAgentScreenshot_DrainsMuxEvents(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var pollCalls = 0;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			pollEvents: function() { pollCalls++; return 1; },
			childScreen: function() { return 'ANSI screen'; },
			screenshot: function() { return 'plain screen'; },
			snapshot: function(id) { return { fullScreen: 'ANSI screen', plainText: 'plain screen' }; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			activate: function(id) {},
			input: function(data) {},
			lastActivityMs: function() { return 42; }
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.agentScreen = '';
			s.agentScreenshot = '';
			globalThis.prSplit._pollAgentScreenshot(s);
			return JSON.stringify({
				pollCalls: pollCalls,
				screen: s.agentScreen,
				shot: s.agentScreenshot
			});
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.agentSessionID = null;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"pollCalls":1,"screen":"ANSI screen","shot":"plain screen"}`
	if raw != want {
		t.Errorf("pollAgentScreenshot drain = %v, want %v", raw, want)
	}
}

// -- pollAgentScreenshot tests (via Tick message — not directly exported) --

func TestChunk16_VTerm_PollScreenshot_CapturesFromMux(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// pollAgentScreenshot is internal — test it via update({type:'Tick', id:'agent-screenshot'}).
	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return '\x1b[33mANSI yellow\x1b[0m'; },
			screenshot: function() { return 'plain screenshot output'; },
			snapshot: function(id) { return { fullScreen: '\x1b[33mANSI yellow\x1b[0m', plainText: 'plain screenshot output' }; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			activate: function(id) {},
			input: function(data) {},
			lastActivityMs: function() { return 100; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.agentScreen = '';
			s.agentScreenshot = '';
			s.isProcessing = false;

			// Trigger the agent-screenshot tick message.
			var r = update({type: 'Tick', id: 'agent-screenshot'}, s);
			var ns = r[0];

			var errors = [];

			if (!ns.agentScreen || ns.agentScreen.indexOf('ANSI yellow') < 0) {
				errors.push('agentScreen should contain ANSI content from childScreen()');  
			}
			if (!ns.agentScreenshot || ns.agentScreenshot.indexOf('plain screenshot') < 0) {
				errors.push('agentScreenshot should contain plain content from screenshot()');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.agentSessionID = null;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("poll capture: %v", raw)
	}
}

func TestChunk16_VTerm_PollScreenshot_NoChild_ClearsState(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; },
			lastActivityMs: function() { return 0; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.agentScreen = 'old ANSI content';
			s.agentScreenshot = 'old screenshot';
			s.agentAutoAttached = false;

			var r = update({type: 'Tick', id: 'agent-screenshot'}, s);
			var ns = r[0];

			var errors = [];

			if (ns.agentScreen !== '') {
				errors.push('agentScreen should be cleared when no child, got: ' + ns.agentScreen);
			}
			if (ns.agentScreenshot !== '') {
				errors.push('agentScreenshot should be cleared when no child');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no child clears state: %v", raw)
	}
}

func TestChunk16_VTerm_PollScreenshot_SplitViewDisabled_StopsPolling(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// When splitViewEnabled=false, the agent-screenshot tick should NOT
	// restart polling (pollAgentScreenshot returns null cmd).
	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;
		s.agentScreen = 'leftover';
		s.agentScreenshot = 'leftover';

		var r = update({type: 'Tick', id: 'agent-screenshot'}, s);
		var ns = r[0];
		var cmd = r[1];

		var errors = [];

		if (typeof ns !== 'object') {
			errors.push('state should be returned');
		}
		// Cmd MUST be null — no further polling when split view disabled.
		if (cmd !== null) {
			errors.push('cmd should be null when split view disabled, got: ' + JSON.stringify(cmd));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("disabled stops polling: %v", raw)
	}
}

func TestChunk16_VTerm_PollScreenshot_AutoCloseOnChildExit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			session: function() { return { isRunning: function() { return false; }, isDone: function() { return true; } }; },
			childScreen: function() { return ''; },
			screenshot: function() { return ''; },
			snapshot: function(id) { return { fullScreen: '', plainText: '' }; },
			isDone: function(id) { return true; },
			activeID: function() { return __mockCID; },
			activate: function(id) {},
			input: function(data) {},
			lastActivityMs: function() { return 0; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.agentAutoAttached = true;
			s.autoSplitRunning = false;

			var r = update({type: 'Tick', id: 'agent-screenshot'}, s);
			var ns = r[0];

			var errors = [];

			// Auto-attached pane should auto-close when child exits.
			if (ns.splitViewEnabled !== false) {
				errors.push('splitViewEnabled should be false after auto-close');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.agentSessionID = null;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-close on child exit: %v", raw)
	}
}

// -- splitView toggle tests -------------------------------------------------

func TestChunk16_VTerm_SplitViewToggle_CtrlL(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;
		s.agentManuallyDismissed = false;

		// Toggle ON.
		var r = sendKey(s, 'ctrl+l');
		var ns = r[0];
		var errors = [];

		if (ns.splitViewEnabled !== true) errors.push('Ctrl+L should enable split view');
		if (ns.agentManuallyDismissed !== false) errors.push('opening should clear dismiss flag');

		// Toggle OFF.
		var r2 = sendKey(ns, 'ctrl+l');
		ns = r2[0];

		if (ns.splitViewEnabled !== false) errors.push('second Ctrl+L should disable split view');
		if (ns.agentManuallyDismissed !== true) errors.push('closing should set dismiss flag');
		if (ns.agentViewOffset !== 0) errors.push('closing should reset scroll offset');
		if (ns.splitViewFocus !== 'wizard') errors.push('closing should reset focus to wizard');

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("Ctrl+L toggle: %v", raw)
	}
}

func TestChunk16_VTerm_SplitViewFocusSwitch_CtrlTab(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';

		// Ctrl+Tab should switch focus to agent.
		var r = sendKey(s, 'ctrl+tab');
		var errors = [];

		if (r[0].splitViewFocus !== 'agent') errors.push('Ctrl+Tab should switch focus to agent, got: ' + r[0].splitViewFocus);

		// T61: Ctrl+Tab again should advance to output tab (not back to wizard).
		var r2 = sendKey(r[0], 'ctrl+tab');
		if (r2[0].splitViewFocus !== 'agent') errors.push('Ctrl+Tab should stay on pane, got focus: ' + r2[0].splitViewFocus);
		if (r2[0].splitViewTab !== 'output') errors.push('Ctrl+Tab should advance to output, got tab: ' + r2[0].splitViewTab);

		// Ctrl+Tab one more time should wrap back to wizard.
		var r3 = sendKey(r2[0], 'ctrl+tab');
		if (r3[0].splitViewFocus !== 'wizard') errors.push('Ctrl+Tab should wrap to wizard, got: ' + r3[0].splitViewFocus);

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("Ctrl+Tab focus switch: %v", raw)
	}
}

// -- agentViewOffset scroll tests ------------------------------------------

func TestChunk16_VTerm_AgentScroll_PgUpPgDn(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'agent';
		s.splitViewTab = 'agent';
		s.agentViewOffset = 0;
		s.agentScreen = Array(60).fill('line').join('\n');

		var errors = [];

		// PgUp should increase offset by 5.
		var r = sendKey(s, 'pgup');
		if (r[0].agentViewOffset !== 5) {
			errors.push('PgUp should set offset to 5, got: ' + r[0].agentViewOffset);
		}

		// PgDn from offset 5 should decrease by 5 to 0.
		var r2 = sendKey(r[0], 'pgdown');
		if (r2[0].agentViewOffset !== 0) {
			errors.push('PgDn from 5 should set offset to 0, got: ' + r2[0].agentViewOffset);
		}

		// PgDn from 0 should stay at 0 (can't go negative).
		var r3 = sendKey(r2[0], 'pgdown');
		if (r3[0].agentViewOffset !== 0) {
			errors.push('PgDn from 0 should stay at 0, got: ' + r3[0].agentViewOffset);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("PgUp/PgDn scroll: %v", raw)
	}
}

func TestChunk16_VTerm_AgentScroll_UpDown(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'agent';
		s.splitViewTab = 'agent';
		s.agentViewOffset = 0;
		s.agentScreen = Array(60).fill('line').join('\n');

		var errors = [];

		// Up should increase by 1.
		var r = sendKey(s, 'up');
		if (r[0].agentViewOffset !== 1) {
			errors.push('Up should set offset to 1, got: ' + r[0].agentViewOffset);
		}

		// k should also increase by 1 (vim binding).
		var r2 = sendKey(r[0], 'k');
		if (r2[0].agentViewOffset !== 2) {
			errors.push('k should set offset to 2, got: ' + r2[0].agentViewOffset);
		}

		// Down should decrease by 1.
		var r3 = sendKey(r2[0], 'down');
		if (r3[0].agentViewOffset !== 1) {
			errors.push('Down should set offset to 1, got: ' + r3[0].agentViewOffset);
		}

		// j should also decrease by 1 (vim binding).
		var r4 = sendKey(r3[0], 'j');
		if (r4[0].agentViewOffset !== 0) {
			errors.push('j should set offset to 0, got: ' + r4[0].agentViewOffset);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("Up/Down scroll: %v", raw)
	}
}

func TestChunk16_VTerm_AgentScroll_HomeEnd(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'agent';
		s.splitViewTab = 'agent';
		s.agentViewOffset = 10;
		s.agentScreen = Array(60).fill('line').join('\n');

		var errors = [];

		// Home should jump to top (large offset).
		var r = sendKey(s, 'home');
		if (r[0].agentViewOffset < 100) {
			errors.push('Home should set large offset (top), got: ' + r[0].agentViewOffset);
		}

		// End should jump to bottom (offset=0, live tail).
		var r2 = sendKey(r[0], 'end');
		if (r2[0].agentViewOffset !== 0) {
			errors.push('End should set offset to 0 (live), got: ' + r2[0].agentViewOffset);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("Home/End scroll: %v", raw)
	}
}

func TestChunk16_VTerm_AgentScroll_MouseWheel(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'agent';
		s.splitViewTab = 'agent';
		s.agentViewOffset = 0;
		s.agentScreen = Array(60).fill('line').join('\n');

		var errors = [];

		// Wheel up should increase offset by 3.
		var r = sendWheel(s, 'up');
		if (r[0].agentViewOffset !== 3) {
			errors.push('wheel up should set offset to 3, got: ' + r[0].agentViewOffset);
		}

		// Wheel down should decrease by 3.
		var r2 = sendWheel(r[0], 'down');
		if (r2[0].agentViewOffset !== 0) {
			errors.push('wheel down from 3 should set offset to 0, got: ' + r2[0].agentViewOffset);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("mouse wheel scroll: %v", raw)
	}
}

func TestChunk16_VTerm_AgentScroll_DoesNotWorkWhenWizardFocused(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'agent';
		s.agentViewOffset = 0;
		s.agentScreen = Array(60).fill('line').join('\n');

		// PgUp with wizard focused — should NOT change agent offset.
		var r = sendKey(s, 'pgup');
		if (r[0].agentViewOffset !== 0) {
			return 'FAIL: PgUp should not scroll agent when wizard focused, got: ' + r[0].agentViewOffset;
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("wizard-focused scroll: %v", raw)
	}
}

// -- split-view rendering in _wizardView -----------------------------------

func TestChunk16_VTerm_SplitViewLayout_InWizardView(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		` + vtermMuxSetup + `
		try {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'agent';
		s.agentScreen = 'Agent says hello\nMore output';
		s.agentScreenshot = '';
		s.agentViewOffset = 0;
		s.width = 80;
		s.height = 30;
		setupPlanCache();

		var view = globalThis.prSplit._wizardView(s);
		var errors = [];

		// Full view should contain Agent pane content (real content, not placeholder).
		if (view.indexOf('Agent says hello') < 0) {
			errors.push('split-view should show Agent content in layout, got: ' + view.substring(0, 200));
		}
		// View should be non-empty.
		if (view.length < 100) {
			errors.push('split-view render seems too short: ' + view.length + ' chars');
		}
		// Verify split-view is wider than a single-pane render (sanity)
		if (view.indexOf('Agent') < 0) {
			errors.push('split-view should have Agent pane header');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally { ` + vtermMuxRestore + ` }
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("split-view layout: %v", raw)
	}
}

func TestChunk16_VTerm_SplitViewLayout_NoSplitView(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// When splitViewEnabled=false, _wizardView should NOT render Agent pane.
	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;
		s.agentScreen = 'MARKER_CONTENT_FOR_TEST';
		s.width = 80;
		s.height = 30;
		setupPlanCache();

		var view = globalThis.prSplit._wizardView(s);
		var errors = [];

		if (typeof view !== 'string') {
			errors.push('view is not a string');
		} else if (view.length < 50) {
			errors.push('view seems too short: ' + view.length);
		}

		// Agent pane content should NOT appear when split view is disabled.
		if (view.indexOf('MARKER_CONTENT_FOR_TEST') >= 0) {
			errors.push('Agent pane content should not render when splitViewEnabled=false');
		}
		// [live] and INPUT indicators from Agent pane should be absent.
		if (view.indexOf('[live]') >= 0) {
			errors.push('[live] should not appear without split view');
		}
		if (view.indexOf('INPUT') >= 0) {
			errors.push('INPUT indicator should not appear without split view');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no split-view layout: %v", raw)
	}
}

// -- Auto-attach tests ------------------------------------------------------

func TestChunk16_VTerm_AutoAttach_SmallTerminalPrevented(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return 'content'; },
			screenshot: function() { return 'content'; },
			lastActivityMs: function() { return 100; }
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = false;
			s.agentAutoAttached = false;
			s.agentManuallyDismissed = false;
			s.autoSplitRunning = true;
			// Terminal too small for split-view.
			s.height = 8;
			s.isProcessing = true;

			// Mock agentExecutor to prevent crash detection.
			globalThis.prSplit._state.agentExecutor = {
				handle: {
					isAlive: function() { return true; },
					receive: function() { return null; }
				},
				captureDiagnostic: function() { return ''; }
			};

			// Fire auto-poll tick — should NOT auto-attach due to small terminal.
			var r = update({type: 'Tick', id: 'auto-poll'}, s);
			var ns = r[0];
			var errors = [];

			if (ns.splitViewEnabled === true) {
				errors.push('should NOT auto-attach in small terminal (height=' + s.height + ')');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			delete globalThis.prSplit._state.agentExecutor;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("small terminal auto-attach: %v", raw)
	}
}

// -- Full pipeline: mux capture → render integration -----------------------

func TestChunk16_VTerm_FullRenderPipeline_MuxToView(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	// Full pipeline: tuiMux → agent-screenshot tick → poll → state → render.
	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		var ansiContent = '\x1b[1;36mAgent is working...\x1b[0m\nAnalyzing repository structure\n\x1b[32mDone!\x1b[0m';
		var plainContent = 'Agent is working...\nAnalyzing repository structure\nDone!';
		var __mockCID = 42;
		prSplit._state = prSplit._state || {};
		prSplit._state.agentSessionID = __mockCID;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			session: function() { return { isRunning: function() { return true; }, isDone: function() { return false; } }; },
			childScreen: function() { return ansiContent; },
			screenshot: function() { return plainContent; },
			snapshot: function(id) { return { fullScreen: ansiContent, plainText: plainContent }; },
			isDone: function(id) { return false; },
			activeID: function() { return __mockCID; },
			activate: function(id) {},
			input: function(data) {},
			lastActivityMs: function() { return 200; },
			writeToChild: function() {}
		};
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.splitViewFocus = 'wizard';
			s.splitViewTab = 'agent';
			s.agentViewOffset = 0;
			s.isProcessing = false;
			s.width = 80;
			s.height = 30;
			setupPlanCache();

			// Step 1: Send agent-screenshot tick to poll from mux.
			var r = update({type: 'Tick', id: 'agent-screenshot'}, s);
			s = r[0];

			var errors = [];

			// Step 2: Verify state has content from mux.
			if (!s.agentScreen || s.agentScreen.indexOf('Agent is working') < 0) {
				errors.push('agentScreen not captured from mux, got: ' + (s.agentScreen || '').substring(0, 100));
			}

			// Step 3: Render the full view — should include Agent pane content.
			var view = globalThis.prSplit._wizardView(s);
			if (view.indexOf('Agent') < 0) {
				errors.push('wizardView should contain Agent pane');
			}

			// Step 4: Render Agent pane directly.
			var pane = globalThis.prSplit._renderAgentPane(s, 60, 12);
			if (pane.indexOf('Agent is working') < 0) {
				errors.push('renderAgentPane should show mux content');
			}
			// ANSI content → no [plain] tag.
			if (pane.indexOf('[plain]') >= 0) {
				errors.push('should not show [plain] for ANSI content');
			}

			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		} finally {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			if (prSplit._state) prSplit._state.agentSessionID = null;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("full render pipeline: %v", raw)
	}
}
