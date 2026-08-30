package scripting

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func newExampleScriptEngine(t *testing.T) *Engine {
	t.Helper()

	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("example-script", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngineConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = engine.Close()
	})
	engine.SetTestMode(true)
	return engine
}

func loadExampleProgram(t *testing.T, engine *Engine, scriptName string) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))
	scriptPath := filepath.Join(projectDir, "scripts", scriptName)
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("ReadFile(%s) failed: %v", scriptPath, err)
	}

	source := string(content)
	if strings.HasPrefix(source, "#!") {
		if idx := strings.Index(source, "\n"); idx >= 0 {
			source = source[idx+1:]
		} else {
			source = ""
		}
	}

	// Script-specific modelStart patterns (variable name and declaration keyword vary).
	modelStarts := map[string]string{
		"example-15-bouncing-logo.js": "var program = tea.newModel({",
	}
	modelStart, ok := modelStarts[scriptName]
	if !ok {
		modelStart = "const program = tea.newModel({"
	}
	if !strings.Contains(source, modelStart) {
		t.Fatalf("%s missing expected tea.newModel declaration %q", scriptName, modelStart)
	}
	source = strings.Replace(source, modelStart, "const __programConfig = {", 1)

	// Script-specific source pre-processing before runMarker handling.
	switch scriptName {
	case "example-15-bouncing-logo.js":
		// Stub flag.parse(args) — args is not available in test engine.
		source = strings.Replace(source, "fs.parse(args)", "fs.parse([])", 1)
		// Stub termmux session creation — PTY operations fail in test environment.
		const termmuxStub = `var __regressionTermmux = (function() {
	var sid = 'test-sid';
	var session = { close: function() {}, resize: function() {} };
	var mgr = {
		resize: function() {},
		resizeSession: function() {},
		snapshot: function() { return { plainText: 'mock shell', rows: 10, cols: 30, mouseTracking: 0 }; },
		input: function() {},
		close: function() {},
		activeID: function() { return sid; },
		sessions: function() { return [{ target: {name:'',kind:'',id:''}, state:'attached', isActive:true, id:sid }]; },
		enterCopyMode: function() {},
		exitCopyMode: function() {},
		isCopyModeActive: function() { return false; }
	};
	return { session: session, mgr: mgr, sid: sid };
})();

var __regressionTermpane = (function() {
	var bounds = { x: 0, y: 0, width: 80, height: 24 };
	function updatePaneBounds(b) {
		bounds.x = b.x;
		bounds.y = b.y;
		bounds.width = b.width;
		bounds.height = b.height;
	}
	function paneObj() {
		return {
			setBounds: function(r) { updatePaneBounds(r); return this; },
			bounds: function() { return bounds; },
			update: function(msg) { return [this, null]; },
			view: function() {
				return {
					content: 'mock shell pane',
					gen: Date.now(),
					cursor: { x: 1, y: 1, shape: 'block', blink: false }
				};
			},
			close: function() {},
			asBubbleteaModel: function() { return { _type: 'bubbleteaGoModel' }; }
		};
	}
	return { termpane: function() { return paneObj(); } };
})();

var __regressionRequire = require;
require = function(name) {
	if (name === 'osm:termmux') {
		return {
			newBoundedSession: function() { return Promise.resolve(__regressionTermmux); },
			newControlRouter: function() { return { handleKey: function() { return {handled:false}; }, inChordMode: function() { return false; } }; },
			handlePrefixKey: function() { return {action:'Cancel'}; }
		};
	}
	if (name === 'osm:termui/termpane') {
		return __regressionTermpane;
	}
	return __regressionRequire(name);
};
`
		source = termmuxStub + "\n" + source
	}

	runMarker := "tea.run(program);"
	switch scriptName {
	case "minimal-bubbletea-test.js":
		runMarker = "const result = tea.run(program);"
	case "example-02-graphical-todo.js", "benchmark-input-latency.js", "example-13-split-pane.js", "example-15-bouncing-logo.js":
	default:
		t.Fatalf("unsupported script %q", scriptName)
	}

	runIdx := strings.Index(source, runMarker)
	if runIdx < 0 {
		t.Fatalf("%s missing expected run marker %q", scriptName, runMarker)
	}
	modelEndIdx := strings.LastIndex(source[:runIdx], "});")
	if modelEndIdx < 0 {
		t.Fatalf("%s missing expected model terminator", scriptName)
	}
	injectedProgram := `};

const program = tea.newModel(__programConfig);
globalThis.__program = program;
globalThis.__programConfig = __programConfig;`
	source = source[:modelEndIdx] + injectedProgram + source[modelEndIdx+3:]

	replacement := "globalThis.__programStarted = false;"
	if scriptName == "minimal-bubbletea-test.js" {
		replacement = "const result = { __stub: true };\nglobalThis.__programStarted = false;"
	}
	source = strings.Replace(source, runMarker, replacement, 1)

	script := engine.LoadScriptString(scriptName, source)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript(%s) failed: %v", scriptName, err)
	}
	if engine.GetGlobal("__program") == nil {
		t.Fatalf("expected %s to expose __program", scriptName)
	}
	if engine.GetGlobal("__programConfig") == nil {
		t.Fatalf("expected %s to expose __programConfig", scriptName)
	}
}

