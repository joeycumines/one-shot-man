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

//go:embed super_document_template.md
var superDocumentTemplate string

//go:embed super_document_script.js
var superDocumentScript string

// SuperDocumentCommand provides the super-document TUI for document merging.
type SuperDocumentCommand struct {
	*BaseCommand
	interactive bool
	testMode    bool
	config      *config.Config
	session     string
	store       string
}

// NewSuperDocumentCommand creates a new super-document command.
func NewSuperDocumentCommand(cfg *config.Config) *SuperDocumentCommand {
	return &SuperDocumentCommand{
		BaseCommand: NewBaseCommand(
			"super-document",
			"TUI for merging documents into a single internally consistent super-document",
			"super-document [options]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the super-document command.
func (c *SuperDocumentCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive TUI mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive TUI mode (short form, default)")
	fs.BoolVar(&c.testMode, "test", false, "Enable test mode with verbose output")
	fs.StringVar(&c.session, "session", "", "Session ID for state persistence (overrides auto-discovery)")
	fs.StringVar(&c.store, "store", "", "Storage backend to use: 'fs' (default) or 'memory'")
}

// Execute runs the super-document command.
func (c *SuperDocumentCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Create scripting engine with explicit session/storage configuration
	engine, err := scripting.NewEngineWithConfig(ctx, stdout, stderr, c.session, c.store)
	if err != nil {
		return fmt.Errorf("failed to create scripting engine: %w", err)
	}
	defer engine.Close()

	if c.testMode {
		engine.SetTestMode(true)
	}

	// Inject command name for state namespacing
	const commandName = "super-document"
	engine.SetGlobal("config", map[string]interface{}{
		"name": commandName,
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("superDocumentTemplate", superDocumentTemplate)

	// Load the embedded script
	script := engine.LoadScriptFromString("super-document", superDocumentScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute super-document script: %w", err)
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
