package storage

import (
	"encoding/json"
	"fmt"
	"sync"
)

// InMemoryBackend implements StorageBackend using in-memory storage (for testing).
type InMemoryBackend struct {
	sessionID string
	sessions  map[string]*Session
}

// Global in-memory storage shared across all instances (for testing)
var globalInMemoryStore = struct {
	sync.RWMutex
	sessions map[string]*Session
}{
	sessions: make(map[string]*Session),
}

// NewInMemoryBackend creates a new in-memory storage backend (for testing).
func NewInMemoryBackend(sessionID string) (*InMemoryBackend, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("sessionID cannot be empty")
	}

	return &InMemoryBackend{
		sessionID: sessionID,
		sessions:  globalInMemoryStore.sessions,
	}, nil
}

// LoadSession retrieves a session by its unique ID.
func (b *InMemoryBackend) LoadSession(sessionID string) (*Session, error) {
	if sessionID != b.sessionID {
		return nil, fmt.Errorf("session ID mismatch: backend is for %q, requested %q", b.sessionID, sessionID)
	}

	globalInMemoryStore.RLock()
	session, exists := globalInMemoryStore.sessions[sessionID]
	globalInMemoryStore.RUnlock()

	if !exists {
		return nil, nil
	}

	// Return a deep copy to prevent concurrent modification
	data, err := json.Marshal(session)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session: %w", err)
	}

	var copied Session
	if err := json.Unmarshal(data, &copied); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session copy: %w", err)
	}

	return &copied, nil
}

// SaveSession atomically persists the entire session state.
func (b *InMemoryBackend) SaveSession(session *Session) error {
	if session.SessionID != b.sessionID {
		return fmt.Errorf("session ID mismatch: backend is for %q, session has %q", b.sessionID, session.SessionID)
	}

	// Create a deep copy to prevent external modification
	data, err := json.Marshal(session)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	var copied Session
	if err := json.Unmarshal(data, &copied); err != nil {
		return fmt.Errorf("failed to unmarshal session copy: %w", err)
	}

	globalInMemoryStore.Lock()
	globalInMemoryStore.sessions[session.SessionID] = &copied
	globalInMemoryStore.Unlock()

	return nil
}

// Close releases any resources (no-op for in-memory backend).
func (b *InMemoryBackend) Close() error {
	return nil
}

// ClearAllInMemorySessions clears all sessions from the in-memory store (for testing).
func ClearAllInMemorySessions() {
	globalInMemoryStore.Lock()
	globalInMemoryStore.sessions = make(map[string]*Session)
	globalInMemoryStore.Unlock()
}

// Ensure InMemoryBackend implements StorageBackend at compile time
var _ StorageBackend = (*InMemoryBackend)(nil)
