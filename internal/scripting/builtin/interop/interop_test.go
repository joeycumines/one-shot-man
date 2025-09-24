package interop

import (
	"context"
	"encoding/json"
	"testing"
)

func TestBasicInteropFunctionality(t *testing.T) {
	ctx := context.Background()
	manager := NewInteropManager(ctx)
	
	// Create test data
	testContext := &SharedContext{
		Version:    "1.0",
		SourceMode: "test",
		ContextItems: []ContextItem{
			{ID: 1, Type: "file", Label: "test.go", Payload: "package main"},
			{ID: 2, Type: "note", Label: "Test note", Payload: "This is a test"},
		},
	}
	
	// Test save
	err := manager.SaveSharedContext(testContext)
	if err != nil {
		t.Fatalf("Failed to save context: %v", err)
	}
	
	// Test load
	loaded, err := manager.LoadSharedContext()
	if err != nil {
		t.Fatalf("Failed to load context: %v", err)
	}
	
	if len(loaded.ContextItems) != 2 {
		t.Errorf("Expected 2 context items, got %d", len(loaded.ContextItems))
	}
	
	// Test commit message generation
	commitData := generateCommitMessage(loaded.ContextItems, nil)
	if commitData.Subject == "" {
		t.Error("Commit message subject is empty")
	}
	
	if commitData.Type == "" {
		t.Error("Commit message type is empty")
	}
	
	// Print for visual verification
	commitJSON, _ := json.MarshalIndent(commitData, "", "  ")
	t.Logf("Generated commit data: %s", commitJSON)
	
	// Clean up
	err = manager.DeleteSharedContext()
	if err != nil {
		t.Errorf("Failed to delete shared context: %v", err)
	}
}