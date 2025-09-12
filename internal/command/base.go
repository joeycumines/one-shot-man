package command

import (
	"flag"
	"io"
)

// Command represents a command that can be executed.
type Command interface {
	// Name returns the command name.
	Name() string

	// Description returns a short description of the command.
	Description() string

	// Usage returns the usage string for the command.
	Usage() string

	// SetupFlags configures the flag.FlagSet for this command.
	// The FlagSet will be used to parse command-specific arguments.
	SetupFlags(fs *flag.FlagSet)

	// Execute runs the command with the given arguments.
	// args contains the arguments after flags have been parsed.
	Execute(args []string, stdout, stderr io.Writer) error
}

// BaseCommand provides a basic implementation that other commands can embed.
type BaseCommand struct {
	name        string
	description string
	usage       string
}

// NewBaseCommand creates a new BaseCommand.
func NewBaseCommand(name, description, usage string) *BaseCommand {
	return &BaseCommand{
		name:        name,
		description: description,
		usage:       usage,
	}
}

// Name returns the command name.
func (c *BaseCommand) Name() string {
	return c.name
}

// Description returns the command description.
func (c *BaseCommand) Description() string {
	return c.description
}

// Usage returns the command usage.
func (c *BaseCommand) Usage() string {
	return c.usage
}

// SetupFlags is a default implementation that does nothing.
// Commands should override this to add their specific flags.
func (c *BaseCommand) SetupFlags(fs *flag.FlagSet) {
	// Default implementation - no flags
}
