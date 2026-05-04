package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T309/T394: Ctrl+] Claude switching — toggleModel + ToggleReturn handler
//
//  T394 moved Ctrl+] handling from the JS update function to the Go-level
//  toggleModel wrapper (BubbleTea ReleaseTerminal/RestoreTerminal lifecycle).
//  The onToggle callback (prSplit._onToggle) handles the guard check and
//  dispatches through the session proxy's passthrough method.
//
//  Task 5: _onToggle now uses the pinned SessionID proxy
//  (getInteractivePaneSession) instead of raw tuiMux.switchTo(). This
//  ensures passthrough targets the correct session regardless of which
//  session is currently active in the SessionManager.
//
//  Tests that:
//  1. _onToggle calls proxy passthrough (activate → switchTo → restore)
//     when Claude has a pinned SessionID and is running.
//  2. _onToggle returns {skipped: true} when Claude has no pinned SessionID.
//  3. _onToggle returns {skipped: true} when tuiMux is completely absent.
//  4. ToggleReturn with skipped=true sets the notification.
//  5. ToggleReturn without skipped does NOT set notification.
//  6. Status bar shows "Ctrl+] Claude" only when Claude proxy isRunning.
//  7. _onToggle activates the correct sessionID before switchTo and
//     restores the previous activeID afterward.
// ---------------------------------------------------------------------------

func TestKeyHandling_CtrlBracket_EquivCheck(t *testing.T) {
	t.Parallel()

	t.Run("onToggle_calls_passthrough_when_claude_attached", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var activateCalls = [];
			var switchCalled = false;
			globalThis.tuiMux = {
				isDone: function(id) { return false; },
				activeID: function() { return 99; },
				activate: function(id) { activateCalls.push(id); },
				switchTo: function() { switchCalled = true; return {reason: 'toggle'}; },
				snapshot: function(id) { return { fullScreen: '', plainText: '' }; }
			};
			// Set pinned Claude SessionID.
			prSplit._state.claudeSessionID = 42;
			try {
				var result = globalThis.prSplit._onToggle();
				if (!switchCalled) return 'FAIL: switchTo was not called';
				if (result.skipped) return 'FAIL: should not be skipped';
				if (result.reason !== 'toggle') return 'FAIL: unexpected result: ' + JSON.stringify(result);
				// Task 5: Verify session-specific activate/restore pattern.
				if (activateCalls.length < 1) return 'FAIL: activate not called';
				if (activateCalls[0] !== 42) return 'FAIL: activate called with wrong ID: ' + activateCalls[0];
				// Restore should re-activate previous (99).
				if (activateCalls.length < 2 || activateCalls[1] !== 99)
					return 'FAIL: previous activeID not restored: ' + JSON.stringify(activateCalls);
				return 'OK';
			} finally {
				delete globalThis.tuiMux;
				delete prSplit._state.claudeSessionID;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("onToggle with claude attached: %v", raw)
		}
	})

	t.Run("onToggle_skipped_when_no_claude_session_id", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var switchCalled = false;
			globalThis.tuiMux = {
				isDone: function(id) { return true; },
				activeID: function() { return 0; },
				activate: function(id) {},
				switchTo: function() { switchCalled = true; },
				snapshot: function(id) { return null; }
			};
			// No claudeSessionID set — Claude not attached.
			delete prSplit._state.claudeSessionID;
			try {
				var result = globalThis.prSplit._onToggle();
				if (switchCalled) return 'FAIL: switchTo should not be called when no claudeSessionID';
				if (!result.skipped) return 'FAIL: should be skipped';
				if (result.reason !== 'no_child') return 'FAIL: wrong reason: ' + result.reason;
				return 'OK';
			} finally {
				delete globalThis.tuiMux;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("onToggle with no claudeSessionID: %v", raw)
		}
	})

	t.Run("onToggle_skipped_when_claude_session_done", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var switchCalled = false;
			globalThis.tuiMux = {
				isDone: function(id) { return true; },
				activeID: function() { return 42; },
				activate: function(id) {},
				switchTo: function() { switchCalled = true; },
				snapshot: function(id) { return null; }
			};
			// Claude SessionID set but session is done.
			prSplit._state.claudeSessionID = 42;
			try {
				var result = globalThis.prSplit._onToggle();
				if (switchCalled) return 'FAIL: switchTo should not be called when session is done';
				if (!result.skipped) return 'FAIL: should be skipped';
				if (result.reason !== 'no_child') return 'FAIL: wrong reason: ' + result.reason;
				return 'OK';
			} finally {
				delete globalThis.tuiMux;
				delete prSplit._state.claudeSessionID;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("onToggle with done claude session: %v", raw)
		}
	})

	t.Run("onToggle_skipped_when_tuiMux_absent", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			// Ensure tuiMux is explicitly absent.
			var saved = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
			delete globalThis.tuiMux;
			try {
				var result = globalThis.prSplit._onToggle();
				if (!result.skipped) return 'FAIL: should be skipped when tuiMux absent';
				if (result.reason !== 'no_child') return 'FAIL: wrong reason: ' + result.reason;
				return 'OK';
			} finally {
				if (saved !== undefined) globalThis.tuiMux = saved;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("onToggle with tuiMux absent: %v", raw)
		}
	})

	t.Run("ToggleReturn_skipped_sets_notification", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var s = initState('EQUIV_CHECK');
			var r = update({type: 'ToggleReturn', skipped: true, reason: 'no_child'}, s);
			s = r[0];
			var errors = [];
			if (!s.claudeAutoAttachNotif) errors.push('expected notification, got empty');
			if (s.claudeAutoAttachNotif && s.claudeAutoAttachNotif.indexOf('not available') < 0)
				errors.push('expected "not available" in notification, got: ' + s.claudeAutoAttachNotif);
			if (!s.claudeAutoAttachNotifAt) errors.push('notifAt timestamp not set');
			return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("ToggleReturn skipped notification: %v", raw)
		}
	})

	t.Run("ToggleReturn_success_no_notification", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var s = initState('PLAN_REVIEW');
			var r = update({type: 'ToggleReturn', reason: 'toggle'}, s);
			s = r[0];
			if (s.claudeAutoAttachNotif && s.claudeAutoAttachNotif.indexOf('not available') >= 0)
				return 'FAIL: should not set "not available" notification on success';
			return 'OK';
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("ToggleReturn success: %v", raw)
		}
	})
}

