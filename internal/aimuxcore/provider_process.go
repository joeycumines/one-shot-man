package aimuxcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// ProcessProvider spawns an arbitrary command through the termmux CaptureSession
// abstraction. It is the sole process gateway for aimux; no direct PTY/exec
// calls are made here.
type ProcessProvider struct {
	name        string
	command     string
	defaultArgs []string
	caps        ProviderCapabilities
}

// NewProcessProvider creates a generic provider that runs `command` with optional
// default args.
func NewProcessProvider(name, command string, defaultArgs []string, caps ProviderCapabilities) *ProcessProvider {
	if name == "" {
		name = command
	}
	return &ProcessProvider{
		name:        name,
		command:     command,
		defaultArgs: append([]string(nil), defaultArgs...),
		caps:        caps,
	}
}

// Name returns the provider identifier.
func (p *ProcessProvider) Name() string { return p.name }

// Capabilities returns the configured capabilities.
func (p *ProcessProvider) Capabilities() ProviderCapabilities { return p.caps }

// Spawn starts command via termmux.CaptureSession.
func (p *ProcessProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	cmd := opts.Command
	if cmd == "" {
		cmd = p.command
	}
	if cmd == "" {
		return nil, errors.New("aimux: provider command is required")
	}

	args := append(append([]string(nil), p.defaultArgs...), opts.Args...)
	rows := int(opts.Rows)
	cols := int(opts.Cols)
	if rows <= 0 {
		rows = termmux.DefaultRows
	}
	if cols <= 0 {
		cols = termmux.DefaultCols
	}

	cfg := termmux.CaptureConfig{
		Name:    p.name,
		Command: cmd,
		Args:    args,
		Dir:     opts.Dir,
		Env:     opts.Env,
		Rows:    rows,
		Cols:    cols,
	}

	cs := termmux.NewCaptureSession(cfg)
	if err := cs.Start(ctx); err != nil {
		return nil, fmt.Errorf("aimux: failed to spawn %q: %w", cmd, err)
	}

	return newCaptureAgentHandle(cs), nil
}

// captureAgentHandle adapts a termmux.CaptureSession to the AgentHandle interface.
type captureAgentHandle struct {
	cs    *termmux.CaptureSession
	ch    chan []byte
	ready chan struct{}
	once  sync.Once
}

func newCaptureAgentHandle(cs *termmux.CaptureSession) *captureAgentHandle {
	h := &captureAgentHandle{
		cs:    cs,
		ch:    make(chan []byte, 1024),
		ready: make(chan struct{}),
	}
	if r := cs.Reader(); r != nil {
		go h.forwardOutput(r)
	}
	return h
}

func (h *captureAgentHandle) forwardOutput(src <-chan []byte) {
	defer close(h.ch)
	for chunk := range src {
		h.ch <- chunk
		h.once.Do(func() { close(h.ready) })
	}
}

func (h *captureAgentHandle) Send(input string) error {
	if h.cs == nil {
		return errors.New("aimux: handle closed")
	}
	return h.cs.WriteString(input)
}

func (h *captureAgentHandle) Receive() (string, error) {
	if h.cs == nil {
		return "", errors.New("aimux: handle closed")
	}
	if h.ch == nil {
		return "", errors.New("aimux: session not started")
	}
	select {
	case chunk, ok := <-h.ch:
		if !ok {
			return "", io.EOF
		}
		return string(chunk), nil
	case <-h.cs.Done():
		return "", io.EOF
	}
}

func (h *captureAgentHandle) Close() error {
	if h.cs == nil {
		return nil
	}
	return h.cs.Close()
}

func (h *captureAgentHandle) IsAlive() bool {
	if h.cs == nil {
		return false
	}
	select {
	case <-h.cs.Done():
		return false
	default:
		return true
	}
}

func (h *captureAgentHandle) Wait() (int, error) {
	if h.cs == nil {
		return -1, errors.New("aimux: handle closed")
	}
	return h.cs.Wait()
}

func (h *captureAgentHandle) Resize(rows, cols int) error {
	if h.cs == nil {
		return errors.New("aimux: handle closed")
	}
	return h.cs.Resize(rows, cols)
}

func (h *captureAgentHandle) WaitReady(ctx context.Context) error {
	if h.cs == nil {
		return errors.New("aimux: handle closed")
	}
	if h.ready == nil {
		return errors.New("aimux: session not started")
	}

	select {
	case <-h.ready:
		return nil
	case <-h.cs.Done():
		return errors.New("aimux: process exited before becoming ready")
	case <-ctx.Done():
		return ctx.Err()
	}
}
