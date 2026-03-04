package claudemux

import (
	"context"

	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
)

// OllamaProvider launches Claude Code via `ollama launch claude`.
//
// This provider runs `ollama launch claude [--model MODEL] [extra flags]`.
// Once past the Ollama launcher menu, the process IS Claude Code — it has
// full MCP support, multi-turn, streaming, etc. The Ollama launcher menu
// may appear first (see IsLauncherMenu / DismissLauncherKeys) and must be
// dismissed before normal Claude Code interaction begins.
type OllamaProvider struct {
	// Command is the Ollama executable path (default: "ollama").
	Command string
	// ExtraArgs are additional CLI flags appended after "launch claude"
	// (e.g., ["--config", "/path/to/cfg"]).
	ExtraArgs []string
}

// Name returns "ollama".
func (p *OllamaProvider) Name() string { return "ollama" }

// Capabilities returns capabilities. Once the Ollama launcher menu is
// dismissed, this IS Claude Code — full MCP, streaming, multi-turn.
// ModelNav is true because the launcher menu must be dismissed first.
func (p *OllamaProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		MCP:       true, // Claude Code via Ollama supports MCP
		Streaming: true,
		MultiTurn: true,
		ModelNav:  true, // Ollama launcher menu must be dismissed
	}
}

// Spawn starts `ollama launch claude` in a PTY.
//
// The command is always `ollama launch claude`. If opts.Model is set, it is
// passed as `--model MODEL`. ExtraArgs and opts.Args are appended after.
func (p *OllamaProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	cmd := p.Command
	if cmd == "" {
		cmd = "ollama"
	}

	// Base args: launch claude
	args := []string{"launch", "claude"}

	// Model via --model flag (not TUI navigation).
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	// ExtraArgs from provider config.
	args = append(args, p.ExtraArgs...)

	// Additional args from spawn opts.
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
