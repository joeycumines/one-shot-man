package command

// T427: Unit tests for chunk 16c confirm-cancel and Claude conversation handlers.
//
// Covers:
//   - updateConfirmCancel (9 tests): Tab/Shift+Tab focus cycling, Enter on Yes/No,
//     y key confirms, n/esc dismisses, mouse click zones, focus auto-init,
//     unknown key noop, session cleanup on confirm
//   - closeClaudeConvo (1 test): active→false, clears input/scroll, preserves
//     history+context
//   - updateClaudeConvo (5 tests): typing/backspace/ctrl+u/esc editing,
//     scroll keys (up/pgup/down/pgdown + mouse wheel + floor clamp),
//     enter non-empty submits (via stub), sending blocks all input
//     (enter/char/bs/ctrl+u), mouse click consumption
//   - pollClaudeConvo (3 tests): sending continues polling, plan-revised resets
//     selection, idle→null
//   - openClaudeConvo (3 tests): no executor sets error, dead handle sets error,
//     live handle succeeds

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ──────────────────────────── updateConfirmCancel ───────────────────────────

// TestChunk16c_ConfirmCancel_TabCyclesFocus verifies that Tab toggles focus
// between index 0 (Yes) and 1 (No) and wraps both ways.
func TestChunk16c_ConfirmCancel_TabCyclesFocus(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {showConfirmCancel: true, confirmCancelFocus: 0, wizard: {cancel: function(){}}};
		// Tab: 0 → 1
		var r1 = prSplit._updateConfirmCancel({type:'Key', key:'tab'}, s);
		var f1 = r1[0].confirmCancelFocus;
		// Tab: 1 → 0 (wrap)
		var r2 = prSplit._updateConfirmCancel({type:'Key', key:'tab'}, r1[0]);
		var f2 = r2[0].confirmCancelFocus;
		return JSON.stringify({after1: f1, after2: f2});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"after1":1`) {
		t.Errorf("Tab from 0 should go to 1: %s", s)
	}
	if !strings.Contains(s, `"after2":0`) {
		t.Errorf("Tab from 1 should wrap to 0: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_ShiftTabCyclesFocus verifies that Shift+Tab
// cycles in reverse order (0→1 with wrap, 1→0).
func TestChunk16c_ConfirmCancel_ShiftTabCyclesFocus(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {showConfirmCancel: true, confirmCancelFocus: 1, wizard: {cancel: function(){}}};
		// Shift+Tab: 1 → 0
		var r1 = prSplit._updateConfirmCancel({type:'Key', key:'shift+tab'}, s);
		var f1 = r1[0].confirmCancelFocus;
		// Shift+Tab: 0 → 1 (wrap)
		var r2 = prSplit._updateConfirmCancel({type:'Key', key:'shift+tab'}, r1[0]);
		var f2 = r2[0].confirmCancelFocus;
		return JSON.stringify({after1: f1, after2: f2});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"after1":0`) {
		t.Errorf("Shift+Tab from 1 should go to 0: %s", s)
	}
	if !strings.Contains(s, `"after2":1`) {
		t.Errorf("Shift+Tab from 0 should wrap to 1: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_EnterOnYes verifies that Enter activates the
// focused button: focus=0 confirms (quits), focus=1 dismisses.
func TestChunk16c_ConfirmCancel_EnterOnYes(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		// Focus 0: Enter confirms (returns quit command).
		var s0 = {
			showConfirmCancel: true,
			confirmCancelFocus: 0,
			isProcessing: true,
			analysisRunning: true,
			autoSplitRunning: true,
			wizard: {cancel: function(){}, transition: function(){}},
			wizardState: 'PLAN_REVIEW',
		};
		var r0 = prSplit._updateConfirmCancel({type:'Key', key:'enter'}, s0);
		var confirmed = r0[0].wizardState === 'CANCELLED' && r0[1] !== null;

		// Focus 1: Enter dismisses (overlay closed, no quit).
		var s1 = {
			showConfirmCancel: true,
			confirmCancelFocus: 1,
			wizard: {cancel: function(){}},
		};
		var r1 = prSplit._updateConfirmCancel({type:'Key', key:'enter'}, s1);
		var dismissed = r1[0].showConfirmCancel === false && r1[1] === null;

		return JSON.stringify({confirmed: confirmed, dismissed: dismissed});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"confirmed":true`) {
		t.Errorf("Enter on focus=0 should confirm (quit): %s", s)
	}
	if !strings.Contains(s, `"dismissed":true`) {
		t.Errorf("Enter on focus=1 should dismiss overlay: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_YKeyAlwaysConfirms verifies that 'y' confirms
// regardless of the current focus index.
func TestChunk16c_ConfirmCancel_YKeyAlwaysConfirms(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var results = [];
		for (var focus = 0; focus < 2; focus++) {
			var s = {
				showConfirmCancel: true,
				confirmCancelFocus: focus,
				isProcessing: true,
				analysisRunning: true,
				autoSplitRunning: true,
				wizard: {cancel: function(){}, transition: function(){}},
				wizardState: 'PLAN_REVIEW',
			};
			var r = prSplit._updateConfirmCancel({type:'Key', key:'y'}, s);
			results.push(r[0].wizardState === 'CANCELLED' && r[1] !== null);
		}
		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s != "[true,true]" {
		t.Errorf("'y' should confirm for both focus values: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_NAndEscDismiss verifies that both 'n' and 'esc'
// dismiss the overlay without quitting.
func TestChunk16c_ConfirmCancel_NAndEscDismiss(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var results = [];
		var keys = ['n', 'esc'];
		for (var i = 0; i < keys.length; i++) {
			var s = {
				showConfirmCancel: true,
				confirmCancelFocus: 1,  // start at 1 so reset to 0 is observable
				wizardState: 'PLAN_REVIEW',
				wizard: {cancel: function(){}},
			};
			var r = prSplit._updateConfirmCancel({type:'Key', key: keys[i]}, s);
			results.push({
				overlayClosed: r[0].showConfirmCancel === false,
				noQuit: r[1] === null,
				stateUnchanged: r[0].wizardState === 'PLAN_REVIEW',
				focusReset: r[0].confirmCancelFocus === 0,
			});
		}
		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, key := range []string{"overlayClosed", "noQuit", "stateUnchanged", "focusReset"} {
		if !strings.Contains(s, `"`+key+`":true`) {
			t.Errorf("n/esc should dismiss correctly (%s): %s", key, s)
		}
	}
}

// TestChunk16c_ConfirmCancel_MouseClickZones verifies that mouse click on
// confirm-yes confirms and confirm-no dismisses.
func TestChunk16c_ConfirmCancel_MouseClickZones(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		// Monkey-patch the captured zone object's inBounds method.
		var zone = prSplit._zone;
		var origInBounds = zone.inBounds;
		var zoneResults = {};
		zone.inBounds = function(id, msg) {
			return zoneResults[id] || false;
		};
		try {
			// Click on confirm-yes.
			zoneResults = {'confirm-yes': true};
			var s1 = {
				showConfirmCancel: true,
				confirmCancelFocus: 0,
				wizard: {cancel: function(){}, transition: function(){}},
				wizardState: 'PLAN_REVIEW',
			};
			var r1 = prSplit._updateConfirmCancel({type:'MouseClick', button:'left', x:10, y:10, mod:[]}, s1);
			var yesConfirms = r1[0].wizardState === 'CANCELLED';

			// Click on confirm-no.
			zoneResults = {'confirm-no': true};
			var s2 = {
				showConfirmCancel: true,
				confirmCancelFocus: 0,
				wizardState: 'PLAN_REVIEW',
				wizard: {cancel: function(){}},
			};
			var r2 = prSplit._updateConfirmCancel({type:'MouseClick', button:'left', x:10, y:10, mod:[]}, s2);
			var noDismisses = r2[0].showConfirmCancel === false && r2[0].wizardState === 'PLAN_REVIEW';

			return JSON.stringify({yesConfirms: yesConfirms, noDismisses: noDismisses});
		} finally {
			zone.inBounds = origInBounds;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"yesConfirms":true`) {
		t.Errorf("mouse click confirm-yes should confirm: %s", s)
	}
	if !strings.Contains(s, `"noDismisses":true`) {
		t.Errorf("mouse click confirm-no should dismiss: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_FocusAutoInit verifies that confirmCancelFocus
// is auto-initialized to 0 when NaN, negative, or missing.
func TestChunk16c_ConfirmCancel_FocusAutoInit(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var results = [];
		var badValues = [NaN, -1, 5, undefined, 'hello'];
		for (var i = 0; i < badValues.length; i++) {
			var s = {
				showConfirmCancel: true,
				confirmCancelFocus: badValues[i],
				wizard: {cancel: function(){}},
			};
			// Send an unknown key to trigger the init check without side effects.
			prSplit._updateConfirmCancel({type:'Key', key:'x'}, s);
			results.push(s.confirmCancelFocus);
		}
		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s != "[0,0,0,0,0]" {
		t.Errorf("all bad focus values should auto-init to 0: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_UnknownKeyNoop verifies that unrecognized keys
// return [s, null] without changing state.
func TestChunk16c_ConfirmCancel_UnknownKeyNoop(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			showConfirmCancel: true,
			confirmCancelFocus: 0,
			wizardState: 'PLAN_REVIEW',
			wizard: {cancel: function(){}},
		};
		var r = prSplit._updateConfirmCancel({type:'Key', key:'x'}, s);
		return JSON.stringify({
			sameState: r[0].wizardState === 'PLAN_REVIEW',
			nullCmd: r[1] === null,
			overlayOpen: r[0].showConfirmCancel === true,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"sameState":true`) || !strings.Contains(s, `"nullCmd":true`) || !strings.Contains(s, `"overlayOpen":true`) {
		t.Errorf("unknown key should be noop: %s", s)
	}
}

// TestChunk16c_ConfirmCancel_CleanupSessions verifies that confirmCancel
// cleans up active verify session and resets tab to output.
func TestChunk16c_ConfirmCancel_CleanupSessions(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var sessionClosed = false;
		var s = {
			showConfirmCancel: true,
			confirmCancelFocus: 0,
			wizard: {cancel: function(){}, transition: function(){}},
			wizardState: 'EXECUTION',
			activeVerifySession: {close: function(){ sessionClosed = true; }},
			activeVerifyBranch: 'split/a',
			activeVerifyDir: '/tmp/verify',
			activeVerifyStartTime: Date.now(),
			verifyElapsedMs: 5000,
			verifyScreen: 'screen content',
			verifyViewportOffset: 5,
			verifyAutoScroll: false,
			verifyPaused: true,
			splitViewTab: 'verify',
		};
		var r = prSplit._updateConfirmCancel({type:'Key', key:'y'}, s);
		var ns = r[0];
		return JSON.stringify({
			sessionClosed: sessionClosed,
			tabReset: ns.splitViewTab === 'output',
			verifyCleared: ns.activeVerifySession === null && ns.activeVerifyBranch === null,
			verifyScreenCleared: ns.verifyScreen === '' && ns.verifyViewportOffset === 0,
			verifyAutoScrollReset: ns.verifyAutoScroll === true,
			verifyPausedReset: ns.verifyPaused === false,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{
		"sessionClosed", "tabReset", "verifyCleared",
		"verifyScreenCleared", "verifyAutoScrollReset", "verifyPausedReset",
	} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("cleanup should set %s to true: %s", field, s)
		}
	}
}

// ───────────────────────────── closeClaudeConvo ────────────────────────────

// TestChunk16c_CloseClaudeConvo_PreservesHistory verifies that closing the
// conversation overlay sets active=false, clears input and scroll, but
// preserves history and context for later reopening.
func TestChunk16c_CloseClaudeConvo_PreservesHistory(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			claudeConvo: {
				active: true,
				inputText: 'some draft',
				scrollOffset: 7,
				history: [
					{role: 'user', text: 'hello'},
					{role: 'assistant', text: 'world'},
				],
				context: 'plan-review',
				lastError: null,
				sending: false,
			},
		};
		var r = prSplit._closeClaudeConvo(s);
		var c = r[0].claudeConvo;
		return JSON.stringify({
			activeFalse: c.active === false,
			inputCleared: c.inputText === '',
			scrollCleared: c.scrollOffset === 0,
			historyPreserved: c.history.length === 2 && c.history[0].role === 'user',
			contextPreserved: c.context === 'plan-review',
			nullCmd: r[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{
		"activeFalse", "inputCleared", "scrollCleared",
		"historyPreserved", "contextPreserved", "nullCmd",
	} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("closeClaudeConvo: %s should be true: %s", field, s)
		}
	}
}

// ───────────────────────────── updateClaudeConvo ───────────────────────────

// TestChunk16c_UpdateClaudeConvo_TypingAndEditing verifies the positive
// functional paths for character input, backspace, and ctrl+u when
// sending=false. Also verifies esc closes the overlay.
func TestChunk16c_UpdateClaudeConvo_TypingAndEditing(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		// Start with empty input.
		var s = {
			claudeConvo: {
				active: true,
				inputText: '',
				sending: false,
				scrollOffset: 0,
				history: [{role:'user', text:'previous'}],
				context: 'plan-review',
			},
		};

		// Type 'h', 'i' — should accumulate.
		prSplit._updateClaudeConvo({type:'Key', key:'h'}, s);
		prSplit._updateClaudeConvo({type:'Key', key:'i'}, s);
		var afterType = s.claudeConvo.inputText;

		// Backspace — should remove last char.
		prSplit._updateClaudeConvo({type:'Key', key:'backspace'}, s);
		var afterBs = s.claudeConvo.inputText;

		// More typing then ctrl+u — should clear.
		prSplit._updateClaudeConvo({type:'Key', key:'x'}, s);
		prSplit._updateClaudeConvo({type:'Key', key:'ctrl+u'}, s);
		var afterCtrlU = s.claudeConvo.inputText;

		// Esc — should close overlay, preserve history.
		s.claudeConvo.inputText = 'draft';
		s.claudeConvo.scrollOffset = 5;
		var r = prSplit._updateClaudeConvo({type:'Key', key:'esc'}, s);
		var c = r[0].claudeConvo;

		return JSON.stringify({
			afterType: afterType,
			afterBs: afterBs,
			afterCtrlU: afterCtrlU,
			escActive: c.active,
			escInputCleared: c.inputText === '',
			escScrollCleared: c.scrollOffset === 0,
			escHistoryPreserved: c.history.length === 1 && c.history[0].text === 'previous',
			escContextPreserved: c.context === 'plan-review',
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"afterType":"hi"`) {
		t.Errorf("typing 'h','i' should produce 'hi': %s", s)
	}
	if !strings.Contains(s, `"afterBs":"h"`) {
		t.Errorf("backspace on 'hi' should produce 'h': %s", s)
	}
	if !strings.Contains(s, `"afterCtrlU":""`) {
		t.Errorf("ctrl+u should clear input: %s", s)
	}
	if !strings.Contains(s, `"escActive":false`) {
		t.Errorf("esc should close overlay: %s", s)
	}
	if !strings.Contains(s, `"escInputCleared":true`) {
		t.Errorf("esc should clear input: %s", s)
	}
	if !strings.Contains(s, `"escScrollCleared":true`) {
		t.Errorf("esc should clear scroll: %s", s)
	}
	if !strings.Contains(s, `"escHistoryPreserved":true`) {
		t.Errorf("esc should preserve history: %s", s)
	}
	if !strings.Contains(s, `"escContextPreserved":true`) {
		t.Errorf("esc should preserve context: %s", s)
	}
}

// TestChunk16c_UpdateClaudeConvo_ScrollKeys verifies that up/pgup/down/pgdown
// scroll the conversation history, including the floor-at-zero clamp.
func TestChunk16c_UpdateClaudeConvo_ScrollKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			claudeConvo: {
				active: true,
				inputText: '',
				sending: false,
				scrollOffset: 0,
			},
		};

		// Up: 0 → 3
		prSplit._updateClaudeConvo({type:'Key', key:'up'}, s);
		var afterUp = s.claudeConvo.scrollOffset;

		// PgUp: 3 → 6
		prSplit._updateClaudeConvo({type:'Key', key:'pgup'}, s);
		var afterPgUp = s.claudeConvo.scrollOffset;

		// Down: 6 → 3
		prSplit._updateClaudeConvo({type:'Key', key:'down'}, s);
		var afterDown = s.claudeConvo.scrollOffset;

		// PgDown: 3 → 0
		prSplit._updateClaudeConvo({type:'Key', key:'pgdown'}, s);
		var afterPgDown = s.claudeConvo.scrollOffset;

		// Down at 0 should clamp to 0 (not go negative).
		prSplit._updateClaudeConvo({type:'Key', key:'down'}, s);
		var afterClamp = s.claudeConvo.scrollOffset;

		// Mouse wheel up/down.
		prSplit._updateClaudeConvo({type:'MouseWheel', button:'wheel up', x:0, y:0, mod:[]}, s);
		var afterWheelUp = s.claudeConvo.scrollOffset;
		prSplit._updateClaudeConvo({type:'MouseWheel', button:'wheel down', x:0, y:0, mod:[]}, s);
		var afterWheelDown = s.claudeConvo.scrollOffset;

		return JSON.stringify({
			afterUp: afterUp,
			afterPgUp: afterPgUp,
			afterDown: afterDown,
			afterPgDown: afterPgDown,
			afterClamp: afterClamp,
			afterWheelUp: afterWheelUp,
			afterWheelDown: afterWheelDown,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"afterUp":3`) {
		t.Errorf("up should increment by 3: %s", s)
	}
	if !strings.Contains(s, `"afterPgUp":6`) {
		t.Errorf("pgup should increment by 3: %s", s)
	}
	if !strings.Contains(s, `"afterDown":3`) {
		t.Errorf("down should decrement by 3: %s", s)
	}
	if !strings.Contains(s, `"afterPgDown":0`) {
		t.Errorf("pgdown should decrement to 0: %s", s)
	}
	if !strings.Contains(s, `"afterClamp":0`) {
		t.Errorf("down at 0 should clamp to 0: %s", s)
	}
	if !strings.Contains(s, `"afterWheelUp":3`) {
		t.Errorf("wheel up should increment by 3: %s", s)
	}
	if !strings.Contains(s, `"afterWheelDown":0`) {
		t.Errorf("wheel down should decrement: %s", s)
	}
}

// TestChunk16c_UpdateClaudeConvo_SubmitNonEmpty verifies that enter submits
// non-empty input text by calling submitClaudeMessage (stubbed to avoid
// actual async). Also tests that empty enter remains a no-op.
func TestChunk16c_UpdateClaudeConvo_SubmitNonEmpty(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		// Stub the dependencies that submitClaudeMessage needs.
		var st = prSplit._state || {};
		st.claudeExecutor = {
			handle: {
				isAlive: function() { return true; },
				sendMessage: function() { return {then: function(f,r) { f({text:'ok'}); }}; },
			},
		};
		if (!prSplit._state) prSplit._state = st;

		// Stub prSplit.sendToHandle to capture the submission.
		var submitted = null;
		prSplit.sendToHandle = function(handle, prompt) {
			submitted = prompt;
			return {then: function(f) { f({text:'response'}); }};
		};

		var s = {
			claudeConvo: {
				active: true,
				inputText: 'fix the bug',
				scrollOffset: 3,
				sending: false,
				history: [],
				context: 'error-resolution',
				lastError: null,
			},
		};
		var r = prSplit._updateClaudeConvo({type:'Key', key:'enter'}, s);
		var c = r[0].claudeConvo;

		// Enter with empty text should be a no-op.
		var s2 = {
			claudeConvo: {
				active: true,
				inputText: '',
				sending: false,
				history: [],
				context: 'plan-review',
			},
		};
		var r2 = prSplit._updateClaudeConvo({type:'Key', key:'enter'}, s2);

		return JSON.stringify({
			sending: c.sending === true,
			inputCleared: c.inputText === '',
			historyHasEntry: c.history.length > 0,
			emptyNoOp: r2[0].claudeConvo.inputText === '' && r2[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"sending":true`) {
		t.Errorf("enter on non-empty should set sending: %s", s)
	}
	if !strings.Contains(s, `"inputCleared":true`) {
		t.Errorf("enter on non-empty should clear input: %s", s)
	}
	if !strings.Contains(s, `"historyHasEntry":true`) {
		t.Errorf("submit should add to history: %s", s)
	}
	if !strings.Contains(s, `"emptyNoOp":true`) {
		t.Errorf("enter on empty should be no-op: %s", s)
	}
}

// TestChunk16c_UpdateClaudeConvo_SendingBlocksAllInput verifies that when
// convo.sending=true, character input, backspace, ctrl+u, and enter are all
// blocked but scroll keys still work.
func TestChunk16c_UpdateClaudeConvo_SendingBlocksAllInput(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var base = function() {
			return {
				claudeConvo: {
					active: true,
					inputText: 'original',
					sending: true,
					scrollOffset: 5,
					history: [],
				},
			};
		};

		// Char input blocked (key 'a' is single printable).
		var s1 = base();
		prSplit._updateClaudeConvo({type:'Key', key:'a'}, s1);
		var charBlocked = s1.claudeConvo.inputText === 'original';

		// Backspace blocked.
		var s2 = base();
		prSplit._updateClaudeConvo({type:'Key', key:'backspace'}, s2);
		var bsBlocked = s2.claudeConvo.inputText === 'original';

		// Ctrl+U blocked.
		var s3 = base();
		prSplit._updateClaudeConvo({type:'Key', key:'ctrl+u'}, s3);
		var ctrlUBlocked = s3.claudeConvo.inputText === 'original';

		// Enter blocked (would submit if not sending).
		var s5 = base();
		s5.claudeConvo.inputText = 'would submit';
		var r5 = prSplit._updateClaudeConvo({type:'Key', key:'enter'}, s5);
		var enterBlocked = s5.claudeConvo.inputText === 'would submit' && r5[1] === null;

		// Scroll still works during sending.
		var s4 = base();
		prSplit._updateClaudeConvo({type:'Key', key:'up'}, s4);
		var scrollWorks = s4.claudeConvo.scrollOffset === 8;

		return JSON.stringify({
			charBlocked: charBlocked,
			bsBlocked: bsBlocked,
			ctrlUBlocked: ctrlUBlocked,
			enterBlocked: enterBlocked,
			scrollWorks: scrollWorks,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{"charBlocked", "bsBlocked", "ctrlUBlocked", "enterBlocked", "scrollWorks"} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("sending state: %s should be true: %s", field, s)
		}
	}
}

// TestChunk16c_UpdateClaudeConvo_MouseClickConsumption verifies that
// non-wheel mouse events are consumed (prevent leakage) and return [s, null].
func TestChunk16c_UpdateClaudeConvo_MouseClickConsumption(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			claudeConvo: {
				active: true,
				inputText: 'test',
				sending: false,
				scrollOffset: 0,
			},
		};
		var r = prSplit._updateClaudeConvo({
			type:'MouseClick', button:'left', x:10, y:10, mod:[],
		}, s);
		return JSON.stringify({
			inputUnchanged: r[0].claudeConvo.inputText === 'test',
			nullCmd: r[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"inputUnchanged":true`) || !strings.Contains(s, `"nullCmd":true`) {
		t.Errorf("non-wheel mouse should be consumed without changes: %s", s)
	}
}

// ──────────────────────────── pollClaudeConvo ──────────────────────────────

// TestChunk16c_PollClaudeConvo_SendingContinuesPolling verifies that when
// convo.sending=true, pollClaudeConvo returns a tick command to continue.
func TestChunk16c_PollClaudeConvo_SendingContinuesPolling(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var s = {
			claudeConvo: {
				sending: true,
				history: [{role:'user', text:'hello'}],
			},
		};
		var r = prSplit._pollClaudeConvo(s);
		return JSON.stringify({
			hasTick: r[1] !== null,
			stateUnchanged: r[0].claudeConvo.sending === true,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"hasTick":true`) {
		t.Errorf("sending should produce tick command: %s", s)
	}
	if !strings.Contains(s, `"stateUnchanged":true`) {
		t.Errorf("sending state should not be altered: %s", s)
	}
}

// TestChunk16c_PollClaudeConvo_PlanRevisedResetsSelection verifies that
// when st.planRevised=true, pollClaudeConvo resets selectedSplitIdx and
// clears the flag.
func TestChunk16c_PollClaudeConvo_PlanRevisedResetsSelection(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.planRevised = true;
		if (!prSplit._state) prSplit._state = st;

		var s = {
			claudeConvo: {sending: false},
			selectedSplitIdx: 5,
		};
		var r = prSplit._pollClaudeConvo(s);
		return JSON.stringify({
			idxReset: r[0].selectedSplitIdx === 0,
			flagCleared: st.planRevised === false,
			nullCmd: r[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"idxReset":true`) {
		t.Errorf("planRevised should reset selectedSplitIdx: %s", s)
	}
	if !strings.Contains(s, `"flagCleared":true`) {
		t.Errorf("planRevised flag should be cleared: %s", s)
	}
	if !strings.Contains(s, `"nullCmd":true`) {
		t.Errorf("completed poll should return null cmd: %s", s)
	}
}

// TestChunk16c_PollClaudeConvo_IdleReturnsNull verifies that when
// sending=false and planRevised=false, poll returns [s, null].
func TestChunk16c_PollClaudeConvo_IdleReturnsNull(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.planRevised = false;
		if (!prSplit._state) prSplit._state = st;

		var s = {claudeConvo: {sending: false}, selectedSplitIdx: 3};
		var r = prSplit._pollClaudeConvo(s);
		return JSON.stringify({
			idxUnchanged: r[0].selectedSplitIdx === 3,
			nullCmd: r[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"idxUnchanged":true`) || !strings.Contains(s, `"nullCmd":true`) {
		t.Errorf("idle poll should be no-op: %s", s)
	}
}

// ──────────────────────────── openClaudeConvo ──────────────────────────────

// TestChunk16c_OpenClaudeConvo_NoExecutor verifies that when no Claude
// executor exists AND Claude is detected as unavailable, openClaudeConvo
// sets lastError and still activates the overlay.
func TestChunk16c_OpenClaudeConvo_NoExecutor(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.claudeExecutor = null;
		if (!prSplit._state) prSplit._state = st;

		// T5: With claudeCheckStatus='unavailable', openClaudeConvo shows
		// an immediate error instead of attempting on-demand spawn.
		var s = {claudeConvo: {}, claudeCheckStatus: 'unavailable'};
		var r = prSplit._openClaudeConvo(s, 'plan-review');
		var c = r[0].claudeConvo;
		return JSON.stringify({
			active: c.active === true,
			hasError: typeof c.lastError === 'string' && c.lastError.length > 0,
			errorMentionsInstall: (c.lastError || '').indexOf('not installed') >= 0,
			contextSet: c.context === 'plan-review',
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{"active", "hasError", "errorMentionsInstall", "contextSet"} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("no-executor-unavailable: %s should be true: %s", field, s)
		}
	}
}

// TestChunk16c_OpenClaudeConvo_OnDemandSpawn verifies that when no executor
// exists but Claude is available, openClaudeConvo triggers on-demand spawn.
func TestChunk16c_OpenClaudeConvo_OnDemandSpawn(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.claudeExecutor = null;
		if (!prSplit._state) prSplit._state = st;

		// T5: When Claude status is not 'unavailable', openClaudeConvo
		// starts an on-demand spawn (async) rather than erroring immediately.
		var s = {claudeConvo: {}, claudeCheckStatus: 'available'};
		var r = prSplit._openClaudeConvo(s, 'plan-review');
		var state = r[0];
		var cmd = r[1]; // tea.tick command
		return JSON.stringify({
			active: state.claudeConvo.active === true,
			noError: !state.claudeConvo.lastError,
			spawning: state.claudeOnDemandSpawning === true,
			hasProgress: typeof state.claudeConvo.spawnProgress === 'string',
			hasCmd: cmd !== null && cmd !== undefined,
			contextSet: state.claudeConvo.context === 'plan-review',
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{"active", "noError", "spawning", "hasProgress", "hasCmd", "contextSet"} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("on-demand-spawn: %s should be true: %s", field, s)
		}
	}
}

// TestChunk16c_OpenClaudeConvo_ReopenDuringSpawn verifies that closing and
// re-opening the overlay while an on-demand spawn is in progress correctly
// re-activates the overlay without launching a second spawn.
func TestChunk16c_OpenClaudeConvo_ReopenDuringSpawn(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.claudeExecutor = null;
		if (!prSplit._state) prSplit._state = st;

		// Step 1: Open — starts on-demand spawn.
		var s = {claudeConvo: {}, claudeCheckStatus: 'available'};
		var r = prSplit._openClaudeConvo(s, 'plan-review');
		s = r[0];
		// s.claudeOnDemandSpawning should be true, overlay active.

		// Step 2: Close overlay (Escape). Spawn continues in background.
		r = prSplit._closeClaudeConvo(s);
		s = r[0];
		var closedActive = s.claudeConvo.active; // false
		var stillSpawning = s.claudeOnDemandSpawning; // true

		// Step 3: Re-open. Should NOT double-launch but SHOULD re-activate.
		r = prSplit._openClaudeConvo(s, 'error-resolution');
		s = r[0];
		var cmd = r[1]; // null (no new tick — existing tick continues)

		return JSON.stringify({
			closedInactive: closedActive === false,
			stillSpawning: stillSpawning === true,
			reopenedActive: s.claudeConvo.active === true,
			contextUpdated: s.claudeConvo.context === 'error-resolution',
			noDoubleCmd: cmd === null,
			spawningPreserved: s.claudeOnDemandSpawning === true,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{
		"closedInactive", "stillSpawning", "reopenedActive",
		"contextUpdated", "noDoubleCmd", "spawningPreserved",
	} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("reopen-during-spawn: %s should be true: %s", field, s)
		}
	}
}

// TestChunk16c_OpenClaudeConvo_DeadHandle verifies that when the executor
// handle reports isAlive()=false, an error about exited process is set.
func TestChunk16c_OpenClaudeConvo_DeadHandle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.claudeExecutor = {
			handle: {isAlive: function() { return false; }},
		};
		if (!prSplit._state) prSplit._state = st;

		var s = {claudeConvo: {}};
		var r = prSplit._openClaudeConvo(s, 'error-resolution');
		var c = r[0].claudeConvo;
		return JSON.stringify({
			active: c.active === true,
			hasError: typeof c.lastError === 'string' && c.lastError.length > 0,
			errorMentionsExited: (c.lastError || '').indexOf('exited') >= 0,
			contextSet: c.context === 'error-resolution',
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{"active", "hasError", "errorMentionsExited", "contextSet"} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("dead-handle: %s should be true: %s", field, s)
		}
	}
}

// TestChunk16c_OpenClaudeConvo_LiveHandle verifies that a live executor
// opens the conversation overlay successfully: active=true, no error,
// inputText and scrollOffset reset.
func TestChunk16c_OpenClaudeConvo_LiveHandle(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`(function() {
		var st = prSplit._state || {};
		st.claudeExecutor = {
			handle: {isAlive: function() { return true; }},
		};
		if (!prSplit._state) prSplit._state = st;

		var s = {
			claudeConvo: {
				inputText: 'leftover',
				scrollOffset: 10,
				lastError: 'old error',
				active: false,
			},
		};
		var r = prSplit._openClaudeConvo(s, 'plan-review');
		var c = r[0].claudeConvo;
		return JSON.stringify({
			active: c.active === true,
			noError: c.lastError === null,
			inputReset: c.inputText === '',
			scrollReset: c.scrollOffset === 0,
			contextSet: c.context === 'plan-review',
			nullCmd: r[1] === null,
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	for _, field := range []string{"active", "noError", "inputReset", "scrollReset", "contextSet", "nullCmd"} {
		if !strings.Contains(s, `"`+field+`":true`) {
			t.Errorf("live-handle: %s should be true: %s", field, s)
		}
	}
}
