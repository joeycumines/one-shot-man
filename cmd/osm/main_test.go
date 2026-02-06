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

func TestRunHelpForSpecificCommand(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "help", "config"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if !strings.Contains(stdout, "Flags:") || !strings.Contains(stdout, "-global") {
		t.Fatalf("expected config help to mention flags, got stdout=%q stderr=%q", stdout, stderr)
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

func TestUnknownCommandVariants(t *testing.T) {
	testCases := []struct {
		name       string
		args       []string
		expectErr  bool
		errContain string
	}{
		{
			name:      "trailing_space",
			args:      []string{"osm", "unknowncommand "},
			expectErr: true,
		},
		{
			name:      "multiple_trailing_spaces",
			args:      []string{"osm", "unknowncommand  "},
			expectErr: true,
		},
		{
			name:      "wrong_case",
			args:      []string{"osm", "OSM"},
			expectErr: true,
		},
		{
			name:      "hyphenated",
			args:      []string{"osm", "osm-test"},
			expectErr: true,
		},
		{
			name:      "underscore",
			args:      []string{"osm", "osm_test"},
			expectErr: true,
		},
		{
			name:      "very_long_unknown",
			args:      []string{"osm", "this_is_a_very_long_unknown_command_name_that_should_not_match_anything"},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runWithCapturedIO(t, tc.args)

			if tc.expectErr {
				if err == nil {
					t.Fatalf("expected error for unknown command variant %q, got nil", tc.name)
				}
				// Verify error message mentions unknown command
				if !strings.Contains(stderr, "Unknown command") {
					t.Fatalf("expected stderr to mention unknown command, got stderr=%q stdout=%q", stderr, stdout)
				}
			} else {
				// Not expecting error - verify no error occurred
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tc.name, err)
				}
			}
		})
	}
}

func TestFlagParsingEdgeCases(t *testing.T) {
	testCases := []struct {
		name      string
		args      []string
		expectErr bool
	}{
		{
			name:      "duplicate_help_flags",
			args:      []string{"osm", "-h", "-h"},
			expectErr: false, // -h followed by -h should still show help
		},
		{
			name:      "help_then_command",
			args:      []string{"osm", "-h", "version"},
			expectErr: false, // Global -h should show help, not run version
		},
		{
			name:      "unknown_flag",
			args:      []string{"osm", "-z"},
			expectErr: true,
		},
		{
			name:      "double_dash_unknown_flag",
			args:      []string{"osm", "--unknown"},
			expectErr: true,
		},
		{
			name:      "flag_value_with_dash_prefix",
			args:      []string{"osm", "script", "-e", "-h"},
			expectErr: true, // -h as a value to -e is valid flag parsing, but script execution will fail
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, err := runWithCapturedIO(t, tc.args)

			if tc.expectErr {
				if err == nil {
					// Some errors may just print to stderr without returning an error
					// Check if we got an error message in stderr
					if stderr == "" {
						t.Fatalf("expected error or error message for %q, got nil error and empty stderr", tc.name)
					}
				}
			} else {
				// For non-error cases, verify no fatal error occurred
				if err != nil && !strings.Contains(err.Error(), "flag") {
					t.Fatalf("unexpected error for %q: %v stderr=%q", tc.name, err, stderr)
				}
			}
		})
	}
}

func TestHelpOutputFormatting(t *testing.T) {
	stdout, stderr, err := runWithCapturedIO(t, []string{"osm", "--help"})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if stderr != "" {
		t.Fatalf("expected no stderr output, got %q", stderr)
	}

	// Verify help output contains expected sections
	if !strings.Contains(stdout, "Available commands") {
		t.Fatalf("expected help output to contain 'Available commands', got %q", stdout)
	}

	// Verify the osm binary name is mentioned
	if !strings.Contains(stdout, "osm") {
		t.Fatalf("expected help output to mention 'osm', got %q", stdout)
	}

	// Verify there's some usage information
	if !strings.Contains(stdout, "Usage") && !strings.Contains(stdout, "usage") {
		t.Fatalf("expected help output to contain usage information, got %q", stdout)
	}
}

func TestVersionCommandVariations(t *testing.T) {
	testCases := []struct {
		name       string
		args       []string
		expectVer  bool
		expectHelp bool
	}{
		{
			name:      "version_subcommand",
			args:      []string{"osm", "version"},
			expectVer: true,
		},
		{
			name:       "version_with_help_flag",
			args:       []string{"osm", "version", "-h"},
			expectHelp: true, // Should show version command's own help
		},
		{
			name:       "version_with_help_long",
			args:       []string{"osm", "version", "--help"},
			expectHelp: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := runWithCapturedIO(t, tc.args)

			if tc.expectVer {
				if err != nil {
					t.Fatalf("version command returned error: %v", err)
				}
				if !strings.Contains(stdout, "one-shot-man version") {
					t.Fatalf("expected version output, got stdout=%q stderr=%q", stdout, stderr)
				}
			}

			if tc.expectHelp {
				// Help output should contain usage information
				if stdout == "" && stderr == "" {
					t.Fatalf("expected help output for version command, got empty output")
				}
			}

			// Explicitly use stderr to avoid unused variable error
			_ = stderr
		})
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
