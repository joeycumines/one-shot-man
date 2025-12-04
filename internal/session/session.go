package session

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GetSessionID implements the full discovery hierarchy.
// Returns (sessionID, source, error) where source describes which method succeeded.
func GetSessionID(explicitOverride string) (string, string, error) {
	// Priority 1: Explicit Override
	if explicitOverride != "" {
		return explicitOverride, "explicit-flag", nil
	}
	if envID := os.Getenv("OSM_SESSION_ID"); envID != "" {
		return envID, "explicit-env", nil
	}

	// Priority 2: Multiplexer Detection
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		if sessionID, err := getTmuxSessionID(); err == nil {
			return sessionID, "tmux", nil
		}
	}
	if sty := os.Getenv("STY"); sty != "" {
		return hashString("screen:" + sty), "screen", nil
	}

	// Priority 3: SSH Context
	if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
		parts := strings.Fields(sshConn)
		if len(parts) == 4 {
			// include client port to differentiate concurrent sessions
			stableString := fmt.Sprintf("ssh:%s:%s:%s:%s", parts[0], parts[1], parts[2], parts[3])
			return hashString(stableString), "ssh-env", nil
		}
		// fallback for malformed SSH_CONNECTION
		return hashString("ssh:" + sshConn), "ssh-env", nil
	}

	// Priority 4: macOS GUI Terminal
	if runtime.GOOS == "darwin" {
		if termID := os.Getenv("TERM_SESSION_ID"); termID != "" {
			return hashString("terminal:" + termID), "macos-terminal", nil
		}
	}

	// Priority 5: Deep Anchor (Platform-Specific)
	ctx, err := resolveDeepAnchor()
	if err == nil && ctx.AnchorPID != 0 {
		return ctx.GenerateHash(), "deep-anchor", nil
	}

	// Priority 6: UUID Fallback
	UUID, err := generateUUID()
	if err != nil {
		return "", "", fmt.Errorf("all session detection methods failed: %w", err)
	}
	return UUID, "uuid-fallback", nil
}

func getTmuxSessionID() (string, error) {
	// Find the absolute path to tmux to avoid PATH manipulation issues
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return "", fmt.Errorf("tmux not found in PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Query tmux for the unique session identifier (e.g., "$0")
	cmd := exec.CommandContext(ctx, tmuxPath, "display-message", "-p", "#{session_id}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func generateUUID() (string, error) {
	return uuid.NewString(), nil
}

// hashString computes a SHA256 hex for various detectors (screen, ssh, terminal).
func hashString(s string) string {
	hasher := sha256.New()
	hasher.Write([]byte(s))
	return hex.EncodeToString(hasher.Sum(nil))
}
