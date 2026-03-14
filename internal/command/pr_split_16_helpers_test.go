package command

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  T020: Comprehensive keyboard & mouse event handling tests for chunk 16
//
//  Covers: overlays (report, editor dialogs, Claude conversation, inline
//  title edit), live verify session, split-view, all mouse zone clicks,
//  focus activation, plan editor keys, navigation handlers, and edge cases.
//
//  Does NOT duplicate tests already in pr_split_13_tui_test.go (help toggle,
//  ctrl+c, confirm cancel y/n/esc/enter, WindowSize, j/k navigation in
//  PLAN_REVIEW, esc back, plan editor shortcut 'e', mouse wheel scroll,
//  msg.string regression, AllKeyBindingsRespond).
// ---------------------------------------------------------------------------

// chunk16Helpers is injected once after loadTUIEngine to provide shared test
// utilities (state initializer, mock helpers, message helpers).
const chunk16Helpers = `
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

// loadTUIEngineWithHelpers loads the full TUI engine and injects chunk16Helpers.
func loadTUIEngineWithHelpers(t testing.TB) func(string) (any, error) {
	t.Helper()
	evalJS := loadTUIEngine(t)
	if _, err := evalJS(chunk16Helpers); err != nil {
		t.Fatalf("failed to inject chunk16 helpers: %v", err)
	}
	return evalJS
}

// numVal safely extracts a numeric value from Goja results which may return
// int64 or float64 depending on the JS value. Returns float64 for comparison.
func numVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}
