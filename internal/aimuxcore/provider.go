package aimuxcore

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
)

// Provider abstracts an interactive terminal agent backend.
type Provider interface {
	// Name returns the provider identifier.
	Name() string
	// Spawn starts an agent instance and returns a handle.
	Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error)
	// Capabilities returns what this provider supports.
	Capabilities() ProviderCapabilities
}

// AgentHandle represents a running interactive process.
type AgentHandle interface {
	// Send writes input to the process stdin.
	Send(input string) error
	// Receive reads available output from the process stdout.
	Receive() (string, error)
	// Close terminates the process and releases resources.
	Close() error
	// IsAlive returns whether the process is still running.
	IsAlive() bool
	// Wait blocks until the process exits. Returns the exit code.
	Wait() (int, error)
	// Resize changes the PTY window dimensions.
	Resize(rows, cols int) error
	// WaitReady blocks until the process signals readiness or the context is cancelled.
	WaitReady(ctx context.Context) error
}

// SpawnOpts configures process spawning.
type SpawnOpts struct {
	// Command is the executable to run.
	Command string
	// Args are additional CLI arguments.
	Args []string
	// Env contains additional environment variables.
	Env map[string]string
	// Dir is the working directory.
	Dir string
	// Rows is the PTY row count (0 = default).
	Rows uint16
	// Cols is the PTY column count (0 = default).
	Cols uint16
}

// ErrNotReady is returned when a process is not ready within the timeout.
var ErrNotReady = errors.New("aimux: agent not ready")

// ProviderCapabilities declares supported features.
type ProviderCapabilities struct {
	MCP       bool // Supports MCP tool calling
	Streaming bool // Supports streaming output
	MultiTurn bool // Supports multi-turn conversation
	Resizable bool // Supports PTY resize
}

// Registry manages available providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

var (
	// ErrProviderNotFound is returned when a provider name is not registered.
	ErrProviderNotFound = errors.New("aimux: provider not found")
	// ErrProviderExists is returned when registering a provider whose name is already taken.
	ErrProviderExists = errors.New("aimux: provider already registered")
	// ErrNoProviders is returned when an operation requires at least one registered provider.
	ErrNoProviders = errors.New("aimux: no providers registered")
)

// NewRegistry creates an empty provider registry.
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
	}
}

// Register adds a provider. Returns ErrProviderExists if name is taken.
func (r *Registry) Register(p Provider) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := p.Name()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("%w: %s", ErrProviderExists, name)
	}
	r.providers[name] = p
	return nil
}

// Get returns a provider by name. Returns ErrProviderNotFound if not registered.
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}
	return p, nil
}

// List returns sorted provider names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
