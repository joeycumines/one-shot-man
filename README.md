# one-shot-man

Command one-shot-man lets you produce high quality implementations with drastically less effort, keeping track of your context, using a simple, extensible, REPL-based, wizard-style workflow.

## Features

- **Extensible Command System**: Support for both built-in and script-based subcommands
- **Kubectl-style Configuration**: Configuration file management with environment variable override
- **dnsmasq-style Config Format**: Simple `optionName remainingLineIsTheValue` format
- **Script Command Discovery**: Automatic discovery and execution of script commands
- **Stdlib Flag Package**: Built using Go's standard library flag package
- **Interactive TUI (go-prompt)**: Rich REPL powered by `github.com/elk-language/go-prompt` with completion, custom key bindings, and configurable colors

---
## Installation

```bash
go install github.com/joeycumines/one-shot-man/cmd/one-shot-man@latest
```

Or build from source:

```bash
git clone https://github.com/joeycumines/one-shot-man.git
cd one-shot-man
go build -o one-shot-man ./cmd/one-shot-man
```

---
## Usage

### Basic Commands

```bash
# Show help
one-shot-man help

# Show version
one-shot-man version

# Initialize configuration
one-shot-man init

# Manage configuration
one-shot-man config --all

# Start interactive scripting terminal (TUI)
one-shot-man script -i
```

### Prompt Flow Command

The `prompt-flow` command provides an interactive prompt builder that follows a goal/context/template workflow to generate and assemble prompts:

```bash
# Start the interactive prompt flow builder
one-shot-man prompt-flow

# Start in test mode (non-interactive, for testing)
one-shot-man prompt-flow --test
```

The prompt flow workflow:

1. **Set Goal**: Define what you want to achieve
2. **Build Context**: Add files, git diffs, and notes
3. **Customize Template**: Edit the meta-prompt template (pre-loaded with a comprehensive template)
4. **Generate**: Create the main prompt using the meta-prompt
5. **Assemble**: Combine prompt with context for final output

Commands available in prompt flow mode:
- `goal [text]` - Set or edit the goal
- `add [files...]` - Add file content to context
- `diff [args]` - Add git diff output to context  
- `note [text]` - Add a freeform note
- `list` - List current goal, template, prompt, and context items
- `edit <id|goal|template|prompt>` - Edit items by ID or name
- `remove <id>` - Remove a context item
- `template` - Edit the meta-prompt template
- `generate` - Generate the main prompt using the meta-prompt
- `show [meta]` - Show meta-prompt or final output
- `copy [meta]` - Copy meta-prompt or final output to clipboard
- `help` - Show available commands
- `exit` - Exit prompt flow mode

---
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

```bash
# Use custom config location
export ONESHOTMAN_CONFIG=/path/to/custom/config
one-shot-man init
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
- These apply to `one-shot-man script -i` and as defaults for prompts created from JavaScript (which can further override per prompt).

---
### Script Commands

Script commands are discovered from these locations (in order):
1. `scripts/` directory relative to the executable
2. `~/.one-shot-man/scripts/` (user scripts)
3. `./scripts/` (current directory scripts)

#### JavaScript Scripting

Create JavaScript scripts with the deferred/declarative API:

```javascript
// File: scripts/example.js

ctx.log("Starting example script");

// Demonstrate deferred execution (cleanup)
ctx.defer(function() {
    ctx.log("Cleaning up resources");
});

// Demonstrate sub-tests similar to testing.T.run()
ctx.run("setup", function() {
    ctx.log("Setting up test environment");
    ctx.logf("Environment: %s", env("PATH") ? "defined" : "undefined");
    
    ctx.defer(function() {
        ctx.log("Cleaning up test environment");
    });
});

