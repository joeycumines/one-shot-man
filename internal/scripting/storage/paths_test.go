package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPaths(t *testing.T) {
	// We don't want to rely on the user's environment, but we also can't easily
	// mock os.UserConfigDir without changing the source. So, we test the suffix
	// of the path, which should be consistent across all environments.
	t.Run("SessionDirectory has correct suffix", func(t *testing.T) {
		dir, err := SessionDirectory()
		if err != nil {
			// This might fail in some CI environments where HOME is not set.
			// We check for specific env vars to decide if we should skip.
			if os.Getenv("HOME") == "" && os.Getenv("USERPROFILE") == "" {
				t.Skip("Skipping test: user config directory cannot be determined in this environment.")
			}
			t.Fatalf("SessionDirectory() error = %v", err)
		}

		expectedSuffix := filepath.Join("one-shot-man", "sessions")
		if !strings.HasSuffix(dir, expectedSuffix) {
			t.Errorf("Expected path to end with %q, but got %q", expectedSuffix, dir)
		}
	})

	t.Run("SessionFilePath has correct structure", func(t *testing.T) {
		sessionID := "my-test-session"
		path, err := SessionFilePath(sessionID)
		if err != nil {
			if os.Getenv("HOME") == "" && os.Getenv("USERPROFILE") == "" {
				t.Skip("Skipping test: user config directory cannot be determined in this environment.")
			}
			t.Fatalf("SessionFilePath() error = %v", err)
		}
		expectedSuffix := filepath.Join("one-shot-man", "sessions", sessionID+".session.json")
		if !strings.HasSuffix(path, expectedSuffix) {
			t.Errorf("Expected path to end with %q, but got %q", expectedSuffix, path)
		}
	})

	t.Run("SessionLockFilePath has correct structure", func(t *testing.T) {
		sessionID := "my-other-session"
		path, err := SessionLockFilePath(sessionID)
		if err != nil {
			if os.Getenv("HOME") == "" && os.Getenv("USERPROFILE") == "" {
				t.Skip("Skipping test: user config directory cannot be determined in this environment.")
			}
			t.Fatalf("SessionLockFilePath() error = %v", err)
		}
		expectedSuffix := filepath.Join("one-shot-man", "sessions", sessionID+".session.lock")
		if !strings.HasSuffix(path, expectedSuffix) {
			t.Errorf("Expected path to end with %q, but got %q", expectedSuffix, path)
		}
	})
}
