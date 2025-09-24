# one-shot-man

Command `osm` helps produce higher quality implementations with less effort, keeping track of your context, supporting extensible, REPL-based, stateful workflows.

## What & Why

The goal of `osm` is to streamline the use of complex one-shot prompts via a stateful TUI (Terminal User Interface). It automates the tedious aspects of building prompt context and scaffolding, which is often required for high-quality, repeatable results.

While agentic workflows have their place, a precisely crafted, context-rich one-shot prompt can yield superior results for specific tasks, such as:

* **Pre-PR and incremental code review** - Detailed sanity checks, alternate perspectives, and (indirect) quantification of quality.
* **Iterating on complex but self-contained implementations** - Precise or strictly constrained adjustments, applied consistently.

This tool is provider-agnostic, integrating primarily via clipboard, file, and command I/O.

## Features

- **Extensible Command System**: Support for both built-in and script-based subcommands.
- **Kubectl-style Configuration**: Configuration file management with environment variable overrides.
- **dnsmasq-style Config Format**: Simple `optionName remainingLineIsTheValue` format.
- **Script Command Discovery**: Automatic discovery and execution of script commands.
- **Stdlib Flag Package**: Built using Go's standard library `flag` package.
- **Interactive TUI (go-prompt)**: Rich REPL powered by `github.com/elk-language/go-prompt` with completion, custom key bindings, and configurable colors.

-----

## Installation

Currently, only source installation is supported. Pre-built binaries will be added if this tool reaches alpha.

```sh
go install github.com/joeycumines/one-shot-man/cmd/osm@latest
```

-----

## Usage

-----

### Basic Commands

```sh
# Show help
osm help
# Show version
osm version
# Initialize configuration
osm init
# Manage configuration
osm config --all
# Start interactive scripting terminal (TUI)
osm script -i
```

-----

### Code Review Command

The `code-review` command provides an interactive TUI to build a comprehensive, single-shot prompt for an LLM to perform a code review. It allows you to progressively add context from files, git diffs, and freeform notes.

```sh
# Start the interactive code review builder
osm code-review
```

The code review workflow:

1.  **Build Context**: Iteratively add relevant files (`add`), git diffs (`diff`), and notes (`note`) to construct the scope of the review.
2.  **Review & Refine**: Use `list`, `edit`, and `remove` to manage the context items.
3.  **Generate & Use**: Once the context is complete, use `show` to view the final generated prompt or `copy` to send it to your clipboard for use with an LLM.

Commands available in code review mode:

- `add [files...]` - Add file content to the context. Without arguments, it opens an editor to list file paths.
- `diff [args...]` - Add git diff output to the context. Arguments are passed directly to `git diff` (e.g., `HEAD~1`, `--staged`).
- `note [text]` - Add a freeform note. Without text, it opens an editor.
- `list` - List all current context items (files, diffs, notes) with their assigned IDs.
- `edit <id>` - Edit a note or git diff specification by its ID using the default editor.
- `remove <id>` - Remove a context item by its ID.
- `show` - Assemble all context and display the final code review prompt.
- `copy` - Copy the final prompt to the system clipboard.
- `help` - Show the list of available commands.
- `exit` - Exit the code review session.

-----

### Prompt Flow Command

The `prompt-flow` command provides an interactive prompt builder that follows a goal/context/template workflow to generate and assemble prompts:

```sh
# Start the interactive prompt flow builder
osm prompt-flow
```

The prompt flow workflow:

1.  **Set Goal**: Define what you want to achieve (e.g., `goal Refactor to unexport all methods except Run`).
2.  **Build Context**: Add relevant files (`add`), git diffs (`diff`), and notes (`note`).
3.  **Generate Meta-Prompt**: Run `generate` to create a prompt designed to be sent to an LLM. You can inspect it with `show meta`.
4.  **Set Task Prompt**: After getting a response from an LLM, use the `use` command to set it as the task prompt (e.g., `use "The user wants a Javascript function..."`). You can also `edit prompt` later to refine it.
5.  **Assemble & Use**: The task prompt is now combined with the context. Use `show` to see the final output or `copy` to send it to your clipboard. If you clear the task prompt via `edit prompt` and save empty, the flow reverts to the meta-prompt phase so `show` defaults back to `show meta`.

Commands available in prompt flow mode:

- `goal [text]` - Set or edit the goal (no args opens your editor).
- `add [files...]` - Add file content to context (no args opens editor for paths, one per line).
- `diff [args]` - Add git diff output to context (e.g., `--staged`, `HEAD~1`).
- `note [text]` - Add a freeform note (no args opens editor).
- `list` - List current goal, template, prompts, and context items.
- `edit <id|goal|template|meta|prompt>` - Edit items by ID or name. `meta` edits the generated meta-prompt; `prompt` edits the task prompt (clearing content will revert back to meta-prompt phase).
- `remove <id>` - Remove a context item (file items also untrack from the backing context).
- `template` - Edit the meta-prompt template.
- `generate` - Generate the meta-prompt and reset the task prompt.
- `use [text]` - Set or edit the task prompt (no args opens editor).
- `show [meta|prompt]` - Show content. Default shows final output if the task prompt is set, otherwise shows the meta-prompt.
- `copy [meta|prompt]` - Copy content to clipboard. Default behavior mirrors `show`.
- `help` - Show available commands.
- `exit` - Exit prompt flow mode.

-----

### Configuration

The configuration file uses a dnsmasq-style format where each line contains an option name followed by its value:

```
# Global options
verbose true
color auto

# Command-specific options
[help]
pager less
format detailed

[version]
format short
```

#### Configuration Location

- Default: `~/.one-shot-man/config`
- Override with `ONESHOTMAN_CONFIG` environment variable

```sh
# Use custom config location
export ONESHOTMAN_CONFIG=/path/to/custom/config
osm init
```

#### Prompt Colors (Configurable)

The interactive TUI uses a readable, high-contrast default theme (input text is green by default). You can override colors via config with `prompt.color.*` keys. Valid values: `black,darkred,darkgreen,brown,darkblue,purple,cyan,lightgray,darkgray,red,green,yellow,blue,fuchsia,turquoise,white`.

Supported keys (12):

- `prompt.color.input`
- `prompt.color.prefix`
- `prompt.color.suggestionText`
- `prompt.color.suggestionBG`
- `prompt.color.selectedSuggestionText`
- `prompt.color.selectedSuggestionBG`
- `prompt.color.descriptionText`
- `prompt.color.descriptionBG`
- `prompt.color.selectedDescriptionText`
- `prompt.color.selectedDescriptionBG`
- `prompt.color.scrollbarThumb`
- `prompt.color.scrollbarBG`

Examples (global scope):

```
prompt.color.input green
prompt.color.prefix cyan
prompt.color.suggestionText yellow
prompt.color.suggestionBG black
prompt.color.selectedSuggestionText black
prompt.color.selectedSuggestionBG cyan
prompt.color.descriptionText white
prompt.color.descriptionBG black
prompt.color.selectedDescriptionText white
prompt.color.selectedDescriptionBG blue
prompt.color.scrollbarThumb darkgray
prompt.color.scrollbarBG black
```

Notes:

- Defaults: input=green, prefix=cyan, suggestionText=yellow, suggestionBG=black, selectedSuggestionText=black, selectedSuggestionBG=cyan, descriptionText=white, descriptionBG=black, selectedDescriptionText=white, selectedDescriptionBG=blue, scrollbarThumb=darkgray, scrollbarBG=black.
- These apply to `osm script -i` and as defaults for prompts created from JavaScript (which can further override per prompt).

#### Built-in Script Configuration

The built-in scripts (`prompt-flow` and `code-review`) support extensive configuration options to customize their behavior and appearance.

##### Template Customization

You can override the default templates used by built-in scripts:

```
[prompt-flow]
# Override with file content (file takes precedence)
template.file /path/to/custom/prompt-flow-template.md

# Override with inline content
template.content Custom template with {{goal}} and {{context_txtar}}

[code-review]  
template.file /path/to/custom/code-review-template.md
template.content Review template: {{context_txtar}}
```

##### Script UI Configuration

Customize the user interface elements of built-in scripts:

```
[prompt-flow]
script.ui.title Custom Prompt Builder
script.ui.banner Custom Prompt Flow: Enhanced workflow
script.ui.prompt (custom-flow) > 
script.ui.help-text Custom commands: goal, context, generate, export
script.ui.history-file .custom-prompt-flow-history
script.ui.enable-history true
script.ui.show-help-on-start true

[code-review]
script.ui.title Advanced Code Review  
script.ui.banner Code Review: Enhanced context analysis
script.ui.prompt (review) > 
script.ui.help-text Enhanced commands: add, analyze, generate, export
script.ui.history-file .custom-code-review-history
script.ui.enable-history true
```

