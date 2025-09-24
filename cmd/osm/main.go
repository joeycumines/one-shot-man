package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/joeycumines/one-shot-man/internal/command"
	"github.com/joeycumines/one-shot-man/internal/config"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

	// Create command registry with configuration
	registry := command.NewRegistryWithConfig(cfg)

	// Register built-in commands
	helpCmd := command.NewHelpCommand(registry)
	registry.Register(helpCmd)
	registry.Register(command.NewVersionCommand(version))
	registry.Register(command.NewConfigCommand(cfg))
	registry.Register(command.NewInitCommand())
	registry.Register(command.NewScriptingCommand(cfg))
	registry.Register(command.NewPromptFlowCommand(cfg))
	registry.Register(command.NewCodeReviewCommand(cfg))
	registry.Register(command.NewCompletionCommand(registry))
	registry.Register(command.NewGoalsCommand(cfg))

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
		_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmdName)
		_, _ = fmt.Fprintln(os.Stderr, "Use 'osm help' to see available commands.")
		return err
	}

	// Create flag set for this command
	fs := flag.NewFlagSet(cmd.Name(), flag.ExitOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s\n", cmd.Usage())
		_, _ = fmt.Fprintf(os.Stderr, "\n%s\n\n", cmd.Description())
		_, _ = fmt.Fprintln(os.Stderr, "Options:")
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
