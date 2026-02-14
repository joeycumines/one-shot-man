package command

import (
	"bytes"
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
		_, _ = fmt.Fprintln(stdout, "one-shot-man - refine reproducible one-shot prompts from your terminal")
		_, _ = fmt.Fprintln(stdout, "")
		_, _ = fmt.Fprintln(stdout, "Usage: osm <command> [options] [args...]")
		_, _ = fmt.Fprintln(stdout, "")
		_, _ = fmt.Fprintln(stdout, "Available commands:")

		w := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)

		// List built-in commands
		builtins := c.registry.ListBuiltin()
		if len(builtins) > 0 {
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Built-in commands:")
			for _, name := range builtins {
				if cmd, err := c.registry.Get(name); err == nil {
					_, _ = fmt.Fprintf(w, "  %s\t%s\n", name, cmd.Description())
				}
			}
		}

		// List script commands
		scripts := c.registry.ListScript()
		if len(scripts) > 0 {
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, "Script commands:")
			for _, name := range scripts {
				_, _ = fmt.Fprintf(w, "  %s\t%s\n", name, "Script command")
			}
		}

		_ = w.Flush()

		_, _ = fmt.Fprintln(stdout, "")
		_, _ = fmt.Fprintln(stdout, "Use 'osm help <command>' for more information about a specific command (includes flags).")
		return nil
	}

	// Show help for a specific command
	cmdName := args[0]
	cmd, err := c.registry.Get(cmdName)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Unknown command: %s\n", cmdName)
		return err
	}

	_, _ = fmt.Fprintf(stdout, "Command: %s\n", cmd.Name())
	_, _ = fmt.Fprintf(stdout, "Description: %s\n", cmd.Description())
	_, _ = fmt.Fprintf(stdout, "Usage: %s\n", cmd.Usage())

	// Show command-specific flags (if any) by invoking SetupFlags on a temporary FlagSet
	fs := flag.NewFlagSet(cmd.Name(), flag.ContinueOnError)
	buf := &bytes.Buffer{}
	fs.SetOutput(buf)
	cmd.SetupFlags(fs)
	// PrintDefaults writes any defined flags to the FlagSet output
	fs.PrintDefaults()
	if buf.Len() > 0 {
		_, _ = fmt.Fprintln(stdout, "")
		_, _ = fmt.Fprintln(stdout, "Flags:")
		_, _ = fmt.Fprint(stdout, buf.String())
	} else {
		// If this is a script command, hint to run the script with -h
		if _, isScript := cmd.(*ScriptCommand); isScript {
			_, _ = fmt.Fprintln(stdout, "")
			_, _ = fmt.Fprintf(stdout, "Note: this is a script command; run 'osm %s -h' for its help (if supported).\n", cmd.Name())
		}
	}

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
	if len(args) > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %v\n", args)
		return fmt.Errorf("unexpected arguments")
	}
	_, _ = fmt.Fprintf(stdout, "one-shot-man version %s\n", c.version)
	return nil
}

// ConfigCommand manages configuration.
type ConfigCommand struct {
	*BaseCommand
	config     *config.Config
	configPath string
	showGlobal bool
	showAll    bool
}

