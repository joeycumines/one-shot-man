package claudemux

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// TestMultiAgent_RegisterAndQuery verifies agent registration, capability
// advertisement, and capability-based querying.
func TestMultiAgent_RegisterAndQuery(t *testing.T) {
	t.Parallel()

	reg := NewAgentRegistry()
	inst := &Instance{ID: "test-inst"}

	// Register agents with distinct capabilities.
	caps := []AgentCapabilities{
		{Tools: []string{"search", "edit"}, Models: []string{"claude-sonnet-4-20250514"}, Specialties: []string{"code-review"}, Streaming: true, MultiTurn: true},
		{Tools: []string{"search", "read"}, Models: []string{"claude-opus-4-20250514"}, Specialties: []string{"architecture"}, Streaming: true, MultiTurn: true},
		{Tools: []string{"test", "exec"}, Models: []string{"claude-sonnet-4-20250514"}, Specialties: []string{"testing"}, Streaming: false, MultiTurn: true},
	}
	ids := []string{"reviewer", "architect", "tester"}

	for i, id := range ids {
		if err := reg.Register(id, inst, caps[i]); err != nil {
			t.Fatalf("Register(%s): %v", id, err)
		}
	}

	// List returns all agents sorted.
	listed := reg.List()
	if len(listed) != 3 {
		t.Fatalf("List() = %d, want 3", len(listed))
	}

	// Query by tool.
	matches := reg.Query(CapabilityRequest{RequiredTools: []string{"search"}})
	if len(matches) != 2 {
		t.Fatalf("Query(search) = %d, want 2", len(matches))
	}

	// Query by specialty.
	matches = reg.Query(CapabilityRequest{RequiredSpecialties: []string{"testing"}})
	if len(matches) != 1 || matches[0] != "tester" {
		t.Fatalf("Query(testing) = %v, want [tester]", matches)
	}

	// Query by streaming requirement.
	matches = reg.Query(CapabilityRequest{NeedStreaming: true})
	if len(matches) != 2 {
		t.Fatalf("Query(streaming) = %d, want 2", len(matches))
	}

	// Query with impossible criteria.
	matches = reg.Query(CapabilityRequest{RequiredTools: []string{"magic"}})
	if len(matches) != 0 {
		t.Fatalf("Query(magic) = %d, want 0", len(matches))
	}

	// Unregister one agent.
	if err := reg.Unregister("tester"); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	if len(reg.List()) != 2 {
		t.Fatalf("List() after unregister = %d, want 2", len(reg.List()))
	}
}

