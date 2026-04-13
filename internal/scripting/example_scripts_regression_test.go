package scripting

import (
	"bytes"
	"context"
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
	engine, err := NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("example-script", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
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

	const modelStart = "const program = tea.newModel({"
	if !strings.Contains(source, modelStart) {
		t.Fatalf("%s missing expected tea.newModel declaration", scriptName)
	}
	source = strings.Replace(source, modelStart, "const __programConfig = {", 1)

	runMarker := "tea.run(program);"
	switch scriptName {
	case "minimal-bubbletea-test.js":
		runMarker = "const result = tea.run(program);"
	case "example-02-graphical-todo.js", "benchmark-input-latency.js":
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

	script := engine.LoadScriptFromString(scriptName, source)
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
	script := engine.LoadScriptFromString(name, source)
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