package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joeycumines/one-shot-man/internal/command"
	"github.com/joeycumines/one-shot-man/internal/config"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// If config doesn't exist, create a new empty one
		cfg = config.NewConfig()
	}

	// Create command registry
	registry := command.NewRegistry()

	// Add script paths
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		registry.AddScriptPath(filepath.Join(execDir, "scripts"))
	}

	// Add user script path
	if configPath, err := config.GetConfigPath(); err == nil {
		configDir := filepath.Dir(configPath)
		registry.AddScriptPath(filepath.Join(configDir, "scripts"))
	}

	// Add current directory scripts
	if cwd, err := os.Getwd(); err == nil {
		registry.AddScriptPath(filepath.Join(cwd, "scripts"))
	}

	// Register built-in commands
	helpCmd := command.NewHelpCommand(registry)
	registry.Register(helpCmd)
	registry.Register(command.NewVersionCommand(version))
	registry.Register(command.NewConfigCommand(cfg))
	registry.Register(command.NewInitCommand())
	registry.Register(command.NewScriptingCommand(cfg))
	registry.Register(command.NewPromptFlowCommand(cfg))
	registry.Register(command.NewCodeReviewCommand(cfg))

	// Parse global flags and command
	if len(os.Args) < 2 {
		// No command specified, show help
		return helpCmd.Execute([]string{}, os.Stdout, os.Stderr)
	}

	cmdName := os.Args[1]

	// Handle special case for help
	if cmdName == "-h" || cmdName == "--help" {
		return helpCmd.Execute([]string{}, os.Stdout, os.Stderr)
	}

	// Get the command
	cmd, err := registry.Get(cmdName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmdName)
		fmt.Fprintln(os.Stderr, "Use 'one-shot-man help' to see available commands.")
		return err
	}

	// Create flag set for this command
	fs := flag.NewFlagSet(cmd.Name(), flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s\n", cmd.Usage())
		fmt.Fprintf(os.Stderr, "\n%s\n\n", cmd.Description())
		fmt.Fprintln(os.Stderr, "Options:")
		fs.PrintDefaults()
	}

	// Let the command setup its flags
	cmd.SetupFlags(fs)

	// Parse command-specific flags
	cmdArgs := os.Args[2:]
	if err := fs.Parse(cmdArgs); err != nil {
		return err
	}

	// Execute the command with remaining arguments
	return cmd.Execute(fs.Args(), os.Stdout, os.Stderr)
}
