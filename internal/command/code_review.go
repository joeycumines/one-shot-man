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
	interactive    bool
	testMode       bool
	config         *config.Config
	session        string
	storageBackend string
}

// NewCodeReviewCommand creates a new code-review command.
func NewCodeReviewCommand(cfg *config.Config) *CodeReviewCommand {
	return &CodeReviewCommand{
		BaseCommand: NewBaseCommand(
			"code-review",
			"Single-prompt code review with context: context -> generate prompt for PR review",
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
	fs.StringVar(&c.storageBackend, "storage-backend", "", "Storage backend to use: 'fs' (default) or 'memory')")
}

// Execute runs the code-review command.
func (c *CodeReviewCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Create scripting engine with explicit session/storage configuration
	engine, err := scripting.NewEngineWithConfig(ctx, stdout, stderr, c.session, c.storageBackend)
	if err != nil {
		return fmt.Errorf("failed to create scripting engine: %w", err)
	}
	defer engine.Close()

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
