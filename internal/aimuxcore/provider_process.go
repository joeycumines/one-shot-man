package aimuxcore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

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
	cs       *termmux.CaptureSession
	ch       chan []byte
	eventsCh chan LineEvent
	ready    chan struct{}
	once     sync.Once

	healthMu  sync.RWMutex
	lastEvent time.Time
	lastSend  time.Time
}

func newCaptureAgentHandle(cs *termmux.CaptureSession) *captureAgentHandle {
	h := &captureAgentHandle{
		cs:       cs,
		ch:       make(chan []byte, 1024),
		eventsCh: make(chan LineEvent, 256),
		ready:    make(chan struct{}),
	}
	if r := cs.Reader(); r != nil {
		go h.forwardOutput(r)
	}
	return h
}

func (h *captureAgentHandle) forwardOutput(src <-chan []byte) {
	defer close(h.ch)
	defer close(h.eventsCh)

	var lineBuf []byte
	for chunk := range src {
		h.ch <- chunk
		h.once.Do(func() { close(h.ready) })

		lineBuf = append(lineBuf, chunk...)
		lineBuf = h.drainLines(lineBuf)
	}

	if len(lineBuf) > 0 {
		line := strings.TrimRight(string(lineBuf), "\r")
		h.emitLineEvent(LineEvent{Line: line})
	}

	h.emitLineEvent(LineEvent{Err: io.EOF})
}

// drainLines splits buf on \n (stripping preceding \r) and emits a LineEvent
// for each complete line. Returns the remaining incomplete tail.
func (h *captureAgentHandle) drainLines(buf []byte) []byte {
	for {
		idx := bytes.IndexByte(buf, '\n')
		if idx < 0 {
			return buf
		}
		line := strings.TrimRight(string(buf[:idx]), "\r")
		h.emitLineEvent(LineEvent{Line: line})
		buf = buf[idx+1:]
	}
}

func (h *captureAgentHandle) emitLineEvent(ev LineEvent) {
	if ev.Err == nil {
		h.healthMu.Lock()
		h.lastEvent = time.Now()
		h.healthMu.Unlock()
	}
	select {
	case h.eventsCh <- ev:
	default:
		slog.Debug("events channel full dropping line event",
			"err", ev.Err, "lineLen", len(ev.Line))
	}
}

func (h *captureAgentHandle) Send(input string) error {
	if h.cs == nil {
		return errors.New("aimux: handle closed")
	}
	if err := h.cs.WriteString(input); err != nil {
		return err
	}
	h.healthMu.Lock()
	h.lastSend = time.Now()
	h.healthMu.Unlock()
	return nil
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
		// Process exited — but output may still be in the pipeline.
		// Wait a brief grace period for the reader to flush remaining
		// output and signal ready. Use NewTimer (not time.After) and
		// defer Stop so the timer is reaped immediately if h.ready or
		// ctx.Done() wins the race, instead of the timer goroutine
		// lingering until the full 500ms elapses.
		timer := time.NewTimer(500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-h.ready:
			return nil
		case <-timer.C:
			return errors.New("aimux: process exited before becoming ready")
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *captureAgentHandle) Events() <-chan LineEvent {
	return h.eventsCh
}

func (h *captureAgentHandle) Health() HealthSnapshot {
	h.healthMu.RLock()
	lastEvent := h.lastEvent
	lastSend := h.lastSend
	h.healthMu.RUnlock()
	return HealthSnapshot{
		Alive:     h.IsAlive(),
		LastEvent: lastEvent,
		LastSend:  lastSend,
	}
}
