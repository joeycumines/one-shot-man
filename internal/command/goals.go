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

	if c.run != "" {
		return c.runGoal(c.run, goals, stdout, stderr)
	}

	if len(args) == 0 {
		return c.listGoals(goals, stdout)
	}

	goalName := args[0]
	return c.runGoal(goalName, goals, stdout, stderr)
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
		_, _ = fmt.Fprintf(stdout, "%s:\n", strings.Title(strings.ReplaceAll(category, "-", " ")))
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

// runGoal executes a specific goal
func (c *GoalsCommand) runGoal(goalName string, goals []Goal, stdout, stderr io.Writer) error {
	var selectedGoal *Goal
	for _, goal := range goals {
		if goal.Name == goalName {
			selectedGoal = &goal
			break
		}
	}

	if selectedGoal == nil {
		_, _ = fmt.Fprintf(stderr, "Goal '%s' not found. Use 'osm goals -l' to list available goals.\n", goalName)
		return fmt.Errorf("goal not found: %s", goalName)
	}

	// Create a scripting engine to run the goal
	ctx := context.Background()
	engine := scripting.NewEngine(ctx, stdout, stderr)

	// Create a script object for the goal
	script := &scripting.Script{
		Name:        selectedGoal.Name,
		Path:        filepath.Join("goals", selectedGoal.FileName),
		Content:     selectedGoal.Script,
		Description: selectedGoal.Description,
	}

	// Execute the goal script
	if err := engine.ExecuteScript(script); err != nil {
		return fmt.Errorf("failed to execute goal '%s': %w", goalName, err)
	}

	return nil
}