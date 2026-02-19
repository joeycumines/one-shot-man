package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

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

// --- MCP session types (bidirectional agent communication) ---

// mcpSession tracks an active agent session.
type mcpSession struct {
	SessionID    string            `json:"sessionId"`
	Capabilities []string          `json:"capabilities"`
	Status       string            `json:"status"`
	Progress     float64           `json:"progress"`
	LastUpdate   time.Time         `json:"lastUpdate"`
	LastHeartbeat time.Time        `json:"lastHeartbeat"`
	LastSeq      int64             `json:"-"` // highest processed sequence number
	Events       []mcpSessionEvent `json:"-"` // drained on getSession
}

// mcpSessionEvent is a queued event from an agent session.
type mcpSessionEvent struct {
	Type      string    `json:"type"` // "progress", "result", "guidance"
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// mcpGetSessionResponse is the JSON response for getSession.
type mcpGetSessionResponse struct {
	SessionID     string            `json:"sessionId"`
	Capabilities  []string          `json:"capabilities"`
	Status        string            `json:"status"`
	Progress      float64           `json:"progress"`
	LastUpdate    time.Time         `json:"lastUpdate"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	LastSeq       int64             `json:"lastSeq"`
	Events        []mcpSessionEvent `json:"events"`
}

// mcpSessionSummary is a JSON-serializable session summary for listSessions.
type mcpSessionSummary struct {
	SessionID     string    `json:"sessionId"`
	Capabilities  []string  `json:"capabilities"`
	Status        string    `json:"status"`
	Progress      float64   `json:"progress"`
	LastUpdate    time.Time `json:"lastUpdate"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
	EventCount    int       `json:"eventCount"`
}

// --- MCP session input types ---

type mcpRegisterSessionInput struct {
	SessionID    string   `json:"sessionId" jsonschema:"Unique session identifier"`
	Capabilities []string `json:"capabilities" jsonschema:"List of agent capabilities"`
}

type mcpReportProgressInput struct {
	SessionID string  `json:"sessionId" jsonschema:"Session identifier"`
	Status    string  `json:"status" jsonschema:"Current status (working, blocked, waiting, idle)"`
	Progress  float64 `json:"progress" jsonschema:"Completion percentage (0-100)"`
	Message   string  `json:"message" jsonschema:"Human-readable progress message"`
	Seq       int64   `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

type mcpReportResultInput struct {
	SessionID    string   `json:"sessionId" jsonschema:"Session identifier"`
	Success      bool     `json:"success" jsonschema:"Whether the task succeeded"`
	Output       string   `json:"output" jsonschema:"Task output or error message"`
	FilesChanged []string `json:"filesChanged,omitempty" jsonschema:"List of modified files"`
	Seq          int64    `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

type mcpRequestGuidanceInput struct {
	SessionID string   `json:"sessionId" jsonschema:"Session identifier"`
	Question  string   `json:"question" jsonschema:"Question for the human operator"`
	Options   []string `json:"options,omitempty" jsonschema:"Available options"`
	Context   string   `json:"context,omitempty" jsonschema:"Additional context"`
	Seq       int64    `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

type mcpGetSessionInput struct {
	SessionID string `json:"sessionId" jsonschema:"Session identifier"`
}

type mcpHeartbeatInput struct {
	SessionID string `json:"sessionId" jsonschema:"Session identifier"`
}

// mcpValidProgressStatuses is the set of allowed status values for reportProgress.
var mcpValidProgressStatuses = map[string]bool{
	"working": true,
	"blocked": true,
	"waiting": true,
	"idle":    true,
}

// mcpMaxSessionIDLen is the maximum length for a session identifier.
const mcpMaxSessionIDLen = 256

// mcpValidateSessionID checks that a session ID is non-empty, within length
// limits, and contains only printable non-whitespace characters (except space
// in the middle).
func mcpValidateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("sessionId is required")
	}
	if len(id) > mcpMaxSessionIDLen {
		return fmt.Errorf("sessionId exceeds maximum length of %d characters", mcpMaxSessionIDLen)
	}
	for _, r := range id {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("sessionId contains invalid character: %q", string(r))
		}
	}
	return nil
}

// mcpCheckSeq returns true if the operation should be processed based on the
// sequence number. If seq > 0 and <= sess.LastSeq, the operation is a duplicate
// and should be skipped (returns false). If seq > sess.LastSeq, updates LastSeq
// (returns true). If seq == 0, always processes (returns true, no dedup).
// Caller must hold mu.
func mcpCheckSeq(sess *mcpSession, seq int64) bool {
	if seq <= 0 {
		return true // no dedup
	}
	if seq <= sess.LastSeq {
		return false // duplicate
	}
	sess.LastSeq = seq
	return true
}

// newMCPServer creates a configured MCP server with all fifteen tools.
// It is unexported for testability via InMemoryTransport.
func newMCPServer(cm *scripting.ContextManager, goalRegistry GoalRegistry, version string) *mcp.Server {
	var mu sync.Mutex
	var items []mcpContextItem
	sessions := make(map[string]*mcpSession)

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

	// --- registerSession ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "registerSession",
		Description: "Register a new agent session with capabilities",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpRegisterSessionInput) (*mcp.CallToolResult, any, error) {
		if err := mcpValidateSessionID(input.SessionID); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(err)
			return result, nil, nil
		}
		caps := input.Capabilities
		if caps == nil {
			caps = []string{}
		}
		now := time.Now()
		mu.Lock()
		sessions[input.SessionID] = &mcpSession{
			SessionID:     input.SessionID,
			Capabilities:  caps,
			Status:        "idle",
			Progress:      0,
			LastUpdate:    now,
			LastHeartbeat: now,
		}
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("session registered: %s", input.SessionID)}},
		}, nil, nil
	})

	// --- reportProgress ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reportProgress",
		Description: "Report progress from an agent session",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpReportProgressInput) (*mcp.CallToolResult, any, error) {
		if !mcpValidProgressStatuses[input.Status] {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("invalid status %q: must be one of working, blocked, waiting, idle", input.Status))
			return result, nil, nil
		}
		progress := input.Progress
		if progress < 0 {
			progress = 0
		}
		if progress > 100 {
			progress = 100
		}
		mu.Lock()
		sess, ok := sessions[input.SessionID]
		if !ok {
			mu.Unlock()
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("session not found: %s", input.SessionID))
			return result, nil, nil
		}
		if !mcpCheckSeq(sess, input.Seq) {
			mu.Unlock()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("duplicate seq %d (idempotent skip)", input.Seq)}},
			}, nil, nil
		}
		now := time.Now()
		sess.Status = input.Status
		sess.Progress = progress
		sess.LastUpdate = now
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "progress",
			Timestamp: now,
			Data: map[string]any{
				"status":   input.Status,
				"progress": progress,
				"message":  input.Message,
			},
		})
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("progress reported: %s %.0f%%", input.Status, progress)}},
		}, nil, nil
	})

	// --- reportResult ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reportResult",
		Description: "Report task completion from an agent session",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpReportResultInput) (*mcp.CallToolResult, any, error) {
		mu.Lock()
		sess, ok := sessions[input.SessionID]
		if !ok {
			mu.Unlock()
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("session not found: %s", input.SessionID))
			return result, nil, nil
		}
		if !mcpCheckSeq(sess, input.Seq) {
			mu.Unlock()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("duplicate seq %d (idempotent skip)", input.Seq)}},
			}, nil, nil
		}
		now := time.Now()
		if input.Success {
			sess.Status = "idle"
		}
		sess.LastUpdate = now
		filesChanged := input.FilesChanged
		if filesChanged == nil {
			filesChanged = []string{}
		}
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "result",
			Timestamp: now,
			Data: map[string]any{
				"success":      input.Success,
				"output":       input.Output,
				"filesChanged": filesChanged,
			},
		})
		mu.Unlock()
		outcome := "succeeded"
		if !input.Success {
			outcome = "failed"
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("result reported: %s", outcome)}},
		}, nil, nil
	})

	// --- requestGuidance ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "requestGuidance",
		Description: "Request guidance from the human operator",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpRequestGuidanceInput) (*mcp.CallToolResult, any, error) {
		if input.Question == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("question is required"))
			return result, nil, nil
		}
		mu.Lock()
		sess, ok := sessions[input.SessionID]
		if !ok {
			mu.Unlock()
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("session not found: %s", input.SessionID))
			return result, nil, nil
		}
		if !mcpCheckSeq(sess, input.Seq) {
			mu.Unlock()
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("duplicate seq %d (idempotent skip)", input.Seq)}},
			}, nil, nil
		}
		now := time.Now()
		sess.LastUpdate = now
		options := input.Options
		if options == nil {
			options = []string{}
		}
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "guidance",
			Timestamp: now,
			Data: map[string]any{
				"question": input.Question,
				"options":  options,
				"context":  input.Context,
			},
		})
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("guidance requested: %s", input.Question)}},
		}, nil, nil
	})

	// --- getSession ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "getSession",
		Description: "Get session info and drain queued events",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpGetSessionInput) (*mcp.CallToolResult, any, error) {
		mu.Lock()
		sess, ok := sessions[input.SessionID]
		if !ok {
			mu.Unlock()
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("session not found: %s", input.SessionID))
			return result, nil, nil
		}
		events := sess.Events
		if events == nil {
			events = []mcpSessionEvent{}
		}
		sess.Events = nil // drain
		resp := mcpGetSessionResponse{
			SessionID:     sess.SessionID,
			Capabilities:  sess.Capabilities,
			Status:        sess.Status,
			Progress:      sess.Progress,
			LastUpdate:    sess.LastUpdate,
			LastHeartbeat: sess.LastHeartbeat,
			LastSeq:       sess.LastSeq,
			Events:        events,
		}
		mu.Unlock()
		data, err := json.Marshal(resp)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal session: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- listSessions ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "listSessions",
		Description: "List all registered agent sessions",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input struct{}) (*mcp.CallToolResult, any, error) {
		mu.Lock()
		summaries := make([]mcpSessionSummary, 0, len(sessions))
		for _, sess := range sessions {
			summaries = append(summaries, mcpSessionSummary{
				SessionID:     sess.SessionID,
				Capabilities:  sess.Capabilities,
				Status:        sess.Status,
				Progress:      sess.Progress,
				LastUpdate:    sess.LastUpdate,
				LastHeartbeat: sess.LastHeartbeat,
				EventCount:    len(sess.Events),
			})
		}
		mu.Unlock()
		data, err := json.Marshal(summaries)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to marshal sessions: %w", err)
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
		}, nil, nil
	})

	// --- heartbeat ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "heartbeat",
		Description: "Update session heartbeat timestamp to indicate the agent is still alive",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpHeartbeatInput) (*mcp.CallToolResult, any, error) {
		if err := mcpValidateSessionID(input.SessionID); err != nil {
			result := &mcp.CallToolResult{}
			result.SetError(err)
			return result, nil, nil
		}
		mu.Lock()
		sess, ok := sessions[input.SessionID]
		if !ok {
			mu.Unlock()
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("session not found: %s", input.SessionID))
			return result, nil, nil
		}
		now := time.Now()
		sess.LastHeartbeat = now
		sess.LastUpdate = now
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("heartbeat: %s", input.SessionID)}},
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
