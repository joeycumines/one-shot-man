package command

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// Embedded goal script - now a generic interpreter
//
//go:embed goal.js
var goalScript string

// Goal represents a pre-written goal with metadata and configuration.
// All goal-specific behavior is defined declaratively here, and the
// JavaScript runtime simply interprets this configuration.
type Goal struct {
	// Basic metadata
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Usage       string `json:"usage"`

	// Script execution (not serialized to JS)
	Script   string `json:"-"`
	FileName string `json:"-"`

	// TUI configuration
	TUITitle  string `json:"tuiTitle"`
	TUIPrompt string `json:"tuiPrompt"`

	// State management
	StateVars map[string]interface{} `json:"stateVars"` // Initial state values

	// Prompt building
	PromptInstructions string                 `json:"promptInstructions"` // Main goal instructions
	PromptTemplate     string                 `json:"promptTemplate"`     // Template for final prompt
	PromptFooter       string                 `json:"promptFooter"`       // Footer text appended after context (template-interpolated)
	PromptOptions      map[string]interface{} `json:"promptOptions"`      // Additional options for prompt building
	ContextHeader      string                 `json:"contextHeader"`      // Header for context section

	// UI text
	BannerTemplate string `json:"bannerTemplate"` // printed on entry to goal, overrides default
	UsageTemplate  string `json:"usageTemplate"`  // printed after the default help

	// Printed in the default banner
	NotableVariables []string `json:"notableVariables"`

	// Post-copy hint: if set, printed after successful copy
	PostCopyHint string `json:"postCopyHint,omitempty"`

	// Hot-snippets embedded in this goal. These are merged with
	// config-file hot-snippets; goal-defined ones take precedence.
	HotSnippets []GoalHotSnippet `json:"hotSnippets,omitempty"`

	// Commands configuration
	Commands []CommandConfig `json:"commands"`
}

// CommandFlagDef describes a flag available for a command, used for tab-completion.
type CommandFlagDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GoalHotSnippet describes an embedded hot-snippet for a goal.
// These are registered as hot-<name> commands in the goal's mode.
type GoalHotSnippet struct {
	Name        string `json:"name"`
	Text        string `json:"text"`
	Description string `json:"description,omitempty"`
}

// CommandConfig defines a command available in a goal mode
type CommandConfig struct {
	Name          string           `json:"name"`
	Type          string           `json:"type"` // "contextManager", "custom", "help"
	Description   string           `json:"description,omitempty"`
	Usage         string           `json:"usage,omitempty"`
	ArgCompleters []string         `json:"argCompleters,omitempty"`
	FlagDefs      []CommandFlagDef `json:"flagDefs,omitempty"`
	Handler       string           `json:"handler,omitempty"` // JavaScript handler code for custom commands
}

// GoalCommand provides access to pre-written goals
type GoalCommand struct {
	*BaseCommand
	scriptCommandBase
	interactive bool
	list        bool
	category    string
	run         string
	registry    GoalRegistry
}

// NewGoalCommand creates a new goal command
func NewGoalCommand(cfg *config.Config, registry GoalRegistry) *GoalCommand {
	return &GoalCommand{
		BaseCommand: &BaseCommand{
			name:        "goal",
			description: "Access pre-written goals for common development tasks",
			usage:       "goal [options] [goal-name]",
		},
		scriptCommandBase: scriptCommandBase{config: cfg},
		registry:          registry,
	}
}

// SetupFlags configures the flags for the goal command
func (c *GoalCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "i", false, "Run goal in interactive mode")
	fs.BoolVar(&c.list, "l", false, "List available goals")
	fs.StringVar(&c.category, "c", "", "Filter goals by category")
	fs.StringVar(&c.run, "r", "", "Run specific goal directly")
	c.RegisterFlags(fs)
}

// Execute runs the goal command
func (c *GoalCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Handle "paths" subcommand: show annotated discovery paths
	if len(args) > 0 && args[0] == "paths" {
		if len(args) > 1 {
			_, _ = fmt.Fprintf(stderr, "unexpected arguments with paths: %v\n", args[1:])
			return &SilentError{Err: ErrUnexpectedArguments}
		}
		return c.showGoalPaths(stdout, stderr)
	}

	goals := c.registry.GetAllGoals()

	if c.list {
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "unexpected arguments with -l: %v\n", args)
			return &SilentError{Err: ErrUnexpectedArguments}
		}
		return c.listGoals(goals, stdout)
	}

	// Determine which goal to run and whether we should be interactive.
	var goalName string
	shouldInteractive := false
	switch {
	case c.run != "":
		if len(args) > 0 {
			_, _ = fmt.Fprintf(stderr, "unexpected arguments with -r: %v\n", args)
			return &SilentError{Err: ErrUnexpectedArguments}
		}
		goalName = c.run
		// -r implies non-interactive by default, unless -i explicitly set
		shouldInteractive = c.interactive
	case len(args) > 0:
		if len(args) > 1 {
			_, _ = fmt.Fprintf(stderr, "unexpected arguments: %v\n", args[1:])
			return &SilentError{Err: ErrUnexpectedArguments}
		}
		goalName = args[0]
		// Positional goal defaults to interactive, per README
		shouldInteractive = true
	default:
		return c.listGoals(goals, stdout)
	}

	// Resolve goal from registry
	selectedGoal, err := c.registry.Get(goalName)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Goal '%s' not found. Use 'osm goal -l' to list available goals.\n", goalName)
		return &SilentError{Err: fmt.Errorf("goal not found: %s", goalName)}
	}

	// Create a scripting engine to run the goal (allow explicit session/backend)
	ctx := context.Background()

	engine, cleanup, err := c.PrepareEngine(ctx, stdout, stderr)
	if err != nil {
		return err
	}
	defer cleanup()

	// Marshal goal configuration to JSON for the JavaScript interpreter
	goalConfigJSON, err := json.Marshal(selectedGoal)
	if err != nil {
		return fmt.Errorf("failed to marshal goal configuration: %w", err)
	}

	// Create a script that passes the configuration to the generic interpreter
	scriptContent := fmt.Sprintf(`
// Goal configuration from Go
var GOAL_CONFIG = %s;

// Execute the generic goal interpreter
%s
`, goalConfigJSON, selectedGoal.Script)

	// Create a script object for the goal
	script := &scripting.Script{
		Name:        selectedGoal.Name,
		Path:        filepath.Join("goal", selectedGoal.FileName),
		Content:     scriptContent,
		Description: selectedGoal.Description,
	}

	// Execute the goal script to register modes/commands
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute goal '%s': %w", goalName, err)
	}

	// Launch interactive TUI if requested (and not in test mode)
	if shouldInteractive {
		// Switch to the goal's mode so onEnter runs and users land in the right place
		_ = engine.GetTUIManager().SwitchMode(selectedGoal.Name)

		if !c.testMode {
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
	}

	return nil
}

