package claudemux

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"testing"
)

// --- Mock Provider for testing ---

type mockProvider struct {
	name    string
	caps    ProviderCapabilities
	spawnFn func(ctx context.Context, opts SpawnOpts) (AgentHandle, error)
}

func (m *mockProvider) Name() string                       { return m.name }
func (m *mockProvider) Capabilities() ProviderCapabilities { return m.caps }
func (m *mockProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	if m.spawnFn != nil {
		return m.spawnFn(ctx, opts)
	}
	return &mockHandle{alive: true}, nil
}

type mockHandle struct {
	alive  bool
	input  []string
	output string
	closed bool
}

func (h *mockHandle) Send(input string) error {
	if h.closed {
		return errors.New("closed")
	}
	h.input = append(h.input, input)
	return nil
}

func (h *mockHandle) Receive() (string, error) {
	if h.closed {
		return "", io.EOF
	}
	if h.output != "" {
		out := h.output
		h.output = ""
		return out, nil
	}
	return "", io.EOF
}

func (h *mockHandle) Close() error {
	h.closed = true
	h.alive = false
	return nil
}

func (h *mockHandle) IsAlive() bool { return h.alive }
func (h *mockHandle) Wait() (int, error) {
	h.alive = false
	return 0, nil
}
func (h *mockHandle) Resize(_, _ int) error { return nil }

// --- Registry Tests ---

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if names := r.List(); len(names) != 0 {
		t.Errorf("new registry should be empty, got %v", names)
	}
}

func TestRegistry_Register(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	p := &mockProvider{name: "test-provider"}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Duplicate should fail
	err := r.Register(p)
	if !errors.Is(err, ErrProviderExists) {
		t.Errorf("expected ErrProviderExists, got %v", err)
	}
}

func TestRegistry_Get(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	p := &mockProvider{name: "test"}
	_ = r.Register(p)

	got, err := r.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "test" {
		t.Errorf("Name() = %q, want %q", got.Name(), "test")
	}

	_, err = r.Get("nonexistent")
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(&mockProvider{name: "beta"})
	_ = r.Register(&mockProvider{name: "alpha"})
	names := r.List()
	if len(names) != 2 {
		t.Fatalf("List() = %v, want 2 entries", names)
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("List() = %v, want [alpha beta]", names)
	}
}

func TestRegistry_Spawn(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(&mockProvider{name: "mock"})

	handle, err := r.Spawn(context.Background(), "mock", SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()
	if !handle.IsAlive() {
		t.Error("handle should be alive")
	}

	// Spawn with unknown provider
	_, err = r.Spawn(context.Background(), "unknown", SpawnOpts{})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("expected ErrProviderNotFound, got %v", err)
	}
}

