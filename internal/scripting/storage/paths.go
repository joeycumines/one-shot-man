package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// To enable testing without polluting the user's home directory,
// these functions are defined as variables. The test suite can then
// override them to point to a temporary directory.
var (
	sessionDirectory    = SessionDirectory
	sessionFilePath     = SessionFilePath
	sessionLockFilePath = SessionLockFilePath
)

// SetTestPaths overrides the path functions for testing.
// This should only be used in tests.
func SetTestPaths(dir string) {
	sessionDirectory = func() (string, error) { return dir, nil }
	sessionFilePath = func(id string) (string, error) {
		return filepath.Join(dir, id+".session.json"), nil
	}
	sessionLockFilePath = func(id string) (string, error) {
		return filepath.Join(dir, id+".session.lock"), nil
	}
}

// ResetPaths resets the path functions to their defaults.
// This should only be used in tests.
func ResetPaths() {
	sessionDirectory = SessionDirectory
	sessionFilePath = SessionFilePath
	sessionLockFilePath = SessionLockFilePath
}

// SessionDirectory returns the directory where session files are stored.
// Uses os.UserConfigDir() to resolve to {UserConfigDir}/one-shot-man/sessions/
func SessionDirectory() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user config directory: %w", err)
	}
	return filepath.Join(configDir, "one-shot-man", "sessions"), nil
}

// SessionFilePath returns the absolute path to a session file.
// File naming: {session_id}.session.json
func SessionFilePath(sessionID string) (string, error) {
	dir, err := sessionDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionID+".session.json"), nil
}

// SessionLockFilePath returns the absolute path to a session lock file.
// File naming: {session_id}.session.lock
func SessionLockFilePath(sessionID string) (string, error) {
	dir, err := sessionDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sessionID+".session.lock"), nil
}
