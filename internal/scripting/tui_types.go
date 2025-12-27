package scripting

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/joeycumines/go-prompt"
	"github.com/joeycumines/one-shot-man/internal/builtin"
)

// atomicBool provides a simple atomic boolean for shutdown coordination.
type atomicBool struct {
	v int32
}

func (b *atomicBool) Set() {
	atomic.StoreInt32(&b.v, 1)
}

func (b *atomicBool) Clear() {
	atomic.StoreInt32(&b.v, 0)
}

func (b *atomicBool) IsSet() bool {
	return atomic.LoadInt32(&b.v) == 1
}

// TUIManager manages rich terminal interfaces for script modes.
//
// LOCKING STRATEGY (for maintainers):
//   - tm.mu is a sync.RWMutex protecting manager-wide mutable state (modes, commands,
//     prompts, activePrompt, keyBindings, completers, promptCompleters, commandOrder, etc.).
//   - Read operations (GetCurrentMode, ListModes, completion lookups) use RLock and are
//     safe to call from JS callbacks.
//   - MUTATIONS from JS callbacks MUST be routed through the writer goroutine via
//     scheduleWrite or scheduleWriteAndWait. This prevents deadlocks when JS callbacks
//     call into mutating APIs while Go code may be waiting on JS.
//   - INVARIANT: Never call into the goja JS runtime while holding mu.Lock(). Reads
//     (RLock) are safe but must not perform mutations. Schedule mutations via the
//     writer queue.
type TUIManager struct {
	engine           *Engine
	ctx              context.Context
	currentMode      *ScriptMode
	modes            map[string]*ScriptMode
	commands         map[string]Command
	commandOrder     []string // maintains insertion order of commands
	mu               sync.RWMutex
	reader           *TUIReader                // Concrete reader type with lazy initialization
	writer           *TUIWriter                // Concrete writer type with lazy initialization
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

	// stateManager orchestrates all persistence logic for this TUI instance
	// Use the builtin.StateManager interface to avoid tight coupling to the
	// concrete implementation.
	stateManager builtin.StateManager

	// commandHistory holds the list of commands from persistent history
	commandHistory []string

	// writerQueue is a channel for queuing mutation tasks to be executed by
	// a single dedicated writer goroutine under tm.mu.Lock(). This prevents
	// deadlocks when JS callbacks (running without a lock) call mutating APIs.
	writerQueue chan writeTask

	// writerDone is closed when the writer goroutine exits.
	writerDone chan struct{}

	// writerShutdown is set atomically to 1 when shutdown is initiated.
	// This is checked before attempting to send to writerQueue.
	writerShutdown atomicBool

	// writerStop is closed to signal the writer goroutine to exit.
	// The writerQueue channel is NEVER closed to prevent panics from racing senders.
	// This implements the "Signal, Don't Close" pattern.
	writerStop chan struct{}

	// queueMu protects the sequencing of checking writerShutdown and sending
	// to writerQueue. This prevents the "check-then-send" race condition.
	queueMu sync.Mutex

	// debugLockState tracks the lock depth for debug assertions.
	// It replaces the global tracker to ensure parallel tests are safe.
	// Used only when built with -tags debug.
	debugLockState atomic.Int32

	// exitRequested is a runtime-only flag (NEVER persisted) that signals the
	// shell loop should exit after the current command completes. This is set
	// by JavaScript via tui.requestExit() and checked by the exit checker.
	// It is an atomicBool for thread-safe access without locking.
	exitRequested atomicBool
}

// writeTask represents a mutation task to be executed by the writer goroutine.
type writeTask struct {
	fn       func() error
	resultCh chan error // nil for fire-and-forget, non-nil for synchronous wait
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
	// InitialCommand is an optional command string  to execute when starting the prompt.
	// Basically, it defers _visibly_ starting the prompt, until after the initial command is run.
	InitialCommand string
	mu             sync.RWMutex
}

// TUIConfig defines the configuration for a rich TUI interface.
type TUIConfig struct {
	Title        string
	Prompt       string
	CompletionFn goja.Callable
	ValidatorFn  goja.Callable
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
