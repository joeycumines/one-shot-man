package scripting

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

// discoverSessionID determines the active session ID using the precedence defined in the plan:
// 1. overrideSessionID parameter (from --session flag)
// 2. OSM_SESSION_ID environment variable
// 3. Controlling terminal device path (POSIX systems)
// 4. Platform-specific stable environment variables (TERM_SESSION_ID on macOS, WINDOWID on X11)
// 5. A newly generated UUID if no other stable ID can be derived
func discoverSessionID(overrideSessionID string) string {
	// 1. Command-line flag (via parameter)
	if overrideSessionID != "" {
		return overrideSessionID
	}

	// 2. OSM_SESSION_ID environment variable
	if envID := os.Getenv("OSM_SESSION_ID"); envID != "" {
		return envID
	}

	// 3. Multiplexer / pane identifiers (tmux, screen) - prefer these before a generic tty
	// Tmux: TMUX_PANE (unique pane identifier within a tmux server)
	if tmuxPane := os.Getenv("TMUX_PANE"); tmuxPane != "" {
		// We include a hash of the TMUX socket path (first part of TMUX env var)
		// to ensure uniqueness across multiple tmux servers.
		tmuxEnv := os.Getenv("TMUX")
		if tmuxEnv != "" {
			// TMUX format: socket_path,pid,session_id
			// We use the socket path to identify the server instance.
			parts := strings.Split(tmuxEnv, ",")
			if len(parts) > 0 {
				socketPath := parts[0]
				// Hash the socket path to keep the ID short and safe
				hash := sha256.Sum256([]byte(socketPath))
				socketHash := hex.EncodeToString(hash[:])[:8]
				return fmt.Sprintf("tmux-%s-%s", socketHash, tmuxPane)
			}
		}
		// Fallback if TMUX env var is malformed but TMUX_PANE exists (unlikely)
		return fmt.Sprintf("tmux-%s", tmuxPane)
	}
	// Screen: STY identifies a screen session
	if sty := os.Getenv("STY"); sty != "" {
		return fmt.Sprintf("screen-%s", sty)
	}

	// 4. Controlling terminal device path (fallback when no multiplexer id exists)
	if termID := getTerminalID(); termID != "" {
		return termID
	}

	// macOS: TERM_SESSION_ID
	if termSessionID := os.Getenv("TERM_SESSION_ID"); termSessionID != "" {
		return termSessionID
	}

	// X11: WINDOWID
	if windowID := os.Getenv("WINDOWID"); windowID != "" {
		return fmt.Sprintf("x11-%s", windowID)
	}

	// 5. Generate a new UUID
	return uuid.New().String()
}

// initializeStateManager creates and initializes a StateManager for the given session ID.
// It uses the storage backend specified by overrideBackend parameter, OSM_STORAGE_BACKEND environment variable, or defaults to 'fs'.
func initializeStateManager(sessionID string, overrideBackend string) (*StateManager, error) {
	// Determine which backend to use with proper precedence
	backendName := overrideBackend
	if backendName == "" {
		backendName = os.Getenv("OSM_STORAGE_BACKEND")
	}
	if backendName == "" {
		backendName = "fs" // Default to file system backend
	}

	backend, err := storage.GetBackend(backendName, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage backend %q: %w", backendName, err)
	}
	var success bool
	defer func() {
		if !success {
			_ = backend.Close()
		}
	}()

	stateManager, err := NewStateManager(backend, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	success = true
	return stateManager, nil
}
