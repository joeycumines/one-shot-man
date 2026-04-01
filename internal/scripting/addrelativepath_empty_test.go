package scripting

import (
	"strings"
	"testing"
)

// TestAddRelativePath_EmptyString_ReturnsError verifies that AddRelativePath
// rejects empty strings, consistent with AddPath and RefreshPath behavior.
// This prevents silent root context mutation during session rehydration
// if a session file becomes corrupted with an empty-string owner path.
func TestAddRelativePath_EmptyString_ReturnsError(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Track the root directory to test that empty strings don't mutate it
	if err := cm.AddPath(base); err != nil {
		t.Fatalf("AddPath failed: %v", err)
	}

	// Get the initial state of the root owner
	rootOwner := "."
	cpBefore, ok := cm.GetPath(rootOwner)
	if !ok {
		t.Fatal("root owner should be tracked")
	}
	updateTimeBefore := cpBefore.UpdateTime

	// Try to add with empty string - should fail
	owner, err := cm.AddRelativePath("")
	if err == nil {
		t.Error("AddRelativePath(\"\") should return an error, got nil")
	}
	if !strings.Contains(err.Error(), "empty path is not valid") {
		t.Errorf("AddRelativePath(\"\") error should mention empty path, got: %v", err)
	}
	if owner != "" {
		t.Errorf("AddRelativePath(\"\") should return empty owner on error, got: %q", owner)
	}

	// Verify that the root owner was not mutated
	cpAfter, ok := cm.GetPath(rootOwner)
	if !ok {
		t.Fatal("root owner should still be tracked after AddRelativePath(\"\")")
	}
	if !cpAfter.UpdateTime.Equal(updateTimeBefore) {
		t.Error("root owner UpdateTime should not have changed after AddRelativePath(\"\")")
	}
}
