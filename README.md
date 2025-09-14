# one-shot-man

Command one-shot-man lets you produce high quality implementations with drastically less effort, keeping track of your context, using a simple, extensible, REPL-based, wizard-style workflow.

## Features

- **Extensible Command System**: Support for both built-in and script-based subcommands
- **Kubectl-style Configuration**: Configuration file management with environment variable override
- **dnsmasq-style Config Format**: Simple `optionName remainingLineIsTheValue` format
- **Script Command Discovery**: Automatic discovery and execution of script commands
- **Stdlib Flag Package**: Built using Go's standard library flag package
- **REPL/Wizard Support**: Script commands can implement interactive workflows

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
```

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
./one-shot-man script scripts/example.js
```

Now you can run it:

```bash
one-shot-man example --wizard
```

## Architecture

### Built-in Commands

- `help` - Display help information
- `version` - Show version information  
- `config` - Manage configuration settings
- `init` - Initialize the one-shot-man environment

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

## Development

### Running Tests

```bash
go test ./...
```

### Building

```bash
go build -o one-shot-man ./cmd/one-shot-man
```

## Examples

### Basic Usage

```bash
# Get help
one-shot-man help

# Initialize configuration  
one-shot-man init

# Run a script command with wizard mode
one-shot-man hello --wizard
```

### Configuration Management

```bash
# Initialize with custom location
ONESHOTMAN_CONFIG=/tmp/myconfig one-shot-man init

# Force re-initialize
one-shot-man init --force
```

## License

See [LICENSE](LICENSE) file for details.
