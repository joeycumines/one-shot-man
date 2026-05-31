package claudemux

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	gojanodejsconsole "github.com/dop251/goja_nodejs/console"
	gojarequire "github.com/dop251/goja_nodejs/require"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux/testutil"
)

func jsIntegrationEnv(t *testing.T) func(string) goja.Value {
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

	runJS(`var cm = require('osm:claudemux');`)

	return runJS
}

func TestJS_MockClaudeProvider(t *testing.T) {
	testutil.SkipSlow(t)
	runJS := jsIntegrationEnv(t)

	runJS(`var reg = cm.newRegistry();`)
	runJS(`var prov = cm.mockClaude({ processingMs: 50 });`)
	runJS(`reg.register(prov);`)
	assert.Equal(t, "mock-claude", runJS(`prov.name()`).String())

	runJS(`var handle = reg.spawn("mock-claude", { mode: cm.MODE_PROTOCOL });`)
	runJS(`handle.waitReady(10000);`)
	runJS(`handle.send(JSON.stringify({ type: "user", content: "hello" }));`)

	require.Eventually(t, func() bool {
		data := runJS(`handle.receive()`).String()
		return strings.Contains(data, "Response to: hello")
	}, 10*time.Second, 50*time.Millisecond, "expected response containing 'Response to: hello'")

	runJS(`handle.close();`)
}

func TestJS_ReliablePrompter(t *testing.T) {
	testutil.SkipSlow(t)
	runJS := jsIntegrationEnv(t)

	runJS(`var reg = cm.newRegistry();`)
	runJS(`var prov = cm.mockClaude({ processingMs: 50 });`)
	runJS(`reg.register(prov);`)
	runJS(`var handle = reg.spawn("mock-claude", { mode: cm.MODE_PROTOCOL });`)

	runJS(`var rp = cm.newReliablePrompter(handle, prov, {
		readyTimeout: 10000,
		acceptTimeout: 5000,
		responseTimeout: 15000,
		maxRetries: 3
	});`)

	runJS(`var result = rp.sendPrompt("hello");`)

	assert.True(t, strings.Contains(runJS(`result.responseText`).String(), "hello"),
		"responseText should contain 'hello'")

	transitions := runJS(`result.stateTransitions.length`).ToInteger()
	assert.True(t, transitions > 0, "expected state transitions")

	runJS(`rp.close();`)
}

func TestJS_TUIStateMachine(t *testing.T) {
	testutil.SkipSlow(t)
	runJS := jsIntegrationEnv(t)

	runJS(`var sm = cm.newTUIStateMachine();`)
	assert.Equal(t, int64(int(StateInitializing)), runJS(`sm.state()`).ToInteger())

	runJS(`var update = sm.processOutput("❯ ");`)
	assert.Equal(t, int64(int(StateReady)), runJS(`update.state`).ToInteger())
	assert.Equal(t, int64(int(StateReady)), runJS(`sm.state()`).ToInteger())

	runJS(`sm.processOutput("· thinking...");`)
	assert.Equal(t, int64(int(StateProcessing)), runJS(`sm.state()`).ToInteger())

	runJS(`sm.processOutput("❯ ");`)
	assert.Equal(t, int64(int(StateReady)), runJS(`sm.state()`).ToInteger())
}

func TestJS_VTStateDetector(t *testing.T) {
	testutil.SkipSlow(t)
	runJS := jsIntegrationEnv(t)

	runJS(`var det = cm.newVTStateDetector();`)
	assert.Equal(t, int64(int(StateInitializing)), runJS(`det.state()`).ToInteger())

	runJS(`det.processRaw("MockAgent ready.\r\n");`)
	runJS(`var update = det.processRaw("❯ ");`)
	assert.Equal(t, int64(int(StateReady)), runJS(`update.state`).ToInteger())
	assert.Equal(t, int64(int(StateReady)), runJS(`det.state()`).ToInteger())

	runJS(`det.processRaw("· thinking...\r\n");`)
	assert.Equal(t, int64(int(StateProcessing)), runJS(`det.state()`).ToInteger())

	runJS(`det.processRaw("Response to: hello\r\n");`)
	runJS(`det.processRaw("❯ ");`)
	assert.Equal(t, int64(int(StateReady)), runJS(`det.state()`).ToInteger())
}

func TestJS_TUIStateConstants(t *testing.T) {
	testutil.SkipSlow(t)
	runJS := jsIntegrationEnv(t)

	assert.Equal(t, int64(int(StateInitializing)), runJS(`cm.TUI_STATE_INITIALIZING`).ToInteger())
	assert.Equal(t, int64(int(StateReady)), runJS(`cm.TUI_STATE_READY`).ToInteger())
	assert.Equal(t, int64(int(StateProcessing)), runJS(`cm.TUI_STATE_PROCESSING`).ToInteger())
	assert.Equal(t, int64(int(StateError)), runJS(`cm.TUI_STATE_ERROR`).ToInteger())
	assert.Equal(t, int64(int(StateRateLimited)), runJS(`cm.TUI_STATE_RATE_LIMITED`).ToInteger())
	assert.Equal(t, int64(int(StatePermissionPrompt)), runJS(`cm.TUI_STATE_PERMISSION_PROMPT`).ToInteger())

	assert.Equal(t, "Ready", runJS(`cm.tuiStateName(cm.TUI_STATE_READY)`).String())
	assert.Equal(t, "Processing", runJS(`cm.tuiStateName(cm.TUI_STATE_PROCESSING)`).String())
}
