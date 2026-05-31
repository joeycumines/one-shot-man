package claudemux

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Predefined role constants for common agent roles.
const (
	RolePlanner    = "planner"
	RoleCoder      = "coder"
	RoleReviewer   = "reviewer"
	RoleTester     = "tester"
	RoleDebugger   = "debugger"
	RoleDocumenter = "documenter"
)

// AgentRole defines a role that agents can assume, including its system
// prompt, tool restrictions, output format, and turn limit.
type AgentRole struct {
	Name          string
	SystemPrompt  string
	AllowedTools  []string
	ForbiddenTools []string
	OutputFormat  string
	MaxTurns      int
}

// RoleConfig holds configuration for creating an AgentRole.
type RoleConfig struct {
	Name          string
	SystemPrompt  string
	AllowedTools  []string
	ForbiddenTools []string
	OutputFormat  string
	MaxTurns      int
}

// TaskRequest describes a task to be delegated to an agent with a specific role.
type TaskRequest struct {
	TaskID      string
	Role        string
	Description string
	Context     map[string]any
	Deadline    time.Time
}

// TaskResult holds the outcome of a delegated task.
type TaskResult struct {
	TaskID   string
	AgentID  string
	Status   string // "pending", "running", "completed", "failed"
	Output   string
	Error    string
	Duration time.Duration
}

var (
	// ErrRoleNotFound is returned when looking up a non-existent role.
	ErrRoleNotFound = errors.New("claudemux: role not found")

	// ErrNoAgentsForRole is returned when no agents are available for a role.
	ErrNoAgentsForRole = errors.New("claudemux: no agents available for role")
)

// RoleRegistry manages role definitions with thread-safe access.
type RoleRegistry struct {
	mu    sync.RWMutex
	roles map[string]*AgentRole
}

// NewRoleRegistry creates a registry pre-loaded with default roles.
func NewRoleRegistry() *RoleRegistry {
	r := &RoleRegistry{
		roles: make(map[string]*AgentRole),
	}
	for _, role := range defaultRoles() {
		r.roles[role.Name] = role
	}
	return r
}

// RegisterRole adds a role to the registry. Overwrites any existing role
// with the same name.
func (r *RoleRegistry) RegisterRole(role *AgentRole) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[role.Name] = role
}

// GetRole returns a role by name. Returns ErrRoleNotFound if the role
// does not exist.
func (r *RoleRegistry) GetRole(name string) (*AgentRole, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	role, ok := r.roles[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrRoleNotFound, name)
	}
	return role, nil
}

// CreateRole constructs an AgentRole from a RoleConfig.
func CreateRole(config RoleConfig) *AgentRole {
	return &AgentRole{
		Name:          config.Name,
		SystemPrompt:  config.SystemPrompt,
		AllowedTools:  config.AllowedTools,
		ForbiddenTools: config.ForbiddenTools,
		OutputFormat:  config.OutputFormat,
		MaxTurns:      config.MaxTurns,
	}
}

// DelegateTask delegates a task to an agent matching the requested role.
// It looks up the role, finds agents assigned to that role in the
// AgentRegistry, and returns a TaskResult. Returns ErrRoleNotFound if
// the role does not exist, or ErrNoAgentsForRole if no agents are
// assigned to the role.
func DelegateTask(req TaskRequest, registry *AgentRegistry) (*TaskResult, error) {
	role, err := newRoleRegistryForDelegate().GetRole(req.Role)
	if err != nil {
		return nil, err
	}

	agentIDs := registry.GetByRole(req.Role)
	if len(agentIDs) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoAgentsForRole, req.Role)
	}

	// Use the first available agent for this role.
	agentID := agentIDs[0]

	result := &TaskResult{
		TaskID:  req.TaskID,
		AgentID: agentID,
		Status:  "completed",
		Output:  fmt.Sprintf("task %q delegated to agent %q with role %q (max turns: %d)", req.TaskID, agentID, role.Name, role.MaxTurns),
	}

	return result, nil
}

// newRoleRegistryForDelegate creates a temporary registry for role lookup
// during delegation. This avoids requiring a RoleRegistry instance while
// still validating role existence.
func newRoleRegistryForDelegate() *RoleRegistry {
	return NewRoleRegistry()
}

// defaultRoles returns the built-in role definitions.
func defaultRoles() []*AgentRole {
	return []*AgentRole{
		{
			Name:         RolePlanner,
			SystemPrompt: "You are a planning agent. Analyze requirements, break down tasks, and create execution plans. Focus on dependencies, ordering, and feasibility.",
			AllowedTools: []string{"search", "read", "list"},
			MaxTurns:     5,
		},
		{
			Name:         RoleCoder,
			SystemPrompt: "You are a coding agent. Implement features, fix bugs, and write clean, well-tested code. Follow project conventions and maintain existing patterns.",
			AllowedTools: []string{"search", "read", "edit", "write", "test", "exec"},
			MaxTurns:     20,
		},
		{
			Name:         RoleReviewer,
			SystemPrompt: "You are a code review agent. Analyze code for correctness, style, performance, and security issues. Provide actionable feedback with specific suggestions.",
			AllowedTools: []string{"search", "read", "diff"},
			MaxTurns:     10,
		},
		{
			Name:         RoleTester,
			SystemPrompt: "You are a testing agent. Write comprehensive tests, identify edge cases, and verify behavior. Focus on coverage, correctness, and failure modes.",
			AllowedTools: []string{"search", "read", "edit", "write", "test", "exec"},
			MaxTurns:     15,
		},
		{
			Name:         RoleDebugger,
			SystemPrompt: "You are a debugging agent. Investigate failures, trace execution paths, and identify root causes. Use systematic elimination and evidence-based reasoning.",
			AllowedTools: []string{"search", "read", "exec", "log"},
			MaxTurns:     10,
		},
		{
			Name:         RoleDocumenter,
			SystemPrompt: "You are a documentation agent. Write clear, accurate documentation for code, APIs, and processes. Focus on clarity, completeness, and usefulness for the target audience.",
			AllowedTools: []string{"search", "read", "write"},
			MaxTurns:     8,
		},
	}
}
