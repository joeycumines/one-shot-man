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
	"strings"
	"syscall"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// ScriptingCommand provides JavaScript scripting capabilities.
type ScriptingCommand struct {
	*BaseCommand
	scriptCommandBase
	interactive     bool
	script          string
	engineFactory   func(context.Context, io.Writer, io.Writer) (*scripting.Engine, error)
	terminalFactory func(context.Context, *scripting.Engine) terminalRunner
	// ctxFactory creates the execution context. If nil, uses signal.NotifyContext for proper
	// signal handling. Tests should set this to avoid signal handling races.
	ctxFactory func() (context.Context, context.CancelFunc)
}

type terminalRunner interface {
	Run()
}

// NewScriptingCommand creates a new scripting command.
func NewScriptingCommand(cfg *config.Config) *ScriptingCommand {
	return &ScriptingCommand{
		BaseCommand: NewBaseCommand(
			"script",
			"Execute JavaScript scripts with deferred/declarative API",
			"script [options] [script-file]",
		),
		scriptCommandBase: scriptCommandBase{
			config:   cfg,
			logLevel: "info", // Default log level - SetupFlags may override this
		},
		// No default engineFactory - Execute() will create the correct one with session/storage params
		terminalFactory: func(ctx context.Context, engine *scripting.Engine) terminalRunner {
			return scripting.NewTerminal(ctx, engine)
		},
	}
}

// SetupFlags configures the flags for the scripting command.
func (c *ScriptingCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", false, "Start interactive scripting terminal")
	fs.BoolVar(&c.interactive, "i", false, "Start interactive scripting terminal (short form)")
	fs.StringVar(&c.script, "script", "", "JavaScript code to execute directly")
	fs.StringVar(&c.script, "e", "", "JavaScript code to execute directly (short form)")
	c.RegisterFlags(fs)
}

// Execute runs the scripting command.
func (c *ScriptingCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Handle "paths" subcommand: show annotated discovery paths
	if len(args) > 0 && args[0] == "paths" {
		if len(args) > 1 {
			_, _ = fmt.Fprintf(stderr, "unexpected arguments with paths: %v\n", args[1:])
			return &SilentError{Err: ErrUnexpectedArguments}
		}
		return c.showScriptPaths(stdout, stderr)
	}

	// Create execution context. Use injected factory if available (for tests),
	// otherwise use signal.NotifyContext for proper signal handling.
	var ctx context.Context
	var cancel context.CancelFunc
	if c.ctxFactory != nil {
		ctx, cancel = c.ctxFactory()
	} else {
		// Production: cancel on interrupt signals (SIGINT, SIGTERM)
		ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
	defer cancel()

	// Create scripting engine — use injected factory (tests) or shared base (production).
	var engine *scripting.Engine
	var cleanup func()
	if c.engineFactory != nil {
		var err error
		engine, err = c.engineFactory(ctx, stdout, stderr)
		if err != nil {
			return fmt.Errorf("failed to create scripting engine: %w", err)
		}
		cleanup = func() { engine.Close() }
		// Manual setup since PrepareEngine wasn't used.
		if c.testMode {
			engine.SetTestMode(true)
		}
		injectConfigHotSnippets(engine, c.config)
	} else {
		var err error
		engine, cleanup, err = c.PrepareEngine(ctx, stdout, stderr)
		if err != nil {
			return err
		}
	}
	defer cleanup()

	// Set global default logger
	// Note: We access the internal logger getter. This is the "modular wiring" part -
	// the engine provides the logger, and the command (entrypoint logic) wires it up.
	slog.SetDefault(engine.Logger())

	// Set up global variables
	// Convention: osm script [options] <script-file> [script-args...]
	// The first positional argument is the script file. All subsequent
	// positional arguments are passed to the script as the 'args' global.
	var scriptFile string
	var scriptArgs []string
	if len(args) > 0 {
		scriptFile = args[0]
		scriptArgs = args[1:]
	}
	engine.SetGlobal("args", scriptArgs)

	// PHASE 1: Configuration - Evaluate scripts to define modes and commands.
	// Load the script file passed as the first argument
	if scriptFile != "" {
		// Resolve script path
		resolvedPath := scriptFile
		if !filepath.IsAbs(resolvedPath) {
			locations := []string{
				resolvedPath,
				filepath.Join("scripts", resolvedPath),
			}
			var found bool
			for _, path := range locations {
				if _, err := os.Stat(path); err == nil {
					resolvedPath = path
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("script file not found: %s", scriptFile)
			}
		}

		// Load and execute the script
		scriptName := filepath.Base(resolvedPath)
		script, err := engine.LoadScript(scriptName, resolvedPath)
		if err != nil {
			return fmt.Errorf("failed to load script %s: %w", resolvedPath, err)
		}
		if err := engine.ExecuteScript(script); err != nil {
			return fmt.Errorf("failed to evaluate script %s: %w", resolvedPath, err)
		}
	}

	// Execute script from -e flag AFTER file scripts.
	if c.script != "" {
		script := engine.LoadScriptFromString("command-line", c.script)
		if err := engine.ExecuteScript(script); err != nil {
			return err
		}
	}

	// PHASE 2: Execution - If interactive, run the TUI with the configured state.
	if c.interactive {
		// Apply prompt color overrides from config if present
		if c.config != nil {
			colorMap := make(map[string]string)
			for k, v := range c.config.Global {
				if key, ok := strings.CutPrefix(k, "prompt.color."); ok && key != "" {
					colorMap[key] = v
				}
			}
			if len(colorMap) > 0 {
				engine.GetTUIManager().SetDefaultColorsFromStrings(colorMap)
			}
		}
		terminalFactory := c.terminalFactory
		if terminalFactory == nil {
			terminalFactory = func(ctx context.Context, engine *scripting.Engine) terminalRunner {
				return scripting.NewTerminal(ctx, engine)
			}
		}
		terminal := terminalFactory(ctx, engine)
		terminal.Run()
		return nil
	}

	// If not interactive and no scripts were provided, it's an error.
	if scriptFile == "" && c.script == "" {
		_, _ = fmt.Fprintln(stderr, "No script file specified. Use -i for interactive mode, -e for direct execution, or provide a script file.")
		return &SilentError{Err: fmt.Errorf("no script specified")}
	}

	return nil
}

// showScriptPaths displays annotated script discovery paths with source and existence status.
func (c *ScriptingCommand) showScriptPaths(stdout, stderr io.Writer) error {
	discovery := NewScriptDiscovery(c.config)
	paths := discovery.DiscoverAnnotatedScriptPaths()

	if len(paths) == 0 {
		_, _ = fmt.Fprintln(stdout, "No script paths discovered.")
		return nil
	}

	_, _ = fmt.Fprintln(stdout, "Script Discovery Paths:")
	_, _ = fmt.Fprintln(stdout)

	var missingCustom int
	for _, ap := range paths {
		status := "✓"
		if !ap.Exists {
			status = "✗"
			if ap.Source == "custom" {
				missingCustom++
			}
		}
		_, _ = fmt.Fprintf(stdout, "  %s %-14s %s\n", status, "["+ap.Source+"]", ap.Path)
	}

	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintf(stdout, "%d path(s) total\n", len(paths))

	// Emit config validation warnings for missing custom paths
	if missingCustom > 0 {
		_, _ = fmt.Fprintln(stderr)
		_, _ = fmt.Fprintf(stderr, "Warning: %d configured script path(s) do not exist on disk.\n", missingCustom)
		_, _ = fmt.Fprintln(stderr, "Check the script.paths option in your config file.")
	}

	return nil
}
