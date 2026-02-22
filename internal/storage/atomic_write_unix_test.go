//go:build !windows

package storage

import "testing"

func TestAtomicRenameWindows_Stub(t *testing.T) {
	t.Parallel()
	err := atomicRenameWindows("old", "new")
	if err == nil {
		t.Fatal("expected error from non-Windows stub")
	}
	if got := err.Error(); got != "atomicRenameWindows called on non-Windows platform" {
		t.Fatalf("unexpected error: %q", got)
	}
}
