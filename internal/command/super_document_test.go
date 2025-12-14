package command

import (
	"bytes"
	"context"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSuperDocumentCommand(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)

	assert.Equal(t, "super-document", cmd.Name())
	assert.Contains(t, cmd.Description(), "TUI")
	assert.Contains(t, cmd.Description(), "document")
	assert.Contains(t, cmd.Usage(), "super-document")
}

func TestSuperDocumentCommand_SetupFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)

	// Default values are set when flags are parsed, not at construction
	// Test that command is created with sensible defaults
	assert.NotNil(t, cmd.config)
	assert.Equal(t, "super-document", cmd.Name())
}

func TestSuperDocumentCommand_Execute_TestMode(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	require.NoError(t, err)

	// In test mode, script should execute without entering interactive mode
	output := stdout.String()
	assert.Contains(t, output, "Super-Document")
}

func TestSuperDocumentCommand_Execute_WithSession(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.session = "test-session-" + t.Name()
	cmd.store = "memory"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Super-Document")
}

func TestSuperDocumentCommand_REPLMode(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.replMode = true
	cmd.interactive = false
	cmd.store = "memory"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Super-Document")
}

// TestSuperDocumentTUIScript_BubbleteaIntegration tests that the TUI script
// can be loaded and the bubbletea/lipgloss modules are available.
func TestSuperDocumentTUIScript_BubbleteaIntegration(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-tui-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	// Test that bubbletea module is available
	script := engine.LoadScriptFromString("test-bubbletea", `
		const tea = require('osm:bubbletea');
		if (!tea.newModel) throw new Error('newModel not available');
		if (!tea.run) throw new Error('run not available');
		if (!tea.quit) throw new Error('quit not available');
		if (!tea.batch) throw new Error('batch not available');
		if (!tea.sequence) throw new Error('sequence not available');
		if (!tea.clearScreen) throw new Error('clearScreen not available');
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}

// TestSuperDocumentTUIScript_LipglossIntegration tests that the lipgloss module
// works correctly for styling.
func TestSuperDocumentTUIScript_LipglossIntegration(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-lipgloss-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	// Test that lipgloss module is available and functional
	script := engine.LoadScriptFromString("test-lipgloss", `
		const lipgloss = require('osm:lipgloss');
		
		// Test style creation
		const style = lipgloss.newStyle();
		if (!style) throw new Error('newStyle failed');
		
		// Test chaining (immutable)
		const boldStyle = style.bold(true);
		if (!boldStyle) throw new Error('bold failed');
		
		// Test rendering
		const rendered = boldStyle.render('test');
		if (typeof rendered !== 'string') throw new Error('render failed');
		
		// Test borders
		const border = lipgloss.roundedBorder();
		if (!border) throw new Error('roundedBorder failed');
		
		// Test alignment constants exist (they are Position objects, not numbers)
		if (lipgloss.Left === undefined) throw new Error('Left constant missing');
		if (lipgloss.Center === undefined) throw new Error('Center constant missing');
		if (lipgloss.Right === undefined) throw new Error('Right constant missing');
		
		// Test layout utilities
		const joined = lipgloss.joinHorizontal(lipgloss.Top, 'a', 'b');
		if (typeof joined !== 'string') throw new Error('joinHorizontal failed');
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}

// TestSuperDocumentTUIScript_ModelCreation tests that the TUI model can be created.
func TestSuperDocumentTUIScript_ModelCreation(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-model-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	// Test model creation
	script := engine.LoadScriptFromString("test-model", `
		const tea = require('osm:bubbletea');
		
		const model = tea.newModel({
			init: function() {
				return { count: 0 };
			},
			update: function(msg, state) {
				return [state, null];
			},
			view: function(state) {
				return 'Count: ' + state.count;
			}
		});
		
		if (model.error) throw new Error('Model creation failed: ' + model.error);
		if (model._type !== 'bubbleteaModel') throw new Error('Invalid model type');
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}

// TestSuperDocumentTUIScript_StyleImmutability tests that lipgloss styles are immutable.
func TestSuperDocumentTUIScript_StyleImmutability(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-immutable-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	// Test style immutability
	script := engine.LoadScriptFromString("test-immutability", `
		const lipgloss = require('osm:lipgloss');
		
		const base = lipgloss.newStyle();
		const derived = base.bold(true).foreground('#FF0000').padding(2);
		
		// Both should be valid styles
		const baseRendered = base.render('test');
		const derivedRendered = derived.render('test');
		
		// Derived should have padding (adds visible characters)
		// This tests that derived is a new object, not a mutation of base
		if (baseRendered.length >= derivedRendered.length) {
			throw new Error('Styles should be immutable - derived should have more content due to padding');
		}
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}

// TestSuperDocumentTUIScript_PromptBuilding tests the prompt building logic.
func TestSuperDocumentTUIScript_PromptBuilding(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-prompt-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	// Test prompt building functions
	script := engine.LoadScriptFromString("test-prompt-building", `
		// Simulate the calculateBacktickFence function
		function calculateBacktickFence(documents) {
			let maxLen = 0;
			for (let i = 0; i < documents.length; i++) {
				const content = documents[i].content || '';
				let currentRun = 0;
				for (let j = 0; j < content.length; j++) {
					if (content[j] === '`+"`"+`') {
						currentRun++;
						if (currentRun > maxLen) {
							maxLen = currentRun;
						}
					} else {
						currentRun = 0;
					}
				}
			}
			let fenceLen = maxLen + 1;
			if (fenceLen < 3) fenceLen = 3;
			let fence = '';
			for (let i = 0; i < fenceLen; i++) fence += '`+"`"+`';
			return fence;
		}
		
		// Test with no backticks
		let fence = calculateBacktickFence([{content: 'no backticks'}]);
		if (fence !== '`+"```"+`') throw new Error('Expected 3 backticks, got: ' + fence);
		
		// Test with backticks
		fence = calculateBacktickFence([{content: 'has `+"```"+` in content'}]);
		if (fence !== '`+"````"+`') throw new Error('Expected 4 backticks, got: ' + fence);
		
		// Test with many backticks
		fence = calculateBacktickFence([{content: 'has `+"`````"+` in content'}]);
		if (fence !== '`+"``````"+`') throw new Error('Expected 6 backticks, got: ' + fence);
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}

// TestSuperDocumentTUIScript_CommandsAvailable tests that tea commands are available.
func TestSuperDocumentTUIScript_CommandsAvailable(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-cmds-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("test-commands", `
		const tea = require('osm:bubbletea');
		
		// Test quit command
		const quitCmd = tea.quit();
		if (!quitCmd._cmdType || quitCmd._cmdType !== 'quit') {
			throw new Error('quit command invalid');
		}
		
		// Test clearScreen command
		const clearCmd = tea.clearScreen();
		if (!clearCmd._cmdType || clearCmd._cmdType !== 'clearScreen') {
			throw new Error('clearScreen command invalid');
		}
		
		// Test batch command
		const batchCmd = tea.batch(tea.quit(), tea.clearScreen());
		if (!batchCmd._cmdType || batchCmd._cmdType !== 'batch') {
			throw new Error('batch command invalid');
		}
		if (!batchCmd.cmds || batchCmd.cmds.length !== 2) {
			throw new Error('batch should have 2 commands');
		}
		
		// Test sequence command
		const seqCmd = tea.sequence(tea.quit());
		if (!seqCmd._cmdType || seqCmd._cmdType !== 'sequence') {
			throw new Error('sequence command invalid');
		}
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}

// TestSuperDocumentTUIScript_LayoutFunctions tests lipgloss layout functions.
func TestSuperDocumentTUIScript_LayoutFunctions(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer

	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, "test-layout-"+t.Name(), "memory")
	require.NoError(t, err)
	defer engine.Close()

	engine.SetTestMode(true)

	script := engine.LoadScriptFromString("test-layout", `
		const lipgloss = require('osm:lipgloss');
		
		// Test joinHorizontal
		const horizontal = lipgloss.joinHorizontal(lipgloss.Top, 'left', 'right');
		if (!horizontal.includes('left') || !horizontal.includes('right')) {
			throw new Error('joinHorizontal failed');
		}
		
		// Test joinVertical
		const vertical = lipgloss.joinVertical(lipgloss.Left, 'top', 'bottom');
		if (!vertical.includes('top') || !vertical.includes('bottom')) {
			throw new Error('joinVertical failed');
		}
		
		// Test place
		const placed = lipgloss.place(10, 5, lipgloss.Center, lipgloss.Center, 'X');
		if (!placed.includes('X')) {
			throw new Error('place failed');
		}
		
		// Test size functions
		const size = lipgloss.size('hello');
		if (size.width !== 5) {
			throw new Error('width calculation failed');
		}
		
		const width = lipgloss.width('hello');
		if (width !== 5) {
			throw new Error('width function failed');
		}
		
		const height = lipgloss.height('a\nb\nc');
		if (height !== 3) {
			throw new Error('height function failed');
		}
	`)
	err = engine.ExecuteScript(script)
	require.NoError(t, err)
}
