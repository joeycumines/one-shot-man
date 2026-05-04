package claudemux

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// moduleTestEnv sets up a minimal JS environment with only osm:claudemux
// registered. Simpler than templateTestEnv — no event loop, no BT module.
func moduleTestEnv(t *testing.T) func(string) goja.Value {
	t.Helper()

	reg := gojarequire.NewRegistry()
	vm := goja.New()
	reg.Enable(vm)
	gojanodejsconsole.Enable(vm)

	ctx := context.Background()
	reg.RegisterNativeModule("osm:claudemux", Require(ctx))

	runJS := func(script string) goja.Value {
		t.Helper()
		res, err := vm.RunString(script)
		require.NoError(t, err, "JS execution failed for: %s", script)
		return res
	}

	// Pre-require the module.
	runJS(`var cm = require('osm:claudemux');`)

	return runJS
}

// ---------------------------------------------------------------------------
// eventToJS — via newParser().parse()
// ---------------------------------------------------------------------------

func TestBinding_EventToJS(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Parse a line that produces an event.
	runJS(`var parser = cm.newParser();`)
	runJS(`var ev = parser.parse("⏳ Waiting for rate limit...");`)

	// Verify the event object has the expected shape.
	assert.NotNil(t, runJS(`ev.type`))
	assert.NotNil(t, runJS(`ev.line`))
	assert.NotNil(t, runJS(`ev.pattern`))
	assert.NotNil(t, runJS(`ev.fields`))

	// Normal text line.
	runJS(`var textEv = parser.parse("hello world");`)
	assert.Equal(t, int64(int(EventText)), runJS(`textEv.type`).ToInteger())
	assert.Equal(t, "hello world", runJS(`textEv.line`).String())
}

// ---------------------------------------------------------------------------
// wrapParser — parse, addPattern, patterns
// ---------------------------------------------------------------------------

func TestBinding_WrapParser(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var parser = cm.newParser();`)

	// Test patterns() returns built-in patterns.
	val := runJS(`parser.patterns().length`)
	assert.True(t, val.ToInteger() > 0, "expected at least one built-in pattern")

	// Test addPattern.
	runJS(`parser.addPattern("custom", "^CUSTOM: (.+)", cm.EVENT_ERROR);`)
	newLen := runJS(`parser.patterns().length`).ToInteger()
	assert.True(t, newLen > 1, "expected more patterns after addPattern")

	// Test parse with custom pattern.
	runJS(`var customEv = parser.parse("CUSTOM: something bad");`)
	assert.Equal(t, int64(int(EventError)), runJS(`customEv.type`).ToInteger())
}

// ---------------------------------------------------------------------------
// modelMenuToJS / jsToModelMenu — via parseModelMenu + navigateToModel
// ---------------------------------------------------------------------------

func TestBinding_ModelMenuToJS(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var menu = cm.parseModelMenu([
		"❯ claude-3-5-sonnet",
		"  gpt-4o",
		"  gemini-pro"
	]);`)

	assert.Equal(t, int64(3), runJS(`menu.models.length`).ToInteger())
	assert.Equal(t, int64(0), runJS(`menu.selectedIndex`).ToInteger())
	assert.Equal(t, "claude-3-5-sonnet", runJS(`menu.models[0]`).String())
}

func TestBinding_JsToModelMenu(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// navigateToModel calls jsToModelMenu internally.
	val := runJS(`cm.navigateToModel(
		{ models: ["model-a", "model-b", "model-c"], selectedIndex: 0 },
		"model-c"
	);`)
	keys := val.String()
	assert.NotEmpty(t, keys, "expected keystroke sequence")
}

// ---------------------------------------------------------------------------
// wrapProvider — name, capabilities, _goProvider
// ---------------------------------------------------------------------------

func TestBinding_WrapProvider(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// claudeCode() factory.
	assert.Equal(t, "claude-code", runJS(`cm.claudeCode().name()`).String())

	caps := runJS(`var p = cm.claudeCode(); p.capabilities();`)
	_ = caps
	assert.True(t, runJS(`p.capabilities().mcp`).ToBoolean())
	assert.True(t, runJS(`p.capabilities().streaming`).ToBoolean())
	assert.True(t, runJS(`p.capabilities().multiTurn`).ToBoolean())

	// ollama() factory with options.
	runJS(`var op = cm.ollama({ command: "/usr/bin/ollama", extraArgs: ["--verbose"] });`)
	assert.Equal(t, "ollama", runJS(`op.name()`).String())
	assert.True(t, runJS(`op.capabilities().mcp`).ToBoolean())
}

// ---------------------------------------------------------------------------
// wrapRegistry — register, get, list
// ---------------------------------------------------------------------------

func TestBinding_WrapRegistry(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var reg = cm.newRegistry();`)
	runJS(`var cc = cm.claudeCode();`)
	runJS(`reg.register(cc);`)

	assert.Equal(t, int64(1), runJS(`reg.list().length`).ToInteger())
	assert.Equal(t, "claude-code", runJS(`reg.list()[0]`).String())
	assert.Equal(t, "claude-code", runJS(`reg.get("claude-code").name()`).String())
}

// ---------------------------------------------------------------------------
// wrapInstanceRegistry + wrapInstance — create, get, list, close
// ---------------------------------------------------------------------------

func TestBinding_WrapInstanceRegistry(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)
	dir := t.TempDir()
	jsDir := filepath.ToSlash(dir) // Windows: backslashes break JS string literals

	runJS(`var ireg = cm.newInstanceRegistry('` + jsDir + `');`)
	assert.Equal(t, int64(0), runJS(`ireg.len()`).ToInteger())
	assert.Equal(t, jsDir, runJS(`ireg.baseDir()`).String())

	// Create an instance.
	runJS(`var inst = ireg.create("sess-1");`)
	assert.Equal(t, "sess-1", runJS(`inst.id`).String())
	assert.NotEmpty(t, runJS(`inst.stateDir`).String())
	assert.NotEmpty(t, runJS(`inst.createdAt`).String())
	assert.False(t, runJS(`inst.isClosed()`).ToBoolean())

	assert.Equal(t, int64(1), runJS(`ireg.len()`).ToInteger())
	assert.Equal(t, int64(1), runJS(`ireg.list().length`).ToInteger())

	// Get the instance.
	runJS(`var inst2 = ireg.get("sess-1");`)
	assert.Equal(t, "sess-1", runJS(`inst2.id`).String())

	// Close individual instance.
	runJS(`ireg.close("sess-1");`)
	assert.Equal(t, int64(0), runJS(`ireg.len()`).ToInteger())

	// Create two and closeAll.
	runJS(`ireg.create("s1"); ireg.create("s2");`)
	assert.Equal(t, int64(2), runJS(`ireg.len()`).ToInteger())
	runJS(`ireg.closeAll();`)
	assert.Equal(t, int64(0), runJS(`ireg.len()`).ToInteger())
}

// ---------------------------------------------------------------------------
// guardConfigToJS / jsToGuardConfig — via defaultGuardConfig + newGuard
// ---------------------------------------------------------------------------

func TestBinding_GuardConfigToJS(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var gcfg = cm.defaultGuardConfig();`)

	// Verify nested structure.
	assert.True(t, runJS(`gcfg.rateLimit.enabled`).ToBoolean())
	assert.True(t, runJS(`gcfg.rateLimit.initialDelayMs > 0`).ToBoolean())
	assert.True(t, runJS(`gcfg.rateLimit.maxDelayMs > 0`).ToBoolean())
	assert.True(t, runJS(`gcfg.rateLimit.multiplier > 0`).ToBoolean())
	assert.True(t, runJS(`gcfg.permission.enabled`).ToBoolean())
	assert.True(t, runJS(`gcfg.crash.enabled`).ToBoolean())
	assert.True(t, runJS(`gcfg.crash.maxRestarts > 0`).ToBoolean())
	assert.True(t, runJS(`gcfg.outputTimeout.enabled`).ToBoolean())
	assert.True(t, runJS(`gcfg.outputTimeout.timeoutMs > 0`).ToBoolean())
}

