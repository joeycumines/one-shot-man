# One-Shot-Man Internal API Documentation

This document provides comprehensive documentation for the internal APIs of one-shot-man, including the scripting system, command interfaces, and configuration.

## Table of Contents

1. [JavaScript Scripting Engine](#javascript-scripting-engine)
2. [TUI (Terminal User Interface) API](#tui-api)
3. [Context API](#context-api)
4. [Native Go Modules (osm: prefix)](#native-go-modules)
5. [Command Registration System](#command-registration-system)
6. [Configuration System](#configuration-system)
7. [Extension Points](#extension-points)

## JavaScript Scripting Engine

The JavaScript scripting engine is built on [Goja](https://github.com/dop251/goja) and provides CommonJS module support. It supports:

- **CommonJS modules** via global `require()` function
- **Native Go modules** with `osm:` prefix (e.g., `require('osm:os')`)
- **File-based modules** with absolute or relative paths
- **Deferred execution** similar to Go's `defer` or testing cleanup functions

### Engine Lifecycle

The engine operates in two phases:

1. **Configuration Phase**: All scripts are loaded and evaluated to register modes and commands
2. **Execution Phase**: Interactive TUI is started (if `-i` flag) with configured state

### Global Objects

The engine provides several global objects to scripts:

- `ctx` - Context API for logging and deferred execution
- `tui` - Terminal User Interface management
- `output` - Terminal output functions
- `console` - Logging functions (log, printf, getLogs, etc.)
- `args` - Command-line arguments array

## TUI API

The `tui` object provides rich terminal interface capabilities.

### Mode Management

#### `tui.registerMode(config)`

Registers a new TUI mode with the following configuration:

```javascript
tui.registerMode({
    name: "my-mode",           // Required: unique mode name
    tui: {
        title: "My Mode",      // Optional: window title
        prompt: "[my]> ",      // Optional: prompt prefix
        enableHistory: true,   // Optional: enable command history
        historyFile: ".my_history" // Optional: history file path
    },
    onEnter: function() {      // Optional: called when entering mode
        output.print("Welcome to my mode!");
        // Initialize mode state
        tui.setState("counter", 0);
    },
    onExit: function() {       // Optional: called when exiting mode
        output.print("Goodbye from my mode!");
    },
    onPrompt: function() {     // Optional: called before each prompt
        // Custom prompt behavior
    },
    commands: {                // Optional: mode-specific commands
        "hello": {
            description: "Say hello",
            usage: "hello [name]",
            handler: function(args) {
                var name = args.length > 0 ? args[0] : "World";
                output.print("Hello, " + name + "!");
            }
        }
    }
});
```

#### `tui.switchMode(name)`

Switch to the specified mode:

```javascript
tui.switchMode("my-mode");
```

#### `tui.getCurrentMode()`

Get the current mode name:

```javascript
var currentMode = tui.getCurrentMode();
```

#### `tui.listModes()`

Get array of all registered mode names:

```javascript
var modes = tui.listModes();
output.print("Available modes: " + modes.join(", "));
```

### State Management

#### `tui.setState(key, value)`

Set mode-specific state:

```javascript
tui.setState("counter", 42);
tui.setState("config", { theme: "dark", debug: true });
```

#### `tui.getState(key)`

Get mode-specific state:

```javascript
var counter = tui.getState("counter");
var config = tui.getState("config");
```

### Command Registration

#### `tui.registerCommand(config)`

Register global commands available in all modes:

```javascript
tui.registerCommand({
    name: "global-cmd",
    description: "A global command",
    usage: "global-cmd <arg>",
    argCompleters: ["file", "directory"], // Optional: completion types
    handler: function(args) {
        output.print("Global command executed: " + args.join(" "));
    }
});
```

### Advanced Prompt System

#### `tui.createAdvancedPrompt(config)`

Create a named prompt instance with custom configuration:

```javascript
var promptName = tui.createAdvancedPrompt({
    title: "Advanced Prompt",
    prefix: "advanced> ",
    colors: {
        input: "green",
        prefix: "cyan",
        suggestionText: "yellow",
        suggestionBG: "black",
        selectedSuggestionText: "black",
        selectedSuggestionBG: "cyan",
        descriptionText: "white",
        descriptionBG: "black",
        selectedDescriptionText: "white",
        selectedDescriptionBG: "blue",
        scrollbarThumb: "darkgray",
        scrollbarBG: "black"
    },
    history: {
        enabled: true,
        file: ".my_history",
        size: 1000
    }
});
```

#### `tui.runPrompt(name)`

Run a created prompt (blocks until exit):

```javascript
tui.runPrompt(promptName);
```

### Completion System

#### `tui.registerCompleter(name, fn)`

Register a JavaScript completion function:

```javascript
tui.registerCompleter('fileCompleter', function(document) {
    var word = document.getWordBeforeCursor();
    var text = document.getText();
    
    // Return array of completion suggestions
    return [
        { text: 'file1.txt', description: 'Text file' },
        { text: 'file2.js', description: 'JavaScript file' }
    ].filter(function(item) {
        return item.text.startsWith(word);
    });
});
```

Completer document helpers:
- `document.getText()` - Get full input text
- `document.getTextBeforeCursor()` - Get text before cursor
- `document.getWordBeforeCursor()` - Get word before cursor

#### `tui.setCompleter(promptName, completerName)`

Attach a completer to a prompt:

```javascript
tui.setCompleter(promptName, 'fileCompleter');
```

### Key Bindings

#### `tui.registerKeyBinding(key, handler)`

Register custom key binding handlers:

```javascript
tui.registerKeyBinding("ctrl-r", function() {
    output.print("Ctrl+R pressed!");
});
```

## Context API

The `ctx` object provides logging and deferred execution capabilities.

### Logging Functions

#### `ctx.log(message)`

Log a message with script context:

```javascript
ctx.log("This is a log message");
// Output: [script-name] This is a log message
```

#### `ctx.logf(format, ...args)`

Log a formatted message:

```javascript
ctx.logf("Counter value: %d", counter);
```

### Deferred Execution

#### `ctx.defer(fn)`

Register a cleanup function to run when the script context exits:

```javascript
ctx.defer(function() {
    ctx.log("Cleaning up resources");
});
```

### Sub-contexts

#### `ctx.run(name, fn)`

Create a sub-context (similar to Go's testing.T.Run):

```javascript
ctx.run("setup", function() {
    ctx.log("Setting up test environment");
    
    ctx.defer(function() {
        ctx.log("Cleaning up test environment");
    });
    
    // Setup logic here
});
```

## Output API

The `output` object provides terminal output functions separate from logging.

### Output Functions

#### `output.print(message)`

Print message to terminal output:

```javascript
output.print("Hello, world!");
```

#### `output.printf(format, ...args)`

Print formatted message to terminal:

```javascript
output.printf("Value: %d", 42);
```

### Console API

The `log` object provides logging functions similar to browser console.

### Console Functions

#### `log.info(message)`, `log.debug(message)`, `log.warn(message)`, `log.error(message)`

Log messages with different severity levels:

```javascript
log.info("Information message");
log.debug("Debug message");
log.warn("Warning message");
log.error("Error message");
```

#### `log.printf(format, ...args)`

Log formatted message:

```javascript
log.printf("Debug: %s = %d", "counter", value);
```

#### Management Functions

- `log.getLogs()` - Get array of log messages
- `log.clearLogs()` - Clear the log buffer
- `log.searchLogs(query)` - Search logs for query string

## Native Go Modules

Native Go modules are available via CommonJS require with the `osm:` prefix.

### osm:argv

Command line argument parsing utilities.

```javascript
const argv = require('osm:argv');

// Parse command line string into argv array
var args = argv.parseArgv('git commit -m "Initial commit"');
// Returns: ["git", "commit", "-m", "Initial commit"]

// Format argv array for display (with quoting)
var formatted = argv.formatArgv(["git", "commit", "-m", "Initial commit"]);
// Returns: 'git commit -m "Initial commit"'
```

### osm:os

Operating system interaction functions.

```javascript
const os = require('osm:os');

// Read file from disk
var result = os.readFile('/path/to/file.txt');
if (!result.error) {
    output.print("File content: " + result.content);
} else {
    output.print("Error: " + result.message);
}

// Check if file exists
if (os.fileExists('/path/to/file.txt')) {
    output.print("File exists");
}

// Open default editor
var content = os.openEditor("My Note", "Initial content");
output.print("Edited content: " + content);

// Copy to clipboard
os.clipboardCopy("Text to copy");

// Get environment variable
var path = os.getenv("PATH");
```

### osm:exec

Execute system commands with context cancellation support.

```javascript
const exec = require('osm:exec');

// Execute command with arguments
var result = exec.exec('git', 'status', '--porcelain');
if (!result.error) {
    output.print("Git status:");
    output.print(result.stdout);
} else {
    output.print("Error: " + result.message);
    output.print("Exit code: " + result.code);
}

// Execute command from array
var result2 = exec.execv(['ls', '-la', '/tmp']);
if (!result2.error) {
    output.print("Directory listing:");
    output.print(result2.stdout);
}
```

Return format:
```javascript
{
    stdout: string,    // Command stdout
    stderr: string,    // Command stderr  
    code: number,      // Exit code
    error: boolean,    // True if command failed
    message: string    // Error message if any
}
```

### osm:time

Time-related utilities.

```javascript
const time = require('osm:time');

// Sleep for specified milliseconds
time.sleep(1000); // Sleep for 1 second
```

### osm:ctxutil

Context building utilities for complex prompts.

```javascript
const ctxutil = require('osm:ctxutil');

var items = [
    { type: 'note', label: 'Task', payload: 'Review the authentication code' },
    { type: 'diff', label: 'git diff HEAD~1', payload: 'diff content here...' },
    { type: 'lazy-diff', payload: ['--staged'] }, // Will run git diff --staged
    { type: 'lazy-diff', payload: 'HEAD~1' }     // Will run git diff HEAD~1
];

var context = ctxutil.buildContext(items, {
    toTxtar: function() {
        return "Additional context as txtar format";
    }
});

output.print(context);
```

Supported item types:
- `note`: Markdown note section
- `diff`: Markdown diff section with syntax highlighting
- `diff-error`: Markdown error section for failed diffs
- `lazy-diff`: Executes `git diff` with provided arguments at build time

### osm:nextIntegerId

Generate unique integer IDs based on existing items.

```javascript
const nextId = require('osm:nextIntegerId');

// Generate ID for new item in a list
var items = [
    { id: 1, name: "item1" },
    { id: 3, name: "item2" }
];
var newId = nextId(items);  // Returns 4 (max id + 1)

// For empty list or no argument
var firstId = nextId([]);   // Returns 1
var initialId = nextId();   // Returns 1
```

## Command Registration System

### Built-in Command Interface

Built-in commands implement the `Command` interface:

```go
type Command interface {
    Name() string
    Description() string  
    Usage() string
    SetupFlags(fs *flag.FlagSet)
    Execute(args []string, stdout, stderr io.Writer) error
}
```

### Command Discovery

Commands are discovered from multiple sources:

1. **Built-in Commands**: Registered via `registry.Register(cmd)`
2. **Embedded Scripts**: Core functionality with JavaScript embedded in Go binary
   - `code-review` command uses embedded script and template
   - `prompt-flow` command uses embedded script and template
3. **External Script Commands**: Executable files in script paths:
   - `scripts/` directory relative to executable
   - `~/.one-shot-man/scripts/` (user scripts)
   - `./scripts/` (current directory scripts)
4. **Example Scripts**: JavaScript files in `scripts/` directory for learning and testing

### Embedded vs. Example Scripts

**Embedded Scripts** (in Go binary):
- Core functionality that should always be available
- Located in `internal/command/` with `//go:embed` directives
- Examples: `code_review_script.js`, `prompt_flow_script.js`

**Example Scripts** (in `scripts/` directory):
- Learning materials and demonstrations
- Not embedded - loaded from filesystem
- Examples: `demo-mode.js`, `llm-prompt-builder.js`, `debug-tui.js`
- Can be customized or extended by users

### Script Command Example

Create an executable script in a script directory:

```bash
#!/bin/bash
# scripts/hello
echo "Hello from script command: $*"
```

Make it executable:
```bash
chmod +x scripts/hello
```

Use it:
```bash
osm hello world
# Output: Hello from script command: world
```

## Configuration System

### Configuration Format

Uses dnsmasq-style format: `optionName remainingLineIsTheValue`

```
# Global options
verbose true
color auto

# Command-specific sections
[script]
defaultMode interactive

[prompt-flow] 
template /path/to/template.md
```

### Configuration Locations

- Default: `~/.one-shot-man/config`
- Override with `ONESHOTMAN_CONFIG` environment variable

### Config API

```go
type Config struct {
    Global   map[string]string
    Commands map[string]map[string]string
}

// Access global option
value, exists := config.GetGlobalOption("verbose")

// Access command-specific option (with global fallback)
value, exists := config.GetCommandOption("script", "defaultMode")
```

### Prompt Color Configuration

Configure TUI colors via config:

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

Available colors: `black`, `darkred`, `darkgreen`, `brown`, `darkblue`, `purple`, `cyan`, `lightgray`, `darkgray`, `red`, `green`, `yellow`, `blue`, `fuchsia`, `turquoise`, `white`.

## Extension Points

### 1. Built-in Commands

Add new commands by implementing the `Command` interface:

```go
type MyCommand struct {
    *BaseCommand
}

func (c *MyCommand) Execute(args []string, stdout, stderr io.Writer) error {
    fmt.Fprintln(stdout, "My custom command")
    return nil
}

// Register with registry
registry.Register(NewMyCommand())
```

### 2. Embedded Commands

Some commands have embedded JavaScript that defines their behavior:
- **code-review** - Uses embedded `code_review_script.js` and `code_review_template.md`
- **prompt-flow** - Uses embedded `prompt_flow_script.js` and `prompt_flow_template.md`

These scripts are embedded using Go's `embed` package for core functionality.

### 3. Example Scripts  

The `scripts/` directory contains example JavaScript files demonstrating various features. These are not embedded but serve as:
- Learning materials for users
- Test cases for API validation
- Templates for creating custom scripts

### 4. Script Commands

Create executable scripts in script directories for simple command extensions.

### 5. JavaScript Modes

Create rich TUI modes via JavaScript:

```javascript
// scripts/my-tool.js
tui.registerMode({
    name: "my-tool",
    tui: {
        title: "My Tool",
        prompt: "[tool]> "
    },
    commands: {
        "process": {
            description: "Process data",
            handler: function(args) {
                // Tool logic here
            }
        }
    }
});
```

### 6. Native Go Modules

Extend the `osm:` module system by adding to `internal/scripting/builtin/`:

```go
// internal/scripting/builtin/mymodule/mymodule.go
func LoadModule(runtime *goja.Runtime, module *goja.Object) {
    exports := module.Get("exports").(*goja.Object)
    
    _ = exports.Set("myFunction", func(call goja.FunctionCall) goja.Value {
        // Module function implementation
        return runtime.ToValue("result")
    })
}

// Register in register.go
registry.RegisterNativeModule(prefix+"mymodule", mymodule.LoadModule)
```

### 7. Configuration Extensions

Add new configuration options by extending the config system to support your commands.

## Examples

### Complete Mode Example

```javascript
// scripts/demo-tool.js

// Load required modules
const os = require('osm:os');
const exec = require('osm:exec');

ctx.log("Initializing demo tool...");

// Register a comprehensive mode
tui.registerMode({
    name: "demo-tool",
    tui: {
        title: "Demo Tool",
        prompt: "[demo]> ",
        enableHistory: true,
        historyFile: ".demo_history"
    },
    
    onEnter: function() {
        output.print("Welcome to Demo Tool!");
        output.print("Type 'help' for available commands.");
        
        // Initialize state
        tui.setState("projects", []);
        tui.setState("currentProject", null);
    },
    
    onExit: function() {
        output.print("Thank you for using Demo Tool!");
    },
    
    commands: {
        "create": {
            description: "Create a new project",
            usage: "create <name> [description]",
            handler: function(args) {
                if (args.length === 0) {
                    output.print("Usage: create <name> [description]");
                    return;
                }
                
                var name = args[0];
                var description = args.slice(1).join(" ") || "No description";
                
                var projects = tui.getState("projects") || [];
                projects.push({ name: name, description: description });
                tui.setState("projects", projects);
                
                output.print("Created project: " + name);
            }
        },
        
        "list": {
            description: "List all projects",
            handler: function(args) {
                var projects = tui.getState("projects") || [];
                if (projects.length === 0) {
                    output.print("No projects found.");
                    return;
                }
                
                output.print("Projects:");
                for (var i = 0; i < projects.length; i++) {
                    var p = projects[i];
                    output.print("  " + (i + 1) + ". " + p.name + " - " + p.description);
                }
            }
        },
        
        "export": {
            description: "Export projects to file",
            handler: function(args) {
                var projects = tui.getState("projects") || [];
                var content = JSON.stringify(projects, null, 2);
                
                var filename = args.length > 0 ? args[0] : "projects.json";
                
                // Use editor to show content and let user save
                var edited = os.openEditor(filename, content);
                output.print("Export completed.");
            }
        }
    }
});

// Register global helper command
tui.registerCommand({
    name: "status",
    description: "Show current tool status",
    handler: function(args) {
        var mode = tui.getCurrentMode();
        output.print("Current mode: " + (mode || "none"));
        output.print("Available modes: " + tui.listModes().join(", "));
    }
});

ctx.log("Demo tool initialized successfully!");
```

### Using the Example

```bash
# Load the mode
osm script scripts/demo-tool.js

# Start interactive mode  
osm script -i scripts/demo-tool.js

# In the TUI:
>>> mode demo-tool
[demo]> create "My Project" "A sample project"
[demo]> create "Another Project"
[demo]> list
[demo]> export my-projects.json
[demo]> exit
>>> exit
```

This example demonstrates:
- Mode registration with full configuration
- State management for persistent data
- Multiple commands with argument handling
- Integration with native modules (osm:os)
- Proper error handling and user feedback
- Global command registration