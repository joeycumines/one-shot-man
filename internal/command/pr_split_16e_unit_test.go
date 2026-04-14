package command

// T423: Unit tests for chunk 16e pure functions.
//
// Covers:
//   - _computeSplitPaneContentOffset: row/col offset calculated from
//     s.height and s.splitViewRatio with clamp logic
//   - _writeMouseToPane: dispatches bytes to the active split-view tab's
//     terminal session; returns true on success, false on failure

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// ────────────────────── computeSplitPaneContentOffset ──────────────────────

func TestChunk16e_ComputeOffset_DefaultState(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Default s.height=0, s.splitViewRatio=0 → falls back to C.DEFAULT_ROWS (24), ratio 0.6.
	// vpHeight = max(3, 24 - 8) = 16
	// wizardH = max(3, floor(16 * 0.6)) = max(3, 9) = 9
	// wizardH = min(9, 16 - 3 - 1) = min(9, 12) = 9
	// row = 5 + 9 = 14
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":14`) {
		t.Errorf("default offset row should be 14: %s", s)
	}
	if !strings.Contains(s, `"col":1`) {
		t.Errorf("col should always be 1: %s", s)
	}
}

func TestChunk16e_ComputeOffset_StandardHeight(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// height=40, ratio=0.6 (default).
	// vpHeight = max(3, 40 - 8) = 32
	// wizardH = max(3, floor(32 * 0.6)) = max(3, 19) = 19
	// wizardH = min(19, 32 - 3 - 1) = min(19, 28) = 19
	// row = 5 + 19 = 24
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({height: 40}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":24`) {
		t.Errorf("height=40 offset row should be 24: %s", s)
	}
}

func TestChunk16e_ComputeOffset_NarrowTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// height=12, ratio=0.6.
	// vpHeight = max(3, 12 - 8) = max(3, 4) = 4
	// wizardH = max(3, floor(4 * 0.6)) = max(3, 2) = 3
	// wizardH = min(3, 4 - 3 - 1) = min(3, 0) = 0
	// row = 5 + 0 = 5
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({height: 12}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":5`) {
		t.Errorf("height=12 offset row should be 5: %s", s)
	}
}

func TestChunk16e_ComputeOffset_TinyTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// height=8, ratio=0.5.
	// vpHeight = max(3, 8 - 8) = max(3, 0) = 3
	// wizardH = max(3, floor(3 * 0.5)) = max(3, 1) = 3
	// wizardH = min(3, 3 - 3 - 1) = min(3, -1) = -1
	// row = 5 + (-1) = 4
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({height: 8, splitViewRatio: 0.5}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":4`) {
		t.Errorf("height=8 offset row should be 4: %s", s)
	}
}

func TestChunk16e_ComputeOffset_CustomRatio(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// height=30, ratio=0.8.
	// vpHeight = max(3, 30 - 8) = 22
	// wizardH = max(3, floor(22 * 0.8)) = max(3, 17) = 17
	// wizardH = min(17, 22 - 3 - 1) = min(17, 18) = 17
	// row = 5 + 17 = 22
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({height: 30, splitViewRatio: 0.8}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":22`) {
		t.Errorf("height=30,ratio=0.8 offset row should be 22: %s", s)
	}
}

func TestChunk16e_ComputeOffset_LowRatio(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// height=30, ratio=0.2.
	// vpHeight = max(3, 30 - 8) = 22
	// wizardH = max(3, floor(22 * 0.2)) = max(3, 4) = 4
	// wizardH = min(4, 22 - 3 - 1) = min(4, 18) = 4
	// row = 5 + 4 = 9
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({height: 30, splitViewRatio: 0.2}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":9`) {
		t.Errorf("height=30,ratio=0.2 offset row should be 9: %s", s)
	}
}

