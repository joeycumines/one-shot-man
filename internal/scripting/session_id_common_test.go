package scripting

import (
	"os"
	"strings"
	"testing"
)

// isolateEnv captures the current environment, clears it for the test,
// and guarantees restoration when the test finishes (via t.Cleanup).
func isolateEnv(t *testing.T) {
	t.Helper()
	// Snapshot the original environment
	originalEnv := os.Environ()

	// Clear environment to give the test the empty slate it requests
	os.Clearenv()

	// Register cleanup to restore the environment
	t.Cleanup(func() {
		// Wipe whatever mess the test left behind
		os.Clearenv()

		// Restore the original state
		for _, env := range originalEnv {
			// os.Environ returns "key=value" strings
			key, val, found := strings.Cut(env, "=")
			if found {
				os.Setenv(key, val)
			}
		}
	})
}

func TestDiscoverSessionID_PrecedenceOverrideFlag(t *testing.T) {
	isolateEnv(t)

	got := discoverSessionID("explicit-override")
	if got != "explicit-override" {
		t.Fatalf("override flag not respected: got %q", got)
	}
}

func TestDiscoverSessionID_PrecedenceEnvVar(t *testing.T) {
	isolateEnv(t)

	os.Setenv("OSM_SESSION_ID", "from-env")
	// Even if TMUX_PANE is set, OSM_SESSION_ID should still win
	os.Setenv("TMUX", "1")
	os.Setenv("TMUX_PANE", "%4")

	got := discoverSessionID("")
	if got != "from-env" {
		t.Fatalf("OSM_SESSION_ID did not win precedence, got %q", got)
	}
}

func TestDiscoverSessionID_ScreenPreferred(t *testing.T) {
	isolateEnv(t)

	os.Setenv("STY", "12345.pts-0.host")

	got := discoverSessionID("")
	// The new implementation returns a SHA256 hash of "screen:" + sty
	// It should be a 64-character hex string (SHA256 hash)
	if len(got) != 64 {
		t.Fatalf("expected SHA256 hash (64 chars), got %d chars: %q", len(got), got)
	}
}

func TestDiscoverSessionID_SSHConnection(t *testing.T) {
	isolateEnv(t)

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	got := discoverSessionID("")
	// The new implementation returns a SHA256 hash of "ssh:client_ip:client_port:server_ip:server_port"
	// It should be a 64-character hex string (SHA256 hash)
	if len(got) != 64 {
		t.Fatalf("expected SHA256 hash (64 chars), got %d chars: %q", len(got), got)
	}
}

func TestDiscoverSessionID_SSHDifferentPorts(t *testing.T) {
	isolateEnv(t)

	// Session 1 with port 12345
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	id1 := discoverSessionID("")

	// We must reset the env for the second "session" within this test,
	// or effectively clear it. Since isolateEnv handles the *global* restore,
	// within the test we can just overwrite the var.
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12346 192.168.1.1 22")
	id2 := discoverSessionID("")

	// Different client ports should produce different session IDs
	// This is a key requirement from the architecture document
	if id1 == id2 {
		t.Fatalf("expected different session IDs for different client ports, both got: %q", id1)
	}
}

func TestDiscoverSessionID_FallbackToDeepAnchorOrUUID(t *testing.T) {
	isolateEnv(t)

	got := discoverSessionID("")
	// Without any environment variables, should fall back to deep-anchor or UUID
	// In either case, should return a non-empty string
	if got == "" {
		t.Fatal("expected non-empty session ID")
	}
	// On Linux, deep-anchor returns a 64-char SHA256 hash
	// UUID fallback returns a UUID format
	// Either way, should have at least 32 characters
	if len(got) < 32 {
		t.Fatalf("session ID seems too short: %q", got)
	}
}
