package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// GlobalMetadata represents metadata for global housekeeping operations.
type globalMetadata struct {
	LastCleanupRun time.Time `json:"lastCleanupRun"`
}

// globalMetadataPathTest returns the metadata.json path adjacent to the sessions dir.
// This helper is only needed in tests so it lives in a _test.go file.
func globalMetadataPathTest() (string, error) {
	sessionsDir, err := sessionDirectory()
	if err != nil {
		return "", err
	}
	parent := filepath.Dir(sessionsDir)
	return filepath.Join(parent, "metadata.json"), nil
}

// loadGlobalMetadataTest loads the metadata.json for tests. If the file does not exist
// a zero-valued globalMetadata is returned with no error.
func loadGlobalMetadataTest() (*globalMetadata, error) {
	path, err := globalMetadataPathTest()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve metadata path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &globalMetadata{}, nil
		}
		return nil, fmt.Errorf("failed to read metadata file: %w", err)
	}

	var m globalMetadata
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse metadata.json: %w", err)
	}
	return &m, nil
}

// updateLastCleanupTest writes the provided time as LastCleanupRun using an atomic write.
func updateLastCleanupTest(t time.Time) error {
	path, err := globalMetadataPathTest()
	if err != nil {
		return fmt.Errorf("failed to resolve metadata path: %w", err)
	}

	m := globalMetadata{LastCleanupRun: t}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := AtomicWriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata atomically: %w", err)
	}
	return nil
}
