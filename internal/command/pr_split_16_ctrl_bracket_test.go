package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T309: Ctrl+] Claude switching — diagnostic logging & conditional status bar
//
//  Tests that:
//  1. Ctrl+] calls tuiMux.switchTo('claude') when a child IS attached.
//  2. Ctrl+] sets a notification when Claude is NOT attached (hasChild false).
//  3. Ctrl+] sets a notification when tuiMux is completely absent.
//  4. Status bar shows "Ctrl+] Claude" only when tuiMux.hasChild() is true.
//  5. All scenarios work specifically on the EQUIV_CHECK screen.
// ---------------------------------------------------------------------------

func TestKeyHandling_CtrlBracket_EquivCheck(t *testing.T) {
	t.Parallel()

	t.Run("switch_succeeds_when_child_attached", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var switchedTo = null;
			globalThis.tuiMux = {
				hasChild: function() { return true; },
				switchTo: function(name) { switchedTo = name; }
			};
			try {
				var s = initState('EQUIV_CHECK');
				var r = sendKey(s, 'ctrl+]');
				if (switchedTo !== 'claude') return 'FAIL: ctrl+] did not switch to claude, got ' + switchedTo;
				// Should NOT set the notification since switch succeeded.
				if (r[0].claudeAutoAttachNotif && r[0].claudeAutoAttachNotif.indexOf('not available') >= 0)
					return 'FAIL: should not set not-available notification on success';
				return 'OK';
			} finally {
				delete globalThis.tuiMux;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("ctrl+] with child attached: %v", raw)
		}
	})

	t.Run("notification_when_hasChild_false", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var switchedTo = null;
			globalThis.tuiMux = {
				hasChild: function() { return false; },
				switchTo: function(name) { switchedTo = name; }
			};
			try {
				var s = initState('EQUIV_CHECK');
				var r = sendKey(s, 'ctrl+]');
				if (switchedTo !== null) return 'FAIL: switchTo should not be called when hasChild is false';
				if (!r[0].claudeAutoAttachNotif) return 'FAIL: expected notification, got empty';
				if (r[0].claudeAutoAttachNotif.indexOf('not available') < 0)
					return 'FAIL: expected "not available" in notification, got: ' + r[0].claudeAutoAttachNotif;
				if (!r[0].claudeAutoAttachNotifAt) return 'FAIL: notifAt timestamp not set';
				return 'OK';
			} finally {
				delete globalThis.tuiMux;
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("ctrl+] with hasChild false: %v", raw)
		}
	})

	t.Run("notification_when_tuiMux_absent", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			// Ensure tuiMux is explicitly absent.
			delete globalThis.tuiMux;

			var s = initState('EQUIV_CHECK');
			var r = sendKey(s, 'ctrl+]');
			if (!r[0].claudeAutoAttachNotif) return 'FAIL: expected notification, got empty';
			if (r[0].claudeAutoAttachNotif.indexOf('not available') < 0)
				return 'FAIL: expected "not available" in notification, got: ' + r[0].claudeAutoAttachNotif;
			if (!r[0].claudeAutoAttachNotifAt) return 'FAIL: notifAt timestamp not set';
			return 'OK';
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("ctrl+] with tuiMux absent: %v", raw)
		}
	})

	t.Run("works_on_multiple_wizard_states", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngineWithHelpers(t)

		raw, err := evalJS(`(function() {
			var states = ['EQUIV_CHECK', 'CONFIG', 'PLAN_REVIEW', 'BRANCH_BUILDING', 'FINALIZATION'];
			var errors = [];
			for (var i = 0; i < states.length; i++) {
				var st = states[i];
				var switchedTo = null;
				globalThis.tuiMux = {
					hasChild: function() { return true; },
					switchTo: function(name) { switchedTo = name; }
				};
				try {
					var s = initState(st);
					sendKey(s, 'ctrl+]');
					if (switchedTo !== 'claude') errors.push(st + ': did not switch');
				} finally {
					delete globalThis.tuiMux;
				}
			}
			return errors.length ? 'FAIL: ' + errors.join('; ') : 'OK';
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		if raw != "OK" {
			t.Errorf("ctrl+] across wizard states: %v", raw)
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
				hasChild: function() { return true; },
				lastActivityMs: function() { return Date.now(); }
			};
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
		if !strings.Contains(rendered, "Ctrl+] Claude") {
			t.Errorf("expected status bar to contain 'Ctrl+] Claude' when child attached, got:\n%s", rendered)
		}
	})

	t.Run("hides_hint_when_no_child", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			globalThis.tuiMux = {
				hasChild: function() { return false; },
				lastActivityMs: function() { return Date.now(); }
			};
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
				hasChild: function() { return false; }
			};
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

// T337: Status bar shortcuts for verify/shell tabs.
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

	t.Run("shell_tab_focused_shows_input_shell", func(t *testing.T) {
		t.Parallel()
		evalJS := prsplittest.NewTUIEngine(t)

		raw, err := evalJS(`(function() {
			var s = {
				width: 80, wizardState: 'BRANCH_BUILDING',
				splitViewEnabled: true, splitViewFocus: 'claude',
				splitViewTab: 'shell', shellSession: { screen: function() { return ''; } }
			};
			return globalThis.prSplit._renderStatusBar(s);
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		rendered := raw.(string)
		if !strings.Contains(rendered, "INPUT") || !strings.Contains(rendered, "Shell") {
			t.Errorf("expected 'INPUT ▸ Shell' when shell tab focused, got:\n%s", rendered)
		}
		if strings.Contains(rendered, "Ctrl+O Tab") {
			t.Errorf("expected Ctrl+O Tab to be replaced by INPUT indicator, got:\n%s", rendered)
		}
	})

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
				hasChild: function() { return true; },
				lastActivityMs: function() { return 500; }
			};
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
			}
		})()`)
		if err != nil {
			t.Fatal(err)
		}
		result := raw.(map[string]interface{})
		width := int(result["width"].(int64))
		if width > 80 {
			t.Errorf("status bar exceeds 80 columns: visual width is %d\n%s", width, result["rendered"])
		}
	})
}

// T337: Help overlay documents verify/shell shortcuts.
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
