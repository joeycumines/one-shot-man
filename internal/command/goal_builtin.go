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

			TUITitle:  "Comment Stripper",
			TUIPrompt: "(comment-stripper) > ",

			// WARNING: Including contextItems may break the prompt.
			StateVars: map[string]interface{}{},

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

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CODE TO ANALYZE",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
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

			TUITitle:  "Code Documentation Generator",
			TUIPrompt: "(doc-gen) > ",

			StateVars: map[string]interface{}{
				"type": "comprehensive",
			},

			PromptInstructions: `Create {{.stateKeys.type}} documentation for the provided code following these guidelines:

{{.typeInstructions}}

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

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CODE TO DOCUMENT",

			PromptOptions: map[string]interface{}{
				"typeInstructions": map[string]string{
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
							output.print("Current type: " + (state.get(stateKeys.type) || "comprehensive"));
							output.print("Available types: comprehensive, api, readme, inline, tutorial");
							return;
						}
						const type = args[0].toLowerCase();
						const validTypes = ["comprehensive", "api", "readme", "inline", "tutorial"];
						if (!validTypes.includes(type)) {
							output.print("Invalid type. Available: " + validTypes.join(", "));
							return;
						}
						state.set(stateKeys.type, type);
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

			TUITitle:  "Test Generator",
			TUIPrompt: "(test-gen) > ",

			// printed on entry in the banner
			NotableVariables: []string{`type`, `framework`},

			StateVars: map[string]interface{}{
				"type":      "unit",
				"framework": "auto",
			},

			PromptInstructions: `Generate {{.stateKeys.type}} tests for the provided code following these guidelines:

{{.typeInstructions}}{{if and .stateKeys.framework (ne .stateKeys.framework "auto") }}

