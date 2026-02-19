package claudemux

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Instance represents a single isolated Claude Code instance with its own
// state directory, MCP endpoint, and agent handle. Each instance has
// independent resources and no shared mutable state.
type Instance struct {
	// ID is the unique session identifier for this instance.
	ID string

	// StateDir is the isolated state directory for this instance
	// (e.g., ~/.osm/claude-sessions/<id>/).
	StateDir string

	// MCP is the per-instance MCP server configuration (from T006).
	// May be nil if MCP is not configured for this instance.
	MCP *MCPInstanceConfig

	// Agent is the PTY-backed agent handle. Set after spawning.
	Agent AgentHandle

	// CreatedAt is the timestamp when this instance was created.
	CreatedAt time.Time

	mu     sync.Mutex
	closed bool
}

// InstanceState is the JSON-serializable metadata written to state.json.
type InstanceState struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	Status    string    `json:"status"` // "active", "closed"
	MCPEndpoint string  `json:"mcpEndpoint,omitempty"`
}

// Close releases all resources held by this instance: stops the agent,
// closes the MCP endpoint, and writes final state. Safe to call multiple
// times.
func (inst *Instance) Close() error {
	inst.mu.Lock()
	if inst.closed {
		inst.mu.Unlock()
		return nil
	}
	inst.closed = true
	agent := inst.Agent
	mcpCfg := inst.MCP
	inst.mu.Unlock()

	var errs []error

	// Close agent first (stops the PTY process).
	if agent != nil {
		if err := agent.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close agent: %w", err))
		}
	}

	// Close MCP endpoint (stops HTTP server, removes temp files).
	if mcpCfg != nil {
		if err := mcpCfg.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close mcp: %w", err))
		}
	}

	// Write final state.
	_ = inst.writeState("closed")

	return errors.Join(errs...)
}

// IsClosed returns whether this instance has been closed.
func (inst *Instance) IsClosed() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.closed
}

// writeState writes the instance state to state.json in the state directory.
func (inst *Instance) writeState(status string) error {
	state := InstanceState{
		ID:        inst.ID,
		CreatedAt: inst.CreatedAt,
		Status:    status,
	}
	if inst.MCP != nil {
		state.MCPEndpoint = inst.MCP.Endpoint()
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	statePath := filepath.Join(inst.StateDir, "state.json")
	return os.WriteFile(statePath, data, 0600)
}

// InstanceRegistry manages all active Claude Code instances with isolated
// state. It uses sync.Map for lock-free concurrent access.
type InstanceRegistry struct {
	instances sync.Map // string -> *Instance
	baseDir   string   // base directory for state dirs
}

var (
	// ErrInstanceNotFound is returned when looking up a non-existent instance.
	ErrInstanceNotFound = errors.New("claudemux: instance not found")

	// ErrInstanceIDExists is returned when creating a duplicate instance ID.
	ErrInstanceIDExists = errors.New("claudemux: instance ID already exists")

	// ErrInstanceIDEmpty is returned when the instance ID is empty.
	ErrInstanceIDEmpty = errors.New("claudemux: instance ID is required")
)

// NewInstanceRegistry creates a registry that stores instance state under
// baseDir. The directory is created if it does not exist.
func NewInstanceRegistry(baseDir string) (*InstanceRegistry, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("claudemux: base directory is required")
	}
	if err := os.MkdirAll(baseDir, 0700); err != nil {
		return nil, fmt.Errorf("claudemux: create base dir: %w", err)
	}
	return &InstanceRegistry{baseDir: baseDir}, nil
}

// Create creates and registers a new isolated instance. It allocates the
// state directory and writes initial state. The caller is responsible for
// setting up the MCP endpoint and spawning the agent.
func (r *InstanceRegistry) Create(sessionID string) (*Instance, error) {
	if sessionID == "" {
		return nil, ErrInstanceIDEmpty
	}

	// Create isolated state directory path.
	safe := mcpSessionIDSafe.ReplaceAllString(sessionID, "_")
	if len(safe) > 64 {
		safe = safe[:64]
	}
	stateDir := filepath.Join(r.baseDir, safe)

	inst := &Instance{
		ID:        sessionID,
		StateDir:  stateDir,
		CreatedAt: time.Now(),
	}

	// Atomically try to register. Only the first Create for this ID wins.
	if _, loaded := r.instances.LoadOrStore(sessionID, inst); loaded {
		return nil, fmt.Errorf("%w: %s", ErrInstanceIDExists, sessionID)
	}

	// We won the race. Create the state directory.
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		r.instances.Delete(sessionID)
		return nil, fmt.Errorf("claudemux: create state dir: %w", err)
	}

	// Create logs subdirectory.
	if err := os.MkdirAll(filepath.Join(stateDir, "logs"), 0700); err != nil {
		r.instances.Delete(sessionID)
		_ = os.RemoveAll(stateDir)
		return nil, fmt.Errorf("claudemux: create logs dir: %w", err)
	}

	// Write initial state.
	if err := inst.writeState("active"); err != nil {
		r.instances.Delete(sessionID)
		_ = os.RemoveAll(stateDir)
		return nil, fmt.Errorf("claudemux: write initial state: %w", err)
	}

	return inst, nil
}

// Get retrieves an instance by session ID. Returns (nil, false) if not found.
func (r *InstanceRegistry) Get(sessionID string) (*Instance, bool) {
	v, ok := r.instances.Load(sessionID)
	if !ok {
		return nil, false
	}
	return v.(*Instance), true
}

// Close closes and deregisters an instance by session ID.
func (r *InstanceRegistry) Close(sessionID string) error {
	v, ok := r.instances.LoadAndDelete(sessionID)
	if !ok {
		return fmt.Errorf("%w: %s", ErrInstanceNotFound, sessionID)
	}
	return v.(*Instance).Close()
}

// CloseAll closes all registered instances. Returns the first error
// encountered, but attempts to close all instances regardless.
func (r *InstanceRegistry) CloseAll() error {
	var errs []error
	r.instances.Range(func(key, value any) bool {
		inst := value.(*Instance)
		r.instances.Delete(key)
		if err := inst.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", inst.ID, err))
		}
		return true
	})
	return errors.Join(errs...)
}

// List returns all active session IDs in sorted order.
func (r *InstanceRegistry) List() []string {
	var ids []string
	r.instances.Range(func(key, _ any) bool {
		ids = append(ids, key.(string))
		return true
	})
	sort.Strings(ids)
	return ids
}

// Len returns the number of active instances.
func (r *InstanceRegistry) Len() int {
	n := 0
	r.instances.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// BaseDir returns the base directory for instance state.
func (r *InstanceRegistry) BaseDir() string {
	return r.baseDir
}
