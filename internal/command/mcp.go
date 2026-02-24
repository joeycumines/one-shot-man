package command

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	server := newMCPServer(cm, c.goalRegistry, c.version, "")
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
	SessionID     string            `json:"sessionId"`
	Capabilities  []string          `json:"capabilities"`
	Status        string            `json:"status"`
	Progress      float64           `json:"progress"`
	LastUpdate    time.Time         `json:"lastUpdate"`
	LastHeartbeat time.Time         `json:"lastHeartbeat"`
	LastSeq       int64             `json:"-"` // highest processed sequence number
	Events        []mcpSessionEvent `json:"-"` // drained on getSession

	// Pending queues: set by osm-facing tools, drained on getSession.
	PendingClassification *mcpRequestClassificationInput `json:"-"`
	PendingPlanRequest    *mcpRequestSplitPlanInput      `json:"-"`
	PendingConflicts      []mcpReportConflictInput       `json:"-"`
	PendingInstructions   []mcpSendInstructionInput      `json:"-"`
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

	// Pending requests from the orchestrator (drained on read).
	PendingClassification *mcpRequestClassificationInput `json:"pendingClassification,omitempty"`
	PendingPlanRequest    *mcpRequestSplitPlanInput      `json:"pendingPlanRequest,omitempty"`
	PendingConflicts      []mcpReportConflictInput       `json:"pendingConflicts,omitempty"`
	PendingInstructions   []mcpSendInstructionInput      `json:"pendingInstructions,omitempty"`
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

// --- MCP PR-split tool input types ---

