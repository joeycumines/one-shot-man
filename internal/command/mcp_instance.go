package command

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPInstanceCommand starts a per-instance MCP server over stdio for a
// specific session. This is the counterpart to MCPInstanceConfig: Claude
// Code spawns this command with the session ID, and osm provides MCP
// tools over stdin/stdout.
//
// Usage: osm mcp-instance --session <session-id> [--result-dir <path>]
type MCPInstanceCommand struct {
	*BaseCommand
	goalRegistry GoalRegistry
	version      string
	session      string
	resultDir    string
}

// NewMCPInstanceCommand creates a new MCPInstanceCommand.
func NewMCPInstanceCommand(goalRegistry GoalRegistry, version string) *MCPInstanceCommand {
	return &MCPInstanceCommand{
		BaseCommand:  NewBaseCommand("mcp-instance", "Start per-instance MCP server (stdio, for Claude Code)", "osm mcp-instance --session <session-id>"),
		goalRegistry: goalRegistry,
		version:      version,
	}
}

// SetupFlags configures the --session and --result-dir flags.
func (c *MCPInstanceCommand) SetupFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.session, "session", "", "Session identifier for this MCP instance")
	fs.StringVar(&c.resultDir, "result-dir", "", "Directory for structured result files (classification.json, split-plan.json)")
}

// Execute starts a per-instance MCP server on stdio, blocking until the
// client disconnects. The --session flag is required.
func (c *MCPInstanceCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if c.session == "" {
		return fmt.Errorf("mcp-instance: --session flag is required")
	}
	if len(args) > 0 {
		return fmt.Errorf("mcp-instance: unexpected arguments: %v", args)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("mcp-instance: failed to get working directory: %w", err)
	}
	cm, err := scripting.NewContextManager(cwd)
	if err != nil {
		return fmt.Errorf("mcp-instance: failed to create context manager: %w", err)
	}

	// Ensure result-dir exists if specified.
	if c.resultDir != "" {
		if err := os.MkdirAll(c.resultDir, 0o755); err != nil {
			return fmt.Errorf("mcp-instance: failed to create result-dir: %w", err)
		}
	}

	// Reuse the same MCP server factory as the main 'osm mcp' command,
	// with optional result directory for structured PR-split results.
	server := newMCPServer(cm, c.goalRegistry, c.version, c.resultDir, c.session)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}
