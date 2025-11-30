package scripting

import (
	"os"
	"testing"
)

func TestDiscoverSessionID_PrecedenceOverrideFlag(t *testing.T) {
	os.Clearenv()
	got := discoverSessionID("explicit-override")
	if got != "explicit-override" {
		t.Fatalf("override flag not respected: got %q", got)
	}
}

func TestDiscoverSessionID_PrecedenceEnvVar(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("OSM_SESSION_ID")
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
	os.Clearenv()
	defer os.Unsetenv("STY")
	os.Setenv("STY", "12345.pts-0.host")

	got := discoverSessionID("")
	// The new implementation returns a SHA256 hash of "screen:" + sty
	// It should be a 64-character hex string (SHA256 hash)
	if len(got) != 64 {
		t.Fatalf("expected SHA256 hash (64 chars), got %d chars: %q", len(got), got)
	}
}

func TestDiscoverSessionID_SSHConnection(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")

	got := discoverSessionID("")
	// The new implementation returns a SHA256 hash of "ssh:client_ip:client_port:server_ip:server_port"
	// It should be a 64-character hex string (SHA256 hash)
	if len(got) != 64 {
		t.Fatalf("expected SHA256 hash (64 chars), got %d chars: %q", len(got), got)
	}
}

func TestDiscoverSessionID_SSHDifferentPorts(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("SSH_CONNECTION")

	// Session 1 with port 12345
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12345 192.168.1.1 22")
	id1 := discoverSessionID("")

	// Session 2 with port 12346 (different tab from same host)
	os.Setenv("SSH_CONNECTION", "192.168.1.100 12346 192.168.1.1 22")
	id2 := discoverSessionID("")

	// Different client ports should produce different session IDs
	// This is a key requirement from the architecture document
	if id1 == id2 {
		t.Fatalf("expected different session IDs for different client ports, both got: %q", id1)
	}
}

func TestDiscoverSessionID_FallbackToDeepAnchorOrUUID(t *testing.T) {
	os.Clearenv()

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