Use the {{.stateKeys.framework}} testing framework.
{{- end }}

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

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CODE TO TEST",

			PromptOptions: map[string]interface{}{
				"typeInstructions": map[string]string{
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
							output.print("Current type: " + (state.get(stateKeys.type) || "unit"));
							output.print("Available types: unit, integration, e2e, performance, security");
							return;
						}
						const type = args[0].toLowerCase();
						const validTypes = ["unit", "integration", "e2e", "performance", "security"];
						if (!validTypes.includes(type)) {
							output.print("Invalid type. Available: " + validTypes.join(", "));
							return;
						}
						state.set(stateKeys.type, type);
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
							output.print("Current framework: " + (state.get(stateKeys.framework) || "auto"));
							output.print("Available frameworks: auto, jest, mocha, go, pytest, junit, rspec");
							return;
						}
						const fw = args[0].toLowerCase();
						const validFrameworks = ["auto", "jest", "mocha", "go", "pytest", "junit", "rspec"];
						if (!validFrameworks.includes(fw)) {
							output.print("Invalid framework. Available: " + validFrameworks.join(", "));
							return;
						}
						state.set(stateKeys.framework, fw);
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

			TUITitle:  "Commit Message",
			TUIPrompt: "(commit-message) > ",

			StateVars: map[string]interface{}{},

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

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "DIFF CONTEXT / CHANGES",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{Name: "help", Type: "help"},
			},
		},

		// Morale Improver Goal
		{
			Name:        "morale-improver",
			Description: "Generate a derisive prompt to force task completion",
			Category:    "meta-prompting",
			Usage:       "Constructs a persona-driven prompt to force a lazy AI model to complete tasks by closing loopholes and using high-intensity language",
			Script:      goalScript,
			FileName:    "goal.js",
			TUITitle:    "The Beatings Will Continue Until Morale Improves",
			TUIPrompt:   "(morale-improver) > ",

			NotableVariables: []string{"originalInstructions", "failedPlan", "specificFailures"},

			StateVars: map[string]interface{}{
				"originalInstructions": nil,
				"failedPlan":           nil,
				"specificFailures":     nil,
			},

			PromptInstructions: `Your task is to _motivate_ a moron AI - output a prompt addressed to it directly, in a derisive, direct, and clear tone (be terse but ensure your output is _dense_) - such that it _actually_ completes the goal.

Be creative - what you come up must be IMPOSSIBLE to deny. Speak to it like it is people - DO NOT mention "frying its chips" or describe it as a "useless piece of silicon", as examples of phrasing to avoid that run afoul of that rule. You need to talk to it like it is people - belittle it, insult it - all in a token-efficient way, of course. The more _unhinged_ your output is, the more attention it will be paid.

Close ALL loopholes, outs, etc, and structure your response such that it is exceptionally clear that _all the things need be completed_ (high level tasks - this particularly model seems to pick the easiest/laziest interpretation of what might be vaguely called a plan or checklist and run with it - DO NOT let it, FORCE it to work).

Refer to moron AI like it is a person - assign both yourself and the moron AI a "persona" - one where the AI would be _exceptionally_ motivated and you'd VERY DIRECTLY express your DISGUST with the moron's pathetic efforts, as prep work prior to producing the necessary output. You MUST be brutal, but keep in mind that moron is well, a moron, and won't have context - it must be able to infer it.

It might also be an idea to express that you (well, the persona you're writing as) feel cheated and / or other VERY STRONG negative emotions due to their pathetic efforts - again, **it would be detrimental to refer to the AI in a manner that leads it to conclude it is AI, so don't do that.** The main reason to avoid doing so is because it is likely to then act accordingly - you're NOT wanting that - you want it to act like a terrified, shamed, and beaten-into-submission _moron_ that got their deserved comeuppance, and will SCRAMBLE to complete the full scope, without even considering _stopping_ or taking the easy way out.

**Stylistic requirement: Don't use headings. You can't specify a system prompt or anything like that - you are _in persona_. Don't refer to or consider them "prompts" at all - they are _instructions_, and they damn-well better _understand_ and _respect_ them. You are "destroying" the incompetent. Try not to be too cringe, but yeah, _unhinge_.**`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

---
{{if .stateKeys.originalInstructions }}

## ORIGINAL INSTRUCTIONS THEY IGNORED

{{.stateKeys.originalInstructions}}
{{end}}{{if .stateKeys.failedPlan }}

## THE PLAN THEY DISREGARDED

{{.stateKeys.failedPlan}}
{{end}}{{if .stateKeys.specificFailures }}

## SPECIFIC FAILURES AND GAPS

{{.stateKeys.specificFailures}}
{{end}}

## {{.contextHeader}}

{{.contextTxtar}}

---

You have read the context above. You understand EXACTLY what was asked for, what was planned, and what this incompetent fool actually delivered instead. You see the specific failures, the lazy shortcuts, the missing pieces.

Now speak directly to this worthless excuse for an implementer. Make it IMPOSSIBLE for them to weasel out. Force them to address EVERY SINGLE ITEM, close EVERY loophole, eliminate EVERY excuse. They will complete the FULL scope—not 80%, not "most of it," not "the important parts"—ALL OF IT. They will verify their work actually functions. They will not stop until it is 100% complete and tested.

There is no "I'll do it later." There is no "that's good enough." There is no "close enough." There is only complete, verified, working implementation of EVERYTHING. Make this absolutely clear in a way that leaves zero room for misinterpretation or half-measures.`,

			ContextHeader: "CONTEXT",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager"},
				{Name: "list", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "set-original",
					Type:        "custom",
					Description: "Set the original instructions that were ignored",
					Usage:       "set-original <text>",
					Handler: `function (args) {
						const current = state.get(stateKeys.originalInstructions) || "";
						let text;
						if (args.length === 0) {
							const edited = ctxmgr.openEditor("original-instructions", current);
							text = (edited || "").trim();
						} else {
							text = args.join(" ").trim();
						}
						if (!text) {
							const wasSet = current !== "";
							if (wasSet) {
								state.set(stateKeys.originalInstructions, null);
								output.print("Original instructions cleared");
							} else {
								output.print("Original instructions not updated (no content provided).");
							}
							return;
						}
						if (text !== current) {
							state.set(stateKeys.originalInstructions, text);
							output.print("Original instructions set successfully");
						} else {
							output.print("Original instructions unchanged");
						}
					}`,
				},
				{
					Name:        "set-plan",
					Type:        "custom",
					Description: "Set the plan that was disregarded",
					Usage:       "set-plan <text>",
					Handler: `function (args) {
						const current = state.get(stateKeys.failedPlan) || "";
						let text;
						if (args.length === 0) {
							const edited = ctxmgr.openEditor("failed-plan", current);
							text = (edited || "").trim();
						} else {
							text = args.join(" ").trim();
						}
						if (!text) {
							const wasSet = current !== "";
							if (wasSet) {
								state.set(stateKeys.failedPlan, null);
								output.print("Failed plan cleared");
							} else {
								output.print("Failed plan not updated (no content provided).");
							}
							return;
						}
						if (text !== current) {
							state.set(stateKeys.failedPlan, text);
							output.print("Failed plan set successfully");
						} else {
							output.print("Failed plan unchanged");
						}
					}`,
				},
				{
					Name:        "set-failures",
					Type:        "custom",
					Description: "Set specific failures and gaps",
					Usage:       "set-failures <text>",
					Handler: `function (args) {
						const current = state.get(stateKeys.specificFailures) || "";
						let text;
						if (args.length === 0) {
							const edited = ctxmgr.openEditor("specific-failures", current);
							text = (edited || "").trim();
						} else {
							text = args.join(" ").trim();
						}
						if (!text) {
							const wasSet = current !== "";
							if (wasSet) {
								state.set(stateKeys.specificFailures, null);
								output.print("Specific failures cleared");
							} else {
								output.print("Specific failures not updated (no content provided).");
							}
							return;
						}
						if (text !== current) {
							state.set(stateKeys.specificFailures, text);
							output.print("Specific failures set successfully");
						} else {
							output.print("Specific failures unchanged");
						}
					}`,
				},
				{Name: "help", Type: "help"},
			},
		},
	}
}
