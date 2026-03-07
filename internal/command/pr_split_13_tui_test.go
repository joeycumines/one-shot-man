package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  Chunk 13: TUI — command dispatch, buildReport, mode registration
// ---------------------------------------------------------------------------

// allChunksForTUI lists all 14 chunks needed for full TUI tests.
// Not used directly with loadChunkEngine (since TUI needs mock globals
// injected between 00-12 and 13), but referenced in documentation.
var _ = []string{ // compile-time proof the list is valid
	"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	"05_execution", "06_verification", "07_prcreation", "08_conflict",
	"09_claude", "10_pipeline", "11_utilities", "12_exports", "13_tui",
}

// setupTUIMocks is JS that sets up minimal tui/ctx/output/log mocks
// so the TUI chunk's guard allows execution.
const setupTUIMocks = `
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

// loadTUIEngine loads chunks 00-12, injects TUI mocks, then loads chunk 13.
// Returns evalJS function.
func loadTUIEngine(t testing.TB) func(string) (any, error) {
	t.Helper()

	evalJS := loadChunkEngine(t, nil, allChunksThrough12...)

	// Inject TUI mocks.
	if _, err := evalJS(setupTUIMocks); err != nil {
		t.Fatalf("failed to inject TUI mocks: %v", err)
	}

	// Evaluate chunk 13 source directly.
	if _, err := evalJS(prSplitChunk13TUI); err != nil {
		t.Fatalf("failed to load chunk 13: %v", err)
	}

	return evalJS
}

// TestChunk13_GuardSkipsWithoutTUI verifies that when tui/ctx/output are
// absent, the IIFE exits cleanly without setting _buildCommands or _buildReport.
func TestChunk13_GuardSkipsWithoutTUI(t *testing.T) {
	// Load chunks 00-12 (the scripting engine provides tui/ctx/output by
	// default — explicitly remove them so the guard fires).
	evalJS := loadChunkEngine(t, nil, allChunksThrough12...)

	// Clear TUI globals so the guard bails.
	if _, err := evalJS(`
		delete globalThis.tui;
		delete globalThis.ctx;
		delete globalThis.output;
	`); err != nil {
		t.Fatal(err)
	}

	// Chunk 13 should not crash even without tui/ctx/output.
	if _, err := evalJS(prSplitChunk13TUI); err != nil {
		t.Fatalf("chunk 13 should not error without TUI globals: %v", err)
	}

	// _buildCommands should NOT be defined since the guard bailed out.
	raw, err := evalJS(`typeof globalThis.prSplit._buildCommands`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "undefined" {
		t.Errorf("_buildCommands should be undefined without TUI globals, got type %v", raw)
	}
}

// TestChunk13_BuildCommandsRegistered verifies that _buildCommands and
// _buildReport are defined when TUI globals are present.
func TestChunk13_BuildCommandsRegistered(t *testing.T) {
	evalJS := loadTUIEngine(t)

	checkDefined := func(name string) {
		t.Helper()
		raw, err := evalJS(`typeof globalThis.prSplit.` + name)
		if err != nil {
			t.Fatalf("typeof prSplit.%s: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("prSplit.%s should be function, got %v", name, raw)
		}
	}
	checkDefined("_buildCommands")
	checkDefined("_buildReport")
}

// TestChunk13_AllCommandNames verifies that buildCommands returns an object
// with all expected command names (including abort and override).
func TestChunk13_AllCommandNames(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(Object.keys(globalThis.prSplit._buildCommands({})).sort())`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal([]byte(raw.(string)), &names); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"abort", "analyze", "auto-split", "cleanup", "conversation", "copy",
		"create-prs", "diff", "edit-plan", "equivalence", "execute",
		"fix", "graph", "group", "help", "hud", "load-plan", "merge", "move",
		"override", "plan", "preview", "rename", "reorder", "report", "retro",
		"run", "save-plan", "set", "stats", "telemetry", "verify",
	}

	if len(names) != len(expected) {
		t.Fatalf("command count = %d, want %d\n  got:  %v\n  want: %v",
			len(names), len(expected), names, expected)
	}
	for i, name := range expected {
		if i >= len(names) || names[i] != name {
			t.Errorf("command[%d] = %q, want %q", i, names[i], name)
		}
	}
}

// TestChunk13_ModeRegistered verifies that the mode was registered via
// tui.registerMode with the correct name.
func TestChunk13_ModeRegistered(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis._registeredModes.length`)
	if err != nil {
		t.Fatal(err)
	}
	count, ok := raw.(int64)
	if !ok || count < 1 {
		t.Fatalf("expected at least 1 registered mode, got %v", raw)
	}

	raw, err = evalJS(`globalThis._registeredModes[0].name`)
	if err != nil {
		t.Fatal(err)
	}
	name, _ := raw.(string)
	if name != "pr-split" {
		t.Errorf("registered mode name = %q, want %q", name, "pr-split")
	}

	// Check switchMode was called (enter-pr-split ctx.run).
	raw, err = evalJS(`globalThis._switchedModes.length`)
	if err != nil {
		t.Fatal(err)
	}
	switchCount, ok := raw.(int64)
	if !ok || switchCount < 1 {
		t.Fatalf("expected switchMode called, got %v", raw)
	}
}

// TestChunk13_BuildReportStructure verifies the report structure when
// no analysis has been done yet.
func TestChunk13_BuildReportStructure(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._buildReport())`)
	if err != nil {
		t.Fatal(err)
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(raw.(string)), &report); err != nil {
		t.Fatal(err)
	}

	// Version should be set.
	if v, ok := report["version"].(string); !ok || v == "" {
		t.Errorf("report.version missing or empty: %v", report["version"])
	}

	// analysis/groups/plan should be null when no work done.
	if report["analysis"] != nil {
		t.Errorf("report.analysis should be null, got %v", report["analysis"])
	}
	if report["groups"] != nil {
		t.Errorf("report.groups should be null, got %v", report["groups"])
	}
	if report["plan"] != nil {
		t.Errorf("report.plan should be null, got %v", report["plan"])
	}
}

