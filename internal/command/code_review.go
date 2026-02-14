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

//go:embed code_review_template.md
var codeReviewTemplate string

//go:embed code_review_script.js
var codeReviewScript string

// CodeReviewCommand provides the baked-in code-review script functionality.
type CodeReviewCommand struct {
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

// NewCodeReviewCommand creates a new code-review command.
func NewCodeReviewCommand(cfg *config.Config) *CodeReviewCommand {
	return &CodeReviewCommand{
		BaseCommand: NewBaseCommand(
			"code-review",
			"Single-prompt code review with context",
			"code-review [options]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the code-review command.
func (c *CodeReviewCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive code review mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive code review mode (short form, default)")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
	fs.StringVar(&c.session, "session", "", "Session ID for state persistence (overrides auto-discovery)")
	fs.StringVar(&c.store, "store", "", "Storage backend to use: 'fs' (default) or 'memory'")
	fs.StringVar(&c.logLevel, "log-level", "info", "Log level (debug, info, warn, error)")
	fs.StringVar(&c.logPath, "log-file", "", "Path to log file (JSON output)")
	fs.IntVar(&c.logBufferSize, "log-buffer", 1000, "Size of in-memory log buffer")
}

// Execute runs the code-review command.
func (c *CodeReviewCommand) Execute(args []string, stdout, stderr io.Writer) error {
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
	const commandName = "code-review"
	engine.SetGlobal("config", map[string]interface{}{
		"name": commandName,
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("codeReviewTemplate", codeReviewTemplate)

	// Expose diff splitter to JS for chunked code reviews.
	engine.SetGlobal("defaultMaxDiffLines", DefaultMaxDiffLines)
	engine.SetGlobal("splitDiff", func(diff string, maxLines int) []map[string]interface{} {
		chunks := SplitDiff(diff, maxLines)
		result := make([]map[string]interface{}, len(chunks))
		for i, c := range chunks {
			result[i] = map[string]interface{}{
				"index":   c.Index,
				"total":   c.Total,
				"files":   c.Files,
				"content": c.Content,
				"lines":   c.Lines,
			}
		}
		return result
	})

	// Load the embedded script
	script := engine.LoadScriptFromString("code-review", codeReviewScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute code-review script: %w", err)
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
