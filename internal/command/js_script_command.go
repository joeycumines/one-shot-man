package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// jsScriptCommand executes a JavaScript script in-process via the Goja engine.
// It is created by findScriptCommand when the discovered script is a .js file
// or has an "osm script" shebang. Unlike scriptCommand (which spawns an external
// process via shebang), this runs the script in the same process, eliminating
// dependency on shebang support, PATH availability, and binary version matching.
type jsScriptCommand struct {
	*BaseCommand
	scriptCommandBase
	scriptPath         string
	interactive        bool
	shebangInteractive bool
	shebangTestMode    bool
	terminalFactory    func(context.Context, *scripting.Engine) terminalRunner
	// ctxFactory creates the execution context. If nil, uses signal.NotifyContext
	// for proper signal handling. Tests should set this to avoid signal handling races.
	ctxFactory func() (context.Context, context.CancelFunc)
}

// newJSScriptCommand creates a new JS script command for in-process execution.
func newJSScriptCommand(name, scriptPath string, cfg *config.Config, peek scriptPeekInfo) *jsScriptCommand {
	return &jsScriptCommand{
		BaseCommand: NewBaseCommand(
			name,
			fmt.Sprintf("JavaScript script: %s", name),
			fmt.Sprintf("%s [options] [args...]", name),
		),
		config:             cfg,
		logLevel:           "info",
		scriptPath:         scriptPath,
		shebangInteractive: peek.interactive,
		shebangTestMode:    peek.testMode,
		terminalFactory: func(ctx context.Context, engine *scripting.Engine) terminalRunner {
			return scripting.NewTerminal(ctx, engine)
		},
	}
}

// SetupFlags registers flags for the JS script command.
// Shebang-derived flags (-i, --test) serve as defaults that CLI flags can override.
func (c *jsScriptCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", c.shebangInteractive, "Start interactive scripting terminal")
	fs.BoolVar(&c.interactive, "i", c.shebangInteractive, "Start interactive scripting terminal (short form)")
	c.RegisterFlags(fs)

	// Override --test default if shebang says so.
	if c.shebangTestMode {
		if f := fs.Lookup("test"); f != nil {
			f.DefValue = "true"
			c.testMode = true
		}
	}
}

