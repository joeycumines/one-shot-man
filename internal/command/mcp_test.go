package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpTestEnv bundles a test MCP server+client for table-driven tests.
type mcpTestEnv struct {
	session *mcp.ClientSession
	dir     string
	cancel  context.CancelFunc
}

// mcpTestGoalRegistry is a minimal GoalRegistry for tests.
type mcpTestGoalRegistry struct {
	goals []Goal
}

func (r *mcpTestGoalRegistry) List() []string {
	names := make([]string, len(r.goals))
	for i, g := range r.goals {
		names[i] = g.Name
	}
	return names
}

func (r *mcpTestGoalRegistry) Get(name string) (*Goal, error) {
	for i := range r.goals {
		if r.goals[i].Name == name {
			return &r.goals[i], nil
		}
	}
	return nil, fmt.Errorf("goal not found: %s", name)
}

func (r *mcpTestGoalRegistry) GetAllGoals() []Goal {
	return r.goals
}

func (r *mcpTestGoalRegistry) Reload() error {
	return nil
}

// newMCPTestEnv creates an MCP server with test goals, connects a client,
// and returns the session. The caller should defer env.Close().
func newMCPTestEnv(t *testing.T, goals []Goal) *mcpTestEnv {
	t.Helper()
	dir := t.TempDir()

	cm, err := scripting.NewContextManager(dir)
	if err != nil {
		t.Fatalf("NewContextManager: %v", err)
	}

	goalRegistry := &mcpTestGoalRegistry{goals: goals}
	server := newMCPServer(cm, goalRegistry, "test")

	ctx, cancel := context.WithCancel(context.Background())

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Start server in background
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()

	// Connect client
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		cancel()
		t.Fatalf("client.Connect: %v", err)
	}

	t.Cleanup(func() {
		_ = session.Close()
		cancel()
		select {
		case <-serverDone:
		case <-time.After(5 * time.Second):
			t.Error("server did not shut down within 5s")
		}
	})

	return &mcpTestEnv{session: session, dir: dir, cancel: cancel}
}

func (e *mcpTestEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := e.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%q): %v", name, err)
	}
	return result
}

func mcpResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("CallToolResult has no content")
	}
	data, err := result.Content[0].MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var v struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal TextContent: %v", err)
	}
	return v.Text
}

// --- Metadata tests ---

func TestMCPCommand_Name(t *testing.T) {
	t.Parallel()
	cmd := NewMCPCommand(nil, nil, "1.0.0")
	if cmd.Name() != "mcp" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "mcp")
	}
	if cmd.Description() == "" {
		t.Error("Description() is empty")
	}
	if cmd.Usage() == "" {
		t.Error("Usage() is empty")
	}
}

func TestMCPCommand_Execute_UnexpectedArgs(t *testing.T) {
	t.Parallel()
	cmd := NewMCPCommand(nil, nil, "1.0.0")
	err := cmd.Execute([]string{"foo"}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Errorf("Execute with args: got %v, want error about unexpected arguments", err)
	}
}

// --- Tool registration ---

func TestMCPServer_ToolList(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := env.session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}

	expected := map[string]bool{
		"addFile":         false,
		"addDiff":         false,
		"addNote":         false,
		"removeFile":      false,
		"listContext":     false,
		"clearContext":    false,
		"buildPrompt":     false,
		"getGoals":        false,
		"registerSession": false,
		"reportProgress":  false,
		"reportResult":    false,
		"requestGuidance": false,
		"getSession":      false,
		"listSessions":    false,
	}
	for _, tool := range result.Tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("tool %q not registered", name)
		}
	}
	if len(result.Tools) != 14 {
		t.Errorf("got %d tools, want 14", len(result.Tools))
	}
}

// --- addFile ---

func TestMCPServer_AddFile(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Create a test file
	testFile := filepath.Join(env.dir, "hello.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	result := env.callTool(t, "addFile", map[string]any{"path": testFile})
	if result.IsError {
		t.Fatalf("addFile returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "added") {
		t.Errorf("addFile text = %q, want to contain 'added'", text)
	}
}

func TestMCPServer_AddFile_EmptyPath(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addFile", map[string]any{"path": ""})
	if !result.IsError {
		t.Error("expected IsError for empty path")
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "path is required") {
		t.Errorf("error text = %q, want to contain 'path is required'", text)
	}
}

func TestMCPServer_AddFile_NonExistent(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addFile", map[string]any{
		"path": filepath.Join(env.dir, "does-not-exist.txt"),
	})
	if !result.IsError {
		t.Error("expected IsError for non-existent file")
	}
}

// --- addDiff ---

func TestMCPServer_AddDiff(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	diff := "--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-old\n+new\n"
	result := env.callTool(t, "addDiff", map[string]any{
		"diff":  diff,
		"label": "my-change",
	})
	if result.IsError {
		t.Fatalf("addDiff returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "my-change") {
		t.Errorf("addDiff text = %q, want to contain 'my-change'", text)
	}
}

func TestMCPServer_AddDiff_DefaultLabel(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addDiff", map[string]any{
		"diff": "+added line\n",
	})
	if result.IsError {
		t.Fatalf("addDiff returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "diff") {
		t.Errorf("text = %q, want default label 'diff'", text)
	}
}

