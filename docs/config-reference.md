# Configuration Reference

This document provides a comprehensive reference for all configuration options available in one-shot-man.

## Configuration Format

one-shot-man uses a dnsmasq-style configuration format:

```
# Global options
optionName remainingLineIsTheValue

# Command-specific options
[command_name]
optionName remainingLineIsTheValue
```

## Configuration Location

- **Default**: `~/.one-shot-man/config`
- **Override**: Set `ONESHOTMAN_CONFIG` environment variable

```bash
# Use custom config location
export ONESHOTMAN_CONFIG=/path/to/custom/config
osm init
```

## Global Configuration Options

### Script Discovery Options

Script discovery controls how one-shot-man finds and loads script commands.

#### `script.autodiscovery` (boolean)
**Default**: `false`
**Description**: Enables advanced autodiscovery features including git repository detection and directory traversal.

```
script.autodiscovery true
```

When enabled, the following additional discovery mechanisms are activated:
- Git repository detection and traversal
- Directory traversal up from current working directory
- Prioritization of scripts from innermost git repository

#### `script.git-traversal` (boolean)
**Default**: `false`
**Description**: Enables traversing up directory tree to find git repositories and their script directories.

```
script.git-traversal true
```

**Dependencies**: Requires the `git` executable to be available on `PATH`.

**Security Note**: This option is disabled by default and requires `script.autodiscovery` to be enabled.

#### `script.max-traversal-depth` (integer)
**Default**: `10`
**Description**: Limits how many directories to traverse upward when looking for git repositories or script directories.

```
script.max-traversal-depth 5
```

**Range**: 1-100. Values outside this range fall back to the default.

#### `script.paths` (path list)
**Default**: (empty)
**Description**: Custom script paths to search, in addition to default locations. Supports comma (`,`) separation and the platform list separator (`:` on Unix, `;` on Windows).

```
script.paths ~/my-scripts:/opt/shared-scripts
script.paths ~/scripts,/usr/local/scripts
script.paths C:\scripts;$EXTRA_SCRIPTS   # Windows example
```

**Path Expansion**: Supports tilde (`~`) expansion and environment variable expansion (`$VAR`).

#### `script.path-patterns` (pattern list)
**Default**: `scripts`
**Description**: Directory names to search for when performing autodiscovery. Supports comma (`,`) separation and the platform list separator.

```
script.path-patterns scripts,bin,commands
script.path-patterns scripts:tools:utils
```

### Goal Discovery Options

Goal discovery controls how one-shot-man finds and loads custom goal definitions.

#### `goal.autodiscovery` (boolean)
**Default**: `true`
**Description**: Enables automatic discovery of custom goal files in standard locations.

```
goal.autodiscovery true
```

When enabled, searches for custom goal files (`*.json`) in:
- `~/.one-shot-man/goals/`
- `<executable-dir>/goals/`
- `./osm-goals/` (current directory)
- Parent directories matching `goal.path-patterns`

#### `goal.paths` (path list)
**Default**: (empty)
**Description**: Additional directories to search for custom goal files. Supports comma (`,`) separation and the platform list separator (`:` on Unix, `;` on Windows).

```
goal.paths ~/my-goals:/opt/shared-goals
goal.paths ~/goals,/usr/local/goals
goal.paths C:\goals;$EXTRA_GOALS   # Windows example
```

**Path Expansion**: Supports tilde (`~`) expansion and environment variable expansion (`$VAR`).

#### `goal.path-patterns` (pattern list)
**Default**: `osm-goals,goals`
**Description**: Directory names to search for when performing autodiscovery. Supports comma (`,`) separation and the platform list separator.

```
goal.path-patterns osm-goals,goals
goal.path-patterns my-goals:custom-goals
```

#### `goal.max-traversal-depth` (integer)
**Default**: `10`
**Description**: Limits how many directories to traverse upward when looking for goal directories.

