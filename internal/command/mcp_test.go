package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
		"addFile":      false,
		"addDiff":      false,
		"addNote":      false,
		"removeFile":   false,
		"listContext":  false,
		"clearContext": false,
		"buildPrompt":  false,
		"getGoals":     false,
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
	if len(result.Tools) != 8 {
		t.Errorf("got %d tools, want 8", len(result.Tools))
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
