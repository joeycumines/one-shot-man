package command

import (
	"testing"
)

//  T45: Auto-attach Claude pane when Claude spawns during wizard
// ---------------------------------------------------------------------------

// TestChunk16_T45_InitStateHasAutoAttachFields verifies that new T45 state
// fields are initialized correctly.
func TestChunk16_T45_InitStateHasAutoAttachFields(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		return {
			claudeAutoAttached: s.claudeAutoAttached,
			claudeManuallyDismissed: s.claudeManuallyDismissed,
			claudeAutoAttachNotif: s.claudeAutoAttachNotif,
			claudeAutoAttachNotifAt: s.claudeAutoAttachNotifAt
		};
	})()`)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	m := raw.(map[string]any)
	if m["claudeAutoAttached"] != false {
		t.Errorf("claudeAutoAttached = %v, want false", m["claudeAutoAttached"])
	}
	if m["claudeManuallyDismissed"] != false {
		t.Errorf("claudeManuallyDismissed = %v, want false", m["claudeManuallyDismissed"])
	}
	if m["claudeAutoAttachNotif"] != "" {
		t.Errorf("claudeAutoAttachNotif = %v, want empty", m["claudeAutoAttachNotif"])
	}
	if numVal(m["claudeAutoAttachNotifAt"]) != 0 {
		t.Errorf("claudeAutoAttachNotifAt = %v, want 0", m["claudeAutoAttachNotifAt"])
	}
}

// TestChunk16_T45_AutoAttachOnAutoPoll verifies that handleAutoSplitPoll
// auto-enables split-view when tuiMux has a child attached.
func TestChunk16_T45_AutoAttachOnAutoPoll(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock tuiMux with hasChild() returning true (Claude attached).
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 500; }
		};

		// Mock alive Claude executor — prevent crash detection.
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return true; }
			}
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.height = 30;
		s.lastClaudeHealthCheckMs = Date.now(); // skip health check

		// Send auto-poll tick.
		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var ns = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		if (!ns.splitViewEnabled) errors.push('splitViewEnabled should be true');
		if (ns.splitViewFocus !== 'wizard') errors.push('focus: got ' + ns.splitViewFocus + ', want wizard');
		if (ns.splitViewTab !== 'claude') errors.push('tab: got ' + ns.splitViewTab + ', want claude');
		if (!ns.claudeAutoAttached) errors.push('claudeAutoAttached should be true');
		if (!ns.claudeAutoAttachNotif) errors.push('notification should be set');
		if (ns.claudeAutoAttachNotifAt === 0) errors.push('notifAt should be > 0');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-attach on auto-poll: %v", raw)
	}
}

// TestChunk16_T45_AutoAttachSkippedSmallTerminal verifies that auto-attach
// does NOT trigger when terminal height < 12 lines.
func TestChunk16_T45_AutoAttachSkippedSmallTerminal(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 500; }
		};
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return true; } }
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.height = 10; // too small
		s.lastClaudeHealthCheckMs = Date.now();

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var ns = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (ns.splitViewEnabled) return 'FAIL: splitViewEnabled should be false for small terminal';
		if (ns.claudeAutoAttached) return 'FAIL: claudeAutoAttached should be false';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-attach small terminal: %v", raw)
	}
}

// TestChunk16_T45_ManualDismissPreventsAutoReopen verifies that Ctrl+L close
// sets claudeManuallyDismissed, preventing auto-attach from re-opening.
func TestChunk16_T45_ManualDismissPreventsAutoReopen(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;

		// User presses Ctrl+L to close split-view.
		var r = sendKey(s, 'ctrl+l');
		s = r[0];

		if (s.splitViewEnabled) return 'FAIL: splitViewEnabled should be false after Ctrl+L';
		if (!s.claudeManuallyDismissed) return 'FAIL: claudeManuallyDismissed should be true';

		// Now simulate auto-poll with Claude available — should NOT auto-open.
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 500; }
		};
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return true; } }
		};

		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.height = 30;
		s.lastClaudeHealthCheckMs = Date.now();

		r = update({type: 'Tick', id: 'auto-poll'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.splitViewEnabled) return 'FAIL: splitViewEnabled should still be false — manual dismiss respected';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("manual dismiss prevents auto-reopen: %v", raw)
	}
}

// TestChunk16_T45_CtrlLReopenClearsDismiss verifies that pressing Ctrl+L to
// re-open split-view clears the claudeManuallyDismissed flag.
func TestChunk16_T45_CtrlLReopenClearsDismiss(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;

		// Close with Ctrl+L.
		var r = sendKey(s, 'ctrl+l');
		s = r[0];
		if (!s.claudeManuallyDismissed) return 'FAIL: should be dismissed after close';

		// Re-open with Ctrl+L.
		r = sendKey(s, 'ctrl+l');
		s = r[0];
		if (!s.splitViewEnabled) return 'FAIL: splitViewEnabled should be true after re-open';
		if (s.claudeManuallyDismissed) return 'FAIL: claudeManuallyDismissed should be cleared on re-open';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("Ctrl+L reopen clears dismiss: %v", raw)
	}
}

// TestChunk16_T45_CrashCloseSplitView verifies that when Claude crashes during
// auto-split, the split-view is auto-closed with a notification.
func TestChunk16_T45_CrashCloseSplitView(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Mock dead Claude.
		globalThis.prSplit._state.claudeExecutor = {
			handle: {
				isAlive: function() { return false; },
				receive: function() { return 'segfault'; }
			},
			captureDiagnostic: function() { return 'crash dump'; }
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.splitViewEnabled = true;
		s.claudeAutoAttached = true;
		s.lastClaudeHealthCheckMs = 0; // Force health check.

		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		var ns = r[0];

		var errors = [];
		if (ns.splitViewEnabled) errors.push('splitViewEnabled should be false after crash');
		if (!ns.claudeAutoAttachNotif) errors.push('notification should be set');
		if (ns.claudeAutoAttachNotif.indexOf('crashed') < 0) errors.push('notif should mention crash: ' + ns.claudeAutoAttachNotif);
		if (ns.wizardState !== 'ERROR_RESOLUTION') errors.push('wizardState: got ' + ns.wizardState);

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash closes split-view: %v", raw)
	}
}

// TestChunk16_T45_ClaudeStatusBadgeOpensView verifies that clicking the
// claude-status zone mark opens split-view when Claude is running.
func TestChunk16_T45_ClaudeStatusBadgeOpensView(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 500; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = false;
		s.claudeManuallyDismissed = true; // Previously dismissed.

		// Click claude-status zone.
		var restore = mockZoneHit('claude-status');
		var r = update({type: 'Mouse', action: 'press', button: 'left', x: 10, y: 10}, s);
		restore();
		var ns = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		if (!ns.splitViewEnabled) errors.push('splitViewEnabled should be true');
		if (ns.claudeManuallyDismissed) errors.push('claudeManuallyDismissed should be cleared');
		if (ns.splitViewFocus !== 'wizard') errors.push('focus: got ' + ns.splitViewFocus + ', want wizard');
		if (ns.splitViewTab !== 'claude') errors.push('tab: got ' + ns.splitViewTab + ', want claude');

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("claude-status badge opens view: %v", raw)
	}
}

// TestChunk16_T45_ExitAutoClosesSplitView verifies that pollClaudeScreenshot
// auto-closes the split-view when Claude exits (hasChild becomes false) and
// the auto-split pipeline is no longer running.
func TestChunk16_T45_ExitAutoClosesSplitView(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		// Mock tuiMux with no child (Claude exited).
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.claudeAutoAttached = true;
		s.autoSplitRunning = false; // Pipeline done.

		// Send screenshot poll tick.
		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var ns = r[0];
		var cmd = r[1];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		if (ns.splitViewEnabled) errors.push('splitViewEnabled should be false');
		if (!ns.claudeAutoAttachNotif) errors.push('notification should be set');
		if (ns.claudeAutoAttachNotif.indexOf('ended') < 0) errors.push('notif should mention ended: ' + ns.claudeAutoAttachNotif);
		// T028: pollClaudeScreenshot now returns a dismiss-attach-notif tick
		// to auto-dismiss the notification. Screenshot polling still stops
		// (no further claude-screenshot ticks are scheduled).

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("exit auto-closes split-view: %v", raw)
	}
}

// TestChunk16_T45_NoAutoCloseWhenPipelineRunning verifies that
// pollClaudeScreenshot does NOT auto-close when the pipeline is still running
// (child may re-attach after restart).
func TestChunk16_T45_NoAutoCloseWhenPipelineRunning(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return false; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; }
		};

		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.claudeAutoAttached = true;
		s.autoSplitRunning = true; // Pipeline still running.

		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		var ns = r[0];
		var cmd = r[1];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (!ns.splitViewEnabled) return 'FAIL: splitViewEnabled should stay true while pipeline running';
		if (cmd === null) return 'FAIL: should continue polling';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no auto-close when pipeline running: %v", raw)
	}
}

// TestChunk16_T45_NotificationRenderedInStatusBar verifies the transient
// notification banner appears in the status bar output.
func TestChunk16_T45_NotificationRenderedInStatusBar(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.width = 80;
		s.claudeAutoAttachNotif = 'Claude connected';
		s.claudeAutoAttachNotifAt = Date.now();

		var statusBar = globalThis.prSplit._renderStatusBar(s);
		if (statusBar.indexOf('Claude connected') < 0) {
			return 'FAIL: notification not found in status bar';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("notification in status bar: %v", raw)
	}
}

// TestChunk16_T45_NotificationAutoExpires verifies that the notification
// auto-clears after 5 seconds.
func TestChunk16_T45_NotificationAutoExpires(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		s.width = 80;
		s.claudeAutoAttachNotif = 'Claude connected';
		s.claudeAutoAttachNotifAt = Date.now() - 6000; // 6s ago — expired

		var statusBar = globalThis.prSplit._renderStatusBar(s);
		if (statusBar.indexOf('Claude connected') >= 0) {
			return 'FAIL: expired notification should not be shown';
		}
		// T028: State clearing is handled by dismiss-attach-notif tick, not
		// by the view function. Send the tick to verify the dismiss path.
		var r = update({type: 'Tick', id: 'dismiss-attach-notif'}, s);
		s = r[0];
		if (s.claudeAutoAttachNotif !== '') {
			return 'FAIL: claudeAutoAttachNotif should be cleared, got: ' + s.claudeAutoAttachNotif;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("notification auto-expires: %v", raw)
	}
}

// TestChunk16_T45_AutoAttachFiresOnlyOnce verifies that auto-attach does not
// re-trigger after claudeAutoAttached is already true.
func TestChunk16_T45_AutoAttachFiresOnlyOnce(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return ''; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 500; }
		};
		globalThis.prSplit._state.claudeExecutor = {
			handle: { isAlive: function() { return true; } }
		};

		var s = initState('BRANCH_BUILDING');
		s.autoSplitRunning = true;
		s.isProcessing = true;
		s.height = 30;
		s.lastClaudeHealthCheckMs = Date.now();

		// First poll — should auto-attach.
		var r = update({type: 'Tick', id: 'auto-poll'}, s);
		s = r[0];
		if (!s.claudeAutoAttached) {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			return 'FAIL: first poll should auto-attach';
		}

		// Manually close.
		s.splitViewEnabled = false;

		// Second poll — should NOT re-attach (claudeAutoAttached is true).
		s.lastClaudeHealthCheckMs = Date.now();
		r = update({type: 'Tick', id: 'auto-poll'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.splitViewEnabled) return 'FAIL: should NOT re-attach after first trigger';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-attach fires only once: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T46: Claude Inline Feedback — Question Detection + Inline Prompt
// ---------------------------------------------------------------------------

// TestChunk16_T46_InitStateHasQuestionFields verifies that the new T46
// state fields are initialised with correct zero-values.
func TestChunk16_T46_InitStateHasQuestionFields(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('CONFIG');
		var errors = [];
		if (s.claudeQuestionDetected !== false) errors.push('claudeQuestionDetected: ' + s.claudeQuestionDetected);
		if (s.claudeQuestionLine !== '') errors.push('claudeQuestionLine: ' + JSON.stringify(s.claudeQuestionLine));
		if (s.claudeQuestionInputText !== '') errors.push('claudeQuestionInputText: ' + JSON.stringify(s.claudeQuestionInputText));
		if (s.claudeQuestionInputActive !== false) errors.push('claudeQuestionInputActive: ' + s.claudeQuestionInputActive);
		if (!Array.isArray(s.claudeConversations)) errors.push('claudeConversations not array');
		if (s.claudeConversations.length !== 0) errors.push('claudeConversations not empty');
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("T46 init state: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_ConfirmPatterns tests that explicit
// confirmation prompts (y/n, proceed?, [yes/no]) are detected.
func TestChunk16_T46_DetectClaudeQuestion_ConfirmPatterns(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;
		var idle = 3000; // well above threshold

		var cases = [
			{ name: 'y/n parens',     text: 'Do you want to continue? (y/n)', want: true },
			{ name: 'Y/N brackets',   text: 'Overwrite existing file? [Y/N]', want: true },
			{ name: 'yes/no parens',  text: 'Are you sure? (yes/no)',         want: true },
			{ name: 'yes/no brackets', text: 'Delete all files? [yes/no]',    want: true },
			{ name: 'proceed?',       text: 'Ready to proceed?',             want: true },
			{ name: 'continue?',      text: 'Should we continue?',           want: true },
			{ name: 'confirm?',       text: 'Please confirm?',              want: true },
			{ name: 'overwrite?',     text: 'Do you want to overwrite?',     want: true },
			{ name: 'replace?',       text: 'Replace the existing branch?',  want: true },
			{ name: 'accept?',        text: 'Do you accept?',               want: true },
			{ name: 'approve?',       text: 'Please approve?',              want: true },
			{ name: 'delete?',        text: 'Should I delete?',             want: true }
		];

		var errors = [];
		for (var i = 0; i < cases.length; i++) {
			var c = cases[i];
			var r = detect(c.text, idle);
			if (r.detected !== c.want) {
				errors.push(c.name + ': got detected=' + r.detected + ', want=' + c.want);
			}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm patterns: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_QuestionOpeners tests conversational
// question opener patterns like "Do you", "Should I", etc.
func TestChunk16_T46_DetectClaudeQuestion_QuestionOpeners(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;
		var idle = 3000;

		var cases = [
			{ name: 'Do you',        text: 'Do you want me to split by directory?', want: true },
			{ name: 'Would you',     text: 'Would you prefer separate PRs?',        want: true },
			{ name: 'Should I',      text: 'Should I proceed with the plan?',        want: true },
			{ name: 'Can you',       text: 'Can you clarify the branch name?',       want: true },
			{ name: 'Could you',     text: 'Could you provide more context?',        want: true },
			{ name: 'Is this',       text: 'Is this the correct approach?',          want: true },
			{ name: 'Are you',       text: 'Are you sure about this change?',        want: true },
			{ name: 'Shall I',       text: 'Shall I create the branches now?',       want: true },
			{ name: 'Want me to',    text: 'Want me to include tests?',              want: true },
			{ name: 'May I',         text: 'May I modify the config file?',          want: true },
			{ name: 'Please confirm', text: 'Please confirm the split strategy.',   want: true },
			{ name: 'Please clarify', text: 'Please clarify which files to include.', want: true },
			{ name: 'What would you', text: 'What would you like me to do next?',   want: true },
			{ name: 'How would you', text: 'How would you like the branches named?', want: true },
			{ name: 'Where should',   text: 'Where should I put the tests?',        want: true },
			{ name: 'indent opener',  text: '  Do you want to continue?',           want: true }
		];

		var errors = [];
		for (var i = 0; i < cases.length; i++) {
			var c = cases[i];
			var r = detect(c.text, idle);
			if (r.detected !== c.want) {
				errors.push(c.name + ': got detected=' + r.detected + ', want=' + c.want);
			}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("question openers: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_GeneralQuestion tests lines ending
// with "?" that are 10+ chars.
func TestChunk16_T46_DetectClaudeQuestion_GeneralQuestion(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;
		var idle = 3000;

		var cases = [
			{ name: 'long question',     text: 'What directory should the output go to?',  want: true },
			{ name: 'short non-question', text: 'Really?',                                  want: false },
			{ name: '11 chars total',     text: '1234567890?',                              want: true },
			{ name: '10 chars total',     text: '123456789?',                               want: true },
			{ name: '9 chars below threshold', text: '12345678?',                            want: false },
			{ name: 'just ? mark',        text: '?',                                        want: false },
			{ name: 'empty',              text: '',                                          want: false }
		];

		var errors = [];
		for (var i = 0; i < cases.length; i++) {
			var c = cases[i];
			var r = detect(c.text, idle);
			if (r.detected !== c.want) {
				errors.push(c.name + ': got detected=' + r.detected + ', want=' + c.want);
			}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("general question: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_IdleGuard verifies that detection
// does not trigger when idle time is below the threshold.
func TestChunk16_T46_DetectClaudeQuestion_IdleGuard(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;

		// Should NOT detect — idle time too low (Claude still streaming).
		var r1 = detect('Do you want to continue? (y/n)', 500);
		if (r1.detected) return 'FAIL: detected with idle=500';

		var r2 = detect('Do you want to continue? (y/n)', 1999);
		if (r2.detected) return 'FAIL: detected with idle=1999';

		var r3 = detect('Do you want to continue? (y/n)', 0);
		if (r3.detected) return 'FAIL: detected with idle=0';

		// Should detect — idle at exactly threshold.
		var r4 = detect('Do you want to continue? (y/n)', 2000);
		if (!r4.detected) return 'FAIL: NOT detected with idle=2000';

		// Should detect — idle well above threshold.
		var r5 = detect('Do you want to continue? (y/n)', 10000);
		if (!r5.detected) return 'FAIL: NOT detected with idle=10000';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("idle guard: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_NegativeCases verifies that normal
// Claude output (non-questions) is NOT flagged.
func TestChunk16_T46_DetectClaudeQuestion_NegativeCases(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;
		var idle = 5000;

		var cases = [
			{ name: 'status line',     text: 'Analyzing 42 files for split candidates...' },
			{ name: 'progress bar',    text: '████████████████░░░░ 80%' },
			{ name: 'branch creation', text: 'Creating branch: feature/auth-module' },
			{ name: 'diff stat',       text: ' 15 files changed, 423 insertions(+), 87 deletions(-)' },
			{ name: 'commit msg',      text: 'feat(auth): add OAuth2 token refresh logic' },
			{ name: 'empty text',      text: '' },
			{ name: 'null text',       text: null },
			{ name: 'short ?',         text: '?' },
			{ name: 'just spaces',     text: '     ' },
			{ name: 'code line',       text: 'func (s *Server) Start() error {' },
			{ name: 'log output',      text: '2024-01-15 10:23:45 INFO: split plan generated successfully' }
		];

		var errors = [];
		for (var i = 0; i < cases.length; i++) {
			var c = cases[i];
			var r = detect(c.text, idle);
			if (r.detected) {
				errors.push(c.name + ': false positive (detected=true)');
			}
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("negative cases: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_MultilineBottomScan verifies that
// the function scans the last 15 lines and finds questions near the bottom.
func TestChunk16_T46_DetectClaudeQuestion_MultilineBottomScan(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;
		var idle = 3000;

		// Build multi-line output: 20 non-question lines, then a question.
		var lines = [];
		for (var i = 0; i < 20; i++) {
			lines.push('Processing file ' + i + '...');
		}
		lines.push('Would you like me to split these into separate PRs?');
		var text = lines.join('\n');

		var r = detect(text, idle);
		if (!r.detected) return 'FAIL: did not detect question at bottom of 21-line output';
		if (r.line.indexOf('Would you') < 0) return 'FAIL: wrong line: ' + r.line;

		// Question in middle (beyond 15-line scan window from bottom).
		var lines2 = [];
		lines2.push('Do you want to proceed?');
		for (var j = 0; j < 20; j++) {
			lines2.push('Still processing file ' + j + '...');
		}
		var text2 = lines2.join('\n');
		var r2 = detect(text2, idle);
		if (r2.detected) return 'FAIL: should NOT detect question beyond 15-line scan window';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("multiline bottom scan: %v", raw)
	}
}

// TestChunk16_T46_DetectClaudeQuestion_TrailingBlanks verifies that trailing
// blank lines (from VTerm padding) are stripped before scanning.
func TestChunk16_T46_DetectClaudeQuestion_TrailingBlanks(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var detect = globalThis.prSplit._detectClaudeQuestion;
		var idle = 3000;

		// Question followed by 10 blank lines (simulating VTerm padding).
		var text = 'Would you like to proceed?\n\n\n\n\n\n\n\n\n\n';
		var r = detect(text, idle);
		if (!r.detected) return 'FAIL: did not detect question with trailing blanks';
		if (r.line !== 'Would you like to proceed?') return 'FAIL: wrong line: ' + r.line;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("trailing blanks: %v", raw)
	}
}

// TestChunk16_T46_PollDetectsQuestion verifies that pollClaudeScreenshot
// sets claudeQuestionDetected when a question is found.
func TestChunk16_T46_PollDetectsQuestion(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return 'Do you want to continue? (y/n)'; },
			childScreen: function() { return 'screen-data'; },
			lastActivityMs: function() { return 5000; } // idle 5s
		};

		var s = initState('PLAN_GENERATION');
		s.splitViewEnabled = true;
		s.isProcessing = true;
		s.claudeLastQuestionCheckMs = 0; // ensure detection runs

		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		if (!s.claudeQuestionDetected) errors.push('claudeQuestionDetected should be true');
		if (s.claudeQuestionLine.indexOf('(y/n)') < 0) errors.push('claudeQuestionLine: ' + JSON.stringify(s.claudeQuestionLine));
		if (s.claudeQuestionInputActive) errors.push('claudeQuestionInputActive should be false initially');
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("poll detects question: %v", raw)
	}
}

// TestChunk16_T46_PollThrottlesDetection verifies that detection is throttled
// to every 2 seconds.
func TestChunk16_T46_PollThrottlesDetection(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return 'Would you like to proceed?'; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 5000; }
		};

		var s = initState('PLAN_GENERATION');
		s.splitViewEnabled = true;
		s.isProcessing = true;
		s.claudeLastQuestionCheckMs = Date.now(); // just checked

		// First poll — should NOT detect because throttle hasn't expired.
		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.claudeQuestionDetected) return 'FAIL: should be throttled (detected=true)';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("poll throttles detection: %v", raw)
	}
}

// TestChunk16_T46_PollNonProcessingSkipsDetection verifies that detection
// does not run when isProcessing is false.
func TestChunk16_T46_PollNonProcessingSkipsDetection(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return 'Do you want to continue? (y/n)'; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 5000; }
		};

		var s = initState('PLAN_REVIEW');
		s.splitViewEnabled = true;
		s.isProcessing = false;  // NOT processing
		s.claudeLastQuestionCheckMs = 0;

		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.claudeQuestionDetected) return 'FAIL: should not detect when not processing';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("non-processing skips detection: %v", raw)
	}
}

// TestChunk16_T46_PollAutoDismissesOnResume verifies that when Claude
// starts streaming again (idle < threshold), the question is auto-dismissed
// (only if user isn't typing).
func TestChunk16_T46_PollAutoDismissesOnResume(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return 'Working on it...'; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 100; } // Claude is streaming
		};

		var s = initState('PLAN_GENERATION');
		s.splitViewEnabled = true;
		s.isProcessing = true;
		s.claudeQuestionDetected = true;   // question was previously detected
		s.claudeQuestionLine = 'old question';
		s.claudeLastQuestionCheckMs = 0;

		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.claudeQuestionDetected) return 'FAIL: should auto-dismiss when Claude resumes (streaming = idle < 2s)';
		if (s.claudeQuestionLine !== '') return 'FAIL: line not cleared: ' + s.claudeQuestionLine;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("auto-dismiss on resume: %v", raw)
	}
}

// TestChunk16_T46_PollDoesNotDismissWhileTyping verifies that the question
// is NOT auto-dismissed when the user is actively typing a response.
func TestChunk16_T46_PollDoesNotDismissWhileTyping(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			hasChild: function() { return true; },
			screenshot: function() { return 'Working on it...'; },
			childScreen: function() { return ''; },
			lastActivityMs: function() { return 100; }
		};

		var s = initState('PLAN_GENERATION');
		s.splitViewEnabled = true;
		s.isProcessing = true;
		s.claudeQuestionDetected = true;
		s.claudeQuestionLine = 'Please confirm';
		s.claudeQuestionInputActive = true;  // user is typing!
		s.claudeQuestionInputText = 'ye';
		s.claudeLastQuestionCheckMs = 0;

		var r = update({type: 'Tick', id: 'claude-screenshot'}, s);
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (!s.claudeQuestionDetected) return 'FAIL: should NOT dismiss while user is typing';
		if (s.claudeQuestionInputText !== 'ye') return 'FAIL: input text cleared';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("does not dismiss while typing: %v", raw)
	}
}

// TestChunk16_T46_PrintableCharActivatesInput verifies that typing a
// printable character activates the inline question input.
func TestChunk16_T46_PrintableCharActivatesInput(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionLine = 'Do you want to proceed?';
		s.claudeQuestionInputActive = false;

		var r = sendKey(s, 'y');
		s = r[0];

		var errors = [];
		if (!s.claudeQuestionInputActive) errors.push('should activate input');
		if (s.claudeQuestionInputText !== 'y') errors.push('inputText: ' + JSON.stringify(s.claudeQuestionInputText));
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("printable char activates input: %v", raw)
	}
}

// TestChunk16_T46_EscDismissesQuestion verifies that pressing Escape
// dismisses the question prompt regardless of input state.
func TestChunk16_T46_EscDismissesQuestion(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		// Test 1: Esc when input active.
		var s1 = initState('PLAN_GENERATION');
		s1.claudeQuestionDetected = true;
		s1.claudeQuestionInputActive = true;
		s1.claudeQuestionInputText = 'partial';
		s1.claudeQuestionLine = 'question';

		var r1 = sendKey(s1, 'esc');
		s1 = r1[0];
		if (s1.claudeQuestionDetected) return 'FAIL 1: detected not cleared';
		if (s1.claudeQuestionInputActive) return 'FAIL 1: inputActive not cleared';
		if (s1.claudeQuestionInputText !== '') return 'FAIL 1: inputText not cleared';

		// Test 2: Esc when input NOT active (just detected).
		var s2 = initState('PLAN_GENERATION');
		s2.claudeQuestionDetected = true;
		s2.claudeQuestionInputActive = false;
		s2.claudeQuestionLine = 'question2';

		var r2 = sendKey(s2, 'esc');
		s2 = r2[0];
		if (s2.claudeQuestionDetected) return 'FAIL 2: detected not cleared';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("esc dismisses question: %v", raw)
	}
}

// TestChunk16_T46_BackspaceInInput verifies that backspace removes the last
// character from the input buffer.
func TestChunk16_T46_BackspaceInInput(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'hello';

		var r = sendKey(s, 'backspace');
		s = r[0];
		if (s.claudeQuestionInputText !== 'hell') return 'FAIL: got ' + JSON.stringify(s.claudeQuestionInputText);

		// Backspace on empty.
		s.claudeQuestionInputText = '';
		r = sendKey(s, 'backspace');
		s = r[0];
		if (s.claudeQuestionInputText !== '') return 'FAIL: not empty after backspace on empty';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("backspace in input: %v", raw)
	}
}

// TestChunk16_T46_CtrlUClearsInput verifies that Ctrl+U clears the entire
// input buffer.
func TestChunk16_T46_CtrlUClearsInput(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'some long response text';

		var r = sendKey(s, 'ctrl+u');
		s = r[0];
		if (s.claudeQuestionInputText !== '') return 'FAIL: got ' + JSON.stringify(s.claudeQuestionInputText);
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+u clears input: %v", raw)
	}
}

// TestChunk16_T46_EnterSendsToClaudePTY verifies that pressing Enter sends
// the response to Claude's PTY via writeToChild and records the conversation.
func TestChunk16_T46_EnterSendsToClaudePTY(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var sentData = [];
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			writeToChild: function(data) { sentData.push(data); },
			hasChild: function() { return true; }
		};

		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'yes';
		s.claudeQuestionLine = 'Do you want to proceed?';

		var r = sendKey(s, 'enter');
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		var errors = [];
		// Verify PTY write.
		if (sentData.length !== 1) errors.push('sentData.length: ' + sentData.length);
		if (sentData[0] !== 'yes\r') errors.push('sentData[0]: ' + JSON.stringify(sentData[0]));
		// Verify state cleared.
		if (s.claudeQuestionDetected) errors.push('detected not cleared');
		if (s.claudeQuestionInputActive) errors.push('inputActive not cleared');
		if (s.claudeQuestionInputText !== '') errors.push('inputText not cleared');
		// Verify conversation history.
		if (s.claudeConversations.length !== 1) errors.push('conversations.length: ' + s.claudeConversations.length);
		if (s.claudeConversations.length === 1) {
			var conv = s.claudeConversations[0];
			if (conv.question !== 'Do you want to proceed?') errors.push('conv.question: ' + conv.question);
			if (conv.answer !== 'yes') errors.push('conv.answer: ' + conv.answer);
			if (typeof conv.ts !== 'number' || conv.ts === 0) errors.push('conv.ts invalid: ' + conv.ts);
		}
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("enter sends to Claude PTY: %v", raw)
	}
}

// TestChunk16_T46_EnterEmptyDoesNotSend verifies that pressing Enter with
// empty or whitespace-only input does NOT send anything.
func TestChunk16_T46_EnterEmptyDoesNotSend(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var sentData = [];
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			writeToChild: function(data) { sentData.push(data); },
			hasChild: function() { return true; }
		};

		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = '   '; // whitespace only

		var r = sendKey(s, 'enter');
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (sentData.length !== 0) return 'FAIL: should not send whitespace-only (' + sentData.length + ' sends)';
		if (s.claudeConversations.length !== 0) return 'FAIL: should not record empty convo';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("enter empty does not send: %v", raw)
	}
}

// TestChunk16_T46_ConversationHistoryAccumulates verifies that multiple
// Q&A exchanges are recorded in claudeConversations.
func TestChunk16_T46_ConversationHistoryAccumulates(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			writeToChild: function(data) {},
			hasChild: function() { return true; }
		};

		var s = initState('PLAN_GENERATION');

		// First Q&A.
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'yes';
		s.claudeQuestionLine = 'Q1?';
		var r1 = sendKey(s, 'enter');
		s = r1[0];

		// Second Q&A.
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'no';
		s.claudeQuestionLine = 'Q2?';
		var r2 = sendKey(s, 'enter');
		s = r2[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.claudeConversations.length !== 2) return 'FAIL: got ' + s.claudeConversations.length + ' conversations';
		if (s.claudeConversations[0].question !== 'Q1?') return 'FAIL: first Q: ' + s.claudeConversations[0].question;
		if (s.claudeConversations[1].answer !== 'no') return 'FAIL: second A: ' + s.claudeConversations[1].answer;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("conversation history accumulates: %v", raw)
	}
}

// TestChunk16_T46_MouseClickActivatesInput verifies that clicking the
// claude-question-input zone activates the input field.
func TestChunk16_T46_MouseClickActivatesInput(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = false;
		s.claudeQuestionLine = 'Proceed?';

		mockZoneHit('claude-question-input');
		var r = update({
			type: 'Mouse', action: 'press', button: 'left',
			x: 10, y: 5, isWheel: false
		}, s);
		s = r[0];

		if (!s.claudeQuestionInputActive) return 'FAIL: input should be active after click';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("mouse click activates input: %v", raw)
	}
}

// TestChunk16_T46_InputInterceptsAllKeys verifies that when input is active,
// non-printable keys like Tab, arrows etc. are consumed (not leaked).
func TestChunk16_T46_InputInterceptsAllKeys(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'test';
		s.focusIndex = 0;

		// Tab, arrow, etc. should NOT change focusIndex.
		var origFocus = s.focusIndex;
		var keysToTest = ['tab', 'up', 'down', 'left', 'right', 'pgup', 'pgdown'];
		for (var i = 0; i < keysToTest.length; i++) {
			var r = sendKey(s, keysToTest[i]);
			s = r[0];
		}

		if (s.focusIndex !== origFocus) return 'FAIL: focusIndex changed from ' + origFocus + ' to ' + s.focusIndex;
		if (s.claudeQuestionInputText !== 'test') return 'FAIL: inputText changed: ' + s.claudeQuestionInputText;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("input intercepts all keys: %v", raw)
	}
}

// TestChunk16_T46_ViewAnalysisShowsQuestionPrompt verifies that
// viewAnalysisScreen renders the inline question prompt when a question
// is detected.
func TestChunk16_T46_ViewAnalysisShowsQuestionPrompt(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionLine = 'Would you like to split by directory?';
		s.width = 80;
		s.height = 24;

		var view = globalThis.prSplit._viewForState(s);
		if (!view) return 'FAIL: no view rendered';
		if (view.indexOf('Claude asks') < 0) return 'FAIL: missing "Claude asks" in view';
		if (view.indexOf('split by directory') < 0) return 'FAIL: missing question text in view';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("view analysis shows question prompt: %v", raw)
	}
}

// TestChunk16_T46_ViewNoQuestionWhenNotDetected verifies that no question
// prompt appears when claudeQuestionDetected is false.
func TestChunk16_T46_ViewNoQuestionWhenNotDetected(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = false;
		s.width = 80;
		s.height = 24;

		var view = globalThis.prSplit._viewForState(s);
		if (!view) return 'FAIL: no view rendered';
		if (view.indexOf('Claude asks') >= 0) return 'FAIL: question prompt should NOT appear';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("view no question when not detected: %v", raw)
	}
}

// TestChunk16_T46_EnterResetsThrottleTimestamp verifies that after sending
// a response, the detection throttle timestamp is updated to prevent
// immediate re-detection of the same question.
func TestChunk16_T46_EnterResetsThrottleTimestamp(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			writeToChild: function(data) {},
			hasChild: function() { return true; }
		};

		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'yes';
		s.claudeQuestionLine = 'Proceed?';
		s.claudeLastQuestionCheckMs = 0;

		var before = Date.now();
		var r = sendKey(s, 'enter');
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.claudeLastQuestionCheckMs < before) return 'FAIL: throttle not reset: ' + s.claudeLastQuestionCheckMs;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("enter resets throttle timestamp: %v", raw)
	}
}

// TestChunk16_T46_ViewExecutionShowsQuestionPrompt verifies that
// viewExecutionScreen renders the inline question prompt.
func TestChunk16_T46_ViewExecutionShowsQuestionPrompt(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		setupPlanCache();
		var s = initState('BRANCH_BUILDING');
		s.claudeQuestionDetected = true;
		s.claudeQuestionLine = 'Should I create the branch now?';
		s.width = 80;
		s.height = 24;

		var view = globalThis.prSplit._viewForState(s);
		if (!view) return 'FAIL: no view rendered';
		if (view.indexOf('Claude asks') < 0) return 'FAIL: missing "Claude asks" in execution view';
		if (view.indexOf('create the branch') < 0) return 'FAIL: missing question text in view';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("view execution shows question prompt: %v", raw)
	}
}

// TestChunk16_T46_WizardTransitionClearsQuestionState verifies that
// transitioning wizard state clears orphaned T46 question state.
func TestChunk16_T46_WizardTransitionClearsQuestionState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.claudeQuestionDetected = true;
		s.claudeQuestionLine = 'Question?';
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'partial';

		// Simulate wizard transition by changing wizardState.
		s.wizardState = 'PLAN_REVIEW';

		// Trigger the transition detection by sending any message.
		var r = update({type: 'WindowSize', width: 80, height: 24}, s);
		// Now send a key — the transition detection runs on next update.
		r = update({type: 'Key', key: 'tab'}, r[0]);
		var ns = r[0];

		var errors = [];
		if (ns.claudeQuestionDetected) errors.push('detected not cleared');
		if (ns.claudeQuestionInputActive) errors.push('inputActive not cleared');
		if (ns.claudeQuestionInputText !== '') errors.push('inputText not cleared: ' + JSON.stringify(ns.claudeQuestionInputText));
		if (ns.claudeQuestionLine !== '') errors.push('line not cleared');
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("wizard transition clears question state: %v", raw)
	}
}

// TestChunk16_T46_CtrlLClearsQuestionState verifies that Ctrl+L close
// clears T46 inline question state.
func TestChunk16_T46_CtrlLClearsQuestionState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.splitViewEnabled = true;
		s.claudeQuestionDetected = true;
		s.claudeQuestionLine = 'Proceed?';
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'ye';

		// Ctrl+L should close split-view AND clear question state.
		var r = sendKey(s, 'ctrl+l');
		var ns = r[0];

		var errors = [];
		if (ns.splitViewEnabled) errors.push('splitViewEnabled not false');
		if (ns.claudeQuestionDetected) errors.push('detected not cleared');
		if (ns.claudeQuestionInputActive) errors.push('inputActive not cleared');
		if (ns.claudeQuestionInputText !== '') errors.push('inputText not cleared');
		if (ns.claudeQuestionLine !== '') errors.push('line not cleared');
		return errors.length > 0 ? 'FAIL: ' + errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+l clears question state: %v", raw)
	}
}

// TestChunk16_T46_CtrlLPassesThroughInputInterceptor verifies that Ctrl+L
// is NOT consumed by the T46 input interceptor — it passes through to the
// split-view toggle handler.
func TestChunk16_T46_CtrlLPassesThroughInputInterceptor(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_GENERATION');
		s.splitViewEnabled = true;
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'hello';

		// Ctrl+L should NOT be consumed as text input — split-view should toggle.
		var r = sendKey(s, 'ctrl+l');
		var ns = r[0];

		if (ns.splitViewEnabled) return 'FAIL: split-view should be closed by Ctrl+L';
		if (ns.claudeQuestionInputText === 'hello' + 'ctrl+l') return 'FAIL: Ctrl+L was appended as text';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+l passes through input interceptor: %v", raw)
	}
}

// TestChunk16_T46_ConversationHistoryCap verifies that claudeConversations
// is capped at 100 entries (trimmed to 80 on overflow).
func TestChunk16_T46_ConversationHistoryCap(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var savedMux = (typeof tuiMux !== 'undefined') ? tuiMux : undefined;
		globalThis.tuiMux = {
			writeToChild: function(data) {},
			hasChild: function() { return true; }
		};

		var s = initState('PLAN_GENERATION');
		// Pre-fill 99 conversations.
		for (var i = 0; i < 99; i++) {
			s.claudeConversations.push({question: 'Q' + i, answer: 'A' + i, ts: i});
		}

		// 100th entry — should still be 100.
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'answer100';
		s.claudeQuestionLine = 'Q100';
		var r = sendKey(s, 'enter');
		s = r[0];

		if (s.claudeConversations.length !== 100) {
			if (savedMux !== undefined) globalThis.tuiMux = savedMux;
			else delete globalThis.tuiMux;
			return 'FAIL: at 100 entries, length should be 100, got ' + s.claudeConversations.length;
		}

		// 101st entry — should trigger trim to 80.
		s.claudeQuestionDetected = true;
		s.claudeQuestionInputActive = true;
		s.claudeQuestionInputText = 'answer101';
		s.claudeQuestionLine = 'Q101';
		r = sendKey(s, 'enter');
		s = r[0];

		if (savedMux !== undefined) globalThis.tuiMux = savedMux;
		else delete globalThis.tuiMux;

		if (s.claudeConversations.length > 100) return 'FAIL: not capped, length=' + s.claudeConversations.length;
		// After adding the 101st (which triggers cap), we should have 81 (80 from trim + 1 that just overflowed).
		// Actually: push makes it 101, then trim to 80. So length should be 80.
		// Wait — push(101) → length=101 → threshold >100 → slice(-80) → length=80.
		if (s.claudeConversations.length !== 80) return 'FAIL: expected 80 after trim, got ' + s.claudeConversations.length;
		// Last entry should be the one we just added.
		var last = s.claudeConversations[s.claudeConversations.length - 1];
		if (last.answer !== 'answer101') return 'FAIL: last answer: ' + last.answer;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("conversation history cap: %v", raw)
	}
}
