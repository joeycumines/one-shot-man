package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/joeycumines/one-shot-man/internal/command"
	"github.com/joeycumines/one-shot-man/internal/config"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		if !command.IsSilent(err) {
			_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		// LoadFromPath already returns NewConfig() for os.IsNotExist,
		// so any error here is a real problem (permissions, parse,
		// symlink attack). Warn but continue with empty config.
		_, _ = fmt.Fprintf(os.Stderr, "Warning: failed to load config: %v\n", err)
		cfg = config.NewConfig()
	}

	// Create command registry with configuration
	registry := command.NewRegistryWithConfig(cfg)

	// Sync startup: auto-pull if configured, then inject discovery paths.
	command.SyncAutoPull(cfg, os.Stderr)
	command.ApplySyncDiscoveryPaths(cfg)

	// Create goal registry
	goalDiscovery := command.NewGoalDiscovery(cfg)
	goalRegistry := command.NewDynamicGoalRegistry(command.GetBuiltInGoals(), goalDiscovery)

	// Resolve config path for commands that need it
	configPath, configPathErr := config.GetConfigPath()
	if configPathErr != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Warning: unable to resolve config path: %v\n", configPathErr)
	}

	// Register built-in commands
	helpCmd := command.NewHelpCommand(registry)
	registry.Register(helpCmd)
	registry.Register(command.NewVersionCommand(version))
	registry.Register(command.NewConfigCommand(cfg, configPath))
	registry.Register(command.NewInitCommand())
	registry.Register(command.NewScriptingCommand(cfg))
	registry.Register(command.NewSessionCommand(cfg))
	registry.Register(command.NewPromptFlowCommand(cfg))
	registry.Register(command.NewCodeReviewCommand(cfg))
	registry.Register(command.NewSuperDocumentCommand(cfg))
	registry.Register(command.NewCompletionCommand(registry, goalRegistry))
	registry.Register(command.NewGoalCommand(cfg, goalRegistry))
	registry.Register(command.NewSyncCommand(cfg))
	registry.Register(command.NewLogCommand(cfg))
	registry.Register(command.NewPrSplitCommand(cfg))
	registry.Register(command.NewMcpBridgeCommand())

	// Parse global flags and command. Avoid manual inspection of args for
	// help tokens; instead rely on the flag package so we consistently
	// support -h and -help at the top level.
	globalFS := flag.NewFlagSet("osm", flag.ContinueOnError)
	globalFS.SetOutput(io.Discard)
	var showHelp bool
	globalFS.BoolVar(&showHelp, "h", false, "Show help")
	globalFS.BoolVar(&showHelp, "help", false, "Show help")
	// Parse will stop at the first non-flag token (the command name), so
	// we can safely parse top-level help flags without consuming subcommand
	// args.
	if err := globalFS.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return helpCmd.Execute([]string{}, os.Stdout, os.Stderr)
		}
		return err
	}

	// Use the remaining args returned by the global flagset rather than
	// blindly indexing into os.Args. This prevents brittle behavior where
	// global flags shift argument positions (e.g. `osm -v session`).
	gargs := globalFS.Args()
	if showHelp || len(gargs) < 1 {
		return helpCmd.Execute([]string{}, os.Stdout, os.Stderr)
	}
	cmdName := gargs[0]

	// Get the command
	cmd, err := registry.Get(cmdName)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmdName)
		_, _ = fmt.Fprintln(os.Stderr, "Use 'osm help' to see available commands; use 'osm help <command>' for details (includes flags).")
		return &command.SilentError{Err: err}
	}

	// Create flag set for this command
	// Use ContinueOnError so we can handle help consistently across
	// top-level commands and subcommands. This avoids os.Exit from the
	// stdlib's default ExitOnError behavior and makes exit codes
	// consistent for scripting and tests.
	fs := flag.NewFlagSet(cmd.Name(), flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// fullUsage prints the complete command help (Usage, Description,
	// flag defaults). Used only for explicit --help requests.
	fullUsage := func() {
		_, _ = fmt.Fprintf(os.Stderr, "Usage: %s\n", cmd.Usage())
		_, _ = fmt.Fprintf(os.Stderr, "\n%s\n\n", cmd.Description())
		_, _ = fmt.Fprintln(os.Stderr, "Options:")
		fs.SetOutput(os.Stderr)
		fs.PrintDefaults()
		fs.SetOutput(io.Discard)
	}

	// T392: Suppress the automatic full-usage dump on parse errors.
	// Go's flag package calls fs.Usage() from failf() before returning
	// errors. Setting it to a no-op during parsing prevents the wall of
	// text that obscures the actual error message. We restore it only
	// for explicit --help requests.
	fs.Usage = func() {} // no-op during parse

	// Let the command setup its flags
	cmd.SetupFlags(fs)

	// Parse flags using the FlagSet so flag values are correctly
	// associated with their flags (avoids breaking flags that accept
	// values, e.g. `-config file.yaml`). The FlagSet will stop parsing
	// at the first non-flag token and the remaining arguments can be
	// retrieved with fs.Args().
	// Parse only the arguments belonging to this command (everything after
	// the command name in the global flagset's remaining args).
	cmdArgs := gargs[1:]
	if err := fs.Parse(cmdArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			// flag.ErrHelp indicates usage/help was requested; treat as
			// non-error so the program can exit successfully and uniformly.
			fullUsage()
			return nil
		}
		// Parse error: show a concise 1-line error + hint instead of the
		// full flag listing that Go's flag package would normally dump.
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		_, _ = fmt.Fprintf(os.Stderr, "Use 'osm %s --help' for usage.\n", cmd.Name())
		return &command.SilentError{Err: err}
	}

	// Execute the command with the arguments remaining after parsing
	// (these are the non-flag args or subcommand-specific args).
	return cmd.Execute(fs.Args(), os.Stdout, os.Stderr)
}
