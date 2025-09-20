package scripting

import (
	"context"
	"os"
	"testing"
)

// TestPromptModeSelection tests that the correct prompt mode is selected
func TestPromptModeSelection(t *testing.T) {
	engine := NewEngine(context.Background(), os.Stdout, os.Stderr)
	engine.SetTestMode(true)
	
	tuiManager := engine.GetTUIManager()
	
	// In test mode, should use simple loop
	if !tuiManager.shouldUseSimpleLoop() {
		t.Error("Expected to use simple loop in test mode, but go-prompt was selected")
	}
	
	// Test with test mode disabled
	engine.SetTestMode(false)
	
	// Should still use simple loop due to non-terminal environment in tests
	if !tuiManager.shouldUseSimpleLoop() {
		t.Log("Note: Still using simple loop due to non-terminal environment (expected in CI)")
	}
}