package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T113: startAutoAnalysis must NOT call synchronous isAvailable()
//
//  Verifies that when the Agent executor is unresolved, startAutoAnalysis
//  defers to the async check-agent tick instead of blocking the BubbleTea
//  event loop. Also verifies handleAgentCheckPoll correctly resumes or
//  falls back when pendingAutoAnalysis is set.
//
//  Access pattern: st = prSplit._state (closure-scoped in chunk 16 IIFE),
//  so tests use globalThis.prSplit._state to manipulate the executor.
//  startAutoAnalysis is triggered via handleNext (Enter key on CONFIG with
//  mode=auto). handleAgentCheckPoll is triggered via Tick id=agent-check-poll.
// ---------------------------------------------------------------------------

// TestChunk16_StartAutoAnalysis_DefersWhenUnresolved verifies that
// startAutoAnalysis returns a check-agent tick when the executor is
// freshly created and not yet resolved (resolved === null).
func TestChunk16_StartAutoAnalysis_DefersWhenUnresolved(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		if (typeof prSplitConfig === 'undefined') {
			prSplitConfig = {
				cleanupOnFailure: false,
				timeoutMs: 0
			};
		}

		prSplit.runtime = prSplit.runtime || {};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';
		prSplit.runtime.mode = 'auto';

		// Mock gitExec to avoid depending on git.
		var restoreGit = setupGitMock();
		try {
			setupPlanCache();
			var s = initState('CONFIG');
			s.configValidationError = null;
			s.availableBranches = [];
			s.outputLines = [];
			s.outputAutoScroll = true;
			s.focusIndex = -1;
			s.configFieldEditing = null;

			// Create an executor that is NOT resolved.
			globalThis.prSplit._state.agentExecutor = { resolved: null };

			// Trigger via Enter key on CONFIG → handleNext → startAutoAnalysis.
			var r = await sendKey(s, 'enter');
			var state = r[0];
			var cmd = r[1];

			if (!state.pendingAutoAnalysis) {
				return 'FAIL: expected pendingAutoAnalysis=true, got ' + state.pendingAutoAnalysis;
			}
			if (!cmd) return 'FAIL: expected tick command, got null';

			return 'OK';
		} finally {
			restoreGit();
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("startAutoAnalysis defers when unresolved: %v", raw)
	}
}

// TestChunk16_StartAutoAnalysis_ProceedsWhenResolved verifies that
// startAutoAnalysis does NOT defer when the executor is already resolved.
func TestChunk16_StartAutoAnalysis_ProceedsWhenResolved(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		if (typeof prSplitConfig === 'undefined') {
			prSplitConfig = {
				cleanupOnFailure: false,
				timeoutMs: 0
			};
		}

		prSplit.runtime = prSplit.runtime || {};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';
		prSplit.runtime.mode = 'auto';

		// Mock gitExec to avoid depending on git.
		var restoreGit = setupGitMock();
		try {
			setupPlanCache();
		var s = initState('CONFIG');
		s.configValidationError = null;
		s.availableBranches = [];
		s.outputLines = [];
		s.outputAutoScroll = true;
		s.autoSplitRunning = false;
		s.autoSplitResult = null;
		s.focusIndex = -1;
		s.configFieldEditing = null;

		// Create a RESOLVED executor.
		globalThis.prSplit._state.agentExecutor = {
			resolved: { command: 'agent', type: 'agent-code' },
			isAvailable: function() { return true; }
		};

		var r = await sendKey(s, 'enter');
		var state = r[0];

		if (state.pendingAutoAnalysis) {
			return 'FAIL: should not have pendingAutoAnalysis when executor is resolved';
		}
		if (!state.autoSplitRunning) {
			return 'FAIL: expected autoSplitRunning=true (pipeline launched)';
		}

		return 'OK';
		} finally {
			restoreGit();
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("startAutoAnalysis proceeds when resolved: %v", raw)
	}
}

// TestChunk16_HandleAgentCheckPoll_ResumesPendingAutoAnalysis verifies
// handleAgentCheckPoll dispatches startAutoAnalysis when the async check
// completes successfully and pendingAutoAnalysis is true.
func TestChunk16_HandleAgentCheckPoll_ResumesPendingAutoAnalysis(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		if (typeof prSplitConfig === 'undefined') {
			prSplitConfig = {
				cleanupOnFailure: false,
				timeoutMs: 0
			};
		}

		prSplit.runtime = prSplit.runtime || {};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';
		prSplit.runtime.mode = 'auto';

		// Mock gitExec - handleAgentCheckPoll calls startAutoAnalysis which calls handleConfigState.
		var restoreGit = setupGitMock();
		try {
			setupPlanCache();
			var s = initState('CONFIG');
			s.configValidationError = null;
			s.availableBranches = [];
			s.outputLines = [];
			s.outputAutoScroll = true;
			s.autoSplitRunning = false;
			s.autoSplitResult = null;

			s.agentCheckRunning = false;
			s.agentCheckStatus = 'available';
			s.pendingAutoAnalysis = true;
			globalThis.prSplit._state.agentExecutor = {
				resolved: { command: 'agent', type: 'agent-code' },
				isAvailable: function() { return true; }
			};

			var r = await update({type: 'Tick', id: 'agent-check-poll'}, s);
			var state = r[0];

			if (state.pendingAutoAnalysis) {
				return 'FAIL: pendingAutoAnalysis should be cleared';
			}
			if (!state.autoSplitRunning) {
				return 'FAIL: expected autoSplitRunning=true after resume';
			}

			return 'OK';
		} finally {
			restoreGit();
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleAgentCheckPoll resumes pending: %v", raw)
	}
}

