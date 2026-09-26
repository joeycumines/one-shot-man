package command

// pr_split_16b_pane_cleanup_deferral_test.go — bounded verify-pane teardown
// deferral.
//
// A pending pane cleanup or timeout kill defers the next pipeline action until
// the tracked operation settles. The deferral is bounded on purpose: a
// teardown whose Promise never settles must not leave the wizard spinning
// forever with no error. These tests cover the three states of that contract —
// deferral while pending, execution once cleared, and a visible error once the
// budget is spent.

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestChunk16b_PaneCleanupDeferralBackoffAndReset verifies the poll schedule
// grows and that clearing the teardown flags resets it.
func TestChunk16b_PaneCleanupDeferralBackoffAndReset(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s._verifyPaneCleanupPending = true;
		var delays = [];
		for (var i = 0; i < 8; i++) {
			delays.push(prSplit._nextPaneCleanupDelay(s));
		}
		if (delays[0] !== 10) return 'FAIL: first delay=' + delays[0];
		if (delays[1] <= delays[0]) return 'FAIL: no backoff: ' + delays.join(',');
		var last = delays[delays.length - 1];
		if (last !== 500) return 'FAIL: delay cap not reached: ' + last;
		for (var j = 2; j < delays.length; j++) {
			if (delays[j] < delays[j-1]) return 'FAIL: non-monotonic: ' + delays.join(',');
		}
		// Flags clear: the counter resets, so the next deferral starts fast.
		s._verifyPaneCleanupPending = false;
		prSplit._paneCleanupTickReset(s);
		if (prSplit._nextPaneCleanupDelay(s) !== 10) return 'FAIL: no reset after clear';
		// Not pending means never expired.
		if (prSplit._paneCleanupExpired(s)) return 'FAIL: expired without a stuck teardown';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pane cleanup deferral schedule: %v", raw)
	}
}

// TestChunk16b_PaneCleanupCeilingFailsWizard verifies the escalation ceiling.
// The scenario is a teardown that never settles: the tick is re-armed while
// the budget lasts, then the wizard is failed with a diagnosable message and
// the stuck flags are cleared so the state machine is not wedged.
func TestChunk16b_PaneCleanupCeilingFailsWizard(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('PLAN_REVIEW');
		// The teardown never settles: the flag stays set for every tick.
		s._verifyPaneCleanupPending = true;
		s._executionStartPending = true;
		var deferred = 0;
		var guard = 0;
		while (s.wizard.current !== 'ERROR' && guard < 200) {
			var r = update({type: 'Tick', id: 'execution-start'}, s);
			s = r[0];
			guard++;
			if (s.wizard.current !== 'ERROR') {
				deferred++;
				if (!s._executionStartPending) return 'FAIL: deferral dropped the action';
				if (!s._verifyPaneCleanupPending) return 'FAIL: flag cleared before the ceiling';
			}
		}
		if (s.wizard.current !== 'ERROR') return 'FAIL: ceiling never reached';
		if (deferred < 2) return 'FAIL: no deferral observed, failed immediately';
		if (s._executionStartPending) return 'FAIL: execution still pending after ceiling';
		if (s._verifyPaneCleanupPending) return 'FAIL: stuck flag left set after ceiling';
		if (s._paneCleanupTickAttempts !== 0) return 'FAIL: attempt counter not reset';
		if (!s.errorDetails || s.errorDetails.indexOf('teardown') < 0) {
			return 'FAIL: error detail=' + s.errorDetails;
		}
		if (s.isProcessing) return 'FAIL: isProcessing still true';
		// The error screen renders from errorDetails in the ERROR state, so
		// the failure is actually visible rather than a silent stall.
		if (s.wizardState !== 'ERROR') return 'FAIL: wizardState=' + s.wizardState;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("pane cleanup ceiling: %v", raw)
	}
}

