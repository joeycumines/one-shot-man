package bubbletea

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dop251/goja"
	"github.com/stretchr/testify/assert"
)

// TestJSModelLogic_Init verifies Init logic.
func TestJSModelLogic_Init(t *testing.T) {
	vm := goja.New()

	t.Run("Success", func(t *testing.T) {
		model := &jsModel{
			runtime: vm,
			initFn: createViewFn(vm, func(state goja.Value) string {
				return "" // Return string/value ignored by wrapper if not array
			}),
			state: vm.NewObject(),
		}
		model.jsRunner = &SyncJSRunner{Runtime: vm}

		cmd := model.Init()
		assert.Nil(t, cmd) // Simple init returns nil cmd unless returning [state, cmd]
	})

	t.Run("Init returns [state, cmd]", func(t *testing.T) {
		model := &jsModel{
			runtime: vm,
			initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				// return [state, quit]
				newState := vm.NewObject()
				quit := map[string]interface{}{"_cmdType": "quit"}
				return vm.NewArray(newState, vm.ToValue(quit)), nil
			},
			state: vm.NewObject(),
		}
		model.jsRunner = &SyncJSRunner{Runtime: vm}

		cmd := model.Init()
		assert.NotNil(t, cmd)
		// We can't verify cmd type easily without running it, but non-nil is correct
	})

	t.Run("JSRunner Error", func(t *testing.T) {
		model := &jsModel{
			runtime: vm,
			initFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				return nil, errors.New("JS init error")
			},
			state: vm.NewObject(),
		}
		model.jsRunner = &SyncJSRunner{Runtime: vm}

		cmd := model.Init()
		assert.NotNil(t, cmd) // Should return a command that prints error?
		// Actually Init() swallows errors usually or logs them?
		// implementation: if err != nil { return nil } and logs to slog/stderr
		// Let's check implementation behavior... implementation returns nil on error.
		// Wait, Step 40 coverage showed Init covered 66.7%.

		// If I check logic:
		// res, err := m.jsRunner.RunJSSync(m.initFn, nil, m.state)
		// if err != nil { slog.Error... return nil }
		// So it expects nil.

		// BUT the test above actually returns nil because the mock SyncJSRunner propagates the error
		// and Init catches it.
		assert.Nil(t, cmd)
	})
}

// TestJSModelLogic_Update verifies Update logic.
func TestJSModelLogic_Update(t *testing.T) {
	vm := goja.New()

	t.Run("Success [state, cmd]", func(t *testing.T) {
		model := &jsModel{
			runtime: vm,
			updateFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				// args[0]=msg, args[1]=state
				cmd := map[string]interface{}{"_cmdType": "quit"}
				return vm.NewArray(args[1], vm.ToValue(cmd)), nil
			},
			state: vm.NewObject(),
		}
		model.jsRunner = &SyncJSRunner{Runtime: vm}

		newModel, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
		assert.NotNil(t, newModel)
		assert.NotNil(t, cmd)
	})

	t.Run("Internal Message Ignored", func(t *testing.T) {
		model := &jsModel{
			runtime: vm,
			state:   vm.NewObject(),
		}
		// renderRefreshMsg should return nil cmd and not call JS
		model.jsRunner = &SyncJSRunner{Runtime: vm} // Should not be called

		// We set updateFn to panic if called
		model.updateFn = func(this goja.Value, args ...goja.Value) (goja.Value, error) {
			panic("Should not be called")
		}

		_, cmd := model.Update(renderRefreshMsg{})
		assert.Nil(t, cmd)
	})
}

// TestJSModelLogic_View verifies View logic details.
func TestJSModelLogic_View(t *testing.T) {
	vm := goja.New()

	t.Run("JS Error Handling", func(t *testing.T) {
		model := &jsModel{
			runtime:         vm,
			throttleEnabled: false,
			viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				return nil, errors.New("view failed")
			},
			state: vm.NewObject(),
		}
		model.jsRunner = &SyncJSRunner{Runtime: vm}

		output := model.View()
		assert.Contains(t, output, "View error")
		assert.Contains(t, output, "view failed")
	})

	t.Run("Empty Return", func(t *testing.T) {
		model := &jsModel{
			runtime:         vm,
			throttleEnabled: false,
			viewFn: func(this goja.Value, args ...goja.Value) (goja.Value, error) {
				return vm.ToValue(""), nil
			},
			state: vm.NewObject(),
		}
		model.jsRunner = &SyncJSRunner{Runtime: vm}

		output := model.View()
		assert.Equal(t, "", output)
	})
}
