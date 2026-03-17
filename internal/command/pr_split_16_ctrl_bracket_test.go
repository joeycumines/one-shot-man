package command

import (
	"strings"
	"testing"
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
		evalJS := loadTUIEngineWithHelpers(t)

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
		evalJS := loadTUIEngineWithHelpers(t)

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
		evalJS := loadTUIEngineWithHelpers(t)

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
		evalJS := loadTUIEngineWithHelpers(t)

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
		evalJS := loadTUIEngine(t)

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
		evalJS := loadTUIEngine(t)

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
		evalJS := loadTUIEngine(t)

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
		evalJS := loadTUIEngine(t)

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
