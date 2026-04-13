package prsplittest

import (
	"testing"
)

// SetupTUIMocks is JS that installs minimal tui/ctx/output/log mocks so that
// TUI chunks (13–16f) can load without errors. Evaluated between chunks 00–12
// and chunks 13+ by [NewTUIEngineE].
//
// Injected globals:
//
//   - _prints ([]string): captures output.print calls
//   - _registeredModes ([]object): captures tui.registerMode calls
//   - _switchedModes ([]string): captures tui.switchMode calls
//   - _ctxRuns (map[string]func): captures ctx.run registrations
//   - tui: mock with createState, registerMode, switchMode
//   - ctx: mock with run (registers and immediately invokes)
//   - output: mock with print (→ _prints) and toClipboard
//   - log: silent mock with error, info, debug, warn, printf
const SetupTUIMocks = `
(function() {
    globalThis._prints = [];
    globalThis._registeredModes = [];
    globalThis._switchedModes = [];
    globalThis._ctxRuns = {};

    globalThis.tui = {
        createState: function(name, def) {
            return { _name: name, _def: def };
        },
        registerMode: function(m) {
            globalThis._registeredModes.push(m);
        },
        switchMode: function(name) {
            globalThis._switchedModes.push(name);
        }
    };

    globalThis.ctx = {
        run: function(name, fn) {
            globalThis._ctxRuns[name] = fn;
            fn();
        }
    };

    globalThis.output = {
        print: function(s) { globalThis._prints.push(String(s)); },
        toClipboard: function(s) { globalThis._clipboardContent = s; }
    };

    globalThis.log = {
        error: function() {},
        info: function() {},
        debug: function() {},
        warn: function() {},
        printf: function() {}
    };
})();
`

// Chunk16Helpers is injected once after [NewTUIEngine] to provide shared test
// utilities for chunk 16+ tests. Used by [NewTUIEngineWithHelpers].
//
// Injected JS functions:
//
//   - initState(targetState, opts): creates a _wizardInit() state transitioned
//     to targetState via the wizard FSM. Supports CONFIG, PLAN_REVIEW,
//     PLAN_EDITOR, BRANCH_BUILDING, EQUIV_CHECK, ERROR_RESOLUTION, FINALIZATION.
//   - update(msg, s): wrapper for prSplit._wizardUpdate(msg, s)
//   - sendKey(s, key): sends a Key message via update
//   - sendClick(s): sends a left mouse click at (10,10) via update
//   - sendWheel(s, direction): sends a mouse wheel event via update
//   - mockZoneHit(zoneId): mocks zone.inBounds to match only zoneId;
//     returns a restore function (MUST use in try/finally)
//   - setupPlanCache(): installs a 3-split test plan (split/api, split/cli,
//     split/docs) into prSplit._state.planCache
//   - setupGitMock(): mocks prSplit._gitExec to return success for common
//     git commands, avoiding dependency on git availability (isolated tests)
const Chunk16Helpers = `
// setupGitMock: mocks prSplit._gitExec to avoid depending on git availability.
// TUI tests should be isolated and not require git in PATH.
// Returns restore function (MUST use in try/finally blocks).
function setupGitMock() {
    var origGitExec = globalThis.prSplit._gitExec;
    globalThis.prSplit._gitExec = function(dir, args) {
        // Return success for common git commands used by handleConfigState
        if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref' && args[2] === 'HEAD') {
            return {stdout: 'feature', stderr: '', code: 0};
        }
        if (args[0] === 'rev-parse' && args[2] === 'refs/heads/main') {
            return {stdout: 'abc123', stderr: '', code: 0};
        }
        if (args[0] === 'merge-base' && args[1] === 'HEAD') {
            return {stdout: 'def456', stderr: '', code: 0};
        }
        if (args[0] === 'diff' && args[1] === '--name-only') {
            return {stdout: 'file1.go\nfile2.go\n', stderr: '', code: 0};
        }
        if (args[0] === 'config' && args[1] === '--get') {
            return {stdout: 'true', stderr: '', code: 0};
        }
        // Default success response
        return {stdout: '', stderr: '', code: 0};
    };
    return function() { globalThis.prSplit._gitExec = origGitExec; };
}

// initState: creates a _wizardInit() state properly transitioned to targetState.
function initState(targetState, opts) {
    opts = opts || {};
    var s = globalThis.prSplit._wizardInit();
    s.needsInitClear = false;
    s.width = opts.width || 80;
    s.height = opts.height || 24;
    s.isProcessing = false;
    s.selectedSplitIdx = opts.selectedSplitIdx || 0;
    s.selectedFileIdx = opts.selectedFileIdx || 0;
    s.focusIndex = opts.focusIndex || 0;

    s.wizard.reset();
    var paths = {
        'CONFIG':           ['CONFIG'],
        'PLAN_REVIEW':      ['CONFIG','PLAN_GENERATION','PLAN_REVIEW'],
        'PLAN_EDITOR':      ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','PLAN_EDITOR'],
        'BRANCH_BUILDING':  ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING'],
        'EQUIV_CHECK':      ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING','EQUIV_CHECK'],
        'ERROR_RESOLUTION': ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING','ERROR_RESOLUTION'],
        'FINALIZATION':     ['CONFIG','PLAN_GENERATION','PLAN_REVIEW','BRANCH_BUILDING','EQUIV_CHECK','FINALIZATION']
    };
    var p = paths[targetState];
    if (p) {
        for (var i = 0; i < p.length; i++) s.wizard.transition(p[i]);
    }
    s.wizardState = targetState;
    s._prevWizardState = targetState;
    return s;
}

// update: wrapper for _wizardUpdate.
function update(msg, s) {
    return globalThis.prSplit._wizardUpdate(msg, s);
}

// sendKey: sends a Key message with v2 named-key normalization.
// BubbleTea v2 converts character codes to named keys (e.g. ' ' → 'space',
// '\t' → 'tab'). The test helper must match this behaviour.
function sendKey(s, key) {
    var nameMap = {' ': 'space', '\t': 'tab', '\r': 'enter', '\n': 'enter'};
    var k = nameMap[key] || key;
    return update({type: 'Key', key: k}, s);
}

// sendClick: sends a left mouse click (v2 split type).
function sendClick(s) {
    return update({type: 'MouseClick', button: 'left', x: 10, y: 10, mod: []}, s);
}

// sendWheel: sends a mouse wheel event (v2 split type).
function sendWheel(s, direction) {
    return update({type: 'MouseWheel', button: 'wheel ' + direction, x: 10, y: 10, mod: []}, s);
}

// mockZoneHit: mocks zone.inBounds to match only the given zone ID.
// Returns restore fn. MUST be used in try/finally blocks.
function mockZoneHit(zoneId) {
    var z = globalThis.prSplit._zone;
    var orig = z.inBounds;
    z.inBounds = function(id) { return id === zoneId; };
    return function() { z.inBounds = orig; };
}

// setupPlanCache: sets up a 3-split test plan.
function setupPlanCache() {
    globalThis.prSplit._state.planCache = {
        baseBranch: 'main',
        sourceBranch: 'feature',
        splits: [
            {name: 'split/api', files: ['pkg/handler.go', 'pkg/types.go'], message: 'API split', order: 0},
            {name: 'split/cli', files: ['cmd/serve.go', 'cmd/main.go'], message: 'CLI split', order: 1},
            {name: 'split/docs', files: ['README.md'], message: 'Docs split', order: 2}
        ]
    };
}
`