func TestBinding_JsToGuardConfig(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// newGuard with custom config exercises jsToGuardConfig.
	runJS(`var g = cm.newGuard({
		rateLimit: { enabled: false, initialDelayMs: 100, maxDelayMs: 5000, multiplier: 1.5, resetAfterMs: 30000 },
		permission: { enabled: true, policy: cm.PERMISSION_POLICY_ALLOW },
		crash: { enabled: true, maxRestarts: 5 },
		outputTimeout: { enabled: false, timeoutMs: 60000 }
	});`)

	// Verify config roundtrip.
	assert.False(t, runJS(`g.config().rateLimit.enabled`).ToBoolean())
	assert.Equal(t, int64(100), runJS(`g.config().rateLimit.initialDelayMs`).ToInteger())
	assert.Equal(t, int64(5), runJS(`g.config().crash.maxRestarts`).ToInteger())
}

// ---------------------------------------------------------------------------
// wrapGuard — processEvent, processCrash, checkTimeout, state, config
// ---------------------------------------------------------------------------

func TestBinding_WrapGuard(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var g = cm.newGuard({
		rateLimit: { enabled: true, initialDelayMs: 1000, maxDelayMs: 30000, multiplier: 2.0, resetAfterMs: 60000 },
		permission: { enabled: true, policy: ` + itoa(int(PermissionPolicyDeny)) + ` },
		crash: { enabled: true, maxRestarts: 3 },
		outputTimeout: { enabled: false }
	});`)

	// Process a normal text event — should return null (no guard action).
	result := runJS(`g.processEvent({ type: cm.EVENT_TEXT, line: "hello" }, Date.now())`)
	assert.True(t, goja.IsNull(result), "expected null for normal text event")

	// Process a rate limit event — should return an action.
	result = runJS(`g.processEvent({ type: cm.EVENT_RATE_LIMIT, line: "rate limited", fields: { retryAfter: "5" } }, Date.now())`)
	assert.False(t, goja.IsNull(result), "expected guard event for rate limit")
	assert.Equal(t, int64(int(GuardActionPause)), runJS(`
		var rlResult = g.processEvent({ type: cm.EVENT_RATE_LIMIT, line: "rate limited" }, Date.now());
		rlResult.action
	`).ToInteger())

	// Process permission event.
	runJS(`var permResult = g.processEvent({ type: cm.EVENT_PERMISSION, line: "permission request" }, Date.now());`)
	assert.False(t, goja.IsNull(runJS(`permResult`)), "expected guard event for permission")

	// State.
	runJS(`var gstate = g.state();`)
	assert.True(t, runJS(`gstate.rateLimitCount >= 0`).ToBoolean())
	assert.True(t, runJS(`typeof gstate.started === 'boolean'`).ToBoolean())

	// processCrash.
	runJS(`var crashResult = g.processCrash(1, Date.now());`)
	assert.False(t, goja.IsNull(runJS(`crashResult`)), "expected guard event for crash")
	assert.Equal(t, int64(int(GuardActionRestart)), runJS(`crashResult.action`).ToInteger())

	// resetCrashCount.
	runJS(`g.resetCrashCount();`)
	assert.Equal(t, int64(0), runJS(`g.state().crashCount`).ToInteger())
}

// ---------------------------------------------------------------------------
// guardEventToJS — verified through wrapGuard processEvent above, additional
// coverage for details field.
// ---------------------------------------------------------------------------

func TestBinding_GuardEventToJS_Details(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var g = cm.newGuard({
		rateLimit: { enabled: true, initialDelayMs: 1000, maxDelayMs: 30000, multiplier: 2.0, resetAfterMs: 60000 },
		permission: { enabled: false },
		crash: { enabled: false },
		outputTimeout: { enabled: false }
	});`)

	runJS(`var ev = g.processEvent({ type: cm.EVENT_RATE_LIMIT, line: "⏳ Waiting...", fields: { retryAfter: "10" } }, Date.now());`)
	assert.Equal(t, "Pause", runJS(`ev.actionName`).String())
	assert.NotEmpty(t, runJS(`ev.reason`).String())
	// details should be an object.
	assert.True(t, runJS(`typeof ev.details === 'object'`).ToBoolean())
}

// ---------------------------------------------------------------------------
// mcpGuardConfigToJS / jsToMCPGuardConfig / wrapMCPGuard
// ---------------------------------------------------------------------------