// NewConfigCommand creates a new config command.
// If configPath is empty, persistence to disk is skipped (useful for tests
// and when the resolved path isn't known at construction time).
func NewConfigCommand(cfg *config.Config, configPath ...string) *ConfigCommand {
	var path string
	if len(configPath) > 0 {
		path = configPath[0]
	}
	return &ConfigCommand{
		BaseCommand: NewBaseCommand(
			"config",
			"Manage configuration settings",
			"config [options] [key] [value]",
		),
		config:     cfg,
		configPath: path,
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
			_, _ = fmt.Fprintln(stdout, "Global configuration:")
			for key, value := range c.config.Global {
				_, _ = fmt.Fprintf(stdout, "  %s: %s\n", key, value)
			}
			_, _ = fmt.Fprintln(stdout, "\nCommand-specific configuration:")
			for cmd, options := range c.config.Commands {
				_, _ = fmt.Fprintf(stdout, "  [%s]\n", cmd)
				for key, value := range options {
					_, _ = fmt.Fprintf(stdout, "    %s: %s\n", key, value)
				}
			}
			return nil
		} else if c.showGlobal {
			// Show global configuration only
			_, _ = fmt.Fprintln(stdout, "Global configuration:")
			for key, value := range c.config.Global {
				_, _ = fmt.Fprintf(stdout, "  %s: %s\n", key, value)
			}
			return nil
		} else {
			// Show usage
			_, _ = fmt.Fprintln(stdout, "Configuration management:")
			_, _ = fmt.Fprintln(stdout, "  config <key>          - Get configuration value")
			_, _ = fmt.Fprintln(stdout, "  config <key> <value>  - Set configuration value")
			_, _ = fmt.Fprintln(stdout, "  config --global       - Show global configuration")
			_, _ = fmt.Fprintln(stdout, "  config --all          - Show all configuration")
			_, _ = fmt.Fprintln(stdout, "  config validate       - Validate configuration")
			_, _ = fmt.Fprintln(stdout, "  config schema         - Show configuration schema")
			return nil
		}
	}

	// Handle subcommands.
	switch args[0] {
	case "validate":
		return c.executeValidate(stdout)
	case "schema":
		_, _ = fmt.Fprint(stdout, config.DefaultSchema().FormatHelp())
		return nil
	}

	if len(args) == 1 {
		// Get configuration value (schema-aware: checks env → config → default).
		key := args[0]
		value := config.DefaultSchema().Resolve(c.config, key)
		if value != "" {
			_, _ = fmt.Fprintf(stdout, "%s: %s\n", key, value)
		} else if _, exists := c.config.GetGlobalOption(key); exists {
			// Value exists but is empty string.
			_, _ = fmt.Fprintf(stdout, "%s: \n", key)
		} else {
			_, _ = fmt.Fprintf(stdout, "Configuration key '%s' not found\n", key)
		}
		return nil
	}

	if len(args) == 2 {
		// Set configuration value
		key, value := args[0], args[1]
		c.config.SetGlobalOption(key, value)

		// Persist to disk if a config path is available
		configPath := c.configPath
		if configPath == "" {
			// Best-effort resolve; if it fails, skip disk write
			configPath, _ = config.GetConfigPath()
		}
		if configPath != "" {
			if err := config.SetKeyInFile(configPath, key, value); err != nil {
				_, _ = fmt.Fprintf(stderr, "Warning: failed to persist config to disk: %v\n", err)
			}
		}

		_, _ = fmt.Fprintf(stdout, "Set configuration: %s = %s\n", key, value)
		return nil
	}

	_, _ = fmt.Fprintln(stderr, "Invalid number of arguments")
	return fmt.Errorf("invalid arguments")
}

// executeValidate validates the current config against the schema.
func (c *ConfigCommand) executeValidate(stdout io.Writer) error {
	issues := config.ValidateConfig(c.config, config.DefaultSchema())
	if len(issues) == 0 {
		_, _ = fmt.Fprintln(stdout, "Configuration is valid.")
		return nil
	}
	_, _ = fmt.Fprintf(stdout, "Configuration has %d issue(s):\n", len(issues))
	for _, issue := range issues {
		_, _ = fmt.Fprintf(stdout, "  - %s\n", issue)
	}
	return nil
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
	if len(args) > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected arguments: %v\n", args)
		return fmt.Errorf("unexpected arguments")
	}
	// Get config path and ensure directory exists
	configPath, err := config.GetConfigPath()
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	// Check if config already exists
	if _, err := os.Stat(configPath); err == nil && !c.force {
		_, _ = fmt.Fprintf(stdout, "Configuration already exists at: %s\n", configPath)
		_, _ = fmt.Fprintln(stdout, "Use --force to overwrite existing configuration")
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

# Prompt color overrides (optional). Keys mirror go-prompt color roles.
# Example values: black,darkred,darkgreen,brown,darkblue,purple,cyan,lightgray
#                 darkgray,red,green,yellow,blue,fuchsia,turquoise,white
# NOTE: Input defaults to green (for readability). Uncomment to customize.
# prompt.color.input green
# prompt.color.prefix cyan
# prompt.color.suggestionText yellow
# prompt.color.suggestionBackground black
# prompt.color.selectedSuggestionText black
# prompt.color.selectedSuggestionBackground cyan
# prompt.color.descriptionText white
# prompt.color.descriptionBackground black
# prompt.color.selectedDescriptionText white
# prompt.color.selectedDescriptionBackground blue
# prompt.color.scrollbarThumb darkgray
# prompt.color.scrollbarBackground black

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
		_, _ = fmt.Fprintf(stderr, "Warning: Failed to load created config: %v\n", err)
	} else {
		// Test the configuration functions
		if verbose, exists := testConfig.GetGlobalOption("verbose"); exists {
			_, _ = fmt.Fprintf(stdout, "Created configuration with verbose=%s\n", verbose)
		}
		if pager, exists := testConfig.GetCommandOption("help", "pager"); exists {
			_, _ = fmt.Fprintf(stdout, "Help command will use pager: %s\n", pager)
		}
	}

	_, _ = fmt.Fprintf(stdout, "Initialized one-shot-man configuration at: %s\n", configPath)
	return nil
}
