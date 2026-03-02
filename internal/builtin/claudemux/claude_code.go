package claudemux

import (
	"context"
	"io"

	"github.com/joeycumines/one-shot-man/internal/builtin/pty"
)

// ClaudeCodeProvider implements Provider for Claude Code via PTY.
type ClaudeCodeProvider struct {
	// Command is the path to the claude executable.
	// Default: "claude"
	Command string
}

// Name returns "claude-code".
func (p *ClaudeCodeProvider) Name() string { return "claude-code" }

// Capabilities returns Claude Code's capabilities.
func (p *ClaudeCodeProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		MCP:       true,
		Streaming: true,
		MultiTurn: true,
	}
}

// Spawn starts a Claude Code instance in a PTY.
func (p *ClaudeCodeProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	cmd := p.Command
	if cmd == "" {
		cmd = "claude"
	}

	args := make([]string, 0, len(opts.Args)+2)
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
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

// ptyAgentHandle wraps a pty.Process as an AgentHandle.
type ptyAgentHandle struct {
	proc *pty.Process
}

func (h *ptyAgentHandle) Send(input string) error {
	return h.proc.Write(input)
}

func (h *ptyAgentHandle) Receive() (string, error) {
	return h.proc.Read()
}

func (h *ptyAgentHandle) Close() error {
	return h.proc.Close()
}

func (h *ptyAgentHandle) IsAlive() bool {
	return h.proc.IsAlive()
}

func (h *ptyAgentHandle) Wait() (int, error) {
	return h.proc.Wait()
}

func (h *ptyAgentHandle) Signal(sig string) error {
	return h.proc.Signal(sig)
}

func (h *ptyAgentHandle) DrainOutput(sink io.Writer) {
	h.proc.DrainOutput(sink)
}
