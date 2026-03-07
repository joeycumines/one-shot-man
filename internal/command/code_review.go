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
	scriptCommandBase
	interactive bool
}

// NewCodeReviewCommand creates a new code-review command.
func NewCodeReviewCommand(cfg *config.Config) *CodeReviewCommand {
	return &CodeReviewCommand{
		BaseCommand: NewBaseCommand(
			"code-review",
			"Single-prompt code review with context",
			"code-review [options]",
		),
		scriptCommandBase: scriptCommandBase{config: cfg},
	}
}

// SetupFlags configures the flags for the code-review command.
func (c *CodeReviewCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive code review mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive code review mode (short form, default)")
	c.RegisterFlags(fs)
}

// Execute runs the code-review command.
func (c *CodeReviewCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	// Inject command name for state namespacing
	const commandName = "code-review"
	engine.SetGlobal("config", map[string]any{
		"name": commandName,
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("codeReviewTemplate", codeReviewTemplate)

	// Expose diff splitter to JS for chunked code reviews.
	engine.SetGlobal("defaultMaxDiffLines", DefaultMaxDiffLines)
	engine.SetGlobal("splitDiff", func(diff string, maxLines int) []map[string]any {
		chunks := SplitDiff(diff, maxLines)
		result := make([]map[string]any, len(chunks))
		for i, c := range chunks {
			result[i] = map[string]any{
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
	}

	return nil
}