func TestBinding_MCPGuardConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var mcfg = cm.defaultMCPGuardConfig();`)
	assert.True(t, runJS(`typeof mcfg.noCallTimeout === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof mcfg.frequencyLimit === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof mcfg.repeatDetection === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof mcfg.toolAllowlist === 'object'`).ToBoolean())

	runJS(`var mg = cm.newMCPGuard({
		noCallTimeout: { enabled: true, timeoutMs: 5000 },
		frequencyLimit: { enabled: true, windowMs: 10000, maxCalls: 50 },
		repeatDetection: { enabled: true, maxRepeats: 3, windowSize: 10, matchTool: true, matchArgHash: true },
		toolAllowlist: { enabled: true, allowedTools: ["read_file", "write_file"] }
	});`)

	// Config roundtrip.
	assert.True(t, runJS(`mg.config().noCallTimeout.enabled`).ToBoolean())
	assert.Equal(t, int64(5000), runJS(`mg.config().noCallTimeout.timeoutMs`).ToInteger())
	assert.Equal(t, int64(50), runJS(`mg.config().frequencyLimit.maxCalls`).ToInteger())
	assert.True(t, runJS(`mg.config().repeatDetection.matchArgHash`).ToBoolean())
	assert.Equal(t, int64(2), runJS(`mg.config().toolAllowlist.allowedTools.length`).ToInteger())
}

func TestBinding_WrapMCPGuard(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var mg = cm.newMCPGuard({
		noCallTimeout: { enabled: false },
		frequencyLimit: { enabled: true, windowMs: 10000, maxCalls: 2 },
		repeatDetection: { enabled: false },
		toolAllowlist: { enabled: false }
	});`)

	// processToolCall.
	result := runJS(`mg.processToolCall({ toolName: "read_file", arguments: "{}", timestampMs: Date.now() })`)
	assert.True(t, goja.IsNull(result), "first call should be allowed")

	runJS(`mg.processToolCall({ toolName: "write_file", arguments: "{}", timestampMs: Date.now() })`)
	// Third call should trigger frequency limit.
	result = runJS(`mg.processToolCall({ toolName: "delete_file", arguments: "{}", timestampMs: Date.now() })`)
	assert.False(t, goja.IsNull(result), "expected guard event for frequency limit")

	// State.
	runJS(`var mgs = mg.state();`)
	assert.Equal(t, int64(3), runJS(`mgs.totalCalls`).ToInteger())
	assert.True(t, runJS(`mgs.started`).ToBoolean())
}

// ---------------------------------------------------------------------------
// supervisorConfigToJS / jsToSupervisorConfig / wrapSupervisor
// ---------------------------------------------------------------------------

func TestBinding_SupervisorConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var scfg = cm.defaultSupervisorConfig();`)
	assert.True(t, runJS(`scfg.maxRetries > 0`).ToBoolean())
	assert.True(t, runJS(`scfg.retryDelayMs > 0`).ToBoolean())
	assert.True(t, runJS(`scfg.shutdownTimeoutMs > 0`).ToBoolean())

	runJS(`var sup = cm.newSupervisor({
		maxRetries: 5,
		maxForceKills: 2,
		retryDelayMs: 500,
		shutdownTimeoutMs: 3000,
		forceKillTimeoutMs: 1000
	});`)

	// Verify config via snapshot.
	runJS(`sup.start();`)
	runJS(`var snap = sup.snapshot();`)
	assert.Equal(t, int64(int(SupervisorRunning)), runJS(`snap.state`).ToInteger())
	assert.Equal(t, "Running", runJS(`snap.stateName`).String())
	assert.Equal(t, int64(0), runJS(`snap.retryCount`).ToInteger())
}

func TestBinding_WrapSupervisor(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var sup = cm.newSupervisor({ maxRetries: 2, retryDelayMs: 10, shutdownTimeoutMs: 100, forceKillTimeoutMs: 50 });`)
	runJS(`sup.start();`)

	// handleError returns recovery decision.
	runJS(`var rd = sup.handleError("crash", ` + itoa(int(ErrorClassPTYCrash)) + `);`)
	assert.NotEmpty(t, runJS(`rd.actionName`).String())
	assert.True(t, runJS(`rd.newState >= 0`).ToBoolean())
	assert.NotEmpty(t, runJS(`rd.newStateName`).String())

	// confirmRecovery.
	runJS(`sup.confirmRecovery();`)
	assert.Equal(t, int64(int(SupervisorRunning)), runJS(`sup.snapshot().state`).ToInteger())

	// shutdown.
	runJS(`var sd = sup.shutdown();`)
	assert.NotEmpty(t, runJS(`sd.actionName`).String())

	// confirmStopped.
	runJS(`sup.confirmStopped();`)
	assert.Equal(t, int64(int(SupervisorStopped)), runJS(`sup.snapshot().state`).ToInteger())

	// reset.
	runJS(`sup.reset();`)
	assert.Equal(t, int64(int(SupervisorIdle)), runJS(`sup.snapshot().state`).ToInteger())

	// cancelled.
	assert.False(t, runJS(`sup.cancelled()`).ToBoolean())
}

// ---------------------------------------------------------------------------
// recoveryDecisionToJS — verified through wrapSupervisor above.
// Additional detail checks.
// ---------------------------------------------------------------------------

func TestBinding_RecoveryDecisionToJS_Details(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var sup = cm.newSupervisor({ maxRetries: 1, retryDelayMs: 10, shutdownTimeoutMs: 100, forceKillTimeoutMs: 50 });`)
	runJS(`sup.start();`)
	runJS(`var rd = sup.handleError("oops", ` + itoa(int(ErrorClassPTYCrash)) + `);`)

	assert.True(t, runJS(`typeof rd.details === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof rd.reason === 'string'`).ToBoolean())
}

// ---------------------------------------------------------------------------
// poolConfigToJS / jsToPoolConfig / wrapPool + wrapPoolWorker
// ---------------------------------------------------------------------------

func TestBinding_PoolConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var pcfg = cm.defaultPoolConfig();`)
	assert.True(t, runJS(`pcfg.maxSize > 0`).ToBoolean())

	runJS(`var pool = cm.newPool({ maxSize: 3 });`)
	assert.Equal(t, int64(3), runJS(`pool.config().maxSize`).ToInteger())
}