// TestMultiAgent_CoordinationBus verifies message routing, broadcast, and
// lifecycle management on the coordination bus.
func TestMultiAgent_CoordinationBus(t *testing.T) {
	t.Parallel()

	bus := NewCoordinationBus(DefaultConfig())
	defer bus.Close()

	// Subscribe multiple agents.
	var received sync.Map // agentID -> []string

	for _, agentID := range []string{"a", "b", "c"} {
		bus.Subscribe(agentID, func(msg CoordinationMessage) {
			vals, _ := received.LoadOrStore(agentID, &[]string{})
			v := vals.(*[]string)
			*v = append(*v, string(msg.Payload))
		})
	}

	// Publish a broadcast message.
	if err := bus.Publish(CoordinationMessage{From: "coordinator", Topic: "task", Payload: []byte("work")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// All three subscribers should receive.
	for _, agentID := range []string{"a", "b", "c"} {
		vals, ok := received.Load(agentID)
		if !ok {
			t.Fatalf("Agent %s received no messages", agentID)
		}
		if len(*vals.(*[]string)) != 1 || (*vals.(*[]string))[0] != "work" {
			t.Errorf("Agent %s payload = %v, want [work]", agentID, vals)
		}
	}

	// Publish after unsubscribe — should skip that agent.
	bus.Unsubscribe("b")
	if err := bus.Publish(CoordinationMessage{From: "coordinator", Topic: "task", Payload: []byte("more")}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	aVals, _ := received.Load("a")
	cVals, _ := received.Load("c")
	bVals, bOk := received.Load("b")

	if len(*aVals.(*[]string)) != 2 || len(*cVals.(*[]string)) != 2 {
		t.Error("a and c should have 2 messages each")
	}
	if bOk && len(*bVals.(*[]string)) != 1 {
		t.Errorf("b should have 1 message, got %d", len(*bVals.(*[]string)))
	}

	// Publish after close should error.
	bus.Close()
	err := bus.Publish(CoordinationMessage{From: "late", Topic: "x", Payload: []byte("fail")})
	if !errors.Is(err, ErrBusClosed) {
		t.Errorf("Publish after close = %v, want ErrBusClosed", err)
	}
}

// TestMultiAgent_RoleDelegation verifies role registration, assignment, and
// task delegation through the role registry and agent registry.
func TestMultiAgent_RoleDelegation(t *testing.T) {
	t.Parallel()

	registry := NewAgentRegistry()

	// Register agents.
	inst := &Instance{ID: "inst"}
	for _, pair := range [][2]string{
		{"planner-1", RolePlanner},
		{"coder-1", RoleCoder},
		{"coder-2", RoleCoder},
		{"reviewer-1", RoleReviewer},
	} {
		if err := registry.Register(pair[0], inst, AgentCapabilities{
			Tools:     []string{"search", "read", "edit"},
			Streaming: true,
		}); err != nil {
			t.Fatalf("Register(%s): %v", pair[0], err)
		}
		if err := registry.AssignRole(pair[0], pair[1]); err != nil {
			t.Fatalf("AssignRole(%s, %s): %v", pair[0], pair[1], err)
		}
	}

	// Verify roles are assigned (only one agent per role due to overwriting).
	agents := registry.GetByRole(RoleCoder)
	if len(agents) != 1 || agents[0] != "coder-2" {
		t.Fatalf("GetByRole(coder) = %v, want [coder-2] (last assignment wins)", agents)
	}
	// planner should still be assigned.
	agents = registry.GetByRole(RolePlanner)
	if len(agents) != 1 || agents[0] != "planner-1" {
		t.Fatalf("GetByRole(planner) = %v, want [planner-1]", agents)
	}

	// Delegate a task to the coder role.
	result, err := DelegateTask(TaskRequest{
		TaskID:      "task-1",
		Role:        RoleCoder,
		Description: "implement feature X",
	}, registry)
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want completed", result.Status)
	}
	if result.AgentID == "" {
		t.Error("AgentID should be set")
	}

	// Delegate to a non-existent role.
	_, err = DelegateTask(TaskRequest{TaskID: "x", Role: "nonexistent"}, registry)
	if errors.Is(err, ErrRoleNotFound) {
		// Good.
	} else if err == nil {
		t.Fatal("expected error for nonexistent role")
	}

	// Assign a different role to an existing agent.
	if err := registry.AssignRole("planner-1", RolePlanner); err != nil {
		t.Fatalf("AssignRole planner-1: %v", err)
	}
	// planner-1 should now be the only one with RolePlanner.
	agents = registry.GetByRole(RolePlanner)
	if len(agents) != 1 || agents[0] != "planner-1" {
		t.Errorf("GetByRole(planner) = %v, want [planner-1]", agents)
	}
}

// TestMultiAgent_PanelWithAgents verifies MultiAgentPanel integration:
// adding agents, subscribing to output, snapshotting, and dashboard rendering.
func TestMultiAgent_PanelWithAgents(t *testing.T) {
	t.Parallel()

	panel := NewPanel(DefaultPanelConfig())
	if err := panel.Start(); err != nil {
		t.Fatalf("panel.Start: %v", err)
	}

	bus := NewCoordinationBus(DefaultConfig())
	defer bus.Close()

	reg := NewAgentRegistry()
	mpanel := NewMultiAgentPanel(panel, bus, reg)

	// Add agents to the panel.
	for _, agent := range []struct{ id, title, role string }{
		{"planner-1", "Planner", RolePlanner},
		{"coder-1", "Coder", RoleCoder},
	} {
		if err := mpanel.AddAgent(agent.id, agent.title, agent.role); err != nil {
			t.Fatalf("AddAgent(%s): %v", agent.id, err)
		}
	}

	// Verify the panel has panes.
	if panel.PaneCount() != 2 {
		t.Fatalf("PaneCount = %d, want 2", panel.PaneCount())
	}

	// Subscribe to agent output via the bus.
	outputCh := mpanel.SubscribeToAgent("planner-1")
	done := make(chan struct{})
	go func() {
		<-outputCh
		close(done)
	}()

	// Publish a message that the subscription will receive.
	if err := bus.Publish(CoordinationMessage{
		From:    "planner-1",
		Topic:   "output",
		Payload: []byte("planning complete"),
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for agent output message")
	}

	// Append output directly to pane.
	_ = panel.AppendOutput("planner-1", "step 1")
	_ = panel.AppendOutput("planner-1", "step 2")

	// Toggle dashboard and snapshot.
	mpanel.ToggleDashboard()
	snap := mpanel.Snapshot()

	if len(snap.Panes) != 2 {
		t.Fatalf("Snapshot panes = %d, want 2", len(snap.Panes))
	}

	// Remove an agent.
	if err := mpanel.RemoveAgent("coder-1"); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}

	if panel.PaneCount() != 1 {
		t.Fatalf("PaneCount after remove = %d, want 1", panel.PaneCount())
	}

	// Adding a duplicate should fail.
	if err := mpanel.AddAgent("planner-1", "Dup", RolePlanner); err == nil {
		t.Fatal("expected error for duplicate agent")
	}
}

// TestMultiAgent_FullPipeline tests the end-to-end multi-agent workflow:
// create registry + role registry + bus + panel, register agents with roles,
// publish coordination messages, and collect results.
func TestMultiAgent_FullPipeline(t *testing.T) {
	t.Parallel()

	// 1. Create infrastructure.
	reg := NewAgentRegistry()
	bus := NewCoordinationBus(DefaultConfig())
	defer bus.Close()

	panel := NewPanel(DefaultPanelConfig())
	if err := panel.Start(); err != nil {
		t.Fatalf("panel.Start: %v", err)
	}

	mpanel := NewMultiAgentPanel(panel, bus, reg)

	// 2. Register agents and assign roles.
	inst := &Instance{ID: "pipeline-inst"}
	type agentDef struct {
		id     string
		role   string
		caps   AgentCapabilities
		output string
	}
	agents := []agentDef{
		{"planner", RolePlanner, AgentCapabilities{Tools: []string{"search", "read"}, Streaming: true}, "plan done"},
		{"coder", RoleCoder, AgentCapabilities{Tools: []string{"search", "edit", "write"}, Streaming: true}, "code done"},
		{"reviewer", RoleReviewer, AgentCapabilities{Tools: []string{"search", "read", "diff"}, Streaming: true}, "review done"},
	}

	for _, a := range agents {
		if err := reg.Register(a.id, inst, a.caps); err != nil {
			t.Fatalf("Register(%s): %v", a.id, err)
		}
		if err := reg.AssignRole(a.id, a.role); err != nil {
			t.Fatalf("AssignRole(%s, %s): %v", a.id, a.role, err)
		}
	}

	// 3. Add all agents to the multi-agent panel.
	for _, a := range agents {
		if err := mpanel.AddAgent(a.id, a.id+" agent", a.role); err != nil {
			t.Fatalf("AddAgent(%s): %v", a.id, err)
		}
	}

	if reg.List()[0] == "" || reg.List()[1] == "" || reg.List()[2] == "" {
		t.Fatal("expected 3 registered agents")
	}

	// 4. Subscribe to each agent and collect output.
	// Each goroutine filters messages to only accept those from its agent.
	results := make(map[string]string)
	var mu sync.Mutex
	done := make(chan struct{}, len(agents))

	for _, a := range agents {
		ch := mpanel.SubscribeToAgent(a.id)
		go func(agentID, output string) {
			for msg := range ch {
				if msg.AgentID == agentID {
					mu.Lock()
					results[agentID] = msg.Line
					mu.Unlock()
					done <- struct{}{}
					return
				}
			}
		}(a.id, a.output)
	}

	// 5. Publish coordination messages — the bus delivers to all subscribers.
	for _, a := range agents {
		if err := bus.Publish(CoordinationMessage{
			From:    a.id,
			Topic:   "output",
			Payload: []byte(a.output),
		}); err != nil {
			t.Fatalf("Publish(%s): %v", a.id, err)
		}
	}

	// Wait for all agents to receive their messages.
	go func() {
		for range agents {
			<-done
		}
	}()

	// 6. Verify results.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	for _, a := range agents {
		if got, ok := results[a.id]; !ok || got != a.output {
			t.Errorf("Agent %s: got %q, want %q", a.id, got, a.output)
		}
	}
	mu.Unlock()

	// 7. Verify panel snapshot has all panes.
	snap := mpanel.Snapshot()
	if len(snap.Panes) != 3 {
		t.Fatalf("Snapshot panes = %d, want 3", len(snap.Panes))
	}

	// 8. Delegate a task and verify.
	result, err := DelegateTask(TaskRequest{
		TaskID:      "pipeline-task",
		Role:        RoleCoder,
		Description: "full pipeline test",
	}, reg)
	if err != nil {
		t.Fatalf("DelegateTask: %v", err)
	}
	if result.Status != "completed" || result.TaskID != "pipeline-task" {
		t.Errorf("TaskResult = %+v, want completed pipeline-task", result)
	}

	// 9. Remove one agent and verify.
	if err := mpanel.RemoveAgent("reviewer"); err != nil {
		t.Fatalf("RemoveAgent: %v", err)
	}
	if panel.PaneCount() != 2 {
		t.Fatalf("PaneCount after remove = %d, want 2", panel.PaneCount())
	}

	// 10. Clean up panel and bus.
	panel.Close()
}
