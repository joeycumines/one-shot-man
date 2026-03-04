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
