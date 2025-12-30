package bubbletea

import (
	"context"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/require"
)

// TestMixedComponents_BatchExecutesAll verifies that a JS-side batch containing
// wrapped Go commands (simulating different components) correctly extracts
// into a single tea.Cmd and that executing it runs all constituent commands.
func TestMixedComponents_BatchExecutesAll(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(ctx, nil, nil, nil, nil)

	vm := goja.New()
	module := vm.NewObject()
	require.NoError(t, module.Set("exports", vm.NewObject()))

	requireFn := Require(ctx, manager)
	requireFn(vm, module)

	_ = vm.Set("tea", module.Get("exports"))

	// Create a model to obtain a jsModel with runtime for valueToCmd
	result, err := vm.RunString(`tea.newModel({ init: function() { return {}; }, update: function(msg, model) { return [model, null]; }, view: function(model) { return ''; } });`)
	require.NoError(t, err)

	modelWrapper := result.ToObject(vm)
	getModelFn, ok := goja.AssertFunction(modelWrapper.Get("_getModel"))
	require.True(t, ok, "_getModel should be a function")
	modelVal, err := getModelFn(goja.Undefined())
	require.NoError(t, err)
	m, ok := modelVal.Export().(*jsModel)
	require.True(t, ok)

	// Create two closure commands that record side-effects
	calls := make([]string, 0)
	var mu sync.Mutex
	c1 := func() tea.Msg { mu.Lock(); defer mu.Unlock(); calls = append(calls, "textarea"); return nil }
	c2 := func() tea.Msg { mu.Lock(); defer mu.Unlock(); calls = append(calls, "viewport"); return nil }

	g1 := WrapCmd(m.runtime, c1)
	g2 := WrapCmd(m.runtime, c2)

	_ = vm.Set("taCmd", g1)
	_ = vm.Set("vpCmd", g2)

	val, err := vm.RunString(`({ _cmdType: 'batch', _cmdID: 42, cmds: [ taCmd, vpCmd ] })`)
	require.NoError(t, err)

	// Ensure individual elements convert to commands and execute
	first, err := vm.RunString(`({ _cmdType: 'batch', _cmdID: 42, cmds: [ taCmd, vpCmd ] }).cmds[0]`)
	require.NoError(t, err)
	second, err := vm.RunString(`({ _cmdType: 'batch', _cmdID: 42, cmds: [ taCmd, vpCmd ] }).cmds[1]`)
	require.NoError(t, err)

	cmd1 := m.valueToCmd(first)
	require.NotNil(t, cmd1)
	cmd1()

	cmd2 := m.valueToCmd(second)
	require.NotNil(t, cmd2)
	cmd2()

	// Execute the batch command
	cmd := m.valueToCmd(val)
	require.NotNil(t, cmd)
	cmd()

	// Wait up to 200ms for both commands to execute (allow for concurrency scheduling)
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		mu.Lock()
		if len(calls) >= 2 {
			mu.Unlock()
			break
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, 2, len(calls), "both component commands should have executed")
	require.Equal(t, "textarea", calls[0])
	require.Equal(t, "viewport", calls[1])
}