##### Available Configuration Options

**Template Options:**
- `template.file` - Path to custom template file (takes precedence)  
- `template.content` - Inline template content

**UI Options:**
- `script.ui.title` - Custom title for the mode
- `script.ui.banner` - Custom banner text displayed on mode entry
- `script.ui.prompt` - Custom command prompt string
- `script.ui.help-text` - Custom help text for commands
- `script.ui.history-file` - Custom history file name 
- `script.ui.enable-history` - Enable/disable command history (true/false)
- `script.ui.show-help-on-start` - Show help on mode entry (true/false)

**Precedence:** Command-specific options override global options. File-based templates override inline content.

-----

### Script Commands

Script commands are discovered from these locations (in order):

1.  `scripts/` directory relative to the executable
2.  `~/.one-shot-man/scripts/` (user scripts)
3.  `./scripts/` (current directory scripts)

#### JavaScript Scripting

Create JavaScript scripts with the deferred/declarative API:

```javascript
// scripts/example.js

const {getenv} = require('osm:os');

ctx.log("Starting example script");

// Demonstrate deferred execution (cleanup)
ctx.defer(function() {
    ctx.log("Cleaning up resources");
});

// Demonstrate sub-tests similar to testing.T.run()
ctx.run("setup", function() {
    ctx.log("Setting up test environment");
    ctx.logf("Environment: %s", getenv("PATH") ? "defined" : "undefined");

    ctx.defer(function() {
        ctx.log("Cleaning up test environment");
    });
});

output.print("Script execution completed successfully");

ctx.log("Example script finished");
```

Execute the script, with `--test` for debug logging, without TUI (no `-i`):

```sh
osm script --test scripts/example.js
```

Will output:

```
[example.js] Starting example script
[example.js/setup] Setting up test environment
[example.js/setup] Environment: defined
[example.js/setup] Cleaning up test environment
[example.js] Sub-test setup passed
Script execution completed successfully
[example.js] Example script finished
[example.js] Cleaning up resources
```

To start an interactive session in the REPL-like TUI, use `-i`:

```sh
$ osm script -i
================================================================
WARNING: EPHEMERAL SESSION - nothing is persisted. Your work will be lost on exit.
Save or export anything you need BEFORE quitting.
================================================================
one-shot-man Rich TUI Terminal
Type 'help' for available commands, 'exit' to quit
Available modes:
Starting advanced go-prompt interface
>>> help
Available commands:
  help                 - Show this help message
  exit, quit           - Exit the terminal
  mode <name>          - Switch to a mode
  modes                - List available modes
  state                - Show current mode state

Registered commands:
  mode                 - Switch to a different mode
    Usage: mode <mode-name>
  modes                - List all available modes
  state                - Show current mode state

Available modes:
Switch to a mode to execute JavaScript code
```

-----

## Architecture

### Built-in Commands

- `help` - Display help information.
- `version` - Show version information.
- `config` - Manage configuration settings.
- `init` - Initialize the one-shot-man environment.
- `script` - Execute JavaScript scripts with deferred/declarative API.
- `prompt-flow` - Interactive prompt builder: goal/context/template -\> generate -\> assemble.
- `code-review` - Single-prompt code review with context: context -\> generate prompt for PR review.

### Interactive TUI

The TUI is built on go-prompt and provides:

- Prompt with configurable prefix per mode (default ` >>>  ` or ` [mode]>  `).
- Command execution (`help`, `exit|quit`, `mode`, `modes`, `state`).
- Completion for built-in and registered commands (first token).
- Custom JavaScript completers and key bindings.
- Color customization via config or per-prompt options.
- History loading from a file if present.

History:

- The default interactive prompt will load history from `.osm_history` if it exists.
- Advanced prompts created from JavaScript can also specify a history file. (Note: history is loaded if present; automatic saving on exit is not currently implemented.)

### Command Structure

Commands implement the `Command` interface:

```go
type Command interface {
    Name() string
    Description() string
    Usage() string
    SetupFlags(fs *flag.FlagSet)
    Execute(args []string, stdout, stderr io.Writer) error
}
```

