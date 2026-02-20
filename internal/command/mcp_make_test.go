package command

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMCPMakeCommand(t *testing.T) {
	t.Parallel()
	cmd := NewMCPMakeCommand()
	assert.Equal(t, "mcp-make", cmd.Name())
	assert.Contains(t, cmd.Description(), "Make")
	assert.Contains(t, cmd.Usage(), "mcp-make")
}

func TestMCPMakeCommand_SetupFlags(t *testing.T) {
	t.Parallel()
	cmd := NewMCPMakeCommand()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	err := fs.Parse([]string{"-workdir", "/tmp/project", "-file", "/tmp/Makefile"})
	require.NoError(t, err)
	assert.Equal(t, "/tmp/project", cmd.workdir)
	assert.Equal(t, "/tmp/Makefile", cmd.file)
}

func TestMCPMakeCommand_Execute_UnexpectedArgs(t *testing.T) {
	t.Parallel()
	cmd := NewMCPMakeCommand()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"extra"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

func TestMCPMakeCommand_DetectMakeBinary(t *testing.T) {
	t.Parallel()

	t.Run("override", func(t *testing.T) {
		t.Parallel()
		cmd := NewMCPMakeCommand()
		cmd.makeBinaryOverride = "/usr/bin/fake-make"
		bin, err := cmd.detectMakeBinary()
		assert.NoError(t, err)
		assert.Equal(t, "/usr/bin/fake-make", bin)
	})

	t.Run("platform-default", func(t *testing.T) {
		t.Parallel()
		cmd := NewMCPMakeCommand()
		bin, err := cmd.detectMakeBinary()
		// On CI/dev machines make or gmake should exist.
		if err != nil {
			t.Skipf("no make binary found: %v", err)
		}
		assert.NotEmpty(t, bin)
		if runtime.GOOS == "darwin" {
			// On macOS, should prefer gmake if available.
			assert.True(t, strings.Contains(bin, "make"), "binary should contain 'make': %s", bin)
		}
	})
}

func TestMCPMakeCommand_ResolveWorkdir(t *testing.T) {
	t.Parallel()

	t.Run("explicit", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cmd := NewMCPMakeCommand()
		resolved, err := cmd.resolveWorkdir(dir)
		assert.NoError(t, err)
		assert.Equal(t, dir, resolved)
	})

	t.Run("command-default", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		cmd := NewMCPMakeCommand()
		cmd.workdir = dir
		resolved, err := cmd.resolveWorkdir("")
		assert.NoError(t, err)
		assert.Equal(t, dir, resolved)
	})

	t.Run("cwd-fallback", func(t *testing.T) {
		t.Parallel()
		cmd := NewMCPMakeCommand()
		resolved, err := cmd.resolveWorkdir("")
		assert.NoError(t, err)
		assert.NotEmpty(t, resolved)
	})

	t.Run("nonexistent", func(t *testing.T) {
		t.Parallel()
		cmd := NewMCPMakeCommand()
		_, err := cmd.resolveWorkdir("/nonexistent/dir/12345")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "working directory")
	})

	t.Run("not-a-directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		f := filepath.Join(dir, "file.txt")
		require.NoError(t, os.WriteFile(f, []byte("x"), 0600))
		cmd := NewMCPMakeCommand()
		_, err := cmd.resolveWorkdir(f)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not a directory")
	})
}

func TestMCPMakeCommand_RunMake_SimpleMakefile(t *testing.T) {
	t.Parallel()

	// Create a temporary Makefile.
	dir := t.TempDir()
	makefile := filepath.Join(dir, "Makefile")
	content := `.PHONY: hello help
hello:
	@echo "Hello from make"

help: ## Show help
	@echo "Available targets: hello, help"
`
	require.NoError(t, os.WriteFile(makefile, []byte(content), 0600))

	cmd := NewMCPMakeCommand()
	makeBin, err := cmd.detectMakeBinary()
	if err != nil {
		t.Skipf("no make binary found: %v", err)
	}

	t.Run("run-target", func(t *testing.T) {
		t.Parallel()
		output, err := cmd.runMake(t.Context(), makeBin, dir, []string{"hello"})
		assert.NoError(t, err)
		assert.Contains(t, output, "Hello from make")
	})

	t.Run("run-help", func(t *testing.T) {
		t.Parallel()
		output, err := cmd.runMake(t.Context(), makeBin, dir, []string{"help"})
		assert.NoError(t, err)
		assert.Contains(t, output, "Available targets")
	})

	t.Run("nonexistent-target", func(t *testing.T) {
		t.Parallel()
		_, err := cmd.runMake(t.Context(), makeBin, dir, []string{"nonexistent"})
		assert.Error(t, err)
	})
}

