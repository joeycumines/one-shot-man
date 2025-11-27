package scripting

import (
	"context"
	"io"
	"os"
	"sync"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	"github.com/joeycumines/one-shot-man/internal/scripting/builtin"
)

// TUIManager manages rich terminal interfaces for script modes.

// TUIManager manages rich terminal interfaces for script modes.
type TUIManager struct {
	engine           *Engine
	ctx              context.Context
	currentMode      *ScriptMode
	modes            map[string]*ScriptMode
	commands         map[string]Command
	commandOrder     []string // maintains insertion order of commands
	mu               sync.RWMutex
	input            io.Reader
	output           io.Writer
	prompts          map[string]*prompt.Prompt // Manages named prompt instances
	activePrompt     *prompt.Prompt            // Pointer to the currently active prompt
	completers       map[string]goja.Callable  // JavaScript completion functions
	keyBindings      map[string]goja.Callable  // JavaScript key binding handlers
	promptCompleters map[string]string         // Maps prompt names to completer names
	// defaultColors controls the default color scheme used when running prompts
	// without explicit color configuration. It is initialized with sensible
	// defaults and can be overridden by configuration (e.g., config file).
	defaultColors PromptColors

	// outputQueue buffers script output so it can be written at safe points
	// in the prompt lifecycle, preventing races with go-prompt redraws.
	outputQueue []string
	outputMu    sync.Mutex

	// history stores command history per mode
	history map[string][]HistoryEntry

	// stateManager orchestrates all persistence logic for this TUI instance
	// Use the builtin.StateManager interface to avoid tight coupling to the
	// concrete implementation.
	stateManager builtin.StateManager

	// sessionID uniquely identifies this TUI session for persistence
	sessionID string

	// commandHistory holds the list of commands from persistent history
	commandHistory []string
}

// ScriptMode represents a specific script mode with its own state and commands.
// NOTE: State and StateContract fields DELETED - JS manages state via tui.createState()
type ScriptMode struct {
	Name            string
	Script          *Script
	Commands        map[string]Command
	CommandsBuilder goja.Callable // Renamed from BuildCommands for consistency
	CommandOrder    []string      // maintains insertion order of commands
	TUIConfig       *TUIConfig
	OnEnter         goja.Callable
	OnExit          goja.Callable
	OnPrompt        goja.Callable
	mu              sync.RWMutex
}

// TUIConfig defines the configuration for a rich TUI interface.
type TUIConfig struct {
	Title         string
	Prompt        string
	CompletionFn  goja.Callable
	ValidatorFn   goja.Callable
	HistoryFile   string
	EnableHistory bool
}

// Command represents a command that can be executed in the terminal.
type Command struct {
	Name          string
	Description   string
	Usage         string
	Handler       interface{} // Can be goja.Callable or Go function
	IsGoCommand   bool
	ArgCompleters []string
}

// PromptColors represents color configuration for a prompt.
type PromptColors struct {
	InputText               prompt.Color
	PrefixText              prompt.Color
	SuggestionText          prompt.Color
	SuggestionBG            prompt.Color
	SelectedSuggestionText  prompt.Color
	SelectedSuggestionBG    prompt.Color
	DescriptionText         prompt.Color
	DescriptionBG           prompt.Color
	SelectedDescriptionText prompt.Color
	SelectedDescriptionBG   prompt.Color
	ScrollbarThumb          prompt.Color
	ScrollbarBG             prompt.Color
}

// HistoryConfig represents history configuration for a prompt.
type HistoryConfig struct {
	Enabled bool
	File    string
	Size    int
}

// syncWriter wraps an io.Writer and calls Sync if it's an *os.File
type syncWriter struct {
	io.Writer
}

func (w *syncWriter) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if f, ok := w.Writer.(*os.File); ok {
		f.Sync()
	}
	return
}