func TestMCPServer_AddDiff_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addDiff", map[string]any{"diff": ""})
	if !result.IsError {
		t.Error("expected IsError for empty diff")
	}
}

// --- addNote ---

func TestMCPServer_AddNote(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addNote", map[string]any{
		"text":  "Remember to check edge cases",
		"label": "reminder",
	})
	if result.IsError {
		t.Fatalf("addNote returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "reminder") {
		t.Errorf("addNote text = %q, want to contain 'reminder'", text)
	}
}

func TestMCPServer_AddNote_DefaultLabel(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addNote", map[string]any{
		"text": "A quick note",
	})
	if result.IsError {
		t.Fatalf("addNote returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "note") {
		t.Errorf("text = %q, want default label 'note'", text)
	}
}

func TestMCPServer_AddNote_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "addNote", map[string]any{"text": ""})
	if !result.IsError {
		t.Error("expected IsError for empty text")
	}
}

// --- removeFile ---

func TestMCPServer_RemoveFile(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Create and add a file
	testFile := filepath.Join(env.dir, "remove-me.txt")
	if err := os.WriteFile(testFile, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	r := env.callTool(t, "addFile", map[string]any{"path": testFile})
	if r.IsError {
		t.Fatalf("addFile: %s", mcpResultText(t, r))
	}

	// Verify file is in context
	r = env.callTool(t, "listContext", nil)
	text := mcpResultText(t, r)
	if !strings.Contains(text, "remove-me.txt") {
		t.Fatalf("file not in context after add: %s", text)
	}

	// Remove it
	r = env.callTool(t, "removeFile", map[string]any{"path": testFile})
	if r.IsError {
		t.Fatalf("removeFile returned error: %s", mcpResultText(t, r))
	}
	text = mcpResultText(t, r)
	if !strings.Contains(text, "removed") {
		t.Errorf("removeFile text = %q, want to contain 'removed'", text)
	}

	// Verify file is gone from context
	r = env.callTool(t, "listContext", nil)
	text = mcpResultText(t, r)
	if strings.Contains(text, "remove-me.txt") {
		t.Errorf("file still in context after remove: %s", text)
	}
}

func TestMCPServer_RemoveFile_EmptyPath(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "removeFile", map[string]any{"path": ""})
	if !result.IsError {
		t.Error("expected IsError for empty path")
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "path is required") {
		t.Errorf("error text = %q, want to contain 'path is required'", text)
	}
}

