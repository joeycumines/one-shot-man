// Package filepathutil provides shared path manipulation utilities.
package filepathutil

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// IsTildeExpansionPath returns true if the given path is a tilde expansion form
// that should be expanded to the user's home directory. This includes:
// - Bare "~"
// - Paths starting with "~/"
// - On Windows, paths starting with "~\"
//
// This function is designed to distinguish between actual tilde expansion forms
// and literal paths that happen to start with "~" (e.g., "~cache", "~tmp").
// Literal paths should not be treated specially for path normalization purposes.
func IsTildeExpansionPath(path string) bool {
	return path == "~" ||
		strings.HasPrefix(path, "~/") ||
		(runtime.GOOS == "windows" && len(path) >= 2 && path[0] == '~' && path[1] == '\\')
}

// userHomeDir is the function used by ExpandTilde to resolve the current user's
// home directory. It defaults to os.UserHomeDir. Same-package tests may override
// this directly for deterministic behavior without relying on global OS state that
// has fallback mechanisms (e.g., getpwuid on Unix).
var userHomeDir = os.UserHomeDir

// ExpandTilde replaces ~ with the user's home directory.
// It handles bare ~, paths starting with ~/ (e.g., ~/foo/bar), and on
// Windows, paths starting with ~\ (e.g., ~\Documents\file.txt).
//
// Note: POSIX ~username/ expansion (to another user's home directory) is
// not supported. Only the current user's home directory is resolved.
//
// This function relies on userHomeDir (os.UserHomeDir by default) which queries
// global system state (environment variables HOME and USERPROFILE on Unix and
// Windows respectively). This means ExpandTilde is NOT a pure function - its
// output depends on the environment at the time of calling. Same-package tests
// that need deterministic home directory resolution should assign directly to
// the unexported userHomeDir variable rather than manipulating environment
// variables, since os.UserHomeDir has OS-level fallback mechanisms (e.g.,
// getpwuid on Unix) that can succeed even when environment variables are unset.
func ExpandTilde(path string) (string, error) {
	if !IsTildeExpansionPath(path) {
		return path, nil
	}

	home, err := userHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to determine home directory for tilde expansion: %w", err)
	}

	if path == "~" {
		return home, nil
	}

	// Manually concatenate and clean to avoid ~//path vulnerability.
	// Some systems treat paths starting with // as absolute, which would
	// discard the home directory entirely. We use filepath.Clean to
	// normalize the result.
	return filepath.Clean(home + string(filepath.Separator) + path[2:]), nil
}
