package command

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

//go:embed pr_split_template.md
var prSplitTemplate string

//go:embed pr_split_script.js
var prSplitScript string

// PrSplitCommand splits a large PR into reviewable stacked branches.
// Supports both heuristic grouping strategies and AI-powered classification
// via claudemux (Claude Code / Ollama).
type PrSplitCommand struct {
	*BaseCommand
	scriptCommandBase
	interactive bool

	// Split configuration flags
	baseBranch    string
	strategy      string
	maxFiles      int
	branchPrefix  string
	verifyCommand string
	dryRun        bool

	// AI mode flags
	aiMode   bool
	provider string
	model    string

	// JSON output flag
	jsonOutput bool
}

// NewPrSplitCommand creates a new pr-split command.
func NewPrSplitCommand(cfg *config.Config) *PrSplitCommand {
	return &PrSplitCommand{
		BaseCommand: NewBaseCommand(
			"pr-split",
			"Split a large PR into reviewable stacked branches",
			"pr-split [options]",
		),
		scriptCommandBase: scriptCommandBase{config: cfg},
	}
}

// SetupFlags configures the flags for the pr-split command.
func (c *PrSplitCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "interactive", true, "Start interactive mode (default)")
	fs.BoolVar(&c.interactive, "i", true, "Start interactive mode (short form)")

	// Split configuration
	fs.StringVar(&c.baseBranch, "base", "main", "Base branch to split against")
	fs.StringVar(&c.strategy, "strategy", "directory", "Grouping strategy: directory, directory-deep, extension, chunks, auto")
	fs.IntVar(&c.maxFiles, "max", 10, "Maximum files per split")
	fs.StringVar(&c.branchPrefix, "prefix", "split/", "Branch name prefix for splits")
	fs.StringVar(&c.verifyCommand, "verify", "make test", "Command to verify each split")
	fs.BoolVar(&c.dryRun, "dry-run", false, "Show plan without executing")

	// AI mode
	fs.BoolVar(&c.aiMode, "ai", false, "Use Claude Code for intelligent classification and planning")
	fs.StringVar(&c.provider, "provider", "ollama", "AI provider: ollama, claude-code")
	fs.StringVar(&c.model, "model", "", "Model identifier for AI provider")
	fs.BoolVar(&c.jsonOutput, "json", false, "Output results as JSON (combine with run or --dry-run)")

	c.RegisterFlags(fs)
}

// Execute runs the pr-split command.
func (c *PrSplitCommand) Execute(args []string, stdout, stderr io.Writer) error {
	ctx := context.Background()

	// Apply config defaults — flags override config values. Config keys
	// are namespaced under the "pr-split" command section or global:
	//   pr-split.base=develop
	//   pr-split.strategy=extension
	//   pr-split.max=8
	//   pr-split.prefix=split/
	//   pr-split.verify=make test
	//   pr-split.dry-run=true
	//   pr-split.ai=true
	//   pr-split.provider=claude-code
	//   pr-split.model=sonnet
	if c.config != nil {
		applyConfigDefault := func(key string, target *string, flagDefault string) {
			if v, ok := c.config.GetCommandOption("pr-split", key); ok && (*target == flagDefault || *target == "") {
				*target = v
			}
		}
		applyConfigDefault("base", &c.baseBranch, "main")
		applyConfigDefault("strategy", &c.strategy, "directory")
		if v, ok := c.config.GetCommandOption("pr-split", "max"); ok && (c.maxFiles == 10 || c.maxFiles == 0) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				c.maxFiles = n
			}
		}
		applyConfigDefault("prefix", &c.branchPrefix, "split/")
		applyConfigDefault("verify", &c.verifyCommand, "make test")
		if v, ok := c.config.GetCommandOption("pr-split", "dry-run"); ok && !c.dryRun {
			c.dryRun = v == "true" || v == "1" || v == "yes"
		}
		if v, ok := c.config.GetCommandOption("pr-split", "ai"); ok && !c.aiMode {
			c.aiMode = v == "true" || v == "1" || v == "yes"
		}
		applyConfigDefault("provider", &c.provider, "ollama")
		if v, ok := c.config.GetCommandOption("pr-split", "model"); ok && c.model == "" {
			c.model = v
		}
	}

	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	// Inject command name for state namespacing
	const commandName = "pr-split"
	engine.SetGlobal("config", map[string]interface{}{
		"name": commandName,
	})

	// Set up global variables
	engine.SetGlobal("args", args)
	engine.SetGlobal("prSplitTemplate", prSplitTemplate)

	// Expose split configuration to JS
	engine.SetGlobal("prSplitConfig", map[string]interface{}{
		"baseBranch":    c.baseBranch,
		"strategy":      c.strategy,
		"maxFiles":      c.maxFiles,
		"branchPrefix":  c.branchPrefix,
		"verifyCommand": c.verifyCommand,
		"dryRun":        c.dryRun,
		"aiMode":        c.aiMode,
		"provider":      c.provider,
		"model":         c.model,
		"jsonOutput":    c.jsonOutput,
	})

	// Load the embedded script
	script := engine.LoadScriptFromString("pr-split", prSplitScript)
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute pr-split script: %w", err)
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
