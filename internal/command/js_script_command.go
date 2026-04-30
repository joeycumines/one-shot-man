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
		scriptCommandBase: scriptCommandBase{
			config:   cfg,
			logLevel: "info",
		},
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
	// Create execution context with signal handling.
	var ctx context.Context
	var cancel context.CancelFunc
	if c.ctxFactory != nil {
		ctx, cancel = c.ctxFactory()
	} else {
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
	defer cancel()

	// Create scripting engine using shared base.
	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

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
		return fmt.Errorf("failed to execute script %s: %w", c.scriptPath, err)
	}

	// If interactive (from shebang -i or CLI flag), launch terminal.
	if c.interactive {
		terminal := c.terminalFactory(ctx, engine)
		terminal.Run()
		return nil
	}

	// Wait for any asynchronous work (timers, fetch, etc.) to complete naturally.
	// This uses the WithAutoExit(true) feature of the event loop.
	engine.Wait()

	return nil
}