func runResultScript(t *testing.T, engine *Engine, name, source string) map[string]any {
	t.Helper()
	script := engine.LoadScriptString(name, source)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript(%s) failed: %v", name, err)
	}
	val := engine.GetGlobal("__result")
	result, ok := val.(map[string]any)
	if !ok {
		t.Fatalf("unexpected __result type for %s: %T", name, val)
	}
	return result
}

func TestExampleScriptsReadme_AccurateDescriptions(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))
	content, err := os.ReadFile(filepath.Join(projectDir, "scripts", "README.md"))
	if err != nil {
		t.Fatalf("ReadFile scripts/README.md failed: %v", err)
	}
	readme := string(content)

	if !strings.Contains(readme, "Basic prompt builder using `tui.registerMode()` and class-local state") {
		t.Fatalf("scripts/README.md missing updated example-01 description")
	}
	if !strings.Contains(readme, "Measure key-event-to-next-frame-tick latency") {
		t.Fatalf("scripts/README.md missing updated benchmark description")
	}
}

func TestExample02GraphicalTodo_AddModeCtrlCQuitDoesNotStealQ(t *testing.T) {
	engine := newExampleScriptEngine(t)
	loadExampleProgram(t, engine, "example-02-graphical-todo.js")

	result := runResultScript(t, engine, "example-02-regression", `
var model = __programConfig.init();
model.mode = 'add';
var textareaUpdates = 0;
model.textarea = {
    update: function (msg) {
        textareaUpdates++;
        return [this, { _cmdType: 'textareaUpdate' }];
    },
    setValue: function () {},
    value: function () { return ''; },
    focus: function () {},
    setWidth: function () {},
    view: function () { return ''; }
};

var qRes = __programConfig.update({ type: 'Key', key: 'q' }, model);
var ctrlRes = __programConfig.update({ type: 'Key', key: 'ctrl+c' }, model);

__result = {
    qCmdType: qRes[1] && qRes[1]._cmdType || null,
    qMode: qRes[0].mode,
    textareaUpdates: textareaUpdates,
    ctrlCmdType: ctrlRes[1] && ctrlRes[1]._cmdType || null
};
`)

	if got := result["qCmdType"]; got != "textareaUpdate" {
		t.Fatalf("expected plain q in add mode to flow to textarea.update, got %v", got)
	}
	if got := result["qMode"]; got != "add" {
		t.Fatalf("expected plain q to keep add mode active, got %v", got)
	}
	if got := result["textareaUpdates"]; got != int64(1) {
		t.Fatalf("expected textarea.update to run once for plain q, got %v", got)
	}
	if got := result["ctrlCmdType"]; got != "quit" {
		t.Fatalf("expected ctrl+c in add mode to return tea.quit, got %v", got)
	}
}

