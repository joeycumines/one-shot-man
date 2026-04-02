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
		t.Fatal("AddRelativePath(\"\") should return an error, got nil")
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

// =============================================================================
// REGRESSION TESTS FOR CRITICAL BUG FIXES
// These tests verify fixes for 5 critical issues in the PR.
// =============================================================================

// TestIssue003_RemovePath_EmptyString_ShouldError verifies API symmetry:
// AddPath(""), AddRelativePath(""), and RefreshPath("") all reject empty strings.
// RemovePath("") should also reject empty strings for consistency.
func TestIssue003_RemovePath_EmptyString_ShouldError(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Verify that other methods reject empty strings
	err = cm.AddPath("")
	if err == nil {
		t.Error("AddPath(\"\") should return an error")
	}
	if err != nil && !strings.Contains(err.Error(), "empty path") {
		t.Errorf("AddPath(\"\") error should mention empty path, got: %v", err)
	}

	_, err = cm.AddRelativePath("")
	if err == nil {
		t.Error("AddRelativePath(\"\") should return an error")
	}
	if err != nil && !strings.Contains(err.Error(), "empty path") {
		t.Errorf("AddRelativePath(\"\") error should mention empty path, got: %v", err)
	}

	err = cm.RefreshPath("")
	if err == nil {
		t.Error("RefreshPath(\"\") should return an error")
	}
	if err != nil && !strings.Contains(err.Error(), "empty path") {
		t.Errorf("RefreshPath(\"\") error should mention empty path, got: %v", err)
	}

	// NOW: RemovePath("") should ALSO reject empty strings for consistency
	err = cm.RemovePath("")
	if err == nil {
		t.Error("RemovePath(\"\") should return an error (API asymmetry fix)")
	}
	if err != nil && !strings.Contains(err.Error(), "empty path") {
		t.Errorf("RemovePath(\"\") error should mention empty path, got: %v", err)
	}
}

// TestIssue005_AddPath_EmptyString_BreakingChange verifies that AddPath("")
// now returns an error instead of defaulting to CWD. This is a breaking change.
func TestIssue005_AddPath_EmptyString_BreakingChange(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	cm, err := NewContextManager(base)
	if err != nil {
		t.Fatalf("NewContextManager failed: %v", err)
	}

	// Verify that AddPath("") now returns an error instead of defaulting to CWD
	err = cm.AddPath("")
	if err == nil {
		t.Error("AddPath(\"\") should return an error (breaking change)")
	}
	if !strings.Contains(err.Error(), "empty path") {
		t.Errorf("AddPath(\"\") error should mention empty path, got: %v", err)
	}

	// Verify the correct way to add the ContextManager's base directory is to pass base explicitly
	err = cm.AddPath(base)
	if err != nil {
		t.Errorf("AddPath(base) should work (correct way to add base directory): %v", err)
	}

	// Verify "." is tracked as the root owner (base directory normalizes to ".")
	cp, ok := cm.GetPath(".")
	if !ok {
		t.Fatal("root owner '.' should be tracked after AddPath(base)")
	}
	if cp.Type != "directory" {
		t.Errorf("expected type 'directory', got %q", cp.Type)
	}
}