func TestBinding_WrapPool(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var pool = cm.newPool({ maxSize: 3 });`)
	runJS(`pool.start();`)

	// addWorker.
	runJS(`var w1 = pool.addWorker("w1");`)
	assert.Equal(t, "w1", runJS(`w1.id`).String())
	assert.True(t, runJS(`typeof w1.state() === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof w1.stateName() === 'string'`).ToBoolean())

	runJS(`pool.addWorker("w2");`)

	// stats.
	runJS(`var ps = pool.stats();`)
	assert.Equal(t, int64(2), runJS(`ps.workerCount`).ToInteger())
	assert.Equal(t, int64(3), runJS(`ps.maxSize`).ToInteger())
	assert.True(t, runJS(`ps.workers.length === 2`).ToBoolean())

	// Acquire and release cycle.
	runJS(`var acquired = pool.tryAcquire();`)
	assert.False(t, goja.IsNull(runJS(`acquired`)), "expected to acquire a worker")
	runJS(`pool.release(acquired);`)

	// removeWorker.
	runJS(`pool.removeWorker("w1");`)
	assert.Equal(t, int64(1), runJS(`pool.stats().workerCount`).ToInteger())

	// drain + waitDrained.
	runJS(`pool.drain();`)
	runJS(`pool.waitDrained();`)

	// close.
	runJS(`var closed = pool.close();`)
	assert.True(t, runJS(`Array.isArray(closed)`).ToBoolean())
}

// ---------------------------------------------------------------------------
// workerStatsToJS / poolStatsToJS — verified through wrapPool stats() above.
// ---------------------------------------------------------------------------

func TestBinding_PoolStatsToJS(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var pool = cm.newPool({ maxSize: 2 });`)
	runJS(`pool.start();`)
	runJS(`pool.addWorker("a");`)
	runJS(`var st = pool.stats();`)

	assert.True(t, runJS(`typeof st.state === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`st.stateName`).String())
	assert.Equal(t, int64(1), runJS(`st.workerCount`).ToInteger())
	assert.Equal(t, int64(0), runJS(`st.inflight`).ToInteger())

	// Worker stats.
	assert.Equal(t, "a", runJS(`st.workers[0].id`).String())
	assert.True(t, runJS(`typeof st.workers[0].state === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof st.workers[0].taskCount === 'number'`).ToBoolean())

	runJS(`pool.close();`)
}

// ---------------------------------------------------------------------------
// panelConfigToJS / jsToPanelConfig / wrapPanel
// ---------------------------------------------------------------------------

func TestBinding_PanelConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var pcfg = cm.defaultPanelConfig();`)
	assert.True(t, runJS(`pcfg.maxPanes > 0`).ToBoolean())
	assert.True(t, runJS(`pcfg.scrollbackSize > 0`).ToBoolean())

	runJS(`var panel = cm.newPanel({ maxPanes: 5, scrollbackSize: 100 });`)
	assert.Equal(t, int64(5), runJS(`panel.config().maxPanes`).ToInteger())
	assert.Equal(t, int64(100), runJS(`panel.config().scrollbackSize`).ToInteger())
}

func TestBinding_WrapPanel(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var panel = cm.newPanel({ maxPanes: 3, scrollbackSize: 100 });`)
	runJS(`panel.start();`)

	// addPane.
	assert.Equal(t, int64(0), runJS(`panel.addPane("pane-1", "Claude #1")`).ToInteger())
	assert.Equal(t, int64(1), runJS(`panel.addPane("pane-2", "Claude #2")`).ToInteger())
	assert.Equal(t, int64(2), runJS(`panel.paneCount()`).ToInteger())

	// activeIndex + setActive.
	assert.Equal(t, int64(0), runJS(`panel.activeIndex()`).ToInteger())
	runJS(`panel.setActive(1);`)
	assert.Equal(t, int64(1), runJS(`panel.activeIndex()`).ToInteger())

	// activePane.
	runJS(`var ap = panel.activePane();`)
	assert.Equal(t, "pane-2", runJS(`ap.id`).String())
	assert.Equal(t, "Claude #2", runJS(`ap.title`).String())

	// appendOutput.
	runJS(`panel.appendOutput("pane-1", "hello world");`)
	runJS(`panel.appendOutput("pane-1", "second line");`)

	// getVisibleLines.
	runJS(`var lines = panel.getVisibleLines("pane-1", 10);`)
	assert.Equal(t, int64(2), runJS(`lines.length`).ToInteger())
	assert.Equal(t, "hello world", runJS(`lines[0]`).String())

	// updateHealth.
	runJS(`panel.updateHealth("pane-1", { state: "running", errorCount: 0, taskCount: 5 });`)

	// statusBar.
	assert.NotEmpty(t, runJS(`panel.statusBar()`).String())

	// routeInput — exercises inputResultToJS.
	runJS(`var ir = panel.routeInput("x");`)
	assert.True(t, runJS(`typeof ir.consumed === 'boolean'`).ToBoolean())
	assert.NotEmpty(t, runJS(`ir.action`).String())

	// removePane.
	runJS(`panel.removePane("pane-2");`)
	assert.Equal(t, int64(1), runJS(`panel.paneCount()`).ToInteger())

	// snapshot — exercises panelSnapshotToJS + paneSnapshotToJS.
	runJS(`var snap = panel.snapshot();`)
	assert.True(t, runJS(`typeof snap.state === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`snap.stateName`).String())
	assert.True(t, runJS(`snap.panes.length > 0`).ToBoolean())

	pane := runJS(`snap.panes[0]`)
	_ = pane
	assert.Equal(t, "pane-1", runJS(`snap.panes[0].id`).String())
	assert.True(t, runJS(`typeof snap.panes[0].lines === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof snap.panes[0].health === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof snap.panes[0].isActive === 'boolean'`).ToBoolean())

	// close.
	runJS(`panel.close();`)
}

// ---------------------------------------------------------------------------
// managedSessionConfigToJS / jsToManagedSessionConfig / wrapManagedSession
// ---------------------------------------------------------------------------

