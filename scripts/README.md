# Example Scripts

This directory contains example JavaScript scripts that demonstrate various features of the one-shot-man scripting system.

## Script Categories

### Examples and Tutorials

- **`demo-mode.js`** - Simple example showing basic mode registration with commands and state management
- **`llm-prompt-builder.js`** - Advanced example demonstrating rich TUI features for building LLM prompts

### Testing and Debugging

- **`debug-tui.js`** - Testing script to validate TUI API bindings
- **`test-advanced-prompt.js`** - Tests advanced prompt features like completion and key bindings
- **`demo-completion-fix.js`** - Tests completion system functionality

## Usage

### Running Examples

```bash
# Load and execute a script
osm script scripts/demo-mode.js

# Run script in interactive mode
osm script -i scripts/demo-mode.js

# Run with test/debug output
osm script --test scripts/debug-tui.js
```

### Learning from Examples

1. **Start with `demo-mode.js`** - Shows basic mode registration, commands, and state
2. **Study `llm-prompt-builder.js`** - Advanced example with complex data structures and workflows
3. **Use `debug-tui.js`** - Validate your environment and test API availability

## Key Concepts Demonstrated

### Mode Registration
- Basic mode setup with title and prompt
- Lifecycle functions (onEnter, onExit, onPrompt)
- Mode-specific commands
- State management per mode

### Command System
- Global command registration
- Mode-specific commands
- Argument handling and validation
- Usage documentation

### Native Module Integration
- Using `osm:` prefixed modules
- File operations with `osm:os`
- Command execution with `osm:exec`
- Context building with `osm:ctxutil`

### Advanced TUI Features
- Custom prompt creation
- Completion system
- Key bindings
- Color customization
- History management

## Creating Your Own Scripts

1. Create a new `.js` file in this directory
2. Use the patterns from existing examples
3. Test with `osm script --test your-script.js`
4. Run interactively with `osm script -i your-script.js`

See the [internal API documentation](../docs/internal-api.md) for comprehensive API reference.