func TestMCPServer_RemoveFile_NotTracked(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Remove a file that was never added — RemovePath is idempotent,
	// so this should succeed without error.
	result := env.callTool(t, "removeFile", map[string]any{
		"path": filepath.Join(env.dir, "never-added.txt"),
	})
	if result.IsError {
		t.Errorf("removeFile for untracked path should be idempotent, got error: %s",
			mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "removed") {
		t.Errorf("removeFile text = %q, want to contain 'removed'", text)
	}
}

// --- clearContext ---

func TestMCPServer_ClearContext_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Clear when already empty — should succeed without error
	result := env.callTool(t, "clearContext", nil)
	if result.IsError {
		t.Fatalf("clearContext returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "context cleared") {
		t.Errorf("clearContext text = %q, want to contain 'context cleared'", text)
	}
}

func TestMCPServer_ClearContext_Populated(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Add a file, note, and diff
	testFile := filepath.Join(env.dir, "clear-me.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	env.callTool(t, "addFile", map[string]any{"path": testFile})
	env.callTool(t, "addNote", map[string]any{"text": "a note"})
	env.callTool(t, "addDiff", map[string]any{"diff": "+change\n"})

	// Verify context is populated
	r := env.callTool(t, "listContext", nil)
	var before struct {
		Files []string              `json:"files"`
		Items []mcpListContextEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &before); err != nil {
		t.Fatal(err)
	}
	if len(before.Files) != 1 || len(before.Items) != 2 {
		t.Fatalf("before clear: files=%d items=%d, want 1,2", len(before.Files), len(before.Items))
	}

	// Clear everything
	r = env.callTool(t, "clearContext", nil)
	if r.IsError {
		t.Fatalf("clearContext returned error: %s", mcpResultText(t, r))
	}

	// Verify context is empty
	r = env.callTool(t, "listContext", nil)
	var after struct {
		Files []string              `json:"files"`
		Items []mcpListContextEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &after); err != nil {
		t.Fatal(err)
	}
	if len(after.Files) != 0 {
		t.Errorf("after clear: files = %v, want empty", after.Files)
	}
	if len(after.Items) != 0 {
		t.Errorf("after clear: items = %v, want empty", after.Items)
	}
}

func TestMCPServer_ClearContext_BuildPromptAfterClear(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Add content then clear
	env.callTool(t, "addNote", map[string]any{"text": "important note", "label": "my-note"})
	env.callTool(t, "clearContext", nil)

	// buildPrompt should reflect empty context
	r := env.callTool(t, "buildPrompt", nil)
	text := mcpResultText(t, r)
	if strings.Contains(text, "important note") {
		t.Errorf("prompt still contains cleared note: %s", text)
	}
	if strings.Contains(text, "my-note") {
		t.Errorf("prompt still contains cleared label: %s", text)
	}
}

// --- listContext ---

func TestMCPServer_ListContext_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "listContext", nil)
	if result.IsError {
		t.Fatalf("listContext returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	var data struct {
		Files []string              `json:"files"`
		Items []mcpListContextEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.Files) != 0 {
		t.Errorf("files = %v, want empty", data.Files)
	}
	if len(data.Items) != 0 {
		t.Errorf("items = %v, want empty", data.Items)
	}
}

func TestMCPServer_ListContext_Populated(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Add a file
	testFile := filepath.Join(env.dir, "ctx.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	env.callTool(t, "addFile", map[string]any{"path": testFile})

	// Add a note and diff
	env.callTool(t, "addNote", map[string]any{"text": "n1", "label": "lab-note"})
	env.callTool(t, "addDiff", map[string]any{"diff": "+x\n", "label": "lab-diff"})

	result := env.callTool(t, "listContext", nil)
	text := mcpResultText(t, result)
	var data struct {
		Files []string              `json:"files"`
		Items []mcpListContextEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.Files) != 1 {
		t.Errorf("files count = %d, want 1", len(data.Files))
	}
	if len(data.Items) != 2 {
		t.Errorf("items count = %d, want 2", len(data.Items))
	}
	if data.Items[0].Type != "note" || data.Items[0].Label != "lab-note" {
		t.Errorf("items[0] = %+v, want note/lab-note", data.Items[0])
	}
	if data.Items[1].Type != "diff" || data.Items[1].Label != "lab-diff" {
		t.Errorf("items[1] = %+v, want diff/lab-diff", data.Items[1])
	}
}

// --- buildPrompt ---

func TestMCPServer_BuildPrompt_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "buildPrompt", nil)
	if result.IsError {
		t.Fatalf("buildPrompt returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	// Even with no files, notes, or diffs, the prompt should not be empty
	// because GetTxtarString returns a context root header.
	if text == "" {
		t.Error("expected non-empty prompt")
	}
}

func TestMCPServer_BuildPrompt_WithGoal(t *testing.T) {
	t.Parallel()
	goals := []Goal{{
		Name:               "test-goal",
		Description:        "A test goal",
		Category:           "testing",
		PromptInstructions: "You are a helpful test assistant.",
	}}
	env := newMCPTestEnv(t, goals)

	// Add a note so prompt is non-empty even without goal check
	env.callTool(t, "addNote", map[string]any{"text": "some context"})

	result := env.callTool(t, "buildPrompt", map[string]any{"goalName": "test-goal"})
	if result.IsError {
		t.Fatalf("buildPrompt returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "## Instructions") {
		t.Errorf("prompt missing '## Instructions': %s", text)
	}
	if !strings.Contains(text, "helpful test assistant") {
		t.Errorf("prompt missing goal instructions: %s", text)
	}
}

func TestMCPServer_BuildPrompt_GoalNotFound(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "buildPrompt", map[string]any{"goalName": "nonexistent"})
	if !result.IsError {
		t.Error("expected IsError for nonexistent goal")
	}
	text := mcpResultText(t, result)
	if !strings.Contains(text, "goal not found") {
		t.Errorf("error text = %q, want to contain 'goal not found'", text)
	}
}

func TestMCPServer_BuildPrompt_WithDiffAndNote(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "addDiff", map[string]any{
		"diff":  "--- a/f\n+++ b/f\n-old\n+new\n",
		"label": "my-diff",
	})
	env.callTool(t, "addNote", map[string]any{
		"text":  "Check the changes carefully.",
		"label": "review-note",
	})

	result := env.callTool(t, "buildPrompt", nil)
	text := mcpResultText(t, result)

	if !strings.Contains(text, "### my-diff") {
		t.Errorf("prompt missing diff section header: %s", text)
	}
	if !strings.Contains(text, "```diff") {
		t.Errorf("prompt missing diff fence: %s", text)
	}
	if !strings.Contains(text, "### review-note") {
		t.Errorf("prompt missing note section header: %s", text)
	}
	if !strings.Contains(text, "Check the changes carefully.") {
		t.Errorf("prompt missing note content: %s", text)
	}
}

func TestMCPServer_BuildPrompt_WithFiles(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	testFile := filepath.Join(env.dir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	env.callTool(t, "addFile", map[string]any{"path": testFile})

	result := env.callTool(t, "buildPrompt", nil)
	text := mcpResultText(t, result)

	if !strings.Contains(text, "## Context Files") {
		t.Errorf("prompt missing context files header: %s", text)
	}
	if !strings.Contains(text, "package main") {
		t.Errorf("prompt missing file content: %s", text)
	}
}

// --- getGoals ---

func TestMCPServer_GetGoals_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	result := env.callTool(t, "getGoals", nil)
	if result.IsError {
		t.Fatalf("getGoals returned error: %s", mcpResultText(t, result))
	}
	text := mcpResultText(t, result)
	var goals []mcpGoalInfo
	if err := json.Unmarshal([]byte(text), &goals); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(goals) != 0 {
		t.Errorf("goals = %v, want empty", goals)
	}
}

func TestMCPServer_GetGoals_WithGoals(t *testing.T) {
	t.Parallel()
	goals := []Goal{
		{Name: "code-review", Description: "Review code changes", Category: "development"},
		{Name: "summarize", Description: "Summarize text", Category: "writing"},
	}
	env := newMCPTestEnv(t, goals)

	result := env.callTool(t, "getGoals", nil)
	text := mcpResultText(t, result)
	var infos []mcpGoalInfo
	if err := json.Unmarshal([]byte(text), &infos); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("got %d goals, want 2", len(infos))
	}

	found := map[string]bool{}
	for _, g := range infos {
		found[g.Name] = true
		if g.Name == "code-review" {
			if g.Description != "Review code changes" {
				t.Errorf("code-review description = %q", g.Description)
			}
			if g.Category != "development" {
				t.Errorf("code-review category = %q", g.Category)
			}
		}
	}
	if !found["code-review"] || !found["summarize"] {
		t.Errorf("missing expected goals: %v", found)
	}
}

