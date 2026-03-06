package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/unicode/norm"
)

var (
	fallbackOnce sync.Once
	fallbackDir  string
	fallbackErr  error

	// Pre-compiled regexes for sanitizeFilename (compiled once, used per call).
	unsafePattern   = regexp.MustCompile(`[/\\:*?"<>|\x00]`)
	collapsePattern = regexp.MustCompile(`_+`)
	reservedPattern = regexp.MustCompile(`(?i)^(CON|PRN|AUX|NUL|COM[1-9]|LPT[1-9])(\..*)?$`)
)

// pathsMu guards the function-variable overrides below against concurrent
// reads (from cleanup/inspector goroutines) and writes (from SetTestPaths /
// ResetPaths in tests). Every access to the three function variables MUST go
// through the get* accessors or the Set/Reset helpers.
var pathsMu sync.RWMutex

// To enable testing without polluting the user's home directory,
// these functions are defined as variables. The test suite can then
// override them to point to a temporary directory.
var (
	sessionDirectory    = SessionDirectory
	sessionFilePath     = SessionFilePath
	sessionLockFilePath = SessionLockFilePath
)

// getSessionDirectory returns the session directory via the (possibly
// overridden) function variable, under a read-lock.
func getSessionDirectory() (string, error) {
	pathsMu.RLock()
	fn := sessionDirectory
	pathsMu.RUnlock()
	return fn()
}

// getSessionLockFilePath returns the session lock file path via the (possibly
// overridden) function variable, under a read-lock.
func getSessionLockFilePath(id string) (string, error) {
	pathsMu.RLock()
	fn := sessionLockFilePath
	pathsMu.RUnlock()
	return fn(id)
}

// SetTestPaths overrides the path functions for testing.
// This should only be used in tests.
func SetTestPaths(dir string) {
	pathsMu.Lock()
	defer pathsMu.Unlock()
	sessionDirectory = func() (string, error) { return dir, nil }
	sessionFilePath = func(id string) (string, error) {
		return filepath.Join(dir, sanitizeFilename(id)+".session.json"), nil
	}
	sessionLockFilePath = func(id string) (string, error) {
		return filepath.Join(dir, sanitizeFilename(id)+".session.lock"), nil
	}
}

// ResetPaths resets the path functions to their defaults.
// This should only be used in tests.
func ResetPaths() {
	pathsMu.Lock()
	defer pathsMu.Unlock()
	sessionDirectory = SessionDirectory
	sessionFilePath = SessionFilePath
	sessionLockFilePath = SessionLockFilePath
}

// SessionDirectory returns the directory where session files are stored.
// Uses os.UserConfigDir() to resolve to {UserConfigDir}/osm/sessions/
//
// In test environments where UserConfigDir() fails, this will create an isolated,
// unpredictable temporary directory using os.MkdirTemp(). The fallback directory
// is cached using sync.Once to ensure the same path is returned for the lifetime
// of the process. This is safe because:
// 1. The directory is created with 0700 permissions (owner-only access)
// 2. The path contains a random suffix, preventing predictable collisions
// 3. It's isolated per process invocation, not shared across users/processes
// 4. The path is stable across all invocations within the same process
func SessionDirectory() (string, error) {
	configDir, err := os.UserConfigDir()
	if err == nil {
		return filepath.Join(configDir, "osm", "sessions"), nil
	}

	// Fallback for environments where UserConfigDir fails (e.g., CI with no $HOME).
	// Create an isolated, unpredictable temp directory that's safe from interference.
	// Use sync.Once to ensure we only create the directory once per process.
	fallbackOnce.Do(func() {
		fallbackDir, fallbackErr = os.MkdirTemp("", "osm-sessions-*")
	})

	return fallbackDir, fallbackErr
}

// SessionFilePath returns the absolute path to a session file.
// File naming: {session_id}.session.json
//
// SECURITY: The session ID is sanitized via sanitizeFilename() to prevent
// path traversal attacks (e.g., "../../../etc/passwd" as a session ID).
func SessionFilePath(sessionID string) (string, error) {
	dir, err := getSessionDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitizeFilename(sessionID)+".session.json"), nil
}

// SessionLockFilePath returns the absolute path to a session lock file.
// File naming: {session_id}.session.lock
//
// SECURITY: The session ID is sanitized via sanitizeFilename() to prevent
// path traversal attacks.
func SessionLockFilePath(sessionID string) (string, error) {
	dir, err := getSessionDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitizeFilename(sessionID)+".session.lock"), nil
}

// sessionArchiveDir returns the directory where archived session files are stored.
// Creates the archive subdirectory if it doesn't exist.
func sessionArchiveDir() (string, error) {
	dir, err := getSessionDirectory()
	if err != nil {
		return "", err
	}
	archiveDir := filepath.Join(dir, "archive")
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		return "", err
	}
	return archiveDir, nil
}

// sanitizeFilename replaces filesystem-unsafe characters with underscores.
// This prevents path traversal and cross-platform filename issues.
func sanitizeFilename(input string) string {
	// Normalize Unicode to NFKC to prevent Unicode normalization-based
	// path traversal / bypass attacks where visually equivalent strings use
	// different combining/compatibility forms.
	input = norm.NFKC.String(input)
	// Replace problematic characters: /, \, :, *, ?, ", <, >, |, null, etc.
	sanitized := unsafePattern.ReplaceAllString(input, "_")
	// Also collapse multiple underscores into one for cleanliness
	sanitized = collapsePattern.ReplaceAllString(sanitized, "_")

	// Trim trailing dots/spaces which are problematic on Windows.
	// Preserve leading dots (e.g. ".gitignore") which are valid POSIX filenames
	// and should not collide with non-dot-prefixed IDs.
	sanitized = strings.TrimRight(sanitized, " .")

	// If sanitized results in empty or dot-like entries, use a safe fallback.
	if sanitized == "" || sanitized == "." || sanitized == ".." {
		return "_"
	}

	// Avoid reserved Windows device names like CON, PRN, AUX, NUL, COM1..COM9, LPT1..LPT9
	// Comparison is case-insensitive.
	upper := strings.ToUpper(sanitized)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return "_" + sanitized
	}

	// COM[1-9] and LPT[1-9] - these are reserved even when followed by an extension
	// e.g. COM1, COM1.txt are reserved names on Windows.
	if reservedPattern.MatchString(sanitized) {
		return "_" + sanitized
	}
	return sanitized
}

// ArchiveSessionFilePath returns the path where a session should be archived.
// Filename format: {sessionDir}/archive/{sanitizedSessionID}--reset--{UTC-ISO8601}--{counter}.session.json
func ArchiveSessionFilePath(sessionID string, ts time.Time, counter int) (string, error) {
	archiveDir, err := sessionArchiveDir()
	if err != nil {
		return "", err
	}
	sanitizedID := sanitizeFilename(sessionID)
	// Format timestamp: 2025-11-26T14-03-00Z (hyphens instead of colons for cross-platform)
	timestampStr := ts.UTC().Format("2006-01-02T15-04-05Z")
	archiveFilename := filepath.Join(archiveDir,
		sanitizedID+"--reset--"+timestampStr+"--"+fmt.Sprintf("%03d", counter)+".session.json")
	return archiveFilename, nil
}
