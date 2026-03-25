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
	// Once launched, this IS Claude Code — MCP must be true.
	if !caps.MCP {
		t.Error("MCP should be true (ollama launch claude = Claude Code)")
	}
	if !caps.Streaming {
		t.Error("Streaming should be true")
	}
	if !caps.MultiTurn {
		t.Error("MultiTurn should be true")
	}
	if !caps.ModelNav {
		t.Error("ModelNav should be true (launcher menu must be dismissed)")
	}
}

func TestOllamaProvider_Capabilities_VsClaude(t *testing.T) {
	t.Parallel()
	ollama := &OllamaProvider{}
	claude := &ClaudeCodeProvider{}

	ollamaCaps := ollama.Capabilities()
	claudeCaps := claude.Capabilities()

	// Ollama requires launcher menu dismissal; Claude doesn't.
	if !ollamaCaps.ModelNav {
		t.Error("Ollama ModelNav should be true")
	}
	if claudeCaps.ModelNav {
		t.Error("Claude ModelNav should be false")
	}

	// Both support MCP — Ollama launches Claude Code.
	if !claudeCaps.MCP {
		t.Error("Claude MCP should be true")
	}
	if !ollamaCaps.MCP {
		t.Error("Ollama MCP should be true (launches Claude Code)")
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

func TestOllamaProvider_SpawnEcho_LaunchClaude(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Use echo to verify the args are: launch claude <extra>
	p := &OllamaProvider{Command: "/bin/echo"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{
		Args: []string{"--extra"},
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

	out := output.String()
	// echo should print: "launch claude --extra"
	if !strings.Contains(out, "launch") {
		t.Errorf("output = %q, want to contain %q", out, "launch")
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("output = %q, want to contain %q", out, "claude")
	}
	if !strings.Contains(out, "--extra") {
		t.Errorf("output = %q, want to contain %q", out, "--extra")
	}
}

func TestOllamaProvider_SpawnWithModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// When Model is set, it should be passed as --model.
	p := &OllamaProvider{Command: "/bin/echo"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{
		Model: "gpt-oss:20b-cloud",
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

	out := output.String()
	// echo should print: "launch claude --model gpt-oss:20b-cloud"
	if !strings.Contains(out, "launch") {
		t.Errorf("output = %q, want to contain %q", out, "launch")
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("output = %q, want to contain %q", out, "claude")
	}
	if !strings.Contains(out, "--model") {
		t.Errorf("output = %q, want to contain %q", out, "--model")
	}
	if !strings.Contains(out, "gpt-oss:20b-cloud") {
		t.Errorf("output = %q, want to contain %q", out, "gpt-oss:20b-cloud")
	}
}

func TestOllamaProvider_SpawnWithExtraArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// ExtraArgs appear after "launch claude".
	p := &OllamaProvider{Command: "/bin/echo", ExtraArgs: []string{"--config", "/tmp/cfg"}}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
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

	out := output.String()
	if !strings.Contains(out, "launch") {
		t.Errorf("output = %q, want to contain %q", out, "launch")
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("output = %q, want to contain %q", out, "claude")
	}
	if !strings.Contains(out, "--config") {
		t.Errorf("output = %q, want to contain %q", out, "--config")
	}
}

func TestOllamaProvider_SpawnCat(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Interactive test: cat echoes input. We verify the handle works.
	// Note: cat ignores args so "launch claude" args don't matter.
	p := &OllamaProvider{Command: "/bin/cat"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer handle.Close()

	if !handle.IsAlive() {
		t.Fatal("cat should be alive after spawn")
	}

	if err := handle.Send("hello claude\n"); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var output strings.Builder
	for range 20 {
		data, err := handle.Receive()
		if data != "" {
			output.WriteString(data)
		}
		if strings.Contains(output.String(), "hello claude") {
			break
		}
		if err != nil {
			break
		}
	}

	if !strings.Contains(output.String(), "hello claude") {
		t.Errorf("output = %q, want to contain %q", output.String(), "hello claude")
	}
}

func TestOllamaProvider_SpawnNoModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY not supported on Windows")
	}
	t.Parallel()

	// Without Model, no --model flag should appear.
	p := &OllamaProvider{Command: "/bin/echo"}
	handle, err := p.Spawn(context.Background(), SpawnOpts{})
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

	out := output.String()
	// Should print: "launch claude" (no --model)
	if !strings.Contains(out, "launch") {
		t.Errorf("output = %q, want to contain %q", out, "launch")
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("output = %q, want to contain %q", out, "claude")
	}
	if strings.Contains(out, "--model") {
		t.Errorf("output = %q, should NOT contain --model when Model is empty", out)
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
