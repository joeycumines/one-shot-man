package bubbletea

import (
	"testing"

	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
)

// TestSetJSRunner_Validation verifies that SetJSRunner panics on nil.
func TestSetJSRunner_Validation(t *testing.T) {
	m := &Manager{}

	assert.Panics(t, func() {
		m.SetJSRunner(nil)
	}, "Should panic on nil runner")

	// Should not panic on valid runner
	runner := &SyncJSRunner{Runtime: goja.New()}
	assert.NotPanics(t, func() {
		m.SetJSRunner(runner)
	})

	assert.Equal(t, runner, m.GetJSRunner())
}

// TestExtractTickCmd verifies tick command extraction logic.
func TestExtractTickCmd(t *testing.T) {
	vm := goja.New()

	model := &jsModel{
		runtime: vm,
	}

	tests := []struct {
		name      string
		obj       func() *goja.Object
		expectCmd bool
	}{
		{
			name: "Valid Tick",
			obj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("duration", 100)
				obj.Set("id", "timer1")
				return obj
			},
			expectCmd: true,
		},
		{
			name: "Missing Duration",
			obj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("id", "timer1")
				return obj
			},
			expectCmd: false,
		},
		{
			name: "Zero Duration",
			obj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("duration", 0)
				return obj
			},
			expectCmd: false,
		},
		{
			name: "Negative Duration",
			obj: func() *goja.Object {
				obj := vm.NewObject()
				obj.Set("duration", -10)
				return obj
			},
			expectCmd: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := model.extractTickCmd(tc.obj())
			if tc.expectCmd {
				assert.NotNil(t, cmd)
				// We can run the command to verify it produces a TickMsg
				// But tea.Tick returns a command that sleeps. We don't want to sleep in unit test.
				// However, extractTickCmd returns the cmd.
				// We can't inspecting the cmd function without running it.
				// Just verifying non-nil is good enough for extraction logic.
			} else {
				assert.Nil(t, cmd)
			}
		})
	}
}

// TestSendStateRefresh_Safety verifies that SendStateRefresh is safe to call.
func TestSendStateRefresh_Safety(t *testing.T) {
	manager := &Manager{}

	// Case 1: No program running
	assert.NotPanics(t, func() {
		manager.SendStateRefresh("key1")
	})

	// Case 2: Program set (mocking behavior requires real program or risky reflection)
	// We only test the safety of the nil check here.
	// Integration tests cover the actual message delivery.
}

// TestExtractBatchSequenceCmd verifies batch/sequence extraction.
func TestExtractBatchSequenceCmd(t *testing.T) {
	vm := goja.New()
	model := &jsModel{runtime: vm}
	model.jsRunner = &SyncJSRunner{Runtime: vm} // needed for valueToCmd internals -> actually valueToCmd checks m/runtime

	// Helper to create a command object
	createCmdObj := func(typ string) goja.Value {
		obj := vm.NewObject()
		obj.Set("_cmdType", typ)
		return obj
	}

	// Test Batch
	batchObj := vm.NewObject()
	cmdsArr := vm.NewArray(createCmdObj("quit"), createCmdObj("clearScreen"))
	batchObj.Set("cmds", cmdsArr)

	batchCmd := model.extractBatchCmd(batchObj)
	assert.NotNil(t, batchCmd, "Should return batch cmd")

	// Test Sequence
	seqObj := vm.NewObject()
	seqObj.Set("cmds", cmdsArr)

	seqCmd := model.extractSequenceCmd(seqObj)
	assert.NotNil(t, seqCmd, "Should return sequence cmd")

	// Test Empty/Invalid inputs
	assert.Nil(t, model.extractBatchCmd(vm.NewObject()))
	assert.Nil(t, model.extractSequenceCmd(vm.NewObject()))
}
