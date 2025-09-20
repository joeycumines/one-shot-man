package command

import (
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// HelpCommand displays help information for commands.
type HelpCommand struct {
	*BaseCommand
	registry *Registry
}

// NewHelpCommand creates a new help command.
func NewHelpCommand(registry *Registry) *HelpCommand {
	return &HelpCommand{
		BaseCommand: NewBaseCommand(
			"help",
			"Display help information for commands",
			"help [command]",
		),
		registry: registry,
	}
}

// Execute displays help information.
func (c *HelpCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		// Show general help and list all commands
		fmt.Fprintln(stdout, "one-shot-man - Command one-shot-man lets you produce high quality implementations with drastically less effort")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Usage: one-shot-man <command> [options] [args...]")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Available commands:")

		w := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)

		// List built-in commands
		builtins := c.registry.ListBuiltin()
		if len(builtins) > 0 {
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "Built-in commands:")
			for _, name := range builtins {
				if cmd, err := c.registry.Get(name); err == nil {
					fmt.Fprintf(w, "  %s\t%s\n", name, cmd.Description())
				}
			}
		}

		// List script commands
		scripts := c.registry.ListScript()
		if len(scripts) > 0 {
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "Script commands:")
			for _, name := range scripts {
				fmt.Fprintf(w, "  %s\t%s\n", name, "Script command")
			}
		}

		w.Flush()

		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Use 'one-shot-man help <command>' for more information about a specific command.")
		return nil
	}

	// Show help for a specific command
	cmdName := args[0]
	cmd, err := c.registry.Get(cmdName)
	if err != nil {
		fmt.Fprintf(stderr, "Unknown command: %s\n", cmdName)
		return err
	}

	fmt.Fprintf(stdout, "Command: %s\n", cmd.Name())
	fmt.Fprintf(stdout, "Description: %s\n", cmd.Description())
	fmt.Fprintf(stdout, "Usage: %s\n", cmd.Usage())

	return nil
}

// VersionCommand displays version information.
type VersionCommand struct {
	*BaseCommand
	version string
}

// NewVersionCommand creates a new version command.
func NewVersionCommand(version string) *VersionCommand {
	return &VersionCommand{
		BaseCommand: NewBaseCommand(
			"version",
			"Display version information",
			"version",
		),
		version: version,
	}
}

// Execute displays version information.
func (c *VersionCommand) Execute(args []string, stdout, stderr io.Writer) error {
	fmt.Fprintf(stdout, "one-shot-man version %s\n", c.version)
	return nil
}

// ConfigCommand manages configuration.
type ConfigCommand struct {
	*BaseCommand
	config     *config.Config
	showGlobal bool
	showAll    bool
}

// NewConfigCommand creates a new config command.
func NewConfigCommand(cfg *config.Config) *ConfigCommand {
	return &ConfigCommand{
		BaseCommand: NewBaseCommand(
			"config",
			"Manage configuration settings",
			"config [options] [key] [value]",
		),
		config: cfg,
	}
}

// SetupFlags configures the flags for the config command.
func (c *ConfigCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.showGlobal, "global", false, "Show only global configuration")
	fs.BoolVar(&c.showAll, "all", false, "Show all configuration (global and command-specific)")
}

// Execute manages configuration.
func (c *ConfigCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		if c.showAll {
			// Show all configuration
			fmt.Fprintln(stdout, "Global configuration:")
			for key, value := range c.config.Global {
				fmt.Fprintf(stdout, "  %s: %s\n", key, value)
			}
			fmt.Fprintln(stdout, "\nCommand-specific configuration:")
			for cmd, options := range c.config.Commands {
				fmt.Fprintf(stdout, "  [%s]\n", cmd)
				for key, value := range options {
					fmt.Fprintf(stdout, "    %s: %s\n", key, value)
				}
			}
			return nil
		} else if c.showGlobal {
			// Show global configuration only
			fmt.Fprintln(stdout, "Global configuration:")
			for key, value := range c.config.Global {
				fmt.Fprintf(stdout, "  %s: %s\n", key, value)
			}
			return nil
		} else {
			// Show usage
			fmt.Fprintln(stdout, "Configuration management:")
			fmt.Fprintln(stdout, "  config <key>          - Get configuration value")
			fmt.Fprintln(stdout, "  config <key> <value>  - Set configuration value")
			fmt.Fprintln(stdout, "  config --global       - Show global configuration")
			fmt.Fprintln(stdout, "  config --all          - Show all configuration")
			return nil
		}
	}

	if len(args) == 1 {
		// Get configuration value
		key := args[0]
		if value, exists := c.config.GetGlobalOption(key); exists {
			fmt.Fprintf(stdout, "%s: %s\n", key, value)
		} else {
			fmt.Fprintf(stdout, "Configuration key '%s' not found\n", key)
		}
		return nil
	}

	if len(args) == 2 {
		// Set configuration value
		key, value := args[0], args[1]
		c.config.SetGlobalOption(key, value)
		fmt.Fprintf(stdout, "Set configuration: %s = %s\n", key, value)
		return nil
	}

	fmt.Fprintln(stderr, "Invalid number of arguments")
	return fmt.Errorf("invalid arguments")
}

// InitCommand initializes the one-shot-man environment.
type InitCommand struct {
	*BaseCommand
	force bool
}

// NewInitCommand creates a new init command.
func NewInitCommand() *InitCommand {
	return &InitCommand{
		BaseCommand: NewBaseCommand(
			"init",
			"Initialize one-shot-man environment",
			"init [options]",
		),
	}
}

// SetupFlags configures the flags for the init command.
func (c *InitCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.force, "force", false, "Force initialization even if config already exists")
}

// Execute initializes the environment.
func (c *InitCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Get config path and ensure directory exists
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil && !c.force {
		fmt.Fprintf(stdout, "Configuration already exists at: %s\n", configPath)
		fmt.Fprintln(stdout, "Use --force to overwrite existing configuration")
		return nil
	}

	// Ensure config directory exists
	if err := config.EnsureConfigDir(); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create default configuration
	defaultConfig := `# one-shot-man configuration file
# Format: optionName remainingLineIsTheValue
# Use [command_name] sections for command-specific options

# Global options
verbose false
color auto

# Example command-specific options
[help]
pager less

[version]
format full
`

	if err := os.WriteFile(configPath, []byte(defaultConfig), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Load and test the configuration
	testConfig, err := config.LoadFromPath(configPath)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: Failed to load created config: %v\n", err)
	} else {
		// Test the configuration functions
		if verbose, exists := testConfig.GetGlobalOption("verbose"); exists {
			fmt.Fprintf(stdout, "Created configuration with verbose=%s\n", verbose)
		}
		if pager, exists := testConfig.GetCommandOption("help", "pager"); exists {
			fmt.Fprintf(stdout, "Help command will use pager: %s\n", pager)
		}
	}

	fmt.Fprintf(stdout, "Initialized one-shot-man configuration at: %s\n", configPath)
	return nil
}
