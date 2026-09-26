package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestPrSplitAgentLiveForwardsNavigationKeys(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var routed = [];
		var oldActive = prSplit._agentLiveActive;
		var oldRoute = prSplit._routeKeyToAgentTermpane;
		prSplit._agentLiveActive = function() { return true; };
		prSplit._routeKeyToAgentTermpane = function(msg) { routed.push(msg.key); };
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.splitViewFocus = 'agent';
			s.splitViewTab = 'agent';

			var keys = ['j', 'k', 'up', 'down', 'home', 'end', 'pgup', 'pgdown'];
			for (var i = 0; i < keys.length; i++) {
				sendKey(s, keys[i]);
			}

			var expected = keys.join(',');
			if (routed.join(',') !== expected) {
				return 'FAIL: live navigation keys were not forwarded: ' + JSON.stringify(routed);
			}
			return 'OK';
		} finally {
			prSplit._agentLiveActive = oldActive;
			prSplit._routeKeyToAgentTermpane = oldRoute;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("live agent key routing: %v", raw)
	}
}

func TestPrSplitAgentLiveIgnoresVerifyInterceptorsWhenAgentFocused(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var routed = [];
		var verifyActions = [];
		var oldActive = prSplit._agentLiveActive;
		var oldRoute = prSplit._routeKeyToAgentTermpane;
		prSplit._agentLiveActive = function() { return true; };
		prSplit._routeKeyToAgentTermpane = function(msg) { routed.push(msg.key); };
		try {
			var s = initState('PLAN_REVIEW');
			s.splitViewEnabled = true;
			s.splitViewFocus = 'agent';
			s.splitViewTab = 'agent';
			s.verifyMode = 'interactive';
			s.verifyShellExited = false;
			s.activeVerifySession = {
				interrupt: function() { verifyActions.push('interrupt'); },
				kill: function() { verifyActions.push('kill'); },
				screen: function() { return ''; },
				output: function() { return ''; },
				isDone: function() { return false; }
			};

			var keys = ['ctrl+c', 'j', 'k', 'up', 'down'];
			for (var i = 0; i < keys.length; i++) {
				sendKey(s, keys[i]);
			}

			if (routed.join(',') !== keys.join(',')) {
				return 'FAIL: Agent keys were intercepted by Verify: ' + JSON.stringify({
					routed: routed,
					verifyActions: verifyActions
				});
			}
			if (verifyActions.length !== 0) {
				return 'FAIL: Verify received Agent keys: ' + JSON.stringify(verifyActions);
			}
			return 'OK';
		} finally {
			prSplit._agentLiveActive = oldActive;
			prSplit._routeKeyToAgentTermpane = oldRoute;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("live Agent routing with Verify present: %v", raw)
	}
}

func TestPrSplitVerifyCtrlCWithWizardFocus(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		var actions = [];
		var s = initState('BRANCH_BUILDING');
		s.splitViewEnabled = true;
		s.splitViewFocus = 'wizard';
		s.splitViewTab = 'verify';
		s.activeVerifySession = {
			interrupt: function() { actions.push('interrupt'); },
			kill: function() { actions.push('kill'); },
			screen: function() { return ''; },
			output: function() { return ''; },
			isDone: function() { return false; }
		};

		var r = await sendKey(s, 'ctrl+c');
		r = await sendKey(r[0], 'ctrl+c');
		if (actions.join(',') !== 'interrupt,kill') {
			return 'FAIL: Verify Ctrl+C actions were ' + JSON.stringify(actions);
		}
		if (r[0].showConfirmCancel) {
			return 'FAIL: Verify Ctrl+C opened the wizard cancel dialog';
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("Verify Ctrl+C with wizard focus: %v", raw)
	}
}