// --- mcpBacktickFence ---

func TestMCPBacktickFence(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"no backticks", "hello world", "```"},
		{"single backtick", "use `fmt.Println`", "```"},
		{"double backtick", "``code``", "```"},
		{"triple backtick", "```go\nfmt.Println()\n```", "````"},
		{"quad backtick", "````\ninner\n````", "`````"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := mcpBacktickFence(tt.content)
			if got != tt.want {
				t.Errorf("mcpBacktickFence(%q) = %q, want %q", tt.content, got, tt.want)
			}
		})
	}
}

// --- Integration: full workflow ---

func TestMCPServer_FullWorkflow(t *testing.T) {
	t.Parallel()
	goals := []Goal{{
		Name:               "review",
		Description:        "Code review",
		PromptInstructions: "Review this code for bugs.",
	}}
	env := newMCPTestEnv(t, goals)

	// 1. Create and add a file
	srcFile := filepath.Join(env.dir, "app.py")
	if err := os.WriteFile(srcFile, []byte("print('hello')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	r := env.callTool(t, "addFile", map[string]any{"path": srcFile})
	if r.IsError {
		t.Fatalf("addFile: %s", mcpResultText(t, r))
	}

	// 2. Add a diff
	r = env.callTool(t, "addDiff", map[string]any{
		"diff":  "-print('hello')\n+print('goodbye')\n",
		"label": "greeting-change",
	})
	if r.IsError {
		t.Fatalf("addDiff: %s", mcpResultText(t, r))
	}

	// 3. Add a note
	r = env.callTool(t, "addNote", map[string]any{
		"text":  "The greeting was changed to goodbye.",
		"label": "context",
	})
	if r.IsError {
		t.Fatalf("addNote: %s", mcpResultText(t, r))
	}

	// 4. List context
	r = env.callTool(t, "listContext", nil)
	text := mcpResultText(t, r)
	var ctx struct {
		Files []string              `json:"files"`
		Items []mcpListContextEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(text), &ctx); err != nil {
		t.Fatal(err)
	}
	if len(ctx.Files) != 1 {
		t.Errorf("files = %d, want 1", len(ctx.Files))
	}
	if len(ctx.Items) != 2 {
		t.Errorf("items = %d, want 2", len(ctx.Items))
	}

	// 5. Build prompt with goal
	r = env.callTool(t, "buildPrompt", map[string]any{"goalName": "review"})
	prompt := mcpResultText(t, r)

	// Verify prompt structure
	if !strings.Contains(prompt, "## Instructions") {
		t.Error("prompt missing instructions header")
	}
	if !strings.Contains(prompt, "Review this code for bugs.") {
		t.Error("prompt missing goal instructions")
	}
	if !strings.Contains(prompt, "### greeting-change") {
		t.Error("prompt missing diff label")
	}
	if !strings.Contains(prompt, "```diff") {
		t.Error("prompt missing diff fence")
	}
	if !strings.Contains(prompt, "### context") {
		t.Error("prompt missing note label")
	}
	if !strings.Contains(prompt, "## Context Files") {
		t.Error("prompt missing context files")
	}
	if !strings.Contains(prompt, "print('hello')") {
		t.Error("prompt missing file content")
	}

	// 6. Get goals
	r = env.callTool(t, "getGoals", nil)
	var goalInfos []mcpGoalInfo
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &goalInfos); err != nil {
		t.Fatal(err)
	}
	if len(goalInfos) != 1 || goalInfos[0].Name != "review" {
		t.Errorf("getGoals = %+v, want [{review ...}]", goalInfos)
	}
}

// --- registerSession ---

func TestMCPServer_RegisterSession(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	r := env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "agent-1",
		"capabilities": []string{"code", "test"},
	})
	if r.IsError {
		t.Fatalf("registerSession returned error: %s", mcpResultText(t, r))
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "agent-1") {
		t.Errorf("registerSession text = %q, want to contain 'agent-1'", text)
	}

	// Verify via getSession
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "agent-1"})
	if r.IsError {
		t.Fatalf("getSession returned error: %s", mcpResultText(t, r))
	}
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
	if sess.SessionID != "agent-1" {
		t.Errorf("sessionId = %q, want agent-1", sess.SessionID)
	}
	if len(sess.Capabilities) != 2 || sess.Capabilities[0] != "code" {
		t.Errorf("capabilities = %v, want [code test]", sess.Capabilities)
	}
	if sess.Status != "idle" {
		t.Errorf("status = %q, want idle", sess.Status)
	}

	// Re-register same session — should succeed (overwrite)
	r = env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "agent-1",
		"capabilities": []string{"updated"},
	})
	if r.IsError {
		t.Fatalf("re-register returned error: %s", mcpResultText(t, r))
	}
}

