package command

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/builtin/claudemux"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock client for MCP parent tests ---

type mockParentClient struct {
	enqueueFn   func(task string) (int, error)
	interruptFn func() error
	statusFn    func() (*claudemux.GetStatusResult, error)
}

func (m *mockParentClient) EnqueueTask(task string) (int, error) {
	if m.enqueueFn != nil {
		return m.enqueueFn(task)
	}
	return 0, nil
}

func (m *mockParentClient) InterruptCurrent() error {
	if m.interruptFn != nil {
		return m.interruptFn()
	}
	return nil
}

func (m *mockParentClient) GetStatus() (*claudemux.GetStatusResult, error) {
	if m.statusFn != nil {
		return m.statusFn()
	}
	return &claudemux.GetStatusResult{}, nil
}

// --- Unit tests ---

func TestNewMCPParentCommand(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()
	assert.Equal(t, "mcp-parent", cmd.Name())
	assert.Contains(t, cmd.Description(), "agent steering")
}

func TestMCPParent_SetupFlags(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	require.NoError(t, fs.Parse([]string{"--socket", "/tmp/test.sock"}))
	assert.Equal(t, "/tmp/test.sock", cmd.socketPath)
}

func TestMCPParent_Execute_NoSocket(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--socket flag is required")
}

func TestMCPParent_Execute_UnexpectedArgs(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()
	cmd.socketPath = "/tmp/test.sock"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"extra"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

func TestMCPParent_ResolveClient_Default(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()
	cmd.socketPath = "/tmp/somewhere.sock"

	client := cmd.resolveClient()
	assert.NotNil(t, client)
	// Default client is *claudemux.ControlClient.
	_, ok := client.(*claudemux.ControlClient)
	assert.True(t, ok)
}

func TestMCPParent_ResolveClient_Override(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()
	mock := &mockParentClient{}
	cmd.clientOverride = mock

	client := cmd.resolveClient()
	assert.Equal(t, mock, client)
}

// --- MCP server factory tests ---

func TestMCPParent_NewServer_ToolRegistration(t *testing.T) {
	t.Parallel()
	cmd := NewMCPParentCommand()
	mock := &mockParentClient{}
	server := cmd.newParentServer(mock)
	assert.NotNil(t, server)
}

// --- MCP transport-level integration tests ---

type mcpParentTestEnv struct {
	t       *testing.T
	session *mcp.ClientSession
	mock    *mockParentClient
}

func newMCPParentTestEnv(t *testing.T) *mcpParentTestEnv {
	t.Helper()
	mock := &mockParentClient{}
	cmd := NewMCPParentCommand()
	server := cmd.newParentServer(mock)

	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-parent-client", Version: "test"}, nil)
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

	return &mcpParentTestEnv{t: t, session: session, mock: mock}
}

func (e *mcpParentTestEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

func mcpParentResultText(t *testing.T, result *mcp.CallToolResult) string {
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

func TestMCPParent_Integration_ToolList(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := env.session.ListTools(ctx, nil)
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, tool := range result.Tools {
		names[tool.Name] = true
	}
	assert.True(t, names["enqueue_task"])
	assert.True(t, names["interrupt_current"])
	assert.True(t, names["get_status"])
	assert.Equal(t, 3, len(result.Tools))
}

func TestMCPParent_Integration_EnqueueTask(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)
	env.mock.enqueueFn = func(task string) (int, error) {
		if task == "build feature X" {
			return 2, nil
		}
		return 0, fmt.Errorf("unexpected task: %s", task)
	}

	result := env.callTool(t, "enqueue_task", map[string]any{
		"task": "build feature X",
	})
	assert.False(t, result.IsError)
	text := mcpParentResultText(t, result)
	assert.Contains(t, text, "position 2")
}

func TestMCPParent_Integration_EnqueueTask_EmptyTask(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)

	result := env.callTool(t, "enqueue_task", map[string]any{
		"task": "",
	})
	assert.True(t, result.IsError)
}

func TestMCPParent_Integration_EnqueueTask_Error(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)
	env.mock.enqueueFn = func(task string) (int, error) {
		return 0, fmt.Errorf("queue full")
	}

	result := env.callTool(t, "enqueue_task", map[string]any{
		"task": "overflow",
	})
	assert.True(t, result.IsError)
}

func TestMCPParent_Integration_InterruptCurrent(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)

	result := env.callTool(t, "interrupt_current", map[string]any{})
	assert.False(t, result.IsError)
	text := mcpParentResultText(t, result)
	assert.Contains(t, text, "Interrupt sent")
}

func TestMCPParent_Integration_InterruptCurrent_Error(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)
	env.mock.interruptFn = func() error {
		return fmt.Errorf("no active task")
	}

	result := env.callTool(t, "interrupt_current", map[string]any{})
	assert.True(t, result.IsError)
}

func TestMCPParent_Integration_GetStatus(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)
	env.mock.statusFn = func() (*claudemux.GetStatusResult, error) {
		return &claudemux.GetStatusResult{
			ActiveTask: "building widgets",
			QueueDepth: 2,
			Queue:      []string{"deploy", "test"},
		}, nil
	}

	result := env.callTool(t, "get_status", map[string]any{})
	assert.False(t, result.IsError)
	text := mcpParentResultText(t, result)
	assert.Contains(t, text, "building widgets")
	assert.Contains(t, text, "Queue depth: 2")
	assert.Contains(t, text, "deploy")
	assert.Contains(t, text, "test")
}

func TestMCPParent_Integration_GetStatus_Error(t *testing.T) {
	t.Parallel()
	env := newMCPParentTestEnv(t)
	env.mock.statusFn = func() (*claudemux.GetStatusResult, error) {
		return nil, fmt.Errorf("connection refused")
	}

	result := env.callTool(t, "get_status", map[string]any{})
	assert.True(t, result.IsError)
}