// TestChunk16c_PaneCleanupCeilingCoversVerifyLoops verifies that the bound is a
// property of the teardown flags rather than of one call site. Both verify
// loops used to re-arm on the same flags with no budget, so a wedged teardown
// left the wizard spinning at 100 Hz forever with no diagnosis.
func TestChunk16c_PaneCleanupCeilingCoversVerifyLoops(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		function drive(msgId, screen) {
			var s = initState(screen);
			s.isProcessing = true;
			s.splitViewEnabled = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s._verifyAdvanceAfterCleanup = true;
			s.activeVerifyBranch = 'split/api';
			s.activeVerifyStartTime = 0;
			s._verifyPaneCleanupPending = true;
			s._paneCleanupWaitedMs = prSplit._PANE_CLEANUP_TIMEOUT_MS;
			var guard = 0;
			while (s.wizard.current !== 'ERROR' && guard < 10) {
				s = update({type: 'Tick', id: msgId}, s)[0];
				guard++;
			}
			return {
				state: s.wizard.current,
				detail: s.errorDetails || '',
				pending: !!s._verifyPaneCleanupPending,
				results: (s.verificationResults || []).length,
				spinning: s.isProcessing
			};
		}
		var branch = drive('verify-branch', 'BRANCH_BUILDING');
		if (branch.state !== 'ERROR') return 'FAIL: verify-branch state=' + branch.state;
		if (branch.detail.indexOf('teardown') < 0) return 'FAIL: verify-branch detail=' + branch.detail;
		if (branch.pending) return 'FAIL: verify-branch left the flag set';
		if (branch.spinning) return 'FAIL: verify-branch still processing';
		var poll = drive('verify-poll', 'BRANCH_BUILDING');
		if (poll.state !== 'ERROR') return 'FAIL: verify-poll state=' + poll.state;
		if (poll.detail.indexOf('teardown') < 0) return 'FAIL: verify-poll detail=' + poll.detail;
		if (poll.spinning) return 'FAIL: verify-poll still processing';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("verify loop teardown ceiling: %v", raw)
	}
}

// TestChunk16e_StuckTeardownNeverBlocksQuit verifies the wizard can always be
// left. The quit is sequenced behind in-flight teardown, but that wait is
// bounded: once the budget is spent the flags clear and the quit proceeds.
//
// The quit command and a re-armed tick are both opaque objects in this
// harness, so the assertion is on observable state instead: a re-armed tick
// keeps the stuck flag set and keeps charging the deferral budget, while the
// quit path clears the flag and resets the budget to zero.
func TestChunk16e_StuckTeardownNeverBlocksQuit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.wizardQuitting = true;
		s.wizardQuitSent = true;
		s.wizardQuitPaneGated = true;
		s.wizardQuitPaneReady = true;
		s._verifyPaneCleanupPending = true;
		s._paneCleanupWaitedMs = prSplit._PANE_CLEANUP_TIMEOUT_MS;
		s = update({type: 'Tick', id: 'wizard-quit'}, s)[0];
		if (s._verifyPaneCleanupPending) return 'FAIL: stuck flag still set, quit deferred again';
		if (s._paneCleanupWaitedMs !== 0) return 'FAIL: deferral budget still charged: ' + s._paneCleanupWaitedMs;
		if (!s.wizardQuitSent) return 'FAIL: quit not sent';
		// A second tick must not restart the backoff: the quit path is
		// reached, not another deferral.
		s = update({type: 'Tick', id: 'wizard-quit'}, s)[0];
		if (s._paneCleanupWaitedMs !== 0) return 'FAIL: quit path re-charged the budget';
		if (!s.wizardQuitSent) return 'FAIL: quit state lost';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("stuck teardown blocks quit: %v", raw)
	}
}

