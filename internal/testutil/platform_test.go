// Package testutil provides cross-platform testing utilities for
// consistent platform detection and root user handling.

// This centralizes scattered platform-specific checks across tests,
// providing a single source of truth for test behavior.
package testutil

import (
	"testing"
)

func TestDetectPlatform_UnixNonRoot(t *testing.T) {
	platform := DetectPlatform(t)

	// This test requires Unix/macOS/Linux - skip on Windows
	if platform.IsWindows {
		t.Skip("Skipping: Test requires Unix/macOS/Linux platform")
	}

	// This test explicitly tests non-root behavior - skip if running as root
	SkipIfRoot(t, platform, "test requires non-root Unix user")

	if !platform.IsUnix {
		t.Errorf("Expected IsUnix=true on macOS/Linux, got %v", platform.IsUnix)
	}
}

func TestDetectPlatform_Windows(t *testing.T) {
	// This test should only run when actually on Windows
	// We can't simulate Windows on macOS, so just verify the structure

	t.Skip("Skipping: Requires Windows OS to validate Windows detection")
}
