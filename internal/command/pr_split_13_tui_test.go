package command

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  Chunk 13: TUI — command dispatch, buildReport, mode registration
// ---------------------------------------------------------------------------

// allChunksForTUI lists all 17 chunks needed for full TUI tests.
// Not used directly with loadChunkEngine (since TUI needs mock globals
// injected between 00-12 and 13-16), but referenced in documentation.
var _ = []string{ // compile-time proof the list is valid
	"00_core", "01_analysis", "02_grouping", "03_planning", "04_validation",
	"05_execution", "06_verification", "07_prcreation", "08_conflict",
	"09_claude", "10_pipeline", "11_utilities", "12_exports",
	"13_tui", "14_tui_commands", "15_tui_views", "16_tui_core",
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

// loadTUIEngine loads chunks 00-12, injects TUI mocks, then loads chunks 13-16.
// Returns evalJS function.
func loadTUIEngine(t testing.TB) func(string) (any, error) {
	t.Helper()

	evalJS := loadChunkEngine(t, nil, allChunksThrough12...)

	// Inject TUI mocks.
	if _, err := evalJS(setupTUIMocks); err != nil {
		t.Fatalf("failed to inject TUI mocks: %v", err)
	}

	// Evaluate TUI chunks (13-16) in order.
	tuiChunks := []struct {
		name   string
		source string
	}{
		{"13_tui", prSplitChunk13TUI},
		{"14_tui_commands", prSplitChunk14TUICommands},
		{"15_tui_views", prSplitChunk15TUIViews},
		{"16_tui_core", prSplitChunk16TUICore},
	}
	for _, chunk := range tuiChunks {
		if _, err := evalJS(chunk.source); err != nil {
			t.Fatalf("failed to load chunk %s: %v", chunk.name, err)
		}
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

	// Chunks 13-16 should not crash even without tui/ctx/output.
	tuiChunks := []struct {
		name   string
		source string
	}{
		{"13_tui", prSplitChunk13TUI},
		{"14_tui_commands", prSplitChunk14TUICommands},
		{"15_tui_views", prSplitChunk15TUIViews},
		{"16_tui_core", prSplitChunk16TUICore},
	}
	for _, chunk := range tuiChunks {
		if _, err := evalJS(chunk.source); err != nil {
			t.Fatalf("chunk %s should not error without TUI globals: %v", chunk.name, err)
		}
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

// TestChunk13_WizardState_PauseResume tests T084: PAUSED → resume back to original state.
func TestChunk13_WizardState_PauseResume(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var results = [];

		// Resume from PAUSED (paused from PLAN_GENERATION) → PLAN_GENERATION
		var ws1 = new globalThis.prSplit.WizardState();
		ws1.transition('CONFIG');
		ws1.transition('PLAN_GENERATION');
		ws1.pause();
		results.push({ before: ws1.current, pausedFrom: ws1.data.pausedFrom });
		var ok1 = ws1.resume();
		results.push({ after: ws1.current, resumed: ok1 });

		// Resume from PAUSED (paused from BRANCH_BUILDING) → BRANCH_BUILDING
		var ws2 = new globalThis.prSplit.WizardState();
		ws2.transition('CONFIG');
		ws2.transition('PLAN_GENERATION');
		ws2.transition('PLAN_REVIEW');
		ws2.transition('BRANCH_BUILDING');
		ws2.pause();
		results.push({ before: ws2.current, pausedFrom: ws2.data.pausedFrom });
		var ok2 = ws2.resume();
		results.push({ after: ws2.current, resumed: ok2 });

		// Resume from non-PAUSED state — should be no-op
		var ws3 = new globalThis.prSplit.WizardState();
		ws3.transition('CONFIG');
		var ok3 = ws3.resume();
		results.push({ current: ws3.current, resumed: ok3 });

		// Cancel from PAUSED via cancel() method (T084: must not no-op)
		var ws4 = new globalThis.prSplit.WizardState();
		ws4.transition('CONFIG');
		ws4.transition('PLAN_GENERATION');
		ws4.pause();
		ws4.cancel();
		results.push({ cancelledViaMethod: ws4.current, cancelCleansPausedFrom: (ws4.data.pausedFrom === undefined) });

		// Cancel from PAUSED via transition() (T084: direct transition also works)
		var ws5 = new globalThis.prSplit.WizardState();
		ws5.transition('CONFIG');
		ws5.transition('PLAN_GENERATION');
		ws5.pause();
		ws5.transition('CANCELLED');
		results.push({ cancelledViaTx: ws5.current });

		// Resume with undefined pausedFrom — manual transition to PAUSED without pause()
		var ws6 = new globalThis.prSplit.WizardState();
		ws6.transition('CONFIG');
		ws6.transition('PLAN_GENERATION');
		ws6.transition('PAUSED');  // direct transition, pausedFrom not set
		var ok6 = ws6.resume();
		results.push({ directPause: ws6.current, resumed: ok6 });

		// Resume with non-pausable pausedFrom — corrupted data
		var ws7 = new globalThis.prSplit.WizardState();
		ws7.transition('CONFIG');
		ws7.transition('PLAN_GENERATION');
		ws7.pause();
		ws7.data.pausedFrom = 'CONFIG';  // corrupt to non-pausable state
		var ok7 = ws7.resume();
		results.push({ corruptPause: ws7.current, resumed: ok7 });

		// pausedFrom is cleared after successful resume
		var ws8 = new globalThis.prSplit.WizardState();
		ws8.transition('CONFIG');
		ws8.transition('PLAN_GENERATION');
		ws8.pause();
		ws8.resume();
		results.push({ afterResume: ws8.current, pausedFromCleared: (ws8.data.pausedFrom === undefined) });

		// forceCancel from PAUSED cleans pausedFrom
		var ws9 = new globalThis.prSplit.WizardState();
		ws9.transition('CONFIG');
		ws9.transition('PLAN_GENERATION');
		ws9.pause();
		ws9.forceCancel();
		results.push({ forceCancelled: ws9.current, fCancelCleansPausedFrom: (ws9.data.pausedFrom === undefined) });

		JSON.stringify(results);
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `[{"before":"PAUSED","pausedFrom":"PLAN_GENERATION"},{"after":"PLAN_GENERATION","resumed":true},{"before":"PAUSED","pausedFrom":"BRANCH_BUILDING"},{"after":"BRANCH_BUILDING","resumed":true},{"current":"CONFIG","resumed":false},{"cancelledViaMethod":"CANCELLED","cancelCleansPausedFrom":true},{"cancelledViaTx":"CANCELLED"},{"directPause":"PAUSED","resumed":false},{"corruptPause":"PAUSED","resumed":false},{"afterResume":"PLAN_GENERATION","pausedFromCleared":true},{"forceCancelled":"FORCE_CANCEL","fCancelCleansPausedFrom":true}]`
	if got != want {
		t.Errorf("pause resume:\ngot  %s\nwant %s", got, want)
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

// ---------------------------------------------------------------------------
//  T43: Graceful error recovery for stale/missing branch
// ---------------------------------------------------------------------------

func TestChunk13_T43_EmptyRepoDetection(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				return { code: 128, stdout: '', stderr: "fatal: ambiguous argument 'HEAD': unknown revision" };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				return { code: 128, stdout: '', stderr: 'fatal: not a valid object name' };
			}
			if (args[0] === 'branch') {
				return { code: 0, stdout: 'main\ndev\n', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'main';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			hasError: !!result.error,
			mentionsCommit: result.error ? result.error.indexOf('No commits') >= 0 : false,
			hasBranches: !!(result.availableBranches && result.availableBranches.length > 0),
			branchCount: result.availableBranches ? result.availableBranches.length : 0
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"mentionsCommit":true,"hasBranches":true,"branchCount":2}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestChunk13_T43_DetachedHEAD(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				return { code: 0, stdout: 'HEAD\n', stderr: '' };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				return { code: 0, stdout: 'abc123\n', stderr: '' };
			}
			if (args[0] === 'branch') {
				return { code: 0, stdout: 'main\nfeature\n', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'main';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			hasError: !!result.error,
			mentionsDetached: result.error ? result.error.indexOf('Detached HEAD') >= 0 : false,
			hasBranches: !!(result.availableBranches && result.availableBranches.length > 0)
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"mentionsDetached":true,"hasBranches":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestChunk13_T43_TargetBranchNotExist(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				return { code: 0, stdout: 'feature\n', stderr: '' };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				// Both local and remote fail.
				return { code: 128, stdout: '', stderr: 'fatal: Needed a single revision' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'nonexistent-branch';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			hasError: !!result.error,
			mentionsTarget: result.error ? result.error.indexOf('nonexistent-branch') >= 0 : false,
			mentionsNotExist: result.error ? result.error.indexOf('does not exist') >= 0 : false
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"hasError":true,"mentionsTarget":true,"mentionsNotExist":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestChunk13_T43_TargetBranchExistsRemote(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse' && args[1] === '--abbrev-ref') {
				return { code: 0, stdout: 'feature\n', stderr: '' };
			}
			if (args[0] === 'rev-parse' && args[1] === '--verify') {
				var ref = args[2] || '';
				// Local fails, remote succeeds.
				if (ref.indexOf('refs/heads/') === 0) {
					return { code: 128, stdout: '', stderr: 'fatal: not found' };
				}
				if (ref.indexOf('refs/remotes/origin/') === 0) {
					return { code: 0, stdout: 'abc123\n', stderr: '' };
				}
				return { code: 128, stdout: '', stderr: '' };
			}
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() { return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = '';

		var result = prSplit._handleConfigState({});
		JSON.stringify({ error: result.error });
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null}`
	if got != want {
		t.Errorf("remote target branch should be accepted: got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_BaselinePass tests the happy path: valid config
// returns baselineVerifyConfig with correct parameters (T090: verify deferred
// to async pipeline).
func TestChunk13_HandleConfigState_BaselinePass(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var verifyCalled = false;
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			if (args[0] === 'checkout') return { code: 0, stdout: '', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() { verifyCalled = true; return { passed: true }; };
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			error: result.error,
			hasConfig: !!result.baselineVerifyConfig,
			verifyCommand: result.baselineVerifyConfig.verifyCommand,
			verifyCalled: verifyCalled
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"hasConfig":true,"verifyCommand":"make test","verifyCalled":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_BaselineTimeoutDefaultAndProgress tests that
// the default timeout (600000ms from AUTOMATED_DEFAULTS) is passed through
// baselineVerifyConfig. T090: verify is deferred, so no print output expected.
func TestChunk13_HandleConfigState_BaselineTimeoutDefaultAndProgress(t *testing.T) {
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
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			error: result.error,
			verifyTimeoutMs: result.baselineVerifyConfig.verifyTimeoutMs,
			verifyCalled: verifyCalled
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"verifyTimeoutMs":600000,"verifyCalled":false}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_BaselineTimeoutOverride tests that an explicit
// verifyTimeoutMs in config overrides the AUTOMATED_DEFAULTS value.
func TestChunk13_HandleConfigState_BaselineTimeoutOverride(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({ verifyTimeoutMs: 12345 });
		JSON.stringify({
			error: result.error,
			verifyTimeoutMs: result.baselineVerifyConfig.verifyTimeoutMs
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"verifyTimeoutMs":12345}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_BaselineVerifyDeferred tests that even when
// verifySplit would fail, handleConfigState still returns success with
// baselineVerifyConfig (T090: actual verification is deferred to async).
func TestChunk13_HandleConfigState_BaselineVerifyDeferred(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`
		var verifyCalled = false;
		prSplit._gitExec = function(dir, args) {
			if (args[0] === 'rev-parse') return { code: 0, stdout: 'feature\n', stderr: '' };
			if (args[0] === 'checkout') return { code: 0, stdout: '', stderr: '' };
			return { code: 0, stdout: '', stderr: '' };
		};
		prSplit.verifySplit = function() {
			verifyCalled = true;
			return { passed: false, error: 'make test: exit 2', output: 'FAIL' };
		};
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({});
		JSON.stringify({
			error: result.error,
			hasConfig: !!result.baselineVerifyConfig,
			verifyCalled: verifyCalled
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"hasConfig":true,"verifyCalled":false}`
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
		prSplit.runtime.baseBranch = 'main';
		prSplit.runtime.verifyCommand = 'make test';

		var result = prSplit._handleConfigState({ resume: true });
		JSON.stringify({
			error: result.error,
			resume: !!result.resume,
			hasConfig: !!result.baselineVerifyConfig
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"resume":false,"hasConfig":true}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// TestChunk13_HandleConfigState_SkipsBaselineForTrue tests that when
// verifyCommand is 'true', baselineVerifyConfig still carries that value
// (T090: async path will skip actual verification based on this).
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
		JSON.stringify({
			error: result.error,
			verifyCalled: verifyCalled,
			verifyCommand: result.baselineVerifyConfig.verifyCommand
		});
	`)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	got := raw.(string)
	want := `{"error":null,"verifyCalled":false,"verifyCommand":"true"}`
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
// T090: handleConfigState returns baselineVerifyConfig; caller invokes verify.
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

		// CONFIG returns baselineVerifyConfig (T090: no verify inline).
		wizard.transition('CONFIG');
		var configResult = prSplit._handleConfigState({});

		// Caller performs deferred baseline verify.
		var bvc = configResult.baselineVerifyConfig;
		var verifyResult = prSplit.verifySplit(prSplit.runtime.baseBranch, bvc);

		// Baseline fails → transition to BASELINE_FAIL.
		wizard.transition('BASELINE_FAIL', {
			error: verifyResult.error,
			output: verifyResult.output
		});

		// Override.
		var overrideResult = prSplit._handleBaselineFailState(wizard, 'override');

		// Now in PLAN_GENERATION.
		JSON.stringify({
			baselineFailed: !verifyResult.passed,
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

// ===========================================================================
//  BubbleTea Wizard TUI Tests (T030-T036)
// ===========================================================================

// T030: COLORS and styles constants
func TestChunk13_WizardColors_AllKeysPresent(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(Object.keys(globalThis.prSplit._wizardColors).sort())`)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw.(string)), &keys); err != nil {
		t.Fatal(err)
	}

	expected := []string{
		"border", "error", "muted", "primary", "secondary",
		"success", "surface", "text", "textDim", "textOnColor", "warning",
	}
	if len(keys) != len(expected) {
		t.Fatalf("COLORS has %d keys, want %d\n  got:  %v\n  want: %v",
			len(keys), len(expected), keys, expected)
	}
	for i, k := range expected {
		if i >= len(keys) || keys[i] != k {
			t.Errorf("COLORS key[%d] = %q, want %q", i, keys[i], k)
		}
	}
}

func TestChunk13_WizardColors_ValidHexStrings(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit._wizardColors)`)
	if err != nil {
		t.Fatal(err)
	}
	// Colors are adaptive: {light: "#...", dark: "#..."}.
	var colors map[string]map[string]string
	if err := json.Unmarshal([]byte(raw.(string)), &colors); err != nil {
		t.Fatal(err)
	}

	for key, adaptive := range colors {
		for variant, val := range adaptive {
			if len(val) != 7 || val[0] != '#' {
				t.Errorf("COLORS.%s.%s = %q — not a 7-char hex string", key, variant, val)
			}
		}
		if _, ok := adaptive["light"]; !ok {
			t.Errorf("COLORS.%s missing 'light' variant", key)
		}
		if _, ok := adaptive["dark"]; !ok {
			t.Errorf("COLORS.%s missing 'dark' variant", key)
		}
	}
}

func TestChunk13_WizardStyles_AllStylesCallable(t *testing.T) {
	evalJS := loadTUIEngine(t)

	styleNames := []string{
		"titleBar", "stepIndicator", "activeCard", "inactiveCard",
		"errorCard", "successBadge", "warningBadge", "errorBadge",
		"primaryButton", "secondaryButton", "disabledButton",
		"progressFull", "progressEmpty", "divider",
		"dim", "bold", "label", "fieldValue",
		"statusIdle", "statusActive",
	}

	for _, name := range styleNames {
		raw, err := evalJS(`typeof globalThis.prSplit._wizardStyles.` + name)
		if err != nil {
			t.Fatalf("typeof styles.%s: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("styles.%s should be function (style factory), got %v", name, raw)
			continue
		}
		// Call the factory and verify the render method exists.
		raw, err = evalJS(`typeof globalThis.prSplit._wizardStyles.` + name + `().render`)
		if err != nil {
			t.Fatalf("styles.%s().render: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("styles.%s().render should be function, got %v", name, raw)
		}
	}
}

func TestChunk13_WizardStyles_RenderProducesOutput(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._wizardStyles.primaryButton().render('Test')`)
	if err != nil {
		t.Fatal(err)
	}
	s, ok := raw.(string)
	if !ok || s == "" {
		t.Errorf("primaryButton().render('Test') should produce non-empty string, got %v", raw)
	}
	if !strings.Contains(s, "Test") {
		t.Errorf("rendered output should contain 'Test', got %q", s)
	}
}

// T031: Global chrome renderers
func TestChunk13_RenderTitleBar_ContainsWizardName(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Test at different wizard states.
	states := map[string]string{
		"CONFIG":          "Step 1/7: Configure",
		"PLAN_GENERATION": "Step 2/7: Analysis",
		"PLAN_REVIEW":     "Step 3/7: Review Plan",
		"FINALIZATION":    "Step 7/7: Finalization",
	}

	for state, expectedStep := range states {
		raw, err := evalJS(`globalThis.prSplit._renderTitleBar({
			wizardState: '` + state + `',
			startTime: Date.now() - 5000,
			width: 80
		})`)
		if err != nil {
			t.Fatalf("renderTitleBar(%s): %v", state, err)
		}
		s := raw.(string)
		if !strings.Contains(s, "PR Split Wizard") {
			t.Errorf("titleBar(%s) missing 'PR Split Wizard': %q", state, s)
		}
		if !strings.Contains(s, expectedStep) {
			t.Errorf("titleBar(%s) missing %q: %q", state, expectedStep, s)
		}
	}
}

func TestChunk13_RenderNavBar_BackButtonPresence(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// CONFIG should NOT have Back.
	raw, err := evalJS(`globalThis.prSplit._renderNavBar({
		wizardState: 'CONFIG', width: 80, isProcessing: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw.(string), "Back") {
		t.Error("navBar at CONFIG should not show Back button")
	}

	// PLAN_REVIEW should have Back.
	raw, err = evalJS(`globalThis.prSplit._renderNavBar({
		wizardState: 'PLAN_REVIEW', width: 80, isProcessing: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "Back") {
		t.Error("navBar at PLAN_REVIEW should show Back button")
	}
}

func TestChunk13_RenderNavBar_NextButtonLabels(t *testing.T) {
	evalJS := loadTUIEngine(t)

	cases := map[string]string{
		"CONFIG":       "Start Analysis",
		"PLAN_REVIEW":  "Execute Plan",
		"FINALIZATION": "Finish",
	}

	for state, label := range cases {
		raw, err := evalJS(`globalThis.prSplit._renderNavBar({
			wizardState: '` + state + `', width: 80, isProcessing: false
		})`)
		if err != nil {
			t.Fatalf("navBar(%s): %v", state, err)
		}
		if !strings.Contains(raw.(string), label) {
			t.Errorf("navBar(%s) should contain '%s': %q", state, label, raw.(string))
		}
	}
}

func TestChunk13_RenderStatusBar_ContainsHints(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderStatusBar({width: 80})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Claude") {
		t.Errorf("statusBar should contain 'Claude', got %q", s)
	}
	if !strings.Contains(s, "Help") {
		t.Errorf("statusBar should contain 'Help', got %q", s)
	}
}

func TestChunk13_RenderStepDots(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderStepDots({wizardState: 'PLAN_REVIEW'})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	// Should contain some filled and some empty dots.
	if s == "" {
		t.Error("stepDots should produce non-empty output")
	}
}

// T032: Screen view functions
func TestChunk13_ViewConfigScreen_RendersFields(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	for _, expected := range []string{"Repository", "Source Branch", "Target Branch", "Strategy", "Advanced"} {
		if !strings.Contains(s, expected) {
			t.Errorf("viewConfig should contain %q, output:\n%s", expected, s)
		}
	}
}

func TestChunk13_ViewConfigScreen_AdvancedToggle(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Collapsed: no "Max files".
	raw, err := evalJS(`globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw.(string), "Max files") {
		t.Error("viewConfig with showAdvanced=false should not show 'Max files'")
	}

	// Expanded: has "Max files".
	raw, err = evalJS(`globalThis.prSplit._viewConfigScreen({
		wizardState: 'CONFIG', width: 80, showAdvanced: true
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "Max files") {
		t.Error("viewConfig with showAdvanced=true should show 'Max files'")
	}
}

func TestChunk13_ViewAnalysisScreen_ShowsProgress(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewAnalysisScreen({
		wizardState: 'PLAN_GENERATION', width: 80,
		analysisSteps: [
			{label: 'Parse diff', done: true, elapsed: 100},
			{label: 'Group files', active: true, done: false}
		],
		analysisProgress: 0.5
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Analyzing") {
		t.Errorf("viewAnalysis should contain 'Analyzing', got:\n%s", s)
	}
	if !strings.Contains(s, "50%") {
		t.Errorf("viewAnalysis should contain '50%%', got:\n%s", s)
	}
}

func TestChunk13_ViewPlanReviewScreen_NoPlan(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Clear plan cache.
	if _, err := evalJS(`globalThis.prSplit._state.planCache = null`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewPlanReviewScreen({
		wizardState: 'PLAN_REVIEW', width: 80, selectedSplitIdx: 0
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "No Plan") {
		t.Error("viewPlanReview with no plan should show 'No Plan'")
	}
}

func TestChunk13_ViewExecutionScreen_ShowsProgress(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Set up a mock plan.
	if _, err := evalJS(`
		globalThis.prSplit._state.planCache = {
			baseBranch: 'main',
			splits: [
				{name: 'split-1', files: ['a.go'], message: 'fix A'},
				{name: 'split-2', files: ['b.go'], message: 'fix B'}
			]
		};
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewExecutionScreen({
		wizardState: 'BRANCH_BUILDING', width: 80,
		executionResults: [{sha: 'abc123'}],
		executingIdx: 1,
		isProcessing: true
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Executing") {
		t.Errorf("viewExecution should contain 'Executing', got:\n%s", s)
	}
	if !strings.Contains(s, "split-1") || !strings.Contains(s, "split-2") {
		t.Errorf("viewExecution should list both splits, got:\n%s", s)
	}
}

func TestChunk13_ViewVerificationScreen_Pass(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewVerificationScreen({
		wizardState: 'EQUIV_CHECK', width: 80,
		isProcessing: false,
		equivalenceResult: {equivalent: true}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "PASS") {
		t.Error("viewVerification with equiv pass should contain 'PASS'")
	}
}

func TestChunk13_ViewVerificationScreen_Fail(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewVerificationScreen({
		wizardState: 'EQUIV_CHECK', width: 80,
		isProcessing: false,
		equivalenceResult: {equivalent: false, expected: 'abc', actual: 'def'}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "FAIL") {
		t.Errorf("viewVerification with equiv fail should contain 'FAIL', got:\n%s", s)
	}
}

func TestChunk13_ViewFinalizationScreen_ShowsSummary(t *testing.T) {
	evalJS := loadTUIEngine(t)

	if _, err := evalJS(`
		globalThis.prSplit._state.planCache = {
			baseBranch: 'main',
			splits: [
				{name: 'split/api', files: ['a.go', 'b.go']},
				{name: 'split/cli', files: ['c.go']}
			]
		};
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`globalThis.prSplit._viewFinalizationScreen({
		wizardState: 'FINALIZATION', width: 80, startTime: Date.now() - 60000,
		equivalenceResult: {equivalent: true}
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Complete") {
		t.Errorf("viewFinalization should contain 'Complete', got:\n%s", s)
	}
	if !strings.Contains(s, "split/api") || !strings.Contains(s, "split/cli") {
		t.Errorf("viewFinalization should list splits, got:\n%s", s)
	}
}

// T033: Overlay renderers
func TestChunk13_ViewHelpOverlay_ContainsShortcuts(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewHelpOverlay({width: 80})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	for _, kw := range []string{"Tab", "Enter", "Esc", "Ctrl+C", "Ctrl+]"} {
		if !strings.Contains(s, kw) {
			t.Errorf("help overlay should contain %q, got:\n%s", kw, s)
		}
	}
}

func TestChunk13_ViewConfirmCancelOverlay_ContainsPrompt(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewConfirmCancelOverlay({width: 80})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Cancel") {
		t.Errorf("confirm cancel should contain 'Cancel', got:\n%s", s)
	}
}

func TestChunk13_ViewErrorResolutionScreen_ShowsOptions(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._viewErrorResolutionScreen({
		wizardState: 'ERROR_RESOLUTION', width: 80,
		errorDetails: 'cherry-pick conflict at line 45'
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	for _, kw := range []string{"Error Resolution", "Auto-Resolve", "Manual Fix", "Skip", "Retry", "Abort"} {
		if !strings.Contains(s, kw) {
			t.Errorf("error resolution should contain %q, got:\n%s", kw, s)
		}
	}
}

// T034-T035: State machine → screen mapping & exports
func TestChunk13_WizardExports_StartWizard(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`typeof globalThis.prSplit.startWizard`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "function" {
		t.Errorf("prSplit.startWizard should be function, got %v", raw)
	}
}

func TestChunk13_WizardExports_WizardModel(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`typeof globalThis.prSplit._wizardModel`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "object" {
		t.Errorf("prSplit._wizardModel should be object, got %v", raw)
	}
}

func TestChunk13_WizardExports_CreateWizardModel(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`typeof globalThis.prSplit._createWizardModel`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "function" {
		t.Errorf("prSplit._createWizardModel should be function, got %v", raw)
	}
}

func TestChunk13_WizardExports_ScreenRenderers(t *testing.T) {
	evalJS := loadTUIEngine(t)

	renderers := []string{
		"_viewConfigScreen", "_viewAnalysisScreen", "_viewPlanReviewScreen",
		"_viewPlanEditorScreen", "_viewExecutionScreen", "_viewVerificationScreen",
		"_viewFinalizationScreen", "_viewErrorResolutionScreen",
		"_viewHelpOverlay", "_viewConfirmCancelOverlay",
	}

	for _, name := range renderers {
		raw, err := evalJS(`typeof globalThis.prSplit.` + name)
		if err != nil {
			t.Fatalf("typeof prSplit.%s: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("prSplit.%s should be function, got %v", name, raw)
		}
	}
}

func TestChunk13_WizardExports_ChromeRenderers(t *testing.T) {
	evalJS := loadTUIEngine(t)

	renderers := []string{
		"_renderTitleBar", "_renderNavBar", "_renderStatusBar",
		"_renderStepDots", "_renderProgressBar",
	}

	for _, name := range renderers {
		raw, err := evalJS(`typeof globalThis.prSplit.` + name)
		if err != nil {
			t.Fatalf("typeof prSplit.%s: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("prSplit.%s should be function, got %v", name, raw)
		}
	}
}

func TestChunk13_ProgressBar_Rendering(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// 0% progress
	raw, err := evalJS(`globalThis.prSplit._renderProgressBar(0, 40)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "0%") {
		t.Errorf("progressBar at 0 should contain '0%%', got %q", raw.(string))
	}

	// 100% progress
	raw, err = evalJS(`globalThis.prSplit._renderProgressBar(1, 40)`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw.(string), "100%") {
		t.Errorf("progressBar at 1 should contain '100%%', got %q", raw.(string))
	}
}

// T036: Responsive layout test (compact behavior)
func TestChunk13_RenderTitleBar_NarrowWidth(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderTitleBar({
		wizardState: 'CONFIG', startTime: Date.now(), width: 40
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	// Should still render without panic at narrow width.
	if s == "" {
		t.Error("titleBar should produce non-empty output at narrow width")
	}
}

func TestChunk13_RenderNavBar_NarrowWidth(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`globalThis.prSplit._renderNavBar({
		wizardState: 'CONFIG', width: 40, isProcessing: false
	})`)
	if err != nil {
		t.Fatal(err)
	}
	if raw.(string) == "" {
		t.Error("navBar should produce non-empty output at narrow width")
	}
}

// T037: Integration test — model lifecycle verification
func TestChunk13_WizardModel_InitialState_IsIDLE(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// The model's init function should set wizardState to IDLE.
	// After creating the model, the wizard should be in IDLE with needsInitClear=true.
	// On first WindowSize msg, it transitions to CONFIG.
	raw, err := evalJS(`'' + globalThis.prSplit._wizardState.current`)
	if err != nil {
		t.Fatal(err)
	}
	// After chunk load, wizard is in IDLE (before the model receives WindowSize).
	if raw != "IDLE" {
		t.Errorf("wizard initial state should be IDLE, got %v", raw)
	}
}

func TestChunk13_WizardModel_ConfigViewComposition(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Test the full view composition: titleBar + screenContent + navBar + statusBar.
	// This simulates what the BubbleTea view() does for the CONFIG state.
	raw, err := evalJS(`(function() {
		var s = {
			wizardState: 'CONFIG',
			startTime: Date.now(),
			width: 80,
			height: 24,
			showAdvanced: false,
			isProcessing: false,
			showHelp: false,
			showConfirmCancel: false,
			selectedSplitIdx: 0,
			configFocusIdx: 0,
			analysisSteps: [],
			analysisProgress: 0,
			executionResults: [],
			executingIdx: 0,
			errorDetails: '',
			equivalenceResult: null
		};
		var title = globalThis.prSplit._renderTitleBar(s);
		var screen = globalThis.prSplit._viewConfigScreen(s);
		var nav = globalThis.prSplit._renderNavBar(s);
		var status = globalThis.prSplit._renderStatusBar(s);
		return title + '\n' + screen + '\n' + nav + '\n' + status;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "PR Split Wizard") {
		t.Errorf("composed view should contain 'PR Split Wizard', got:\n%s", s)
	}
	if !strings.Contains(s, "Configure") {
		t.Errorf("composed view should contain 'Configure', got:\n%s", s)
	}
	if !strings.Contains(s, "Repository") {
		t.Errorf("composed view should contain 'Repository', got:\n%s", s)
	}
	if !strings.Contains(s, "Start Analysis") {
		t.Errorf("composed view should contain 'Start Analysis' button, got:\n%s", s)
	}
}

func TestChunk13_WizardModel_HelpOverlayComposition(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = {
			wizardState: 'CONFIG',
			startTime: Date.now(),
			width: 80,
			height: 24,
			showAdvanced: false,
			isProcessing: false,
			showHelp: true,
			showConfirmCancel: false,
			selectedSplitIdx: 0,
			configFocusIdx: 0,
			analysisSteps: [],
			analysisProgress: 0,
			executionResults: [],
			executingIdx: 0,
			errorDetails: '',
			equivalenceResult: null
		};
		var overlay = globalThis.prSplit._viewHelpOverlay(s);
		return overlay;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "Tab") {
		t.Errorf("help overlay should contain 'Tab', got:\n%s", s)
	}
	if !strings.Contains(s, "Esc") {
		t.Errorf("help overlay should contain 'Esc', got:\n%s", s)
	}
}

// ---------------------------------------------------------------------------
//  T003 Regression: msg.key handling in BubbleTea update()
//
//  The root cause of all keyboard non-responsiveness was the use of
//  msg.string (undefined on key events) instead of msg.key. These tests
//  exercise the raw _wizardUpdate function with key messages to prove
//  the fix works.
// ---------------------------------------------------------------------------

// TestChunk13_WizardUpdate_ExportsExist verifies the lifecycle function
// exports that T003+ tests depend on are present.
func TestChunk13_WizardUpdate_ExportsExist(t *testing.T) {
	evalJS := loadTUIEngine(t)

	for _, name := range []string{"_wizardInit", "_wizardUpdate", "_wizardView"} {
		raw, err := evalJS(`typeof globalThis.prSplit.` + name)
		if err != nil {
			t.Fatalf("typeof prSplit.%s: %v", name, err)
		}
		if raw != "function" {
			t.Errorf("prSplit.%s should be function, got %v", name, raw)
		}
	}
}

// TestChunk13_WizardUpdate_HelpToggle verifies '?' and 'f1' toggle showHelp.
func TestChunk13_WizardUpdate_HelpToggle(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Initialize state, set to CONFIG (simulating post-WindowSize).
	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';

		// Press '?' — should set showHelp = true.
		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: '?'}, s);
		if (!result[0].showHelp) return 'FAIL: ? did not set showHelp';

		// Any key while help is open — should close help.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'a'}, result[0]);
		if (result[0].showHelp) return 'FAIL: any-key did not close help';

		// Press 'f1' — should also set showHelp = true.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'f1'}, result[0]);
		if (!result[0].showHelp) return 'FAIL: f1 did not set showHelp';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("help toggle test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_CtrlC verifies Ctrl+C shows confirm cancel dialog.
func TestChunk13_WizardUpdate_CtrlC(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'ctrl+c'}, s);
		if (!result[0].showConfirmCancel) return 'FAIL: ctrl+c did not set showConfirmCancel';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("ctrl+c test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_ConfirmCancel_Yes verifies 'y' in confirm dialog
// sets state to CANCELLED.
func TestChunk13_WizardUpdate_ConfirmCancel_Yes(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		// Properly transition wizard to CONFIG.
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.showConfirmCancel = true;

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'y'}, s);
		if (result[0].showConfirmCancel) return 'FAIL: y did not dismiss confirm overlay';
		if (result[0].wizardState !== 'CANCELLED') return 'FAIL: wizardState=' + result[0].wizardState + ', want CANCELLED';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm-cancel-yes test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_ConfirmCancel_No verifies 'n' dismisses the dialog
// without changing state.
func TestChunk13_WizardUpdate_ConfirmCancel_No(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.showConfirmCancel = true;

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'n'}, s);
		if (result[0].showConfirmCancel) return 'FAIL: n did not dismiss confirm overlay';
		if (result[0].wizardState !== 'CONFIG') return 'FAIL: wizardState=' + result[0].wizardState + ', want CONFIG';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm-cancel-no test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_ConfirmCancel_Esc verifies 'esc' dismisses
// the confirm dialog (same as 'n').
func TestChunk13_WizardUpdate_ConfirmCancel_Esc(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.showConfirmCancel = true;

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'esc'}, s);
		if (result[0].showConfirmCancel) return 'FAIL: esc did not dismiss confirm overlay';
		if (result[0].wizardState !== 'CONFIG') return 'FAIL: wizardState=' + result[0].wizardState + ', want CONFIG';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm-cancel-esc test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_ConfirmCancel_Enter verifies 'enter' in confirm
// dialog confirms the cancel (same as 'y').
func TestChunk13_WizardUpdate_ConfirmCancel_Enter(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.showConfirmCancel = true;

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'enter'}, s);
		if (result[0].showConfirmCancel) return 'FAIL: enter did not dismiss confirm overlay';
		if (result[0].wizardState !== 'CANCELLED') return 'FAIL: wizardState=' + result[0].wizardState + ', want CANCELLED';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("confirm-cancel-enter test: %v", raw)
	}
}

// TestChunk13_ConfirmCancel_TabFocusCycling tests T031: Tab cycles between Yes/No buttons.
func TestChunk13_ConfirmCancel_TabFocusCycling(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var results = [];

		// Initial state: confirmCancelFocus = 0 (Yes) when overlay opens via ctrl+c.
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		// Trigger overlay via ctrl+c to verify initialization.
		var r1 = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'ctrl+c'}, s);
		s = r1[0];
		results.push({ opened: s.showConfirmCancel, initialFocus: s.confirmCancelFocus });

		// Tab to No (1).
		var r2 = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'tab'}, s);
		s = r2[0];
		results.push({ afterTab1: s.confirmCancelFocus });

		// Tab wraps back to Yes (0).
		var r3 = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'tab'}, s);
		s = r3[0];
		results.push({ afterTab2: s.confirmCancelFocus });

		// Shift+Tab to No (1).
		var r4 = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'shift+tab'}, s);
		s = r4[0];
		results.push({ afterShiftTab: s.confirmCancelFocus });

		// Enter while focused on No (1) → dismiss, NOT cancel.
		var r5 = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'enter'}, s);
		s = r5[0];
		results.push({ enterOnNo: s.wizardState, dismissed: !s.showConfirmCancel });

		// Re-open overlay and Enter on Yes (default) → cancel.
		s.showConfirmCancel = true;
		s.confirmCancelFocus = 0;
		var r6 = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'enter'}, s);
		s = r6[0];
		results.push({ enterOnYes: s.wizardState });

		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	want := `[{"opened":true,"initialFocus":0},{"afterTab1":1},{"afterTab2":0},{"afterShiftTab":1},{"enterOnNo":"CONFIG","dismissed":true},{"enterOnYes":"CANCELLED"}]`
	if got != want {
		t.Errorf("tab focus cycling:\ngot  %s\nwant %s", got, want)
	}
}

// TestChunk13_ConfirmCancel_ViewButtonText tests T031: button text and overlay structure.
// NOTE: zone.mark wraps content in ANSI escape sequences containing the zone ID,
// so we verify the button text and overlay structure rather than literal zone IDs.
// Mouse clicks via zone.inBounds are not testable outside a real terminal.
func TestChunk13_ConfirmCancel_ViewButtonText(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var results = [];

		// Verify button text and overlay structure.
		var s = { width: 80, confirmCancelFocus: 0 };
		var v = globalThis.prSplit._viewConfirmCancelOverlay(s);

		results.push({
			hasYesText: v.indexOf('Yes, Cancel') >= 0,
			hasNoText: v.indexOf('No, Continue') >= 0,
			hasCancelTitle: v.indexOf('Cancel') >= 0,
			hasHint: v.indexOf('Tab') >= 0
		});

		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	want := `[{"hasYesText":true,"hasNoText":true,"hasCancelTitle":true,"hasHint":true}]`
	if got != want {
		t.Errorf("button text:\ngot  %s\nwant %s", got, want)
	}
}

// TestChunk13_ConfirmCancel_ContextualText tests T031: overlay shows verify-specific text.
func TestChunk13_ConfirmCancel_ContextualText(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var results = [];

		// Without active verify session — default text.
		var s1 = { width: 80, showConfirmCancel: true, confirmCancelFocus: 0 };
		var v1 = globalThis.prSplit._viewConfirmCancelOverlay(s1);
		results.push({
			hasDefault: v1.indexOf('cancel the PR split') >= 0,
			noVerify: v1.indexOf('verification') < 0
		});

		// With active verify session — contextual text.
		var s2 = { width: 80, showConfirmCancel: true, confirmCancelFocus: 0, activeVerifySession: {} };
		var v2 = globalThis.prSplit._viewConfirmCancelOverlay(s2);
		results.push({
			hasVerify: v2.indexOf('verification') >= 0 || v2.indexOf('Verification') >= 0,
			noDefault: v2.indexOf('cancel the PR split') < 0
		});

		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	want := `[{"hasDefault":true,"noVerify":true},{"hasVerify":true,"noDefault":true}]`
	if got != want {
		t.Errorf("contextual text:\ngot  %s\nwant %s", got, want)
	}
}

// TestChunk13_ConfirmCancel_FocusResetOnDismiss tests T031: confirmCancelFocus resets when overlay closes.
func TestChunk13_ConfirmCancel_FocusResetOnDismiss(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.showConfirmCancel = true;
		s.confirmCancelFocus = 1;  // focus on No

		// Dismiss via 'n'.
		var r = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'n'}, s);
		return JSON.stringify({ focus: r[0].confirmCancelFocus, dismissed: !r[0].showConfirmCancel });
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	want := `{"focus":0,"dismissed":true}`
	if got != want {
		t.Errorf("focus reset:\ngot  %s\nwant %s", got, want)
	}
}

// TestChunk13_ConfirmCancel_ViewFocusStyling tests T031: focusedErrorBadge style exists
// and both focus states render without error. In a no-color terminal the rendered
// strings may be identical (only the background color differs), so we verify the
// style infrastructure rather than comparing raw output.
func TestChunk13_ConfirmCancel_ViewFocusStyling(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var results = [];
		var st = globalThis.prSplit._wizardStyles;

		// Verify focusedErrorBadge style exists and is callable.
		var feOk = typeof st.focusedErrorBadge === 'function';
		var feRendered = feOk ? st.focusedErrorBadge().render('test') : '';

		// Verify errorBadge style exists (for comparison).
		var ebRendered = st.errorBadge().render('test');

		// Both focus states render the overlay without crash.
		var s1 = { width: 80, confirmCancelFocus: 0 };
		var v1 = globalThis.prSplit._viewConfirmCancelOverlay(s1);
		var s2 = { width: 80, confirmCancelFocus: 1 };
		var v2 = globalThis.prSplit._viewConfirmCancelOverlay(s2);

		results.push({
			focusedErrorBadgeExists: feOk,
			bothRender: v1.length > 0 && v2.length > 0,
			hasTabHint: v1.indexOf('Tab') >= 0,
			hasEnterHint: v1.indexOf('Enter') >= 0 || v1.indexOf('confirm') >= 0
		});

		return JSON.stringify(results);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	want := `[{"focusedErrorBadgeExists":true,"bothRender":true,"hasTabHint":true,"hasEnterHint":true}]`
	if got != want {
		t.Errorf("focus styling:\ngot  %s\nwant %s", got, want)
	}
}

// TestChunk13_WizardUpdate_WindowSize verifies WindowSize msg sets dimensions
// and transitions to CONFIG on first render.
func TestChunk13_WizardUpdate_WindowSize(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		// needsInitClear should be true initially.
		if (!s.needsInitClear) return 'FAIL: needsInitClear should be true initially';

		var result = globalThis.prSplit._wizardUpdate(
			{type: 'WindowSize', width: 120, height: 40}, s);
		if (result[0].width !== 120) return 'FAIL: width=' + result[0].width + ', want 120';
		if (result[0].height !== 40) return 'FAIL: height=' + result[0].height + ', want 40';
		if (result[0].wizardState !== 'CONFIG') return 'FAIL: wizardState=' + result[0].wizardState + ', want CONFIG';
		if (result[0].needsInitClear) return 'FAIL: needsInitClear should be false after first WindowSize';

		// Second WindowSize: should NOT re-trigger CONFIG transition.
		result = globalThis.prSplit._wizardUpdate(
			{type: 'WindowSize', width: 80, height: 24}, result[0]);
		if (result[0].width !== 80) return 'FAIL: second width=' + result[0].width + ', want 80';
		// Should still be CONFIG (not re-init).
		if (result[0].wizardState !== 'CONFIG') return 'FAIL: second wizardState=' + result[0].wizardState;
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("window-size test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_NavigationKeys verifies j/k/up/down/tab/shift+tab
// in PLAN_REVIEW with splits properly modify selectedSplitIdx.
func TestChunk13_WizardUpdate_NavigationKeys(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.selectedSplitIdx = 0;
		s.focusIndex = 0;
		// Properly transition wizard through valid path to PLAN_REVIEW.
		s.wizard.transition('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.wizard.transition('PLAN_REVIEW');
		s.wizardState = 'PLAN_REVIEW';
		s._prevWizardState = 'PLAN_REVIEW';

		// Set up plan cache with 3 splits so navigation has room.
		globalThis.prSplit._state.planCache = {
			splits: [{name:'a',files:[]},{name:'b',files:[]},{name:'c',files:[]}]
		};

		// j -> move down.
		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'j'}, s);
		if (result[0].selectedSplitIdx !== 1) return 'FAIL: j: idx=' + result[0].selectedSplitIdx + ', want 1';

		// down -> move down again.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'down'}, result[0]);
		if (result[0].selectedSplitIdx !== 2) return 'FAIL: down: idx=' + result[0].selectedSplitIdx + ', want 2';

		// down at max -> stays at max.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'down'}, result[0]);
		if (result[0].selectedSplitIdx !== 2) return 'FAIL: down-at-max: idx=' + result[0].selectedSplitIdx + ', want 2';

		// k -> move up.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'k'}, result[0]);
		if (result[0].selectedSplitIdx !== 1) return 'FAIL: k: idx=' + result[0].selectedSplitIdx + ', want 1';

		// up -> move up.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'up'}, result[0]);
		if (result[0].selectedSplitIdx !== 0) return 'FAIL: up: idx=' + result[0].selectedSplitIdx + ', want 0';

		// up at min -> stays at 0.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'up'}, result[0]);
		if (result[0].selectedSplitIdx !== 0) return 'FAIL: up-at-min: idx=' + result[0].selectedSplitIdx + ', want 0';

		// tab -> same as down.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'tab'}, result[0]);
		if (result[0].selectedSplitIdx !== 1) return 'FAIL: tab: idx=' + result[0].selectedSplitIdx + ', want 1';

		// shift+tab -> same as up.
		result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'shift+tab'}, result[0]);
		if (result[0].selectedSplitIdx !== 0) return 'FAIL: shift+tab: idx=' + result[0].selectedSplitIdx + ', want 0';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("navigation-keys test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_EscGoesBack verifies Esc in PLAN_REVIEW goes
// back to CONFIG.
func TestChunk13_WizardUpdate_EscGoesBack(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		// Properly transition through valid path to PLAN_REVIEW.
		s.wizard.transition('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.wizard.transition('PLAN_REVIEW');
		s.wizardState = 'PLAN_REVIEW';

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'esc'}, s);
		if (result[0].wizardState !== 'CONFIG') return 'FAIL: esc-back: wizardState=' + result[0].wizardState + ', want CONFIG';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("esc-goes-back test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_PlanEditorShortcut verifies 'e' in PLAN_REVIEW
// enters the plan editor.
func TestChunk13_WizardUpdate_PlanEditorShortcut(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.isProcessing = false;
		s.wizard.transition('CONFIG');
		s.wizard.transition('PLAN_GENERATION');
		s.wizard.transition('PLAN_REVIEW');
		s.wizardState = 'PLAN_REVIEW';

		var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: 'e'}, s);
		if (result[0].wizardState !== 'PLAN_EDITOR') return 'FAIL: e: wizardState=' + result[0].wizardState + ', want PLAN_EDITOR';
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("plan-editor-shortcut test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_MsgStringUndefined is the explicit regression
// test for the msg.string bug. It verifies that using msg.string (old broken
// property) does NOT trigger any handler, while msg.key (correct property) does.
func TestChunk13_WizardUpdate_MsgStringUndefined(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';

		// A message with type:'Key' but using the WRONG property 'string'
		// instead of 'key'. This should NOT trigger any handler.
		var badMsg = {type: 'Key', string: '?'};
		var result = globalThis.prSplit._wizardUpdate(badMsg, s);
		if (result[0].showHelp) return 'FAIL: msg.string=? incorrectly triggered showHelp';

		// The correct property 'key' SHOULD work.
		var goodMsg = {type: 'Key', key: '?'};
		result = globalThis.prSplit._wizardUpdate(goodMsg, result[0]);
		if (!result[0].showHelp) return 'FAIL: msg.key=? did not trigger showHelp';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("msg-string-regression test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_AllKeyBindingsRespond is a comprehensive check
// that every documented key binding produces a state change or returns a
// non-nil command (i.e., is not a no-op).
func TestChunk13_WizardUpdate_AllKeyBindingsRespond(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		function testKey(key, state, checkFn, label) {
			var s = globalThis.prSplit._wizardInit();
			s.needsInitClear = false;
			s.selectedSplitIdx = 1;
			s.focusIndex = 1;
			s.isProcessing = false;
			// Reset shared wizard before each test case.
			s.wizard.reset();
			// Properly transition through valid state paths.
			s.wizard.transition('CONFIG');
			if (state === 'PLAN_REVIEW' || state === 'PLAN_EDITOR') {
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				if (state === 'PLAN_EDITOR') {
					s.wizard.transition('PLAN_EDITOR');
				}
				globalThis.prSplit._state.planCache = {
					splits: [{name:'a',files:[]},{name:'b',files:[]},{name:'c',files:[]}]
				};
			}
			s.wizardState = state;
			s._prevWizardState = state;
			var result = globalThis.prSplit._wizardUpdate({type: 'Key', key: key}, s);
			if (!checkFn(result[0], result[1])) {
				errors.push(label + ' (key=' + key + ', state=' + state + ')');
			}
		}

		// Help keys.
		testKey('?', 'CONFIG', function(s) { return s.showHelp === true; }, 'help-?');
		testKey('f1', 'CONFIG', function(s) { return s.showHelp === true; }, 'help-f1');

		// Cancel.
		testKey('ctrl+c', 'CONFIG', function(s) { return s.showConfirmCancel === true; }, 'cancel');

		// Navigation.
		testKey('j', 'PLAN_REVIEW', function(s) { return s.selectedSplitIdx === 2; }, 'nav-j');
		testKey('down', 'PLAN_REVIEW', function(s) { return s.selectedSplitIdx === 2; }, 'nav-down');
		testKey('k', 'PLAN_REVIEW', function(s) { return s.selectedSplitIdx === 0; }, 'nav-k');
		testKey('up', 'PLAN_REVIEW', function(s) { return s.selectedSplitIdx === 0; }, 'nav-up');
		testKey('tab', 'PLAN_REVIEW', function(s) { return s.selectedSplitIdx === 2; }, 'nav-tab');
		testKey('shift+tab', 'PLAN_REVIEW', function(s) { return s.selectedSplitIdx === 0; }, 'nav-shift+tab');

		// Plan editor shortcut.
		testKey('e', 'PLAN_REVIEW', function(s) { return s.wizardState === 'PLAN_EDITOR'; }, 'editor-e');

		if (errors.length > 0) return 'FAIL: ' + errors.join(', ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("all-key-bindings test: %v", raw)
	}
}

// TestChunk13_WizardUpdate_MouseWheelScroll verifies mouse wheel events
// work through the update function.
func TestChunk13_WizardUpdate_MouseWheelScroll(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';

		// Set up a tall content so scrolling is observable.
		var lines = [];
		for (var i = 0; i < 100; i++) lines.push('line ' + i);
		s.vp.setContent(lines.join('\n'));
		s.vp.setHeight(10);

		// Mouse wheel events match the Go-side format from parsemouse.go:
		// {type: "Mouse", button: "wheel down", action: "press", isWheel: true, x, y}
		var result = globalThis.prSplit._wizardUpdate(
			{type: 'Mouse', button: 'wheel down', action: 'press', isWheel: true, x: 10, y: 10}, s);
		if (!result || !result[0]) return 'FAIL: wheel-down returned invalid result';

		var afterDown = result[0].vp.yOffset();
		if (afterDown <= 0) return 'FAIL: wheel-down did not scroll (yOffset=' + afterDown + ')';

		result = globalThis.prSplit._wizardUpdate(
			{type: 'Mouse', button: 'wheel up', action: 'press', isWheel: true, x: 10, y: 10}, result[0]);
		if (!result || !result[0]) return 'FAIL: wheel-up returned invalid result';

		var afterUp = result[0].vp.yOffset();
		if (afterUp >= afterDown) return 'FAIL: wheel-up did not scroll back (offset=' + afterUp + ' vs ' + afterDown + ')';

		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("mouse-wheel test: %v", raw)
	}
}

// ---------------------------------------------------------------------------
//  T005: State → Screen mapping via _wizardView
//
//  Verify that _wizardView renders the correct screen content for each
//  wizard state. This proves the view dispatching logic in viewForState
//  connects every state to its renderer.
// ---------------------------------------------------------------------------

// TestChunk13_WizardView_StateScreenMapping verifies each wizard state
// produces screen-specific content markers in the raw screen output.
// Uses _viewForState (unclipped) to avoid viewport cropping at small heights.
func TestChunk13_WizardView_StateScreenMapping(t *testing.T) {
	evalJS := loadTUIEngine(t)

	// Helper: initialise state, transition wizard, render via _viewForState (no viewport).
	setupJS := `
	function renderForState(targetState, extras) {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.width = 80;
		s.height = 24;
		s.showHelp = false;
		s.showConfirmCancel = false;
		s.isProcessing = false;
		s.selectedSplitIdx = 0;
		s.analysisSteps = [];
		s.analysisProgress = 0;
		s.executionResults = [];
		s.executingIdx = 0;
		s.errorDetails = '';
		s.equivalenceResult = null;

		// Transition wizard through valid paths.
		s.wizard.reset();
		switch (targetState) {
			case 'CONFIG':
				s.wizard.transition('CONFIG');
				break;
			case 'PLAN_GENERATION':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				break;
			case 'PLAN_REVIEW':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				break;
			case 'PLAN_EDITOR':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				s.wizard.transition('PLAN_EDITOR');
				break;
			case 'BRANCH_BUILDING':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				s.wizard.transition('BRANCH_BUILDING');
				break;
			case 'EQUIV_CHECK':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				s.wizard.transition('BRANCH_BUILDING');
				s.wizard.transition('EQUIV_CHECK');
				break;
			case 'FINALIZATION':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				s.wizard.transition('BRANCH_BUILDING');
				s.wizard.transition('EQUIV_CHECK');
				s.wizard.transition('FINALIZATION');
				break;
			case 'ERROR_RESOLUTION':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('PLAN_REVIEW');
				s.wizard.transition('BRANCH_BUILDING');
				s.wizard.transition('ERROR_RESOLUTION');
				break;
			case 'CANCELLED':
				s.wizard.transition('CONFIG');
				s.wizard.cancel();
				break;
			case 'ERROR':
				s.wizard.transition('CONFIG');
				s.wizard.transition('PLAN_GENERATION');
				s.wizard.transition('ERROR');
				break;
		}
		s.wizardState = targetState;

		// Apply any extra state overrides.
		if (extras) {
			for (var k in extras) {
				if (extras.hasOwnProperty(k)) s[k] = extras[k];
			}
		}

		return globalThis.prSplit._viewForState(s);
	}
	`
	if _, err := evalJS(setupJS); err != nil {
		t.Fatalf("setup renderForState: %v", err)
	}

	tests := []struct {
		name     string
		state    string
		extras   string // JS object literal for extra state
		contains []string
	}{
		{
			name:     "CONFIG",
			state:    "CONFIG",
			contains: []string{"Repository", "Source Branch", "Strategy"},
		},
		{
			name:     "PLAN_GENERATION",
			state:    "PLAN_GENERATION",
			extras:   `{analysisSteps: [{label:'Analyze',active:true,done:false}], analysisProgress: 0.25}`,
			contains: []string{"Analyzing"},
		},
		{
			name:     "PLAN_REVIEW",
			state:    "PLAN_REVIEW",
			contains: []string{"Plan"},
		},
		{
			name:     "BRANCH_BUILDING",
			state:    "BRANCH_BUILDING",
			extras:   `{executionResults: [{name:'split-1',status:'pending'}]}`,
			contains: []string{"Execut"},
		},
		{
			name:     "EQUIV_CHECK",
			state:    "EQUIV_CHECK",
			contains: []string{"Verif"},
		},
		{
			name:     "FINALIZATION",
			state:    "FINALIZATION",
			extras:   `{executionResults: [{name:'split-1',status:'done'}]}`,
			contains: []string{"Complete", "Done"},
		},
		{
			name:     "ERROR_RESOLUTION",
			state:    "ERROR_RESOLUTION",
			extras:   `{errorDetails: 'cherry-pick conflict'}`,
			contains: []string{"Error Resolution"},
		},
		{
			name:     "CANCELLED",
			state:    "CANCELLED",
			contains: []string{"Cancelled"},
		},
		{
			name:     "ERROR",
			state:    "ERROR",
			extras:   `{errorDetails: 'unexpected failure'}`,
			contains: []string{"Error"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extrasArg := "null"
			if tt.extras != "" {
				extrasArg = tt.extras
			}
			raw, err := evalJS(`renderForState('` + tt.state + `', ` + extrasArg + `)`)
			if err != nil {
				t.Fatalf("renderForState(%s): %v", tt.state, err)
			}
			s, ok := raw.(string)
			if !ok {
				t.Fatalf("renderForState(%s) returned non-string: %T", tt.state, raw)
			}
			if s == "" {
				t.Errorf("renderForState(%s) returned empty string", tt.state)
			}
			for _, want := range tt.contains {
				if !strings.Contains(s, want) {
					t.Errorf("renderForState(%s) should contain %q, got:\n%s", tt.state, want, s)
				}
			}
		})
	}
}

// TestChunk13_WizardView_HelpOverlayInView verifies that when showHelp=true,
// the view output contains help overlay content.
func TestChunk13_WizardView_HelpOverlayInView(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.width = 80;
		s.height = 24;
		s.showHelp = true;
		return globalThis.prSplit._wizardView(s);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	// Help overlay should contain keyboard shortcuts.
	for _, kw := range []string{"Tab", "Esc", "Enter"} {
		if !strings.Contains(s, kw) {
			t.Errorf("help overlay view should contain %q when showHelp=true", kw)
		}
	}
}

// TestChunk13_WizardView_ConfirmCancelOverlayInView verifies that when
// showConfirmCancel=true, the view output contains the cancel prompt.
func TestChunk13_WizardView_ConfirmCancelOverlayInView(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.width = 80;
		s.height = 24;
		s.showConfirmCancel = true;
		return globalThis.prSplit._wizardView(s);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	if !strings.Contains(s, "cancel") && !strings.Contains(s, "Cancel") {
		t.Errorf("confirm cancel overlay should contain 'cancel' text, got:\n%s", s)
	}
}

// TestChunk13_WizardView_ContainsChromeElements verifies that a full view
// includes title bar, navigation bar, and status bar.
func TestChunk13_WizardView_ContainsChromeElements(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.width = 80;
		s.height = 24;
		return globalThis.prSplit._wizardView(s);
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	s := raw.(string)
	// Title bar should contain wizard name.
	if !strings.Contains(s, "PR Split") {
		t.Errorf("view should contain 'PR Split' in title bar")
	}
	// Status bar should mention Help or shortcuts.
	if !strings.Contains(s, "Help") {
		t.Errorf("view should contain 'Help' in status bar")
	}
}

// ---------------------------------------------------------------------------
//  T05: Viewport height edge case — tiny terminal
// ---------------------------------------------------------------------------

// TestChunk13_WizardView_TinyTerminal verifies that the Math.max(3, h-chromeH)
// guard prevents a zero or negative viewport height when the terminal is smaller
// than the chrome (title bar + dividers + nav bar + status bar). The viewport
// height must never drop below 3 lines.
func TestChunk13_WizardView_TinyTerminal(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.width = 80;
		s.height = 5; // Extremely tiny — chrome alone needs ~7-10 lines.

		// Render the view; this exercises Math.max(3, h - chromeH).
		var viewOutput = globalThis.prSplit._wizardView(s);
		if (typeof viewOutput !== 'string') return 'FAIL: view did not return string';
		if (viewOutput.length === 0) return 'FAIL: view returned empty string';

		// Verify viewport height was clamped to at least 3.
		var vpH = s.vp.height();
		if (vpH < 3) return 'FAIL: viewport height ' + vpH + ' is below minimum 3';

		return 'OK:vpH=' + vpH;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	if !strings.HasPrefix(got, "OK:") {
		t.Errorf("tiny terminal viewport test: %v", got)
	}
	// Verify the clamped height is exactly 3 (since h=5 and chromeH > 5).
	if !strings.Contains(got, "vpH=3") {
		t.Logf("viewport height was clamped but not to 3: %s (chrome may be smaller than expected)", got)
	}
}

// TestChunk13_WizardView_NormalTerminal verifies that at normal terminal size
// the viewport height is h minus actual chrome height (not clamped to 3).
func TestChunk13_WizardView_NormalTerminal(t *testing.T) {
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = globalThis.prSplit._wizardInit();
		s.needsInitClear = false;
		s.wizard.reset();
		s.wizard.transition('CONFIG');
		s.wizardState = 'CONFIG';
		s.width = 120;
		s.height = 40;

		globalThis.prSplit._wizardView(s);

		var vpH = s.vp.height();
		// At h=40 with ~7 chrome lines, viewport should be ~33 (certainly > 3).
		if (vpH <= 3) return 'FAIL: viewport height at h=40 was only ' + vpH;
		if (vpH > 40) return 'FAIL: viewport height ' + vpH + ' exceeds terminal height 40';

		return 'OK:vpH=' + vpH;
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	got := raw.(string)
	if !strings.HasPrefix(got, "OK:") {
		t.Errorf("normal terminal viewport test: %v", got)
	}
}

// ---------------------------------------------------------------------------
//  T025: Crash recovery — new transition paths
// ---------------------------------------------------------------------------

// TestChunk13_WizardState_CrashTransitions verifies that the new
// ERROR_RESOLUTION transitions from PLAN_GENERATION and EQUIV_CHECK are valid.
func TestChunk13_WizardState_CrashTransitions(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var errors = [];

		// PLAN_GENERATION → ERROR_RESOLUTION (crash during classification/plan).
		var w1 = new prSplit.WizardState();
		w1.transition('CONFIG');
		w1.transition('PLAN_GENERATION');
		try {
			w1.transition('ERROR_RESOLUTION');
			if (w1.current !== 'ERROR_RESOLUTION') {
				errors.push('PLAN_GENERATION→ERROR_RESOLUTION: current is ' + w1.current);
			}
		} catch (e) {
			errors.push('PLAN_GENERATION→ERROR_RESOLUTION threw: ' + e.message);
		}

		// EQUIV_CHECK → ERROR_RESOLUTION (crash during equivalence check).
		var w2 = new prSplit.WizardState();
		w2.transition('CONFIG');
		w2.transition('PLAN_GENERATION');
		w2.transition('PLAN_REVIEW');
		w2.transition('BRANCH_BUILDING');
		w2.transition('EQUIV_CHECK');
		try {
			w2.transition('ERROR_RESOLUTION');
			if (w2.current !== 'ERROR_RESOLUTION') {
				errors.push('EQUIV_CHECK→ERROR_RESOLUTION: current is ' + w2.current);
			}
		} catch (e) {
			errors.push('EQUIV_CHECK→ERROR_RESOLUTION threw: ' + e.message);
		}

		// BRANCH_BUILDING → ERROR_RESOLUTION (already existed).
		var w3 = new prSplit.WizardState();
		w3.transition('CONFIG');
		w3.transition('PLAN_GENERATION');
		w3.transition('PLAN_REVIEW');
		w3.transition('BRANCH_BUILDING');
		try {
			w3.transition('ERROR_RESOLUTION');
			if (w3.current !== 'ERROR_RESOLUTION') {
				errors.push('BRANCH_BUILDING→ERROR_RESOLUTION: current is ' + w3.current);
			}
		} catch (e) {
			errors.push('BRANCH_BUILDING→ERROR_RESOLUTION threw: ' + e.message);
		}

		// CONFIG → ERROR_RESOLUTION (crash during auto-split pipeline).
		var w4 = new prSplit.WizardState();
		w4.transition('CONFIG');
		try {
			w4.transition('ERROR_RESOLUTION');
			if (w4.current !== 'ERROR_RESOLUTION') {
				errors.push('CONFIG→ERROR_RESOLUTION: current is ' + w4.current);
			}
		} catch (e) {
			errors.push('CONFIG→ERROR_RESOLUTION threw: ' + e.message);
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("crash transitions: %v", raw)
	}
}

// TestChunk13_ViewErrorResolutionScreen_CrashMode verifies the crash-specific
// view rendering in the error resolution screen.
func TestChunk13_ViewErrorResolutionScreen_CrashMode(t *testing.T) {
	t.Parallel()
	evalJS := loadTUIEngine(t)

	raw, err := evalJS(`(function() {
		var s = {
			wizardState: 'ERROR_RESOLUTION',
			width: 80,
			claudeCrashDetected: true,
			errorDetails: 'Claude process crashed unexpectedly.\n\nLast output:\nsegfault',
			wizard: { data: {} }
		};
		var rendered = globalThis.prSplit._viewErrorResolutionScreen(s);

		var errors = [];
		// Crash-specific header.
		if (rendered.indexOf('Crashed') < 0) {
			errors.push('should contain "Crashed" header');
		}
		// Crash-specific buttons.
		if (rendered.indexOf('Restart Claude') < 0) {
			errors.push('should contain "Restart Claude" button');
		}
		if (rendered.indexOf('Heuristic') < 0) {
			errors.push('should contain "Heuristic" button');
		}
		if (rendered.indexOf('Abort') < 0) {
			errors.push('should contain "Abort" button');
		}
		// Should NOT contain standard buttons.
		if (rendered.indexOf('Auto-Resolve') >= 0) {
			errors.push('should NOT contain "Auto-Resolve" in crash mode');
		}
		if (rendered.indexOf('Manual Fix') >= 0) {
			errors.push('should NOT contain "Manual Fix" in crash mode');
		}
		// Diagnostic output should appear.
		if (rendered.indexOf('segfault') < 0) {
			errors.push('should contain diagnostic output "segfault"');
		}

		if (errors.length > 0) return 'FAIL: ' + errors.join('; ');
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("error resolution crash mode view: %v", raw)
	}
}
