package storage

import (
	"fmt"
)

// BackendFactory is a function that creates a new StorageBackend instance.
type BackendFactory func(sessionID string) (StorageBackend, error)

// BackendRegistry maps backend names to their factory functions.
var BackendRegistry = make(map[string]BackendFactory)

func init() {
	// Register the file system backend as the default
	BackendRegistry["fs"] = func(sessionID string) (StorageBackend, error) {
		return NewFileSystemBackend(sessionID)
	}

	// Register an in-memory backend for testing
	BackendRegistry["memory"] = func(sessionID string) (StorageBackend, error) {
		return NewInMemoryBackend(sessionID)
	}
}

// GetBackend retrieves a backend by name and creates an instance.
func GetBackend(name, sessionID string) (StorageBackend, error) {
	factory, ok := BackendRegistry[name]
	if !ok {
		return nil, fmt.Errorf("unknown storage backend: %s", name)
	}
	return factory(sessionID)
}
