package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
// T342: Input routing tests for verify/shell/claude/output tabs
//
// These are mock-only tests — no PTY spawning. They exercise the keyboard
// input dispatch logic in _wizardUpdate (chunk 16e) when split-view is
// enabled and the bottom pane is focused.
//
// Key routing rules:
//   - Verify tab + activeVerifySession: non-reserved keys → session.write()
//   - Shell tab + shellSession:         non-reserved keys → session.write()
//   - Output tab:                       read-only, keys consumed (no forwarding)
//   - Claude tab:                       non-reserved keys → tuiMux.writeToChild()
//   - Reserved keys (ctrl+tab, ctrl+o): always handled by split-view controls
//   - Wizard focus:                     all keys go to wizard, not terminal
// ---------------------------------------------------------------------------

func TestInputRouting_VerifyTabConsumedKey(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		var written = [];
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'verify';
		s.activeVerifySession = {
			write: function(b) { written.push(b); },
			screen: function() { return ''; },
			isDone: function() { return false; }
		};
		s.verifyScreen = '';
		s.verifyViewportOffset = 0;
		s.verifyAutoScroll = true;

		var r = sendKey(s, 'a');
		var ns = r[0];

		// State should be unchanged — key consumed by verify tab forwarding.
		if (ns.wizardState !== 'BRANCH_BUILDING') {
			errors.push('wizardState changed to ' + ns.wizardState);
		}

		// The 'a' key should have been forwarded to the session.
		if (written.length === 0) {
			errors.push('session.write was not called');
		} else if (written[0] !== 'a') {
			errors.push('wrong bytes written: ' + JSON.stringify(written[0]));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify tab consumed key: %v", raw)
	}
}

func TestInputRouting_ShellTabConsumedKey(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		var written = [];
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'shell';
		s.shellSession = {
			write: function(b) { written.push(b); },
			screen: function() { return ''; },
			isDone: function() { return false; }
		};
		s.shellScreen = '';
		s.shellViewOffset = 0;
		s.shellAutoScroll = true;

		var r = sendKey(s, 'a');
		var ns = r[0];

		// State should be unchanged — key consumed by shell tab forwarding.
		if (ns.wizardState !== 'BRANCH_BUILDING') {
			errors.push('wizardState changed to ' + ns.wizardState);
		}

		// The 'a' key should have been forwarded to the shell session.
		if (written.length === 0) {
			errors.push('shellSession.write was not called');
		} else if (written[0] !== 'a') {
			errors.push('wrong bytes written: ' + JSON.stringify(written[0]));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("shell tab consumed key: %v", raw)
	}
}

func TestInputRouting_OutputTabPassthrough(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'output';
		s.outputViewOffset = 0;
		s.outputAutoScroll = true;

		var r = sendKey(s, 'a');
		var ns = r[0];

		// Output tab is read-only: non-scroll keys are consumed (not forwarded,
		// not passed to wizard). State should remain unchanged.
		if (ns.wizardState !== 'BRANCH_BUILDING') {
			errors.push('wizardState changed to ' + ns.wizardState);
		}

		// Verify scroll state was not perturbed by the 'a' key.
		if (ns.outputViewOffset !== 0) {
			errors.push('outputViewOffset changed to ' + ns.outputViewOffset);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("output tab passthrough: %v", raw)
	}
}

func TestInputRouting_CtrlTabSwitchesFocus(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Start with wizard focus, send ctrl+tab → should switch to claude.
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'claude';

		var r = sendKey(s, 'ctrl+tab');
		var ns = r[0];
		if (ns.splitViewFocus !== 'claude') {
			errors.push('wizard→claude: got ' + ns.splitViewFocus);
		}

		// From claude focus, send ctrl+tab → should switch back to wizard.
		var s2 = initState('BRANCH_BUILDING');
		s2.splitViewEnabled = true;
		s2.splitViewFocus = 'claude';
		s2.splitViewTab = 'claude';

		var r2 = sendKey(s2, 'ctrl+tab');
		var ns2 = r2[0];
		if (ns2.splitViewFocus !== 'wizard') {
			errors.push('claude→wizard: got ' + ns2.splitViewFocus);
		}

		// Verify wizardState is unchanged in both cases.
		if (ns.wizardState !== 'BRANCH_BUILDING') {
			errors.push('first toggle changed wizardState to ' + ns.wizardState);
		}
		if (ns2.wizardState !== 'BRANCH_BUILDING') {
			errors.push('second toggle changed wizardState to ' + ns2.wizardState);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+tab switches focus: %v", raw)
	}
}