func TestChunk16e_ComputeOffset_VeryLargeTerminal(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// height=100, ratio=0.6.
	// vpHeight = max(3, 100 - 8) = 92
	// wizardH = max(3, floor(92 * 0.6)) = max(3, 55) = 55
	// wizardH = min(55, 92 - 3 - 1) = min(55, 88) = 55
	// row = 5 + 55 = 60
	val, err := evalJS(`JSON.stringify(prSplit._computeSplitPaneContentOffset({height: 100}))`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"row":60`) {
		t.Errorf("height=100 offset row should be 60: %s", s)
	}
}

// ────────────────────── writeMouseToPane ──────────────────────

func TestChunk16e_WriteMouseToPane_ClaudeTab(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var written = [];
			var __mockCID = 42;
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = __mockCID;
			globalThis.tuiMux = {
				writeToChild: function(b) { written.push(b); },
				snapshot: function(id) { return { fullScreen: '', plainText: '' }; },
				isDone: function(id) { return false; },
				activeID: function() { return __mockCID; },
				activate: function(id) {},
				input: function(data) { written.push(data); },
			};
			try {
				var s = {splitViewTab: 'claude'};
				var ok = prSplit._writeMouseToPane('test-bytes', s);
				return JSON.stringify({ok: ok, written: written});
			} finally {
				delete globalThis.tuiMux;
				if (prSplit._state) prSplit._state.claudeSessionID = null;
			}
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"ok":true`) {
		t.Errorf("claude tab write should succeed: %s", s)
	}
	if !strings.Contains(s, `"written":["test-bytes"]`) {
		t.Errorf("bytes should be written to tuiMux: %s", s)
	}
}

func TestChunk16e_WriteMouseToPane_ClaudeTabSessionWrapper(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var written = [];
			var __mockCID = 42;
			prSplit._state = prSplit._state || {};
			prSplit._state.claudeSessionID = __mockCID;
			globalThis.tuiMux = {
				session: function() {
					return {
						write: function(b) { written.push(b); }
					};
				},
				snapshot: function(id) { return { fullScreen: '', plainText: '' }; },
				isDone: function(id) { return false; },
				activeID: function() { return __mockCID; },
				activate: function(id) {},
				input: function(data) { written.push(data); },
			};
			try {
				var s = {splitViewTab: 'claude'};
				var ok = prSplit._writeMouseToPane('wrapped-bytes', s);
				return JSON.stringify({ok: ok, written: written});
			} finally {
				delete globalThis.tuiMux;
				if (prSplit._state) prSplit._state.claudeSessionID = null;
			}
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"ok":true`) {
		t.Errorf("claude session wrapper write should succeed: %s", s)
	}
	if !strings.Contains(s, `"written":["wrapped-bytes"]`) {
		t.Errorf("bytes should be written through session wrapper: %s", s)
	}
}

func TestChunk16e_WriteMouseToPane_VerifyTab(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var written = [];
			var s = {
				splitViewTab: 'verify',
				activeVerifySession: {write: function(b) { written.push(b); }},
			};
			var ok = prSplit._writeMouseToPane('verify-bytes', s);
			return JSON.stringify({ok: ok, written: written});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if !strings.Contains(s, `"ok":true`) {
		t.Errorf("verify tab write should succeed: %s", s)
	}
	if !strings.Contains(s, `"written":["verify-bytes"]`) {
		t.Errorf("bytes should be written to verify session: %s", s)
	}
}

func TestChunk16e_WriteMouseToPane_NoSession(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var results = [];
			// claude tab but no tuiMux.
			results.push(prSplit._writeMouseToPane('x', {splitViewTab: 'claude'}));
			// verify tab but no session.
			results.push(prSplit._writeMouseToPane('x', {splitViewTab: 'verify'}));
			// unknown tab.
			results.push(prSplit._writeMouseToPane('x', {splitViewTab: 'unknown'}));
			// empty state.
			results.push(prSplit._writeMouseToPane('x', {}));
			return JSON.stringify(results);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s != "[false,false,false,false]" {
		t.Errorf("all should return false when no session: %s", s)
	}
}

func TestChunk16e_WriteMouseToPane_WriteThrows(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`
		(function() {
			var results = [];
			// claude tab with throwing writeToChild.
			globalThis.tuiMux = {
				writeToChild: function() { throw new Error('claude-fail'); },
			};
			results.push(prSplit._writeMouseToPane('x', {splitViewTab: 'claude'}));
			delete globalThis.tuiMux;

			// verify tab with throwing write.
			results.push(prSplit._writeMouseToPane('x', {
				splitViewTab: 'verify',
				activeVerifySession: {write: function() { throw new Error('verify-fail'); }},
			}));

			return JSON.stringify(results);
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	s := val.(string)
	if s != "[false,false]" {
		t.Errorf("all should return false when write throws: %s", s)
	}
}
