// Package testutil provides cross-platform testing utilities for
// consistent platform detection and root user handling.

// This centralizes scattered platform-specific checks across tests,
// providing a single source of truth for test behavior.
package testutil

import (
	"os"
	"runtime"
	"testing"
)

// Platform captures the current test execution environment.
// It provides boolean flags for common platform checks.
type Platform struct {
	IsUnix    bool
	IsWindows bool
	IsRoot    bool
	UID       int
	GID       int
}

// DetectPlatform inspects the current runtime environment
// and returns a Platform struct with detection results.
//
// Example usage:
//
//	platform := DetectPlatform(t)
//	if platform.IsRoot {
//		t.Skip("Skipping test - requires non-root environment")
//	}
//	if platform.IsWindows {
//		t.Skip("Unix-only test")
//	}
func DetectPlatform(t *testing.T) Platform {
	uid := os.Geteuid()
	gid := os.Getgid()

	platform := Platform{
		IsUnix:    runtime.GOOS != "windows",
		IsWindows: runtime.GOOS == "windows",
		IsRoot:    uid == 0,
		UID:       uid,
		GID:       gid,
	}

	t.Logf("Platform detection: OS=%s, UID=%d, GID=%d, IsRoot=%v",
		runtime.GOOS, uid, gid, platform.IsRoot)

	return platform
}

// SkipIfRoot marks the test as skipped if running as root user.
// This is useful for tests that simulate permission restrictions
// which don't work correctly when root bypasses chmod.
//
// Example:
//
//	platform := DetectPlatform(t)
//	if platform.IsRoot {
//		SkipIfRoot(t, "chmod-based failure simulation doesn't work for root")
//	}
//
// NOTE: Only use for tests that explicitly need non-root environment.
// Most tests should be modified to work correctly as root.
func SkipIfRoot(t *testing.T, platform Platform, reason string) {
	if platform.IsRoot {
		t.Skipf("Skipping test - %s (requires non-root user, running as UID 0)", reason)
	}
}

// SkipIfWindows marks the test as skipped on Windows platforms.
// Useful for Unix-specific integration tests that require PTY or Unix permissions.
//
// Example:
//
//	platform := DetectPlatform(t)
//	if platform.IsWindows {
//		SkipIfWindows(t, "test requires Unix PTY or permissions")
//	}
func SkipIfWindows(t *testing.T, platform Platform, reason string) {
	if platform.IsWindows {
		t.Skipf("Skipping test - %s (Windows platform detected)", reason)
	}
}

// AssertCanBypassPermissions tests if current user can bypass
// filesystem permission restrictions ( chmod, chown, etc.).
//
// Root user (UID 0) on Unix systems can bypass chmod restrictions.
// This is useful for documenting why permission-based tests
// won't work in certain environments.
//
// Example:
//
//	platform := DetectPlatform(t)
//	if !platform.IsRoot && !platform.IsWindows {
//		t.Fatalf("Expected to be root to bypass permissions, got UID %d", platform.UID)
//	}
func AssertCanBypassPermissions(t *testing.T, platform Platform) {
	// Root user (UID 0) on Unix can bypass chmod restrictions
	if platform.IsUnix && platform.IsRoot {
		return // Can bypass
	}

	// Windows has different permission model, but we don't test it
	if platform.IsWindows {
		t.Skip("Windows permission model test skipped")
	}

	// Non-root Unix users cannot bypass chmod
	if platform.IsUnix && !platform.IsRoot {
		t.Fatalf("Expected to be root to bypass permissions, got UID %d", platform.UID)
	}
}
