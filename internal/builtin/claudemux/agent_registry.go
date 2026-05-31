package claudemux

import (
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// AgentCapabilities describes what an agent can do.
type AgentCapabilities struct {
	Tools       []string // Tool names the agent provides (e.g., "search", "edit").
	Models      []string // Model identifiers the agent supports.
	Specialties []string // Domain specialties (e.g., "code-review", "security").
	Streaming   bool     // Agent supports streaming output.
	MultiTurn   bool     // Agent supports multi-turn conversation.
}

// RegisteredAgent tracks a registered agent with its capabilities and health.
type RegisteredAgent struct {
	ID           string
	Instance     *Instance
	Capabilities AgentCapabilities
	Role         string
	Health       PaneHealth
	LastUsed     time.Time

	mu sync.RWMutex
}

// SetLastUsed updates the LastUsed timestamp.
func (a *RegisteredAgent) SetLastUsed(t time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.LastUsed = t
}

// GetHealth returns a copy of the current health state.
func (a *RegisteredAgent) GetHealth() PaneHealth {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Health
}

// GetRole returns the agent's assigned role.
func (a *RegisteredAgent) GetRole() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.Role
}

// CapabilityRequest describes required capabilities for agent selection.
type CapabilityRequest struct {
	RequiredTools       []string // Tools the agent must provide.
	RequiredModels      []string // Models the agent must support.
	RequiredSpecialties []string // Specialties the agent must have.
	NeedStreaming       bool     // Agent must support streaming.
	NeedMultiTurn       bool     // Agent must support multi-turn.
}

// RoleAssignment maps a role name to an agent ID.
type RoleAssignment struct {
	Name    string
	AgentID string
}

// AgentRegistry manages registered agents with capability advertisement,
// dynamic discovery, health-aware selection, and role assignment.
//
// AgentRegistry is safe for concurrent use from multiple goroutines.
type AgentRegistry struct {
	mu     sync.RWMutex
	agents map[string]*RegisteredAgent
	roles  map[string]*RoleAssignment
}

var (
	// ErrAgentNotFound is returned when looking up a non-existent agent.
	ErrAgentNotFound = errors.New("claudemux: agent not found")

	// ErrAgentExists is returned when registering a duplicate agent ID.
	ErrAgentExists = errors.New("claudemux: agent already registered")

	// ErrNoMatchingAgents is returned when no agent satisfies a capability request.
	ErrNoMatchingAgents = errors.New("claudemux: no matching agents")
)

// NewAgentRegistry creates an empty agent registry.
func NewAgentRegistry() *AgentRegistry {
	return &AgentRegistry{
		agents: make(map[string]*RegisteredAgent),
		roles:  make(map[string]*RoleAssignment),
	}
}

// Register adds an agent with the given capabilities. Returns
// ErrAgentExists if the ID is already registered.
func (r *AgentRegistry) Register(id string, inst *Instance, caps AgentCapabilities) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[id]; exists {
		return fmt.Errorf("%w: %s", ErrAgentExists, id)
	}

	r.agents[id] = &RegisteredAgent{
		ID:           id,
		Instance:     inst,
		Capabilities: caps,
		Health: PaneHealth{
			State: "idle",
		},
	}
	return nil
}

// Unregister removes an agent by ID. Returns ErrAgentNotFound if the
// agent does not exist. Also removes any role assignment for this agent.
func (r *AgentRegistry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, id)
	}

	// Remove role assignment if present.
	if agent.Role != "" {
		delete(r.roles, agent.Role)
	}

	delete(r.agents, id)
	return nil
}

// Query returns agent IDs that satisfy the capability request. Agents
// are sorted by ID for deterministic ordering.
func (r *AgentRegistry) Query(req CapabilityRequest) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matches []string
	for id, agent := range r.agents {
		if matchesCapabilities(agent.Capabilities, req) {
			matches = append(matches, id)
		}
	}
	slices.Sort(matches)
	return matches
}