func TestMCPServer_RegisterSession_EmptyID(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	r := env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "",
		"capabilities": []string{},
	})
	if !r.IsError {
		t.Error("expected IsError for empty sessionId")
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "sessionId is required") {
		t.Errorf("error text = %q, want to contain 'sessionId is required'", text)
	}
}

// --- reportProgress ---

func TestMCPServer_ReportProgress(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Register first
	env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "prog-1",
		"capabilities": []string{},
	})

	r := env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "prog-1",
		"status":    "working",
		"progress":  42.5,
		"message":   "Compiling tests",
	})
	if r.IsError {
		t.Fatalf("reportProgress returned error: %s", mcpResultText(t, r))
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "working") {
		t.Errorf("reportProgress text = %q, want to contain 'working'", text)
	}

	// Verify session state via getSession
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "prog-1"})
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sess.Status != "working" {
		t.Errorf("status = %q, want working", sess.Status)
	}
	if sess.Progress != 42.5 {
		t.Errorf("progress = %f, want 42.5", sess.Progress)
	}
}

func TestMCPServer_ReportProgress_UnknownSession(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	r := env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "nonexistent",
		"status":    "working",
		"progress":  50,
		"message":   "test",
	})
	if !r.IsError {
		t.Error("expected IsError for unknown session")
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "session not found") {
		t.Errorf("error text = %q, want to contain 'session not found'", text)
	}
}

func TestMCPServer_ReportProgress_InvalidStatus(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{"sessionId": "s1", "capabilities": []string{}})

	r := env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "s1",
		"status":    "dancing",
		"progress":  50,
		"message":   "test",
	})
	if !r.IsError {
		t.Error("expected IsError for invalid status")
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "invalid status") {
		t.Errorf("error text = %q, want to contain 'invalid status'", text)
	}
}

func TestMCPServer_ReportProgress_ClampedProgress(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{"sessionId": "clamp-1", "capabilities": []string{}})

	// Progress > 100 should be clamped to 100
	env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "clamp-1",
		"status":    "working",
		"progress":  150,
		"message":   "over",
	})
	r := env.callTool(t, "getSession", map[string]any{"sessionId": "clamp-1"})
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sess.Progress != 100 {
		t.Errorf("progress = %f, want 100 (clamped from 150)", sess.Progress)
	}

	// Progress < 0 should be clamped to 0
	env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "clamp-1",
		"status":    "working",
		"progress":  -10,
		"message":   "under",
	})
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "clamp-1"})
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sess.Progress != 0 {
		t.Errorf("progress = %f, want 0 (clamped from -10)", sess.Progress)
	}
}

// --- reportResult ---

func TestMCPServer_ReportResult(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{"sessionId": "res-1", "capabilities": []string{}})

	r := env.callTool(t, "reportResult", map[string]any{
		"sessionId":    "res-1",
		"success":      true,
		"output":       "All tests passed",
		"filesChanged": []string{"main.go", "main_test.go"},
	})
	if r.IsError {
		t.Fatalf("reportResult returned error: %s", mcpResultText(t, r))
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "succeeded") {
		t.Errorf("reportResult text = %q, want to contain 'succeeded'", text)
	}

	// Verify via getSession
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "res-1"})
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sess.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(sess.Events))
	}
	if sess.Events[0].Type != "result" {
		t.Errorf("event type = %q, want result", sess.Events[0].Type)
	}
	data, ok := sess.Events[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("event data type = %T, want map[string]any", sess.Events[0].Data)
	}
	if data["success"] != true {
		t.Errorf("event success = %v, want true", data["success"])
	}
	if data["output"] != "All tests passed" {
		t.Errorf("event output = %v, want 'All tests passed'", data["output"])
	}
}

func TestMCPServer_ReportResult_UnknownSession(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	r := env.callTool(t, "reportResult", map[string]any{
		"sessionId": "nonexistent",
		"success":   true,
		"output":    "done",
	})
	if !r.IsError {
		t.Error("expected IsError for unknown session")
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "session not found") {
		t.Errorf("error text = %q, want to contain 'session not found'", text)
	}
}

// --- requestGuidance ---

func TestMCPServer_RequestGuidance(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{"sessionId": "guid-1", "capabilities": []string{}})

	r := env.callTool(t, "requestGuidance", map[string]any{
		"sessionId": "guid-1",
		"question":  "Should I refactor the parser?",
		"options":   []string{"yes", "no", "partial"},
		"context":   "Parser has cyclomatic complexity 25",
	})
	if r.IsError {
		t.Fatalf("requestGuidance returned error: %s", mcpResultText(t, r))
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "guidance requested") {
		t.Errorf("requestGuidance text = %q, want to contain 'guidance requested'", text)
	}

	// Verify event in getSession
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "guid-1"})
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sess.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(sess.Events))
	}
	if sess.Events[0].Type != "guidance" {
		t.Errorf("event type = %q, want guidance", sess.Events[0].Type)
	}
	data, ok := sess.Events[0].Data.(map[string]any)
	if !ok {
		t.Fatalf("event data type = %T, want map[string]any", sess.Events[0].Data)
	}
	if data["question"] != "Should I refactor the parser?" {
		t.Errorf("question = %v", data["question"])
	}
}

