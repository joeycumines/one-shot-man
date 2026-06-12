package claudemux

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestAgentRegistry_RegisterUnregister(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()

	// Register an agent.
	inst := &Instance{ID: "inst-1"}
	caps := AgentCapabilities{
		Tools:       []string{"search", "edit"},
		Models:      []string{"claude-sonnet-4-20250514"},
		Specialties: []string{"code-review"},
		Streaming:   true,
		MultiTurn:   true,
	}

	if err := reg.Register("agent-1", inst, caps); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Verify it appears in List.
	ids := reg.List()
	if len(ids) != 1 || ids[0] != "agent-1" {
		t.Errorf("List() = %v, want [agent-1]", ids)
	}

	// Duplicate registration should fail.
	if err := reg.Register("agent-1", inst, caps); !errors.Is(err, ErrAgentExists) {
		t.Errorf("duplicate Register error = %v, want ErrAgentExists", err)
	}

	// Register a second agent.
	if err := reg.Register("agent-2", inst, caps); err != nil {
		t.Fatalf("Register agent-2: %v", err)
	}

	ids = reg.List()
	if len(ids) != 2 {
		t.Errorf("List() len = %d, want 2", len(ids))
	}

	// Unregister the first agent.
	if err := reg.Unregister("agent-1"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}

	ids = reg.List()
	if len(ids) != 1 || ids[0] != "agent-2" {
		t.Errorf("List() after unregister = %v, want [agent-2]", ids)
	}

	// Unregister non-existent agent should fail.
	if err := reg.Unregister("agent-1"); !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("Unregister missing error = %v, want ErrAgentNotFound", err)
	}
}

func TestAgentRegistry_Query(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()
	inst := &Instance{ID: "inst"}

	// Register agents with different capabilities.
	if err := reg.Register("alpha", inst, AgentCapabilities{
		Tools:       []string{"search", "edit"},
		Models:      []string{"claude-sonnet-4-20250514"},
		Specialties: []string{"code-review"},
		Streaming:   true,
		MultiTurn:   true,
	}); err != nil {
		t.Fatalf("Register alpha: %v", err)
	}

	if err := reg.Register("beta", inst, AgentCapabilities{
		Tools:       []string{"search"},
		Models:      []string{"claude-sonnet-4-20250514", "gpt-4"},
		Specialties: []string{"security"},
		Streaming:   false,
		MultiTurn:   true,
	}); err != nil {
		t.Fatalf("Register beta: %v", err)
	}

	if err := reg.Register("gamma", inst, AgentCapabilities{
		Tools:       []string{"edit", "format"},
		Models:      []string{"gpt-4"},
		Specialties: []string{"code-review", "security"},
		Streaming:   true,
		MultiTurn:   false,
	}); err != nil {
		t.Fatalf("Register gamma: %v", err)
	}

	// Query: need "search" tool.
	matches := reg.Query(CapabilityRequest{RequiredTools: []string{"search"}})
	if len(matches) != 2 || matches[0] != "alpha" || matches[1] != "beta" {
		t.Errorf("Query(search) = %v, want [alpha beta]", matches)
	}

	// Query: need streaming.
	matches = reg.Query(CapabilityRequest{NeedStreaming: true})
	if len(matches) != 2 || matches[0] != "alpha" || matches[1] != "gamma" {
		t.Errorf("Query(streaming) = %v, want [alpha gamma]", matches)
	}

	// Query: need "security" specialty and streaming.
	matches = reg.Query(CapabilityRequest{
		RequiredSpecialties: []string{"security"},
		NeedStreaming:       true,
	})
	if len(matches) != 1 || matches[0] != "gamma" {
		t.Errorf("Query(security+streaming) = %v, want [gamma]", matches)
	}

	// Query: need multi-turn and "gpt-4" model.
	matches = reg.Query(CapabilityRequest{
		RequiredModels: []string{"gpt-4"},
		NeedMultiTurn:  true,
	})
	if len(matches) != 1 || matches[0] != "beta" {
		t.Errorf("Query(gpt-4+multiturn) = %v, want [beta]", matches)
	}

	// Query: impossible combination.
	matches = reg.Query(CapabilityRequest{
		RequiredTools: []string{"nonexistent"},
	})
	if len(matches) != 0 {
		t.Errorf("Query(nonexistent) = %v, want []", matches)
	}

	// Query: empty request matches all.
	matches = reg.Query(CapabilityRequest{})
	if len(matches) != 3 {
		t.Errorf("Query(empty) = %v, want 3 matches", matches)
	}
}

