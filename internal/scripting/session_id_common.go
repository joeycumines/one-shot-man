package scripting

import (
	"fmt"
	"os"

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

	// 3. Controlling terminal device path
	if termID := getTerminalID(); termID != "" {
		return termID
	}

	// 4. Platform-specific stable environment variables
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

	stateManager, err := NewStateManager(backend, sessionID)
	if err != nil {
		// Clean up backend if StateManager creation fails
		backend.Close()
		return nil, fmt.Errorf("failed to create state manager: %w", err)
	}

	return stateManager, nil
}
