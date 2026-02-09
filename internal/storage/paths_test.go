package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPaths(t *testing.T) {
	// We don't want to rely on the user's environment, but we also can't easily
	// mock os.UserConfigDir without changing the source. So, we test the suffix
	// of the path, which should be consistent across all environments.
	t.Run("SessionDirectory has correct suffix", func(t *testing.T) {
		dir, err := SessionDirectory()
		if err != nil {
			t.Fatalf("SessionDirectory() error = %v", err)
		}

		// If os.UserConfigDir succeeds we expect the standard suffix. Otherwise
		// the code falls back to a temp directory and we assert that pattern.
		if _, err := os.UserConfigDir(); err == nil {
			expectedSuffix := filepath.Join("one-shot-man", "sessions")
			if !strings.HasSuffix(dir, expectedSuffix) {
				t.Errorf("Expected path to end with %q, but got %q", expectedSuffix, dir)
			}
		} else {
			// In CI or headless environments we should get a temp directory starting with osm-sessions-
			if !strings.Contains(filepath.Base(dir), "osm-sessions-") {
				t.Errorf("Expected temp directory to contain 'osm-sessions-', but got %q", dir)
			}
		}
	})

	t.Run("SessionFilePath has correct structure", func(t *testing.T) {
		sessionID := "my-test-session"
		path, err := SessionFilePath(sessionID)
		if err != nil {
			t.Fatalf("SessionFilePath() error = %v", err)
		}

		expectedFilename := sessionID + ".session.json"
		if !strings.HasSuffix(path, expectedFilename) {
			t.Errorf("Expected path to end with %q, but got %q", expectedFilename, path)
		}
	})

	t.Run("SessionLockFilePath has correct structure", func(t *testing.T) {
		sessionID := "my-other-session"
		path, err := SessionLockFilePath(sessionID)
		if err != nil {
			t.Fatalf("SessionLockFilePath() error = %v", err)
		}

		expectedFilename := sessionID + ".session.lock"
		if !strings.HasSuffix(path, expectedFilename) {
			t.Errorf("Expected path to end with %q, but got %q", expectedFilename, path)
		}
	})

	t.Run("SessionDirectoryFallbackStability", func(t *testing.T) {
		// This test verifies that when os.UserConfigDir fails, the fallback
		// directory is cached and returns the same path across multiple invocations.
		// This is critical for persistence to work in headless/CI environments.

		// Save original HOME/USERPROFILE and XDG_CONFIG_HOME
		originalHome := os.Getenv("HOME")
		originalUserProfile := os.Getenv("USERPROFILE")
		originalXDG := os.Getenv("XDG_CONFIG_HOME")
		originalAppData := os.Getenv("AppData")
		originalLocalAppData := os.Getenv("LocalAppData")

		// Clear environment variables that os.UserConfigDir depends on
		os.Unsetenv("HOME")
		// XDG_CONFIG_HOME is also consulted on Unix-like systems.
		os.Unsetenv("XDG_CONFIG_HOME")
		os.Unsetenv("USERPROFILE")
		os.Unsetenv("AppData")
		os.Unsetenv("LocalAppData")

		defer func() {
			// Restore original environment
			if originalHome != "" {
				os.Setenv("HOME", originalHome)
			}
			if originalUserProfile != "" {
				os.Setenv("USERPROFILE", originalUserProfile)
			}
			if originalXDG != "" {
				os.Setenv("XDG_CONFIG_HOME", originalXDG)
			}
			if originalAppData != "" {
				os.Setenv("AppData", originalAppData)
			}
			if originalLocalAppData != "" {
				os.Setenv("LocalAppData", originalLocalAppData)
			}
		}()

		// Call SessionDirectory multiple times
		dir1, err1 := SessionDirectory()
		if err1 != nil {
			t.Fatalf("First SessionDirectory() call failed: %v", err1)
		}

		dir2, err2 := SessionDirectory()
		if err2 != nil {
			t.Fatalf("Second SessionDirectory() call failed: %v", err2)
		}

		dir3, err3 := SessionDirectory()
		if err3 != nil {
			t.Fatalf("Third SessionDirectory() call failed: %v", err3)
		}

		// All calls must return the exact same path
		if dir1 != dir2 {
			t.Errorf("SessionDirectory not stable: first call returned %q, second call returned %q", dir1, dir2)
		}
		if dir1 != dir3 {
			t.Errorf("SessionDirectory not stable: first call returned %q, third call returned %q", dir1, dir3)
		}

		// Verify the directory actually exists
		if _, err := os.Stat(dir1); os.IsNotExist(err) {
			t.Errorf("SessionDirectory returned path that doesn't exist: %q", dir1)
		}

		// Verify it follows the expected naming pattern
		if !strings.Contains(filepath.Base(dir1), "osm-sessions-") {
			t.Errorf("Fallback directory doesn't match expected pattern 'osm-sessions-*': %q", dir1)
		}
	})
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple alphanumeric", "session123", "session123"},
		{"slashes", "session/with/slashes", "session_with_slashes"},
		{"backslashes", "session\\with\\backslashes", "session_with_backslashes"},
		{"colons", "session:with:colons", "session_with_colons"},
		{"wildcard chars", "session*?with<>chars", "session_with_chars"},
		{"pipe and quotes", "session|with\"chars", "session_with_chars"},
		{"multiple underscores", "session___multiple___underscores", "session_multiple_underscores"},
		{"reserved CON", "CON", "_CON"},
		{"reserved nocase con", "con", "_con"},
		{"reserved COM1", "COM1", "_COM1"},
		{"reserved lpt9", "lpt9", "_lpt9"},
		{"reserved COM1 with ext", "COM1.txt", "_COM1.txt"},
		{"reserved con with ext", "Con.Txt", "_Con.Txt"},
		{"reserved nul dot", "NUL.", "_NUL"},
		{"dot only", ".", "_"},
		{"leading dot preserved", ".config", ".config"},
		{"empty string", "", "_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename_PreserveLeadingDot(t *testing.T) {
	// Ensure leading dots are preserved and do not collide with non-dot names
	a := SanitizeFilename(".config")
	b := SanitizeFilename("config")
	if a == b {
		t.Fatalf("expected leading dot to be preserved; got equal sanitized values %q", a)
	}
}

func TestSanitizeFilename_UnicodeNormalization(t *testing.T) {
	// 'e' + combining acute accent vs precomposed 'é' should normalize
	decomposed := "e\u0301-session"
	precomposed := "é-session"

	d := SanitizeFilename(decomposed)
	p := SanitizeFilename(precomposed)

	if d != p {
		t.Fatalf("expected decomposed and precomposed forms to sanitize equally; got %q vs %q", d, p)
	}
}

func TestArchiveSessionFilePath(t *testing.T) {
	defer ResetPaths()

	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "osm-archive-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Override path functions to use temp dir
	SetTestPaths(tmpDir)
	defer ResetPaths()

	ts := time.Date(2025, 11, 26, 14, 3, 0, 0, time.UTC)

	tests := []struct {
		sessionID string
		counter   int
		wantParts []string
	}{
		{
			sessionID: "session-123",
			counter:   0,
			wantParts: []string{"archive", "session-123", "reset", "2025-11-26T14-03-00Z", "000.session.json"},
		},
		{
			sessionID: "/dev/ttys001",
			counter:   1,
			wantParts: []string{"archive", "_dev_ttys001", "reset", "2025-11-26T14-03-00Z", "001.session.json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.sessionID, func(t *testing.T) {
			got, err := ArchiveSessionFilePath(tt.sessionID, ts, tt.counter)
			if err != nil {
				t.Fatalf("ArchiveSessionFilePath failed: %v", err)
			}

			// Check that all expected parts are in the path
			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("ArchiveSessionFilePath(%q, %v, %d) missing part %q in path: %s",
						tt.sessionID, ts, tt.counter, part, got)
				}
			}

			// Check that it's under the archive dir
			if !strings.Contains(got, "archive") {
				t.Errorf("Path should contain 'archive' dir: %s", got)
			}

			// Verify the path can be created
			archiveDir := filepath.Dir(got)
			if err := os.MkdirAll(archiveDir, 0755); err != nil {
				t.Fatalf("Failed to create archive dir: %v", err)
			}

			// Verify it's distinct from the session file path
			sessionPath, _ := SessionFilePath(tt.sessionID)
			if got == sessionPath {
				t.Errorf("Archive path should differ from session path")
			}
		})
	}
}

func TestSessionArchiveDir(t *testing.T) {
	defer ResetPaths()

	tmpDir, err := os.MkdirTemp("", "osm-archive-dirtest-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	SetTestPaths(tmpDir)
	defer ResetPaths()

	// First call should create the directory
	dir1, err := SessionArchiveDir()
	if err != nil {
		t.Fatalf("SessionArchiveDir failed: %v", err)
	}

	if !strings.Contains(dir1, "archive") {
		t.Errorf("Archive dir should contain 'archive': %s", dir1)
	}

	if _, err := os.Stat(dir1); err != nil {
		t.Errorf("Archive directory was not created: %v", err)
	}

	// Second call should return the same path and not error
	dir2, err := SessionArchiveDir()
	if err != nil {
		t.Fatalf("Second SessionArchiveDir failed: %v", err)
	}

	if dir1 != dir2 {
		t.Errorf("SessionArchiveDir should return same path consistently: %s vs %s", dir1, dir2)
	}
}
