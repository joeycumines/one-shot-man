package claudemux

import (
	"context"

	"github.com/joeycumines/one-shot-man/internal/builtin/pty"
)

// OllamaProvider implements Provider for Ollama-hosted models via PTY.
//
// Unlike ClaudeCodeProvider, Ollama selects models via an interactive TUI menu
// after spawning. The model is NOT passed as a CLI flag — instead, the caller
// navigates the TUI using keystroke sequences (see NavigateToModel).
type OllamaProvider struct {
	// Command is the base Ollama executable path (default: "ollama").
	Command string
	// SubArgs are additional arguments appended after the command
	// (e.g., ["run"] for "ollama run"). If empty, Command is used as-is
	// and the full command string may include subcommands via shell
	// word-splitting (e.g., Command="ollama run" with empty SubArgs).
	SubArgs []string
}

// Name returns "ollama".
func (p *OllamaProvider) Name() string { return "ollama" }

// Capabilities returns Ollama's capabilities.
func (p *OllamaProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		MCP:       false, // Ollama does not natively support MCP
		Streaming: true,
		MultiTurn: true,
		ModelNav:  true, // Model selected via TUI navigation post-spawn
	}
}

// Spawn starts an Ollama instance in a PTY.
//
// Model selection is NOT handled here — the caller is responsible for
// navigating the TUI after spawn (see ParseModelMenu + NavigateToModel).
// The opts.Model field is ignored; use TUI navigation instead.
func (p *OllamaProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	cmd := p.Command
	if cmd == "" {
		cmd = "ollama"
	}

	// Build args: SubArgs first, then any extra Args from opts.
	args := make([]string, 0, len(p.SubArgs)+len(opts.Args))
	args = append(args, p.SubArgs...)
	args = append(args, opts.Args...)

	cfg := pty.SpawnConfig{
		Command: cmd,
		Args:    args,
		Env:     opts.Env,
		Dir:     opts.Dir,
		Rows:    opts.Rows,
		Cols:    opts.Cols,
	}

	proc, err := pty.Spawn(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return &ptyAgentHandle{proc: proc}, nil
}
