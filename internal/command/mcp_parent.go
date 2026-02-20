package command

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPParentCommand starts an MCP server over stdio that provides agent
// steering tools. It connects to a running orchestrator's control socket
// (T116-T117) and exposes enqueue, interrupt, and status operations as
// MCP tools.
//
// Usage: osm mcp-parent --socket <path>
//
// This enables a parent Claude Code process (or any MCP client) to
// manage a claude-mux orchestration session via standard MCP protocol.
type MCPParentCommand struct {
	*BaseCommand
	socketPath string // path to the orchestrator's control socket

	// clientOverride allows tests to inject a mock control client.
	clientOverride mcpParentClient
}

// mcpParentClient abstracts the control socket client for testability.
type mcpParentClient interface {
	EnqueueTask(task string) (int, error)
	InterruptCurrent() error
	GetStatus() (*claudemux.GetStatusResult, error)
}

// NewMCPParentCommand creates a new mcp-parent command.
func NewMCPParentCommand() *MCPParentCommand {
	return &MCPParentCommand{
		BaseCommand: NewBaseCommand(
			"mcp-parent",
			"Start MCP server for agent steering (stdio transport)",
			"osm mcp-parent --socket <path>",
		),
	}
}

// SetupFlags configures the flags for the mcp-parent command.
func (c *MCPParentCommand) SetupFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.socketPath, "socket", "", "Path to orchestrator control socket")
}

// Execute starts the MCP server on stdio, blocking until the client disconnects.
func (c *MCPParentCommand) Execute(args []string, _ /*stdout*/, _ /*stderr*/ io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("mcp-parent: unexpected arguments: %v", args)
	}
	if c.socketPath == "" {
		return fmt.Errorf("mcp-parent: --socket flag is required")
	}

	client := c.resolveClient()
	server := c.newParentServer(client)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// resolveClient returns the control client (real or mock).
func (c *MCPParentCommand) resolveClient() mcpParentClient {
	if c.clientOverride != nil {
		return c.clientOverride
	}
	return claudemux.NewControlClient(c.socketPath)
}

// --- MCP tool input types ---

type mcpEnqueueInput struct {
	Task string `json:"task" jsonschema:"Task description to enqueue for the orchestrator"`
}

type mcpInterruptInput struct{}

type mcpGetStatusInput struct{}

// --- MCP server factory ---

// newParentServer creates an MCP server with agent steering tools.
func (c *MCPParentCommand) newParentServer(client mcpParentClient) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "osm-mcp-parent",
			Version: "0.1.0",
		},
		nil,
	)

	// Tool: enqueue_task
	mcp.AddTool(server, &mcp.Tool{
		Name:        "enqueue_task",
		Description: "Submit a task to the claude-mux orchestrator queue",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpEnqueueInput) (*mcp.CallToolResult, any, error) {
		if input.Task == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("task description is required"))
			return result, nil, nil
		}

		pos, err := client.EnqueueTask(input.Task)
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("enqueue failed: %w", err))
			return result, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("Task enqueued at position %d", pos)},
			},
		}, nil, nil
	})

	// Tool: interrupt_current
	mcp.AddTool(server, &mcp.Tool{
		Name:        "interrupt_current",
		Description: "Interrupt the currently active task in the orchestrator",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpInterruptInput) (*mcp.CallToolResult, any, error) {
		if err := client.InterruptCurrent(); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("interrupt failed: %w", err))
			return result, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: "Interrupt sent to active task"},
			},
		}, nil, nil
	})

	// Tool: get_status
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_status",
		Description: "Get current orchestrator status (active task, queue)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpGetStatusInput) (*mcp.CallToolResult, any, error) {
		status, err := client.GetStatus()
		if err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("status failed: %w", err))
			return result, nil, nil
		}
		text := fmt.Sprintf("Active: %s\nQueue depth: %d",
			valueOrNone(status.ActiveTask), status.QueueDepth)
		for i, q := range status.Queue {
			text += fmt.Sprintf("\n  [%d] %s", i, q)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: text},
			},
		}, nil, nil
	})

	return server
}
