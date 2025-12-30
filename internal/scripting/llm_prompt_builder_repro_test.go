package scripting

import (
	"context"
	"path/filepath"
	"testing"
)

// Reproduction test: ensure 'new' then 'template' commands are available and succeed
func TestLLMPromptBuilderCommands(t *testing.T) {
	ctx := context.Background()
	engine := mustNewEngine(t, ctx, nil, nil)
	scriptPath := filepath.Join("..", "..", "scripts", "example-01-llm-prompt-builder.js")
	script, err := engine.LoadScript("llm-prompt-builder", scriptPath)
	if err != nil {
		t.Fatalf("Failed to load script: %v", err)
	}
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to execute script: %v", err)
	}

	tuiManager := engine.GetTUIManager()
	if err := tuiManager.SwitchMode("llm-prompt-builder"); err != nil {
		t.Fatalf("SwitchMode failed: %v", err)
	}

	// Create a new prompt
	if err := tuiManager.ExecuteCommand("new", []string{"customer-service", "A customer service assistant prompt"}); err != nil {
		t.Fatalf("Failed to execute 'new' command: %v", err)
	}

	// Set template
	tpl := "You are a {{role}} for {{company}}. You should be {{tone}} and {{helpfulLevel}}. Customer issue: {{issue}}"
	if err := tuiManager.ExecuteCommand("template", []string{tpl}); err != nil {
		t.Fatalf("Failed to execute 'template' command: %v", err)
	}
}