// TestChunk16e_StuckQuestionWriteDoesNotStrandPrompt covers the same bounded-wait
// contract for the Agent question prompt. agentQuestionSendPending disables
// both dismissal keys while a pane write is in flight; if that write never
// settles, the user could neither dismiss nor retry the prompt.
func TestChunk16e_StuckQuestionWriteDoesNotStrandPrompt(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var s = initState('BRANCH_BUILDING');
		s.agentQuestionDetected = true;
		s.agentQuestionLine = 'Which module should own this?';
		s.agentQuestionInputActive = true;
		s.agentQuestionInputText = 'the api module';
		s.agentQuestionSendPending = true;
		// Stamped long enough ago that the budget is spent.
		s.agentQuestionSendAtMs = 0;
		// A non-dismissal key observes the diagnosis left behind.
		var diagnosed = update({type: 'Key', key: 'a'}, s)[0];
		if (diagnosed.agentQuestionSendPending) return 'FAIL: still pending after the budget';
		if (diagnosed.agentQuestionInputActive) return 'FAIL: prompt still traps input';
		if (diagnosed.agentQuestionLine.indexOf('Error sending response') < 0) {
			return 'FAIL: no diagnosis, line=' + diagnosed.agentQuestionLine;
		}
		// Escape now works again and dismisses the prompt.
		var dismissed = update({type: 'Key', key: 'esc'}, diagnosed)[0];
		if (dismissed.agentQuestionDetected) return 'FAIL: escape did not dismiss';
		if (dismissed.agentQuestionSendPending) return 'FAIL: pending after dismissal';
		// A fresh prompt is fully usable again.
		var t2 = initState('BRANCH_BUILDING');
		t2.agentQuestionDetected = true;
		t2.agentQuestionInputActive = true;
		t2.agentQuestionInputText = 'x';
		t2.agentQuestionSendPending = true;
		t2.agentQuestionSendAtMs = Date.now();
		var held = update({type: 'Key', key: 'esc'}, t2)[0];
		if (!held.agentQuestionSendPending) return 'FAIL: in-flight write released too early';
		if (!held.agentQuestionDetected) return 'FAIL: in-flight write swallowed the key';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("stuck question write strands the prompt: %v", raw)
	}
}

// TestChunk16e_LateQuestionSettlementDoesNotClobberNewerPrompt drives a real
// send: the pane write returns a Promise the test controls, the send budget
// expires, a newer question arrives, and only then does the write settle. The
// late settlement must not clear state it does not own.
func TestChunk16e_LateQuestionSettlementDoesNotClobberNewerPrompt(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		// The budget is in wall-clock terms measured from the send stamp, so
		// the clock must jump only AFTER the stamp is taken.
		var realNow = Date.now;
		var jumped = false;
		Date.now = function() { return realNow.call(Date) + (jumped ? 60000 : 0); };
		try {
			var s = initState('BRANCH_BUILDING');
			s.agentQuestionDetected = true;
			s.agentQuestionLine = 'first question';
			s.agentQuestionInputActive = true;
			s.agentQuestionInputText = 'first answer';
			s.agentConversations = [];

			// A pane whose write never settles until the test says so.
			var releaseWrite;
			s.activeAgentSession = {
				write: function() { return new Promise(function(resolve) { releaseWrite = resolve; }); },
				close: function() {}
			};

			// Enter starts the send.
			s = update({type: 'Key', key: 'enter'}, s)[0];
			if (!s.agentQuestionSendPending) return 'FAIL: send not marked pending';
			if (typeof releaseWrite !== 'function') return 'FAIL: write was never called';
			if (!s.agentQuestionSendAtMs) return 'FAIL: send not stamped';

			// A keypress now finds the budget spent and abandons the send.
			jumped = true;
			s = update({type: 'Key', key: 'a'}, s)[0];
			if (s.agentQuestionSendPending) return 'FAIL: send still pending after the budget';
			if (s.agentQuestionLine.indexOf('Error sending response') < 0) {
				return 'FAIL: abandonment not surfaced, line=' + s.agentQuestionLine;
			}

			// A newer Agent question arrives while the write is still in flight.
			s.agentQuestionDetected = true;
			s.agentQuestionLine = 'second question';
			s.agentQuestionInputActive = true;
			s.agentQuestionInputText = '';

			// The old write finally settles.
			releaseWrite();
			await new Promise(function(resolve) { setTimeout(resolve, 0); });

			if (!s.agentQuestionDetected) return 'FAIL: late settlement cleared the newer question';
			if (s.agentQuestionLine !== 'second question') {
				return 'FAIL: late settlement rewrote the line: ' + s.agentQuestionLine;
			}
			if (s.agentConversations.length !== 0) {
				return 'FAIL: abandoned send was recorded as delivered';
			}
			return 'OK';
		} finally {
			Date.now = realNow;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("late question settlement: %v", raw)
	}
}