// TestChunk13_HelpCommand verifies the help handler prints expected content.
func TestChunk13_HelpCommand(t *testing.T) {
	evalJS := loadTUIEngine(t)

	_, err := evalJS(`
        globalThis._prints = [];
        var cmds = globalThis.prSplit._buildCommands({});
        cmds.help.handler();
    `)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis._prints)`)
	if err != nil {
		t.Fatal(err)
	}
	var prints []string
	if err := json.Unmarshal([]byte(raw.(string)), &prints); err != nil {
		t.Fatal(err)
	}

	// Should contain "PR Split Commands:" header.
	found := false
	for _, line := range prints {
		if strings.Contains(line, "PR Split Commands") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("help output missing 'PR Split Commands' header, got %d lines", len(prints))
	}

	// Check that it mentions key commands.
	joined := strings.Join(prints, "\n")
	for _, cmd := range []string{"analyze", "group", "plan", "run", "execute", "auto-split"} {
		if !strings.Contains(joined, cmd) {
			t.Errorf("help output missing mention of %q", cmd)
		}
	}
}

// TestChunk13_AnalyzeMockAnalyzeDiff verifies the analyze command calls
// prSplit.analyzeDiff and stores results in _state.analysisCache.
// NOTE: We mock analyzeDiff directly (not _gitExec) because chunk 01
// captures _gitExec at module load time.
func TestChunk13_AnalyzeMockAnalyzeDiff(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Mock analyzeDiff to return controlled data.
	_, err := evalJS(`
        globalThis._prints = [];
        globalThis.prSplit.analyzeDiff = function(opts) {
            return {
                files: ['src/a.go', 'src/b.go'],
                fileStatuses: { 'src/a.go': 'M', 'src/b.go': 'A' },
                error: null,
                baseBranch: opts.baseBranch || 'main',
                currentBranch: 'feature-branch'
            };
        };
    `)
	if err != nil {
		t.Fatal(err)
	}

	_, err = evalJS(`
        var cmds = globalThis.prSplit._buildCommands({});
        cmds.analyze.handler([]);
    `)
	if err != nil {
		t.Fatal(err)
	}

	// Check that analysisCache is populated.
	raw, err := evalJS(`JSON.stringify({
        files: globalThis.prSplit._state.analysisCache.files,
        branch: globalThis.prSplit._state.analysisCache.currentBranch
    })`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Files  []string `json:"files"`
		Branch string   `json:"branch"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(result.Files))
	}
	if result.Branch != "feature-branch" {
		t.Errorf("branch = %q, want %q", result.Branch, "feature-branch")
	}
}

// TestChunk13_SetCommand verifies the set command updates runtime config.
func TestChunk13_SetCommand(t *testing.T) {
	evalJS := loadTUIEngine(t)

	_, err := evalJS(`
        globalThis._prints = [];
        var cmds = globalThis.prSplit._buildCommands({});
        cmds.set.handler(['base', 'develop']);
    `)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit.runtime.baseBranch`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "develop" {
		t.Errorf("baseBranch = %v, want 'develop'", raw)
	}
}

// TestChunk13_ReportCommandOutputsJSON verifies the report command prints
// valid JSON.
func TestChunk13_ReportCommandOutputsJSON(t *testing.T) {
	evalJS := loadTUIEngine(t)

	_, err := evalJS(`
        globalThis._prints = [];
        var cmds = globalThis.prSplit._buildCommands({});
        cmds.report.handler();
    `)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis._prints.join('\n')`)
	if err != nil {
		t.Fatal(err)
	}

	// The output should be valid JSON.
	var parsed map[string]any
	str, ok := raw.(string)
	if !ok || str == "" {
		t.Fatal("report output is empty")
	}
	if err := json.Unmarshal([]byte(str), &parsed); err != nil {
		t.Errorf("report output is not valid JSON: %v", err)
	}
}

// TestChunk13_CommandHandlersAllCallable verifies that every command handler
// can be called without crashing (guards should handle missing state).
func TestChunk13_CommandHandlersAllCallable(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Get command names.
	raw, err := evalJS(`JSON.stringify(Object.keys(globalThis.prSplit._buildCommands({})))`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := json.Unmarshal([]byte(raw.(string)), &names); err != nil {
		t.Fatal(err)
	}

	// Call each synchronous handler (skip async ones: fix, run, auto-split).
	asyncCmds := map[string]bool{"fix": true, "run": true, "auto-split": true}
	for _, name := range names {
		if asyncCmds[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			_, err := evalJS(`
                globalThis._prints = [];
                var cmds = globalThis.prSplit._buildCommands({});
                cmds['` + name + `'].handler([]);
            `)
			if err != nil {
				t.Errorf("handler %q threw: %v", name, err)
			}
		})
	}
}