func TestStatusBar_CtrlBracketHint_ConditionalOnMux(t *testing.T) {
	t.Parallel()

	t.Run("shows_hint_when_child_attached", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			globalThis.tuiMux = {
				isDone: function(id) { return false; },
				activeID: function() { return 1; },
				activate: function(id) {},
				snapshot: function(id) { return { fullScreen: '', plainText: '' }; },
				lastActivityMs: function() { return Date.now(); }
			};
			prSplit._state.claudeSessionID = 1;
			try {
				var s = { width: 80, wizardState: 'EQUIV_CHECK' };
				var rendered = globalThis.prSplit._renderStatusBar(s);
				return rendered;
			} finally {
				delete globalThis.tuiMux;
				delete prSplit._state.claudeSessionID;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "Ctrl+] Claude") {
			t.Errorf("expected status bar to contain 'Ctrl+] Claude' when child attached, got:\n%s", rendered)
		}
	})

	t.Run("hides_hint_when_no_child", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			globalThis.tuiMux = {
				isDone: function(id) { return true; },
				activeID: function() { return 0; },
				activate: function(id) {},
				snapshot: function(id) { return null; },
				lastActivityMs: function() { return Date.now(); }
			};
			// No claudeSessionID — Claude not attached.
			delete prSplit._state.claudeSessionID;
			try {
				var s = { width: 80, wizardState: 'EQUIV_CHECK' };
				var rendered = globalThis.prSplit._renderStatusBar(s);
				return rendered;
			} finally {
				delete globalThis.tuiMux;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if strings.Contains(rendered, "Ctrl+] Claude") || strings.Contains(rendered, "C-]") {
			t.Errorf("expected status bar to NOT contain 'Ctrl+] Claude' when no child, got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "Ctrl+L Split") {
			t.Errorf("expected status bar to still show 'Ctrl+L Split', got:\n%s", rendered)
		}
	})

	t.Run("hides_hint_when_tuiMux_absent", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			delete globalThis.tuiMux;
			var s = { width: 80, wizardState: 'EQUIV_CHECK' };
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if strings.Contains(rendered, "Ctrl+] Claude") || strings.Contains(rendered, "C-]") {
			t.Errorf("expected status bar to NOT contain 'Ctrl+] Claude' when tuiMux absent, got:\n%s", rendered)
		}
	})

	t.Run("narrow_hides_hint_when_no_child", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			globalThis.tuiMux = {
				isDone: function(id) { return true; },
				activeID: function() { return 0; },
				activate: function(id) {},
				snapshot: function(id) { return null; }
			};
			// No claudeSessionID — Claude not attached.
			delete prSplit._state.claudeSessionID;
			try {
				// veryNarrow (<40): would show 'C-]' if child attached, empty if not.
				var s = { width: 30, wizardState: 'EQUIV_CHECK' };
				var rendered = globalThis.prSplit._renderStatusBar(s);
				return rendered;
			} finally {
				delete globalThis.tuiMux;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if strings.Contains(rendered, "C-]") {
			t.Errorf("expected narrow status bar to NOT contain 'C-]' when no child, got:\n%s", rendered)
		}
	})
}