type mcpReportClassificationInput struct {
	SessionID string            `json:"sessionId" jsonschema:"Session identifier"`
	Files     map[string]string `json:"files" jsonschema:"Map of file path to category name"`
	Seq       int64             `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

type mcpReportSplitPlanInput struct {
	SessionID string          `json:"sessionId" jsonschema:"Session identifier"`
	Stages    []mcpSplitStage `json:"stages" jsonschema:"Ordered array of split stages"`
	Seq       int64           `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

// mcpSplitStage describes one stage in a PR split plan.
type mcpSplitStage struct {
	Name    string   `json:"name" jsonschema:"Branch/stage name"`
	Files   []string `json:"files" jsonschema:"Files in this stage"`
	Message string   `json:"message" jsonschema:"Commit message for this stage"`
	Order   int      `json:"order" jsonschema:"0-based ordering"`
}

// --- MCP bidirectional split protocol input types ---

// mcpRepoContext provides repository metadata for classification requests.
type mcpRepoContext struct {
	ModulePath string `json:"modulePath,omitempty" jsonschema:"Go module path from go.mod"`
	Language   string `json:"language,omitempty" jsonschema:"Primary language (go, js, python, etc.)"`
	BaseRef    string `json:"baseRef,omitempty" jsonschema:"Base branch or reference"`
}

type mcpRequestClassificationInput struct {
	SessionID string            `json:"sessionId" jsonschema:"Session identifier"`
	Files     map[string]string `json:"files" jsonschema:"Map of file path to git status (A, M, D, R)"`
	Context   mcpRepoContext    `json:"context" jsonschema:"Repository context"`
	MaxGroups int               `json:"maxGroups,omitempty" jsonschema:"Maximum groups (0 = no limit)"`
}

// mcpPlanConstraints constrains split plan generation.
type mcpPlanConstraints struct {
	MaxFilesPerSplit  int    `json:"maxFilesPerSplit,omitempty" jsonschema:"Max files per split (0 = no limit)"`
	BranchPrefix      string `json:"branchPrefix,omitempty" jsonschema:"Branch name prefix"`
	PreferIndependent bool   `json:"preferIndependent,omitempty" jsonschema:"Prefer independently mergeable splits"`
}

type mcpRequestSplitPlanInput struct {
	SessionID      string             `json:"sessionId" jsonschema:"Session identifier"`
	Classification map[string]string  `json:"classification" jsonschema:"Map of file path to category"`
	Constraints    mcpPlanConstraints `json:"constraints" jsonschema:"Plan generation constraints"`
}

type mcpReportConflictInput struct {
	SessionID    string   `json:"sessionId" jsonschema:"Session identifier"`
	BranchName   string   `json:"branchName" jsonschema:"Branch that failed verification"`
	VerifyOutput string   `json:"verifyOutput" jsonschema:"Verify command stdout+stderr"`
	ExitCode     int      `json:"exitCode" jsonschema:"Verify command exit code"`
	Files        []string `json:"files" jsonschema:"Files in the failing branch"`
	GoModContent string   `json:"goModContent,omitempty" jsonschema:"go.mod content if applicable"`
	Seq          int64    `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

// mcpFilePatch describes a file content replacement in a conflict resolution.
type mcpFilePatch struct {
	File    string `json:"file" jsonschema:"File path to patch"`
	Content string `json:"content" jsonschema:"New file content (full replacement)"`
}

type mcpReportResolutionInput struct {
	SessionID        string         `json:"sessionId" jsonschema:"Session identifier"`
	BranchName       string         `json:"branchName" jsonschema:"Branch being fixed"`
	Patches          []mcpFilePatch `json:"patches,omitempty" jsonschema:"File content replacements"`
	Commands         []string       `json:"commands,omitempty" jsonschema:"Commands to run for fix"`
	ReSplitSuggested bool           `json:"reSplitSuggested,omitempty" jsonschema:"Suggest re-classification"`
	ReSplitReason    string         `json:"reSplitReason,omitempty" jsonschema:"Why re-split is needed"`
	Seq              int64          `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

type mcpSendInstructionInput struct {
	SessionID string `json:"sessionId" jsonschema:"Session identifier"`
	Type      string `json:"type" jsonschema:"Instruction type: abort, modify-plan, re-classify, focus"`
	Payload   any    `json:"payload,omitempty" jsonschema:"Type-dependent instruction payload"`
}

type mcpAcknowledgeInstructionInput struct {
	SessionID       string `json:"sessionId" jsonschema:"Session identifier"`
	InstructionType string `json:"instructionType" jsonschema:"Type of instruction being acknowledged"`
	Status          string `json:"status" jsonschema:"Ack status: received, executing, completed, rejected"`
	Message         string `json:"message,omitempty" jsonschema:"Optional status message"`
	Seq             int64  `json:"seq,omitempty" jsonschema:"Sequence number for idempotency (0 = no dedup)"`
}

// mcpValidProgressStatuses is the set of allowed status values for reportProgress.
var mcpValidProgressStatuses = map[string]bool{
	"working": true,
	"blocked": true,
	"waiting": true,
	"idle":    true,
}

// mcpValidInstructionTypes is the set of allowed steering instruction types.
var mcpValidInstructionTypes = map[string]bool{
	"abort":       true,
	"modify-plan": true,
	"re-classify": true,
	"focus":       true,
}

// mcpValidAckStatuses is the set of allowed acknowledgement statuses.
var mcpValidAckStatuses = map[string]bool{
	"received":  true,
	"executing": true,
	"completed": true,
	"rejected":  true,
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

// newMCPServer creates a configured MCP server with all tools.
// It is unexported for testability via InMemoryTransport.
// If resultDir is non-empty, PR-split tools (reportClassification,
// reportSplitPlan) also write results atomically to files in that directory.
func newMCPServer(cm *scripting.ContextManager, goalRegistry GoalRegistry, version string, resultDir string) *mcp.Server {
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
		Description: "Get session info, drain queued events, and retrieve pending requests (classification, plan, conflicts, instructions)",
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
		sess.Events = nil // drain events

		// Drain pending queues.
		pendingClassification := sess.PendingClassification
		sess.PendingClassification = nil
		pendingPlanRequest := sess.PendingPlanRequest
		sess.PendingPlanRequest = nil
		var pendingConflicts []mcpReportConflictInput
		if len(sess.PendingConflicts) > 0 {
			pendingConflicts = sess.PendingConflicts
			sess.PendingConflicts = nil
		}
		var pendingInstructions []mcpSendInstructionInput
		if len(sess.PendingInstructions) > 0 {
			pendingInstructions = sess.PendingInstructions
			sess.PendingInstructions = nil
		}

		resp := mcpGetSessionResponse{
			SessionID:             sess.SessionID,
			Capabilities:          sess.Capabilities,
			Status:                sess.Status,
			Progress:              sess.Progress,
			LastUpdate:            sess.LastUpdate,
			LastHeartbeat:         sess.LastHeartbeat,
			LastSeq:               sess.LastSeq,
			Events:                events,
			PendingClassification: pendingClassification,
			PendingPlanRequest:    pendingPlanRequest,
			PendingConflicts:      pendingConflicts,
			PendingInstructions:   pendingInstructions,
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

	// --- reportClassification ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reportClassification",
		Description: "Report file classification results for PR splitting. Each file is mapped to a category (e.g., 'types', 'impl', 'docs'). Call this tool to send classification results back to the orchestrating osm process.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpReportClassificationInput) (*mcp.CallToolResult, any, error) {
		if len(input.Files) == 0 {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("files map is required and must not be empty"))
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
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "classification",
			Timestamp: now,
			Data:      input.Files,
		})
		mu.Unlock()

		// Write result file atomically if result-dir is set.
		if resultDir != "" {
			if err := mcpWriteResultFile(resultDir, "classification.json", input.Files); err != nil {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("failed to write result file: %w", err))
				return result, nil, nil
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("classification reported: %d files", len(input.Files))}},
		}, nil, nil
	})

	// --- reportSplitPlan ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reportSplitPlan",
		Description: "Report a suggested PR split plan with ordered stages. Each stage has a name, file list, commit message, and order. Call this tool to send the split plan back to the orchestrating osm process.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpReportSplitPlanInput) (*mcp.CallToolResult, any, error) {
		if len(input.Stages) == 0 {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("stages array is required and must not be empty"))
			return result, nil, nil
		}
		// Validate stages: each must have name and files.
		allFiles := make(map[string]string)
		for i, stage := range input.Stages {
			if stage.Name == "" {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("stage at index %d has no name", i))
				return result, nil, nil
			}
			if len(stage.Files) == 0 {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("stage %q has no files", stage.Name))
				return result, nil, nil
			}
			for _, f := range stage.Files {
				if prev, dup := allFiles[f]; dup {
					result := &mcp.CallToolResult{}
					result.SetError(fmt.Errorf("duplicate file %q in stages %q and %q", f, prev, stage.Name))
					return result, nil, nil
				}
				allFiles[f] = stage.Name
			}
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
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "split-plan",
			Timestamp: now,
			Data:      input.Stages,
		})
		mu.Unlock()

		// Write result file atomically if result-dir is set.
		if resultDir != "" {
			if err := mcpWriteResultFile(resultDir, "split-plan.json", input.Stages); err != nil {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("failed to write result file: %w", err))
				return result, nil, nil
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("split plan reported: %d stages", len(input.Stages))}},
		}, nil, nil
	})

	// --- requestClassification ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "requestClassification",
		Description: "Request the agent to classify changed files into categories for PR splitting. The request is stored in the session and retrieved by the agent via getSession.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpRequestClassificationInput) (*mcp.CallToolResult, any, error) {
		if len(input.Files) == 0 {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("files map is required and must not be empty"))
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
		sess.LastUpdate = now
		sess.PendingClassification = &input
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("classification requested: %d files", len(input.Files))}},
		}, nil, nil
	})

	// --- requestSplitPlan ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "requestSplitPlan",
		Description: "Request the agent to generate a split plan based on file classification. The request is stored in the session and retrieved by the agent via getSession.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpRequestSplitPlanInput) (*mcp.CallToolResult, any, error) {
		if len(input.Classification) == 0 {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("classification map is required and must not be empty"))
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
		sess.LastUpdate = now
		sess.PendingPlanRequest = &input
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("split plan requested: %d classified files", len(input.Classification))}},
		}, nil, nil
	})

	// --- reportConflict ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reportConflict",
		Description: "Report that a split branch failed verification. Stores the conflict in the session for the agent to read via getSession, and optionally writes to result-dir.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpReportConflictInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(input.BranchName) == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("branchName is required"))
			return result, nil, nil
		}
		if len(input.Files) == 0 {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("files must not be empty"))
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
		sess.PendingConflicts = append(sess.PendingConflicts, input)
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "conflict",
			Timestamp: now,
			Data: map[string]any{
				"branchName": input.BranchName,
				"exitCode":   input.ExitCode,
				"fileCount":  len(input.Files),
			},
		})
		mu.Unlock()

		// Write conflict file atomically if result-dir is set.
		if resultDir != "" {
			filename := fmt.Sprintf("%s-conflict.json", input.BranchName)
			if err := mcpWriteResultFile(resultDir, filename, input); err != nil {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("failed to write conflict file: %w", err))
				return result, nil, nil
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("conflict reported: %s (exit %d)", input.BranchName, input.ExitCode)}},
		}, nil, nil
	})

	// --- reportResolution ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "reportResolution",
		Description: "Report a proposed resolution for a verification conflict. Called by the agent to suggest fixes (patches, commands, or re-split).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpReportResolutionInput) (*mcp.CallToolResult, any, error) {
		if strings.TrimSpace(input.BranchName) == "" {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("branchName is required"))
			return result, nil, nil
		}
		if len(input.Patches) == 0 && len(input.Commands) == 0 && !input.ReSplitSuggested {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("resolution must include patches, commands, or re-split suggestion"))
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
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "resolution",
			Timestamp: now,
			Data: map[string]any{
				"branchName":       input.BranchName,
				"patchCount":       len(input.Patches),
				"commandCount":     len(input.Commands),
				"reSplitSuggested": input.ReSplitSuggested,
			},
		})
		mu.Unlock()

		// Write resolution file atomically if result-dir is set.
		if resultDir != "" {
			filename := fmt.Sprintf("%s-resolution.json", input.BranchName)
			if err := mcpWriteResultFile(resultDir, filename, input); err != nil {
				result := &mcp.CallToolResult{}
				result.SetError(fmt.Errorf("failed to write resolution file: %w", err))
				return result, nil, nil
			}
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("resolution reported: %s (%d patches, %d commands)", input.BranchName, len(input.Patches), len(input.Commands))}},
		}, nil, nil
	})

	// --- sendInstruction ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "sendInstruction",
		Description: "Send a steering instruction to the agent mid-task. The instruction is queued in the session and retrieved by the agent via getSession.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpSendInstructionInput) (*mcp.CallToolResult, any, error) {
		if !mcpValidInstructionTypes[input.Type] {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("invalid instruction type %q: must be one of abort, modify-plan, re-classify, focus", input.Type))
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
		sess.LastUpdate = now
		sess.PendingInstructions = append(sess.PendingInstructions, input)
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "instruction",
			Timestamp: now,
			Data: map[string]any{
				"type":    input.Type,
				"payload": input.Payload,
			},
		})
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("instruction sent: %s", input.Type)}},
		}, nil, nil
	})

	// --- acknowledgeInstruction ---
	mcp.AddTool(server, &mcp.Tool{
		Name:        "acknowledgeInstruction",
		Description: "Acknowledge receipt of a steering instruction. Called by the agent to confirm it received and is processing an instruction.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input mcpAcknowledgeInstructionInput) (*mcp.CallToolResult, any, error) {
		if !mcpValidInstructionTypes[input.InstructionType] {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("invalid instruction type %q: must be one of abort, modify-plan, re-classify, focus", input.InstructionType))
			return result, nil, nil
		}
		if !mcpValidAckStatuses[input.Status] {
			result := &mcp.CallToolResult{}
			result.SetError(fmt.Errorf("invalid ack status %q: must be one of received, executing, completed, rejected", input.Status))
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
		sess.Events = append(sess.Events, mcpSessionEvent{
			Type:      "instruction-ack",
			Timestamp: now,
			Data: map[string]any{
				"instructionType": input.InstructionType,
				"status":          input.Status,
				"message":         input.Message,
			},
		})
		mu.Unlock()
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("instruction acknowledged: %s (%s)", input.InstructionType, input.Status)}},
		}, nil, nil
	})

	return server
}

// mcpWriteResultFile atomically writes v as indented JSON to dir/filename.
// It writes to a temp file first, then renames for crash safety.
// If dir is empty, it is a no-op (returns nil).
func mcpWriteResultFile(dir, filename string, v any) error {
	if dir == "" {
		return nil
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	target := filepath.Join(dir, filename)
	parent := filepath.Dir(target)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	return os.Rename(tmp, target)
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