func TestBinding_ManagedSessionConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var mscfg = cm.defaultManagedSessionConfig();`)
	assert.True(t, runJS(`typeof mscfg.guard === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof mscfg.mcpGuard === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof mscfg.supervisor === 'object'`).ToBoolean())
}

func TestBinding_WrapManagedSession(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var sess = cm.createSession("test-sess-1", {
		guard: {
			rateLimit: { enabled: true, initialDelayMs: 500, maxDelayMs: 5000, multiplier: 2.0, resetAfterMs: 30000 },
			permission: { enabled: false },
			crash: { enabled: true, maxRestarts: 2 },
			outputTimeout: { enabled: false }
		},
		mcpGuard: {
			noCallTimeout: { enabled: false },
			frequencyLimit: { enabled: false },
			repeatDetection: { enabled: false },
			toolAllowlist: { enabled: false }
		},
		supervisor: { maxRetries: 2, retryDelayMs: 10, shutdownTimeoutMs: 100, forceKillTimeoutMs: 50 }
	});`)

	assert.Equal(t, "test-sess-1", runJS(`sess.id`).String())

	// start.
	runJS(`sess.start();`)
	assert.Equal(t, int64(int(SessionActive)), runJS(`sess.state()`).ToInteger())

	// processLine — exercises lineResultToJS.
	runJS(`var lr = sess.processLine("hello world", Date.now());`)
	assert.True(t, runJS(`typeof lr.event === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof lr.action === 'string'`).ToBoolean())
	assert.Equal(t, "none", runJS(`lr.action`).String())

	// processLine with rate limit.
	runJS(`var lr2 = sess.processLine("⏳ Waiting for rate limit...", Date.now());`)
	assert.NotEmpty(t, runJS(`lr2.action`).String())

	// processToolCall — exercises toolCallResultToJS.
	runJS(`var tcr = sess.processToolCall({ toolName: "read_file", arguments: "{}" });`)
	assert.True(t, runJS(`typeof tcr.action === 'string'`).ToBoolean())

	// checkTimeout.
	result := runJS(`sess.checkTimeout(Date.now())`)
	_ = result // May be null.

	// handleError — exercises recoveryDecisionToJS.
	runJS(`var hrd = sess.handleError("crash", ` + itoa(int(ErrorClassPTYCrash)) + `);`)
	assert.NotEmpty(t, runJS(`hrd.actionName`).String())

	// confirmRecovery.
	runJS(`sess.confirmRecovery();`)

	// resume (from paused state, but session may be in a different state — just test the binding).
	// This might error if not in paused state; that's fine, we're testing the binding exists.

	// parser — exercises wrapParser on session's parser.
	runJS(`var sp = sess.parser();`)
	assert.True(t, runJS(`sp.patterns().length > 0`).ToBoolean())

	// snapshot — exercises managedSessionSnapshotToJS.
	runJS(`var ssnap = sess.snapshot();`)
	assert.Equal(t, "test-sess-1", runJS(`ssnap.id`).String())
	assert.True(t, runJS(`ssnap.linesProcessed > 0`).ToBoolean())
	assert.True(t, runJS(`typeof ssnap.eventCounts === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof ssnap.guardState === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof ssnap.mcpGuardState === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof ssnap.supervisorState === 'object'`).ToBoolean())

	// shutdown.
	runJS(`var ssd = sess.shutdown();`)
	assert.NotEmpty(t, runJS(`ssd.actionName`).String())

	// close.
	runJS(`sess.close();`)
	assert.Equal(t, int64(int(SessionClosed)), runJS(`sess.state()`).ToInteger())
}

// ---------------------------------------------------------------------------
// ManagedSession event callbacks — onEvent, onGuardAction, onRecoveryDecision
// ---------------------------------------------------------------------------

func TestBinding_ManagedSession_Callbacks(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var sess = cm.createSession("cb-sess", {
		guard: {
			rateLimit: { enabled: true, initialDelayMs: 500, maxDelayMs: 5000, multiplier: 2.0, resetAfterMs: 30000 },
			permission: { enabled: false },
			crash: { enabled: true, maxRestarts: 2 },
			outputTimeout: { enabled: false }
		}
	});`)
	runJS(`sess.start();`)

	// onEvent callback.
	runJS(`var eventLog = [];`)
	runJS(`sess.onEvent(function(ev) { eventLog.push(ev.type); });`)
	runJS(`sess.processLine("hello", Date.now());`)
	assert.Equal(t, int64(1), runJS(`eventLog.length`).ToInteger())

	// onGuardAction callback.
	runJS(`var guardLog = [];`)
	runJS(`sess.onGuardAction(function(ge) { guardLog.push(ge.actionName); });`)
	runJS(`sess.processLine("⏳ Waiting for rate limit...", Date.now());`)
	assert.True(t, runJS(`guardLog.length > 0`).ToBoolean())

	// onRecoveryDecision callback.
	runJS(`var recoveryLog = [];`)
	runJS(`sess.onRecoveryDecision(function(rd) { recoveryLog.push(rd.actionName); });`)
	runJS(`sess.processCrash(1, Date.now());`)
	assert.True(t, runJS(`recoveryLog.length > 0`).ToBoolean())

	// Clear callbacks by passing null.
	runJS(`sess.onEvent(null);`)
	runJS(`sess.onGuardAction(null);`)
	runJS(`sess.onRecoveryDecision(null);`)

	runJS(`sess.close();`)
}

// ---------------------------------------------------------------------------
// safetyConfigToJS / jsToSafetyConfig / wrapSafetyValidator
// ---------------------------------------------------------------------------

func TestBinding_SafetyConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var scfg = cm.defaultSafetyConfig();`)
	assert.True(t, runJS(`typeof scfg.enabled === 'boolean'`).ToBoolean())
	assert.True(t, runJS(`typeof scfg.warnThreshold === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof scfg.confirmThreshold === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof scfg.blockThreshold === 'number'`).ToBoolean())
	assert.True(t, runJS(`Array.isArray(scfg.blockedTools)`).ToBoolean())
	assert.True(t, runJS(`Array.isArray(scfg.blockedPaths)`).ToBoolean())
	assert.True(t, runJS(`Array.isArray(scfg.allowedPaths)`).ToBoolean())
	assert.True(t, runJS(`Array.isArray(scfg.sensitivePatterns)`).ToBoolean())

	runJS(`var sv = cm.newSafetyValidator({
		enabled: true,
		defaultAction: ` + itoa(int(PolicyWarn)) + `,
		warnThreshold: 0.2,
		confirmThreshold: 0.5,
		blockThreshold: 0.8,
		blockedTools: ["rm", "kubectl"],
		blockedPaths: ["/etc", "/root"],
		allowedPaths: ["/tmp"],
		sensitivePatterns: ["password", "secret"]
	});`)

	// Config roundtrip.
	assert.True(t, runJS(`sv.config().enabled`).ToBoolean())
	assert.Equal(t, 0.2, runJS(`sv.config().warnThreshold`).ToFloat())
	assert.Equal(t, int64(2), runJS(`sv.config().blockedTools.length`).ToInteger())
	assert.Equal(t, int64(2), runJS(`sv.config().sensitivePatterns.length`).ToInteger())
}

