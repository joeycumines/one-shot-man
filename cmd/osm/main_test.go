package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunNoArgsShowsHelp(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout, "Available commands") {
		t.Fatalf("expected help output to contain 'Available commands', got %q", stdout)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr output, got %q", stderr)
	}
}

func TestRunHelpFlagDisplaysHelp(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "--help"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout, "Available commands") {
		t.Fatalf("expected help output to contain 'Available commands', got %q", stdout)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr output, got %q", stderr)
	}
}

func TestRunGlobalFlagBeforeCommandShowsGlobalHelp(t *testing.T) {
	// Ensure a global flag consumed before a command doesn't make the
	// program treat the flag token as the command name (bug fixed).
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "-h", "session"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout, "Available commands") {
		t.Fatalf("expected help output to contain 'Available commands', got %q", stdout)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr output, got %q", stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "unknown"})
	if err == nil {
		t.Fatalf("expected error for unknown command")
	}

	if stdout != "" {
		t.Fatalf("expected no stdout output, got %q", stdout)
	}

	if !strings.Contains(stderr, "Unknown command: unknown") {
		t.Fatalf("expected stderr to mention unknown command, got %q", stderr)
	}
}

func TestRunVersionCommand(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "version"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout, "one-shot-man version") {
		t.Fatalf("expected version output, got %q", stdout)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr output, got %q", stderr)
	}
}

func TestRunSessionSubcommandHelp(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "session", "delete", "-h"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stderr, "Usage:") {
		t.Fatalf("expected usage output in stderr, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestRunParseFlagValue(t *testing.T) {
	// This used to fail when the run() logic attempted to split flags
	// manually; a flag that expects a value (e.g. -e) caused the parser
	// to be invoked without the value (parse error "flag needs an argument").
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "script", "-e", "console.log('x')", "extra"})

	// If parsing failed due to missing flag argument the error will
	// contain the phrase "flag needs an argument" — ensure that is
	// not the case.
	if err != nil && strings.Contains(err.Error(), "flag needs an argument") {
		t.Fatalf("unexpected parse error: %v stdout=%q stderr=%q", err, stdout, stderr)
	}
}

func runWithCapturedIO(t *testing.T, args []string) (string, string, error) {
	t.Helper()

	configDir := t.TempDir()
	t.Setenv("OSM_CONFIG", filepath.Join(configDir, "config"))

	origArgs := os.Args
	os.Args = args
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

	if err := stdoutFile.Close(); err != nil {
		t.Fatalf("failed to close stdout file: %v", err)
	}
	if err := stderrFile.Close(); err != nil {
		t.Fatalf("failed to close stderr file: %v", err)
	}

	stdoutData, readOutErr := os.ReadFile(stdoutPath)
	if readOutErr != nil {
		t.Fatalf("failed to read stdout file: %v", readOutErr)
	}
	stderrData, readErrErr := os.ReadFile(stderrPath)
	if readErrErr != nil {
		t.Fatalf("failed to read stderr file: %v", readErrErr)
	}

	return strings.TrimSpace(string(stdoutData)), strings.TrimSpace(string(stderrData)), err
}

func newTempFile(t *testing.T) *os.File {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "osm-io-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	t.Cleanup(func() {
		name := file.Name()
		_ = os.Remove(name)
	})

	return file
}

func TestMainInvokesRun(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("OSM_CONFIG", filepath.Join(configDir, "config"))

	origArgs := os.Args
	os.Args = []string{"osm"}
	t.Cleanup(func() {
		os.Args = origArgs
	})

	stdoutFile := newTempFile(t)
	stderrFile := newTempFile(t)
	tmpOut := stdoutFile.Name()
	tmpErr := stderrFile.Name()

	origStdout := os.Stdout
	origStderr := os.Stderr
	os.Stdout = stdoutFile
	os.Stderr = stderrFile
	t.Cleanup(func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	})

	main()

	if err := stdoutFile.Close(); err != nil {
		t.Fatalf("failed to close stdout file: %v", err)
	}
	if err := stderrFile.Close(); err != nil {
		t.Fatalf("failed to close stderr file: %v", err)
	}

	stdoutData, readOutErr := os.ReadFile(tmpOut)
	if readOutErr != nil {
		t.Fatalf("failed to read stdout file: %v", readOutErr)
	}
	stderrData, readErrErr := os.ReadFile(tmpErr)
	if readErrErr != nil {
		t.Fatalf("failed to read stderr file: %v", readErrErr)
	}

	stdout := strings.TrimSpace(string(stdoutData))
	stderr := strings.TrimSpace(string(stderrData))

	if stdout == "" {
		t.Fatalf("expected stdout to contain help output")
	}
	if stderr != "" {
		t.Fatalf("expected stderr to be empty, got %q", stderr)
	}
}
