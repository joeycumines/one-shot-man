package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPCommand starts an MCP (Model Context Protocol) server over stdio,
// exposing osm's context management and prompt building as tools.
type MCPCommand struct {
	*BaseCommand
	cfg          *config.Config
	goalRegistry GoalRegistry
	version      string
}

// NewMCPCommand creates a new MCPCommand.
func NewMCPCommand(cfg *config.Config, goalRegistry GoalRegistry, version string) *MCPCommand {
	return &MCPCommand{
		BaseCommand:  NewBaseCommand("mcp", "Start MCP server (stdio transport)", "osm mcp"),
		cfg:          cfg,
		goalRegistry: goalRegistry,
		version:      version,
	}
}

// Execute starts the MCP server on stdio, blocking until the client disconnects.
func (c *MCPCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("mcp: unexpected arguments: %v", args)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("mcp: failed to get working directory: %w", err)
	}
	cm, err := scripting.NewContextManager(cwd)
	if err != nil {
		return fmt.Errorf("mcp: failed to create context manager: %w", err)
	}
	server := newMCPServer(cm, c.goalRegistry, c.version)
	return server.Run(context.Background(), &mcp.StdioTransport{})
}

// --- MCP input types (unexported; used for schema inference) ---

type mcpAddFileInput struct {
	Path string `json:"path" jsonschema:"File or directory path to add to context"`
}

type mcpAddDiffInput struct {
	Diff  string `json:"diff" jsonschema:"Diff content (unified diff format)"`
	Label string `json:"label,omitempty" jsonschema:"Optional label for this diff"`
}

type mcpAddNoteInput struct {
	Text  string `json:"text" jsonschema:"Note text to include in prompt"`
	Label string `json:"label,omitempty" jsonschema:"Optional label for this note"`
}

type mcpBuildPromptInput struct {
	GoalName string `json:"goalName,omitempty" jsonschema:"Optional goal name to include instructions for"`
}

type mcpRemoveFileInput struct {
	Path string `json:"path" jsonschema:"File or directory path to remove from context"`
}

// mcpContextItem holds a note or diff added via MCP tools.
type mcpContextItem struct {
	itemType string // "note" or "diff"
	label    string
	payload  string
}

// mcpListContextEntry is a JSON-serializable item for listContext output.
type mcpListContextEntry struct {
	Type  string `json:"type"`
	Label string `json:"label"`
}

// mcpGoalInfo is a JSON-serializable goal summary for getGoals output.
type mcpGoalInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category,omitempty"`
}