// TestChunk16_HandleAgentCheckPoll_FallsBackWhenUnavailable verifies
// handleAgentCheckPoll falls back to heuristic analysis when the async
// check completes with Agent unavailable and pendingAutoAnalysis is true.
func TestChunk16_HandleAgentCheckPoll_FallsBackWhenUnavailable(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		if (typeof prSplitConfig === 'undefined') {
			prSplitConfig = {
				cleanupOnFailure: false,
				timeoutMs: 0
			};
		}

		prSplit.runtime = prSplit.runtime || {};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';
		prSplit.runtime.mode = 'auto';

		// Mock gitExec - handleAgentCheckPoll calls startAnalysis which calls handleConfigState.
		var restoreGit = setupGitMock();
		try {
			setupPlanCache();
			var s = initState('CONFIG');
			s.configValidationError = null;
			s.availableBranches = [];
			s.outputLines = [];
			s.outputAutoScroll = true;

			s.agentCheckRunning = false;
			s.agentCheckStatus = 'unavailable';
			s.pendingAutoAnalysis = true;
			globalThis.prSplit._state.agentExecutor = { resolved: null };

			var r = await update({type: 'Tick', id: 'agent-check-poll'}, s);
			var state = r[0];

			if (state.pendingAutoAnalysis) {
				return 'FAIL: pendingAutoAnalysis should be cleared';
			}
			if (!state.isProcessing) {
				return 'FAIL: expected isProcessing=true (heuristic fallback)';
			}

			return 'OK';
		} finally {
			restoreGit();
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("handleAgentCheckPoll fallback: %v", raw)
	}
}

// TestChunk16_StartAutoAnalysis_NoSyncIsAvailableCall ensures the
// startAutoAnalysis code path no longer calls the synchronous isAvailable().
// An unresolved executor with a TRAPPING isAvailable (throws) must NOT
// trigger the trap — deferral should happen first via .resolved check.
func TestChunk16_StartAutoAnalysis_NoSyncIsAvailableCall(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		if (typeof prSplitConfig === 'undefined') {
			prSplitConfig = {
				cleanupOnFailure: false,
				timeoutMs: 0
			};
		}

		prSplit.runtime = prSplit.runtime || {};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.dir = '.';
		prSplit.runtime.strategy = 'directory';
		prSplit.runtime.verifyCommand = '';
		prSplit.runtime.mode = 'auto';

		// Mock gitExec - startAutoAnalysis calls handleConfigState which runs git.
		var restoreGit = setupGitMock();
		try {
			setupPlanCache();
			var s = initState('CONFIG');
			s.configValidationError = null;
			s.availableBranches = [];
			s.outputLines = [];
			s.outputAutoScroll = true;
			s.focusIndex = -1;
			s.configFieldEditing = null;

			// Create an unresolved executor with a TRAP on isAvailable.
			globalThis.prSplit._state.agentExecutor = {
				resolved: null,
				isAvailable: function() {
					throw new Error('TRAP: sync isAvailable was called');
				}
			};

			// Should NOT throw — deferral via .resolved check happens first.
			try {
				var r = await sendKey(s, 'enter');
				var state = r[0];
				if (!state.pendingAutoAnalysis) {
					return 'FAIL: expected deferral (pendingAutoAnalysis), got ' + state.pendingAutoAnalysis;
				}
				return 'OK';
			} catch (e) {
				return 'FAIL: isAvailable trap triggered: ' + e.message;
			}
		} finally {
			restoreGit();
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no sync isAvailable assertion: %v", raw)
	}
}

// TestChunk16_HandleAgentCheckPoll_NoPendingNoAction verifies that
// handleAgentCheckPoll returns [s, null] when there is no pending
// auto analysis — normal non-deferred completion path.
func TestChunk16_HandleAgentCheckPoll_NoPendingNoAction(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		var s = initState('CONFIG');

		s.agentCheckRunning = false;
		s.agentCheckStatus = 'available';
		s.pendingAutoAnalysis = false;

		var r = await update({type: 'Tick', id: 'agent-check-poll'}, s);
		var state = r[0];
		var cmd = r[1];

		if (cmd !== null) {
			return 'FAIL: expected null cmd when no pendingAutoAnalysis';
		}

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("no-pending no-action: %v", raw)
	}
}