// NewTUIEngineE loads chunks 00–12, injects TUI mocks, then loads chunks
// 13–16f. Returns the [Engine] wrapper so callers can access the underlying
// [scripting.Engine] (e.g. for RunOnLoopSync in hang-reproducer tests).
//
// Most tests should prefer [NewTUIEngine] or [NewTUIEngineWithHelpers].
func NewTUIEngineE(t testing.TB) *Engine {
	t.Helper()

	eng := NewEngine(t, nil)
	eng.LoadChunks(t, ChunkNamesThrough("12")...)
	evalJS := eng.EvalJS(t)

	// Inject TUI mocks between chunk 12 and chunk 13.
	if _, err := evalJS(SetupTUIMocks); err != nil {
		t.Fatalf("prsplittest: TUI mocks failed: %v", err)
	}

	// Load TUI chunks (13+) via proper script loading (preserves script
	// names in stack traces for debugging).
	eng.LoadChunks(t, ChunkNamesAfter("12")...)

	return eng
}

// NewTUIEngine loads chunks 00–12, injects TUI mocks, then loads chunks
// 13–16f. Returns an evalJS function ready for TUI-level testing.
//
// This is a drop-in replacement for loadTUIEngine in pr_split_13_tui_test.go.
func NewTUIEngine(t testing.TB) func(string) (any, error) {
	t.Helper()
	return NewTUIEngineE(t).EvalJS(t)
}

// NewTUIEngineWithHelpers loads the full TUI engine and injects
// [Chunk16Helpers]. Returns an evalJS function ready for chunk-16-level tests.
//
// This is a drop-in replacement for loadTUIEngineWithHelpers in
// pr_split_16_helpers_test.go.
func NewTUIEngineWithHelpers(t testing.TB) func(string) (any, error) {
	t.Helper()
	eng := NewTUIEngineE(t)
	evalJS := eng.EvalJS(t)
	if _, err := evalJS(Chunk16Helpers); err != nil {
		t.Fatalf("prsplittest: chunk16 helpers failed: %v", err)
	}
	return evalJS
}
