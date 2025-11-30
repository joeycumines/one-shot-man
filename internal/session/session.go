package session

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
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
		// TMUX_PANE present but tmux unreachable; treat as stale, continue
	}
	if sty := os.Getenv("STY"); sty != "" {
		return hashString("screen:" + sty), "screen", nil
	}

	// Priority 3: SSH Context
	if sshConn := os.Getenv("SSH_CONNECTION"); sshConn != "" {
		// SSH_CONNECTION = "client_ip client_port server_ip server_port"
		parts := strings.Fields(sshConn)
		if len(parts) == 4 {
			// CONFLICT RESOLUTION: We must include the client port (parts[1])
			// to distinguish between concurrent sessions (e.g. tabs) from the same host.
			// Persistence across reconnects is handled by Multiplexers (Priority 2).
			stableString := fmt.Sprintf("ssh:%s:%s:%s:%s", parts[0], parts[1], parts[2], parts[3])
			return hashString(stableString), "ssh-env", nil
		}
		// Fallback for malformed string
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
	uuid, err := generateUUID()
	if err != nil {
		return "", "", fmt.Errorf("all session detection methods failed: %w", err)
	}
	return uuid, "uuid-fallback", nil
}

func getTmuxSessionID() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Query tmux for the unique session identifier (e.g., "$0")
	cmd := exec.CommandContext(ctx, "tmux", "display-message", "-p", "#{session_id}")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func generateUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func hashString(s string) string {
	ctx := &SessionContext{BootID: s}
	// CONFLICT RESOLUTION: Removed truncation.
	// Returning 16 chars (64-bits) created a collision risk.
	return ctx.GenerateHash()
}
