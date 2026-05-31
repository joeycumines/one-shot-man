package claudemux

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRoleRegistry_RegisterAndGet(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry()

	role := &AgentRole{
		Name:           "custom-role",
		SystemPrompt:   "You are a custom agent.",
		AllowedTools:   []string{"search", "read"},
		ForbiddenTools: []string{"write"},
		OutputFormat:   "json",
		MaxTurns:       10,
	}
	reg.RegisterRole(role)

	got, err := reg.GetRole("custom-role")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if got.Name != "custom-role" {
		t.Errorf("Name = %q, want %q", got.Name, "custom-role")
	}
	if got.SystemPrompt != "You are a custom agent." {
		t.Errorf("SystemPrompt = %q, want custom prompt", got.SystemPrompt)
	}
	if len(got.AllowedTools) != 2 {
		t.Errorf("len(AllowedTools) = %d, want 2", len(got.AllowedTools))
	}
	if len(got.ForbiddenTools) != 1 {
		t.Errorf("len(ForbiddenTools) = %d, want 1", len(got.ForbiddenTools))
	}
	if got.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want json", got.OutputFormat)
	}
	if got.MaxTurns != 10 {
		t.Errorf("MaxTurns = %d, want 10", got.MaxTurns)
	}

	// Overwrite existing role.
	reg.RegisterRole(&AgentRole{
		Name:         "custom-role",
		SystemPrompt: "Updated prompt",
		MaxTurns:     5,
	})
	got, err = reg.GetRole("custom-role")
	if err != nil {
		t.Fatalf("GetRole after overwrite: %v", err)
	}
	if got.SystemPrompt != "Updated prompt" {
		t.Errorf("SystemPrompt after overwrite = %q, want Updated prompt", got.SystemPrompt)
	}
}

func TestRoleRegistry_PredefinedRoles(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry()

	expected := []string{RolePlanner, RoleCoder, RoleReviewer, RoleTester, RoleDebugger, RoleDocumenter}
	for _, name := range expected {
		role, err := reg.GetRole(name)
		if err != nil {
			t.Errorf("GetRole(%q): %v", name, err)
			continue
		}
		if role.Name != name {
			t.Errorf("Name = %q, want %q", role.Name, name)
		}
		if role.SystemPrompt == "" {
			t.Errorf("role %q has empty SystemPrompt", name)
		}
		if role.MaxTurns <= 0 {
			t.Errorf("role %q has MaxTurns = %d, want > 0", name, role.MaxTurns)
		}
	}
}

func TestRoleRegistry_DelegateTask(t *testing.T) {
	t.Parallel()

	agentReg := NewAgentRegistry()
	inst := &Instance{ID: "inst-1"}

	if err := agentReg.Register("agent-coder", inst, AgentCapabilities{
		Tools: []string{"search", "read", "edit"},
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := agentReg.AssignRole("agent-coder", RoleCoder); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	req := TaskRequest{
		TaskID:      "task-1",
		Role:        RoleCoder,
		Description: "Implement feature X",
		Context:     map[string]any{"priority": "high"},
		Deadline:    time.Now().Add(10 * time.Minute),
	}

	result, err := DelegateTask(req, agentReg)
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if result.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want task-1", result.TaskID)
	}
	if result.AgentID != "agent-coder" {
		t.Errorf("AgentID = %q, want agent-coder", result.AgentID)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want completed", result.Status)
	}
}

func TestRoleRegistry_Concurrent(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry()
	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			role := &AgentRole{
				Name:         "concurrent-role",
				SystemPrompt: "concurrent",
				MaxTurns:     i + 1,
			}
			reg.RegisterRole(role)
		}()
	}
	wg.Wait()

	got, err := reg.GetRole("concurrent-role")
	if err != nil {
		t.Fatalf("GetRole after concurrent writes: %v", err)
	}
	if got.Name != "concurrent-role" {
		t.Errorf("Name = %q, want concurrent-role", got.Name)
	}

	for i := range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = reg.GetRole(RoleCoder)
			_, _ = reg.GetRole(RoleReviewer)
			_ = i
		}()
	}
	wg.Wait()
}

func TestRoleRegistry_MissingRole(t *testing.T) {
	t.Parallel()

	reg := NewRoleRegistry()

	_, err := reg.GetRole("nonexistent")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("GetRole(nonexistent) error = %v, want ErrRoleNotFound", err)
	}

	// DelegateTask with missing role.
	agentReg := NewAgentRegistry()
	_, err = DelegateTask(TaskRequest{
		TaskID: "task-missing",
		Role:   "nonexistent",
	}, agentReg)
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("DelegateTask with missing role error = %v, want ErrRoleNotFound", err)
	}

	// DelegateTask with valid role but no agents.
	_, err = DelegateTask(TaskRequest{
		TaskID: "task-no-agents",
		Role:   RoleCoder,
	}, agentReg)
	if !errors.Is(err, ErrNoAgentsForRole) {
		t.Errorf("DelegateTask with no agents error = %v, want ErrNoAgentsForRole", err)
	}
}

func TestCreateRole(t *testing.T) {
	t.Parallel()

	config := RoleConfig{
		Name:           "custom",
		SystemPrompt:   "You are custom.",
		AllowedTools:   []string{"read"},
		ForbiddenTools: []string{"write"},
		OutputFormat:   "text",
		MaxTurns:       7,
	}

	role := CreateRole(config)
	if role.Name != "custom" {
		t.Errorf("Name = %q, want custom", role.Name)
	}
	if role.SystemPrompt != "You are custom." {
		t.Errorf("SystemPrompt = %q, want custom prompt", role.SystemPrompt)
	}
	if len(role.AllowedTools) != 1 || role.AllowedTools[0] != "read" {
		t.Errorf("AllowedTools = %v, want [read]", role.AllowedTools)
	}
	if len(role.ForbiddenTools) != 1 || role.ForbiddenTools[0] != "write" {
		t.Errorf("ForbiddenTools = %v, want [write]", role.ForbiddenTools)
	}
	if role.OutputFormat != "text" {
		t.Errorf("OutputFormat = %q, want text", role.OutputFormat)
	}
	if role.MaxTurns != 7 {
		t.Errorf("MaxTurns = %d, want 7", role.MaxTurns)
	}
}
