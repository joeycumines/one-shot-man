package command

import (
	"bytes"
	"flag"
	"fmt"
	"strings"
	"testing"
)

// stubGoalRegistry is a minimal GoalRegistry for mcp-instance tests.
type stubGoalRegistry struct {
	goals []Goal
}

func (r *stubGoalRegistry) List() []string {
	names := make([]string, len(r.goals))
	for i, g := range r.goals {
		names[i] = g.Name
	}
	return names
}

func (r *stubGoalRegistry) Get(name string) (*Goal, error) {
	for i := range r.goals {
		if r.goals[i].Name == name {
			return &r.goals[i], nil
		}
	}
	return nil, fmt.Errorf("goal not found: %s", name)
}

func (r *stubGoalRegistry) GetAllGoals() []Goal {
	return r.goals
}

func (r *stubGoalRegistry) Reload() error {
	return nil
}

func TestNewMCPInstanceCommand(t *testing.T) {
	t.Parallel()

	goals := &stubGoalRegistry{}
	cmd := NewMCPInstanceCommand(goals, "0.1.0-test")

	if cmd.Name() != "mcp-instance" {
		t.Errorf("Name() = %q, want %q", cmd.Name(), "mcp-instance")
	}
	if cmd.Description() == "" {
		t.Error("Description() is empty")
	}
	if !strings.Contains(cmd.Usage(), "mcp-instance") {
		t.Errorf("Usage() = %q, want to contain 'mcp-instance'", cmd.Usage())
	}
}

func TestMCPInstanceCommand_SetupFlags(t *testing.T) {
	t.Parallel()

	goals := &stubGoalRegistry{}
	cmd := NewMCPInstanceCommand(goals, "0.1.0-test")

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Parse with --session flag.
	if err := fs.Parse([]string{"--session", "test-session-123"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if cmd.session != "test-session-123" {
		t.Errorf("session = %q, want %q", cmd.session, "test-session-123")
	}
}

func TestMCPInstanceCommand_Execute_MissingSession(t *testing.T) {
	t.Parallel()

	goals := &stubGoalRegistry{}
	cmd := NewMCPInstanceCommand(goals, "0.1.0-test")

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing --session")
	}
	if !strings.Contains(err.Error(), "--session flag is required") {
		t.Errorf("error = %q, want to mention --session", err.Error())
	}
}

func TestMCPInstanceCommand_Execute_UnexpectedArgs(t *testing.T) {
	t.Parallel()

	goals := &stubGoalRegistry{}
	cmd := NewMCPInstanceCommand(goals, "0.1.0-test")
	cmd.session = "test-sess"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"extra-arg"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unexpected arguments")
	}
	if !strings.Contains(err.Error(), "unexpected arguments") {
		t.Errorf("error = %q, want to mention unexpected arguments", err.Error())
	}
}
