//go:build unix

package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

// osmBinaryPath caches the built binary path across tests.
var (
	osmBinaryOnce sync.Once
	osmBinaryPath string
	osmBinaryErr  error
)

// buildOSMBinary compiles the osm binary once per test run. Returns the
// path to the binary or an error. The binary is placed in a temp directory
// generated from the test's context.
//
// The binary is built with -tags=integration to enable go-prompt's sync
// protocol. Without this tag, go-prompt processes multiple bytes arriving
// in a single PTY read as a single key event — causing input like
// "auto-split\r" to be treated as unrecognised text rather than a command
// followed by Enter. The sync protocol, combined with termtest.Console's
// WriteSync/SendLine methods, ensures deterministic input processing.
func buildOSMBinary(t *testing.T) string {
	t.Helper()
	osmBinaryOnce.Do(func() {
		binDir, err := os.MkdirTemp("", "osm-test-bin-*")
		if err != nil {
			osmBinaryErr = fmt.Errorf("failed to create temp dir: %w", err)
			return
		}
		osmBinaryPath = filepath.Join(binDir, "osm")
		cmd := exec.Command("go", "build", "-tags=integration", "-o", osmBinaryPath, "./cmd/osm")
		// Build from the repository root — two levels up from internal/command/.
		cmd.Dir = filepath.Join(projectRoot(t))
		cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			osmBinaryErr = fmt.Errorf("go build failed: %w\n%s", err, out)
		}
	})
	if osmBinaryErr != nil {
		t.Fatalf("failed to build osm: %v", osmBinaryErr)
	}
	return osmBinaryPath
}

// projectRoot returns the repository root by walking up from this file.
func projectRoot(t *testing.T) string {
	t.Helper()
	// internal/command/ → ../.. → repo root
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}