func TestBenchmarkInputLatency_SingleTickChainAndCompactView(t *testing.T) {
	engine := newExampleScriptEngine(t)
	loadExampleProgram(t, engine, "benchmark-input-latency.js")

	result := runResultScript(t, engine, "benchmark-regression", `
var initRes = __programConfig.init();
var model = initRes[0];
var keyRes = __programConfig.update({ type: 'Key', key: 'right' }, model);
var resizeRes = __programConfig.update({ type: 'WindowSize', width: 20, height: 8 }, keyRes[0]);
var viewRes = __programConfig.view(resizeRes[0]);
var lines = viewRes.content.split('\n');

__result = {
    initCmdType: initRes[1] && initRes[1]._cmdType || null,
    keyCmdType: keyRes[1] && keyRes[1]._cmdType || null,
    resizeCmdType: resizeRes[1] && resizeRes[1]._cmdType || null,
    widthOk: lines.every(function (line) { return line.length <= 20; }),
    heightOk: lines.length <= 8,
    playerX: resizeRes[0].playerX,
    playerY: resizeRes[0].playerY
};
`)

	if got := result["initCmdType"]; got != "tick" {
		t.Fatalf("expected benchmark init to start tick chain, got %v", got)
	}
	if got := result["keyCmdType"]; got != nil {
		t.Fatalf("expected benchmark key update to return no extra tick cmd, got %v", got)
	}
	if got := result["resizeCmdType"]; got != nil {
		t.Fatalf("expected benchmark resize update to return no extra tick cmd, got %v", got)
	}
	if got := result["widthOk"]; got != true {
		t.Fatalf("expected compact benchmark view lines to fit width, got %v", got)
	}
	if got := result["heightOk"]; got != true {
		t.Fatalf("expected compact benchmark view lines to fit height, got %v", got)
	}
	playerX, ok := result["playerX"].(int64)
	if !ok {
		t.Fatalf("expected playerX int64, got %T (%v)", result["playerX"], result["playerX"])
	}
	if playerX < 2 || playerX > 17 {
		t.Fatalf("expected playerX clamped into visible compact play area, got %d", playerX)
	}
}

func TestExample13SplitPane_InitCreatesCompositorAndStartsTick(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}
	engine := newExampleScriptEngine(t)
	loadExampleProgram(t, engine, "example-13-split-pane.js")

	result := runResultScript(t, engine, "example-13-regression", `
var initRes = __programConfig.init();
var model = initRes[0];
var cmd = initRes[1];

var viewRes = __programConfig.view(model);

__result = {
    initIsArray: Array.isArray(initRes),
    initCmdType: cmd && cmd._cmdType || null,
    modelHasTick: model.tick === 0,
    modelHasFocusIdx: model.focusIdx === 0,
    viewHasContent: typeof viewRes.content === 'string' && viewRes.content.length > 0,
    viewAltScreen: viewRes.altScreen === true
};
`)

	if got := result["initIsArray"]; got != true {
		t.Fatalf("expected init to return [state, cmd], got %v", got)
	}
	if got := result["initCmdType"]; got != "tick" {
		t.Fatalf("expected init to schedule tick, got %v", got)
	}
	if got := result["modelHasTick"]; got != true {
		t.Fatalf("expected initial model.tick === 0, got %v", got)
	}
	if got := result["modelHasFocusIdx"]; got != true {
		t.Fatalf("expected initial model.focusIdx === 0, got %v", got)
	}
	if got := result["viewHasContent"]; got != true {
		t.Fatalf("expected view to produce non-empty content, got %v", got)
	}
	if got := result["viewAltScreen"]; got != true {
		t.Fatalf("expected view altScreen=true, got %v", got)
	}
}

func TestExample13SplitPane_TabCyclesFocusAndQuitExits(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}
	engine := newExampleScriptEngine(t)
	loadExampleProgram(t, engine, "example-13-split-pane.js")

	result := runResultScript(t, engine, "example-13-regression-tab", `
var initRes = __programConfig.init();
var model = initRes[0];

var tickRes = __programConfig.update({ type: 'Tick' }, model);
model = tickRes[0];

var tabRes = __programConfig.update({ type: 'Key', key: 'tab' }, model);
var tabModel = tabRes[0];

var secondTabRes = __programConfig.update({ type: 'Key', key: 'tab' }, tabModel);

var quitRes = __programConfig.update({ type: 'Key', key: 'q' }, tabModel);

__result = {
    tickIncrements: tickRes[0].tick === 1,
    tabSwitchesFocus: tabModel.focusIdx === 1,
    secondTabSwitchesBack: secondTabRes[0].focusIdx === 0,
    quitCmdType: quitRes[1] && quitRes[1]._cmdType || null,
    tickReschedulesTick: tickRes[1] && tickRes[1]._cmdType || null
};
`)

	if got := result["tickIncrements"]; got != true {
		t.Fatalf("expected tick to increment counter, got %v", got)
	}
	if got := result["tabSwitchesFocus"]; got != true {
		t.Fatalf("expected tab to switch focus from 0 to 1, got %v", got)
	}
	if got := result["secondTabSwitchesBack"]; got != true {
		t.Fatalf("expected second tab to switch focus back to 0, got %v", got)
	}
	if got := result["quitCmdType"]; got != "quit" {
		t.Fatalf("expected q key to produce quit cmd, got %v", got)
	}
	if got := result["tickReschedulesTick"]; got != "tick" {
		t.Fatalf("expected tick to reschedule tick, got %v", got)
	}
}

