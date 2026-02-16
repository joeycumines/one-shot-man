---
title: Goal reference
description: Goal reference for `osm goal` (built-in and discovered)
tags:
  - goal
  - goals
  - osm goal
  - goal discovery
---

# Goal reference

This document describes the implementation and behavior of the `goal` feature: built-in goals, user-defined goals, discovery rules, TUI interaction, session/storage behavior, configuration options, validation, and authoring patterns.

**Quick summary**

- `osm goal` lists known goals and runs pre-written “goal” modes (interactive workflows) for common engineering tasks.
- Built-in goals are provided by the application and can be extended or overridden by user goal JSON files discovered by the goal discovery engine.
- The base behavior is implemented as a JavaScript "loader script" - embedded in `osm` - and drives all goal modes, by consuming the declarative `Goal` configuration, provided by Go.

See the implementation in: [internal/command/goal.go](../../internal/command/goal.go), [internal/command/goal_loader.go](../../internal/command/goal_loader.go), and the base behavior in [internal/command/goal.js](../../internal/command/goal.js).

---

**Usage (command-line)**

- List goals:

    - `osm goal -l`
    - `osm goal -c <category>` (filter by category)

- Run a goal interactively (default when supplying the positional name):

    - `osm goal <goal-name>`

- Run a goal directly (non-interactive):

    - `osm goal -r <goal-name>`

- Optional runtime flags (common to other interactive commands):

    - `-i` run in interactive mode (if omitted `-r` implies non-interactive)
    - `-session <id>` explicitly set session id used for persistence
    - `-store <fs|memory>` select storage backend for session persistence (default: `fs`)

---

**High level architecture**

1. Goals can come from: built-in list (compiled into the binary), or discovered JSON files under configured directories.
2. At startup (and on `Reload`), discovered goal files are loaded, validated, and merged with built-ins. User-defined goals override built-ins on name collisions.
3. When a goal is invoked the CLI marshals the `Goal` struct to JSON, injects it into the embedded JavaScript runtime as `GOAL_CONFIG`, and executes the interpreter script (`goal.js`) to register a TUI mode.
4. Interactive runs will switch the active TUI mode to the goal (causing `onEnter` hooks to run). Non-interactive `-r` runs still execute the script to perform any side-effects but do not switch to the TUI unless explicitly asked.

Registry + discovery code: [internal/command/goal_registry.go](../../internal/command/goal_registry.go) and [internal/command/goal_discovery.go](../../internal/command/goal_discovery.go).

---

**Goal data model (JSON)**

Goals are defined by the `Goal` struct implemented in Go and marshalled to/from JSON by `LoadGoalFromFile`.

Key fields:

