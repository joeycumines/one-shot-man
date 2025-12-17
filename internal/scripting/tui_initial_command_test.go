package scripting

import (
	"bytes"
	"context"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/testutil"
)

func TestGetInitialCommand_WithActiveMode(t *testing.T) {
	engine := &Engine{vm: nil, ctx: context.Background(), stdout: nil, stderr: nil, globals: make(map[string]interface{})}
	var out bytes.Buffer
	tm := NewTUIManagerWithConfig(context.Background(), engine, nil, &out, testutil.NewTestSessionID("scripting", t.Name()), "memory")

	if err := tm.RegisterMode(&ScriptMode{Name: "m1", InitialCommand: "doit"}); err != nil {
		t.Fatalf("RegisterMode failed: %v", err)
	}

	if err := tm.SwitchMode("m1"); err != nil {
		t.Fatalf("SwitchMode failed: %v", err)
	}

	expected := "doit"
	if got := tm.getInitialCommand(); got != expected {
		t.Fatalf("expected initial command %q, got %q", expected, got)
	}
}

func TestGetInitialCommand_NoActiveMode(t *testing.T) {
	engine := &Engine{vm: nil, ctx: context.Background(), stdout: nil, stderr: nil, globals: make(map[string]interface{})}
	var out bytes.Buffer
	tm := NewTUIManagerWithConfig(context.Background(), engine, nil, &out, testutil.NewTestSessionID("scripting", t.Name()), "memory")

	if got := tm.getInitialCommand(); got != "" {
		t.Fatalf("expected empty initial command when no mode active, got %q", got)
	}
}
