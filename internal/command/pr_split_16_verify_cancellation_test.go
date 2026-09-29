package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ---------------------------------------------------------------------------
//  T386: keyToTermBytes audit — comprehensive key mapping correctness
// ---------------------------------------------------------------------------

// TestKeyToTermBytes_SpecialKeys_T386 validates all named key → escape sequence
// mappings in keyToTermBytes against standard VT100/xterm terminal sequences.
func TestKeyToTermBytes_SpecialKeys_T386(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var fn = globalThis.prSplit._keyToTermBytes;
		var errors = [];
		function check(name, got, want) {
			if (got !== want) {
				var gotHex = '';
				for (var i = 0; i < (got||'').length; i++) gotHex += got.charCodeAt(i).toString(16).padStart(2, '0');
				var wantHex = '';
				for (var j = 0; j < want.length; j++) wantHex += want.charCodeAt(j).toString(16).padStart(2, '0');
				errors.push(name + ': got 0x' + gotHex + ', want 0x' + wantHex);
			}
		}

		// Basic keys.
		check('enter', fn('enter'), '\r');
		check('tab', fn('tab'), '\t');
		check('shift+tab', fn('shift+tab'), '\x1b[Z');
		check('backspace', fn('backspace'), '\x7f');
		check('space (literal)', fn(' '), ' ');
		check('esc', fn('esc'), '\x1b');
		check('delete', fn('delete'), '\x1b[3~');

		// Arrow keys.
		check('up', fn('up'), '\x1b[A');
		check('down', fn('down'), '\x1b[B');
		check('right', fn('right'), '\x1b[C');
		check('left', fn('left'), '\x1b[D');

		// Navigation.
		check('home', fn('home'), '\x1b[H');
		check('end', fn('end'), '\x1b[F');
		check('pgup', fn('pgup'), '\x1b[5~');
		check('pgdown', fn('pgdown'), '\x1b[6~');
		check('insert', fn('insert'), '\x1b[2~');

		// Function keys (VT220/xterm sequences).
		check('f1', fn('f1'), '\x1bOP');
		check('f2', fn('f2'), '\x1bOQ');
		check('f3', fn('f3'), '\x1bOR');
		check('f4', fn('f4'), '\x1bOS');
		check('f5', fn('f5'), '\x1b[15~');
		check('f6', fn('f6'), '\x1b[17~');
		check('f7', fn('f7'), '\x1b[18~');
		check('f8', fn('f8'), '\x1b[19~');
		check('f9', fn('f9'), '\x1b[20~');
		check('f10', fn('f10'), '\x1b[21~');
		check('f11', fn('f11'), '\x1b[23~');
		check('f12', fn('f12'), '\x1b[24~');

		// Ctrl+letter → control characters.
		check('ctrl+a', fn('ctrl+a'), '\x01');
		check('ctrl+c', fn('ctrl+c'), '\x03');
		check('ctrl+d', fn('ctrl+d'), '\x04');
		check('ctrl+z', fn('ctrl+z'), '\x1a');
		check('ctrl+A', fn('ctrl+A'), '\x01');
		check('ctrl+Z', fn('ctrl+Z'), '\x1a');

		// Alt+key → ESC prefix.
		check('alt+a', fn('alt+a'), '\x1ba');
		check('alt+enter', fn('alt+enter'), '\x1b\r');
		check('alt+up', fn('alt+up'), '\x1b\x1b[A');

		// Bracketed paste.
		check('paste[hello]', fn('[hello]'), 'hello');

		// Single char passthrough.
		check('char-a', fn('a'), 'a');
		check('char-Z', fn('Z'), 'Z');
		check('char-1', fn('1'), '1');

		// Unknown modifier returns null.
		if (fn('super+a') !== null) errors.push('super+a should return null');

		// T386: Modifier+arrow keys (xterm CSI {modifier} sequences).
		// Shift+arrows (modifier 2).
		check('shift+up', fn('shift+up'), '\x1b[1;2A');
		check('shift+down', fn('shift+down'), '\x1b[1;2B');
		check('shift+left', fn('shift+left'), '\x1b[1;2D');
		check('shift+right', fn('shift+right'), '\x1b[1;2C');
		check('shift+home', fn('shift+home'), '\x1b[1;2H');
		check('shift+end', fn('shift+end'), '\x1b[1;2F');

		// Ctrl+arrows (modifier 5).
		check('ctrl+up', fn('ctrl+up'), '\x1b[1;5A');
		check('ctrl+down', fn('ctrl+down'), '\x1b[1;5B');
		check('ctrl+left', fn('ctrl+left'), '\x1b[1;5D');
		check('ctrl+right', fn('ctrl+right'), '\x1b[1;5C');

		// Ctrl+Shift+arrows (modifier 6).
		check('ctrl+shift+up', fn('ctrl+shift+up'), '\x1b[1;6A');
		check('ctrl+shift+down', fn('ctrl+shift+down'), '\x1b[1;6B');
		check('ctrl+shift+left', fn('ctrl+shift+left'), '\x1b[1;6D');
		check('ctrl+shift+right', fn('ctrl+shift+right'), '\x1b[1;6C');

		// Modifier+tilde keys.
		check('shift+pgup', fn('shift+pgup'), '\x1b[5;2~');
		check('shift+pgdown', fn('shift+pgdown'), '\x1b[6;2~');
		check('ctrl+delete', fn('ctrl+delete'), '\x1b[3;5~');
		check('shift+insert', fn('shift+insert'), '\x1b[2;2~');

		return errors.length > 0 ? errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("keyToTermBytes audit: %v", raw)
	}
}