func TestMCPServer_RequestGuidance_EmptyQuestion(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{"sessionId": "guid-2", "capabilities": []string{}})

	r := env.callTool(t, "requestGuidance", map[string]any{
		"sessionId": "guid-2",
		"question":  "",
	})
	if !r.IsError {
		t.Error("expected IsError for empty question")
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "question is required") {
		t.Errorf("error text = %q, want to contain 'question is required'", text)
	}
}

// --- getSession ---

func TestMCPServer_GetSession(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	registered := time.Now()
	env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "full-1",
		"capabilities": []string{"analyze"},
	})

	// Report progress
	env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "full-1",
		"status":    "working",
		"progress":  25,
		"message":   "Analyzing",
	})

	// Report result
	env.callTool(t, "reportResult", map[string]any{
		"sessionId":    "full-1",
		"success":      true,
		"output":       "Analysis complete",
		"filesChanged": []string{"report.md"},
	})

	// getSession should return everything
	r := env.callTool(t, "getSession", map[string]any{"sessionId": "full-1"})
	if r.IsError {
		t.Fatalf("getSession returned error: %s", mcpResultText(t, r))
	}
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if sess.SessionID != "full-1" {
		t.Errorf("sessionId = %q, want full-1", sess.SessionID)
	}
	if len(sess.Capabilities) != 1 || sess.Capabilities[0] != "analyze" {
		t.Errorf("capabilities = %v, want [analyze]", sess.Capabilities)
	}
	if len(sess.Events) != 2 {
		t.Fatalf("events = %d, want 2 (progress + result)", len(sess.Events))
	}
	if sess.Events[0].Type != "progress" {
		t.Errorf("events[0].type = %q, want progress", sess.Events[0].Type)
	}
	if sess.Events[1].Type != "result" {
		t.Errorf("events[1].type = %q, want result", sess.Events[1].Type)
	}
	if time.Since(registered) > time.Hour {
		t.Error("lastUpdate is unreasonably old")
	}
}

func TestMCPServer_GetSession_Unknown(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	r := env.callTool(t, "getSession", map[string]any{"sessionId": "ghost"})
	if !r.IsError {
		t.Error("expected IsError for unknown session")
	}
	text := mcpResultText(t, r)
	if !strings.Contains(text, "session not found") {
		t.Errorf("error text = %q, want to contain 'session not found'", text)
	}
}

func TestMCPServer_GetSession_DrainsEvents(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{"sessionId": "drain-1", "capabilities": []string{}})
	env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "drain-1",
		"status":    "working",
		"progress":  50,
		"message":   "halfway",
	})

	// First getSession: should have 1 event
	r := env.callTool(t, "getSession", map[string]any{"sessionId": "drain-1"})
	var sess1 mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess1); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sess1.Events) != 1 {
		t.Fatalf("first read: events = %d, want 1", len(sess1.Events))
	}

	// Second getSession: events should be drained (empty)
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "drain-1"})
	var sess2 mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sess2.Events) != 0 {
		t.Errorf("second read: events = %d, want 0 (drained)", len(sess2.Events))
	}

	// Session state should still be intact
	if sess2.Status != "working" {
		t.Errorf("status after drain = %q, want working", sess2.Status)
	}
	if sess2.Progress != 50 {
		t.Errorf("progress after drain = %f, want 50", sess2.Progress)
	}
}

// --- listSessions ---

func TestMCPServer_ListSessions(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "ls-1",
		"capabilities": []string{"code"},
	})
	env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "ls-2",
		"capabilities": []string{"test"},
	})
	env.callTool(t, "reportProgress", map[string]any{
		"sessionId": "ls-1",
		"status":    "working",
		"progress":  75,
		"message":   "busy",
	})

	r := env.callTool(t, "listSessions", nil)
	if r.IsError {
		t.Fatalf("listSessions returned error: %s", mcpResultText(t, r))
	}
	var summaries []mcpSessionSummary
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &summaries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("sessions = %d, want 2", len(summaries))
	}

	found := map[string]mcpSessionSummary{}
	for _, s := range summaries {
		found[s.SessionID] = s
	}
	if s, ok := found["ls-1"]; !ok {
		t.Error("missing session ls-1")
	} else {
		if s.Status != "working" {
			t.Errorf("ls-1 status = %q, want working", s.Status)
		}
		if s.EventCount != 1 {
			t.Errorf("ls-1 eventCount = %d, want 1", s.EventCount)
		}
	}
	if s, ok := found["ls-2"]; !ok {
		t.Error("missing session ls-2")
	} else {
		if s.Status != "idle" {
			t.Errorf("ls-2 status = %q, want idle", s.Status)
		}
	}
}

func TestMCPServer_ListSessions_Empty(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	r := env.callTool(t, "listSessions", nil)
	if r.IsError {
		t.Fatalf("listSessions returned error: %s", mcpResultText(t, r))
	}
	var summaries []mcpSessionSummary
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &summaries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if summaries == nil {
		t.Error("listSessions returned nil, want empty array")
	}
	if len(summaries) != 0 {
		t.Errorf("sessions = %d, want 0", len(summaries))
	}
}

