package bubbletea

import (
	"context"
	"io"
	"os"
	"os/signal"
	"sync"

	tea "charm.land/bubbletea/v2"
	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"golang.org/x/term"
)

// TerminalChecker provides terminal detection and file descriptor access.
// This interface allows dependency injection for testing and proper integration
// with terminal management wrappers like TUIReader/TUIWriter.
type TerminalChecker interface {
	// Fd returns the file descriptor of the underlying terminal.
	// Returns ^uintptr(0) (invalid) if the underlying resource doesn't have an Fd.
	Fd() uintptr

	// IsTerminal returns true if the underlying resource is a terminal.
	IsTerminal() bool
}

// JSRunner provides thread-safe JavaScript execution on the event loop.
// This interface abstracts the event loop synchronization mechanism,
// allowing BubbleTea's jsModel to safely call JavaScript functions from
// the BubbleTea goroutine without violating goja.Runtime's thread-safety
// requirements.
//
// CRITICAL: goja.Runtime is NOT thread-safe. All JS calls MUST go through
// RunSync when called from goroutines other than the event loop goroutine.
// This is especially important for BubbleTea's Init/Update/View methods
// which run on the BubbleTea goroutine, not the event loop goroutine.
type JSRunner interface {
	// RunSync schedules a function on the event loop and waits for completion.
	// The provided goja.Runtime is the event loop's runtime instance.
	// Returns an error if the event loop is not running or stops while waiting.
	// This method blocks until the callback completes.
	RunSync(ctx context.Context, fn func(*goja.Runtime) error) error
}

// AsyncJSRunner extends JSRunner with non-blocking async execution.
// This is required for tea.run() to return a Promise without blocking
// the event loop, allowing the Promise to be resolved/rejected when
// the BubbleTea program finishes.
type AsyncJSRunner interface {
	JSRunner
	// Run schedules a function on the event loop WITHOUT blocking.
	// Returns true if the function was successfully scheduled.
	// Returns false if the event loop is not running.
	Run(fn func(*goja.Runtime)) bool
}

// TrySyncJSRunner extends JSRunner with deadlock-safe synchronous execution.
// TryRunSync executes the callback directly when already on the event
// loop goroutine, and otherwise schedules-and-waits on the loop.
type TrySyncJSRunner interface {
	JSRunner
	TryRunSync(ctx context.Context, currentVM *goja.Runtime, fn func(*goja.Runtime) error) error
}

// Manager holds bubbletea-related state per engine instance.
type Manager struct {
	ctx          context.Context
	mu           sync.Mutex
	input        io.Reader
	output       io.Writer
	stderr       io.Writer // Stderr for logging panics/errors
	signalNotify func(c chan<- os.Signal, sig ...os.Signal)
	signalStop   func(c chan<- os.Signal)
	isTTY        bool                   // Whether input is a TTY
	ttyFd        int                    // TTY file descriptor (if available)
	program      *tea.Program           // Currently running program (if any)
	jsRunner     JSRunner               // REQUIRED: thread-safe JS execution via event loop
	promisify    PromisifyFunc          // Optional: keeps loop alive while program runs
	programDone  chan error             // Signals program exit; used by WaitForProgram()
	adapter      *gojaeventloop.Adapter // Optional: JS-promise exports (waitForProgram)
}

// PromisifyFunc is a function that executes work in a goroutine and returns a Future.
// This is used to keep the event loop alive while BubbleTea programs run.
type PromisifyFunc func(ctx context.Context, fn func(ctx context.Context) (any, error)) goeventloop.Future

// NewManager creates a new bubbletea manager for an engine instance.
// Input and output can be nil to use os.Stdin and os.Stdout.
// Automatically detects TTY and sets up proper terminal handling.
//
// CRITICAL: jsRunner is REQUIRED. It provides thread-safe JS execution from
// BubbleTea's goroutine. Without it, JS calls would cause data races.
// Use *bt.Bridge which implements JSRunner directly.
// Panics if jsRunner is nil.
func NewManager(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	jsRunner JSRunner,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if jsRunner == nil {
		panic("bubbletea.NewManager: jsRunner is REQUIRED - cannot be nil; provide a JSRunner implementation (e.g., *bt.Bridge)")
	}
	return NewManagerWithStderr(ctx, input, output, nil, jsRunner, signalNotify, signalStop)
}