func TestAgentRegistry_Select(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()
	inst := &Instance{ID: "inst"}
	now := time.Now()

	// Register two agents with same capabilities.
	if err := reg.Register("agent-a", inst, AgentCapabilities{
		Tools:     []string{"search"},
		Streaming: true,
	}); err != nil {
		t.Fatalf("Register agent-a: %v", err)
	}

	if err := reg.Register("agent-b", inst, AgentCapabilities{
		Tools:     []string{"search"},
		Streaming: true,
	}); err != nil {
		t.Fatalf("Register agent-b: %v", err)
	}

	// Both have default health (0 errors). Select should be deterministic.
	agent, err := reg.Select(CapabilityRequest{RequiredTools: []string{"search"}})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	// Deterministic: alphabetical tiebreak.
	if agent.ID != "agent-a" {
		t.Errorf("Select() = %q, want agent-a (alphabetical tiebreak)", agent.ID)
	}

	// Give agent-b better health (fewer errors).
	if err := reg.UpdateHealth("agent-b", PaneHealth{
		State:      "running",
		ErrorCount: 0,
		LastUpdate: now,
	}); err != nil {
		t.Fatalf("UpdateHealth agent-b: %v", err)
	}

	// Give agent-a worse health (more errors).
	if err := reg.UpdateHealth("agent-a", PaneHealth{
		State:      "running",
		ErrorCount: 5,
		LastUpdate: now,
	}); err != nil {
		t.Fatalf("UpdateHealth agent-a: %v", err)
	}

	agent, err = reg.Select(CapabilityRequest{RequiredTools: []string{"search"}})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if agent.ID != "agent-b" {
		t.Errorf("Select() = %q, want agent-b (fewer errors)", agent.ID)
	}

	// No matching agents.
	_, err = reg.Select(CapabilityRequest{RequiredTools: []string{"nonexistent"}})
	if !errors.Is(err, ErrNoMatchingAgents) {
		t.Errorf("Select(nonexistent) error = %v, want ErrNoMatchingAgents", err)
	}
}