console.log("Script execution completed successfully");
```

Execute the script:

```bash
one-shot-man script scripts/example.js
```

Start an interactive session instead:

```bash
one-shot-man script -i
```

Inside the TUI, try:

- `help` to list commands
- `modes` to list registered modes (if your scripts define any)
- `mode <name>` to switch modes
- `state` to print current mode state

---
## Architecture

### Built-in Commands

- `help` - Display help information
- `version` - Show version information
- `config` - Manage configuration settings
- `init` - Initialize the one-shot-man environment
- `script` - Execute JavaScript scripts with deferred/declarative API
- `prompt-flow` - Interactive prompt builder: goal/context/template -> generate -> assemble

### Interactive TUI (Implemented)

The TUI is built on go-prompt and provides:

- Prompt with configurable prefix per mode (default `>>> ` or `[mode]> `)
- Command execution (`help`, `exit|quit`, `mode`, `modes`, `state`)
- Completion for built-in and registered commands (first token)
- Custom JavaScript completers and key bindings
- Color customization via config or per-prompt options
- History loading from a file if present

History:
- The default interactive prompt will load history from `.one-shot-man_history` if it exists.
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

1. **Built-in Commands**: Implement the `Command` interface and register with the registry
2. **Script Commands**: Create executable scripts in designated script directories
3. **Configuration**: Use the dnsmasq-style config format for both global and command-specific options

---
## JavaScript TUI API

In addition to the deferred testing-style API on `ctx`, scripts can control the TUI via the global `tui` object and interact with the host system via the `system` object.

### `tui` Object

Available functions (implemented):

- `tui.registerMode(modeConfig)` — Register a mode with optional TUI config:
    - `name` (string)
    - `tui.prompt` (string) to customize the prefix like `[my-mode]> `
    - `onEnter`, `onExit`, `onPrompt` (callbacks)
- `tui.switchMode(name)` — Switch active mode
- `tui.getCurrentMode()` — Get current mode name
- `tui.setState(key, value)` / `tui.getState(key)` — Per-mode state storage
- `tui.registerCommand({ name, description, usage, handler })` — Register global JS commands
- `tui.listModes()` — List mode names
- `tui.createAdvancedPrompt(config)` — Create a named go-prompt instance
    - `config.title` (string)
    - `config.prefix` (string)
    - `config.colors` (object; keys same as prompt.color.* without the prefix)
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
        prefix: 'cyan'
    },
    history: { enabled: true, file: '.demo_history' }
});

tui.setCompleter(promptName, 'files');

// Start the prompt
tui.runPrompt(promptName);
```

Run:

```bash
one-shot-man script scripts/demo-prompt.js
```

---
## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o one-shot-man ./cmd/one-shot-man
```

---
## Examples

### Basic Usage

```bash
# Get help
one-shot-man help

# Initialize configuration
one-shot-man init

# Start interactive terminal
one-shot-man script -i
```

### Prompt Flow (single-file template-driven)

A minimal prompt-flow implementation is available as `scripts/prompt-flow.js`. It lets you:

- Set a goal (required first)
- Add context via files, git diffs, and freeform notes
- Edit a meta-prompt template and generate a main prompt
- Preview and copy either the meta-prompt or the final assembled output (prompt + context)

Run interactively:

```
one-shot-man script -i scripts/prompt-flow.js
```

Key commands inside the mode:

- `goal [text]` — Set or edit the goal (no args opens your editor)
- `add [file ...]` — Track files for context; when omitted opens editor for paths (one per line)
- `list` — List goal/template/prompt and context items
- `edit <id|goal|template|prompt>` — Edit an item or top-level state; file items are edited on disk
- `remove <id>` — Remove a context item; file items also untrack from the backing context
- `diff [args]` — Capture `git diff` output with optional args (e.g., `--staged`, `HEAD~1`)
- `note [text]` — Add a note (no args opens editor)
- `template` — Edit the meta-prompt template; default lives in-memory and includes the txtar context dump
- `generate` — Build the meta-prompt and open it for editing as your main prompt, transitioning to GENERATED
- `show [meta]` — Show meta-prompt (or final output by default in GENERATED state)
- `copy [meta]` — Copy meta or final output to clipboard

Implementation details:

- Uses existing `context.toTxtar()` from the engine to dump tracked files in txtar format for template use
- Exposes `system.exec`, `system.execv(argv)`, `system.readFile(path)`, `system.openEditor(title, initial)`, and `system.clipboardCopy(text)` for script ergonomics
- Keeps `scripts/llm-prompt-builder.js` behavior unchanged

Clipboard/editor support:
- Clipboard: macOS (pbcopy), Linux/BSD (wl-copy, xclip, xsel), Windows (clip). Override with ONESHOT_CLIPBOARD_CMD (executed via sh/cmd with stdin = content).
- Editor default: $VISUAL | $EDITOR | OS-specific fallback (Windows: notepad; Unix: nano | vi | ed).

### Configuration Management

```bash
# Initialize with custom location
ONESHOTMAN_CONFIG=/tmp/myconfig one-shot-man init

# Force re-initialize
one-shot-man init --force
```

---
## License

See [LICENSE](LICENSE) file for details.