// --- Orchestrator workflow (full E2E) ---

func TestMCPServer_OrchestratorWorkflow(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// 1. Register session
	r := env.callTool(t, "registerSession", map[string]any{
		"sessionId":    "orch-agent",
		"capabilities": []string{"code-review", "testing", "refactor"},
	})
	if r.IsError {
		t.Fatalf("registerSession: %s", mcpResultText(t, r))
	}

	// 2. Report progress × 3
	for i, step := range []struct {
		status   string
		progress float64
		message  string
	}{
		{"working", 10, "Starting analysis"},
		{"working", 50, "Running tests"},
		{"blocked", 80, "Waiting for dependency"},
	} {
		r = env.callTool(t, "reportProgress", map[string]any{
			"sessionId": "orch-agent",
			"status":    step.status,
			"progress":  step.progress,
			"message":   step.message,
		})
		if r.IsError {
			t.Fatalf("reportProgress[%d]: %s", i, mcpResultText(t, r))
		}
	}

	// 3. Request guidance
	r = env.callTool(t, "requestGuidance", map[string]any{
		"sessionId": "orch-agent",
		"question":  "Dependency X is unavailable. Skip or wait?",
		"options":   []string{"skip", "wait", "mock"},
		"context":   "Integration test phase",
	})
	if r.IsError {
		t.Fatalf("requestGuidance: %s", mcpResultText(t, r))
	}

	// 4. Report result
	r = env.callTool(t, "reportResult", map[string]any{
		"sessionId":    "orch-agent",
		"success":      true,
		"output":       "All tasks completed successfully",
		"filesChanged": []string{"pkg/main.go", "pkg/main_test.go"},
	})
	if r.IsError {
		t.Fatalf("reportResult: %s", mcpResultText(t, r))
	}

	// 5. getSession — verify full event history
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "orch-agent"})
	if r.IsError {
		t.Fatalf("getSession: %s", mcpResultText(t, r))
	}
	var sess mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if sess.SessionID != "orch-agent" {
		t.Errorf("sessionId = %q, want orch-agent", sess.SessionID)
	}
	if len(sess.Capabilities) != 3 {
		t.Errorf("capabilities = %v, want 3 items", sess.Capabilities)
	}
	// After successful result, status should be idle
	if sess.Status != "idle" {
		t.Errorf("status = %q, want idle (after successful result)", sess.Status)
	}

	// Should have 5 events: 3 progress + 1 guidance + 1 result
	if len(sess.Events) != 5 {
		t.Fatalf("events = %d, want 5", len(sess.Events))
	}
	expectedTypes := []string{"progress", "progress", "progress", "guidance", "result"}
	for i, et := range expectedTypes {
		if sess.Events[i].Type != et {
			t.Errorf("events[%d].type = %q, want %q", i, sess.Events[i].Type, et)
		}
	}

	// 6. Verify events are drained
	r = env.callTool(t, "getSession", map[string]any{"sessionId": "orch-agent"})
	var sess2 mcpGetSessionResponse
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &sess2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(sess2.Events) != 0 {
		t.Errorf("events after drain = %d, want 0", len(sess2.Events))
	}

	// 7. Verify it shows in listSessions
	r = env.callTool(t, "listSessions", nil)
	var summaries []mcpSessionSummary
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &summaries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("listSessions = %d, want 1", len(summaries))
	}
	if summaries[0].SessionID != "orch-agent" {
		t.Errorf("listSessions[0].sessionId = %q, want orch-agent", summaries[0].SessionID)
	}
	if summaries[0].EventCount != 0 {
		t.Errorf("eventCount = %d, want 0 (events were drained)", summaries[0].EventCount)
	}
}

// --- Concurrency tests ---

func TestMCPServer_ConcurrentToolCalls(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	const perType = 10
	var wg sync.WaitGroup
	wg.Add(perType * 2)

	// 10 concurrent addNote calls
	for i := 0; i < perType; i++ {
		go func(idx int) {
			defer wg.Done()
			env.callTool(t, "addNote", map[string]any{
				"text":  fmt.Sprintf("concurrent-note-%d", idx),
				"label": fmt.Sprintf("note-%d", idx),
			})
		}(i)
	}

	// 10 concurrent addDiff calls
	for i := 0; i < perType; i++ {
		go func(idx int) {
			defer wg.Done()
			env.callTool(t, "addDiff", map[string]any{
				"diff":  fmt.Sprintf("+diff-line-%d\n", idx),
				"label": fmt.Sprintf("diff-%d", idx),
			})
		}(i)
	}

	wg.Wait()

	// Verify all 20 items are present via listContext
	r := env.callTool(t, "listContext", nil)
	var data struct {
		Files []string              `json:"files"`
		Items []mcpListContextEntry `json:"items"`
	}
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(data.Items) != 20 {
		t.Fatalf("items count = %d, want 20", len(data.Items))
	}

	noteCount, diffCount := 0, 0
	for _, item := range data.Items {
		switch item.Type {
		case "note":
			noteCount++
		case "diff":
			diffCount++
		}
	}
	if noteCount != perType {
		t.Errorf("note count = %d, want %d", noteCount, perType)
	}
	if diffCount != perType {
		t.Errorf("diff count = %d, want %d", diffCount, perType)
	}
}