// Select returns the healthiest agent that satisfies the capability request.
// Selection priority: lowest error count, then most recent task activity,
// then most recent LastUsed, then alphabetical ID for determinism.
// Returns ErrNoMatchingAgents if no agent matches.
func (r *AgentRegistry) Select(req CapabilityRequest) (*RegisteredAgent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*RegisteredAgent
	for _, agent := range r.agents {
		if matchesCapabilities(agent.Capabilities, req) {
			candidates = append(candidates, agent)
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoMatchingAgents
	}

	// Sort by health priority: fewest errors, then most recent activity,
	// then most recent LastUsed, then ID for determinism.
	slices.SortFunc(candidates, func(a, b *RegisteredAgent) int {
		aHealth := a.GetHealth()
		bHealth := b.GetHealth()

		// Prefer fewer errors.
		if aHealth.ErrorCount != bHealth.ErrorCount {
			return int(aHealth.ErrorCount - bHealth.ErrorCount)
		}

		// Prefer more recent LastUpdate (health activity).
		if !aHealth.LastUpdate.Equal(bHealth.LastUpdate) {
			// More recent is "less" (comes first).
			if aHealth.LastUpdate.After(bHealth.LastUpdate) {
				return -1
			}
			return 1
		}

		// Prefer more recent LastUsed.
		a.mu.RLock()
		aLastUsed := a.LastUsed
		a.mu.RUnlock()

		b.mu.RLock()
		bLastUsed := b.LastUsed
		b.mu.RUnlock()

		if !aLastUsed.Equal(bLastUsed) {
			if aLastUsed.After(bLastUsed) {
				return -1
			}
			return 1
		}

		// Deterministic tiebreak by ID.
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	return candidates[0], nil
}

// AssignRole assigns a named role to an agent. Overwrites any existing
// role for the agent and any existing assignment for the role name.
// Returns ErrAgentNotFound if the agent does not exist.
func (r *AgentRegistry) AssignRole(agentID, roleName string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	// Remove old role assignment if the agent had one.
	if agent.Role != "" {
		delete(r.roles, agent.Role)
	}

	// Remove old agent from this role if one was assigned.
	if existing, ok := r.roles[roleName]; ok {
		if oldAgent, ok := r.agents[existing.AgentID]; ok {
			oldAgent.Role = ""
		}
	}

	agent.Role = roleName
	r.roles[roleName] = &RoleAssignment{
		Name:    roleName,
		AgentID: agentID,
	}
	return nil
}

// GetByRole returns agent IDs assigned to the given role. Returns an
// empty slice if no agents have the role.
func (r *AgentRegistry) GetByRole(roleName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	role, ok := r.roles[roleName]
	if !ok {
		return nil
	}

	// Verify the agent still exists.
	if _, exists := r.agents[role.AgentID]; !exists {
		return nil
	}

	return []string{role.AgentID}
}

// UpdateHealth updates the health state for an agent. Returns
// ErrAgentNotFound if the agent does not exist.
func (r *AgentRegistry) UpdateHealth(agentID string, health PaneHealth) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	agent.mu.Lock()
	agent.Health = health
	agent.mu.Unlock()

	return nil
}

// List returns all registered agent IDs in sorted order.
func (r *AgentRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// matchesCapabilities checks whether agent capabilities satisfy a request.
func matchesCapabilities(caps AgentCapabilities, req CapabilityRequest) bool {
	if req.NeedStreaming && !caps.Streaming {
		return false
	}
	if req.NeedMultiTurn && !caps.MultiTurn {
		return false
	}
	if !containsAll(caps.Tools, req.RequiredTools) {
		return false
	}
	if !containsAll(caps.Models, req.RequiredModels) {
		return false
	}
	if !containsAll(caps.Specialties, req.RequiredSpecialties) {
		return false
	}
	return true
}

// containsAll returns true if superset contains every element in subset.
func containsAll(superset, subset []string) bool {
	for _, s := range subset {
		if !slices.Contains(superset, s) {
			return false
		}
	}
	return true
}
