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
	// testMode prevents launching the interactive TUI during tests while
	// still executing JS (so onEnter hooks can print to stdout).
	testMode bool
}

// NewGoalCommand creates a new goal command
func NewGoalCommand(cfg *config.Config) *GoalCommand {
	return &GoalCommand{
		BaseCommand: &BaseCommand{
			name:        "goal",
			description: "Access pre-written goals for common development tasks",
			usage:       "goal [options] [goal-name]",
		},
		config: cfg,
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

// getAvailableGoals returns all available pre-written goals.
// All goal behavior is defined declaratively here.
func (c *GoalCommand) getAvailableGoals() []Goal {
	return []Goal{
		// Comment Stripper Goal
		{
			Name:        "comment-stripper",
			Description: "Remove useless comments and refactor useful ones",
			Category:    "code-refactoring",
			Usage:       "Analyzes code files and removes redundant comments while preserving valuable documentation",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:      "Comment Stripper",
			TUIPrompt:     "(comment-stripper) > ",
			HistoryFile:   ".comment-stripper_history",
			EnableHistory: true,

			StateKeys: map[string]interface{}{
				"contextItems": []interface{}{},
			},

			PromptInstructions: `Analyze the provided code and remove useless comments while refactoring useful ones according to these rules:

1. **Remove useless comments:**
   - Comments that merely repeat what the code does (e.g., "increment counter" for i++)
   - Outdated comments that no longer match the code
   - TODO/FIXME comments that are no longer relevant
   - Commented-out code that serves no purpose
   - Redundant header comments in simple functions

2. **Preserve and refactor useful comments:**
   - Business logic explanations and reasoning
   - Complex algorithm explanations
   - Performance considerations
   - Security implications
   - API documentation and usage examples
   - Configuration and setup instructions
   - Workaround explanations

3. **Refactoring guidelines:**
   - Move complex logic explanations closer to the relevant code
   - Convert inline comments to proper documentation where appropriate
   - Ensure remaining comments add genuine value
   - Fix grammar, spelling, and formatting of preserved comments
   - Update outdated but still relevant comments

4. **Output format:**
   - Show the cleaned code with explanations for each change
   - List removed comments with reasons for removal
   - List preserved/refactored comments with explanations
   - Provide a summary of the improvements made

Maintain all functionality and behavior of the original code while improving its readability and maintainability.`,

			PromptTemplate: `**{{.Description | upper}}**

{{.PromptInstructions}}

## {{.ContextHeader}}

{{.ContextTxtar}}`,

			ContextHeader: "CODE TO ANALYZE",

			BannerText: "Comment Stripper: Remove useless comments, refactor useful ones",
			HelpText:   "Commands: add, note, list, edit, remove, show, copy, run, help, exit",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:          "run",
					Type:          "custom",
					Description:   "Quick run - add files and show prompt",
					Usage:         "run [file ...]",
					ArgCompleters: []string{"file"},
					Handler: `function (args) {
						if (args.length === 0) {
							output.print("Usage: run [file ...]");
							return;
						}
						ctxmgr.commands.add.handler(args);
						output.print("\n" + buildPrompt());
					}`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Doc Generator Goal
		{
			Name:        "doc-generator",
			Description: "Generate comprehensive documentation for code structures",
			Category:    "documentation",
			Usage:       "Analyzes code and generates detailed documentation including API docs, examples, and usage guides",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:      "Code Documentation Generator",
			TUIPrompt:     "(doc-gen) > ",
			HistoryFile:   ".doc-generator_history",
			EnableHistory: true,

			StateKeys: map[string]interface{}{
				"contextItems": []interface{}{},
				"docType":      "comprehensive",
			},

			PromptInstructions: `Create {{.StateKeys.docType}} documentation for the provided code following these guidelines:

{{.DocTypeInstructions}}

**Documentation Standards:**
- Use clear, concise language appropriate for the target audience
- Include practical examples that users can copy and run
- Organize information logically with proper headings and structure
- Maintain consistency in formatting and style
- Ensure accuracy and completeness
- Include relevant links and cross-references

**Output Format:**
- Provide the documentation in appropriate format (Markdown, JSDoc, etc.)
- Include a brief explanation of the documentation structure
- Highlight key sections and important information
- Ensure the documentation is ready to use without further editing`,

			PromptTemplate: `**{{.Description | upper}}**

{{.PromptInstructions}}

## {{.ContextHeader}}

{{.ContextTxtar}}`,

			ContextHeader: "CODE TO DOCUMENT",

			PromptOptions: map[string]interface{}{
				"docTypeInstructions": map[string]string{
					"comprehensive": `Generate comprehensive documentation including:
- Overview and purpose
- Architecture and design decisions
- API documentation with examples
- Usage guides and tutorials
- Configuration options
- Troubleshooting guides
- Contributing guidelines`,
					"api": `Generate API documentation including:
- Function/method signatures with parameter descriptions
- Return value specifications
- Usage examples for each function
- Error handling information
- Type definitions and interfaces`,
					"readme": `Generate a README.md file including:
- Project description and purpose
- Installation instructions
- Quick start guide
- Basic usage examples
- Configuration overview
- Links to additional documentation`,
					"inline": `Generate inline code documentation:
- Add comprehensive comments to functions and methods
- Document complex algorithms and business logic
- Add type annotations and parameter descriptions
- Include usage examples in comments`,
					"tutorial": `Generate step-by-step tutorials including:
- Learning objectives
- Prerequisites
- Detailed implementation steps
- Code examples with explanations
- Common pitfalls and solutions
- Next steps and advanced topics`,
				},
			},

			BannerText: "Code Documentation Generator: Create comprehensive code documentation",
			HelpText:   "Commands: add, note, list, type, edit, remove, show, copy, help, exit\nDoc types: comprehensive, api, readme, inline, tutorial",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about documentation requirements"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "type",
					Type:        "custom",
					Description: "Set documentation type",
					Usage:       "type <comprehensive|api|readme|inline|tutorial>",
					Handler: `function (args) {
						if (args.length === 0) {
							output.print("Current type: " + (state.get(StateKeys.docType) || "comprehensive"));
							output.print("Available types: comprehensive, api, readme, inline, tutorial");
							return;
						}
						const type = args[0].toLowerCase();
						const validTypes = ["comprehensive", "api", "readme", "inline", "tutorial"];
						if (!validTypes.includes(type)) {
							output.print("Invalid type. Available: " + validTypes.join(", "));
							return;
						}
						state.set(StateKeys.docType, type);
						output.print("Documentation type set to: " + type);
					}`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Test Generator Goal
		{
			Name:        "test-generator",
			Description: "Generate comprehensive test suites for existing code",
			Category:    "testing",
			Usage:       "Analyzes code and generates unit tests, integration tests, and edge case coverage",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:      "Test Generator",
			TUIPrompt:     "(test-gen) > ",
			HistoryFile:   ".test-generator_history",
			EnableHistory: true,

			StateKeys: map[string]interface{}{
				"contextItems": []interface{}{},
				"testType":     "unit",
				"framework":    "auto",
			},

			PromptInstructions: `Generate {{.StateKeys.testType}} tests for the provided code following these guidelines:

{{.TestTypeInstructions}}{{.FrameworkInfo}}

**Test Quality Standards:**
- Write clear, descriptive test names that explain what is being tested
- Use the AAA pattern (Arrange, Act, Assert) for test structure
- Include setup and teardown code where appropriate
- Mock external dependencies and side effects
- Use meaningful test data that represents real-world scenarios
- Add comments explaining complex test logic or edge cases
- Ensure tests are deterministic and not flaky
- Group related tests logically

**Coverage Requirements:**
- Achieve high code coverage (aim for 90%+ for unit tests)
- Cover all public interfaces and methods
- Test both successful and failure paths
- Include edge cases and boundary conditions
- Test error handling and validation logic

**Output Format:**
- Provide complete, runnable test files
- Include necessary imports and setup code
- Add brief explanations for complex test scenarios
- Organize tests in logical groups or describe blocks
- Include any additional test utilities or helpers needed`,

			PromptTemplate: `**{{.Description | upper}}**

{{.PromptInstructions}}

## {{.ContextHeader}}

{{.ContextTxtar}}`,

			ContextHeader: "CODE TO TEST",

			PromptOptions: map[string]interface{}{
				"testTypeInstructions": map[string]string{
					"unit": `Generate comprehensive unit tests including:
- Test all public methods and functions
- Cover all branches and edge cases
- Test error conditions and exception handling
- Mock external dependencies appropriately
- Include boundary value testing
- Test both positive and negative scenarios`,
					"integration": `Generate integration tests including:
- Test interactions between components
- Verify data flow between modules
- Test API endpoints and database interactions
- Cover cross-component workflows
- Test configuration and setup scenarios
- Include realistic test data scenarios`,
					"e2e": `Generate end-to-end tests including:
- Test complete user workflows
- Verify UI interactions and responses
- Test data persistence across operations
- Cover critical user journeys
- Include performance expectations
- Test error recovery scenarios`,
					"performance": `Generate performance tests including:
- Benchmark critical functions and operations
- Test resource usage and memory leaks
- Measure response times and throughput
- Test under various load conditions
- Include stress testing scenarios
- Set performance baselines and thresholds`,
					"security": `Generate security tests including:
- Test input validation and sanitization
- Verify authentication and authorization
- Test for common vulnerabilities (XSS, SQL injection, etc.)
- Check data encryption and protection
- Test access controls and permissions
- Include penetration testing scenarios`,
				},
			},

			BannerText: "Test Generator: Create comprehensive test suites for your code",
			HelpText:   "Commands: add, note, list, type, framework, edit, remove, show, copy, help, exit\nTest types: unit, integration, e2e, performance, security\nFrameworks: auto, jest, mocha, go, pytest, junit, rspec",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about test requirements"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "type",
					Type:        "custom",
					Description: "Set test type",
					Usage:       "type <unit|integration|e2e|performance|security>",
					Handler: `function (args) {
						if (args.length === 0) {
							output.print("Current type: " + (state.get(StateKeys.testType) || "unit"));
							output.print("Available types: unit, integration, e2e, performance, security");
							return;
						}
						const type = args[0].toLowerCase();
						const validTypes = ["unit", "integration", "e2e", "performance", "security"];
						if (!validTypes.includes(type)) {
							output.print("Invalid type. Available: " + validTypes.join(", "));
							return;
						}
						state.set(StateKeys.testType, type);
						output.print("Test type set to: " + type);
					}`,
				},
				{
					Name:        "framework",
					Type:        "custom",
					Description: "Set testing framework",
					Usage:       "framework <auto|jest|mocha|go|pytest|junit|rspec>",
					Handler: `function (args) {
						if (args.length === 0) {
							output.print("Current framework: " + (state.get(StateKeys.framework) || "auto"));
							output.print("Available frameworks: auto, jest, mocha, go, pytest, junit, rspec");
							return;
						}
						const fw = args[0].toLowerCase();
						const validFrameworks = ["auto", "jest", "mocha", "go", "pytest", "junit", "rspec"];
						if (!validFrameworks.includes(fw)) {
							output.print("Invalid framework. Available: " + validFrameworks.join(", "));
							return;
						}
						state.set(StateKeys.framework, fw);
						output.print("Testing framework set to: " + fw);
					}`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Commit Message Goal
		{
			Name:        "commit-message",
			Description: "Generate Kubernetes-style commit messages from diffs and context",
			Category:    "git-workflow",
			Usage:       "Generates commit messages following Kubernetes semantic guidelines from git diffs and additional context",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:      "Commit Message",
			TUIPrompt:     "(commit-message) > ",
			HistoryFile:   ".commit-message_history",
			EnableHistory: true,

			StateKeys: map[string]interface{}{
				"contextItems": []interface{}{},
			},

			PromptInstructions: `You MUST produce a commit message strictly utilizing the following syntax / style / semantics.

Kubernetes (K8s) commit message guidelines emphasize clarity, conciseness, and adherence to a specific format for better code history and reviewability. ATTN: This style is explicitly NOT "conventional commit" formatted. The core principles are:

Subject Line:
    Concise Summary: The first line, the subject, should provide a brief summary of the change.
    Length: Aim for 50 characters or less, and do not exceed 72 characters.
    Imperative Mood: Use the imperative mood (e.g., "Add feature," "Fix bug," not "Added feature" or "Adds feature").
    Capitalization: Capitalize the first word of the subject unless it's a lowercase symbol or identifier.
    No Period: Do not end the subject line with a period.

Body:
    Blank Line Separation: Add a single blank line between the subject and the body.
    Detailed Explanation: The body should explain the "what" and "why" of the commit, providing context and rationale for the changes. Avoid simply restating "how" the change was implemented, as the code itself will show that.
    Wrap at 72 Characters: Wrap the lines of the body at 72 characters for readability.

General Guidelines:
    Avoid GitHub Keywords/Mentions:
    Do not use GitHub keywords (e.g., "fixes #123") or @mentions within the commit message itself. These belong in the Pull Request description.
    Squash Small Commits:
    For minor changes like typos or style fixes, consider squashing commits to maintain a cleaner git history.
    Meaningful Messages:
    Avoid vague messages like "fixed stuff" or "updated code." Strive for clear and meaningful descriptions.

Generate a commit message that follows these guidelines based on the provided diff and context information.`,

			PromptTemplate: `**{{.Description | upper}}**

{{.PromptInstructions}}

## {{.ContextHeader}}

{{.ContextTxtar}}`,

			ContextHeader: "DIFF CONTEXT / CHANGES",

			BannerText: "Commit Message: Generate Kubernetes-style commit messages",
			HelpText:   "Commands: add, diff, note, list, edit, remove, show, copy, run, help, exit",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "run",
					Type:        "custom",
					Description: "Quick run - add git diff and show prompt",
					Usage:       "run [git-diff-args...]",
					Handler: `function (args) {
						ctxmgr.commands.diff.handler(args.length > 0 ? args : []);
						output.print("\n" + buildPrompt());
					}`,
				},
				{Name: "help", Type: "help"},
			},
		},
	}
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
