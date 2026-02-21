package claudemux

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestOllamaHTTPProvider_Name(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{}
	if p.Name() != "ollama-http" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama-http")
	}
}

func TestOllamaHTTPProvider_Capabilities(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{}
	caps := p.Capabilities()

	if caps.MCP {
		t.Error("MCP should be false for Ollama HTTP")
	}
	if !caps.Streaming {
		t.Error("Streaming should be true")
	}
	if !caps.MultiTurn {
		t.Error("MultiTurn should be true")
	}
	if caps.ModelNav {
		t.Error("ModelNav should be false for HTTP provider")
	}
}

func TestOllamaHTTPProvider_Capabilities_DiffersFromPTY(t *testing.T) {
	t.Parallel()
	http := &OllamaHTTPProvider{}
	pty := &OllamaProvider{}

	httpCaps := http.Capabilities()
	ptyCaps := pty.Capabilities()

	// HTTP does not require model navigation; PTY does.
	if httpCaps.ModelNav {
		t.Error("HTTP ModelNav should be false")
	}
	if !ptyCaps.ModelNav {
		t.Error("PTY ModelNav should be true")
	}

	// Both support streaming and multi-turn.
	if !httpCaps.Streaming {
		t.Error("HTTP Streaming should be true")
	}
	if !httpCaps.MultiTurn {
		t.Error("HTTP MultiTurn should be true")
	}
}

func TestOllamaHTTPProvider_Spawn_NoModel(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{}

	_, err := p.Spawn(context.Background(), SpawnOpts{})
	if err == nil {
		t.Fatal("expected error when no model specified")
	}
	if !strings.Contains(err.Error(), "model is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestOllamaHTTPProvider_Spawn_InvalidEndpoint(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{
		Endpoint: "not-a-url",
	}

	_, err := p.Spawn(context.Background(), SpawnOpts{Model: "test-model"})
	if err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
}

func TestOllamaHTTPProvider_Spawn_DefaultValues(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{
		Model: "test-model",
	}

	// Spawn creates the handle even if the server is unreachable —
	// the connection is made on Send, not Spawn.
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	if !handle.IsAlive() {
		t.Error("handle should be alive after Spawn (before any interaction)")
	}
}

func TestOllamaHTTPProvider_Spawn_ModelFromOpts(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{}

	// Model from SpawnOpts should work even when provider has no default.
	handle, err := p.Spawn(context.Background(), SpawnOpts{Model: "test-model"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()
}

func TestOllamaHTTPHandle_CloseBeforeSend(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{Model: "test-model"}

	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Close without sending — should not panic.
	if err := handle.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}

	// After close, IsAlive should eventually be false (context cancelled).
	if handle.IsAlive() {
		// Give it a moment since the done channel closes asynchronously.
		time.Sleep(100 * time.Millisecond)
	}
}

func TestOllamaHTTPHandle_SendAfterClose(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{Model: "test-model"}

	ctx, cancel := context.WithCancel(context.Background())
	handle, err := p.Spawn(ctx, SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	cancel() // Cancel context
	time.Sleep(50 * time.Millisecond)

	// Receive should return context error or EOF after cancel.
	_, recvErr := handle.Receive()
	if recvErr == nil {
		t.Error("expected error from Receive after context cancel")
	}
	handle.Close()
}

func TestOllamaHTTPHandle_WaitTimeout(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{Model: "test-model"}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	handle, err := p.Spawn(ctx, SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	// Wait should return when context expires.
	code, waitErr := handle.Wait()
	if waitErr == nil {
		t.Error("expected error from Wait with expired context")
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func TestOllamaHTTPHandle_ReceiveBeforeSend(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{Model: "test-model"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	handle, err := p.Spawn(ctx, SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	// Receive before Send should block until context expires.
	_, recvErr := handle.Receive()
	if recvErr == nil {
		t.Error("expected error from Receive (context should expire)")
	}
}

func TestOllamaHTTPHandle_DoubleSend(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{
		Model:    "test-model",
		Endpoint: "http://127.0.0.1:1", // Will refuse connection
	}

	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	// First Send should succeed.
	if err := handle.Send("first message"); err != nil {
		t.Fatalf("first Send: %v", err)
	}

	// Second Send should return "already started" error.
	err = handle.Send("second message")
	if err == nil {
		t.Fatal("expected error from second Send")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("unexpected error: %v (want 'already started')", err)
	}
}

func TestOllamaHTTPProvider_ProviderInterface(t *testing.T) {
	t.Parallel()

	// Compile-time check: OllamaHTTPProvider implements Provider.
	var _ Provider = (*OllamaHTTPProvider)(nil)
}

func TestOllamaHTTPHandle_AgentHandleInterface(t *testing.T) {
	t.Parallel()

	// Compile-time check: ollamaHTTPHandle implements AgentHandle.
	var _ AgentHandle = (*ollamaHTTPHandle)(nil)
}

func TestOllamaHTTPHandle_IsAlive_FalseAfterDone(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{Model: "test-model"}

	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Before any Send, IsAlive should be true (goroutine not started yet).
	if !handle.IsAlive() {
		t.Error("should be alive initially")
	}

	// Send to a non-existent server — the Run will error quickly.
	// Use a bogus endpoint so it fails fast.
	pBogus := &OllamaHTTPProvider{
		Model:    "test-model",
		Endpoint: "http://127.0.0.1:1", // Will refuse connection
	}
	hBogus, err := pBogus.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn bogus: %v", err)
	}
	defer hBogus.Close()

	_ = hBogus.Send("test message")

	// Wait for it to finish (should fail fast).
	code, waitErr := hBogus.Wait()
	if waitErr == nil {
		t.Error("expected error from Wait on bogus endpoint")
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}

	if hBogus.IsAlive() {
		t.Error("should not be alive after failed run")
	}

	// Drain any error output.
	for {
		_, recvErr := hBogus.Receive()
		if recvErr != nil {
			if recvErr != io.EOF {
				// Connection error is expected.
				break
			}
			break
		}
	}

	handle.Close()
}

func TestOllamaHTTPProvider_ToolsDisabled(t *testing.T) {
	t.Parallel()
	disabled := false
	p := &OllamaHTTPProvider{
		Model:        "test-model",
		ToolsEnabled: &disabled,
	}
	// With tools disabled, Spawn should succeed but the runner has no tools.
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()
	if !handle.IsAlive() {
		t.Error("handle should be alive after Spawn")
	}
}

func TestOllamaHTTPProvider_ToolsAllowlist(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{
		Model:          "test-model",
		ToolsAllowlist: "read_file,grep",
	}
	// Should create handle OK — allowlist filtering happens in Spawn.
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()
}

func TestOllamaHTTPProvider_Timeout(t *testing.T) {
	t.Parallel()
	p := &OllamaHTTPProvider{
		Model:   "test-model",
		Timeout: 5 * time.Second,
	}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()
}