func TestMCPMakeCommand_NewMakeServer_ToolRegistration(t *testing.T) {
	t.Parallel()

	cmd := NewMCPMakeCommand()
	makeBin, err := cmd.detectMakeBinary()
	if err != nil {
		t.Skipf("no make binary found: %v", err)
	}

	server := cmd.newMakeServer(makeBin)
	assert.NotNil(t, server)
	// Server should have at least the make and make_help tools registered.
	// We can't easily list tools from the server directly, but verifying
	// it was created without error is a good baseline.
}

func TestMCPMakeCommand_RunMake_WithMakefileFlag(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a Makefile in a non-standard location.
	customMakefile := filepath.Join(dir, "custom.mk")
	content := `.PHONY: custom-target
custom-target:
	@echo "Custom Makefile target"
`
	require.NoError(t, os.WriteFile(customMakefile, []byte(content), 0600))

	cmd := NewMCPMakeCommand()
	makeBin, err := cmd.detectMakeBinary()
	if err != nil {
		t.Skipf("no make binary found: %v", err)
	}

	output, err := cmd.runMake(t.Context(), makeBin, dir, []string{"-f", customMakefile, "custom-target"})
	assert.NoError(t, err)
	assert.Contains(t, output, "Custom Makefile target")
}

// --- MCP transport-level integration tests ---

// newMakeMCPTestEnv creates a make MCP server+client pair via
// InMemoryTransport for integration testing.
func newMakeMCPTestEnv(t *testing.T, workdir string) *mcpMakeTestEnv {
	t.Helper()

	cmd := NewMCPMakeCommand()
	cmd.workdir = workdir

	makeBin, err := cmd.detectMakeBinary()
	if err != nil {
		t.Skipf("no make binary found: %v", err)
	}

	server := cmd.newMakeServer(makeBin)

	ctx, cancel := context.WithCancel(context.Background())
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- server.Run(ctx, serverTransport)
	}()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-make-client", Version: "test"}, nil)
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

	return &mcpMakeTestEnv{session: session, cancel: cancel}
}

type mcpMakeTestEnv struct {
	session *mcp.ClientSession
	cancel  context.CancelFunc
}

func (e *mcpMakeTestEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

func mcpMakeResultText(t *testing.T, result *mcp.CallToolResult) string {
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

func TestMCPMake_Integration_MakeTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makefile := filepath.Join(dir, "Makefile")
	content := `.PHONY: greet
greet:
	@echo "Hello MCP"
`
	require.NoError(t, os.WriteFile(makefile, []byte(content), 0600))

	env := newMakeMCPTestEnv(t, dir)
	result := env.callTool(t, "make", map[string]any{
		"target": "greet",
	})

	text := mcpMakeResultText(t, result)
	assert.Contains(t, text, "Hello MCP")
	assert.False(t, result.IsError, "make greet should succeed")
}

func TestMCPMake_Integration_MakeTarget_NoTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makefile := filepath.Join(dir, "Makefile")
	require.NoError(t, os.WriteFile(makefile, []byte(".PHONY: all\nall:\n\t@echo ok\n"), 0600))

	env := newMakeMCPTestEnv(t, dir)
	result := env.callTool(t, "make", map[string]any{
		"target": "",
	})

	assert.True(t, result.IsError, "empty target should error")
}

func TestMCPMake_Integration_MakeHelp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makefile := filepath.Join(dir, "Makefile")
	content := `.PHONY: help
help: ## Show available targets
	@echo "Available: build, test, help"
`
	require.NoError(t, os.WriteFile(makefile, []byte(content), 0600))

	env := newMakeMCPTestEnv(t, dir)
	result := env.callTool(t, "make_help", map[string]any{})

	text := mcpMakeResultText(t, result)
	assert.Contains(t, text, "Available:")
	assert.False(t, result.IsError, "make help should succeed")
}

func TestMCPMake_Integration_MakeTarget_Failure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	makefile := filepath.Join(dir, "Makefile")
	content := `.PHONY: fail
fail:
	@exit 1
`
	require.NoError(t, os.WriteFile(makefile, []byte(content), 0600))

	env := newMakeMCPTestEnv(t, dir)
	result := env.callTool(t, "make", map[string]any{
		"target": "fail",
	})

	assert.True(t, result.IsError, "failing target should propagate error")
}

func TestMCPMake_Integration_ToolList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Makefile"), []byte(".PHONY: x\nx:\n\t@true\n"), 0600))

	env := newMakeMCPTestEnv(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tools, err := env.session.ListTools(ctx, nil)
	require.NoError(t, err)

	toolNames := make([]string, len(tools.Tools))
	for i, tool := range tools.Tools {
		toolNames[i] = tool.Name
	}
	assert.Contains(t, toolNames, "make")
	assert.Contains(t, toolNames, "make_help")
	assert.Len(t, toolNames, 2, "should have exactly 2 tools")
}