func TestInputRouting_CtrlOCyclesTabs(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// Basic cycle with no sessions: claude → output → claude.
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'claude';

		var r = sendKey(s, 'ctrl+o');
		if (r[0].splitViewTab !== 'output') {
			errors.push('claude→output: got ' + r[0].splitViewTab);
		}
		r = sendKey(r[0], 'ctrl+o');
		if (r[0].splitViewTab !== 'claude') {
			errors.push('output→claude: got ' + r[0].splitViewTab);
		}

		// Extended cycle with both sessions: claude → output → verify → shell → claude.
		var s2 = initState('BRANCH_BUILDING');
		s2.splitViewEnabled = true;
		s2.splitViewFocus = 'claude';
		s2.splitViewTab = 'claude';
		s2.activeVerifySession = { write: function(){}, screen: function(){return '';}, isDone: function(){return false;} };
		s2.shellSession = { write: function(){}, screen: function(){return '';}, isDone: function(){return false;} };

		var tabs = [];
		var cur = s2;
		for (var i = 0; i < 5; i++) {
			var r2 = sendKey(cur, 'ctrl+o');
			cur = r2[0];
			tabs.push(cur.splitViewTab);
		}
		var expected = 'output,verify,shell,claude,output';
		if (tabs.join(',') !== expected) {
			errors.push('extended cycle: expected [' + expected + '] got [' + tabs.join(',') + ']');
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+o cycles tabs: %v", raw)
	}
}

func TestInputRouting_WizardFocusNormalKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// With wizard focus, keys should reach wizard, not the terminal.
		// The '?' key toggles showHelp — a clear indicator the wizard handled it.
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'verify';
		s.showHelp = false;

		var r = sendKey(s, '?');
		var ns = r[0];
		if (!ns.showHelp) {
			errors.push('? key did not toggle showHelp (wizard did not handle it)');
		}
		if (ns.wizardState !== 'BRANCH_BUILDING') {
			errors.push('wizardState changed to ' + ns.wizardState);
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("wizard focus normal keys: %v", raw)
	}
}

func TestInputRouting_ReservedKeysNotForwarded(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		var written = [];

		// Set up verify tab with a mock session that records writes.
		// activeVerifySession is left falsy so ctrl+tab can toggle focus
		// (the ctrl+tab handler requires !activeVerifySession).
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'claude';
		s.splitViewTab = 'verify';

		// Ctrl+Tab is a reserved key — it should NOT be forwarded to terminal.
		// Instead, it should toggle focus (since activeVerifySession is falsy).
		var r = sendKey(s, 'ctrl+tab');
		var ns = r[0];

		if (ns.splitViewFocus !== 'wizard') {
			errors.push('ctrl+tab did not toggle focus: got ' + ns.splitViewFocus);
		}
		if (ns.wizardState !== 'BRANCH_BUILDING') {
			errors.push('wizardState changed to ' + ns.wizardState);
		}

		// Also verify Ctrl+O is reserved: it should cycle tabs, not forward.
		var s2 = initState('BRANCH_BUILDING');
		s2.splitViewEnabled = true;
		s2.splitViewFocus = 'claude';
		s2.splitViewTab = 'verify';
		s2.activeVerifySession = {
			write: function(b) { written.push(b); },
			screen: function() { return ''; },
			isDone: function() { return false; }
		};

		var r2 = sendKey(s2, 'ctrl+o');
		if (r2[0].splitViewTab === 'verify') {
			errors.push('ctrl+o did not cycle tab (still verify)');
		}
		if (written.length > 0) {
			errors.push('reserved key ctrl+o was forwarded to session: ' + JSON.stringify(written));
		}

		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("reserved keys not forwarded: %v", raw)
	}
}
