package command

// GetBuiltInGoals returns all built-in pre-written goals.
// All goal behavior is defined declaratively here.
func GetBuiltInGoals() []Goal {
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

			StateKeys: map[string]interface{}{},

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

			HelpText: "Commands: add, note, list, edit, remove, show, copy, run, help, exit",

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
				"docType": "comprehensive",
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

			HelpText: "Commands: add, note, list, diff, type, edit, remove, show, copy, help, exit\nDoc types: comprehensive, api, readme, inline, tutorial",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
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
				"testType":  "unit",
				"framework": "auto",
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

			HelpText: "Commands: add, note, list, diff, type, framework, edit, remove, show, copy, help, exit\nTest types: unit, integration, e2e, performance, security\nFrameworks: auto, jest, mocha, go, pytest, junit, rspec",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about test requirements"},
				{Name: "list", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
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

			StateKeys: map[string]interface{}{},

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

			HelpText: "Commands: add, diff, note, list, edit, remove, show, copy, run, help, exit",

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