// NewManagerWithStderr creates a new bubbletea manager with explicit stderr for error logging.
// Input, output, and stderr can be nil to use os.Stdin, os.Stdout, and os.Stderr.
// Automatically detects TTY and sets up proper terminal handling.
//
// CRITICAL: jsRunner is REQUIRED. It provides thread-safe JS execution from
// BubbleTea's goroutine. Without it, JS calls would cause data races.
// Use *bt.Bridge which implements JSRunner directly.
// Panics if jsRunner is nil.
func NewManagerWithStderr(
	ctx context.Context,
	input io.Reader,
	output io.Writer,
	stderr io.Writer,
	jsRunner JSRunner,
	signalNotify func(c chan<- os.Signal, sig ...os.Signal),
	signalStop func(c chan<- os.Signal),
) *Manager {
	if jsRunner == nil {
		panic("bubbletea.NewManagerWithStderr: jsRunner is REQUIRED - cannot be nil; provide a JSRunner implementation (e.g., *bt.Bridge)")
	}
	if ctx == nil {
		panic("bubbletea: nil context requires baseCtx threading")
	}
	// Track if we received nil inputs - if so, skip TTY detection entirely.
	// This avoids triggering stdin access during engine construction in tests.
	skipTTYDetection := input == nil && output == nil
	if input == nil {
		input = os.Stdin
	}
	if output == nil {
		output = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	if signalNotify == nil {
		signalNotify = signal.Notify
	}
	if signalStop == nil {
		signalStop = signal.Stop
	}

	m := &Manager{
		ctx:          ctx,
		input:        input,
		output:       output,
		stderr:       stderr,
		signalNotify: signalNotify,
		signalStop:   signalStop,
		isTTY:        false,
		ttyFd:        -1,
		jsRunner:     jsRunner, // REQUIRED: set at construction time
	}

	// Skip TTY detection if caller passed nil for both input and output.
	// This indicates the caller doesn't need terminal detection (e.g., during
	// engine construction for non-interactive scripts).
	if skipTTYDetection {
		return m
	}

	// Detect TTY for input - check TerminalChecker interface first (for TUIReader/TUIWriter wrappers)
	if tc, ok := input.(TerminalChecker); ok {
		if tc.IsTerminal() {
			fd := tc.Fd()
			if fd != ^uintptr(0) {
				m.isTTY = true
				m.ttyFd = int(fd)
			}
		}
	} else if f, ok := input.(*os.File); ok {
		// Fallback for direct *os.File
		fd := int(f.Fd())
		if term.IsTerminal(fd) {
			m.isTTY = true
			m.ttyFd = fd
		}
	}

	// If input isn't a TTY, check output
	if !m.isTTY {
		if tc, ok := output.(TerminalChecker); ok {
			if tc.IsTerminal() {
				fd := tc.Fd()
				if fd != ^uintptr(0) {
					m.isTTY = true
					m.ttyFd = int(fd)
				}
			}
		} else if f, ok := output.(*os.File); ok {
			fd := int(f.Fd())
			if term.IsTerminal(fd) {
				m.isTTY = true
				m.ttyFd = fd
			}
		}
	}

	return m
}

// SetJSRunner configures the Manager to use thread-safe JS execution.
// This MUST be called before any BubbleTea programs are run.
// The JSRunner ensures all JS calls from BubbleTea's goroutine are safely
// routed through the event loop.
//
// CRITICAL: JSRunner is MANDATORY. Without it, JS calls from BubbleTea's
// Init/Update/View methods would execute directly on the BubbleTea goroutine,
// causing data races with the event loop. This function panics if runner is nil.
func (m *Manager) SetJSRunner(runner JSRunner) {
	if runner == nil {
		panic("bubbletea: SetJSRunner called with nil runner - JSRunner is mandatory for thread-safe operation")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jsRunner = runner
}

// GetJSRunner returns the configured JSRunner, or nil if not set.
func (m *Manager) GetJSRunner() JSRunner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.jsRunner
}

// SetPromisify configures the Manager to use Promisify for keeping the event loop alive.
// When set, the goroutine running a BubbleTea program is wrapped in Promisify, which
// increments promisifyCount and keeps the loop alive while the program runs. This
// prevents premature loop shutdown during BubbleTea program execution.
func (m *Manager) SetPromisify(fn PromisifyFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.promisify = fn
}

// SetAdapter records the engine's goja-eventloop adapter so exports that must
// return JS promises can Promisify through it (the same pattern ctxutil,
// difftriage, and path use). Optional until such an export is called;
// waitForProgram panics with a TypeError when it is unset.
func (m *Manager) SetAdapter(a *gojaeventloop.Adapter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.adapter = a
}

// getAdapter returns the configured adapter, or nil when none was set.
func (m *Manager) getAdapter() *gojaeventloop.Adapter {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.adapter
}

// IsTTY returns whether the manager has access to a TTY.
func (m *Manager) IsTTY() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isTTY
}