- `name` (string, required): short identifier for the goal. Must validate against the `isValidGoalName` rule (alnum+hyphen; starts with alnum). See validation in [internal/command/goal_loader.go](../../internal/command/goal_loader.go#L20-L40).

- `description` (string, required): short human description.

- `category` (string, optional): category name for grouping in `osm goal -l`.

- `usage` (string, optional): short usage string, user-facing.

- `script` (string, optional): embedded JavaScript interpreter content for the goal. If absent, the default embedded interpreter `goal.js` is used. Note: there is NO `ScriptFile` support for loading an external JS file from disk (the loader currently prefers inline script or default interpreter). See `resolveGoalScript` [internal/command/goal_loader.go](../../internal/command/goal_loader.go#L44-L68).

- `fileName` (string, computed): the basename of the JSON file used to load the goal. (Set during LoadGoalFromFile.)

TUI-specific fields:

- `tuiTitle` (string): title for the TUI mode banner. Defaults to `name` if not provided.
- `tuiPrompt` (string): prompt string (e.g., `(doc-gen) > `).

UI text fields:

- `bannerTemplate` (string, optional): custom banner printed on entry to the goal mode. Overrides the default auto-generated banner that prints the goal name, description, and notable variables. Use this when you want full control over the initial message shown to users.

- `usageTemplate` (string, optional): custom text appended after the default help output when the user types `help`. Useful for documenting goal-specific commands and conventions without replacing the built-in help entirely.

State & prompt building:

- `stateVars` (`object`): initial state for named keys used by goal templates. These are injected into `tui.createState` as default values.

- `notableVariables` (`string[]`): names of state keys that will be printed in the banner (e.g. `type`, `framework`).

- `contextHeader` (string): headline used in final prompts for the context block (e.g. "CODE TO ANALYZE").

- `promptInstructions` (string): instructions (a template string) to be rendered prior to the final prompt. This is itself rendered as a template to allow dynamic substitution via template data.

- `promptTemplate` (string): the primary template for the final prompt (a go-style template string). Contains placeholders such as `{{.description}}`, `{{.promptInstructions}}`, `{{.contextTxtar}}`, etc. See `goal.js` for details on available template data.

- `promptOptions` (`object`): further directives used to influence prompt generation, and especially dynamic templates keyed to state values (like `typeInstructions` mapping state `type` -> instruction string).

- `promptFooter` (string, optional): footer text appended after context in the final prompt. This string is itself rendered as a template (like `promptInstructions`), so it can reference state variables (e.g., `{{index .stateKeys "outputFormat"}}`). The rendered result is available as `{{.promptFooter}}` in the `promptTemplate`. Useful for adding reminders, constraints, or follow-up instructions that should appear at the end of the generated prompt.

- `postCopyHint` (string, optional): if set, this text is printed to the terminal after the user successfully copies the prompt to clipboard. Useful for suggesting follow-up actions (e.g., "Try pasting into Claude and asking it to review the output.").

Hot-snippets:

- `hotSnippets` (array of GoalHotSnippet, optional): embedded hot-snippets for the goal. Each entry defines a reusable text snippet that is registered as a `hot-<name>` command in the goal's interactive mode.

    Each `GoalHotSnippet` has:
    - `name` (string, required): the snippet name. Registered as the command `hot-<name>`.
    - `text` (string, required): the text that is copied to clipboard when the command runs.
    - `description` (string, optional): shown in help output.

    **Merge behavior:** Goal-defined hot-snippets are merged with hot-snippets defined in the config file (`[hot-snippets]` section). When both sources define a snippet with the same name, the **goal-defined snippet takes precedence** — this allows goals to provide specialized overrides for shared snippets.

Commands:

- `commands` (array of CommandConfig): each command is described by an object with:
    - `name` (string): command name
    - `type` (string): `contextManager` | `custom` | `help`
    - `description` / `usage` (strings)
    - `argCompleters` (array[string]): completer keys (e.g. `file`) provided to the TUI.
    - `flagDefs` (array of `CommandFlagDef`, optional): flag definitions for tab-completion in the interactive TUI. Each entry has `name` (string) and `description` (string). These are used by the REPL completer to suggest flags (e.g., `-format`, `-sort`) when the user presses Tab after a command name. The `osm:flag` module can also consume these definitions.
    - `handler` (string): for `custom` commands, a JS function body (or `function (...) { ... }`) provided as source string. This is lazily converted into a function and executed in the interpreter.

Example:

```json
{
  "name": "doc-generator",
  "description": "Generate detailed project documentation",
  "category": "documentation",
  "tuiTitle": "Document Generator",
  "tuiPrompt": "(doc-gen) > ",
  "bannerTemplate": "Welcome to Doc Generator\nType 'help' for commands.",
  "usageTemplate": "  type <kind>   Set documentation kind (api, readme, tutorial)",
  "postCopyHint": "[Hint] Paste into your LLM and ask for inline code examples.",
  "stateVars": {
    "type": "comprehensive"
  },
  "notableVariables": [
    "type"
  ],
  "promptInstructions": "Create {{.stateKeys.type}} documentation for the codebase.",
  "promptFooter": "Remember: all code examples must be runnable.",
  "promptTemplate": "**{{.description | upper}}**\n\n{{.promptInstructions}}\n\n## {{.contextHeader}}\n\n{{.contextTxtar}}{{if .promptFooter}}\n\n---\n{{.promptFooter}}{{end}}",
  "promptOptions": {
    "typeInstructions": {
      "comprehensive": "Generate comprehensive documentation including...",
      "api": "Focus on API surface and examples"
    }
  },
  "hotSnippets": [
    {
      "name": "review-docs",
      "text": "Review the generated documentation for accuracy and completeness.",
      "description": "Follow-up: review generated docs"
    }
  ],
  "commands": [
    {
      "name": "add",
      "type": "contextManager"
    },
    {
      "name": "help",
      "type": "help"
    },
    {
      "name": "publish",
      "type": "custom",
      "description": "Publish generated docs",
      "flagDefs": [
        { "name": "format", "description": "Output format (html, pdf)" }
      ],
      "handler": "function (args) { output.print('Publishing docs...'); }"
    }
  ]
}
```

The above sample mirrors the built-in `doc-generator` goal (with added illustration of new fields). See the built-in goals defined in: [internal/command/goal_builtin.go](../../internal/command/goal_builtin.go).

---

**Loading & validation**

- Files discovered by `FindGoalFiles` are any files with a `.json` suffix found in goal directories (case-insensitive extension matching). See [internal/command/goal_loader.go](../../internal/command/goal_loader.go#L84-L120).

- `LoadGoalFromFile(path)` reads, parses and validates a JSON file. Key validation: `name` required, `description` required, `name` must match a safe regex (alphanumeric + hyphens, no spaces). When the file is loaded it sets `FileName` and resolves the `Script` (defaulting to the embedded `goal.js` interpreter if `Script` is not set in the JSON). See [internal/command/goal_loader.go](../../internal/command/goal_loader.go).

- If JSON parsing or validation fails, `LoadGoalFromFile` returns an error and the registry logs a warning; the failing goal is skipped.

---

**Discovery & path resolution**

`GoalDiscovery` (constructed via `NewGoalDiscovery(cfg)`) builds candidate goal directories based on a mix of standard locations, configured paths, and autodiscovered directories. Key behaviors and configuration:

- Default configuration:
    - `goal.autodiscovery` defaults to `true` (enable traversal and pattern matching beyond standard locations).
    - Standard patterns `goal.path-patterns` default to `osm-goals,goals`.
    - `goal.max-traversal-depth` default is `10`.

- Standard paths (unless `goal.disable-standard-paths` is set):
    1. The `goals/` directory next to the config file (e.g., `~/.one-shot-man/goals/`)
    2. The `goals/` directory next to the executable directory (e.g., `/usr/local/bin/goals/`)
    3. `./osm-goals/` in the current working directory

- `goal.paths` can be set to a list of additional paths (comma-separated or using OS list separator).

- When `goal.autodiscovery` is enabled, the engine traverses upward from current working directory up to `goal.max-traversal-depth` and collects directories with names matching `goal.path-patterns`. This enables repo-local goal directories (e.g., `./osm-goals/` in a monorepo) to be discovered automatically.

- `OSM_DISABLE_GOAL_AUTODISCOVERY=true` will disable automatic traversal regardless of config.

- Paths are normalized and deduplicated — symlinks are resolved and only the first unique real path is kept.

- `DiscoverGoalPaths` returns paths sorted by priority (closest to CWD first), computed using `computePathScore` which classifies paths as:
    - Class 0: CWD descendants (closest priority)
    - Class 1: Ancestor directories matching configured patterns
    - Class 2: Config directory `~/.one-shot-man/goals`
    - Class 3: Executable directory `goals/` next to executable
    - Class 4: Other paths

See [internal/command/goal_discovery.go](../../internal/command/goal_discovery.go) for the full algorithm and sorting details.

---

**Goal registry & precedence**

- `DynamicGoalRegistry` merges built-in goals (compiled-in) and user-defined discovered goals. User goals (discovered) win on name collisions by overriding built-in definitions.

- The registry performs discovery scanning in priority order; when multiple discovered paths contain the same goal name, the first (closest/higher-priority) wins.

- `Reload()` re-discover and re-merge. This allows hot-reload in long-running processes or tests where config changes.

See [`internal/command/goal_registry.go`](../../internal/command/goal_registry.go) and associated tests in [internal/command/goal_registry_test.go](../../internal/command/goal_registry_test.go).

---

**Prompt file (`.prompt.md`) discovery**

In addition to JSON goal files, `osm` can discover and load `.prompt.md` files — the same format used by VS Code's prompt file feature. This allows sharing prompt files across tools.

How it works:

1. **Discovery locations.** `.prompt.md` files are scanned in two sets of directories:
    - **Goal directories** (the same directories that contain `.json` goal files). `.prompt.md` files found here are loaded alongside JSON goals.
    - **Dedicated prompt file paths.** By default this includes `.github/prompts` relative to the current working directory (the VS Code standard location). Additional paths can be configured via `prompt.file-paths` in the config file.

2. **File format.** A `.prompt.md` file consists of optional YAML frontmatter and a Markdown body:

    ```markdown
    ---
    name: my-prompt
    description: A shared prompt for code review
    model: gpt-4
    tools: [codebase, terminal]
    ---

    Review the provided code for correctness and style issues.
    Focus on error handling and edge cases.
    ```

    Supported frontmatter keys:
    - `name` (string): goal name. Falls back to the filename stem if omitted (e.g., `review.prompt.md` → `review`).
    - `description` (string): goal description. Falls back to `"Imported from <filename>"`.
    - `model` (string): stored in `promptOptions["model"]` (informational; `osm` does not call APIs).
    - `tools` (list of strings): stored in `promptOptions["tools"]` (informational).

3. **Conversion to Goals.** Each `.prompt.md` file is converted to a `Goal` with:
    - `category` set to `"prompt-file"`
    - The Markdown body becomes `promptInstructions`
    - Standard `contextManager` commands (`add`, `note`, `list`, `edit`, `remove`, `show`, `copy`) plus `help`
    - The default `promptTemplate` and embedded `goalScript` interpreter

4. **File reference expansion.** Markdown links of the form `[text](relative/path.ext)` where the target file exists on disk are expanded inline — the linked file's content is embedded as a fenced code block. This matches VS Code's behavior for prompt file references.

5. **Name sanitization.** The filename is sanitized to a valid goal name: non-alphanumeric characters become hyphens, consecutive hyphens are collapsed, and leading/trailing hyphens are trimmed. Example: `My Cool Prompt.prompt.md` → `My-Cool-Prompt`.

6. **Precedence.** `.prompt.md` goals participate in the same precedence rules as JSON goals:
    - User-discovered goals (JSON or `.prompt.md`) override built-in goals on name collision.
    - Among discovered goals, the first occurrence (from the highest-priority path) wins.
    - Within a single directory, JSON goals are scanned before `.prompt.md` files.

Configuration keys:
- `prompt.file-paths` (list): additional directories to search for `.prompt.md` files.
- `goal.disable-standard-paths` (bool): when true, also skips `.github/prompts`.

See: [internal/command/prompt_file.go](../../internal/command/prompt_file.go), [internal/command/goal_registry.go](../../internal/command/goal_registry.go).

---

**Built-in goals (complete catalog)**

`osm` ships with 14 built-in goals across 13 categories. All use the standard `goalScript` interpreter and provide `contextManager` commands for managing context items.

| Name | Category | Description | State Variables | Custom Commands |
|------|----------|-------------|-----------------|-----------------|
| `comment-stripper` | code-refactoring | Remove useless comments and refactor useful ones | — | — |
| `doc-generator` | documentation | Generate comprehensive documentation for code structures | `type` (comprehensive) | `type` |
| `test-generator` | testing | Generate comprehensive test suites for existing code | `type` (unit), `framework` (auto) | `type`, `framework` |
| `commit-message` | git-workflow | Generate Kubernetes-style commit messages from diffs and context | — | — |
| `morale-improver` | meta-prompting | Generate a derisive prompt to force task completion | `originalInstructions`, `failedPlan`, `specificFailures` | `set-original`, `set-plan`, `set-failures` |
| `implementation-plan` | planning | Prepare a detailed, explicit implementation plan | `goalText` | `goal` |
| `bug-buster` | code-quality | Detect and fix bugs in code | — | — |
| `code-optimizer` | code-quality | Suggest performance optimizations for code | — | — |
| `code-explainer` | code-understanding | Explain code in plain language for onboarding | `depth` (detailed) | `depth` |
| `meeting-notes` | productivity | Generate structured meeting summaries with action items | — | — |
| `pii-scrubber` | data-privacy | Redact PII from code, logs, and data | `level` (strict) | `level` |
| `prose-polisher` | writing | Seven-step copyediting pipeline for prose | `style` (technical) | `style` |
| `data-to-json` | data-transformation | Extract structured JSON from unstructured text | `mode` (auto) | `mode` |
| `cite-sources` | research | Answer questions with numbered inline citations | `format` (numbered) | `format` |

**Goals with hot-snippets:**

- `commit-message`: `hot-review-response` — "Review the commit message you generated…"
- `morale-improver`: `hot-review-plan` — "Now review each section of your plan…"; `hot-prove-it` — "Prove the issue exists by reproducing it…"
- `prose-polisher`: `hot-expand-section` — "The section I highlighted is too thin. Expand it…"
- `cite-sources`: `hot-challenge-claims` — "Review your answer critically. For each claim, verify the citation…"

**Goals with PostCopyHint:**

- `morale-improver`: suggests a follow-up review prompt after copying.

**Goals with BannerTemplate / UsageTemplate:**

- `implementation-plan`: uses custom `bannerTemplate` ("Implementation Plan Generator…") and `usageTemplate` (lists all available commands).

**Goals with NotableVariables (printed in banner):**

- `test-generator`: `type`, `framework`
- `code-explainer`: `depth`
- `morale-improver`: `originalInstructions`, `failedPlan`, `specificFailures`
- `pii-scrubber`: `level`
- `prose-polisher`: `style`
- `data-to-json`: `mode`
- `cite-sources`: `format`

See: [internal/command/goal_builtin.go](../../internal/command/goal_builtin.go) for the full definitions.

---

**TUI & session persistence**

- Goals are executed in the embedded JavaScript runtime (Goja). The CLI constructs an engine via `scripting.NewEngineWithConfig(ctx, stdout, stderr, session, store)` which creates a `TUIManager` with explicit session id and storage backend.
    - `session` (from `--session` or env `OSM_SESSION`) overrides session discovery.
    - `store` selects backend; recognized values: `fs` (filesystem-based persistent sessions) and `memory` (ephemeral in-memory sessions used for tests or ephemeral runs). See [internal/storage/registry.go](../../internal/storage/registry.go).

- The `TUIManager` creates or attaches a `StateManager` for the session, which stores `contextItems`, prompts/histories, and mode-specific state. The manager captures history snapshots automatically when commands are executed and `enableHistory` is true.

- `TUI` behavior:
    - On mode `onEnter` the `goal.js` interpreter prints the `bannerTemplate` and runs the `OnEnter` hooks which are used by built-ins to print the banner and notable variables.
    - `SwitchMode` prints a `Switched to mode: <name>` message, restores session context (rehydrates `contextItems`), and then executes the mode `onEnter` callback.
    - `help` is built-in and the `UsageTemplate` from the `Goal` is appended to the default help output.

See [internal/scripting/tui_manager.go](../../internal/scripting/tui_manager.go) and `NewTUIManagerWithConfig` for persistence and session creation.

---

**Prompt building & dynamic instructions**

- The generic `goal.js` interpreter combines several pieces of data into final prompts:
    - State values from `stateVars` via `tui.createState`
    - `contextItems` (files/diffs/notes) built by `contextManager`
    - `promptInstructions` — a template rendered for inclusion inside the final prompt
    - `promptTemplate` — primary template for the final prompt itself

- `promptOptions` supports a small but powerful dynamic mapping convention: any map keyed by `XInstructions` is considered as `XInstructions`, where `X` is a state key. If a `stateKeys.X` is present and its value matches a key in `XInstructions`, the interpreter injects that instruction into template data as `XInstructions` and the template may reference it as `{{.typeInstructions}}` or similar.

- Template functions: the interpreter exposes small helper functions (`upper` etc.) and provides the current state keys via `templateData.stateKeys` for use in templates.

See `goal.js` to understand how `buildPrompt` and `buildBaseTemplateData` construct the final prompt.

---

**Commands and context manager**

- Goals commonly use the `contextManager` factory (JS) for managing context items (files, diffs, notes). The `contextManager` provides common commands: `add`, `diff`, `note`, `list`, `edit`, `remove`, `show`, `copy`, and they are available to goals via `CommandConfig` entries with `type: "contextManager"`.

- `contextManager` implements built-in behavior for handling files, lazy diffs (diff performed only when generating prompts), and editors/clipboard via helpers exposed by native modules. See [internal/builtin/ctxutil/contextManager.js](../../internal/builtin/ctxutil/contextManager.js).

- Custom goal commands (type `custom`) can be injected as JS handlers. The handler function receives `args` (array), and the runtime exposes helper variables/objects:
    - `output` with `print/printf` for TUI output
    - `tui` for tui API (e.g. shell-style prompts - see [tui-api.md](tui-api.md))
    - `ctxmgr` context manager instance (if used)
    - `state` and `stateKeys` for script-local state
    - `buildPrompt` function for the latest generated prompt

Note: custom handlers are created from user-supplied source strings (via `new Function`); this allows a flexible workflow but any code in the handler runs in JS runtime context — authoring code from untrusted sources is a security consideration.

---

**Templating best-practices**

- Use `promptInstructions` for the human-readable instructions and `promptTemplate` for a consistent final prompt layout.
- Use `stateVars` and named `stateKeys` to enable dynamic `PromptOptions` mapping (e.g. `type`/`typeInstructions` patterns used by `test-generator`).
- Keep `promptTemplate` consistent with `{{.contextTxtar}}` for context and `{{.promptInstructions}}` substitution; built-in templates use `{{.description | upper}}` helper.

---

**Authoring goals (practical guidance)**

- Create a JSON file with at least `name` and `description` and put it in one of the discovered goal paths (e.g., `~/.one-shot-man/goals`, `./osm-goals/`, or a configured custom path via `goal.paths`).
- Start with a minimal skeleton and incrementally add `commands` and `stateVars`.
- Use `contextManager` commands for file and diff management unless you need custom behavior.
- Keep `Script` empty unless you need a completely custom JS handler/per-mode implementation; otherwise rely on the default interpreter.

Example (minimal):

```json
{
  "name": "my-minigoal",
  "description": "Small example goal",
  "category": "devops",
  "promptInstructions": "Summarize the provided context.",
  "promptTemplate": "{{.description}}\n\n{{.promptInstructions}}\n\n{{.contextTxtar}}",
  "commands": [
    {
      "name": "add",
      "type": "contextManager"
    },
    {
      "name": "show",
      "type": "contextManager"
    }
  ]
}
```

A complete example demonstrating all available features is available at:
[custom-goal-example.json](../../custom-goal-example.json)

Authoring tip: Start with a `custom` handler only when you need custom runtime logic not provided by `contextManager`.

---

**Validation & errors**

- Unknown goal: `Goal '%s' not found. Use 'osm goal -l' to list available goals.` (stderr) when the requested goal name is not present in the registry. See [internal/command/goal.go](../../internal/command/goal.go#L139-L149).

- `LoadGoalFromFile` will error on invalid JSON, missing `name` or `description`, or invalid `name` format. See tests in [internal/command/goal_loader_test.go](../../internal/command/goal_loader_test.go).

- Loader uses the default `goalScript` as the JS interpreter when `script` is unspecified in JSON.

---

**Integration with configuration**

- Config keys relevant to `goal` discovery are outlined in: [config.md](config.md#goal-discovery) and include:
    - `goal.autodiscovery` (bool, default true) — enable/disable upward traversal.
    - `goal.paths` (list) — additional explicit paths.
    - `goal.path-patterns` (list) — name patterns to search for when traversing up.
    - `goal.max-traversal-depth` (int, default 10) — maximum upward traversal steps during autodiscovery.
    - `goal.disable-standard-paths` (bool) — skip standard search locations.

- Session and store flags interact with TUI state persistence.
    - `--store` chooses the storage backend (`fs` or `memory`) — see [internal/storage/registry.go](../../internal/storage/registry.go).
    - `--session` or env `OSM_SESSION` override session id selection used by StateManager; session ID is resolved using a sophisticated hierarchy described in [internal/scripting/session_id_common.go](../../internal/scripting/session_id_common.go).

---

**Limitations & security**

- Handlers for custom commands are executed as JavaScript functions created via `new Function(...)`. This enables authoring but means any code present in custom handlers will be executed in the interpreter. Only use trusted goal files or host goals in controlled directories.

- The `Goal` `Script` field can embed an entire custom interpreter for advanced cases, but file-based script references are not part of the stable loader behavior. If you need more advanced behavior, consider contributing or extending the loader.

- Goal `Name` must conform to `^[a-zA-Z0-9][a-zA-Z0-9-]*$` and explicitly forbids spaces and underscores; your JSON must include valid `name` and `description` fields to be accepted.

---

**Examples & commands**

- List goals:

    - `osm goal -l` (list all)
    - `osm goal -c testing` (list `testing` category)

- Run `doc-generator` (interactive):

    - `osm goal doc-generator`

- Run `doc-generator` non-interactive (runs the JS script, registers modes, but TUI not started):

    - `osm goal -r doc-generator`

- Use `--store memory --session test-id` during tests or ephemeral runs to avoid writing to disk.

---

**Implementation references**

- `Goal` + CLI behavior: [internal/command/goal.go](../../internal/command/goal.go)
- Loading & validation: [internal/command/goal_loader.go](../../internal/command/goal_loader.go)
- Generic JS interpreter: [internal/command/goal.js](../../internal/command/goal.js)
- Discovery & sorting: [internal/command/goal_discovery.go](../../internal/command/goal_discovery.go)
- Dynamic registry & override semantics: [internal/command/goal_registry.go](../../internal/command/goal_registry.go)
- Built-in goals: [internal/command/goal_builtin.go](../../internal/command/goal_builtin.go)
- DB/state persistence & TUI engine: [internal/scripting/engine_core.go](../../internal/scripting/engine_core.go), [internal/scripting/tui_manager.go](../../internal/scripting/tui_manager.go)
- Context manager commands: [internal/builtin/ctxutil/contextManager.js](../../internal/builtin/ctxutil/contextManager.js)
