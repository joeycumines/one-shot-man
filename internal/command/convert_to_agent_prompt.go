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

//go:embed convert_to_agent_prompt_template.md
var convertToAgentPromptTemplate string

//go:embed convert_to_agent_prompt_script.js
var convertToAgentPromptScript string

// ConvertToAgentPromptCommand provides the baked-in convert-to-agent-prompt script functionality.
type ConvertToAgentPromptCommand struct {
	*BaseCommand
	interactive bool
	testMode    bool
	config      *config.Config
}

// NewConvertToAgentPromptCommand creates a new convert-to-agent-prompt command.
func NewConvertToAgentPromptCommand(cfg *config.Config) *ConvertToAgentPromptCommand {
	return &ConvertToAgentPromptCommand{
		BaseCommand: NewBaseCommand(
			"convert-to-agent-prompt",
			"Convert goal/context to structured agentic AI prompt",
			"convert-to-agent-prompt [options]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the convert-to-agent-prompt command.
func (c *ConvertToAgentPromptCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive agent prompt converter mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive agent prompt converter mode (short form, default)")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
}

// Execute runs the convert-to-agent-prompt command.
func (c *ConvertToAgentPromptCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Create scripting engine
	engine := scripting.NewEngine(ctx, stdout, stderr)
	defer engine.Close()

	if c.testMode {
		engine.SetTestMode(true)
	}

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("convertToAgentPromptTemplate", convertToAgentPromptTemplate)

	// Load the embedded script
	script := engine.LoadScriptFromString("convert-to-agent-prompt", convertToAgentPromptScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute convert-to-agent-prompt script: %w", err)
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