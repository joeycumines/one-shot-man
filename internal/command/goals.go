package command

import (
	"context"
	_ "embed"
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

// Embedded goal scripts
//
//go:embed goals/comment_stripper.js
var commentStripperGoal string

//go:embed goals/doc_generator.js
var docGeneratorGoal string

//go:embed goals/test_generator.js
var testGeneratorGoal string

//go:embed goals/commit_message_generator.js
var commitMessageGeneratorGoal string

// Goal represents a pre-written goal with metadata
type Goal struct {
	Name        string
	Description string
	Category    string
	Usage       string
	Script      string
	FileName    string
}

// GoalsCommand provides access to pre-written goals
type GoalsCommand struct {
	*BaseCommand
	interactive bool
	list        bool
	category    string
	run         string
	config      *config.Config
	// testMode prevents launching the interactive TUI during tests while
	// still executing JS (so onEnter hooks can print to stdout).
	testMode bool
}

// NewGoalsCommand creates a new goals command
func NewGoalsCommand(cfg *config.Config) *GoalsCommand {
	return &GoalsCommand{
		BaseCommand: &BaseCommand{
			name:        "goals",
			description: "Access pre-written goals for common development tasks",
			usage:       "goals [options] [goal-name]",
		},
		config: cfg,
	}
}

// SetupFlags configures the flags for the goals command
func (c *GoalsCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.interactive, "i", false, "Run goal in interactive mode")
	fs.BoolVar(&c.list, "l", false, "List available goals")
	fs.StringVar(&c.category, "c", "", "Filter goals by category")
	fs.StringVar(&c.run, "r", "", "Run specific goal directly")
}

// Execute runs the goals command
func (c *GoalsCommand) Execute(args []string, stdout, stderr io.Writer) error {
	goals := c.getAvailableGoals()

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

	// Resolve goal
	var selectedGoal *Goal
	for i := range goals {
		if goals[i].Name == goalName {
			selectedGoal = &goals[i]
			break
		}
	}
	if selectedGoal == nil {
		_, _ = fmt.Fprintf(stderr, "Goal '%s' not found. Use 'osm goals -l' to list available goals.\n", goalName)
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

	// Create a script object for the goal
	script := &scripting.Script{
		Name:        selectedGoal.Name,
		Path:        filepath.Join("goals", selectedGoal.FileName),
		Content:     selectedGoal.Script,
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

// getAvailableGoals returns all available pre-written goals
func (c *GoalsCommand) getAvailableGoals() []Goal {
	return []Goal{
		{
			Name:        "comment-stripper",
			Description: "Remove useless comments and refactor useful ones",
			Category:    "code-refactoring",
			Usage:       "Analyzes code files and removes redundant comments while preserving valuable documentation",
			Script:      commentStripperGoal,
			FileName:    "comment_stripper.js",
		},
		{
			Name:        "doc-generator",
			Description: "Generate comprehensive documentation for code structures",
			Category:    "documentation",
			Usage:       "Analyzes code and generates detailed documentation including API docs, examples, and usage guides",
			Script:      docGeneratorGoal,
			FileName:    "doc_generator.js",
		},
		{
			Name:        "test-generator",
			Description: "Generate comprehensive test suites for existing code",
			Category:    "testing",
			Usage:       "Analyzes code and generates unit tests, integration tests, and edge case coverage",
			Script:      testGeneratorGoal,
			FileName:    "test_generator.js",
		},
		{
			Name:        "commit-message-generator",
			Description: "Generate Kubernetes-style commit messages from diffs and context",
			Category:    "git-workflow",
			Usage:       "Generates commit messages following Kubernetes semantic guidelines from git diffs and additional context",
			Script:      commitMessageGeneratorGoal,
			FileName:    "commit_message_generator.js",
		},
	}
}

// listGoals displays available goals
func (c *GoalsCommand) listGoals(goals []Goal, stdout io.Writer) error {
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
	_, _ = fmt.Fprintf(stdout, "  osm goals <goal-name>           Run a goal interactively\n")
	_, _ = fmt.Fprintf(stdout, "  osm goals -r <goal-name>        Run a goal directly\n")
	_, _ = fmt.Fprintf(stdout, "  osm goals -c <category>         List goals by category\n")
	_, _ = fmt.Fprintf(stdout, "  osm script goals/<goal>.js      Run goal as regular script\n")

	return nil
}