func TestMockHandle_SendReceive(t *testing.T) {
	t.Parallel()
	h := &mockHandle{alive: true, output: "hello world\n"}

	if err := h.Send("test input"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(h.input) != 1 || h.input[0] != "test input" {
		t.Errorf("input = %v", h.input)
	}

	out, err := h.Receive()
	if err != nil {
		t.Fatalf("Receive: %v", err)
	}
	if out != "hello world\n" {
		t.Errorf("Receive() = %q, want %q", out, "hello world\n")
	}

	// Second receive should be EOF
	_, err = h.Receive()
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestMockHandle_Close(t *testing.T) {
	t.Parallel()
	h := &mockHandle{alive: true}
	if err := h.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if h.IsAlive() {
		t.Error("should not be alive after close")
	}

	// Send after close should fail
	if err := h.Send("test"); err == nil {
		t.Error("Send after close should fail")
	}
}

func TestMockHandle_Wait(t *testing.T) {
	t.Parallel()
	h := &mockHandle{alive: true}
	code, err := h.Wait()
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if h.IsAlive() {
		t.Error("should not be alive after wait")
	}
}

// --- ClaudeCodeProvider Tests ---

func TestClaudeCodeProvider_Name(t *testing.T) {
	t.Parallel()
	p := &ClaudeCodeProvider{}
	if p.Name() != "claude-code" {
		t.Errorf("Name() = %q, want %q", p.Name(), "claude-code")
	}
}

func TestClaudeCodeProvider_Capabilities(t *testing.T) {
	t.Parallel()
	p := &ClaudeCodeProvider{}
	caps := p.Capabilities()
	if !caps.MCP {
		t.Error("MCP should be true")
	}
	if !caps.Streaming {
		t.Error("Streaming should be true")
	}
	if !caps.MultiTurn {
		t.Error("MultiTurn should be true")
	}
}

func TestClaudeCodeProvider_SpawnEcho(t *testing.T) {
	// Skip on Windows — PTY not supported
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Use /bin/echo as a stand-in for claude — it prints and exits
	p := &ClaudeCodeProvider{Command: "/bin/echo"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{
		Args: []string{"hello from claude mock"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	// Read the echo output
	var output strings.Builder
	for range 10 {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if err != nil {
			break
		}
	}
	if !strings.Contains(output.String(), "hello from claude mock") {
		t.Errorf("output = %q, want to contain %q", output.String(), "hello from claude mock")
	}

	// Wait for exit
	code, _ := handle.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestClaudeCodeProvider_SpawnCat(t *testing.T) {
	// Skip on Windows — PTY not supported
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Use cat as an interactive agent: write → read → close
	p := &ClaudeCodeProvider{Command: "/bin/cat"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	if !handle.IsAlive() {
		t.Fatal("cat should be alive after spawn")
	}

	// Send and receive (PTY echo may include the sent text)
	if err := handle.Send("provider test\n"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Read output (may need multiple reads due to PTY buffering)
	var output strings.Builder
	for range 20 {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "provider test") {
			break
		}
		if err != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "provider test") {
		t.Errorf("output = %q, want to contain %q", output.String(), "provider test")
	}
}

func TestClaudeCodeProvider_SpawnWithModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Use echo to verify model flag is passed
	p := &ClaudeCodeProvider{Command: "/bin/echo"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{
		Model: "sonnet",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	var output strings.Builder
	for range 10 {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if err != nil {
			break
		}
	}
	if !strings.Contains(output.String(), "--model") || !strings.Contains(output.String(), "sonnet") {
		t.Errorf("output = %q, want to contain --model and sonnet", output.String())
	}
}

func TestClaudeCodeProvider_DefaultCommand(t *testing.T) {
	t.Parallel()
	// Don't actually spawn — just verify the command defaulting logic
	p := &ClaudeCodeProvider{}
	if p.Command != "" {
		t.Errorf("default Command should be empty (gets resolved in Spawn)")
	}
	// Custom command
	p2 := &ClaudeCodeProvider{Command: "/usr/local/bin/claude"}
	if p2.Command != "/usr/local/bin/claude" {
		t.Errorf("Command = %q, want /usr/local/bin/claude", p2.Command)
	}
}

func TestSpawnOpts_Defaults(t *testing.T) {
	t.Parallel()
	opts := SpawnOpts{}
	if opts.Model != "" {
		t.Error("default Model should be empty")
	}
	if opts.Rows != 0 {
		t.Error("default Rows should be 0")
	}
	if opts.Cols != 0 {
		t.Error("default Cols should be 0")
	}
	if opts.Dir != "" {
		t.Error("default Dir should be empty")
	}
	if opts.Env != nil {
		t.Error("default Env should be nil")
	}
	if opts.Args != nil {
		t.Error("default Args should be nil")
	}
}

func TestProviderCapabilities_ZeroValue(t *testing.T) {
	t.Parallel()
	caps := ProviderCapabilities{}
	if caps.MCP {
		t.Error("zero MCP should be false")
	}
	if caps.Streaming {
		t.Error("zero Streaming should be false")
	}
	if caps.MultiTurn {
		t.Error("zero MultiTurn should be false")
	}
}
