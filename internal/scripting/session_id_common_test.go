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
	// New format: ex--{payload}_{8hex} for explicit overrides
	if !strings.HasPrefix(got, "ex--explicit-override_") {
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
	// New format: ex--{payload}_{8hex} for explicit overrides (env var)
	if !strings.HasPrefix(got, "ex--from-env_") {
		t.Fatalf("OSM_SESSION_ID did not win precedence, got %q", got)
	}
}

func TestDiscoverSessionID_ScreenPreferred(t *testing.T) {
	isolateEnv(t)

	os.Setenv("STY", "12345.pts-0.host")

	got := discoverSessionID("")
	// New format: screen--{hash16}, total 24 chars
	if len(got) != 24 {
		t.Fatalf("expected screen-- prefix + 16-char hash (24 chars), got %d chars: %q", len(got), got)
	}
	if got[:8] != "screen--" {
		t.Fatalf("expected screen-- prefix, got %q", got[:8])
	}
}

func TestDiscoverSessionID_SSHConnection(t *testing.T) {
	isolateEnv(t)

	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	got := discoverSessionID("")
	// New format: ssh--{hash16}, total 21 chars
	if len(got) != 21 {
		t.Fatalf("expected ssh-- prefix + 16-char hash (21 chars), got %d chars: %q", len(got), got)
	}
	if got[:5] != "ssh--" {
		t.Fatalf("expected ssh-- prefix, got %q", got[:5])
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
	// New format: namespaced IDs (anchor--{hash16} or uuid--{uuid})
	// Check it has a namespace delimiter
	if !strings.Contains(got, "--") {
		t.Fatalf("session ID should have namespace delimiter: %q", got)
	}
	// Should have at least a namespace + delimiter + some payload
	if len(got) < 10 {
		t.Fatalf("session ID seems too short: %q", got)
	}
}
