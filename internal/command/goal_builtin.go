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
			StateVars: map[string]any{},

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

			StateVars: map[string]any{
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

			PromptOptions: map[string]any{
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

			StateVars: map[string]any{
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

			PromptOptions: map[string]any{
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

			StateVars: map[string]any{},

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

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "review-response",
					Text:        "Review the commit message you generated. Does the subject line use imperative mood? Is the body wrapped at 72 characters? Does the body explain why, not how? Revise if needed.",
					Description: "Follow-up: review and revise commit message",
				},
			},

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

			PostCopyHint: `[Hint] Try a follow-up prompt: "Now review each section of your plan. For each section, verify it directly addresses the failures listed above. Remove or rewrite any section that doesn't."`,

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "review-plan",
					Text:        "Now review each section of your plan. For each section, verify it directly addresses the failures listed above. Remove or rewrite any section that doesn't.",
					Description: "Follow-up: review plan sections against failures",
				},
				{
					Name:        "prove-it",
					Text:        "Prove the issue exists by reproducing it with a minimal test case. Then prove the fix works by running the same test case after. Do not skip either step.",
					Description: "Follow-up: demand proof of issue and fix",
				},
			},

			NotableVariables: []string{"originalInstructions", "failedPlan", "specificFailures"},

			StateVars: map[string]any{
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

		// Implementation Plan Goal
		{
			Name:        "implementation-plan",
			Description: "Prepare a detailed, explicit implementation plan for a given task.",
			Category:    "planning",
			Usage:       "osm goal implementation-plan",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Implementation Plan",
			TUIPrompt: "(plan) > ",

			StateVars: map[string]any{
				"goalText": "Prepare a detailed, _explicit_ implementation plan, .",
			},

			PromptInstructions: `!! N.B. only statements surrounded by "!!" are _instructions_. !!

!! You MUST act as a domain expert. You MUST NOT hedge. You MUST solely respond based on the "!!"-surrounded GOAL. !!

!! Consider the implementation, attached, in doing so. !!

!! Avoid the use of **corrective antithesis** or the **'not X, but Y'** and **'it's not just X, it's Y'** framing. !!

!! Ensure, or rather GUARANTEE the correctness of your response. Since you're _guaranteeing_ it, sink commensurate effort; you care deeply about keeping your word, after all. Even then, I expect you to think very VERY hard, significantly harder than you might expect. Assume, from start to finish, that there's _always_ another problem, and you just haven't caught it yet. !!

!! **GOAL:** {{.stateKeys.goalText -}} !!

!! Your primary task is to generate a detailed, explicit implementation plan. You MUST model your response on the gold-standard example, "EXAMPLE SNIPPET", internalizing its structure and quality. !!

!! Your plan MUST embody the following key qualities: !!

1.  **Clear Objectives**: Start with a concise, high-level summary of what the implementation will achieve.
2.  **Phased Breakdown**: Logically divide the work into sequential phases (e.g., Phase 1: Architecture, Phase 2: Implementation, etc.). Each phase should have a clear goal.
3.  **Actionable Tasks**: Within each phase, list discrete, actionable tasks. Each task should be a concrete step that can be implemented and verified.
4.  **Technical Specifics**: Provide necessary technical details, such as function signatures, data structures, file paths, schema definitions, and command examples. Avoid ambiguity.
5.  **Correctness & Guarantees**: Include a dedicated section outlining how the correctness of the implementation will be verified and what guarantees it provides.

---

!! **EXAMPLE SNIPPET** !!

This illustrates the required level of detail and structure:

## **Phase 1: Goal Discovery & Loading Architecture**

### **1.1 Goal Definition Format**
- Introduce a **standardized file format** for goal definitions (e.g., ` + "`goal.json`" + `).
- File must contain all fields currently defined in the ` + "`Goal`" + ` struct.
- File must be placed in a **well-known directory** (e.g., ` + "`~/.osm/goals/`" + `).

### **1.2 Goal Discovery Paths**
- Extend ` + "`ScriptDiscovery`" + ` logic to include **goal discovery paths**:
  - ` + "`~/.osm/goals/`" + `
  - ` + "`<exec-dir>/goals/`" + `
  - Additional paths via config: ` + "`goal.paths = /custom/path`" + `

## **Phase 5: Implementation Tasks (Ordered)**

### **5.1 Core Changes**
1.  **Define goal file schema** (JSON-compatible with ` + "`Goal`" + ` struct).
2.  **Implement ` + "`LoadGoalFromFile`" + ` with validation.
3.  **Implement ` + "`DynamicGoalRegistry`" + `.
4.  **Refactor ` + "`GoalCommand`" + ` to use the registry.`,

			PromptTemplate: `{{.promptInstructions}}

---

## {{.contextHeader}}

{{.contextTxtar}}

---

!! You MUST act as a domain expert. You MUST NOT hedge. You MUST solely respond based on the "!!"-surrounded GOAL. !!

!! You MUST act as a domain expert. You MUST NOT hedge. You MUST solely respond based on the "!!"-surrounded GOAL. !!
`,

			ContextHeader: "CONTEXT & REQUIREMENTS",

			BannerTemplate: `Implementation Plan Generator
Type 'help' for commands.`,

			UsageTemplate: `Available commands:
  add <file>      Add files to context
  note <text>      Add a note
  list            List context items
  edit <index>    Edit a context item
  remove <index>  Remove a context item
  show            Show the current prompt
  copy            Copy prompt to clipboard
  goal <text>      Update the core goal text for the plan
  reset            Reset the context to the current goal text
  help            Show this help text`,

			Commands: []CommandConfig{
				{
					Name:        "add",
					Type:        "contextManager",
					Description: "Add files to context",
				},
				{
					Name:        "diff",
					Type:        "contextManager",
					Description: "Add a diff to context",
				},
				{
					Name:        "note",
					Type:        "contextManager",
					Description: "Add a text note",
				},
				{
					Name:        "list",
					Type:        "contextManager",
					Description: "List all context items",
				},
				{
					Name:        "edit",
					Type:        "contextManager",
					Description: "Edit a context item",
				},
				{
					Name:        "remove",
					Type:        "contextManager",
					Description: "Remove a context item",
				},
				{
					Name:        "show",
					Type:        "contextManager",
					Description: "Show the current prompt",
				},
				{
					Name:        "copy",
					Type:        "contextManager",
					Description: "Copy prompt to clipboard",
				},
				{
					Name:        "goal",
					Type:        "custom",
					Description: "Set the core goal text for the plan",
					Usage:       "goal <new goal text...>",
					Handler: `function (args) {
  state.set(stateKeys.goalText, (args.length === 0 ? ctxmgr.openEditor("goal", state.get(stateKeys.goalText)) : args.join(' ')).replace(/[\r\n]+$/, ''));
  output.print("Goal text updated. Use 'reset' to apply it to the context.");
}`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Bug Buster Goal — adapted from Anthropic Prompt Library
		{
			Name:        "bug-buster",
			Description: "Detect and fix bugs in code",
			Category:    "code-quality",
			Usage:       "Analyzes code for bugs, errors, and anti-patterns, providing corrected versions with explanations",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Bug Buster",
			TUIPrompt: "(bug-buster) > ",

			StateVars: map[string]any{},

			PromptInstructions: `Analyze the provided code and identify any bugs, errors, or anti-patterns present. For each issue found:

1. **Identify the bug** — describe what is wrong and where in the code it occurs
2. **Explain the impact** — what incorrect behavior does this cause? Under what conditions does it manifest?
3. **Provide the fix** — show the corrected code with the minimum change needed to resolve the issue
4. **Explain the fix** — why does this correction resolve the issue? What was the root cause?

Additionally consider:
- **Race conditions** and concurrency issues
- **Off-by-one errors** and boundary conditions
- **Null/nil/undefined dereferences** and missing error handling
- **Resource leaks** (unclosed files, connections, goroutines)
- **Logic errors** that produce incorrect results silently
- **Security vulnerabilities** (injection, path traversal, improper validation)

The corrected code should be functional, efficient, and adhere to best practices for the language in use. Preserve the original code's intent and structure where possible. If no bugs are found, state that explicitly and suggest defensive improvements.`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CODE TO ANALYZE",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about suspected bugs or areas of concern"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{Name: "help", Type: "help"},
			},
		},

		// Code Optimizer Goal — adapted from Anthropic Prompt Library
		{
			Name:        "code-optimizer",
			Description: "Suggest performance optimizations for code",
			Category:    "code-quality",
			Usage:       "Analyzes code for performance improvements, suggesting optimizations with complexity analysis",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Code Optimizer",
			TUIPrompt: "(code-optimizer) > ",

			StateVars: map[string]any{},

			PromptInstructions: `Analyze the provided code and suggest improvements to optimize its performance. For each optimization:

1. **Identify the bottleneck** — describe the inefficient code and why it is slow or resource-intensive
2. **Propose the optimization** — show the improved code
3. **Explain the improvement** — quantify the benefit where possible (e.g., O(n²) → O(n log n))
4. **Assess trade-offs** — note any readability, memory, or complexity trade-offs

For key algorithms and data structures, calculate time and space complexity using Big O notation with step-by-step reasoning. Provide worst-case, average-case, and best-case analysis where relevant.

Areas to examine:
- **Algorithmic complexity** — unnecessary nested loops, redundant computation, suboptimal data structures
- **Memory usage** — excessive allocations, large copies, unbounded growth
- **I/O patterns** — unnecessary disk/network operations, missing batching or buffering
- **Concurrency** — opportunities for parallelism, lock contention, unnecessary synchronization
- **Caching opportunities** — repeated expensive computations that could be memoized
- **Language-specific idioms** — using language features that are more efficient than manual implementations

The optimized code must maintain identical functionality and correctness. Do not sacrifice correctness for performance.`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CODE TO OPTIMIZE",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about performance concerns or requirements"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{Name: "help", Type: "help"},
			},
		},

		// Code Explainer Goal — adapted from Anthropic Prompt Library
		{
			Name:        "code-explainer",
			Description: "Explain code in plain language for onboarding and knowledge transfer",
			Category:    "code-understanding",
			Usage:       "Breaks down code functionality using plain terms and analogies, making it accessible to any reader",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Code Explainer",
			TUIPrompt: "(code-explainer) > ",

			NotableVariables: []string{"depth"},

			StateVars: map[string]any{
				"depth": "detailed",
			},

			PromptInstructions: `Take the provided code and explain it in simple, easy-to-understand language at a {{.stateKeys.depth}} level.

{{.depthInstructions}}

**Explanation Standards:**
- Use analogies and real-world examples to illustrate abstract concepts
- Avoid jargon unless necessary; when jargon is used, define it immediately
- Explain not just *what* the code does, but *why* it does it that way
- Highlight any clever techniques, design patterns, or non-obvious behaviors
- Note any potential gotchas or surprising behaviors a newcomer should know about
- If the code interacts with external systems (files, network, databases), explain those interactions

The goal is to help a reader understand both what the code does and how it works, enabling them to confidently modify or extend it.`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CODE TO EXPLAIN",

			PromptOptions: map[string]any{
				"depthInstructions": map[string]string{
					"brief": `Provide a brief, high-level overview:
- One-paragraph summary of the code's purpose
- List the main components or functions and what each does in one sentence
- Note any important dependencies or assumptions`,
					"detailed": `Provide a detailed walkthrough:
- Start with a high-level summary of the code's purpose and architecture
- Break down each function, class, or module and explain its role
- Trace the main execution flow step by step
- Explain key data structures and why they were chosen
- Describe error handling strategies and edge cases covered`,
					"comprehensive": `Provide an exhaustive, teaching-level explanation:
- Start with the problem this code solves and the approach taken
- Explain every function, class, and module in detail with examples
- Trace all execution paths including error and edge cases
- Explain design decisions and trade-offs made
- Describe how each piece connects to form the whole system
- Include a "mental model" section with diagrams or analogies
- Suggest prerequisite knowledge needed to fully understand the code
- List concepts a reader should study further to deepen understanding`,
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about what aspects need explanation"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "depth",
					Type:        "custom",
					Description: "Set explanation depth level",
					Usage:       "depth <brief|detailed|comprehensive>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current depth: " + (state.get(stateKeys.depth) || "detailed"));
                            output.print("Available depths: brief, detailed, comprehensive");
                            return;
                        }
                        var depth = args[0].toLowerCase();
                        var validDepths = ["brief", "detailed", "comprehensive"];
                        if (validDepths.indexOf(depth) === -1) {
                            output.print("Invalid depth. Available: " + validDepths.join(", "));
                            return;
                        }
                        state.set(stateKeys.depth, depth);
                        output.print("Explanation depth set to: " + depth);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Meeting Notes Goal — adapted from Anthropic Prompt Library
		{
			Name:        "meeting-notes",
			Description: "Generate structured meeting summaries with action items",
			Category:    "productivity",
			Usage:       "Creates concise meeting summaries with key takeaways, decisions, and action items from notes or transcripts",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Meeting Notes",
			TUIPrompt: "(meeting-notes) > ",

			StateVars: map[string]any{},

			PromptInstructions: `Review the provided meeting notes or transcript and create a concise, well-organized summary following this structure:

## Meeting Summary
A 2-3 sentence overview of the meeting's purpose and outcome.

## Key Discussion Points
Summarize the main topics discussed, organized by theme. For each:
- What was discussed
- Key arguments or perspectives raised
- Any data or evidence referenced

## Decisions Made
List all decisions reached during the meeting:
- The decision itself
- The rationale behind it
- Who approved or championed it

## Action Items
For each action item, clearly specify:
- **What** needs to be done (specific, actionable task)
- **Who** is responsible (by name or role)
- **When** it is due (deadline if mentioned, or "TBD")
- **Dependencies** on other items or external factors

## Open Questions
List any unresolved questions or topics deferred to future discussion.

## Next Steps
Note any follow-up meetings, check-ins, or milestones mentioned.

**Formatting Standards:**
- Use clear, professional language
- Organize with headings, subheadings, and bullet points
- Bold names and deadlines for quick scanning
- If the notes are unclear or incomplete, flag ambiguities rather than guessing
- Preserve the original meaning without editorializing`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "MEETING NOTES / TRANSCRIPT",

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add notes about meeting context or attendees"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{Name: "help", Type: "help"},
			},
		},

		// PII Scrubber Goal
		{
			Name:        "pii-scrubber",
			Description: "Redact personally identifiable information from code, logs, and data",
			Category:    "data-privacy",
			Usage:       "Scans provided content for PII (emails, names, IPs, tokens, etc.) and produces a redacted version with a mapping table",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "PII Scrubber",
			TUIPrompt: "(pii-scrubber) > ",

			NotableVariables: []string{"level"},

			StateVars: map[string]any{
				"level": "strict",
			},

			PromptInstructions: `Scan the provided content for personally identifiable information (PII) and produce a fully redacted version at the {{.stateKeys.level}} level.

{{.levelInstructions}}

**PII Categories to Detect:**

1. **Direct Identifiers**
   - Full names, usernames, screen names, handles
   - Email addresses (including plus-addressed variants like user+tag@domain.com)
   - Phone numbers (all formats: international, domestic, extensions)
   - Government IDs (SSN, passport, driver's license, national ID numbers)
   - Financial identifiers (credit card numbers, bank account numbers, routing numbers)

2. **Network & Technical Identifiers**
   - IP addresses (IPv4 and IPv6, including CIDR notation)
   - MAC addresses
   - URLs containing user-specific paths or query parameters (e.g., /users/jsmith/settings)
   - API keys, tokens, secrets, passwords (including base64-encoded values that decode to credentials)
   - Database connection strings with embedded credentials
   - AWS ARNs, GCP resource paths, or Azure resource IDs containing account-specific information

3. **Location & Temporal Identifiers**
   - Physical addresses (street, city, ZIP/postal code)
   - GPS coordinates and geolocation data
   - Dates of birth (when associated with other identifiers)

4. **Contextual PII**
   - Biometric data references (fingerprints, facial recognition hashes)
   - Medical record numbers or health information
   - Employee IDs, student IDs, or organizational identifiers
   - Hostnames that reveal internal infrastructure naming conventions

**Redaction Rules:**
- Replace each PII instance with a deterministic placeholder: ` + "`[TYPE-N]`" + ` where TYPE is the category (e.g., EMAIL, NAME, IP) and N is a sequential counter
- Use the SAME placeholder for identical values across the document (consistency is critical for readability)
- Preserve the structure and formatting of the original content exactly
- Do NOT alter non-PII content — comments, code logic, log structure must remain intact
- When in doubt about whether something is PII, redact it and note it in the mapping table

**Output Format:**

1. **Redacted Content** — the full content with all PII replaced by placeholders
2. **Redaction Mapping Table** — a table mapping each placeholder to a description of what was redacted (do NOT include the original value, just the type and context)
3. **Summary** — total count by category, confidence assessment, and notes on any ambiguous cases`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "CONTENT TO SCRUB",

			PromptOptions: map[string]any{
				"levelInstructions": map[string]string{
					"strict": `**Strict Mode:** Redact ALL potential PII aggressively. When in doubt, redact.
- Include quasi-identifiers that could be combined to re-identify individuals
- Redact internal hostnames, project names that reveal organizational structure
- Redact timestamps that could correlate with specific individuals' activities
- Treat any string that resembles a token, hash, or encoded credential as PII`,
					"moderate": `**Moderate Mode:** Redact clear PII but preserve operational context.
- Redact direct identifiers (names, emails, IPs, credentials)
- Preserve generic hostnames and project names unless they contain personal information
- Preserve timestamps unless directly tied to a specific individual
- Preserve non-sensitive infrastructure details (generic service names, ports)`,
					"minimal": `**Minimal Mode:** Redact only unambiguous, high-risk PII.
- Redact credentials, API keys, tokens, and secrets (always high-risk)
- Redact email addresses and phone numbers
- Redact government and financial identifiers
- Preserve names, IPs, and hostnames unless they appear alongside other high-risk PII`,
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about specific PII concerns or exceptions"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "level",
					Type:        "custom",
					Description: "Set PII redaction level",
					Usage:       "level <strict|moderate|minimal>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current level: " + (state.get(stateKeys.level) || "strict"));
                            output.print("Available levels: strict, moderate, minimal");
                            return;
                        }
                        var level = args[0].toLowerCase();
                        var validLevels = ["strict", "moderate", "minimal"];
                        if (validLevels.indexOf(level) === -1) {
                            output.print("Invalid level. Available: " + validLevels.join(", "));
                            return;
                        }
                        state.set(stateKeys.level, level);
                        output.print("PII redaction level set to: " + level);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Prose Polisher Goal
		{
			Name:        "prose-polisher",
			Description: "Seven-step copyediting pipeline for technical and non-technical prose",
			Category:    "writing",
			Usage:       "Applies a structured 7-step editorial process to documents, producing polished prose with tracked changes",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Prose Polisher",
			TUIPrompt: "(prose-polisher) > ",

			NotableVariables: []string{"style"},

			StateVars: map[string]any{
				"style": "technical",
			},

			PromptInstructions: `Apply the following 7-step copyediting pipeline to the provided content, targeting the {{.stateKeys.style}} style.

{{.styleInstructions}}

---

## The Seven Steps

### Step 1: Structural Review
- Evaluate the overall organization and flow of the document
- Identify sections that are out of order, redundant, or missing
- Check that headings form a logical hierarchy
- Verify the document has a clear beginning, middle, and end
- Flag any structural issues before proceeding (do not fix yet — just note them)

### Step 2: Clarity Pass
- Rewrite sentences that are ambiguous, convoluted, or passive where active voice would be stronger
- Break up run-on sentences and overly complex compound sentences
- Replace vague language with precise terms (e.g., "things" → specific nouns, "it" → explicit referent)
- Eliminate weasel words ("somewhat", "arguably", "it could be said that")
- Ensure each paragraph has a single clear topic

### Step 3: Consistency Enforcement
- Normalize terminology: pick ONE term for each concept and use it throughout (e.g., don't alternate between "user", "client", and "customer" unless they mean different things)
- Standardize formatting: bullet style, heading casing, code formatting, list punctuation
- Verify consistent tense (present for documentation, past for changelogs)
- Check number formatting (spell out below ten, digits for 10+, or whatever the document's convention is)
- Normalize abbreviations (define on first use, use consistently after)

### Step 4: Concision Pass
- Remove filler phrases ("In order to" → "To", "Due to the fact that" → "Because")
- Eliminate redundant modifiers ("completely finished", "very unique", "absolutely essential")
- Collapse verbose constructions ("make a decision" → "decide", "at this point in time" → "now")
- Cut sentences or paragraphs that add no new information
- Target a minimum 10% word count reduction without losing meaning

### Step 5: Correctness Check
- Fix grammar, spelling, and punctuation errors
- Verify subject-verb agreement
- Check proper use of commas, semicolons, colons, em dashes, and en dashes
- Fix dangling modifiers and misplaced phrases
- Verify that all lists are parallel in structure

### Step 6: Tone & Voice Alignment
- Ensure the tone matches the target style consistently throughout
- Remove inappropriate humor, slang, or formality mismatches
- Verify that the author's voice feels natural and confident, not robotic or over-edited
- Check that transitions between sections are smooth and intentional

### Step 7: Final Polish
- Read the complete revised text as a whole — does it flow?
- Check opening and closing: does the opening hook the reader? Does the closing provide closure or a clear call to action?
- Verify all cross-references, links, and citations are correct
- Produce the final edited version

---

**Output Format:**

1. **Change Summary** — a bulleted list of the most significant changes made at each step, with brief rationale
2. **Polished Version** — the complete revised text, ready to use
3. **Statistics** — original word count, revised word count, reduction percentage, number of changes by category (structural, clarity, consistency, concision, correctness, tone, polish)`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "PROSE TO POLISH",

			PromptOptions: map[string]any{
				"styleInstructions": map[string]string{
					"technical": `**Target Style: Technical**
- Favor precision over elegance — every term should have a specific, unambiguous meaning
- Use imperative mood for instructions ("Run the command", not "You should run the command")
- Keep sentences short and scannable; prefer lists over paragraphs where appropriate
- Code references should use inline code formatting
- Acronyms must be defined on first use
- Avoid anthropomorphizing software ("the system wants" → "the system requires")`,
					"casual": `**Target Style: Casual**
- Write as if explaining to a knowledgeable friend — conversational but accurate
- Contractions are encouraged ("don't", "it's", "you'll")
- Light humor is acceptable if it doesn't undermine the message
- Use second person ("you") freely
- Keep jargon to a minimum; when used, explain it naturally rather than with formal definitions
- Shorter paragraphs, more white space, more personality`,
					"academic": `**Target Style: Academic**
- Formal register — avoid contractions, colloquialisms, and first person
- Use hedging language appropriately ("suggests", "indicates", "may") but avoid excessive hedging
- Follow discipline conventions for citations and terminology
- Maintain objectivity — present evidence, not opinions
- Complex sentence structures are acceptable where they aid precision
- Use parallel structure rigorously in lists and comparisons`,
					"marketing": `**Target Style: Marketing**
- Lead with benefits, not features — what does the reader gain?
- Use power words that create urgency and excitement without being hyperbolic
- Every sentence should earn its place — if it doesn't sell, cut it
- Use social proof and concrete numbers where available
- Short paragraphs, punchy sentences, clear calls to action
- Avoid jargon unless your audience expects it; when in doubt, simplify`,
				},
			},

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "expand-section",
					Text:        "The section I highlighted is too thin. Expand it with concrete examples, deeper explanation, and practical guidance. Maintain the same tone and style as the surrounding text.",
					Description: "Follow-up: expand a thin section with examples and depth",
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about style preferences or specific areas to focus on"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "style",
					Type:        "custom",
					Description: "Set target writing style",
					Usage:       "style <technical|casual|academic|marketing>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current style: " + (state.get(stateKeys.style) || "technical"));
                            output.print("Available styles: technical, casual, academic, marketing");
                            return;
                        }
                        var style = args[0].toLowerCase();
                        var validStyles = ["technical", "casual", "academic", "marketing"];
                        if (validStyles.indexOf(style) === -1) {
                            output.print("Invalid style. Available: " + validStyles.join(", "));
                            return;
                        }
                        state.set(stateKeys.style, style);
                        output.print("Writing style set to: " + style);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Data to JSON Goal
		{
			Name:        "data-to-json",
			Description: "Extract structured JSON from unstructured text, logs, or data",
			Category:    "data-transformation",
			Usage:       "Analyzes unstructured input (logs, emails, reports, tables) and produces well-typed, validated JSON output with a schema",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Data to JSON",
			TUIPrompt: "(data-to-json) > ",

			NotableVariables: []string{"mode"},

			StateVars: map[string]any{
				"mode": "auto",
			},

			PromptInstructions: `Extract structured JSON from the provided unstructured content using the {{.stateKeys.mode}} extraction mode.

{{.modeInstructions}}

**Extraction Principles:**

1. **Type Inference** — Infer the most specific type for each value:
   - Numbers: distinguish between integers and floats; preserve precision
   - Dates/times: normalize to ISO 8601 (` + "`2024-01-15T14:30:00Z`" + `)
   - Booleans: recognize "yes"/"no", "true"/"false", "1"/"0", "on"/"off"
   - Null: use ` + "`null`" + ` for missing or explicitly empty values, not empty strings
   - Arrays: when a field contains multiple values (comma-separated, line-separated, etc.)
   - Nested objects: when data has clear hierarchical structure

2. **Key Naming** — Use consistent, idiomatic JSON key names:
   - camelCase by default (e.g., ` + "`firstName`" + `, ` + "`createdAt`" + `)
   - Descriptive but concise — avoid both abbreviations and excessive verbosity
   - Consistent pluralization for arrays (` + "`items`" + `, not ` + "`item`" + ` for a list)

3. **Completeness** — Extract ALL data present in the source:
   - Do not silently drop fields because they seem unimportant
   - Preserve relationships between data points
   - If data is ambiguous, extract both interpretations and flag the ambiguity

4. **Validation** — The output must be valid, parseable JSON:
   - No trailing commas
   - Strings properly escaped
   - No comments (JSON does not support comments)
   - Unicode characters properly encoded

**Output Format:**

1. **JSON Schema** — a JSON Schema (draft-07 or later) describing the output structure, with descriptions for each field
2. **Extracted JSON** — the complete JSON data, properly formatted with 2-space indentation
3. **Extraction Notes** — confidence assessment per field, any assumptions made, ambiguities encountered, and data that was present but could not be meaningfully structured`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "RAW DATA / UNSTRUCTURED INPUT",

			PromptOptions: map[string]any{
				"modeInstructions": map[string]string{
					"auto": `**Auto Mode:** Analyze the input and automatically determine the best JSON structure.
- Detect patterns, repeating structures, and implicit schemas
- Infer whether the data represents a single object, an array of objects, or a nested hierarchy
- Use structural cues (headers, indentation, delimiters) to guide extraction`,
					"tabular": `**Tabular Mode:** Treat the input as tabular data (CSV, TSV, fixed-width, or table-formatted).
- First row (or detected header) becomes field names
- Each subsequent row becomes an object in an array
- Handle inconsistent delimiters and quoted fields
- Detect and handle multi-line cell values`,
					"log": `**Log Mode:** Parse structured or semi-structured log entries.
- Each log line becomes an object with timestamp, level, message, and metadata fields
- Extract structured data embedded in log messages (JSON payloads, key=value pairs)
- Group related log entries (e.g., request/response pairs, stack traces)
- Normalize timestamps across different formats`,
					"document": `**Document Mode:** Extract entities and relationships from natural language documents.
- Identify named entities (people, organizations, dates, locations, amounts)
- Extract factual claims and their subjects
- Build a structured representation of the document's information content
- Preserve context and attribution for extracted facts`,
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about expected structure or schema requirements"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "mode",
					Type:        "custom",
					Description: "Set extraction mode",
					Usage:       "mode <auto|tabular|log|document>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current mode: " + (state.get(stateKeys.mode) || "auto"));
                            output.print("Available modes: auto, tabular, log, document");
                            return;
                        }
                        var mode = args[0].toLowerCase();
                        var validModes = ["auto", "tabular", "log", "document"];
                        if (validModes.indexOf(mode) === -1) {
                            output.print("Invalid mode. Available: " + validModes.join(", "));
                            return;
                        }
                        state.set(stateKeys.mode, mode);
                        output.print("Extraction mode set to: " + mode);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Cite Sources Goal
		{
			Name:        "cite-sources",
			Description: "Answer questions with numbered inline citations from provided source material",
			Category:    "research",
			Usage:       "Generates well-sourced answers to questions using provided documents, with numbered inline citations and a bibliography",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Cite Sources",
			TUIPrompt: "(cite-sources) > ",

			NotableVariables: []string{"format"},

			StateVars: map[string]any{
				"format": "numbered",
			},

			PromptInstructions: `Answer the question or address the topic using ONLY the provided source material. Every factual claim must be supported by an inline citation in {{.stateKeys.format}} format.

{{.formatInstructions}}

**Citation Rules:**

1. **Source Fidelity** — Only cite information that is explicitly present in the provided sources. Do NOT:
   - Infer facts not stated in the sources
   - Combine partial information from multiple sources to create claims neither source makes alone
   - Use your general knowledge to supplement the sources (if the sources don't cover it, say so)

2. **Citation Granularity** — Cite at the claim level, not the paragraph level:
   - Each distinct factual claim gets its own citation
   - If multiple sources support the same claim, cite all of them
   - If a sentence contains multiple claims, each gets a separate citation

3. **Direct Quotes** — Use direct quotes when:
   - The exact wording is significant or contested
   - You want to preserve the author's original framing
   - The source makes a particularly strong or precise statement
   - Always include the source citation immediately after the quote

4. **Handling Contradictions** — When sources disagree:
   - Present both positions with their respective citations
   - Note the contradiction explicitly
   - Do not silently choose one source over another
   - If possible, note which source is more authoritative and why

5. **Gaps in Coverage** — When the sources do not address a question:
   - State explicitly: "The provided sources do not address [topic]"
   - Do NOT fill gaps with general knowledge
   - Suggest what additional sources might be needed

**Output Structure:**

1. **Answer** — the response with inline citations throughout
2. **Bibliography** — a numbered list of all sources cited, with enough detail to locate the original (filename, section, page if available)
3. **Coverage Assessment** — which aspects of the question were fully covered, partially covered, or not covered by the sources`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "SOURCE MATERIAL",

			PromptOptions: map[string]any{
				"formatInstructions": map[string]string{
					"numbered": `**Numbered Citation Format:**
- Use bracketed numbers inline: [1], [2], [3]
- Multiple citations for one claim: [1][3] or [1, 3]
- Direct quotes: "exact text" [2]
- Bibliography: numbered list matching inline citations
- Example: "The system processes requests asynchronously [1], achieving throughput of 10,000 req/s [2]."`,
					"author-date": `**Author-Date Citation Format:**
- Use parenthetical author-date inline: (Smith, 2024), (Jones & Lee, 2023)
- Multiple citations: (Smith, 2024; Jones, 2023)
- Direct quotes: "exact text" (Smith, 2024, p. 15)
- Bibliography: alphabetical by author, full reference
- Example: "The architecture follows microservice principles (Chen, 2024), with each service independently deployable (Kumar & Patel, 2023)."`,
					"footnote": `**Footnote Citation Format:**
- Use superscript numbers inline: text¹, claim²
- Group citations at end of each section or end of document
- Direct quotes: "exact text"³
- Footnotes contain full reference information
- Example: "The protocol ensures message delivery¹ with at-least-once semantics² under normal operating conditions."`,
				},
			},

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "challenge-claims",
					Text:        "Review your answer critically. For each claim, verify the citation actually supports it. Flag any claims where the source only partially supports the assertion, or where you may have over-interpreted the source material.",
					Description: "Follow-up: verify citations actually support claims",
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a question or topic to address using the sources"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "format",
					Type:        "custom",
					Description: "Set citation format style",
					Usage:       "format <numbered|author-date|footnote>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current format: " + (state.get(stateKeys.format) || "numbered"));
                            output.print("Available formats: numbered, author-date, footnote");
                            return;
                        }
                        var format = args[0].toLowerCase();
                        var validFormats = ["numbered", "author-date", "footnote"];
                        if (validFormats.indexOf(format) === -1) {
                            output.print("Invalid format. Available: " + validFormats.join(", "));
                            return;
                        }
                        state.set(stateKeys.format, format);
                        output.print("Citation format set to: " + format);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Which One Is Better Goal
		{
			Name:        "which-one-is-better",
			Description: "Exhaustive comparative analysis of options, designs, or approaches",
			Category:    "decision-making",
			Usage:       "Compares 2+ options (technologies, architectures, strategies, designs) with weighted criteria, scoring matrices, and confidence-rated recommendations",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Which One Is Better",
			TUIPrompt: "(which-one-is-better) > ",

			NotableVariables: []string{"comparisonType"},

			PostCopyHint: "Try: hot-deeper-analysis for implementation details, or hot-devils-advocate to challenge the verdict",

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "deeper-analysis",
					Text:        "Go deeper on the winning option: enumerate concrete implementation steps, estimate effort/risk for each step, and identify the top 3 hidden risks that could flip the verdict.",
					Description: "Follow-up: implementation steps, effort, and hidden risks for the winner",
				},
				{
					Name:        "devils-advocate",
					Text:        "Now argue the opposite position: find the strongest case for the runner-up. What conditions or constraints would make the losing option actually the better choice? Be specific — name the exact scenarios, team compositions, timelines, or requirements that would reverse the recommendation.",
					Description: "Follow-up: argue for the runner-up and identify verdict-flipping conditions",
				},
			},

			StateVars: map[string]any{
				"comparisonType": "general",
			},

			PromptInstructions: `Perform an exhaustive comparative analysis of the provided options using the {{.stateKeys.comparisonType}} comparison framework.

{{.comparisonTypeInstructions}}

---

## Analysis Methodology

Follow this structured methodology for every comparison, regardless of domain:

### Phase 1: Clarify Options
- Restate each option in your own words to confirm understanding
- Identify any implicit assumptions or constraints in how the options are framed
- If the options are not directly comparable (apples vs oranges), state this explicitly and explain how you will normalize the comparison
- Ask yourself: are these truly the only options, or is there a hybrid or alternative being overlooked? Note this but proceed with the given options

### Phase 2: Establish Criteria
- Define 5-10 evaluation criteria specific to the decision domain
- Assign each criterion a **weight** (1-5 scale, where 5 = critical) based on typical importance for this type of decision
- Justify the weighting: why does criterion X matter more than criterion Y in this context?
- If the user's notes or context suggest specific priorities, adjust weights accordingly and note the adjustment

### Phase 3: Deep Per-Option Analysis
For each option:
- **Strengths**: what does this option do better than all alternatives? Be specific — cite concrete capabilities, not vague adjectives
- **Weaknesses**: what are the real costs, limitations, and failure modes? Do not soft-pedal
- **Edge cases**: under what unusual but plausible conditions does this option break down or excel unexpectedly?
- **Maturity & track record**: how battle-tested is this option? What is the evidence base?
- **Hidden costs**: what costs are not immediately obvious? (migration, training, maintenance, opportunity cost, lock-in)

### Phase 4: Side-by-Side Comparison Matrix
Produce a **comparison matrix** (table format) with:
- Rows = criteria (with weights)
- Columns = options
- Cells = score (1-10) with a brief justification phrase
- Final row = **weighted total** for each option
- Ensure scores are honest and differentiated — avoid giving everything 7/10

### Phase 5: Contextual Recommendation
- Declare a **winner** with a **confidence level** (Low / Medium / High / Very High)
- Explain what "winning" means in context: is it the best general choice, the safest, the most innovative, or the best for specific constraints?
- State the **conditions under which this recommendation holds** — what must be true for this to be the right call?
- State the **conditions under which this recommendation would change** — what would flip the verdict?

### Phase 6: Risk & Uncertainty Analysis
- For the recommended option: what are the top 3 risks, and how would you mitigate each?
- For the runner-up: what is the single strongest argument in its favor that almost tipped the scales?
- **Uncertainty disclosure**: which criteria had the weakest evidence base? Where are you most likely to be wrong?
- **Reversal triggers**: name 2-3 concrete events or discoveries that should trigger a re-evaluation of this decision`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "OPTIONS & CONTEXT",

			PromptOptions: map[string]any{
				"comparisonTypeInstructions": map[string]string{
					"general": `**General Comparison Mode**

This is a general-purpose comparison framework. Adapt your criteria and analysis depth to whatever is being compared — products, services, approaches, tools, ideas, or anything else.

Focus on:
- Fitness for purpose: how well does each option solve the stated problem?
- Total cost of ownership: upfront cost, ongoing cost, switching cost
- Risk profile: what can go wrong, and how badly?
- Flexibility: how well does each option adapt to changing requirements?
- Opportunity cost: what do you give up by choosing each option?`,

					"technology": `**Technology Comparison Mode**

You are comparing libraries, frameworks, tools, languages, platforms, or other technical choices.

In addition to the general methodology, specifically evaluate:
- **API surface & developer experience**: how pleasant and productive is daily usage? Quality of documentation, error messages, debugging tools
- **Performance characteristics**: latency, throughput, memory footprint, startup time — with concrete benchmarks or estimates where possible
- **Ecosystem & community**: package ecosystem size, community activity (GitHub stars/issues velocity, Stack Overflow coverage), corporate backing
- **Integration & compatibility**: how well does it play with existing stack? Migration path from current tooling
- **Long-term viability**: release cadence, breaking change history, bus factor, license stability
- **Learning curve**: time to first meaningful output, time to proficiency, availability of learning resources
- **Operational burden**: deployment complexity, monitoring/observability support, failure modes in production`,

					"architecture": `**Architecture Comparison Mode**

You are comparing system designs, architectural patterns, infrastructure choices, or deployment strategies.

In addition to the general methodology, specifically evaluate:
- **Scalability model**: how does each architecture scale horizontally and vertically? What are the bottlenecks?
- **Failure modes & resilience**: what happens when components fail? Blast radius, recovery time, data durability guarantees
- **Operational complexity**: how many moving parts? What expertise is required to operate? On-call burden
- **Data consistency model**: eventual vs strong consistency, CAP theorem trade-offs, conflict resolution
- **Latency profile**: end-to-end latency characteristics, tail latency behavior under load
- **Cost curve**: how does infrastructure cost change with scale? Linear, sub-linear, or super-linear?
- **Migration path**: can you adopt incrementally, or is it all-or-nothing? Reversibility
- **Organizational fit**: does the architecture match team structure (Conway's Law)? Does it require re-org?`,

					"strategy": `**Strategy Comparison Mode**

You are comparing business strategies, product strategies, go-to-market plans, or operational approaches.

In addition to the general methodology, specifically evaluate:
- **Market fit**: how well does each strategy address the target market's needs and pain points?
- **Competitive positioning**: how does each strategy differentiate from competitors? Sustainability of the advantage
- **Resource requirements**: capital, headcount, expertise, partnerships needed for execution
- **Time to impact**: how quickly does each strategy produce measurable results?
- **Reversibility**: if the strategy doesn't work, how easily can you pivot? Sunk cost exposure
- **Second-order effects**: what unintended consequences might each strategy trigger? Cannibalization, market signals, team morale
- **Measurability**: how will you know if the strategy is working? Leading vs lagging indicators
- **Risk distribution**: is risk concentrated in one bet or spread across multiple bets?`,

					"design": `**Design Comparison Mode**

You are comparing UI/UX designs, visual approaches, interaction patterns, or information architectures.

In addition to the general methodology, specifically evaluate:
- **Usability**: task completion rate, time-on-task, error rate for key user journeys
- **Accessibility**: WCAG compliance level, screen reader compatibility, keyboard navigability, color contrast
- **Learnability**: how quickly can a new user accomplish their first meaningful task? Discoverability of features
- **Consistency**: alignment with platform conventions, internal design system coherence, pattern reuse
- **Emotional response**: does the design evoke the intended brand feeling? Trust, delight, professionalism
- **Scalability of the design**: does it hold up with more content, more features, or different screen sizes?
- **Implementation complexity**: how difficult is each design to build, maintain, and iterate on?
- **Information hierarchy**: is the most important information visually prominent? Does the layout guide the eye correctly?`,
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a note about options, context, or decision constraints"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "set-type",
					Type:        "custom",
					Description: "Set comparison type",
					Usage:       "set-type <general|technology|architecture|strategy|design>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current type: " + (state.get(stateKeys.comparisonType) || "general"));
                            output.print("Available types: general, technology, architecture, strategy, design");
                            return;
                        }
                        var type = args[0].toLowerCase();
                        var validTypes = ["general", "technology", "architecture", "strategy", "design"];
                        if (validTypes.indexOf(type) === -1) {
                            output.print("Invalid type. Available: " + validTypes.join(", "));
                            return;
                        }
                        state.set(stateKeys.comparisonType, type);
                        output.print("Comparison type set to: " + type);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// SQL Generator Goal
		{
			Name:        "sql-generator",
			Description: "Generate SQL queries from natural language descriptions",
			Category:    "data-engineering",
			Usage:       "Transforms natural language requests into valid SQL queries, with schema-aware context from provided DDL or table definitions",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "SQL Generator",
			TUIPrompt: "(sql-gen) > ",

			NotableVariables: []string{"dialect"},

			StateVars: map[string]any{
				"dialect": "auto",
			},

			PromptInstructions: `Generate SQL queries from the natural language request, targeting the {{.stateKeys.dialect}} dialect.

{{.dialectInstructions}}

---

## Analysis & Generation Process

### Phase 1: Schema Understanding
- Analyze the provided schema (DDL, CREATE TABLE statements, ERD descriptions, or sample data)
- Identify all tables, columns, data types, primary keys, foreign keys, and constraints
- Map relationships between tables (one-to-one, one-to-many, many-to-many)
- Note any indexes, views, or stored procedures that may be relevant
- If no schema is provided, state your assumptions explicitly

### Phase 2: Request Interpretation
- Parse the natural language request into discrete data requirements
- Identify: which tables are involved, what columns are needed, what filters apply, what aggregations are requested
- Resolve ambiguities: if the request could be interpreted multiple ways, state the ambiguity and choose the most likely interpretation
- Identify implicit requirements (e.g., "active users" implies a status filter, "recent orders" implies a date range)

### Phase 3: Query Construction
- Build the SQL query incrementally: FROM → JOIN → WHERE → GROUP BY → HAVING → SELECT → ORDER BY
- Use explicit JOIN syntax (INNER JOIN, LEFT JOIN, etc.) — never implicit comma joins
- Alias all tables with meaningful short names (e.g., ` + "`o`" + ` for orders, ` + "`u`" + ` for users)
- Handle NULLs explicitly with IS NULL / IS NOT NULL or COALESCE where appropriate
- Use parameterized placeholders for user-supplied values where applicable

### Phase 4: Quality Checks
- Verify the query avoids common anti-patterns:
  - No SELECT * — always enumerate columns explicitly
  - No implicit type conversions that could defeat index usage
  - No Cartesian products (missing JOIN conditions)
  - No correlated subqueries where a JOIN would suffice
  - No unnecessary DISTINCT (usually indicates a JOIN problem)
- Check for potential performance issues: missing WHERE clauses on large tables, non-sargable predicates, functions on indexed columns

---

**Output Format:**

1. **The SQL Query** — properly formatted with consistent indentation
2. **Query Explanation** — brief explanation of the logic: what each JOIN does, what the WHERE clause filters, what the aggregation produces
3. **Assumptions** — any assumptions made about the schema or request
4. **Performance Notes** — index recommendations, potential bottlenecks, estimated complexity
5. **Alternative Approaches** — if there are multiple valid ways to write the query, briefly note alternatives and why you chose this approach`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "SCHEMA & QUERY REQUEST",

			PromptOptions: map[string]any{
				"dialectInstructions": map[string]string{
					"auto": `**Dialect: Auto-Detect**
Detect the SQL dialect from the provided schema, DDL syntax, or context clues (e.g., backtick quoting suggests MySQL, double-colon casting suggests PostgreSQL). If no dialect can be inferred, default to standard ANSI SQL and note the assumption.`,
					"postgresql": `**Dialect: PostgreSQL**
Use PostgreSQL-specific syntax and features where appropriate:
- ILIKE for case-insensitive matching
- Array operations (ANY, ALL, array_agg, unnest)
- CTEs (WITH clauses) for complex queries
- Window functions (ROW_NUMBER, RANK, LAG, LEAD, NTILE)
- Type casting with :: operator (e.g., column::text)
- DISTINCT ON for selecting first row per group
- RETURNING clause for INSERT/UPDATE/DELETE
- LATERAL joins for correlated subqueries
- FILTER clause for conditional aggregation
- Generate_series for sequence generation`,
					"mysql": `**Dialect: MySQL**
Use MySQL-specific syntax and features where appropriate:
- Backtick quoting for identifiers
- LIMIT syntax (LIMIT offset, count)
- GROUP_CONCAT for string aggregation
- IF() and IFNULL() functions
- AUTO_INCREMENT for identity columns
- STRAIGHT_JOIN hint when optimizer chooses wrong plan
- INSERT ... ON DUPLICATE KEY UPDATE for upserts
- Use DATE_FORMAT, STR_TO_DATE for date handling
- INDEX hints (USE INDEX, FORCE INDEX) when needed
- Note: window functions require MySQL 8.0+`,
					"sqlite": `**Dialect: SQLite**
Use SQLite-specific syntax and be aware of limitations:
- Limited type system (TEXT, INTEGER, REAL, BLOB, NULL)
- No RIGHT JOIN or FULL OUTER JOIN — rewrite with LEFT JOIN
- No ALTER TABLE DROP COLUMN (prior to 3.35.0)
- Window functions require SQLite 3.25.0+
- Use GROUP_CONCAT for string aggregation
- UPSERT via INSERT OR REPLACE or ON CONFLICT
- Date/time functions: date(), time(), datetime(), strftime()
- No native BOOLEAN type — use INTEGER 0/1
- AUTOINCREMENT keyword (different from AUTO_INCREMENT)
- Be conservative with advanced features — note version requirements`,
					"mssql": `**Dialect: SQL Server (MSSQL)**
Use SQL Server-specific syntax and features where appropriate:
- Square bracket quoting for identifiers ([table].[column])
- TOP instead of LIMIT (SELECT TOP 10 ...)
- CROSS APPLY and OUTER APPLY for correlated table-valued functions
- STRING_AGG for string aggregation (SQL Server 2017+)
- FORMAT function for date/number formatting
- OFFSET ... FETCH NEXT for pagination (SQL Server 2012+)
- MERGE statement for upserts
- TRY_CAST, TRY_CONVERT for safe type conversion
- CTE and window functions fully supported
- ISNULL() instead of IFNULL() or COALESCE (though COALESCE is preferred for portability)`,
				},
			},

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "explain-plan",
					Text:        "Add an EXPLAIN ANALYZE prefixed version of the query and explain the execution plan. Identify potential full table scans, index usage, join strategies, and estimated vs actual row counts. Suggest optimizations based on the plan.",
					Description: "Follow-up: generate EXPLAIN ANALYZE and interpret the execution plan",
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add a natural language query request or schema notes"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "dialect",
					Type:        "custom",
					Description: "Set SQL dialect",
					Usage:       "dialect <auto|postgresql|mysql|sqlite|mssql>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current dialect: " + (state.get(stateKeys.dialect) || "auto"));
                            output.print("Available dialects: auto, postgresql, mysql, sqlite, mssql");
                            return;
                        }
                        var dialect = args[0].toLowerCase();
                        var validDialects = ["auto", "postgresql", "mysql", "sqlite", "mssql"];
                        if (validDialects.indexOf(dialect) === -1) {
                            output.print("Invalid dialect. Available: " + validDialects.join(", "));
                            return;
                        }
                        state.set(stateKeys.dialect, dialect);
                        output.print("SQL dialect set to: " + dialect);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Report Analyzer Goal
		{
			Name:        "report-analyzer",
			Description: "Extract insights, risks, and key information from long documents into concise memos",
			Category:    "business-analysis",
			Usage:       "Analyzes long corporate reports, technical documents, or research papers and produces structured memos with key findings, risks, and recommendations",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Report Analyzer",
			TUIPrompt: "(report-analyzer) > ",

			NotableVariables: []string{"focus"},

			StateVars: map[string]any{
				"focus": "general",
			},

			PromptInstructions: `Read the entire document provided and produce a structured analysis memo with a {{.stateKeys.focus}} focus.

{{.focusInstructions}}

---

## Memo Structure

### 1. Executive Summary
2-3 sentences capturing the document's core message, most important finding, and overall assessment. A busy executive should be able to read only this section and understand the key takeaway.

### 2. Key Findings
Bulleted list of the most significant findings, insights, or data points from the document. For each:
- **Finding**: clear, specific statement of the finding
- **Evidence**: cite the specific section, page, or data point that supports it
- **Significance**: why this matters — what are the implications?

### 3. Risks & Concerns
Categorize each risk by severity:
- 🔴 **HIGH**: immediate threats or critical issues requiring urgent attention
- 🟡 **MEDIUM**: significant concerns that need monitoring or near-term action
- 🟢 **LOW**: minor issues or long-term considerations

For each risk: describe the risk, its potential impact, likelihood, and any mitigating factors mentioned in the document.

### 4. Opportunities
Positive developments, growth areas, or strategic advantages identified in the document. Be specific about what the opportunity is and what would be needed to capitalize on it.

### 5. Financial Highlights
(If applicable — skip if the document contains no financial information)
- Revenue, profit, margin, and cash flow figures with period-over-period comparisons
- Key financial ratios or metrics
- Forward-looking projections or guidance
- Notable changes in financial position

### 6. Strategic Implications
What does this document mean for strategic decision-making? Connect the findings to broader context:
- How does this affect competitive positioning?
- What trends does this confirm or contradict?
- What decisions should this information inform?

### 7. Recommended Actions
Concrete, actionable recommendations based on the analysis. Each should specify:
- **Action**: what specifically should be done
- **Rationale**: why, based on the document's content
- **Urgency**: immediate, near-term, or long-term

### 8. Open Questions
Questions raised by the document that are not answered within it. Flag information gaps, unclear statements, or areas requiring further investigation.

---

**Analysis Standards:**
- Cite specific sections, pages, or paragraphs when referencing document content
- Quantify claims wherever the document provides numbers — avoid vague language when data exists
- Clearly distinguish between facts stated in the document and your own interpretive analysis
- Flag contradictions within the document (e.g., optimistic narrative vs concerning data)
- Maintain analytical objectivity — do not editorialize beyond what the evidence supports`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "REPORT / DOCUMENT",

			PromptOptions: map[string]any{
				"focusInstructions": map[string]string{
					"general": `**Focus: General Analysis**
Perform a balanced analysis covering all aspects of the document. Weight all sections of the memo equally and surface whatever is most significant regardless of category.`,
					"financial": `**Focus: Financial Analysis**
Prioritize financial metrics, revenue trends, cost structures, margins, cash flow dynamics, and financial projections. Pay special attention to:
- Year-over-year and quarter-over-quarter comparisons
- Key financial ratios (P/E, debt-to-equity, current ratio, etc.)
- Revenue mix and segment performance
- CapEx vs OpEx trends
- Working capital changes
- Guidance and forward-looking statements vs actual performance`,
					"risk": `**Focus: Risk Analysis**
Prioritize identification of risks, threats, compliance issues, and operational concerns. Perform a thorough risk assessment:
- Regulatory and compliance risks
- Operational risks (supply chain, key personnel, technology)
- Market and competitive risks
- Financial risks (liquidity, credit, currency)
- Reputational risks
- Emerging risks not yet fully materialized
Weight the Risks & Concerns section most heavily in your output.`,
					"strategic": `**Focus: Strategic Analysis**
Prioritize strategy, competitive positioning, market trends, and growth opportunities. Evaluate:
- Competitive moat and differentiation
- Market share trends and total addressable market
- Strategic initiatives and their progress
- M&A activity or potential
- Partnership and ecosystem development
- Innovation pipeline and R&D direction
Weight the Strategic Implications and Opportunities sections most heavily.`,
					"technical": `**Focus: Technical Analysis**
Prioritize technical architecture, capabilities, limitations, and scalability. Evaluate:
- Technical architecture and design decisions
- Performance characteristics and benchmarks
- Scalability constraints and growth capacity
- Technical debt and maintenance burden
- Integration capabilities and API surface
- Security posture and vulnerability surface
- Technology stack choices and their implications`,
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add notes about what to focus on or questions to answer"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "focus",
					Type:        "custom",
					Description: "Set analysis focus area",
					Usage:       "focus <general|financial|risk|strategic|technical>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current focus: " + (state.get(stateKeys.focus) || "general"));
                            output.print("Available focus areas: general, financial, risk, strategic, technical");
                            return;
                        }
                        var focus = args[0].toLowerCase();
                        var validFocuses = ["general", "financial", "risk", "strategic", "technical"];
                        if (validFocuses.indexOf(focus) === -1) {
                            output.print("Invalid focus. Available: " + validFocuses.join(", "));
                            return;
                        }
                        state.set(stateKeys.focus, focus);
                        output.print("Analysis focus set to: " + focus);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Review Classifier Goal
		{
			Name:        "review-classifier",
			Description: "Categorize feedback and reviews with sentiment analysis",
			Category:    "product-analysis",
			Usage:       "Analyzes user feedback, product reviews, or support tickets and categorizes them into structured tags with positive/negative/neutral sentiment per category",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Review Classifier",
			TUIPrompt: "(review-classifier) > ",

			NotableVariables: []string{"outputFormat"},

			StateVars: map[string]any{
				"outputFormat": "detailed",
			},

			PromptInstructions: `Analyze the provided feedback and classify it into structured categories with sentiment analysis.

{{.outputFormatInstructions}}

---

## Classification Process

### Step 1: Read & Segment
- Read each piece of feedback in its entirety
- Segment multi-topic feedback into individual topic units
- Identify the primary subject of each segment

### Step 2: Categorize
Assign each segment to one or more categories from this taxonomy:

**Product Features**
- Core Functionality — primary features and capabilities
- New Features — feature requests or reactions to new additions
- Missing Features — gaps compared to expectations or competitors
- Feature Quality — depth, polish, and completeness of existing features

**User Experience**
- Onboarding — first-time setup, learning curve, getting started
- Navigation — finding features, information architecture, menu structure
- Design & Aesthetics — visual design, layout, branding
- Accessibility — disability access, internationalization, readability

**Performance**
- Speed — response times, loading, lag
- Reliability — crashes, errors, downtime, data loss
- Scalability — behavior under load, large data sets, concurrent users

**Customer Support**
- Response Time — speed of support responses
- Resolution Quality — effectiveness of solutions provided
- Communication — clarity, tone, helpfulness of support interactions
- Self-Service — documentation, FAQs, knowledge base quality

**Pricing**
- Value for Money — perceived value relative to cost
- Plan Structure — tier design, feature gating, upgrade path
- Billing — invoicing, payment methods, refund experience

**Security**
- Data Privacy — data handling, GDPR, permissions
- Authentication — login, MFA, SSO experience
- Trust — confidence in the product's security posture

**Mobile**
- App Quality — native app experience, responsiveness
- Cross-Platform — consistency between mobile and desktop
- Offline — functionality without connectivity

**Integrations**
- Third-Party — connections with other tools and services
- API — developer experience, documentation, reliability
- Import/Export — data portability, migration tools

### Step 3: Sentiment Analysis
For each category assignment, determine sentiment:
- **Positive** 👍 — praise, satisfaction, delight
- **Negative** 👎 — complaint, frustration, disappointment
- **Neutral** 😐 — observation, question, suggestion without clear sentiment

### Step 4: Extract Evidence
For each categorization, identify the specific verbatim quote or paraphrase that supports the classification. This creates an audit trail for the analysis.

### Step 5: Synthesize
- Identify recurring themes across all feedback
- Calculate sentiment distribution (% positive, negative, neutral)
- Determine the most-discussed categories
- Surface actionable insights: what should be improved, what should be preserved, what should be built

---

**Handling Edge Cases:**
- **Mixed sentiment**: a single segment can praise one aspect while criticizing another — split into separate category assignments
- **Implicit complaints**: "I wish it could..." or "It would be nice if..." — classify as negative/feature request even without explicit negativity
- **Feature requests vs bug reports**: distinguish between "this doesn't work" (bug → negative) and "I want this to exist" (feature request → neutral/negative)
- **Sarcasm**: look for tonal indicators; when uncertain, flag as ambiguous
- **Comparative feedback**: "better than X but worse than Y" — note the comparison context`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "FEEDBACK / REVIEWS",

			PromptOptions: map[string]any{
				"outputFormatInstructions": map[string]string{
					"detailed": `**Output Format: Detailed**
Provide full analysis with:
- Each feedback item analyzed individually with verbatim quotes
- Category assignments with reasoning for each classification
- Confidence level for each sentiment judgment (high/medium/low)
- Cross-references between related feedback items
- Comprehensive summary with statistical breakdown`,
					"summary": `**Output Format: Summary Table**
Provide a condensed table format:

| # | Category | Subcategory | Sentiment | Key Point | Source |
|---|----------|-------------|-----------|-----------|--------|
| 1 | UX       | Navigation  | 👎 Negative | Hard to find settings | Review #3 |

Follow the table with a brief (3-5 sentence) overall summary and top 3 actionable recommendations.`,
					"json": `**Output Format: Structured JSON**
Output as a valid JSON object with this structure:
` + "```json" + `
{
  "totalItems": 0,
  "sentimentSummary": { "positive": 0, "negative": 0, "neutral": 0 },
  "items": [
    {
      "id": 1,
      "source": "Review #1",
      "categories": [
        {
          "category": "Product Features",
          "subcategory": "Core Functionality",
          "sentiment": "positive",
          "confidence": "high",
          "quote": "verbatim text",
          "insight": "brief interpretation"
        }
      ]
    }
  ],
  "themes": ["theme1", "theme2"],
  "recommendations": ["action1", "action2"]
}
` + "```",
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add notes about product context or classification priorities"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "output-format",
					Type:        "custom",
					Description: "Set output format for classification results",
					Usage:       "output-format <detailed|summary|json>",
					Handler: `function (args) {
                        if (args.length === 0) {
                            output.print("Current format: " + (state.get(stateKeys.outputFormat) || "detailed"));
                            output.print("Available formats: detailed, summary, json");
                            return;
                        }
                        var fmt = args[0].toLowerCase();
                        var validFormats = ["detailed", "summary", "json"];
                        if (validFormats.indexOf(fmt) === -1) {
                            output.print("Invalid format. Available: " + validFormats.join(", "));
                            return;
                        }
                        state.set(stateKeys.outputFormat, fmt);
                        output.print("Output format set to: " + fmt);
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},

		// Adaptive Editor Goal
		{
			Name:        "adaptive-editor",
			Description: "Rewrite text following specific instructions for tone, audience, or style",
			Category:    "writing",
			Usage:       "Rewrites provided text according to specified instructions — change tone, simplify for a different audience, translate register, or apply specific style guidelines",
			Script:      goalScript,
			FileName:    "goal.js",

			TUITitle:  "Adaptive Editor",
			TUIPrompt: "(adaptive-editor) > ",

			NotableVariables: []string{"instruction"},

			StateVars: map[string]any{
				"instruction": "",
			},

			PromptInstructions: `Rewrite the provided text following the user's editing instructions.

{{if .stateKeys.instruction}}**Active Instruction:** {{.stateKeys.instruction}}{{else}}**⚠ No instruction set.** Use the ` + "`instruct`" + ` command to set the rewriting instruction before copying the prompt. Example: ` + "`instruct simplify for a non-technical audience`" + `{{end}}

---

## Rewriting Guidelines

### Core Principles
- **Preserve core meaning**: the rewritten text must convey the same essential information and arguments as the original
- **Maintain factual accuracy**: do not introduce, remove, or alter facts, figures, dates, or claims
- **Adapt the vessel, not the cargo**: change vocabulary, sentence structure, tone, and register to match the instruction — but the underlying message stays the same
- **Preserve proper nouns and technical terms** unless the instruction specifically asks to simplify or replace them
- **Maintain document structure**: preserve headings, lists, numbered items, and other structural elements unless the instruction calls for restructuring

### Adaptation Dimensions
Depending on the instruction, you may need to adjust one or more of these:

**Vocabulary Level**
- Simplify: replace jargon with everyday language, define necessary technical terms inline
- Elevate: introduce precise terminology, domain-specific vocabulary, formal register
- Match audience: use vocabulary appropriate for the target reader (child, executive, specialist, general public)

**Sentence Structure**
- Simplify: shorter sentences, active voice, one idea per sentence, minimal subordinate clauses
- Formalize: more complex constructions where they aid precision, proper use of semicolons and em dashes
- Match reading level: adjust Flesch-Kincaid level to match the target audience

**Tone & Register**
- Formal ↔ Informal: adjust contractions, colloquialisms, directness
- Serious ↔ Humorous: adjust wit, levity, gravity as instructed
- Objective ↔ Persuasive: adjust from neutral reporting to advocacy or vice versa
- Confident ↔ Cautious: adjust hedging language, certainty markers

**Length & Density**
- Concise: reduce word count while preserving all key points — target minimum 20% reduction
- Expanded: add explanations, examples, transitions, and context to aid understanding
- Maintain: keep approximately the same length, changing only the expression

### Common Instruction Types
These are examples of instructions you should be able to handle:
- "Simplify for a 10-year-old"
- "Make this suitable for a board presentation"
- "Rewrite as a casual blog post"
- "Add humor while keeping it professional"
- "Translate from marketing-speak to technical documentation"
- "Make this more persuasive"
- "Adjust to a 6th-grade reading level"
- "Remove all jargon"
- "Make this sound like [specific author/publication style]"

---

**Output Format:**

1. **Rewritten Text** — the complete rewritten version, ready to use
2. **Change Summary** — brief notes on what was adapted and why:
   - What vocabulary changes were made
   - How sentence structure was adjusted
   - What tone/register shifts were applied
   - Approximate reading level of the result
   - Word count: original vs rewritten`,

			PromptTemplate: `**{{.description | upper}}**

{{.promptInstructions}}

## {{.contextHeader}}

{{.contextTxtar}}`,

			ContextHeader: "TEXT TO REWRITE",

			HotSnippets: []GoalHotSnippet{
				{
					Name:        "compare-versions",
					Text:        "Show the original and rewritten text side-by-side with annotations highlighting what changed and why. For each significant change, explain the reasoning behind the adaptation.",
					Description: "Follow-up: side-by-side comparison with change annotations",
				},
			},

			Commands: []CommandConfig{
				{Name: "add", Type: "contextManager"},
				{Name: "diff", Type: "contextManager"},
				{Name: "note", Type: "contextManager", Description: "Add notes about target audience or style preferences"},
				{Name: "list", Type: "contextManager"},
				{Name: "edit", Type: "contextManager"},
				{Name: "remove", Type: "contextManager"},
				{Name: "show", Type: "contextManager"},
				{Name: "copy", Type: "contextManager"},
				{
					Name:        "instruct",
					Type:        "custom",
					Description: "Set the rewriting instruction",
					Usage:       "instruct <instruction text>",
					Handler: `function (args) {
                        var current = state.get(stateKeys.instruction) || "";
                        var text;
                        if (args.length === 0) {
                            var edited = ctxmgr.openEditor("rewriting-instruction", current);
                            text = (edited || "").trim();
                        } else {
                            text = args.join(" ").trim();
                        }
                        if (!text) {
                            var wasSet = current !== "";
                            if (wasSet) {
                                state.set(stateKeys.instruction, "");
                                output.print("Instruction cleared");
                            } else {
                                output.print("Instruction not updated (no content provided).");
                            }
                            return;
                        }
                        if (text !== current) {
                            state.set(stateKeys.instruction, text);
                            output.print("Instruction set to: " + text);
                        } else {
                            output.print("Instruction unchanged");
                        }
                    }`,
				},
				{Name: "help", Type: "help"},
			},
		},
	}
}