// Execute runs the JavaScript script in-process via the Goja engine.
// The execution path is equivalent to `osm script <file>`:
// PrepareEngine creates the engine with logging, terminal I/O, and session
// management; LoadScript loads the file; ExecuteScript evaluates it and
// blocks on WaitForProgram if tea.run() was called.
func (c *jsScriptCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Create execution context. Interactive (terminal-driven) scripts keep the
	// blunt NotifyContext lifecycle; plain script runs get Node-style signal
	// handling below.
	var ctx context.Context
	var cancel context.CancelFunc
	var startSignals func(*scripting.Engine, context.CancelFunc) (fallback func() int, stop func())
	if c.ctxFactory != nil {
		ctx, cancel = c.ctxFactory()
	} else if c.interactive {
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
		startSignals = startNodeSignalDelivery // bound to the cancel below
	}
	defer cancel()

	// Create scripting engine using shared base.
	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	// Deliver SIGINT/SIGTERM to the script the way Node would: the first
	// signal reaches process listeners, a second signal of the same kind
	// forces termination, and an unlistened signal terminates with the
	// default status.
	var signalFallback func() int
	if startSignals != nil {
		var stopSignals func()
		signalFallback, stopSignals = startSignals(engine, cancel)
		defer stopSignals()
	}

	// Set global default logger.
	slog.SetDefault(engine.Logger())

	// Pass remaining args to the script as the 'args' global.
	engine.SetGlobal("args", args)

	// Load and execute the script.
	scriptName := filepath.Base(c.scriptPath)
	script, err := engine.LoadScript(scriptName, c.scriptPath)
	if err != nil {
		return fmt.Errorf("failed to load script %s: %w", c.scriptPath, err)
	}

	if err := engine.ExecuteScript(script); err != nil {
		// An unlistened signal force-cancels the runtime, which surfaces as a
		// context-cancellation error from the in-flight script (a running
		// program is aborted via WaitForProgram). Node's default disposition
		// still applies: report 128+N, not a generic failure.
		if signalFallback != nil {
			if code := signalFallback(); code != 0 {
				return &SilentError{Err: &ExitError{Code: code}}
			}
		}
		return fmt.Errorf("failed to execute script %s: %w", c.scriptPath, err)
	}

	// If interactive (from shebang -i or CLI flag), launch terminal.
	if c.interactive {
		terminal := c.terminalFactory(ctx, engine)
		terminal.Run()
		return nil
	}

	// An unlistened signal terminated the run: Node's default status is
	// 128 plus the signal number (130 for SIGINT, 143 for SIGTERM), and it
	// wins over any script-settled exit code because signal death is what
	// Node's default disposition would have produced. Check it BEFORE the
	// async drain: an unhandled signal terminates immediately in Node, so a
	// pending timer must not keep the process alive after the fallback fires
	// (observed: a probe with a 60s timer lingered for the full minute after
	// SIGTERM even though the runtime had already been cancelled).
	if signalFallback != nil {
		if code := signalFallback(); code != 0 {
			return &SilentError{Err: &ExitError{Code: code}}
		}
	}

	// Wait for any asynchronous work (timers, fetch, etc.) to complete naturally.
	// This uses the WithAutoExit(true) feature of the event loop.
	engine.Wait()

	// Re-check the fallback after the drain: a signal that arrived while Wait
	// was blocked sets the status and force-cancels, and Wait then returns —
	// without this the run would fall through to success (exit 0) instead of
	// Node's 128+N for an unlistened signal.
	if signalFallback != nil {
		if code := signalFallback(); code != 0 {
			return &SilentError{Err: &ExitError{Code: code}}
		}
	}
	// Node exit channel: a script-settled process.exit / process.exitCode
	// becomes this process's status, silently — Node prints nothing for a
	// nonzero exit.
	if code, ok := engine.ExitCode(); ok && code != 0 {
		return &SilentError{Err: &ExitError{Code: code}}
	}
	return nil
}

// startNodeSignalDelivery installs Node-style SIGINT/SIGTERM handling for a
// plain script run and returns (fallback, stop). The first signal of a kind
// is submitted to the event loop as process.emit(name); when a listener
// handled it the script stays in charge of its own lifecycle. When no
// listener exists — emit reported none, or the submit failed because the loop
// was already gone — the context is cancelled, which terminates the runtime,
// and fallback() later reports Node's default status (128 plus the signal
// number). A second signal of the same kind cancels unconditionally: that is
// Node's force-terminate contract. stop removes the notification and releases
// the handler goroutine.
func startNodeSignalDelivery(engine *scripting.Engine, forceCancel context.CancelFunc) (func() int, func()) {
	var mu sync.Mutex
	counts := make(map[string]int)
	var fallbackStatus atomic.Int64

	signals := make(chan os.Signal, 4)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-signals:
				name := signalName(sig)
				mu.Lock()
				counts[name]++
				count := counts[name]
				mu.Unlock()
				if count > 1 {
					// Second signal of the same kind: force terminate.
					fallbackStatus.CompareAndSwap(0, int64(128+signalNumber(sig)))
					forceCancel()
					continue
				}
				if engine.DeliverSignal(name) {
					// A listener received the signal; the script decides.
					continue
				}
				fallbackStatus.CompareAndSwap(0, int64(128+signalNumber(sig)))
				forceCancel()
			case <-done:
				return
			}
		}
	}()

	return func() int { return int(fallbackStatus.Load()) },
		func() {
			signal.Stop(signals)
			close(done)
		}
}

// signalName maps a delivered signal to the name Node uses for process.emit.
func signalName(sig os.Signal) string {
	switch sig {
	case os.Interrupt:
		return "SIGINT"
	case syscall.SIGTERM:
		return "SIGTERM"
	default:
		return sig.String()
	}
}

// signalNumber returns the POSIX number of a delivered signal for the
// 128+N default exit status.
func signalNumber(sig os.Signal) int {
	switch sig {
	case os.Interrupt:
		return int(syscall.SIGINT)
	case syscall.SIGTERM:
		return int(syscall.SIGTERM)
	default:
		return 0
	}
}