func TestAgentRegistry_AssignRole(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()
	inst := &Instance{ID: "inst"}

	if err := reg.Register("agent-1", inst, AgentCapabilities{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := reg.Register("agent-2", inst, AgentCapabilities{}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Assign role to agent-1.
	if err := reg.AssignRole("agent-1", "reviewer"); err != nil {
		t.Fatalf("AssignRole: %v", err)
	}

	// Retrieve by role.
	ids := reg.GetByRole("reviewer")
	if len(ids) != 1 || ids[0] != "agent-1" {
		t.Errorf("GetByRole(reviewer) = %v, want [agent-1]", ids)
	}

	// Reassign the role to agent-2 (should remove from agent-1).
	if err := reg.AssignRole("agent-2", "reviewer"); err != nil {
		t.Fatalf("AssignRole agent-2: %v", err)
	}

	ids = reg.GetByRole("reviewer")
	if len(ids) != 1 || ids[0] != "agent-2" {
		t.Errorf("GetByRole(reviewer) after reassign = %v, want [agent-2]", ids)
	}

	// Non-existent role.
	ids = reg.GetByRole("nonexistent")
	if len(ids) != 0 {
		t.Errorf("GetByRole(nonexistent) = %v, want []", ids)
	}

	// Assign role to non-existent agent.
	if err := reg.AssignRole("agent-3", "reviewer"); !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("AssignRole missing agent error = %v, want ErrAgentNotFound", err)
	}

	// Assign a different role to agent-2 (should remove old role).
	if err := reg.AssignRole("agent-2", "builder"); err != nil {
		t.Fatalf("AssignRole builder: %v", err)
	}

	// Old role should be gone.
	ids = reg.GetByRole("reviewer")
	if len(ids) != 0 {
		t.Errorf("GetByRole(reviewer) after role change = %v, want []", ids)
	}

	// New role should work.
	ids = reg.GetByRole("builder")
	if len(ids) != 1 || ids[0] != "agent-2" {
		t.Errorf("GetByRole(builder) = %v, want [agent-2]", ids)
	}
}

func TestAgentRegistry_Concurrent(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()
	inst := &Instance{ID: "inst"}
	caps := AgentCapabilities{Tools: []string{"search"}, Streaming: true}

	var wg sync.WaitGroup

	// Concurrent registrations.
	for i := range 20 {
		wg.Go(func() {
			_ = reg.Register(fmt.Sprintf("agent-%d", i), inst, caps)
		})
	}
	wg.Wait()

	// All 20 should be registered.
	if len(reg.List()) != 20 {
		t.Errorf("List() len = %d, want 20", len(reg.List()))
	}

	// Concurrent queries and selects.
	for range 50 {
		wg.Go(func() {
			_ = reg.Query(CapabilityRequest{RequiredTools: []string{"search"}})
			_, _ = reg.Select(CapabilityRequest{RequiredTools: []string{"search"}})
		})
	}
	wg.Wait()

	// Concurrent unregistrations.
	for i := range 20 {
		wg.Go(func() {
			_ = reg.Unregister(fmt.Sprintf("agent-%d", i))
		})
	}
	wg.Wait()

	if len(reg.List()) != 0 {
		t.Errorf("List() after unregister = %v, want empty", reg.List())
	}
}

func TestAgentRegistry_UpdateHealth(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()
	inst := &Instance{ID: "inst"}
	now := time.Now()

	if err := reg.Register("agent-1", inst, AgentCapabilities{
		Tools:     []string{"search"},
		Streaming: true,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := reg.Register("agent-2", inst, AgentCapabilities{
		Tools:     []string{"search"},
		Streaming: true,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Both start with default health (0 errors). Set agent-1 LastUsed
	// so it would normally be preferred, then give agent-2 better health.
	reg.agents["agent-1"].SetLastUsed(now)

	if err := reg.UpdateHealth("agent-2", PaneHealth{
		State:      "running",
		ErrorCount: 0,
		TaskCount:  10,
		LastUpdate: now,
	}); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}

	// Give agent-1 worse health.
	if err := reg.UpdateHealth("agent-1", PaneHealth{
		State:      "error",
		ErrorCount: 3,
		TaskCount:  5,
		LastUpdate: now,
	}); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}

	// Select should prefer agent-2 (fewer errors).
	agent, err := reg.Select(CapabilityRequest{RequiredTools: []string{"search"}})
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if agent.ID != "agent-2" {
		t.Errorf("Select() = %q, want agent-2 (fewer errors)", agent.ID)
	}

	// UpdateHealth on non-existent agent should fail.
	if err := reg.UpdateHealth("agent-3", PaneHealth{}); !errors.Is(err, ErrAgentNotFound) {
		t.Errorf("UpdateHealth missing error = %v, want ErrAgentNotFound", err)
	}

	// Verify health is reflected in GetHealth.
	health := reg.agents["agent-1"].GetHealth()
	if health.ErrorCount != 3 {
		t.Errorf("GetHealth().ErrorCount = %d, want 3", health.ErrorCount)
	}
	if health.State != "error" {
		t.Errorf("GetHealth().State = %q, want error", health.State)
	}
}