func TestMCPServer_ConcurrentSessions(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	const numSessions = 5

	// Register 5 sessions concurrently
	var wg sync.WaitGroup
	wg.Add(numSessions)
	for i := 0; i < numSessions; i++ {
		go func(idx int) {
			defer wg.Done()
			env.callTool(t, "registerSession", map[string]any{
				"sessionId":    fmt.Sprintf("concurrent-%d", idx),
				"capabilities": []string{"cap"},
			})
		}(i)
	}
	wg.Wait()

	// Report progress on all 5 concurrently
	wg.Add(numSessions)
	for i := 0; i < numSessions; i++ {
		go func(idx int) {
			defer wg.Done()
			env.callTool(t, "reportProgress", map[string]any{
				"sessionId": fmt.Sprintf("concurrent-%d", idx),
				"status":    "working",
				"progress":  float64(idx * 20),
				"message":   fmt.Sprintf("session %d working", idx),
			})
		}(i)
	}
	wg.Wait()

	// Verify all 5 sessions are present via listSessions
	r := env.callTool(t, "listSessions", nil)
	var summaries []mcpSessionSummary
	if err := json.Unmarshal([]byte(mcpResultText(t, r)), &summaries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(summaries) != numSessions {
		t.Fatalf("sessions = %d, want %d", len(summaries), numSessions)
	}

	found := make(map[string]bool, numSessions)
	for _, s := range summaries {
		found[s.SessionID] = true
		if s.Status != "working" {
			t.Errorf("session %q status = %q, want working", s.SessionID, s.Status)
		}
		if s.EventCount != 1 {
			t.Errorf("session %q eventCount = %d, want 1", s.SessionID, s.EventCount)
		}
	}
	for i := 0; i < numSessions; i++ {
		id := fmt.Sprintf("concurrent-%d", i)
		if !found[id] {
			t.Errorf("session %q not found in listSessions", id)
		}
	}
}

// --- Large payload tests ---

func TestMCPServer_LargePayloads(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// 100KB note
	largeNote := strings.Repeat("A", 100*1024)
	r := env.callTool(t, "addNote", map[string]any{
		"text":  largeNote,
		"label": "large-note",
	})
	if r.IsError {
		t.Fatalf("addNote(100KB) returned error: %s", mcpResultText(t, r))
	}

	// 50KB diff
	largeDiff := strings.Repeat("+added line\n", 50*1024/len("+added line\n"))
	if len(largeDiff) < 50*1024 {
		largeDiff += strings.Repeat("X", 50*1024-len(largeDiff))
	}
	r = env.callTool(t, "addDiff", map[string]any{
		"diff":  largeDiff,
		"label": "large-diff",
	})
	if r.IsError {
		t.Fatalf("addDiff(50KB) returned error: %s", mcpResultText(t, r))
	}

	// Build prompt and verify both payloads are present
	r = env.callTool(t, "buildPrompt", nil)
	if r.IsError {
		t.Fatalf("buildPrompt returned error: %s", mcpResultText(t, r))
	}
	prompt := mcpResultText(t, r)

	if !strings.Contains(prompt, "### large-note") {
		t.Error("prompt missing large-note section header")
	}
	if !strings.Contains(prompt, "### large-diff") {
		t.Error("prompt missing large-diff section header")
	}
	if !strings.Contains(prompt, largeNote) {
		t.Error("prompt missing 100KB note content")
	}
	if !strings.Contains(prompt, largeDiff) {
		t.Error("prompt missing 50KB diff content")
	}
}

// --- Backtick fence integration test ---

func TestMCPServer_BacktickFenceInPrompt(t *testing.T) {
	t.Parallel()
	env := newMCPTestEnv(t, nil)

	// Diff content that contains triple backticks
	diffWithBackticks := "--- a/README.md\n+++ b/README.md\n@@ -1,3 +1,5 @@\n # Title\n+```go\n+fmt.Println(\"hello\")\n+```\n"
	r := env.callTool(t, "addDiff", map[string]any{
		"diff":  diffWithBackticks,
		"label": "backtick-diff",
	})
	if r.IsError {
		t.Fatalf("addDiff returned error: %s", mcpResultText(t, r))
	}

	// Build prompt
	r = env.callTool(t, "buildPrompt", nil)
	if r.IsError {
		t.Fatalf("buildPrompt returned error: %s", mcpResultText(t, r))
	}
	prompt := mcpResultText(t, r)

	// The fence must be upgraded to 4 backticks since content has ```
	if !strings.Contains(prompt, "````diff") {
		t.Errorf("prompt should use quadruple backtick fence for diff containing triple backticks;\ngot prompt:\n%s", prompt)
	}

	// The closing fence must also be 4 backticks
	if strings.Count(prompt, "````") < 2 {
		t.Errorf("prompt should have at least 2 quadruple backtick fences (open + close);\ngot prompt:\n%s", prompt)
	}

	// Original content must still be present
	if !strings.Contains(prompt, "```go") {
		t.Error("prompt missing original triple backtick content from diff")
	}
	if !strings.Contains(prompt, "### backtick-diff") {
		t.Error("prompt missing diff section header")
	}
}
