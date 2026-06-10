package claudemux

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/joeycumines/one-shot-man/internal/termmux/pty"
)

// ClaudeCodeProvider implements Provider for Claude Code via PTY.
type ClaudeCodeProvider struct {
	// Command is the path to the claude executable.
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
	proc     *pty.Process
	detector *VTStateDetector
}

func (h *ptyAgentHandle) Send(input string) error {
	_, err := h.proc.Write([]byte(input))
	return err
}

func (h *ptyAgentHandle) Receive() (string, error) {
	data, err := h.proc.Read()
	if err != nil {
		return "", err
	}
	return string(data), nil
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

func (h *ptyAgentHandle) Resize(rows, cols int) error {
	if rows <= 0 || cols <= 0 {
		return errors.New("resize: rows and cols must be positive")
	}
	if rows > 65535 || cols > 65535 {
		return errors.New("resize: rows and cols must be <= 65535")
	}
	return h.proc.Resize(uint16(rows), uint16(cols))
}

func (h *ptyAgentHandle) Signal(sig string) error {
	return h.proc.Signal(sig)
}

func (h *ptyAgentHandle) DrainOutput(sink io.Writer) {
	h.proc.DrainOutput(sink)
}

func (h *ptyAgentHandle) WaitReady(ctx context.Context) error {
	if h.detector == nil {
		return nil
	}
	for {
		if h.detector.State() == StateReady {
			return nil
		}
		chunk, err := h.proc.Read()
		if err != nil {
			return fmt.Errorf("waitReady: read: %w", err)
		}
		if len(chunk) > 0 {
			h.detector.ProcessRaw(chunk, time.Now())
			if h.detector.State() == StateReady {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}
