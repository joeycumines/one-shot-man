package prsplittest

import (
	"testing"
)

// SetupTUIMocks is JS that sets up minimal tui/ctx/output/log mocks
// so the TUI chunk's guard allows execution.
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
// utilities (state initializer, mock helpers, message helpers) for chunk 16
// tests.
const Chunk16Helpers = `
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

// sendKey: sends a Key message.
function sendKey(s, key) {
    return update({type: 'Key', key: key}, s);
}

// sendClick: sends a left mouse click.
function sendClick(s) {
    return update({type: 'Mouse', button: 'left', action: 'press', isWheel: false, x: 10, y: 10}, s);
}

// sendWheel: sends a mouse wheel event.
function sendWheel(s, direction) {
    return update({type: 'Mouse', button: 'wheel ' + direction, action: 'press', isWheel: true, x: 10, y: 10}, s);
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