func TestBinding_WrapSafetyValidator(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var sv = cm.newSafetyValidator({
		enabled: true,
		blockedTools: ["rm"],
		blockedPaths: ["/etc"],
		allowedPaths: ["/tmp"]
	});`)

	// validate — exercises jsToSafetyAction + safetyAssessmentToJS.
	runJS(`var assessment = sv.validate({
		type: "command",
		name: "rm",
		raw: "rm -rf /important",
		args: { path: "/important" },
		filePaths: ["/important/file.txt"]
	});`)

	assert.True(t, runJS(`typeof assessment.intent === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`assessment.intentName`).String())
	assert.True(t, runJS(`typeof assessment.scope === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`assessment.scopeName`).String())
	assert.True(t, runJS(`typeof assessment.riskScore === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof assessment.riskLevel === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`assessment.riskLevelName`).String())
	assert.True(t, runJS(`typeof assessment.action === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`assessment.actionName`).String())
	assert.True(t, runJS(`typeof assessment.details === 'object'`).ToBoolean())

	// stats — exercises safetyStatsToJS.
	runJS(`var stats = sv.stats();`)
	assert.True(t, runJS(`stats.totalChecks > 0`).ToBoolean())
	assert.True(t, runJS(`typeof stats.allowCount === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof stats.intentCounts === 'object'`).ToBoolean())
	assert.True(t, runJS(`typeof stats.scopeCounts === 'object'`).ToBoolean())
}

// ---------------------------------------------------------------------------
// wrapCompositeValidator
// ---------------------------------------------------------------------------

func TestBinding_WrapCompositeValidator(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var sv1 = cm.newSafetyValidator({ enabled: true, blockedTools: ["rm"] });`)
	runJS(`var sv2 = cm.newSafetyValidator({ enabled: true, blockedPaths: ["/etc"] });`)
	runJS(`var cv = cm.newCompositeValidator([sv1, sv2]);`)

	runJS(`var result = cv.validate({
		type: "command",
		name: "ls",
		raw: "ls /tmp"
	});`)

	assert.True(t, runJS(`typeof result.action === 'number'`).ToBoolean())
	assert.NotEmpty(t, runJS(`result.actionName`).String())
}

// ---------------------------------------------------------------------------
// choiceConfigToJS / jsToChoiceConfig / wrapChoiceResolver
// ---------------------------------------------------------------------------

func TestBinding_ChoiceConfigRoundtrip(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var ccfg = cm.defaultChoiceConfig();`)
	assert.True(t, runJS(`typeof ccfg.confirmThreshold === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof ccfg.minCandidates === 'number'`).ToBoolean())
	assert.True(t, runJS(`Array.isArray(ccfg.defaultCriteria)`).ToBoolean())
	assert.True(t, runJS(`ccfg.defaultCriteria.length > 0`).ToBoolean())

	// Verify criteria shape.
	assert.NotEmpty(t, runJS(`ccfg.defaultCriteria[0].name`).String())
	assert.True(t, runJS(`typeof ccfg.defaultCriteria[0].weight === 'number'`).ToBoolean())
}

func TestBinding_WrapChoiceResolver(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var cr = cm.newChoiceResolver({
		confirmThreshold: 0.5,
		minCandidates: 2,
		defaultCriteria: [
			{ name: "risk", weight: 0.6, description: "risk level" },
			{ name: "speed", weight: 0.4, description: "execution speed" }
		]
	});`)

	// analyze with default scorer (attribute-based).
	runJS(`var result = cr.analyze([
		{ id: "a", name: "Option A", description: "first option", attributes: { risk: "0.8", speed: "0.6" } },
		{ id: "b", name: "Option B", description: "second option", attributes: { risk: "0.3", speed: "0.9" } }
	]);`)

	assert.NotEmpty(t, runJS(`result.recommendedID`).String())
	assert.True(t, runJS(`typeof result.needsConfirm === 'boolean'`).ToBoolean())
	assert.True(t, runJS(`result.rankings.length === 2`).ToBoolean())

	// Rankings shape.
	assert.NotEmpty(t, runJS(`result.rankings[0].candidateID`).String())
	assert.NotEmpty(t, runJS(`result.rankings[0].name`).String())
	assert.True(t, runJS(`typeof result.rankings[0].totalScore === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof result.rankings[0].rank === 'number'`).ToBoolean())
	assert.True(t, runJS(`typeof result.rankings[0].scores === 'object'`).ToBoolean())

	// stats — exercises choiceStatsToJS.
	runJS(`var cs = cr.stats();`)
	assert.Equal(t, int64(1), runJS(`cs.totalAnalyses`).ToInteger())
	assert.Equal(t, int64(2), runJS(`cs.totalCandidates`).ToInteger())

	// config roundtrip.
	assert.Equal(t, 0.5, runJS(`cr.config().confirmThreshold`).ToFloat())
	assert.Equal(t, int64(2), runJS(`cr.config().defaultCriteria.length`).ToInteger())
}

func TestBinding_ChoiceResolver_CustomScoreFn(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var cr = cm.newChoiceResolver({ confirmThreshold: 0.5, minCandidates: 2 });`)

	// analyze with custom JS score function.
	runJS(`var result = cr.analyze(
		[
			{ id: "x", name: "X", description: "desc x" },
			{ id: "y", name: "Y", description: "desc y" }
		],
		[
			{ name: "quality", weight: 1.0, description: "quality metric" }
		],
		function(candidate, criterion) {
			if (candidate.id === "x") return 0.9;
			return 0.4;
		}
	);`)

	assert.Equal(t, "x", runJS(`result.recommendedID`).String())
}

// ---------------------------------------------------------------------------
// Name helper function constants — exercise the JS-exposed name functions.
// ---------------------------------------------------------------------------

func TestBinding_NameHelpers(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Event type name.
	assert.Equal(t, "Text", runJS(`cm.eventTypeName(cm.EVENT_TEXT)`).String())
	assert.Equal(t, "RateLimit", runJS(`cm.eventTypeName(cm.EVENT_RATE_LIMIT)`).String())

	// Guard action name.
	assert.Equal(t, "None", runJS(`cm.guardActionName(cm.GUARD_ACTION_NONE)`).String())
	assert.Equal(t, "Escalate", runJS(`cm.guardActionName(cm.GUARD_ACTION_ESCALATE)`).String())

	// Supervisor state name.
	assert.Equal(t, "Idle", runJS(`cm.supervisorStateName(cm.SUPERVISOR_IDLE)`).String())
	assert.Equal(t, "Running", runJS(`cm.supervisorStateName(cm.SUPERVISOR_RUNNING)`).String())

	// Error class name.
	assert.Equal(t, "None", runJS(`cm.errorClassName(cm.ERROR_CLASS_NONE)`).String())
	assert.Equal(t, "PTY-EOF", runJS(`cm.errorClassName(cm.ERROR_CLASS_PTY_EOF)`).String())

	// Recovery action name.
	assert.Equal(t, "None", runJS(`cm.recoveryActionName(cm.RECOVERY_NONE)`).String())
	assert.Equal(t, "Retry", runJS(`cm.recoveryActionName(cm.RECOVERY_RETRY)`).String())

	// Managed session state name.
	assert.Equal(t, "Idle", runJS(`cm.managedSessionStateName(cm.SESSION_IDLE)`).String())
	assert.Equal(t, "Active", runJS(`cm.managedSessionStateName(cm.SESSION_ACTIVE)`).String())

	// Intent name.
	assert.Equal(t, "Unknown", runJS(`cm.intentName(cm.INTENT_UNKNOWN)`).String())
	assert.Equal(t, "Destructive", runJS(`cm.intentName(cm.INTENT_DESTRUCTIVE)`).String())

	// Scope name.
	assert.Equal(t, "Unknown", runJS(`cm.scopeName(cm.SCOPE_UNKNOWN)`).String())
	assert.Equal(t, "Infra", runJS(`cm.scopeName(cm.SCOPE_INFRA)`).String())

	// Risk level name.
	assert.Equal(t, "None", runJS(`cm.riskLevelName(cm.RISK_NONE)`).String())
	assert.Equal(t, "Critical", runJS(`cm.riskLevelName(cm.RISK_CRITICAL)`).String())

	// Policy action name.
	assert.Equal(t, "Allow", runJS(`cm.policyActionName(cm.POLICY_ALLOW)`).String())
	assert.Equal(t, "Block", runJS(`cm.policyActionName(cm.POLICY_BLOCK)`).String())
}

// ---------------------------------------------------------------------------
// parseSpawnOpts — via registry.spawn (but requires a real provider with PTY).
// Instead, test the factory options parsing for claudeCode and ollama.
// ---------------------------------------------------------------------------

func TestBinding_ClaudeCodeFactory_WithOptions(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var p = cm.claudeCode({ command: "/custom/claude" });`)
	assert.Equal(t, "claude-code", runJS(`p.name()`).String())
}