// TestInteractiveReservedKeys_T386 verifies that the shell tab's reserved key
// set correctly allows navigation keys through while blocking pane-management keys.
func TestInteractiveReservedKeys_T386(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {
		var errors = [];
		var reservedKeys = globalThis.prSplit._INTERACTIVE_RESERVED_KEYS;
		if (!reservedKeys) return 'FAIL: INTERACTIVE_RESERVED_KEYS not exported';

		// These pane-management keys MUST be reserved.
		var mustReserve = ['ctrl+tab', 'ctrl+l', 'ctrl+o', 'ctrl+]', 'ctrl++', 'ctrl+=', 'ctrl+-', 'f1'];
		for (var i = 0; i < mustReserve.length; i++) {
			if (!reservedKeys[mustReserve[i]]) {
				errors.push(mustReserve[i] + ' should be reserved in INTERACTIVE_RESERVED_KEYS');
			}
		}

		// These navigation keys MUST NOT be reserved (shell needs them).
		var mustForward = ['up', 'down', 'left', 'right', 'j', 'k', 'pgup', 'pgdown', 'home', 'end'];
		for (var j = 0; j < mustForward.length; j++) {
			if (reservedKeys[mustForward[j]]) {
				errors.push(mustForward[j] + ' should NOT be reserved in INTERACTIVE_RESERVED_KEYS');
			}
		}

		return errors.length > 0 ? errors.join('; ') : 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("INTERACTIVE_RESERVED_KEYS: %v", raw)
	}
}

func TestVerifyRunBranch_CancelDisposesLateWorktree(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalPrepare = prSplit.prepareVerifyWorktree;
		var originalCleanup = prSplit.cleanupVerifyWorktree;
		var resolvePrepare;
		var cleaned = [];

		prSplit.canSpawnInteractiveShell = function() { return true; };
		prSplit.prepareVerifyWorktree = function() {
			return new Promise(function(resolve) { resolvePrepare = resolve; });
		};
		prSplit.cleanupVerifyWorktree = function(dir, worktree) {
			cleaned.push([dir, worktree]);
			return Promise.resolve();
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;

			var first = update({type: 'Tick', id: 'verify-branch'}, s);
			if (!first[0]._verifySetup || !first[0]._verifySetup.pending) {
				return 'FAIL: worktree setup was not deferred';
			}
			prSplit._clearVerifyPaneSession(first[0], { debugPrefix: 'cancel-test', keepDisplay: false });
			if (first[0]._verifySetup !== null) {
				return 'FAIL: cancel did not invalidate pending setup';
			}
			resolvePrepare({ worktreeDir: '/tmp/late-worktree', dir: '/tmp/repo' });
			await Promise.resolve();
			await Promise.resolve();
			if (cleaned.length !== 1 || cleaned[0][1] !== '/tmp/late-worktree') {
				return 'FAIL: late worktree was not cleaned: ' + JSON.stringify(cleaned);
			}
			return 'OK';
		} finally {
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.prepareVerifyWorktree = originalPrepare;
			prSplit.cleanupVerifyWorktree = originalCleanup;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("late worktree cleanup: %v", raw)
	}
}

