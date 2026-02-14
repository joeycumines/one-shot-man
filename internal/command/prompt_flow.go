package command

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

//go:embed prompt_flow_template.md
var promptFlowTemplate string

//go:embed prompt_flow_script.js
var promptFlowScript string

// PromptFlowCommand provides the baked-in prompt-flow script functionality.
type PromptFlowCommand struct {
	*BaseCommand
	interactive   bool
	testMode      bool
	config        *config.Config
	session       string
	store         string
	logLevel      string
	logPath       string
	logBufferSize int
}

// NewPromptFlowCommand creates a new prompt-flow command.
func NewPromptFlowCommand(cfg *config.Config) *PromptFlowCommand {
	return &PromptFlowCommand{
		BaseCommand: NewBaseCommand(
			"prompt-flow",
			"Interactive prompt builder: goal/context/template -> generate -> assemble",
			"prompt-flow [options]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the prompt-flow command.
func (c *PromptFlowCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive prompt flow mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive prompt flow mode (short form, default)")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
	fs.StringVar(&c.session, "session", "", "Session ID for state persistence (overrides auto-discovery)")
	fs.StringVar(&c.store, "store", "", "Storage backend to use: 'fs' (default) or 'memory'")
	fs.StringVar(&c.logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	fs.StringVar(&c.logPath, "log-file", "", "Path to log file (JSON output)")
	fs.IntVar(&c.logBufferSize, "log-buffer", 1000, "Size of in-memory log buffer")
}

// Execute runs the prompt-flow command.
func (c *PromptFlowCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Resolve logging configuration via config + flags.
	lc, err := resolveLogConfig(c.logPath, c.logLevel, c.logBufferSize, c.config)
	if err != nil {
		return err
	}
	if lc.logFile != nil {
		defer lc.logFile.Close()
	}

	// Create scripting engine with explicit session/storage and logging configuration
	engine, err := scripting.NewEngineDetailed(ctx, stdout, stderr, c.session, c.store, lc.logFile, lc.bufferSize, lc.level, modulePathOpts(c.config)...)
	if err != nil {
		return fmt.Errorf("failed to create scripting engine: %w", err)
	}
	defer engine.Close()

	// Start background session cleanup if enabled in config.
	stopCleanup := maybeStartCleanupScheduler(c.config, c.session)
	defer stopCleanup()

	if c.testMode {
		engine.SetTestMode(true)
	}

	// Inject command name for state namespacing
	const commandName = "prompt-flow"
	engine.SetGlobal("config", map[string]interface{}{
		"name": commandName,
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("promptFlowTemplate", promptFlowTemplate)

	// Load the embedded script
	script := engine.LoadScriptFromString("prompt-flow", promptFlowScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute prompt-flow script: %w", err)
	}

	// Only run interactive mode if requested and not in test mode
	if c.interactive && !c.testMode {
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
		terminal := scripting.NewTerminal(ctx, engine)
		terminal.Run()
	}

	return nil
}
