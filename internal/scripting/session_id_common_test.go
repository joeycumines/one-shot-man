package scripting

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

func TestDiscoverSessionID_TmuxPanePreferred(t *testing.T) {
	os.Clearenv()
	defer os.Unsetenv("TMUX")
	defer os.Unsetenv("TMUX_PANE")

	socketPath := "/tmp/tmux-1000/default"
	os.Setenv("TMUX", fmt.Sprintf("%s,1234,0", socketPath))
	os.Setenv("TMUX_PANE", "pane-1234")

	// Calculate expected hash
	hash := sha256.Sum256([]byte(socketPath))
	socketHash := hex.EncodeToString(hash[:])[:8]

	got := discoverSessionID("")
	want := fmt.Sprintf("tmux-%s-pane-1234", socketHash)
	if got != want {
		t.Fatalf("expected tmux pane id %q got %q", want, got)
	}
}
