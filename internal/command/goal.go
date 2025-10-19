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
	Name        string `json:"Name"`
	Description string `json:"Description"`
	Category    string `json:"Category"`
	Usage       string `json:"Usage"`

	// Script execution (not serialized to JS)
	Script   string `json:"-"`
	FileName string `json:"-"`

	// TUI configuration
	TUITitle      string `json:"TUITitle"`
	TUIPrompt     string `json:"TUIPrompt"`
	HistoryFile   string `json:"HistoryFile"`
	EnableHistory bool   `json:"EnableHistory"`

	// State management
	StateKeys map[string]interface{} `json:"StateKeys"` // Initial state values

	// Prompt building
	PromptInstructions string                 `json:"PromptInstructions"` // Main goal instructions
	PromptTemplate     string                 `json:"PromptTemplate"`     // Template for final prompt
	PromptOptions      map[string]interface{} `json:"PromptOptions"`      // Additional options for prompt building
	ContextHeader      string                 `json:"ContextHeader"`      // Header for context section

	// UI text
	BannerText string `json:"BannerText"`
	HelpText   string `json:"HelpText"`

	// Commands configuration
	Commands []CommandConfig `json:"Commands"`
}

// CommandConfig defines a command available in a goal mode
type CommandConfig struct {
	Name          string   `json:"Name"`
	Type          string   `json:"Type"` // "contextManager", "custom", "help"
	Description   string   `json:"Description,omitempty"`
	Usage         string   `json:"Usage,omitempty"`
	ArgCompleters []string `json:"ArgCompleters,omitempty"`
	Handler       string   `json:"Handler,omitempty"` // JavaScript handler code for custom commands
}

// GoalCommand provides access to pre-written goals
type GoalCommand struct {
	*BaseCommand
	interactive bool
	list        bool
	category    string
	run         string
	config      *config.Config
	registry    GoalRegistry
	// testMode prevents launching the interactive TUI during tests while
	// still executing JS (so onEnter hooks can print to stdout).
	testMode bool
}

// NewGoalCommand creates a new goal command
func NewGoalCommand(cfg *config.Config, registry GoalRegistry) *GoalCommand {
	return &GoalCommand{
		BaseCommand: &BaseCommand{
			name:        "goal",
			description: "Access pre-written goals for common development tasks",
			usage:       "goal [options] [goal-name]",
		},
		config:   cfg,
		registry: registry,
	}
}

// SetupFlags configures the flags for the goal command
func (c *GoalCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "i", false, "Run goal in interactive mode")
	fs.BoolVar(&c.list, "l", false, "List available goals")
	fs.StringVar(&c.category, "c", "", "Filter goals by category")
	fs.StringVar(&c.run, "r", "", "Run specific goal directly")
}

// Execute runs the goal command
func (c *GoalCommand) Execute(args []string, stdout, stderr io.Writer) error {
	goals := c.registry.GetAllGoals()

	if c.list {
		return c.listGoals(goals, stdout)
	}

	// Determine which goal to run and whether we should be interactive.
	var goalName string
	shouldInteractive := false
	switch {
	case c.run != "":
		goalName = c.run
		// -r implies non-interactive by default, unless -i explicitly set
		shouldInteractive = c.interactive
	case len(args) > 0:
		goalName = args[0]
		// Positional goal defaults to interactive, per README
		shouldInteractive = true || c.interactive
		if c.interactive { // keep explicit flag honored (no-op here but clearer intent)
			shouldInteractive = true
		}
	default:
		return c.listGoals(goals, stdout)
	}

	// Resolve goal from registry
	selectedGoal, err := c.registry.Get(goalName)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Goal '%s' not found. Use 'osm goal -l' to list available goals.\n", goalName)
		return fmt.Errorf("goal not found: %s", goalName)
	}

	// Create a scripting engine to run the goal
	ctx := context.Background()
	engine, err := scripting.NewEngine(ctx, stdout, stderr)
	if err != nil {
		return fmt.Errorf("failed to create scripting engine: %w", err)
	}
	defer engine.Close()

	if c.testMode {
		engine.SetTestMode(true)
	}

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
			_, _ = fmt.Fprintf(stdout, "  %-20s %s\n", goal.Name, goal.Description)
		}
		_, _ = fmt.Fprintf(stdout, "\n")
	}

	_, _ = fmt.Fprintf(stdout, "Usage:\n")
	_, _ = fmt.Fprintf(stdout, "  osm goal <goal-name>           Run a goal interactively\n")
	_, _ = fmt.Fprintf(stdout, "  osm goal -r <goal-name>        Run a goal directly\n")
	_, _ = fmt.Fprintf(stdout, "  osm goal -c <category>         List goals by category\n")

	return nil
}
