package claudemux

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

func TestOllamaProvider_Name(t *testing.T) {
	t.Parallel()
	p := &OllamaProvider{}
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

func TestOllamaProvider_Capabilities(t *testing.T) {
	t.Parallel()
	p := &OllamaProvider{}
	caps := p.Capabilities()
	if caps.MCP {
		t.Error("MCP should be false for Ollama")
	}
	if !caps.Streaming {
		t.Error("Streaming should be true")
	}
	if !caps.MultiTurn {
		t.Error("MultiTurn should be true")
	}
	if !caps.ModelNav {
		t.Error("ModelNav should be true for Ollama")
	}
}

func TestOllamaProvider_Capabilities_DiffersFromClaude(t *testing.T) {
	t.Parallel()
	ollama := &OllamaProvider{}
	claude := &ClaudeCodeProvider{}

	ollamaCaps := ollama.Capabilities()
	claudeCaps := claude.Capabilities()

	// Ollama requires model navigation; Claude doesn't.
	if !ollamaCaps.ModelNav {
		t.Error("Ollama ModelNav should be true")
	}
	if claudeCaps.ModelNav {
		t.Error("Claude ModelNav should be false")
	}

	// Claude supports MCP; Ollama doesn't.
	if !claudeCaps.MCP {
		t.Error("Claude MCP should be true")
	}
	if ollamaCaps.MCP {
		t.Error("Ollama MCP should be false")
	}
}

func TestOllamaProvider_DefaultCommand(t *testing.T) {
	t.Parallel()
	p := &OllamaProvider{}
	if p.Command != "" {
		t.Errorf("default Command should be empty (gets resolved in Spawn)")
	}
}

func TestOllamaProvider_CustomCommand(t *testing.T) {
	t.Parallel()
	p := &OllamaProvider{Command: "/usr/local/bin/ollama"}
	if p.Command != "/usr/local/bin/ollama" {
		t.Errorf("Command = %q, want /usr/local/bin/ollama", p.Command)
	}
}

func TestOllamaProvider_SpawnEcho(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Use echo as a stand-in for ollama to verify args are passed.
	p := &OllamaProvider{Command: "/bin/echo", SubArgs: []string{"run"}}
	handle, err := p.Spawn(context.Background(), SpawnOpts{
		Args: []string{"--extra"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	var output strings.Builder
	for i := 0; i < 10; i++ {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if err != nil {
			break
		}
	}

	out := output.String()
	// echo should print: "run --extra"
	if !strings.Contains(out, "run") {
		t.Errorf("output = %q, want to contain %q", out, "run")
	}
	if !strings.Contains(out, "--extra") {
		t.Errorf("output = %q, want to contain %q", out, "--extra")
	}
}

func TestOllamaProvider_SpawnDoesNotInjectModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Even when Model is set, OllamaProvider should NOT pass --model.
	p := &OllamaProvider{Command: "/bin/echo", SubArgs: []string{"run"}}
	handle, err := p.Spawn(context.Background(), SpawnOpts{
		Model: "should-be-ignored",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	var output strings.Builder
	for i := 0; i < 10; i++ {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if err != nil {
			break
		}
	}

	out := output.String()
	if strings.Contains(out, "--model") {
		t.Errorf("output = %q, should NOT contain --model (Ollama uses TUI navigation)", out)
	}
	if strings.Contains(out, "should-be-ignored") {
		t.Errorf("output = %q, should NOT contain model name", out)
	}
}

func TestOllamaProvider_SpawnCat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Interactive test: cat acts as a simple interactive agent.
	p := &OllamaProvider{Command: "/bin/cat"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	if !handle.IsAlive() {
		t.Fatal("cat should be alive after spawn")
	}

	if err := handle.Send("ollama test\n"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var output strings.Builder
	for i := 0; i < 20; i++ {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "ollama test") {
			break
		}
		if err != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "ollama test") {
		t.Errorf("output = %q, want to contain %q", output.String(), "ollama test")
	}
}

func TestOllamaProvider_SpawnNoSubArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// When SubArgs is empty, only the base command runs.
	p := &OllamaProvider{Command: "/bin/echo"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	code, _ := handle.Wait()
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
}

func TestOllamaProvider_RegisterInRegistry(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	p := &OllamaProvider{}
	if err := r.Register(p); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := r.Get("ollama")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", got.Name(), "ollama")
	}
}

func TestOllamaProvider_CoexistsWithClaude(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	_ = r.Register(&ClaudeCodeProvider{})
	_ = r.Register(&OllamaProvider{})

	names := r.List()
	if len(names) != 2 {
		t.Fatalf("List() = %v, want 2 entries", names)
	}
	if names[0] != "claude-code" || names[1] != "ollama" {
		t.Errorf("List() = %v, want [claude-code ollama]", names)
	}
}