// listGoals displays available goals
func (c *GoalCommand) listGoals(goals []Goal, stdout io.Writer) error {
	filteredGoals := goals
	if c.category != "" {
		filteredGoals = []Goal{}
		for _, goal := range goals {
			if strings.EqualFold(goal.Category, c.category) {
				filteredGoals = append(filteredGoals, goal)
			}
		}
	}

	if len(filteredGoals) == 0 {
		if c.category != "" {
			_, _ = fmt.Fprintf(stdout, "No goals found for category: %s\n", c.category)
		} else {
			_, _ = fmt.Fprintf(stdout, "No goals available\n")
		}
		return nil
	}

	_, _ = fmt.Fprintf(stdout, "Available Goals:\n\n")

	// Group by category
	categories := make(map[string][]Goal)
	for _, goal := range filteredGoals {
		categories[goal.Category] = append(categories[goal.Category], goal)
	}

	// Sort categories
	var sortedCategories []string
	for category := range categories {
		sortedCategories = append(sortedCategories, category)
	}
	sort.Strings(sortedCategories)

	for _, category := range sortedCategories {
		_, _ = fmt.Fprintf(stdout, "%s:\n", cases.Title(language.Und).String(strings.ToLower(strings.ReplaceAll(category, "-", " "))))
		for _, goal := range categories[category] {
			_, _ = fmt.Fprintln(stdout, formatGoalLine(goal))
		}
		_, _ = fmt.Fprintf(stdout, "\n")
	}

	_, _ = fmt.Fprintf(stdout, "Usage:\n")
	_, _ = fmt.Fprintf(stdout, "  osm goal <goal-name>           Run a goal interactively\n")
	_, _ = fmt.Fprintf(stdout, "  osm goal -r <goal-name>        Run a goal directly\n")
	_, _ = fmt.Fprintf(stdout, "  osm goal -c <category>         List goals by category\n")

	return nil
}

// showGoalPaths displays annotated goal discovery paths with source and existence status.
func (c *GoalCommand) showGoalPaths(stdout, stderr io.Writer) error {
	discovery := NewGoalDiscovery(c.config)
	paths := discovery.DiscoverAnnotatedGoalPaths()

	if len(paths) == 0 {
		_, _ = fmt.Fprintln(stdout, "No goal paths discovered.")
		return nil
	}

	_, _ = fmt.Fprintln(stdout, "Goal Discovery Paths:")
	_, _ = fmt.Fprintln(stdout)

	var missingCustom int
	for _, ap := range paths {
		status := "✓"
		if !ap.Exists {
			status = "✗"
			if ap.Source == "custom" {
				missingCustom++
			}
		}
		_, _ = fmt.Fprintf(stdout, "  %s %-14s %s\n", status, "["+ap.Source+"]", ap.Path)
	}

	_, _ = fmt.Fprintln(stdout)
	_, _ = fmt.Fprintf(stdout, "%d path(s) total\n", len(paths))

	// Emit config validation warnings for missing custom paths
	if missingCustom > 0 {
		_, _ = fmt.Fprintln(stderr)
		_, _ = fmt.Fprintf(stderr, "Warning: %d configured goal path(s) do not exist on disk.\n", missingCustom)
		_, _ = fmt.Fprintln(stderr, "Check the goal.paths option in your config file.")
	}

	return nil
}

// formatGoalLine produces a single display line for a goal in the list output.
// Format: "  <name>  <description>  [vars: k=v, ...]  [cmds: a, b, ...]"
func formatGoalLine(goal Goal) string {
	line := fmt.Sprintf("  %-20s %s", goal.Name, goal.Description)

	// Summarize non-nil state variables with their default values.
	var varParts []string
	for key, val := range goal.StateVars {
		if val == nil {
			continue
		}
		s := fmt.Sprintf("%v", val)
		if s == "" {
			continue
		}
		const maxValLen = 30
		if len(s) > maxValLen {
			s = s[:maxValLen-3] + "..."
		}
		varParts = append(varParts, key+"="+s)
	}
	if len(varParts) > 0 {
		sort.Strings(varParts) // deterministic order
		line += "  [vars: " + strings.Join(varParts, ", ") + "]"
	}

	// Summarize custom commands.
	var customCmds []string
	for _, cmd := range goal.Commands {
		if cmd.Type == "custom" {
			customCmds = append(customCmds, cmd.Name)
		}
	}
	if len(customCmds) > 0 {
		line += "  [cmds: " + strings.Join(customCmds, ", ") + "]"
	}

	return line
}