func TestVerifyRunBranch_CancelDisposesLateOneShotSession(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalStart = prSplit.startVerifySession;
		var originalCleanup = prSplit.cleanupVerifyWorktree;
		var resolveStart;
		var closed = 0;
		var cleaned = [];

		prSplit.canSpawnInteractiveShell = function() { return false; };
		prSplit.startVerifySession = function() {
			return new Promise(function(resolve) { resolveStart = resolve; });
		};
		prSplit.cleanupVerifyWorktree = function(dir, worktree) {
			cleaned.push([dir, worktree]);
			return Promise.resolve();
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;

			var first = update({type: 'Tick', id: 'verify-branch'}, s);
			if (!first[0]._verifySetup || !first[0]._verifySetup.pending) {
				return 'FAIL: one-shot setup was not deferred';
			}
			prSplit._clearVerifyPaneSession(first[0], { debugPrefix: 'cancel-test', keepDisplay: false });
			resolveStart({
				session: { close: function() { closed++; } },
				worktreeDir: '/tmp/late-session-worktree',
				dir: '/tmp/repo'
			});
			await Promise.resolve();
			await Promise.resolve();
			if (closed !== 1 || cleaned.length !== 1 || cleaned[0][1] !== '/tmp/late-session-worktree') {
				return 'FAIL: late session was not disposed: ' + JSON.stringify({closed: closed, cleaned: cleaned});
			}
			return 'OK';
		} finally {
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.startVerifySession = originalStart;
			prSplit.cleanupVerifyWorktree = originalCleanup;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("late session cleanup: %v", raw)
	}
}

func TestVerifyRunBranch_ReplacementDisposesOldSetup(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(async function() {
		setupPlanCache();
		prSplit.runtime.dir = '.';
		prSplit.runtime.verifyCommand = 'make test';
		var originalCanSpawn = prSplit.canSpawnInteractiveShell;
		var originalPrepare = prSplit.prepareVerifyWorktree;
		var originalSpawn = prSplit.spawnShellSession;
		var originalCleanup = prSplit.cleanupVerifyWorktree;
		var resolvers = [];
		var cleaned = [];
		var prepareCalls = 0;

		prSplit.canSpawnInteractiveShell = function() { return true; };
		prSplit.prepareVerifyWorktree = function() {
			prepareCalls++;
			return new Promise(function(resolve) { resolvers.push(resolve); });
		};
		prSplit.spawnShellSession = function() {
			return {
				screen: function() { return ''; },
				output: function() { return ''; },
				isDone: function() { return false; },
				interrupt: function() {},
				kill: function() {},
				pause: function() {},
				resume: function() {}
			};
		};
		prSplit.cleanupVerifyWorktree = function(dir, worktree) {
			cleaned.push([dir, worktree]);
			return Promise.resolve();
		};

		try {
			var s = initState('EXECUTING');
			s.isProcessing = true;
			s.verificationResults = [];
			s.verifyingIdx = 0;
			s.verifyOutput = {};
			s.width = 100;
			s.height = 30;
			var first = update({type: 'Tick', id: 'verify-branch'}, s);
			prSplit._clearVerifyPaneSession(first[0], { debugPrefix: 'restart-test', keepDisplay: false });
			var second = update({type: 'Tick', id: 'verify-branch'}, first[0]);
			if (prepareCalls !== 2 || !second[0]._verifySetup || !second[0]._verifySetup.pending) {
				return 'FAIL: replacement setup was not pending: ' + prepareCalls;
			}
			resolvers[0]({ worktreeDir: '/tmp/old-worktree', dir: '/tmp/repo' });
			await Promise.resolve();
			await Promise.resolve();
			if (cleaned.length !== 1 || cleaned[0][1] !== '/tmp/old-worktree' || !second[0]._verifySetup.pending) {
				return 'FAIL: old setup was not isolated from replacement: ' + JSON.stringify(cleaned);
			}
			resolvers[1]({ worktreeDir: '/tmp/new-worktree', dir: '/tmp/repo' });
			await Promise.resolve();
			await Promise.resolve();
			var third = update({type: 'Tick', id: 'verify-branch'}, second[0]);
			if (third[0].verifyMode !== 'interactive' || third[0].activeVerifyBranch !== 'split/api') {
				return 'FAIL: replacement setup did not start verification';
			}
			return 'OK';
		} finally {
			prSplit.canSpawnInteractiveShell = originalCanSpawn;
			prSplit.prepareVerifyWorktree = originalPrepare;
			prSplit.spawnShellSession = originalSpawn;
			prSplit.cleanupVerifyWorktree = originalCleanup;
		}
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("replacement setup cleanup: %v", raw)
	}
}