// TestChunk13_OnEnterPrintsConfig verifies the mode onEnter callback prints
// configuration info.
func TestChunk13_OnEnterPrintsConfig(t *testing.T) {
	evalJS := loadTUIEngine(t)

	_, err := evalJS(`
        globalThis._prints = [];
        globalThis._registeredModes[0].onEnter();
    `)
	if err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis._prints)`)
	if err != nil {
		t.Fatal(err)
	}
	var prints []string
	if err := json.Unmarshal([]byte(raw.(string)), &prints); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(prints, "\n")
	if !strings.Contains(joined, "PR Split") {
		t.Errorf("onEnter output missing 'PR Split', got: %v", prints)
	}
	if !strings.Contains(joined, "help") {
		t.Errorf("onEnter output missing help hint, got: %v", prints)
	}
}

// TestChunk13_ParseIntNaN verifies that commands with index arguments
// (move, rename, merge, reorder) reject non-numeric input instead of
// silently bypassing bounds checks. parseInt("abc", 10) returns NaN,
// and NaN < 0 is false, so without an explicit isNaN guard the value
// would pass through and cause undefined behavior downstream.
func TestChunk13_ParseIntNaN(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Inject a minimal planCache with 2 splits so we get past the
	// "No plan" guard and actually reach the parseInt code path.
	_, err := evalJS(`
		globalThis.prSplit._state.planCache = {
			splits: [
				{ name: 'split/01-alpha', order: 0, files: ['a.go'], message: 'alpha' },
				{ name: 'split/02-beta',  order: 1, files: ['b.go'], message: 'beta'  }
			]
		};
	`)
	if err != nil {
		t.Fatalf("failed to set planCache: %v", err)
	}

	cases := []struct {
		cmd  string
		args string // JSON array
		want string // substring expected in output
	}{
		// move: fromIdx = parseInt("abc") → NaN
		{"move", `['a.go', 'abc', '2']`, "Invalid from-index"},
		// move: toIdx = parseInt("xyz") → NaN
		{"move", `['a.go', '1', 'xyz']`, "Invalid to-index"},
		// rename: idx = parseInt("abc") → NaN
		{"rename", `['abc', 'new-name']`, "Invalid index"},
		// merge: idxA = parseInt("abc") → NaN
		{"merge", `['abc', '2']`, "Invalid index A"},
		// merge: idxB = parseInt("abc") → NaN
		{"merge", `['1', 'abc']`, "Invalid index B"},
		// reorder: fromIdx = parseInt("abc") → NaN
		{"reorder", `['abc', '2']`, "Invalid index"},
		// reorder: toIdx = parseInt("abc") → NaN
		{"reorder", `['1', 'abc']`, "Invalid position"},
	}

	for _, tc := range cases {
		t.Run(tc.cmd+"_"+tc.want, func(t *testing.T) {
			script := `
				globalThis._prints = [];
				var cmds = globalThis.prSplit._buildCommands({});
				cmds['` + tc.cmd + `'].handler(` + tc.args + `);
				JSON.stringify(globalThis._prints);
			`
			raw, err := evalJS(script)
			if err != nil {
				t.Fatalf("handler threw: %v", err)
			}
			var prints []string
			if err := json.Unmarshal([]byte(raw.(string)), &prints); err != nil {
				t.Fatalf("failed to unmarshal prints: %v", err)
			}
			joined := strings.Join(prints, "\n")
			if !strings.Contains(joined, tc.want) {
				t.Errorf("expected output to contain %q, got: %s", tc.want, joined)
			}
		})
	}
}

// ---------------------------------------------------------------------------
//  WizardState — state machine tests (T15)
// ---------------------------------------------------------------------------

// TestChunk13_WizardState_InitialState verifies a WizardState starts in IDLE.
func TestChunk13_WizardState_InitialState(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		JSON.stringify({ current: ws.current, histLen: ws.history.length, terminal: ws.isTerminal() });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"IDLE","histLen":0,"terminal":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_HappyPath tests the full happy-path transition sequence.
func TestChunk13_WizardState_HappyPath(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('PLAN_GENERATION');
		ws.transition('PLAN_REVIEW');
		ws.transition('BRANCH_BUILDING');
		ws.transition('EQUIV_CHECK');
		ws.transition('FINALIZATION');
		ws.transition('DONE');
		JSON.stringify({ current: ws.current, histLen: ws.history.length, terminal: ws.isTerminal() });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"DONE","histLen":7,"terminal":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_InvalidTransition verifies that invalid transitions throw.
func TestChunk13_WizardState_InvalidTransition(t *testing.T) {
	evalJS := loadTUIEngine(t)

	cases := []struct {
		name, js string
	}{
		{"IDLE→DONE", `var ws = new globalThis.prSplit.WizardState(); ws.transition('DONE');`},
		{"IDLE→BRANCH_BUILDING", `var ws = new globalThis.prSplit.WizardState(); ws.transition('BRANCH_BUILDING');`},
		{"CONFIG→FINALIZATION", `var ws = new globalThis.prSplit.WizardState(); ws.transition('CONFIG'); ws.transition('FINALIZATION');`},
		{"DONE→CONFIG", `var ws = new globalThis.prSplit.WizardState(); ws.transition('CONFIG'); ws.transition('PLAN_GENERATION'); ws.transition('PLAN_REVIEW'); ws.transition('BRANCH_BUILDING'); ws.transition('EQUIV_CHECK'); ws.transition('FINALIZATION'); ws.transition('DONE'); ws.transition('CONFIG');`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evalJS(tc.js)
			if err == nil {
				t.Errorf("expected error for invalid transition %s", tc.name)
			}
		})
	}
}

// TestChunk13_WizardState_Cancel verifies cancel from various states.
func TestChunk13_WizardState_Cancel(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var results = [];

		// Cancel from CONFIG
		var ws1 = new globalThis.prSplit.WizardState();
		ws1.transition('CONFIG');
		ws1.cancel();
		results.push(ws1.current);

		// Cancel from BRANCH_BUILDING
		var ws2 = new globalThis.prSplit.WizardState();
		ws2.transition('CONFIG');
		ws2.transition('PLAN_GENERATION');
		ws2.transition('PLAN_REVIEW');
		ws2.transition('BRANCH_BUILDING');
		ws2.cancel();
		results.push(ws2.current);

		// Cancel on terminal state — no-op
		var ws3 = new globalThis.prSplit.WizardState();
		ws3.transition('CONFIG');
		ws3.cancel();
		ws3.cancel(); // Already CANCELLED, should stay
		results.push(ws3.current);

		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `["CANCELLED","CANCELLED","CANCELLED"]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_ForceCancel tests double-cancel escalation.