### Extension Points

1.  **Built-in Commands**: Implement the `Command` interface and register with the registry.
2.  **Script Commands**: Create executable scripts in designated script directories.
3.  **Configuration**: Use the dnsmasq-style config format for both global and command-specific options.

-----

## JavaScript TUI API

In addition to the deferred testing-style API on `ctx`, scripts can control the TUI via the global `tui` object and interact with the host system via the `system` object.

### `tui` Object

Available functions (implemented):

- `tui.registerMode(modeConfig)` — Register a mode with optional TUI config:
    - `name` (string)
    - `tui.prompt` (string) to customize the prefix like ` [my-mode]>  `
    - `onEnter`, `onExit`, `onPrompt` (callbacks)
- `tui.switchMode(name)` — Switch active mode
- `tui.getCurrentMode()` — Get current mode name
- `tui.setState(key, value)` / `tui.getState(key)` — Per-mode state storage
- `tui.registerCommand({ name, description, usage, handler })` — Register global JS commands
- `tui.listModes()` — List mode names
- `tui.createAdvancedPrompt(config)` — Create a named go-prompt instance
    - `config.title` (string)
    - `config.prefix` (string)
    - `config.colors` (object; keys same as prompt.color.\* without the prefix)
    - `config.history` (object) e.g. `{ enabled: true, file: ".script_history", size: 1000 }` (loads if present)
- `tui.runPrompt(name)` — Run a created prompt (blocks until exit)
- `tui.registerCompleter(name, fn)` — Register a JS completer
- `tui.setCompleter(promptName, completerName)` — Attach completer to a prompt
- `tui.registerKeyBinding(key, fn)` — Register a JS key handler (e.g., `"ctrl-r"`)

Completer document helpers available in JS: `getText()`, `getTextBeforeCursor()`, `getWordBeforeCursor()`.

### `system` Object

Available functions for system interaction:

- `system.exec(command, ...args)` — Executes a system command. Returns an object: `{ stdout: string, stderr: string, code: int, error: bool, message: string }`.
- `system.execv(argv)` — Executes a command from an array of strings (e.g., `['git', 'diff']`).
- `system.readFile(path)` — Reads a file from disk. Returns an object: `{ content: string, error: bool, message: string }`.
- `system.openEditor(title, initialContent)` — Opens the user's default editor (`$VISUAL` / `$EDITOR` / fallback) with initial content and returns the final edited content as a string.
- `system.clipboardCopy(text)` — Copies the given text to the system clipboard.

Example: Create a prompt, add a completer, then run it

```javascript
// scripts/demo-prompt.js

// Register a simple completer
tui.registerCompleter('files', (doc) => {
    const word = doc.getWordBeforeCursor();
    // Suggest some static items for demo
    return [
        { text: 'help', description: 'Built-in command' },
        { text: 'modes', description: 'List modes' },
    ].filter(s => s.text.startsWith(word));
});

// Create prompt with custom prefix and colors
const promptName = tui.createAdvancedPrompt({
    title: 'Demo Prompt',
    prefix: 'demo> ',
    colors: {
        input: 'green',
        prefix: 'red'
    },
    history: { enabled: true, file: '.demo_history' }
});

tui.setCompleter(promptName, 'files');

// Start the prompt
tui.runPrompt(promptName);
```

Run:

```sh
osm script scripts/demo-mode.js
```

-----

## Development

Run all code quality checks:

```sh
make
```

### Configuration

#### Config Files

```sh
# Initialize with custom location
ONESHOTMAN_CONFIG=/tmp/myconfig osm init

# Force re-initialize
osm init --force
```

#### Environment Variables

| Variable                | Description                                                                                                             |
|:------------------------|:------------------------------------------------------------------------------------------------------------------------|
| `VISUAL`                | The preferred command to launch a text editor. Takes precedence over `EDITOR`.                                          |
| `EDITOR`                | A fallback command to launch a text editor if `VISUAL` is not set. Defaults to `nano`, `vi`, or `ed`.                   |
| `ONESHOT_CLIPBOARD_CMD` | A user-defined shell command to use for copying text to the clipboard, overriding the built-in platform-specific logic. |

-----

## License

See [LICENSE](LICENSE) file for details.