// T337: Status bar shortcuts for verify tab.
func TestStatusBar_VerifyShellShortcuts(t *testing.T) {
	t.Parallel()

	t.Run("split_disabled_no_tab_hints", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = { width: 80, wizardState: 'BRANCH_BUILDING', splitViewEnabled: false };
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if strings.Contains(rendered, "Ctrl+O") {
			t.Errorf("expected no Ctrl+O when split disabled, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "INPUT") {
			t.Errorf("expected no INPUT when split disabled, got:\n%s", rendered)
		}
	})

	t.Run("split_enabled_wizard_focused_shows_ctrl_o", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = {
				width: 80, wizardState: 'BRANCH_BUILDING',
				splitViewEnabled: true, splitViewFocus: 'wizard',
				splitViewTab: 'claude'
			};
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "Ctrl+O Tab") {
			t.Errorf("expected 'Ctrl+O Tab' when wizard focused, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "INPUT") {
			t.Errorf("expected no INPUT when wizard focused, got:\n%s", rendered)
		}
	})

	t.Run("verify_tab_focused_shows_input_verify", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = {
				width: 80, wizardState: 'BRANCH_BUILDING',
				splitViewEnabled: true, splitViewFocus: 'claude',
				splitViewTab: 'verify', activeVerifySession: { screen: function() { return ''; } }
			};
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "INPUT") || !strings.Contains(rendered, "Verify") {
			t.Errorf("expected 'INPUT ▸ Verify' when verify tab focused, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "Ctrl+O Tab") {
			t.Errorf("expected Ctrl+O Tab to be replaced by INPUT indicator, got:\n%s", rendered)
		}
	})

	// Task 8: shell_tab_focused_shows_input_shell subtest removed —
	// shell tab unified into verify pane.

	t.Run("verify_tab_no_session_shows_ctrl_o", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = {
				width: 80, wizardState: 'BRANCH_BUILDING',
				splitViewEnabled: true, splitViewFocus: 'claude',
				splitViewTab: 'verify', activeVerifySession: null
			};
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "Ctrl+O Tab") {
			t.Errorf("expected 'Ctrl+O Tab' when verify tab has no session, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "INPUT") {
			t.Errorf("expected no INPUT when verify session is null, got:\n%s", rendered)
		}
	})

	t.Run("output_tab_focused_shows_ctrl_o", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = {
				width: 80, wizardState: 'BRANCH_BUILDING',
				splitViewEnabled: true, splitViewFocus: 'claude',
				splitViewTab: 'output'
			};
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "Ctrl+O Tab") {
			t.Errorf("expected 'Ctrl+O Tab' on output tab, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "INPUT") {
			t.Errorf("expected no INPUT on output tab, got:\n%s", rendered)
		}
	})

	t.Run("narrow_hides_all_tab_hints", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = {
				width: 50, wizardState: 'BRANCH_BUILDING',
				splitViewEnabled: true, splitViewFocus: 'claude',
				splitViewTab: 'verify', activeVerifySession: { screen: function() { return ''; } }
			};
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if strings.Contains(rendered, "Ctrl+O") || strings.Contains(rendered, "INPUT") {
			t.Errorf("expected no tab hints at narrow width, got:\n%s", rendered)
		}
	})

	t.Run("no_overflow_at_80_columns", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		// Worst case: Claude attached + split enabled + INPUT indicator.
		raw, err := evalJS(`(function() {
			globalThis.tuiMux = {
				isDone: function(id) { return false; },
				activeID: function() { return 1; },
				activate: function(id) {},
				snapshot: function(id) { return { fullScreen: '', plainText: '' }; },
				lastActivityMs: function() { return 500; }
			};
			prSplit._state.claudeSessionID = 1;
			try {
				var s = {
					width: 80, wizardState: 'BRANCH_BUILDING',
					splitViewEnabled: true, splitViewFocus: 'claude',
					splitViewTab: 'verify', activeVerifySession: { screen: function() { return ''; } }
				};
				var bar = globalThis.prSplit._renderStatusBar(s);
				// Split by newlines and check the last line (the status line itself).
				var lines = bar.split('\n');
				var statusLine = lines[lines.length - 1];
				// Use lipgloss.width for ANSI-aware width.
				return { rendered: bar, width: globalThis.prSplit._lipgloss.width(statusLine) };
			} finally {
				delete globalThis.tuiMux;
				delete prSplit._state.claudeSessionID;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		result := raw.(map[string]any)
		width := int(result["width"].(int64))
		if width > 80 {
			t.Errorf("status bar exceeds 80 columns: visual width is %d\n%s", width, result["rendered"])
		}
	})
}

// T337: Help overlay documents verify tab shortcuts.
func TestHelpOverlay_VerifyShellDocs(t *testing.T) {
	t.Parallel()

	t.Run("branch_building_shows_terminal_hints", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`globalThis.prSplit._viewHelpOverlay({
			wizardState: 'BRANCH_BUILDING', width: 80, splitViewEnabled: true
		})`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		// Section header.
		if !strings.Contains(rendered, "Split View") {
			t.Errorf("expected 'Split View' section in help, got:\n%s", rendered)
		}
		// Terminal forwarding hints (only on BRANCH_BUILDING).
		if !strings.Contains(rendered, "forwarded") {
			t.Errorf("expected terminal forwarding hint in help, got:\n%s", rendered)
		}
		// Tab cycling description.
		if !strings.Contains(rendered, "Cycle tabs") {
			t.Errorf("expected tab cycling description in help, got:\n%s", rendered)
		}
	})

	t.Run("config_screen_no_terminal_hints", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`globalThis.prSplit._viewHelpOverlay({
			wizardState: 'CONFIG', width: 80
		})`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		// Should still have Split View section.
		if !strings.Contains(rendered, "Split View") {
			t.Errorf("expected 'Split View' section in help, got:\n%s", rendered)
		}
		// But NOT terminal forwarding hints (CONFIG has no verify/shell).
		if strings.Contains(rendered, "forwarded to focused terminal") {
			t.Errorf("expected no terminal forwarding hint on CONFIG screen, got:\n%s", rendered)
		}
	})

	t.Run("equiv_check_shows_terminal_hints", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`globalThis.prSplit._viewHelpOverlay({
			wizardState: 'EQUIV_CHECK', width: 80, splitViewEnabled: true
		})`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "forwarded") {
			t.Errorf("expected terminal forwarding hint on EQUIV_CHECK, got:\n%s", rendered)
		}
		if !strings.Contains(rendered, "SGR") {
			t.Errorf("expected SGR mouse hint on EQUIV_CHECK, got:\n%s", rendered)
		}
	})
}
