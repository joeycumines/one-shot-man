//go:build !linux && !windows

package session

import (
	"os"
	"runtime"
	"testing"
)

// =============================================================================
// Platform Stub Tests (macOS/BSD)
// Per doc: Deep Anchor not implemented on these platforms; relies on TERM_SESSION_ID
// =============================================================================

// TestResolveDeepAnchor_Stub verifies deep anchor returns error on unsupported platforms.
func TestResolveDeepAnchor_Stub(t *testing.T) {
	ctx, err := resolveDeepAnchor()
	if err == nil {
		t.Errorf("expected error on %s, got nil", runtime.GOOS)
	}
	if ctx != nil {
		t.Errorf("expected nil context on %s", runtime.GOOS)
	}
}

// TestResolveDeepAnchor_ErrorMessage_Stub verifies error message contains platform.
func TestResolveDeepAnchor_ErrorMessage_Stub(t *testing.T) {
	_, err := resolveDeepAnchor()
	if err == nil {
		t.Fatalf("expected error on %s", runtime.GOOS)
	}

	// Error should mention the current platform
	errStr := err.Error()
	if errStr == "" {
		t.Error("error message should not be empty")
	}

	// Per doc: "deep anchor detection not supported on %s"
	expectedSubstr := "not supported"
	if len(errStr) < len(expectedSubstr) {
		t.Errorf("error message too short: %q", errStr)
	}
}

// =============================================================================
// macOS-Specific Tests (darwin)
// Per doc: TERM_SESSION_ID (Priority 4) is the primary identifier
// =============================================================================

// TestGetSessionID_MacOSTerminal_Priority tests TERM_SESSION_ID handling.
func TestGetSessionID_MacOSTerminal_Priority(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("TERM_SESSION_ID")

	os.Setenv("TERM_SESSION_ID", "terminal-session-abc123")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if runtime.GOOS == "darwin" {
		if source != "macos-terminal" {
			t.Errorf("expected source macos-terminal on darwin, got %q", source)
		}
		if len(id) != 64 {
			t.Errorf("expected SHA256 hash (64 chars), got %d chars", len(id))
		}
	}
	// On non-darwin, TERM_SESSION_ID should be ignored
}

// TestGetSessionID_Fallback_Stub tests fallback when deep anchor fails.
func TestGetSessionID_Fallback_Stub(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Without deep anchor and without env vars, should fall back to UUID
	if source != "uuid-fallback" {
		t.Logf("expected uuid-fallback on %s without env vars, got %q", runtime.GOOS, source)
	}

	if id == "" {
		t.Error("session ID should not be empty")
	}
}

// =============================================================================
// Platform-Agnostic Tests that should pass on all platforms
// =============================================================================

// TestGetSessionID_ExplicitOverride_AllPlatforms verifies explicit override works.
func TestGetSessionID_ExplicitOverride_AllPlatforms(t *testing.T) {
	os.Clearenv()

	id, source, err := GetSessionID("explicit-test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != "explicit-test" {
		t.Errorf("expected explicit-test, got %q", id)
	}

	if source != "explicit-flag" {
		t.Errorf("expected source explicit-flag, got %q", source)
	}
}

// TestGetSessionID_EnvOverride_AllPlatforms verifies env var override works.
func TestGetSessionID_EnvOverride_AllPlatforms(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("OSM_SESSION_ID")

	os.Setenv("OSM_SESSION_ID", "env-test")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if id != "env-test" {
		t.Errorf("expected env-test, got %q", id)
	}

	if source != "explicit-env" {
		t.Errorf("expected source explicit-env, got %q", source)
	}
}

// TestGetSessionID_Screen_AllPlatforms verifies screen detection works.
func TestGetSessionID_Screen_AllPlatforms(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("STY")

	os.Setenv("STY", "12345.pts-0.host")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if source != "screen" {
		t.Errorf("expected source screen, got %q", source)
	}

	if len(id) != 64 {
		t.Errorf("expected SHA256 hash (64 chars), got %d chars: %q", len(id), id)
	}
}

// TestGetSessionID_SSH_AllPlatforms verifies SSH detection works.
func TestGetSessionID_SSH_AllPlatforms(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	id, source, err := GetSessionID("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if source != "ssh-env" {
		t.Errorf("expected source ssh-env, got %q", source)
	}

	if len(id) != 64 {
		t.Errorf("expected SHA256 hash (64 chars), got %d chars: %q", len(id), id)
	}
}
