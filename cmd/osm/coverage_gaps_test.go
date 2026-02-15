package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRun_ConfigLoadError covers the config.Load() error path in run().
// When config.Load() returns an error, run() creates an empty config
// and continues. This test triggers the error by pointing OSM_CONFIG
// to a symlink, which LoadFromPath rejects for security.
//
// We cannot use runWithCapturedIO here because it always overwrites
// OSM_CONFIG with its own temp path, stomping the symlink we set up.
func TestRun_ConfigLoadError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular file, then symlink to it.
	target := filepath.Join(tmpDir, "real-config")
	if err := os.WriteFile(target, []byte{}, 0644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	configPath := filepath.Join(tmpDir, "config")
	if err := os.Symlink(target, configPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	t.Setenv("OSM_CONFIG", configPath)

	origArgs := os.Args
	os.Args = []string{"osm"}
	t.Cleanup(func() { os.Args = origArgs })

	stdoutFile := newTempFile(t)
	stderrFile := newTempFile(t)
	stdoutPath := stdoutFile.Name()
	stderrPath := stderrFile.Name()

	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = stdoutFile
	os.Stderr = stderrFile

	err := run()

	os.Stdout = origStdout
	os.Stderr = origStderr

	if closeErr := stdoutFile.Close(); closeErr != nil {
		t.Fatalf("close stdout: %v", closeErr)
	}
	if closeErr := stderrFile.Close(); closeErr != nil {
		t.Fatalf("close stderr: %v", closeErr)
	}

	stdoutData, readErr := os.ReadFile(stdoutPath)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	stderrData, readErr := os.ReadFile(stderrPath)
	if readErr != nil {
		t.Fatalf("read stderr: %v", readErr)
	}

	stdout := strings.TrimSpace(string(stdoutData))
	stderr := strings.TrimSpace(string(stderrData))

	// run() should still succeed — it falls back to NewConfig().
	if err != nil {
		t.Fatalf("run returned error: %v (stdout=%q stderr=%q)", err, stdout, stderr)
	}
	// With no args, help is shown.
	if !strings.Contains(stdout, "Available commands") {
		t.Fatalf("expected help output, got stdout=%q stderr=%q", stdout, stderr)
	}
}

// TestRun_CommandFlagParseNonErrHelp covers the non-ErrHelp error path
// in command-level flag parsing. When a command receives an unknown flag,
// fs.Parse returns an error that is NOT flag.ErrHelp, and run() returns it.
func TestRun_CommandFlagParseNonErrHelp(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "version", "--nonexistent-flag"})
	if err == nil {
		t.Fatalf("expected error for unknown command flag, got nil (stdout=%q stderr=%q)", stdout, stderr)
	}
	// The error should mention the unknown flag.
	if !strings.Contains(err.Error(), "nonexistent-flag") {
		t.Fatalf("expected error to mention unknown flag, got: %v", err)
	}
}
