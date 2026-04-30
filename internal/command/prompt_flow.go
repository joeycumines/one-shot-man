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
	scriptCommandBase
	interactive bool
}

// NewPromptFlowCommand creates a new prompt-flow command.
func NewPromptFlowCommand(cfg *config.Config) *PromptFlowCommand {
	return &PromptFlowCommand{
		BaseCommand: NewBaseCommand(
			"prompt-flow",
			"Interactive prompt builder: goal/context/template -> generate -> assemble",
			"prompt-flow [options]",
		),
		scriptCommandBase: scriptCommandBase{config: cfg},
	}
}

// SetupFlags configures the flags for the prompt-flow command.
func (c *PromptFlowCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive prompt flow mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive prompt flow mode (short form, default)")
	c.RegisterFlags(fs)
}

// Execute runs the prompt-flow command.
func (c *PromptFlowCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	// Inject command name for state namespacing
	const commandName = "prompt-flow"
	engine.SetGlobal("config", map[string]any{
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
				if key, ok := strings.CutPrefix(k, "prompt.color."); ok && key != "" {
					colorMap[key] = v
				}
			}
			if len(colorMap) > 0 {
				engine.GetTUIManager().SetDefaultColorsFromStrings(colorMap)
			}
		}
		terminal := scripting.NewTerminal(ctx, engine)
		terminal.Run()
		return nil
	}

	// Wait for any asynchronous work (timers, fetch, etc.) to complete naturally.
	// This uses the WithAutoExit(true) feature of the event loop.
	engine.Wait()

	return nil
}
