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
	interactive bool
	testMode    bool
	config      *config.Config
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
}

// Execute runs the prompt-flow command.
func (c *PromptFlowCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Create scripting engine
	engine := scripting.NewEngine(ctx, stdout, stderr)
	defer engine.Close()

	if c.testMode {
		engine.SetTestMode(true)
	}

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
