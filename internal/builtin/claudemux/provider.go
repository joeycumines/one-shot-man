package claudemux

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Provider abstracts an AI agent backend (Claude Code, Ollama, etc.).
type Provider interface {
	// Name returns the provider identifier (e.g., "claude-code").
	Name() string
	// Spawn starts an agent instance and returns a handle.
	Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error)
	// Capabilities returns what this provider supports.
	Capabilities() ProviderCapabilities
}

// AgentHandle represents a running agent instance.
type AgentHandle interface {
	// Send writes input to the agent's stdin.
	Send(input string) error
	// Receive reads available output from the agent's stdout.
	// Returns ("", io.EOF) when the agent has exited.
	Receive() (string, error)
	// Close terminates the agent and releases resources.
	Close() error
	// IsAlive returns whether the agent process is still running.
	IsAlive() bool
	// Wait blocks until the agent exits. Returns the exit code.
	Wait() (int, error)
}

// SpawnOpts configures agent spawning.
type SpawnOpts struct {
	// Model identifier (provider-specific, e.g., "claude-sonnet-4-20250514").
	Model string
	// Env contains additional environment variables.
	Env map[string]string
	// Dir is the working directory.
	Dir string
	// Rows is the PTY row count (0 = provider default).
	Rows uint16
	// Cols is the PTY column count (0 = provider default).
	Cols uint16
	// Args are additional CLI arguments for the provider command.
	Args []string
}

// ProviderCapabilities declares supported features.
type ProviderCapabilities struct {
	MCP       bool // Supports MCP tool calling
	Streaming bool // Supports streaming output
	MultiTurn bool // Supports multi-turn conversation
	ModelNav  bool // Requires TUI model navigation after spawn
}

// Registry manages available providers.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

var (
	// ErrProviderNotFound is returned when a provider name is not registered.
	ErrProviderNotFound = errors.New("claudemux: provider not found")
	// ErrProviderExists is returned when registering a provider whose name is already taken.
	ErrProviderExists = errors.New("claudemux: provider already registered")
	// ErrNoProviders is returned when an operation requires at least one registered provider.
	ErrNoProviders = errors.New("claudemux: no providers registered")
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
	sort.Strings(names)
	return names
}

// Spawn is a convenience method that gets a provider by name and spawns an agent.
func (r *Registry) Spawn(ctx context.Context, providerName string, opts SpawnOpts) (AgentHandle, error) {
	p, err := r.Get(providerName)
	if err != nil {
		return nil, err
	}
	return p.Spawn(ctx, opts)
}