func TestChunk13_WizardState_ForceCancel(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('PLAN_GENERATION');
		ws.cancel();
		ws.forceCancel();
		var state = ws.current;
		// Force cancel from DONE — no-op
		ws.transition('DONE');
		ws.forceCancel();
		JSON.stringify({ afterForce: state, afterDone: ws.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"afterForce":"FORCE_CANCEL","afterDone":"DONE"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_Pause tests pausing from allowed states.
func TestChunk13_WizardState_Pause(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var results = [];

		// Pause from PLAN_GENERATION — allowed
		var ws1 = new globalThis.prSplit.WizardState();
		ws1.transition('CONFIG');
		ws1.transition('PLAN_GENERATION');
		ws1.pause();
		results.push(ws1.current);

		// Pause from BRANCH_BUILDING — allowed
		var ws2 = new globalThis.prSplit.WizardState();
		ws2.transition('CONFIG');
		ws2.transition('PLAN_GENERATION');
		ws2.transition('PLAN_REVIEW');
		ws2.transition('BRANCH_BUILDING');
		ws2.pause();
		results.push(ws2.current);

		// Pause from CONFIG — not allowed, no-op
		var ws3 = new globalThis.prSplit.WizardState();
		ws3.transition('CONFIG');
		ws3.pause();
		results.push(ws3.current);

		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `["PAUSED","PAUSED","CONFIG"]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_ErrorFromAnyActive tests error transition.
func TestChunk13_WizardState_ErrorFromAnyActive(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var results = [];

		// Error from CONFIG
		var ws1 = new globalThis.prSplit.WizardState();
		ws1.transition('CONFIG');
		ws1.error('bad config');
		results.push({ state: ws1.current, msg: ws1.data.error });

		// Error from terminal — no-op
		var ws2 = new globalThis.prSplit.WizardState();
		ws2.transition('CONFIG');
		ws2.error('first');
		ws2.error('second');
		results.push({ state: ws2.current, msg: ws2.data.error });

		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `[{"state":"ERROR","msg":"bad config"},{"state":"ERROR","msg":"first"}]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_DataMerge tests that transition merges data correctly.
func TestChunk13_WizardState_DataMerge(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG', { baseBranch: 'main', dryRun: false });
		ws.transition('PLAN_GENERATION', { analysisFiles: 42 });
		JSON.stringify({ baseBranch: ws.data.baseBranch, dryRun: ws.data.dryRun, files: ws.data.analysisFiles });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"baseBranch":"main","dryRun":false,"files":42}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_OnTransition tests the listener callback.
func TestChunk13_WizardState_OnTransition(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		var transitions = [];
		ws.onTransition(function(from, to) { transitions.push(from + '->' + to); });
		ws.transition('CONFIG');
		ws.transition('PLAN_GENERATION');
		ws.cancel();
		JSON.stringify(transitions);
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `["IDLE->CONFIG","CONFIG->PLAN_GENERATION","PLAN_GENERATION->CANCELLED"]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_History tests transition history tracking.
func TestChunk13_WizardState_History(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('PLAN_GENERATION');
		var h = ws.history.map(function(e) { return e.from + '->' + e.to; });
		JSON.stringify(h);
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `["IDLE->CONFIG","CONFIG->PLAN_GENERATION"]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_Reset tests that reset clears all state.
func TestChunk13_WizardState_Reset(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG', { x: 1 });
		ws.reset();
		JSON.stringify({ current: ws.current, dataKeys: Object.keys(ws.data).length, histLen: ws.history.length });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"IDLE","dataKeys":0,"histLen":0}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_Checkpoint tests saveCheckpoint.
func TestChunk13_WizardState_Checkpoint(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG', { plan: 'test' });
		ws.transition('PLAN_GENERATION');
		var cp = ws.saveCheckpoint();
		JSON.stringify({ state: cp.state, plan: cp.data.plan, hasAt: typeof cp.at === 'number' });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"state":"PLAN_GENERATION","plan":"test","hasAt":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_BaselineFailPath tests the baseline failure flow.
func TestChunk13_WizardState_BaselineFailPath(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('BASELINE_FAIL');
		ws.transition('PLAN_GENERATION'); // override
		JSON.stringify({ current: ws.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"PLAN_GENERATION"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_PlanEditorRoundtrip tests the PLAN_REVIEW ↔ PLAN_EDITOR cycle.
func TestChunk13_WizardState_PlanEditorRoundtrip(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('PLAN_GENERATION');
		ws.transition('PLAN_REVIEW');
		ws.transition('PLAN_EDITOR');
		ws.transition('PLAN_REVIEW');
		ws.transition('PLAN_EDITOR');
		ws.transition('PLAN_REVIEW');
		ws.transition('BRANCH_BUILDING');
		JSON.stringify({ current: ws.current, histLen: ws.history.length });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"BRANCH_BUILDING","histLen":8}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_ErrorResolutionReSplit tests the re-split path.
func TestChunk13_WizardState_ErrorResolutionReSplit(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('PLAN_GENERATION');
		ws.transition('PLAN_REVIEW');
		ws.transition('BRANCH_BUILDING');
		ws.transition('ERROR_RESOLUTION');
		ws.transition('PLAN_GENERATION'); // re-split
		ws.transition('PLAN_REVIEW');
		ws.transition('BRANCH_BUILDING');
		ws.transition('EQUIV_CHECK');
		ws.transition('FINALIZATION');
		ws.transition('DONE');
		JSON.stringify({ current: ws.current, histLen: ws.history.length });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"DONE","histLen":11}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_ResumeFromBranchBuilding tests CONFIG→BRANCH_BUILDING.
func TestChunk13_WizardState_ResumeFromBranchBuilding(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.transition('CONFIG');
		ws.transition('BRANCH_BUILDING'); // resume path
		JSON.stringify({ current: ws.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"BRANCH_BUILDING"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_ExportsAvailable tests that WizardState and constants
// are exported on prSplit for cross-chunk access.
func TestChunk13_WizardState_ExportsAvailable(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		JSON.stringify({
			hasWizardState: typeof globalThis.prSplit.WizardState === 'function',
			hasTransitions: typeof globalThis.prSplit.WIZARD_VALID_TRANSITIONS === 'object',
			hasTerminal: typeof globalThis.prSplit.WIZARD_TERMINAL_STATES === 'object',
			idleValid: !!globalThis.prSplit.WIZARD_VALID_TRANSITIONS['IDLE']['CONFIG'],
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasWizardState":true,"hasTransitions":true,"hasTerminal":true,"idleValid":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_WizardState_ListenerErrorSwallowed tests that a throwing listener
// doesn't break the state machine.
func TestChunk13_WizardState_ListenerErrorSwallowed(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var ws = new globalThis.prSplit.WizardState();
		ws.onTransition(function() { throw new Error('boom'); });
		ws.transition('CONFIG'); // should not throw
		JSON.stringify({ current: ws.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"CONFIG"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
//  T18: handleConfigState tests
// ---------------------------------------------------------------------------

// TestChunk13_HandleConfigState_MissingBaseBranch tests that CONFIG state
// produces an error when baseBranch is empty.
func TestChunk13_HandleConfigState_MissingBaseBranch(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		// Mock gitExec to return a valid feature branch.
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		// Mock verifySplit to pass baseline.
		prSplit.verifySplit = function() { return { passed: true, name: 'main', output: '' }; };

		// Clear baseBranch.
		prSplit.runtime.baseBranch = '';

		var result = prSplit._handleConfigState({});
		JSON.stringify({ hasError: !!result.error, errorContains: result.error ? result.error.indexOf('baseBranch') >= 0 : false });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"errorContains":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_OnBaseBranch tests that CONFIG state detects
// when already on the base branch.
func TestChunk13_HandleConfigState_OnBaseBranch(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		// Mock gitExec: current branch IS the base branch.
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'main\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() { return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';

		var result = prSplit._handleConfigState({});
		JSON.stringify({ hasError: !!result.error, mentionsBase: result.error ? result.error.indexOf('base branch') >= 0 : false });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"mentionsBase":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_GitFails tests CONFIG when git rev-parse fails.
func TestChunk13_HandleConfigState_GitFails(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 128, stdout: '', stderr: 'not a git repo' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'main';

		var result = prSplit._handleConfigState({});
		JSON.stringify({ hasError: !!result.error, mentionsBranch: result.error ? result.error.indexOf('current branch') >= 0 : false });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"mentionsBranch":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_BaselinePass tests the happy path: valid config
// and passing baseline verification → PLAN_GENERATION.
func TestChunk13_HandleConfigState_BaselinePass(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			if (args[0] === 'checkout') return { code: 0, stdout: '', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function(branch, opts) {
			return { passed: true, name: branch, output: 'ok' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			error: result.error,
			baselineFailed: !!result.baselineFailed,
			resume: !!result.resume
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"baselineFailed":false,"resume":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_BaselineFail tests that failing baseline
// verification returns baselineFailed=true.
func TestChunk13_HandleConfigState_BaselineFail(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			if (args[0] === 'checkout') return { code: 0, stdout: '', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function(branch, opts) {
			return { passed: false, name: branch, error: 'make test: exit 2', output: 'FAIL' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			baselineFailed: !!result.baselineFailed,
			hasBaselineError: !!result.baselineError
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"baselineFailed":true,"hasBaselineError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_ResumeWithCheckpoint tests that --resume with
// a valid checkpoint returns resume=true.
func TestChunk13_HandleConfigState_ResumeWithCheckpoint(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.loadPlan = function() {
			return { plan: { splits: [{ name: 'split/01', files: ['a.go'] }] } };
		};
		prSplit.runtime.baseBranch = 'main';

		var result = prSplit._handleConfigState({ resume: true });
		JSON.stringify({ resume: !!result.resume, hasCheckpoint: !!result.checkpoint });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"resume":true,"hasCheckpoint":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_ResumeNoCheckpoint tests that --resume without
// a valid checkpoint falls through to normal config flow.
func TestChunk13_HandleConfigState_ResumeNoCheckpoint(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			if (args[0] === 'checkout') return { code: 0, stdout: '', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.loadPlan = function() { return { error: 'no checkpoint' }; };
		prSplit.verifySplit = function() { return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({ resume: true });
		JSON.stringify({
			error: result.error,
			resume: !!result.resume,
			baselineFailed: !!result.baselineFailed
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"resume":false,"baselineFailed":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_SkipsBaselineForTrue tests that baseline
// verification is skipped when verifyCommand is 'true'.
func TestChunk13_HandleConfigState_SkipsBaselineForTrue(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var verifyCalled = false;
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() { verifyCalled = true; return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'true';

		var result = prSplit._handleConfigState({});
		JSON.stringify({ error: result.error, verifyCalled: verifyCalled });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"verifyCalled":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handleBaselineFailState tests (T17)
// ---------------------------------------------------------------------------

// TestChunk13_HandleBaselineFailState_Override tests that choosing 'override'
// transitions from BASELINE_FAIL to PLAN_GENERATION.
func TestChunk13_HandleBaselineFailState_Override(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('BASELINE_FAIL', { error: 'tests fail', output: 'FAIL main_test.go' });

		var result = prSplit._handleBaselineFailState(wizard, 'override');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"override","state":"PLAN_GENERATION","current":"PLAN_GENERATION"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBaselineFailState_Abort tests that choosing 'abort'
// transitions from BASELINE_FAIL to CANCELLED.
func TestChunk13_HandleBaselineFailState_Abort(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('BASELINE_FAIL', { error: 'tests fail', output: 'FAIL' });

		var result = prSplit._handleBaselineFailState(wizard, 'abort');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"abort","state":"CANCELLED","current":"CANCELLED"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBaselineFailState_DefaultAbort tests that no choice
// (undefined or empty) defaults to abort.
func TestChunk13_HandleBaselineFailState_DefaultAbort(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('BASELINE_FAIL', { error: 'tests fail', output: '' });

		var result = prSplit._handleBaselineFailState(wizard);
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"abort","state":"CANCELLED","current":"CANCELLED"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBaselineFailState_WrongState tests that calling
// handleBaselineFailState when wizard is NOT in BASELINE_FAIL returns error.
func TestChunk13_HandleBaselineFailState_WrongState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');

		var result = prSplit._handleBaselineFailState(wizard, 'override');
		JSON.stringify({ hasError: !!result.error, errorContains: result.error.indexOf('PLAN_GENERATION') >= 0 });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"errorContains":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBaselineFailState_OverridePreservesData tests that
// wizard.data is preserved through override transition.
func TestChunk13_HandleBaselineFailState_OverridePreservesData(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('BASELINE_FAIL', { error: 'make test fails', output: 'exit 1' });

		prSplit._handleBaselineFailState(wizard, 'override');
		JSON.stringify({
			current: wizard.current,
			errorData: wizard.data.error,
			outputData: wizard.data.output
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"current":"PLAN_GENERATION","errorData":"make test fails","outputData":"exit 1"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handlePlanReviewState tests (T19/T21)
// ---------------------------------------------------------------------------

// TestChunk13_HandlePlanReviewState_Approve tests approve → BRANCH_BUILDING.
func TestChunk13_HandlePlanReviewState_Approve(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		var result = prSplit._handlePlanReviewState(wizard, 'approve');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"approve","state":"BRANCH_BUILDING","current":"BRANCH_BUILDING"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanReviewState_Edit tests edit → PLAN_EDITOR.
func TestChunk13_HandlePlanReviewState_Edit(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		var result = prSplit._handlePlanReviewState(wizard, 'edit');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"edit","state":"PLAN_EDITOR","current":"PLAN_EDITOR"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanReviewState_Regenerate tests regenerate → PLAN_GENERATION with feedback.
func TestChunk13_HandlePlanReviewState_Regenerate(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		var result = prSplit._handlePlanReviewState(wizard, 'regenerate', { feedback: 'too many splits' });
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			feedback: wizard.data.feedback
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"regenerate","state":"PLAN_GENERATION","current":"PLAN_GENERATION","feedback":"too many splits"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanReviewState_Cancel tests cancel → CANCELLED.
func TestChunk13_HandlePlanReviewState_Cancel(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		var result = prSplit._handlePlanReviewState(wizard, 'cancel');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"cancel","state":"CANCELLED","current":"CANCELLED"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanReviewState_DefaultCancel tests that no choice defaults to cancel.
func TestChunk13_HandlePlanReviewState_DefaultCancel(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		var result = prSplit._handlePlanReviewState(wizard);
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"cancel","state":"CANCELLED","current":"CANCELLED"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanReviewState_WrongState tests calling from wrong state.
func TestChunk13_HandlePlanReviewState_WrongState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');

		var result = prSplit._handlePlanReviewState(wizard, 'approve');
		JSON.stringify({ hasError: !!result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handlePlanEditorState tests (T20/T21)
// ---------------------------------------------------------------------------

// TestChunk13_HandlePlanEditorState_Done tests done → PLAN_REVIEW.
func TestChunk13_HandlePlanEditorState_Done(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('PLAN_EDITOR');

		var result = prSplit._handlePlanEditorState(wizard, 'done');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"done","state":"PLAN_REVIEW","current":"PLAN_REVIEW"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanEditorState_DoneWithPlan tests done with valid plan → PLAN_REVIEW.
func TestChunk13_HandlePlanEditorState_DoneWithPlan(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('PLAN_EDITOR');

		var plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			verifyCommand: 'true',
			splits: [
				{ name: 'split/01-a', files: ['a.go'], message: 'add a', order: 0 }
			]
		};

		var result = prSplit._handlePlanEditorState(wizard, 'done', plan);
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			hasPlan: !!wizard.data.plan,
			planSplitCount: wizard.data.plan ? wizard.data.plan.splits.length : 0
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"done","state":"PLAN_REVIEW","current":"PLAN_REVIEW","hasPlan":true,"planSplitCount":1}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanEditorState_ValidationFailure tests done with invalid plan.
func TestChunk13_HandlePlanEditorState_ValidationFailure(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('PLAN_EDITOR');

		// Plan with no splits — should fail validation.
		var plan = {
			baseBranch: 'main',
			sourceBranch: 'feature',
			verifyCommand: 'true',
			splits: []
		};

		var result = prSplit._handlePlanEditorState(wizard, 'done', plan);
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			hasValidationErrors: !!(result.validationErrors && result.validationErrors.length > 0)
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"validation_failed","state":"PLAN_EDITOR","current":"PLAN_EDITOR","hasValidationErrors":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanEditorState_WrongState tests calling from wrong state.
func TestChunk13_HandlePlanEditorState_WrongState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		var result = prSplit._handlePlanEditorState(wizard, 'done');
		JSON.stringify({ hasError: !!result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandlePlanEditorState_EditReviewRoundTrip tests edit→done→review cycle.
func TestChunk13_HandlePlanEditorState_EditReviewRoundTrip(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		// Enter editor.
		var r1 = prSplit._handlePlanReviewState(wizard, 'edit');
		// Finish editing.
		var r2 = prSplit._handlePlanEditorState(wizard, 'done');
		// Approve from review.
		var r3 = prSplit._handlePlanReviewState(wizard, 'approve');

		JSON.stringify({
			editState: r1.state,
			doneState: r2.state,
			approveState: r3.state,
			final: wizard.current
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"editState":"PLAN_EDITOR","doneState":"PLAN_REVIEW","approveState":"BRANCH_BUILDING","final":"BRANCH_BUILDING"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handleBranchBuildingState tests (T22/T23)
// ---------------------------------------------------------------------------

// TestChunk13_HandleBranchBuildingState_AllPass tests all branches pass → EQUIV_CHECK.
func TestChunk13_HandleBranchBuildingState_AllPass(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');

		// Mock executeSplit to succeed.
		prSplit.executeSplit = function(plan) {
			return {
				error: null,
				results: [
					{ name: 'split/01-a', files: ['a.go'], sha: 'abc1234', error: null },
					{ name: 'split/02-b', files: ['b.go'], sha: 'def5678', error: null }
				]
			};
		};
		// Mock verifySplit to pass.
		prSplit.verifySplit = function(branch, config) {
			return { name: branch, passed: true, output: 'ok', error: null };
		};

		var plan = {
			baseBranch: 'main', sourceBranch: 'feature', verifyCommand: 'make test',
			splits: [
				{ name: 'split/01-a', files: ['a.go'], message: 'add a', order: 0 },
				{ name: 'split/02-b', files: ['b.go'], message: 'add b', order: 1 }
			]
		};

		var result = prSplit._handleBranchBuildingState(wizard, plan);
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			resultCount: result.results.length,
			failedCount: result.failedBranches.length
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"success","state":"EQUIV_CHECK","current":"EQUIV_CHECK","resultCount":2,"failedCount":0}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBranchBuildingState_OneFail tests one branch fails → ERROR_RESOLUTION.
func TestChunk13_HandleBranchBuildingState_OneFail(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');

		prSplit.executeSplit = function(plan) {
			return {
				error: null,
				results: [
					{ name: 'split/01-a', files: ['a.go'], sha: 'abc', error: null },
					{ name: 'split/02-b', files: ['b.go'], sha: 'def', error: null }
				]
			};
		};
		prSplit.verifySplit = function(branch) {
			if (branch === 'split/02-b') return { name: branch, passed: false, output: 'FAIL', error: 'test failed' };
			return { name: branch, passed: true, output: 'ok', error: null };
		};

		var plan = {
			baseBranch: 'main', sourceBranch: 'feature', verifyCommand: 'make test',
			splits: [
				{ name: 'split/01-a', files: ['a.go'], message: 'add a', order: 0 },
				{ name: 'split/02-b', files: ['b.go'], message: 'add b', order: 1 }
			]
		};

		var result = prSplit._handleBranchBuildingState(wizard, plan);
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			failedCount: result.failedBranches.length,
			failedName: result.failedBranches[0].name
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"failed","state":"ERROR_RESOLUTION","current":"ERROR_RESOLUTION","failedCount":1,"failedName":"split/02-b"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBranchBuildingState_EmptyPlan tests empty plan → ERROR.
func TestChunk13_HandleBranchBuildingState_EmptyPlan(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');

		var result = prSplit._handleBranchBuildingState(wizard, { splits: [] });
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current, hasError: !!result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"error","state":"ERROR","current":"ERROR","hasError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBranchBuildingState_Cancel tests cancellation mid-build.
func TestChunk13_HandleBranchBuildingState_Cancel(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');

		prSplit.executeSplit = function(plan) {
			return {
				error: null,
				results: [
					{ name: 'split/01-a', files: ['a.go'], sha: 'abc', error: null }
				]
			};
		};

		// Cancel immediately after execution.
		var cancelCount = 0;
		var result = prSplit._handleBranchBuildingState(wizard, {
			baseBranch: 'main', sourceBranch: 'feature', verifyCommand: 'make test',
			splits: [{ name: 'split/01-a', files: ['a.go'], message: 'add a', order: 0 }]
		}, { isCancelled: function() { cancelCount++; return cancelCount > 1; } });

		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"cancelled","state":"CANCELLED","current":"CANCELLED"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleBranchBuildingState_WrongState tests wrong state guard.
func TestChunk13_HandleBranchBuildingState_WrongState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');

		var result = prSplit._handleBranchBuildingState(wizard, {});
		JSON.stringify({ hasError: !!result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handleErrorResolutionState tests (T24/T25)
// ---------------------------------------------------------------------------

// TestChunk13_HandleErrorResolutionState_AutoResolve tests auto-resolve → BRANCH_BUILDING.
func TestChunk13_HandleErrorResolutionState_AutoResolve(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('ERROR_RESOLUTION');

		var result = prSplit._handleErrorResolutionState(wizard, 'auto-resolve');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"auto-resolve","state":"BRANCH_BUILDING","current":"BRANCH_BUILDING"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleErrorResolutionState_Skip tests skip → EQUIV_CHECK.
func TestChunk13_HandleErrorResolutionState_Skip(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('ERROR_RESOLUTION');

		var result = prSplit._handleErrorResolutionState(wizard, 'skip');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"skip","state":"EQUIV_CHECK","current":"EQUIV_CHECK"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleErrorResolutionState_Retry tests retry → PLAN_GENERATION.
func TestChunk13_HandleErrorResolutionState_Retry(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('ERROR_RESOLUTION');

		var result = prSplit._handleErrorResolutionState(wizard, 'retry');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"retry","state":"PLAN_GENERATION","current":"PLAN_GENERATION"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleErrorResolutionState_Abort tests abort → CANCELLED.
func TestChunk13_HandleErrorResolutionState_Abort(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('ERROR_RESOLUTION');

		var result = prSplit._handleErrorResolutionState(wizard, 'abort');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"abort","state":"CANCELLED","current":"CANCELLED"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleErrorResolutionState_WrongState tests wrong state guard.
func TestChunk13_HandleErrorResolutionState_WrongState(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');

		var result = prSplit._handleErrorResolutionState(wizard, 'skip');
		JSON.stringify({ hasError: !!result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handleEquivCheckState tests (T26)
// ---------------------------------------------------------------------------

// TestChunk13_HandleEquivCheckState_Pass tests equivalence pass → FINALIZATION.
func TestChunk13_HandleEquivCheckState_Pass(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');

		prSplit.verifyEquivalence = function(plan) {
			return { equivalent: true, splitTree: 'abc123', sourceTree: 'abc123', error: null };
		};

		var result = prSplit._handleEquivCheckState(wizard, { baseBranch: 'main', splits: [] });
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			equivalent: result.equivalence.equivalent
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"checked","state":"FINALIZATION","current":"FINALIZATION","equivalent":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleEquivCheckState_Mismatch tests tree mismatch still → FINALIZATION (warning).
func TestChunk13_HandleEquivCheckState_Mismatch(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');

		prSplit.verifyEquivalence = function(plan) {
			return { equivalent: false, splitTree: 'abc', sourceTree: 'def', error: null };
		};

		var result = prSplit._handleEquivCheckState(wizard, { baseBranch: 'main', splits: [] });
		JSON.stringify({
			action: result.action,
			state: result.state,
			equivalent: result.equivalence.equivalent,
			storedInData: !!wizard.data.equivalence
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"checked","state":"FINALIZATION","equivalent":false,"storedInData":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleEquivCheckState_NoPlan tests no plan → ERROR.
func TestChunk13_HandleEquivCheckState_NoPlan(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');

		var result = prSplit._handleEquivCheckState(wizard, null);
		JSON.stringify({ action: result.action, state: result.state, hasError: !!result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"error","state":"ERROR","hasError":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// handleFinalizationState tests (T26)
// ---------------------------------------------------------------------------

// TestChunk13_HandleFinalizationState_Done tests done → DONE.
func TestChunk13_HandleFinalizationState_Done(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');
		wizard.transition('FINALIZATION');

		var result = prSplit._handleFinalizationState(wizard, 'done');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"done","state":"DONE","current":"DONE"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleFinalizationState_CreatePRs tests create-prs → FINALIZATION (self).
func TestChunk13_HandleFinalizationState_CreatePRs(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');
		wizard.transition('FINALIZATION');

		var result = prSplit._handleFinalizationState(wizard, 'create-prs');
		JSON.stringify({
			action: result.action,
			state: result.state,
			current: wizard.current,
			prsRequested: !!wizard.data.prsRequested
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"create-prs","state":"FINALIZATION","current":"FINALIZATION","prsRequested":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleFinalizationState_Report tests report stays in FINALIZATION.
func TestChunk13_HandleFinalizationState_Report(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');
		wizard.transition('FINALIZATION');

		var result = prSplit._handleFinalizationState(wizard, 'report');
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"report","state":"FINALIZATION","current":"FINALIZATION"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleFinalizationState_DefaultDone tests default is done.
func TestChunk13_HandleFinalizationState_DefaultDone(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');
		wizard.transition('EQUIV_CHECK');
		wizard.transition('FINALIZATION');

		var result = prSplit._handleFinalizationState(wizard);
		JSON.stringify({ action: result.action, state: result.state, current: wizard.current });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"action":"done","state":"DONE","current":"DONE"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// E2E wizard flow tests (T28)
// ---------------------------------------------------------------------------

// TestChunk13_Wizard_HappyPath_E2E tests full CONFIG→...→DONE flow.
func TestChunk13_Wizard_HappyPath_E2E(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		// --- Setup mocks ---
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function(branch, config) {
			return { name: branch, passed: true, output: 'ok', error: null };
		};
		prSplit.executeSplit = function(plan) {
			return {
				error: null,
				results: [
					{ name: 'split/01-a', files: ['a.go'], sha: 'abc1234', error: null },
					{ name: 'split/02-b', files: ['b.go'], sha: 'def5678', error: null }
				]
			};
		};
		prSplit.verifyEquivalence = function(plan) {
			return { equivalent: true, splitTree: 'abc123', sourceTree: 'abc123', error: null };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		// --- Execute wizard ---
		var wizard = new prSplit.WizardState();
		var history = [];
		wizard.onTransition(function(from, to) { history.push(from + '→' + to); });

		// CONFIG
		wizard.transition('CONFIG');
		var configResult = prSplit._handleConfigState({});
		if (configResult.error) throw 'config error: ' + configResult.error;
		wizard.transition('PLAN_GENERATION');

		// PLAN_REVIEW (skip PLAN_GENERATION — that's the pipeline's job)
		wizard.transition('PLAN_REVIEW');
		var reviewResult = prSplit._handlePlanReviewState(wizard, 'approve');

		// BRANCH_BUILDING
		var plan = {
			baseBranch: 'main', sourceBranch: 'feature', verifyCommand: 'make test',
			splits: [
				{ name: 'split/01-a', files: ['a.go'], message: 'add a', order: 0 },
				{ name: 'split/02-b', files: ['b.go'], message: 'add b', order: 1 }
			]
		};
		var buildResult = prSplit._handleBranchBuildingState(wizard, plan);

		// EQUIV_CHECK
		var equivResult = prSplit._handleEquivCheckState(wizard, plan);

		// FINALIZATION → DONE
		var finalResult = prSplit._handleFinalizationState(wizard, 'done');

		JSON.stringify({
			final: wizard.current,
			isTerminal: wizard.isTerminal(),
			historyLength: wizard.history.length,
			transitions: history.join(', '),
			buildSuccess: buildResult.action === 'success',
			equivPass: equivResult.equivalence.equivalent
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"final":"DONE","isTerminal":true,"historyLength":7,"transitions":"IDLE→CONFIG, CONFIG→PLAN_GENERATION, PLAN_GENERATION→PLAN_REVIEW, PLAN_REVIEW→BRANCH_BUILDING, BRANCH_BUILDING→EQUIV_CHECK, EQUIV_CHECK→FINALIZATION, FINALIZATION→DONE","buildSuccess":true,"equivPass":true}`
	if got != want {
		t.Errorf("got:\n  %s\nwant:\n  %s", got, want)
	}
}

// TestChunk13_Wizard_PlanRejection_E2E tests reject plan → regenerate → approve flow.
func TestChunk13_Wizard_PlanRejection_E2E(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');

		// Reject: regenerate with feedback.
		var r1 = prSplit._handlePlanReviewState(wizard, 'regenerate', { feedback: 'fewer splits' });

		// Back to PLAN_REVIEW after regeneration.
		wizard.transition('PLAN_REVIEW');

		// Approve this time.
		var r2 = prSplit._handlePlanReviewState(wizard, 'approve');

		JSON.stringify({
			afterReject: r1.state,
			feedbackStored: wizard.data.feedback,
			afterApprove: r2.state,
			final: wizard.current
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"afterReject":"PLAN_GENERATION","feedbackStored":"fewer splits","afterApprove":"BRANCH_BUILDING","final":"BRANCH_BUILDING"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_Wizard_BaselineFailRecovery_E2E tests baseline fail → override → complete.
func TestChunk13_Wizard_BaselineFailRecovery_E2E(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function(branch, config) {
			return { name: branch, passed: false, output: 'FAIL', error: 'test failed' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var wizard = new prSplit.WizardState();

		// CONFIG
		wizard.transition('CONFIG');
		var configResult = prSplit._handleConfigState({});

		// Baseline fails.
		wizard.transition('BASELINE_FAIL', {
			error: configResult.baselineError,
			output: configResult.baselineOutput
		});

		// Override.
		var overrideResult = prSplit._handleBaselineFailState(wizard, 'override');

		// Now in PLAN_GENERATION.
		JSON.stringify({
			baselineFailed: !!configResult.baselineFailed,
			overrideAction: overrideResult.action,
			final: wizard.current,
			historyLength: wizard.history.length
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"baselineFailed":true,"overrideAction":"override","final":"PLAN_GENERATION","historyLength":3}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_Wizard_BranchFailRecovery_E2E tests branch fail → auto-resolve → complete.
func TestChunk13_Wizard_BranchFailRecovery_E2E(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var callCount = 0;
		prSplit.executeSplit = function(plan) {
			callCount++;
			return {
				error: null,
				results: [
					{ name: 'split/01-a', files: ['a.go'], sha: 'abc', error: null }
				]
			};
		};
		prSplit.verifySplit = function(branch, config) {
			if (callCount <= 1) return { name: branch, passed: false, output: 'FAIL', error: 'test error' };
			return { name: branch, passed: true, output: 'ok', error: null };
		};
		prSplit.verifyEquivalence = function() {
			return { equivalent: true, splitTree: 'aaa', sourceTree: 'aaa', error: null };
		};

		var wizard = new prSplit.WizardState();
		wizard.transition('CONFIG');
		wizard.transition('PLAN_GENERATION');
		wizard.transition('PLAN_REVIEW');
		wizard.transition('BRANCH_BUILDING');

		var plan = {
			baseBranch: 'main', sourceBranch: 'feature', verifyCommand: 'make test',
			splits: [{ name: 'split/01-a', files: ['a.go'], message: 'add a', order: 0 }]
		};

		// First build — fails.
		var r1 = prSplit._handleBranchBuildingState(wizard, plan);

		// Auto-resolve → re-enter BRANCH_BUILDING.
		var r2 = prSplit._handleErrorResolutionState(wizard, 'auto-resolve');

		// Second build — succeeds.
		var r3 = prSplit._handleBranchBuildingState(wizard, plan);

		// Equiv check.
		var r4 = prSplit._handleEquivCheckState(wizard, plan);

		// Done.
		var r5 = prSplit._handleFinalizationState(wizard, 'done');

		JSON.stringify({
			firstBuild: r1.action,
			resolution: r2.action,
			secondBuild: r3.action,
			equiv: r4.action,
			final: r5.action,
			current: wizard.current,
			calls: callCount
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"firstBuild":"failed","resolution":"auto-resolve","secondBuild":"success","equiv":"checked","final":"done","current":"DONE","calls":2}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// ═══════════════════════════════════════════════════════════════════════
//  HUD Overlay Tests (T32)
// ═══════════════════════════════════════════════════════════════════════

// TestChunk13_HUD_ExportedFunctions verifies that HUD functions are
// exported on prSplit after chunk 13 loads.
func TestChunk13_HUD_ExportedFunctions(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify({
		hudEnabled: typeof globalThis.prSplit._hudEnabled,
		renderHudPanel: typeof globalThis.prSplit._renderHudPanel,
		renderHudStatusLine: typeof globalThis.prSplit._renderHudStatusLine,
		getActivityInfo: typeof globalThis.prSplit._getActivityInfo,
		getLastOutputLines: typeof globalThis.prSplit._getLastOutputLines
	})`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	wantExports := `{"hudEnabled":"function","renderHudPanel":"function","renderHudStatusLine":"function","getActivityInfo":"function","getLastOutputLines":"function"}`
	if got != wantExports {
		t.Errorf("HUD exports:\n  got:  %s\n  want: %s", got, wantExports)
	}
}

// TestChunk13_HUD_ActivityInfoWithoutMux verifies that _getActivityInfo
// returns 'unknown' when tuiMux is not available.
func TestChunk13_HUD_ActivityInfoWithoutMux(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._getActivityInfo())`)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		Label string `json:"label"`
		Ms    int    `json:"ms"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &info); err != nil {
		t.Fatal(err)
	}
	if info.Label != "unknown" {
		t.Errorf("activity label = %q, want 'unknown'", info.Label)
	}
	if info.Ms != -1 {
		t.Errorf("activity ms = %d, want -1", info.Ms)
	}
}

// TestChunk13_HUD_LastOutputLinesWithoutMux verifies that _getLastOutputLines
// returns an empty array when tuiMux is not available.
func TestChunk13_HUD_LastOutputLinesWithoutMux(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._getLastOutputLines(5))`)
	if err != nil {
		t.Fatal(err)
	}
	if raw.(string) != "[]" {
		t.Errorf("got %s, want []", raw.(string))
	}
}

// TestChunk13_HUD_ToggleState verifies that _hudEnabled toggles correctly.
func TestChunk13_HUD_ToggleState(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Initially disabled.
	raw, err := evalJS(`globalThis.prSplit._hudEnabled()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != false {
		t.Errorf("initial _hudEnabled() = %v, want false", raw)
	}
}

// TestChunk13_HUD_RenderPanel verifies that _renderHudPanel returns a
// non-empty string containing expected markers.
func TestChunk13_HUD_RenderPanel(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderHudPanel()`)
	if err != nil {
		t.Fatal(err)
	}
	panel := raw.(string)
	if len(panel) < 20 {
		t.Errorf("panel too short: %q", panel)
	}
	// Should contain key elements.
	for _, needle := range []string{"Claude Process HUD", "Status:", "Wizard:"} {
		if !strings.Contains(panel, needle) {
			t.Errorf("panel missing %q:\n%s", needle, panel)
		}
	}
}

// TestChunk13_HUD_StatusLineFormat verifies the compact status line format.
func TestChunk13_HUD_StatusLineFormat(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderHudStatusLine()`)
	if err != nil {
		t.Fatal(err)
	}
	line := raw.(string)
	// Should contain activity icon in brackets and wizard state.
	if !strings.Contains(line, "[") || !strings.Contains(line, "]") {
		t.Errorf("status line missing brackets: %q", line)
	}
}

// TestChunk13_HUD_CommandRegistered verifies that 'hud' command exists
// in buildCommands output.
func TestChunk13_HUD_CommandRegistered(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var cmds = globalThis.prSplit._buildCommands({});
		JSON.stringify({
			hasHud: typeof cmds.hud === 'object',
			description: cmds.hud && cmds.hud.description || '',
			usage: cmds.hud && cmds.hud.usage || '',
			hasHandler: typeof (cmds.hud && cmds.hud.handler) === 'function'
		})
	`)
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		HasHud      bool   `json:"hasHud"`
		Description string `json:"description"`
		Usage       string `json:"usage"`
		HasHandler  bool   `json:"hasHandler"`
	}
	if err := json.Unmarshal([]byte(raw.(string)), &info); err != nil {
		t.Fatal(err)
	}
	if !info.HasHud {
		t.Error("hud command not found in buildCommands")
	}
	if !info.HasHandler {
		t.Error("hud command has no handler")
	}
	if info.Description == "" {
		t.Error("hud command has empty description")
	}
}