func TestBinding_OllamaFactory_AllOptions(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	runJS(`var p = cm.ollama({ command: "/custom/ollama", extraArgs: ["--verbose", "--debug"] });`)
	assert.Equal(t, "ollama", runJS(`p.name()`).String())
}

// ---------------------------------------------------------------------------
// Keystroke constants.
// ---------------------------------------------------------------------------

func TestBinding_KeystrokeConstants(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	assert.NotEmpty(t, runJS(`cm.KEY_ARROW_UP`).String())
	assert.NotEmpty(t, runJS(`cm.KEY_ARROW_DOWN`).String())
	assert.NotEmpty(t, runJS(`cm.KEY_ENTER`).String())
}

// ---------------------------------------------------------------------------
// Split protocol factory bindings.
// ---------------------------------------------------------------------------

func TestBinding_NewClassificationRequest(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid request — full round-trip.
	runJS(`var req = cm.newClassificationRequest({
		sessionId: "sess-1",
		files: { "a.go": "M", "b.go": "A" },
		context: { modulePath: "github.com/x/y", language: "go", baseRef: "main" },
		maxGroups: 5
	});`)
	assert.Equal(t, "sess-1", runJS(`req.sessionId`).String())
	assert.Equal(t, "M", runJS(`req.files["a.go"]`).String())
	assert.Equal(t, "A", runJS(`req.files["b.go"]`).String())
	assert.Equal(t, "github.com/x/y", runJS(`req.context.modulePath`).String())
	assert.Equal(t, "go", runJS(`req.context.language`).String())
	assert.Equal(t, "main", runJS(`req.context.baseRef`).String())
	assert.Equal(t, int64(5), runJS(`req.maxGroups`).ToInteger())

	// Validation error — empty sessionId.
	val := runJS(`(function(){ try { cm.newClassificationRequest({ sessionId: "", files: {"a.go":"M"} }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "sessionId is required")

	// Validation error — empty files.
	val = runJS(`(function(){ try { cm.newClassificationRequest({ sessionId: "s1", files: {} }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "files must not be empty")
}

func TestBinding_NewClassificationResponse(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid response — full fields.
	runJS(`var resp = cm.newClassificationResponse({
		files: { "a.go": "core", "b.go": "tests" },
		confidence: { "a.go": 0.9, "b.go": 0.8 },
		groupNames: ["core", "tests"],
		independentPairs: [["core", "tests"]],
		rationale: { "core": "main logic", "tests": "test files" }
	});`)
	assert.Equal(t, "core", runJS(`resp.files["a.go"]`).String())
	assert.Equal(t, "tests", runJS(`resp.files["b.go"]`).String())
	assert.InDelta(t, 0.9, runJS(`resp.confidence["a.go"]`).ToFloat(), 0.001)
	assert.Equal(t, int64(2), runJS(`resp.groupNames.length`).ToInteger())
	assert.Equal(t, int64(1), runJS(`resp.independentPairs.length`).ToInteger())
	assert.Equal(t, "core", runJS(`resp.independentPairs[0][0]`).String())
	assert.Equal(t, "tests", runJS(`resp.independentPairs[0][1]`).String())
	assert.Equal(t, "main logic", runJS(`resp.rationale["core"]`).String())

	// Validation error — empty files.
	val := runJS(`(function(){ try { cm.newClassificationResponse({ files: {} }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "files must not be empty")

	// Validation error — confidence out of range.
	val = runJS(`(function(){ try { cm.newClassificationResponse({ files: {"a.go":"core"}, confidence: {"a.go": 1.5} }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "out of range")
}

func TestBinding_NewSplitPlanProposal(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid proposal.
	runJS(`var prop = cm.newSplitPlanProposal({
		sessionId: "sess-2",
		stages: [
			{ name: "core", files: ["a.go", "b.go"], message: "core changes", order: 1, rationale: "core logic", independent: true, estConflicts: 0 },
			{ name: "tests", files: ["c_test.go"], message: "test changes", order: 2 }
		]
	});`)
	assert.Equal(t, "sess-2", runJS(`prop.sessionId`).String())
	assert.Equal(t, int64(2), runJS(`prop.stages.length`).ToInteger())
	assert.Equal(t, "core", runJS(`prop.stages[0].name`).String())
	assert.Equal(t, int64(2), runJS(`prop.stages[0].files.length`).ToInteger())
	assert.Equal(t, "a.go", runJS(`prop.stages[0].files[0]`).String())
	assert.Equal(t, "core changes", runJS(`prop.stages[0].message`).String())
	assert.Equal(t, int64(1), runJS(`prop.stages[0].order`).ToInteger())
	assert.Equal(t, "core logic", runJS(`prop.stages[0].rationale`).String())
	assert.Equal(t, true, runJS(`prop.stages[0].independent`).ToBoolean())
	assert.Equal(t, int64(0), runJS(`prop.stages[0].estConflicts`).ToInteger())

	// Validation error — empty stages.
	val := runJS(`(function(){ try { cm.newSplitPlanProposal({ sessionId: "s", stages: [] }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "stages must not be empty")

	// Validation error — duplicate files across stages.
	val = runJS(`(function(){ try { cm.newSplitPlanProposal({
		sessionId: "s",
		stages: [
			{ name: "a", files: ["x.go"], order: 1 },
			{ name: "b", files: ["x.go"], order: 2 }
		]
	}); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "duplicate file")
}

func TestBinding_NewConflictReport(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid report.
	runJS(`var report = cm.newConflictReport({
		sessionId: "sess-3",
		branchName: "split/core",
		verifyOutput: "FAIL: something",
		exitCode: 1,
		files: ["a.go", "b.go"],
		goModContent: "module example.com"
	});`)
	assert.Equal(t, "sess-3", runJS(`report.sessionId`).String())
	assert.Equal(t, "split/core", runJS(`report.branchName`).String())
	assert.Equal(t, "FAIL: something", runJS(`report.verifyOutput`).String())
	assert.Equal(t, int64(1), runJS(`report.exitCode`).ToInteger())
	assert.Equal(t, int64(2), runJS(`report.files.length`).ToInteger())
	assert.Equal(t, "module example.com", runJS(`report.goModContent`).String())

	// Validation error — empty branchName.
	val := runJS(`(function(){ try { cm.newConflictReport({ sessionId: "s", branchName: "", verifyOutput: "x", exitCode: 1, files: ["a.go"] }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "branchName is required")

	// Validation error — empty files.
	val = runJS(`(function(){ try { cm.newConflictReport({ sessionId: "s", branchName: "b", verifyOutput: "x", exitCode: 1, files: [] }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "files must not be empty")
}

func TestBinding_NewConflictResolution(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid resolution with patches.
	runJS(`var res = cm.newConflictResolution({
		sessionId: "sess-4",
		branchName: "split/core",
		patches: [{ file: "go.mod", content: "module fixed" }],
		commands: ["go mod tidy"],
		reSplitSuggested: false,
		reSplitReason: ""
	});`)
	assert.Equal(t, "sess-4", runJS(`res.sessionId`).String())
	assert.Equal(t, "split/core", runJS(`res.branchName`).String())
	assert.Equal(t, int64(1), runJS(`res.patches.length`).ToInteger())
	assert.Equal(t, "go.mod", runJS(`res.patches[0].file`).String())
	assert.Equal(t, "module fixed", runJS(`res.patches[0].content`).String())
	assert.Equal(t, int64(1), runJS(`res.commands.length`).ToInteger())
	assert.Equal(t, "go mod tidy", runJS(`res.commands[0]`).String())

	// Valid resolution with re-split suggestion only.
	runJS(`var res2 = cm.newConflictResolution({
		sessionId: "sess-5",
		branchName: "split/core",
		reSplitSuggested: true,
		reSplitReason: "too many conflicts"
	});`)
	assert.Equal(t, true, runJS(`res2.reSplitSuggested`).ToBoolean())
	assert.Equal(t, "too many conflicts", runJS(`res2.reSplitReason`).String())

	// Validation error — no patches, commands, or re-split.
	val := runJS(`(function(){ try { cm.newConflictResolution({ sessionId: "s", branchName: "b" }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "resolution must include")
}

func TestBinding_NewSteeringInstruction(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid instruction.
	runJS(`var inst = cm.newSteeringInstruction({
		sessionId: "sess-6",
		type: cm.STEERING_ABORT,
		payload: { reason: "user cancelled" }
	});`)
	assert.Equal(t, "sess-6", runJS(`inst.sessionId`).String())
	assert.Equal(t, "abort", runJS(`inst.type`).String())

	// All steering type constants.
	assert.Equal(t, "abort", runJS(`cm.STEERING_ABORT`).String())
	assert.Equal(t, "modify-plan", runJS(`cm.STEERING_MODIFY_PLAN`).String())
	assert.Equal(t, "re-classify", runJS(`cm.STEERING_RE_CLASSIFY`).String())
	assert.Equal(t, "focus", runJS(`cm.STEERING_FOCUS`).String())

	// Validation error — unknown type.
	val := runJS(`(function(){ try { cm.newSteeringInstruction({ sessionId: "s", type: "invalid" }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "unknown steering type")
}

func TestBinding_NewInstructionAck(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid ack.
	runJS(`var ack = cm.newInstructionAck({
		sessionId: "sess-7",
		instructionType: cm.STEERING_FOCUS,
		status: cm.ACK_COMPLETED,
		message: "done"
	});`)
	assert.Equal(t, "sess-7", runJS(`ack.sessionId`).String())
	assert.Equal(t, "focus", runJS(`ack.instructionType`).String())
	assert.Equal(t, "completed", runJS(`ack.status`).String())
	assert.Equal(t, "done", runJS(`ack.message`).String())

	// All ack status constants.
	assert.Equal(t, "received", runJS(`cm.ACK_RECEIVED`).String())
	assert.Equal(t, "executing", runJS(`cm.ACK_EXECUTING`).String())
	assert.Equal(t, "completed", runJS(`cm.ACK_COMPLETED`).String())
	assert.Equal(t, "rejected", runJS(`cm.ACK_REJECTED`).String())

	// Validation error — unknown status.
	val := runJS(`(function(){ try { cm.newInstructionAck({ sessionId: "s", instructionType: "abort", status: "bogus" }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "unknown ack status")
}

func TestBinding_NewSplitPlanRequest(t *testing.T) {
	t.Parallel()
	runJS := moduleTestEnv(t)

	// Valid request.
	runJS(`var planReq = cm.newSplitPlanRequest({
		sessionId: "sess-8",
		classification: { "a.go": "core", "b_test.go": "tests" },
		constraints: { maxFilesPerSplit: 10, branchPrefix: "split/", preferIndependent: true }
	});`)
	assert.Equal(t, "sess-8", runJS(`planReq.sessionId`).String())
	assert.Equal(t, "core", runJS(`planReq.classification["a.go"]`).String())
	assert.Equal(t, int64(10), runJS(`planReq.constraints.maxFilesPerSplit`).ToInteger())
	assert.Equal(t, "split/", runJS(`planReq.constraints.branchPrefix`).String())
	assert.Equal(t, true, runJS(`planReq.constraints.preferIndependent`).ToBoolean())

	// Validation error — empty classification.
	val := runJS(`(function(){ try { cm.newSplitPlanRequest({ sessionId: "s", classification: {} }); return ""; } catch(e) { return e.message || String(e); } })()`)
	assert.Contains(t, val.String(), "classification must not be empty")
}

// itoa is a small helper for integer-to-string conversion in test scripts.
func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
