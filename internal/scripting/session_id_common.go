package scripting

import (
	"fmt"
	"os"

	"github.com/joeycumines/one-shot-man/internal/session"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// discoverSessionID determines the active session ID using the sophisticated hierarchy defined in
// docs/sophisticated-auto-determination-of-session-id.md:
// 1. Explicit Override (--session flag or OSM_SESSION_ID env)
// 2. Multiplexer (TMUX_PANE / STY)
// 3. SSH Context (SSH_CONNECTION with client port)
// 4. macOS GUI Terminal (TERM_SESSION_ID)
// 5. Deep Anchor (recursive process walk)
// 6. UUID Fallback
func discoverSessionID(overrideSessionID string) string {
	id, _, err := session.GetSessionID(overrideSessionID)
	if err != nil {
		// This should never happen since GetSessionID always falls back to UUID
		// but handle it gracefully just in case
		return "fallback-error-session"
	}
	return id
}

// GetSessionID is the exported entrypoint used by other packages to resolve
// the session ID that would be used for the current terminal. It simply
// delegates to discoverSessionID and preserves the resolution precedence.
func GetSessionID(overrideSessionID string) string {
	return discoverSessionID(overrideSessionID)
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