func TestMinimalBubbleteaScript_InitStartsTick(t *testing.T) {
	engine := newExampleScriptEngine(t)
	loadExampleProgram(t, engine, "minimal-bubbletea-test.js")

	result := runResultScript(t, engine, "minimal-bubbletea-regression", `
var initRes = __programConfig.init();
var updateRes = __programConfig.update({ type: 'Tick' }, initRes[0]);
__result = {
    initIsArray: Array.isArray(initRes),
    initCmdType: initRes[1] && initRes[1]._cmdType || null,
    tickCount: updateRes[0].count,
    tickCmdType: updateRes[1] && updateRes[1]._cmdType || null
};
`)

	if got := result["initIsArray"]; got != true {
		t.Fatalf("expected minimal bubbletea init to return [state, cmd], got %v", got)
	}
	if got := result["initCmdType"]; got != "tick" {
		t.Fatalf("expected minimal bubbletea init to schedule tick, got %v", got)
	}
	if got := result["tickCount"]; got != int64(1) {
		t.Fatalf("expected Tick update to increment count to 1, got %v", got)
	}
	if got := result["tickCmdType"]; got != "tick" {
		t.Fatalf("expected Tick update to reschedule tick, got %v", got)
	}
}

func TestExample15BouncingLogo_InitStartsTick(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test")
	}
	engine := newExampleScriptEngine(t)
	loadExampleProgram(t, engine, "example-15-bouncing-logo.js")

	result := runResultScript(t, engine, "example-15-regression", `
var initRes = __programConfig.init();
var model = initRes[0];
var cmd = initRes[1];
var viewRes = __programConfig.view(model);

__result = {
    initIsArray: Array.isArray(initRes),
    initCmdType: cmd && cmd._cmdType || null,
    modelHasWidth: model.width === 80,
    modelHasHeight: model.height === 24,
    modelHasBounces: model.bounces === 0,
    modelHasTickCount: model.tickCount === 0,
    modelHasPaused: model.paused === false,
    viewHasContent: typeof viewRes.content === 'string' && viewRes.content.length > 0,
    viewAltScreen: viewRes.altScreen === true,
    viewMouseMode: viewRes.mouseMode === 'allMotion'
};
`)

	if got := result["initIsArray"]; got != true {
		t.Fatalf("expected init to return [state, cmd], got %v", got)
	}
	if got := result["initCmdType"]; got != "tick" {
		t.Fatalf("expected init to schedule tick, got %v", got)
	}
	if got := result["modelHasWidth"]; got != true {
		t.Fatalf("expected initial model.width === 80, got %v", got)
	}
	if got := result["modelHasHeight"]; got != true {
		t.Fatalf("expected initial model.height === 24, got %v", got)
	}
	if got := result["modelHasBounces"]; got != true {
		t.Fatalf("expected initial model.bounces === 0, got %v", got)
	}
	if got := result["modelHasTickCount"]; got != true {
		t.Fatalf("expected initial model.tickCount === 0, got %v", got)
	}
	if got := result["modelHasPaused"]; got != true {
		t.Fatalf("expected initial model.paused === false, got %v", got)
	}
	if got := result["viewHasContent"]; got != true {
		t.Fatalf("expected view to produce non-empty content, got %v", got)
	}
	if got := result["viewAltScreen"]; got != true {
		t.Fatalf("expected view altScreen=true, got %v", got)
	}
	if got := result["viewMouseMode"]; got != true {
		t.Fatalf("expected view mouseMode='allMotion', got %v", got)
	}
}
