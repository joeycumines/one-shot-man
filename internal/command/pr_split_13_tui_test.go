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
func loadTUIEngine(t testing.TB) func(string) (interface{}, error) {
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
// with all 25 expected command names.
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
		"analyze", "auto-split", "cleanup", "conversation", "copy",
		"create-prs", "diff", "edit-plan", "equivalence", "execute",
		"fix", "graph", "group", "help", "load-plan", "merge", "move",
		"plan", "preview", "rename", "reorder", "report", "retro",
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

	var report map[string]interface{}
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
	var parsed map[string]interface{}
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
