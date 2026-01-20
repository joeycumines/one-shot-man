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
	interactive     bool
	script          string
	testMode        bool
	session         string
	store           string
	logPath         string
	logBufferSize   int
	logLevel        string
	config          *config.Config
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
		config: cfg,
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
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
	fs.StringVar(&c.session, "session", "", "Session ID for state persistence (overrides auto-discovery)")
	fs.StringVar(&c.store, "store", "", "Storage backend to use: 'fs' (default) or 'memory' (overrides OSM_STORE)")
	fs.StringVar(&c.logPath, "log-file", "", "Path to log file (JSON output)")
	fs.IntVar(&c.logBufferSize, "log-buffer", 1000, "Size of in-memory log buffer")
	fs.StringVar(&c.logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
}

// Execute runs the scripting command.
func (c *ScriptingCommand) Execute(args []string, stdout, stderr io.Writer) error {
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

	// Parse log level
	var level slog.Level
	switch strings.ToLower(c.logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %s", c.logLevel)
	}

	// Prepare logging configuration
	var logFile io.Writer
	if c.logPath != "" {
		f, err := os.OpenFile(c.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %w", c.logPath, err)
		}
		defer f.Close()
		logFile = f
	}

	// Create scripting engine with explicit session configuration (no globals!)
	engineFactory := c.engineFactory
	if engineFactory == nil {
		// Use the new API with explicit parameters to avoid data races
		engineFactory = func(ctx context.Context, stdout, stderr io.Writer) (*scripting.Engine, error) {
			return scripting.NewEngineDetailed(ctx, stdout, stderr, c.session, c.store, logFile, c.logBufferSize, level)
		}
	}

	engine, err := engineFactory(ctx, stdout, stderr)
	if err != nil {
		return fmt.Errorf("failed to create scripting engine: %w", err)
	}
	defer engine.Close()

	// Set global default logger
	// Note: We access the internal logger getter. This is the "modular wiring" part -
	// the engine provides the logger, and the command (entrypoint logic) wires it up.
	slog.SetDefault(engine.Logger())

	if c.testMode {
		engine.SetTestMode(true)
	}

	// Set up global variables
	engine.SetGlobal("args", args)

	// PHASE 1: Configuration - Evaluate all scripts to define modes and commands.
	// Load any script files passed as arguments
	if len(args) > 0 {
		for _, scriptFile := range args {
			// Resolve script path
			if !filepath.IsAbs(scriptFile) {
				locations := []string{
					scriptFile,
					filepath.Join("scripts", scriptFile),
				}
				var found bool
				for _, path := range locations {
					if _, err := os.Stat(path); err == nil {
						scriptFile = path
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("script file not found: %s", scriptFile)
				}
			}

			// Load and execute the script
			scriptName := filepath.Base(scriptFile)
			script, err := engine.LoadScript(scriptName, scriptFile)
			if err != nil {
				return fmt.Errorf("failed to load script %s: %w", scriptFile, err)
			}
			if err := engine.ExecuteScript(script); err != nil {
				return fmt.Errorf("failed to evaluate script %s: %w", scriptFile, err)
			}
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
				if strings.HasPrefix(k, "prompt.color.") {
					key := strings.TrimPrefix(k, "prompt.color.")
					if key != "" {
						colorMap[key] = v
					}
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
	if len(args) == 0 && c.script == "" {
		fmt.Fprintln(stderr, "No script file specified. Use -i for interactive mode, -e for direct execution, or provide a script file.")
		return fmt.Errorf("no script specified")
	}

	return nil
}