```
goal.max-traversal-depth 5
```

**Range**: 1-100. Values outside this range fall back to the default.

### Prompt Configuration Options

#### `prompt.color.*` (color specification)
**Description**: Configure colors for the interactive TUI prompt. See [TUI Color Configuration](#tui-color-configuration) for details.

```
prompt.color.prefix #00FF00
prompt.color.text white
prompt.color.cursor red
```

## Command-Specific Configuration

Configuration options can be specified per command using section headers.

### Script Command Options

The script command does not currently define any command-specific settings. This section is reserved for future enhancements.

```
[script]
# Reserved for future script command options
```

### Code Review Command Options

No dedicated configuration keys are available yet for the code review workflow. This section remains reserved for forthcoming options.

```
[code-review]
# Reserved for future code review options
```

### Prompt Flow Command Options

Prompt flow currently relies on the global configuration. Command-specific options will be documented here when they become available.

```
[prompt-flow]
# Reserved for future prompt flow options
```

## Script Path Discovery Rules

Script paths are discovered and prioritized in the following order:

### 1. Legacy Paths (Always Searched)
For backward compatibility, these paths are always searched:

1. `scripts/` directory relative to the executable
2. `~/.one-shot-man/scripts/` (user scripts)
3. `./scripts/` (current directory scripts)

### 2. Custom Paths (If Configured)
Paths specified in `script.paths` configuration option.

### 3. Autodiscovered Paths (If Enabled)
When `script.autodiscovery` is enabled:

- **Current Directory Pattern Matching**: Searches current directory and parent directories for directories matching `script.path-patterns`
- **Git Repository Discovery** (if `script.git-traversal` enabled): Traverses up from current directory to find git repositories, then searches for script directories within them

### Priority Rules

Scripts are resolved with the following priority (highest to lowest):

1. **Current Working Directory Tree (Class 0)**: Script directories located in the current working directory or its descendants. Direct children outrank deeper paths.
2. **Ancestor Directories (Class 1)**: Script directories that require traversing upward from the current working directory, ordered by the fewest `..` steps required.
3. **User Configuration Scripts (Class 2)**: Paths beneath the resolved configuration directory (respects `ONESHOTMAN_CONFIG`), with shallower paths preferred.
4. **Executable Scripts (Class 3)**: Directories relative to the one-shot-man executable, again ranked by proximity to the executable directory.
5. **Other Scripts (Class 4)**: Any remaining locations.

Because every discovered path is normalized to an absolute location, duplicate entries (even when specified through different relative forms) are eliminated. When multiple git repositories are encountered, the script directories closest to the current working directory automatically outrank those that are further away.

## TUI Color Configuration

The interactive TUI supports color customization through `prompt.color.*` options:

### Supported Color Keys

- `prefix`: Color for the command prompt prefix
- `text`: Default text color
- `cursor`: Cursor color
- `selected`: Color for selected items
- `description`: Color for command descriptions

### Color Value Formats

- **Named Colors**: `black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white`
- **Hex Colors**: `#FF0000`, `#00FF00`, `#0000FF`
- **RGB Colors**: `rgb(255,0,0)`, `rgb(0,255,0)`, `rgb(0,0,255)`

### Example Color Configuration

```
# Bright green prefix
prompt.color.prefix #00FF00

# White text on dark background
prompt.color.text white

# Red cursor
prompt.color.cursor red

# Yellow for selected items
prompt.color.selected yellow

# Gray for descriptions
prompt.color.description #808080
```


## Security Considerations

### Path Traversal Limits

- `script.max-traversal-depth` prevents excessive filesystem traversal
- Autodiscovery is disabled by default
- Git traversal requires explicit enablement

### Path Validation

- All paths are validated before use
- Symbolic links are resolved safely
- Invalid or inaccessible paths are ignored

### Script Execution

- Only executable files are considered as script commands
- Scripts execute with the permissions of the calling user
- No privilege escalation is performed

## Performance Considerations

### Caching

- Script discovery results are cached per registry instance
- Git repository detection uses lightweight git commands
- Directory existence checks are minimized

### Traversal Optimization

- Directory traversal stops at filesystem boundaries
- Maximum depth limits prevent infinite traversal
- Duplicate path detection prevents redundant searches

## Migration from Previous Versions

### Backward Compatibility

- All existing script discovery behavior is preserved
- Configuration is additive - new options don't break existing setups
- Default behavior remains unchanged unless explicitly configured

### Upgrading Configuration

1. Existing configurations continue to work without changes
2. New options can be added gradually
3. Autodiscovery must be explicitly enabled

## Examples

### Basic Script Discovery

```
# Minimal configuration - uses defaults
script.autodiscovery false
```

### Advanced Autodiscovery

```
# Enable full autodiscovery with git traversal
script.autodiscovery true
script.git-traversal true
script.max-traversal-depth 5
script.path-patterns scripts,bin,tools
```

### Custom Script Locations

```
# Additional script paths
script.paths ~/work/scripts:/opt/company-scripts
script.path-patterns scripts,automation
```

### Complete Example Configuration

```
# Global script discovery configuration
script.autodiscovery true
script.git-traversal true
script.max-traversal-depth 8
script.paths ~/dev/scripts:/usr/local/custom-scripts
script.path-patterns scripts,bin,tools,automation

# TUI customization
prompt.color.prefix #00AA00
prompt.color.text white
prompt.color.cursor #FF6600
prompt.color.selected yellow
prompt.color.description #888888

# Script command specific options
[script]
# Any script-command specific options would go here

# Code review command options
[code-review]
# Any code-review specific options would go here
```

## Troubleshooting

### Common Issues

**Scripts Not Found**
1. Check script file permissions (must be executable)
2. Verify script paths exist and are accessible
3. Enable autodiscovery if expecting advanced discovery
4. Check `script.paths` configuration syntax

**Autodiscovery Not Working**
1. Ensure `script.autodiscovery true` is set
2. For git traversal, also set `script.git-traversal true`
3. Check `script.max-traversal-depth` isn't too restrictive
4. Verify you're in a directory tree with script directories

**Performance Issues**
1. Reduce `script.max-traversal-depth` value
2. Limit `script.path-patterns` to essential patterns
3. Use specific `script.paths` instead of relying on autodiscovery

### Debug Information

Use the following commands to debug script discovery:

```bash
# Show all discovered commands
osm help

# List only script commands
osm help | grep -A 100 "Script commands:"

# Test configuration loading
osm config --all
```

## Related Documentation

- [Main README](../README.md) - General usage and overview
- [Script Command Documentation](../README.md#script-commands) - JavaScript scripting details
- [Configuration Management](../README.md#configuration) - Basic configuration guide
- [Custom Goals Example](custom-goal-example.json) - Example custom goal definition

## Custom Goals

Custom goals allow you to define reusable AI workflows with custom commands, state management, and context handling.

### Goal File Format

Custom goals are defined as JSON files with the following structure:

```json
{
  "Name": "my-custom-goal",
  "Description": "Brief description of what this goal does",
  "Category": "category-name",
  "Usage": "Detailed usage information",
  "TUITitle": "Display Title",
  "TUIPrompt": "(prompt) > ",
  "HistoryFile": ".my_custom_goal_history",
  "EnableHistory": true,
  "StateKeys": {
    "contextItems": [],
    "customState": "initial-value"
  },
  "PromptInstructions": "Instructions for the AI assistant...",
  "PromptTemplate": "{{.PromptInstructions}}\\n\\n{{.ContextTxtar}}",
  "ContextHeader": "CONTEXT",
  "BannerText": "Banner displayed on launch",
  "HelpText": "Help text shown with the help command",
  "Commands": [
    {
      "Name": "add",
      "Type": "contextManager"
    },
    {
      "Name": "custom",
      "Type": "custom",
      "Description": "Custom command description",
      "Handler": "function (args) { /* JavaScript handler */ }"
    }
  ]
}
```

### Required Fields

- `Name`: Goal identifier (alphanumeric and hyphens only, no spaces)
- `Description`: Brief description of the goal's purpose

### Discovery Locations

Custom goal files are discovered using a sophisticated score-based prioritization system. When multiple goal files define the same goal name, the file from the highest-priority location is used.

#### Priority Order (Highest to Lowest)

1. **Project-Local Goals (Class 0)**
   - Current working directory: `./osm-goals/` or other patterns in `goal.path-patterns`
   - Subdirectories of the current working directory
   - Closer subdirectories are preferred over deeper ones

2. **Ancestor Directory Goals (Class 1)**
   - Goal directories found by traversing upward from the current directory
   - Closer ancestors (fewer `..` steps) are preferred over more distant ones
   - Must match patterns specified in `goal.path-patterns` (default: `osm-goals`, `goals`)

3. **User Configuration Goals (Class 2)**
   - `~/.one-shot-man/goals/` (or the directory specified by `ONESHOTMAN_CONFIG`)
   - Shallower paths within the config directory are preferred

4. **Executable Directory Goals (Class 3)**
   - `<executable-dir>/goals/` - Goals relative to the `osm` executable location
   - Shallower paths are preferred

5. **Custom Configuration Paths (Evaluated by Class)**
   - Paths specified in the `goal.paths` configuration option
   - Classified and prioritized according to the rules above

6. **Other/Unrelated Paths (Class 4)**
   - Any paths that don't match the above categories
   - Lowest priority

**Note:** All paths are normalized to absolute paths, and duplicates are automatically eliminated. The discovery system ensures that the most contextually relevant goal definition is always selected.

### Goal Naming

- Goal names must be valid identifiers (alphanumeric characters and hyphens)
- No spaces or special characters (except hyphens)
- User goals override built-in goals with the same name
- Goal files should be named `<goal-name>.json`

### Creating Custom Goals

1. Create a goal file in one of the discovery locations:
   ```bash
   mkdir -p ~/.one-shot-man/goals
   nano ~/.one-shot-man/goals/my-goal.json
   ```

2. Define your goal (see [custom-goal-example.json](custom-goal-example.json) for a complete example)

3. Test your goal:
   ```bash
   osm goal -l              # List all goals
   osm goal my-goal         # Launch your custom goal
   ```

### Example: Simple Custom Goal

```json
{
  "Name": "bug-analyzer",
  "Description": "Analyze bug reports and suggest fixes",
  "Category": "debugging",
  "TUITitle": "Bug Analyzer",
  "TUIPrompt": "(bug-analyzer) > ",
  "EnableHistory": true,
  "StateKeys": {
    "contextItems": []
  },
  "PromptInstructions": "Analyze the provided bug report and code to:\n1. Identify the root cause\n2. Suggest potential fixes\n3. Provide reproduction steps",
  "PromptTemplate": "{{.PromptInstructions}}\\n\\n## BUG REPORT\\n\\n{{.ContextTxtar}}",
  "BannerText": "Bug Analyzer - Find and fix issues",
  "Commands": [
    {"Name": "add", "Type": "contextManager"},
    {"Name": "list", "Type": "contextManager"},
    {"Name": "show", "Type": "contextManager"},
    {"Name": "copy", "Type": "contextManager"},
    {"Name": "help", "Type": "help"}
  ]
}
```

### Shell Completion

Shell completion scripts automatically include all discovered goals:

```bash
# Generate and install completion
osm completion bash > /etc/bash_completion.d/osm
osm completion zsh > ~/.zsh/completions/_osm
osm completion fish > ~/.config/fish/completions/osm.fish

# Your custom goals will appear in tab completion:
osm goal <TAB>
```