// newMCPServer creates a configured MCP server with all eight tools.
// It is unexported for testability via InMemoryTransport.
func newMCPServer(cm *scripting.ContextManager, goalRegistry GoalRegistry, version string) *mcp.Server {
	var mu sync.Mutex
	var items []mcpContextItem

	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "osm",
			Version: version,
		},
		nil,
	)

	// --- addFile ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "addFile",
		Description: "Add a file or directory to the prompt context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpAddFileInput) (*mcp.CallToolResult, any, error) {
		if input.Path == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("path is required"))
			return result, nil, nil
		}
		if err := cm.AddPath(input.Path); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(err)
			return result, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("added: %s", input.Path)}},
		}, nil, nil
	})

	// --- addDiff ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "addDiff",
		Description: "Add a diff to the prompt context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpAddDiffInput) (*mcp.CallToolResult, any, error) {
		if input.Diff == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("diff content is required"))
			return result, nil, nil
		}
		label := input.Label
		if label == "" {
			label = "diff"
		}
		mu.Lock()
		items = append(items, mcpContextItem{itemType: "diff", label: label, payload: input.Diff})
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("added diff: %s", label)}},
		}, nil, nil
	})

	// --- addNote ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "addNote",
		Description: "Add a text note to the prompt context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpAddNoteInput) (*mcp.CallToolResult, any, error) {
		if input.Text == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("text is required"))
			return result, nil, nil
		}
		label := input.Label
		if label == "" {
			label = "note"
		}
		mu.Lock()
		items = append(items, mcpContextItem{itemType: "note", label: label, payload: input.Text})
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("added note: %s", label)}},
		}, nil, nil
	})

	// --- listContext ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "listContext",
		Description: "List all files, diffs, and notes currently in the prompt context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		files := cm.ListPaths()
		if files == nil {
			files = []string{}
		}
		mu.Lock()
		entries := make([]mcpListContextEntry, len(items))
		for i, item := range items {
			entries[i] = mcpListContextEntry{Type: item.itemType, Label: item.label}
		}
		mu.Unlock()
		data, err := json.Marshal(struct {
			Files []string              `json:"files"`
			Items []mcpListContextEntry `json:"items"`
		}{Files: files, Items: entries})
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal context list: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- buildPrompt ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "buildPrompt",
		Description: "Build the complete prompt from current context, optionally with a goal",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpBuildPromptInput) (*mcp.CallToolResult, any, error) {
		var sb strings.Builder

		// Goal instructions
		if input.GoalName != "" {
			goal, err := goalRegistry.Get(input.GoalName)
			if err != nil {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("goal not found: %w", err))
				return result, nil, nil
			}
			if goal.PromptInstructions != "" {
				sb.WriteString("## Instructions\n\n")
				sb.WriteString(goal.PromptInstructions)
				sb.WriteString("\n\n")
			}
		}

		// Notes and diffs
		mu.Lock()
		itemsCopy := make([]mcpContextItem, len(items))
		copy(itemsCopy, items)
		mu.Unlock()

		for _, item := range itemsCopy {
			sb.WriteString("### ")
			sb.WriteString(item.label)
			sb.WriteString("\n\n")
			fence := mcpBacktickFence(item.payload)
			if item.itemType == "diff" {
				sb.WriteString(fence)
				sb.WriteString("diff\n")
			} else {
				sb.WriteString(fence)
				sb.WriteString("\n")
			}
			sb.WriteString(item.payload)
			if !strings.HasSuffix(item.payload, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString(fence)
			sb.WriteString("\n\n")
		}

		// File context (txtar)
		txtarStr := cm.GetTxtarString()
		if txtarStr != "" {
			sb.WriteString("## Context Files\n\n")
			sb.WriteString(txtarStr)
		}

		prompt := sb.String()
		if prompt == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "(empty context — add files, diffs, or notes first)"}},
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: prompt}},
		}, nil, nil
	})

	// --- getGoals ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "getGoals",
		Description: "List all available goals with their descriptions",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		allGoals := goalRegistry.GetAllGoals()
		infos := make([]mcpGoalInfo, len(allGoals))
		for i, g := range allGoals {
			infos[i] = mcpGoalInfo{
				Name:        g.Name,
				Description: g.Description,
				Category:    g.Category,
			}
		}
		data, err := json.Marshal(infos)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal goals: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- removeFile ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "removeFile",
		Description: "Remove a file or directory from the prompt context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpRemoveFileInput) (*mcp.CallToolResult, any, error) {
		if input.Path == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("path is required"))
			return result, nil, nil
		}
		if err := cm.RemovePath(input.Path); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(err)
			return result, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("removed: %s", input.Path)}},
		}, nil, nil
	})

	// --- clearContext ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "clearContext",
		Description: "Remove all files, diffs, and notes from the prompt context",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		cm.Clear()
		mu.Lock()
		items = nil
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "context cleared"}},
		}, nil, nil
	})

	return server
}

// mcpBacktickFence returns a backtick fence string that is safe
// for the given content (at least 3 backticks, longer if the content
// itself contains a run of backticks).
func mcpBacktickFence(content string) string {
	maxRun := 2
	run := 0
	for _, r := range content {
		if r == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", maxRun+1)
}
