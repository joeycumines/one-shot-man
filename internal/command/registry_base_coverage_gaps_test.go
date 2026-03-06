package command

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// ─────────────────────────────────────────────────────────────────────
// base.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

// writeExecScript writes content to path with 0755 using the
// write-to-temp-then-rename pattern. On Linux, execve can race with
// write-back and return ETXTBSY ("text file busy") even after f.Sync+
// f.Close, because the kernel may still have the inode's page cache
// marked as "open for write" for a brief window. Renaming from a temp
// file to the final name avoids this because the final inode was never
// opened for writing from the kernel's perspective.
func writeExecScript(t *testing.T, path, content string) {
	t.Helper()
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	// Sync the directory to flush metadata. On Docker overlayfs, exec can
	// still see ETXTBSY unless the rename's directory entry is flushed.
	if d, err := os.Open(filepath.Dir(path)); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

func TestBaseCommand_SetupFlags_NoOp(t *testing.T) {
	t.Parallel()
	bc := NewBaseCommand("test", "A test command", "test [options]")
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	bc.SetupFlags(fs) // default no-op
	// No flags should be registered
	count := 0
	fs.VisitAll(func(*flag.Flag) { count++ })
	if count != 0 {
		t.Fatalf("expected 0 flags from default SetupFlags, got %d", count)
	}
}

func TestBaseCommand_Accessors(t *testing.T) {
	t.Parallel()
	bc := NewBaseCommand("myname", "mydesc", "myusage")
	if bc.Name() != "myname" {
		t.Fatalf("Name() = %q", bc.Name())
	}
	if bc.Description() != "mydesc" {
		t.Fatalf("Description() = %q", bc.Description())
	}
	if bc.Usage() != "myusage" {
		t.Fatalf("Usage() = %q", bc.Usage())
	}
}

// ─────────────────────────────────────────────────────────────────────
// registry.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestRegistry_AddScriptPath_Duplicate(t *testing.T) {
	t.Parallel()
	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{"/path/a"},
	}
	// Add duplicate — should be ignored
	r.AddScriptPath("/path/a")
	if len(r.scriptPaths) != 1 {
		t.Fatalf("expected 1 script path after duplicate add, got %d: %v",
			len(r.scriptPaths), r.scriptPaths)
	}
	// Add non-duplicate
	r.AddScriptPath("/path/b")
	if len(r.scriptPaths) != 2 {
		t.Fatalf("expected 2 script paths, got %d: %v",
			len(r.scriptPaths), r.scriptPaths)
	}
}

func TestRegistry_FindScriptCommand_RealScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell scripts")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "my-tool")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
	}

	cmd, err := r.Get("my-tool")
	if err != nil {
		t.Fatalf("Get my-tool: %v", err)
	}
	if cmd.Name() != "my-tool" {
		t.Fatalf("expected name 'my-tool', got %q", cmd.Name())
	}
}

func TestRegistry_FindScriptCommands_Mixed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions")
	}
	t.Parallel()

	dir := t.TempDir()
	// Create an executable script
	if err := os.WriteFile(filepath.Join(dir, "runnable"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	// Create a non-executable file
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a directory (should be skipped)
	if err := os.MkdirAll(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
	}

	scripts := r.listScript()
	found := false
	for _, n := range scripts {
		if n == "runnable" {
			found = true
		}
		if n == "data.txt" {
			t.Fatal("non-executable file should not appear in script list")
		}
		if n == "subdir" {
			t.Fatal("directory should not appear in script list")
		}
	}
	if !found {
		t.Fatalf("expected 'runnable' in script list, got: %v", scripts)
	}
}

func TestRegistry_List_Deduplication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions")
	}
	t.Parallel()

	dir := t.TempDir()
	// Create a script called "test-cmd" in the script path
	if err := os.WriteFile(filepath.Join(dir, "test-cmd"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	r := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir, dir}, // duplicate path
	}
	// Also register a built-in with a different name
	r.Register(NewTestCommand("alpha", "desc", "usage"))

	names := r.List()
	count := 0
	for _, n := range names {
		if n == "test-cmd" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 'test-cmd' exactly once in list, got %d in %v", count, names)
	}
}

func TestRemoveDuplicates_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{"nil", nil, nil},
		{"empty", []string{}, []string{}},
		{"single", []string{"a"}, []string{"a"}},
		{"no_dups", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"all_same", []string{"x", "x", "x"}, []string{"x"}},
		{"adjacent", []string{"a", "a", "b", "b", "c"}, []string{"a", "b", "c"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := removeDuplicates(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("removeDuplicates(%v) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("removeDuplicates(%v)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestScriptCommand_Execute_RunsScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "hello")
	writeExecScript(t, scriptPath, "#!/bin/sh\necho \"hello world\"\n")

	cmd := newScriptCommand("hello", scriptPath)
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}
}

func TestScriptCommand_Execute_PassesArgs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "echo-args")
	writeExecScript(t, scriptPath, "#!/bin/sh\necho \"$@\"\n")

	cmd := newScriptCommand("echo-args", scriptPath)
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"foo", "bar"}, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "foo bar" {
		t.Fatalf("expected 'foo bar', got %q", got)
	}
}

func TestScriptCommand_Execute_CapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "stderr-test")
	writeExecScript(t, scriptPath, "#!/bin/sh\necho err >&2\n")

	cmd := newScriptCommand("stderr-test", scriptPath)
	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Use Contains rather than exact match: when parallel tests delete their
	// temp dirs, the shell may emit "shell-init: error retrieving current
	// directory: getcwd: ..." before our script output.
	if got := stderr.String(); !strings.Contains(got, "err") {
		t.Fatalf("expected stderr to contain 'err', got %q", got)
	}
}

func TestScriptCommand_Execute_ExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fail")
	writeExecScript(t, scriptPath, "#!/bin/sh\nexit 42\n")

	cmd := newScriptCommand("fail", scriptPath)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
	if !strings.Contains(err.Error(), "exit status 42") {
		t.Fatalf("expected exit status 42 in error, got: %v", err)
	}
}

func TestScriptCommand_Execute_NotFound(t *testing.T) {
	t.Parallel()

	cmd := newScriptCommand("noexist", "/nonexistent/path/to/script")
	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for nonexistent script")
	}
}

func TestScriptCommand_ExecuteWithContext_Cancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "sleeper")
	writeExecScript(t, scriptPath, "#!/bin/sh\nsleep 60\n")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := newScriptCommand("sleeper", scriptPath)
	var stdout, stderr bytes.Buffer

	done := make(chan error, 1)
	go func() {
		done <- cmd.ExecuteWithContext(ctx, nil, &stdout, &stderr)
	}()

	// Give the process time to start
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from cancelled context")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for process to terminate")
	}
}

func TestScriptCommand_ExecuteWithContext_NormalCompletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "quick")
	writeExecScript(t, scriptPath, "#!/bin/sh\necho done\n")

	ctx := context.Background()
	cmd := newScriptCommand("quick", scriptPath)
	var stdout, stderr bytes.Buffer
	if err := cmd.ExecuteWithContext(ctx, nil, &stdout, &stderr); err != nil {
		t.Fatalf("ExecuteWithContext: %v", err)
	}
	if got := strings.TrimSpace(stdout.String()); got != "done" {
		t.Fatalf("expected 'done', got %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// builtin.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestHelpCommand_ScriptCommandHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions for executable script")
	}
	t.Parallel()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "my-script")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	registry := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
	}
	helper := NewHelpCommand(registry)
	registry.Register(helper)

	var stdout, stderr bytes.Buffer
	if err := helper.Execute([]string{"my-script"}, &stdout, &stderr); err != nil {
		t.Fatalf("help my-script: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Note: this is a script command") {
		t.Fatalf("expected script command hint in output, got: %q", output)
	}
	if !strings.Contains(output, "my-script -h") {
		t.Fatalf("expected 'my-script -h' in hint, got: %q", output)
	}
}

func TestHelpCommand_ListsScriptCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix file permissions")
	}
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "discovered-script"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	registry := &Registry{
		commands:    make(map[string]Command),
		scriptPaths: []string{dir},
	}
	helper := NewHelpCommand(registry)
	registry.Register(helper)

	var stdout, stderr bytes.Buffer
	if err := helper.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("help: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Script commands:") {
		t.Fatalf("expected 'Script commands:' section, got: %q", output)
	}
	if !strings.Contains(output, "discovered-script") {
		t.Fatalf("expected 'discovered-script' in output, got: %q", output)
	}
}

func TestConfigCommand_ShowGlobal(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("editor", "vim")

	cmd := NewConfigCommand(cfg)
	cmd.showGlobal = true

	var stdout bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("config --global: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Global configuration:") {
		t.Fatalf("expected 'Global configuration:' header, got: %q", output)
	}
	if !strings.Contains(output, "editor: vim") {
		t.Fatalf("expected 'editor: vim' in output, got: %q", output)
	}
}

func TestConfigCommand_GetEmptyStringValue(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("mykey", "")

	cmd := NewConfigCommand(cfg)
	var stdout bytes.Buffer
	if err := cmd.Execute([]string{"mykey"}, &stdout, &bytes.Buffer{}); err != nil {
		t.Fatalf("config mykey: %v", err)
	}

	output := stdout.String()
	// Should show the key with empty value rather than "not found"
	if strings.Contains(output, "not found") {
		t.Fatalf("expected empty value to be displayed, not 'not found': %q", output)
	}
	if !strings.Contains(output, "mykey:") {
		t.Fatalf("expected 'mykey:' in output, got: %q", output)
	}
}

func TestConfigCommand_SetWithoutConfigPath_FallbackResolve(t *testing.T) {
	// Test the best-effort path resolution when no configPath is set.
	// Use OSM_CONFIG env to provide a path.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	t.Setenv("OSM_CONFIG", configPath)

	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg) // no configPath

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"testkey", "testval"}, &stdout, &stderr); err != nil {
		t.Fatalf("config set: %v", err)
	}

	if !strings.Contains(stdout.String(), "Set configuration: testkey = testval") {
		t.Fatalf("expected confirmation, got: %q", stdout.String())
	}
}

func TestConfigCommand_SetPersistFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping as root")
	}
	t.Parallel()

	// Create a read-only directory so SetKeyInFile fails
	dir := t.TempDir()
	readonlyDir := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(readonlyDir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonlyDir, 0755) })

	configPath := filepath.Join(readonlyDir, "config")

	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"broken", "value"}, &stdout, &stderr); err != nil {
		t.Fatalf("config set should not return error (persist is best-effort): %v", err)
	}

	// Should still confirm the in-memory set
	if !strings.Contains(stdout.String(), "Set configuration: broken = value") {
		t.Fatalf("expected confirmation, got: %q", stdout.String())
	}
	// Should warn about persistence failure
	if !strings.Contains(stderr.String(), "Warning: failed to persist") {
		t.Fatalf("expected persistence warning in stderr, got: %q", stderr.String())
	}
}

func TestInitCommand_ForceOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	if err := os.WriteFile(configPath, []byte("old content"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OSM_CONFIG", configPath)

	cmd := NewInitCommand()
	cmd.force = true

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("init --force: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "old content" {
		t.Fatal("expected config to be overwritten")
	}
	if !strings.Contains(string(data), "# osm configuration file") {
		t.Fatalf("expected default config content, got: %q", string(data))
	}
}

func TestInitCommand_LoadedConfigTest(t *testing.T) {
	// Test that init command verifies the created config by loading it.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")
	t.Setenv("OSM_CONFIG", configPath)

	cmd := NewInitCommand()
	cmd.force = true

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute(nil, &stdout, &stderr); err != nil {
		t.Fatalf("init --force: %v", err)
	}

	output := stdout.String()
	// The init command should print verbose= and pager= messages
	if !strings.Contains(output, "verbose=") {
		t.Fatalf("expected verbose confirmation, got: %q", output)
	}
	if !strings.Contains(output, "pager:") {
		t.Fatalf("expected pager confirmation, got: %q", output)
	}
}

// ─────────────────────────────────────────────────────────────────────
// testutils.go coverage gap
// ─────────────────────────────────────────────────────────────────────

func TestAnsiRegex_StripsSequences(t *testing.T) {
	t.Parallel()
	input := "\x1b[31mred\x1b[0m normal \x1b[1;34mbold blue\x1b[0m"
	got := ansiRegex.ReplaceAllString(input, "")
	if got != "red normal bold blue" {
		t.Fatalf("expected stripped text, got: %q", got)
	}
}

func TestAnsiRegex_NoChange(t *testing.T) {
	t.Parallel()
	input := "plain text without ansi"
	got := ansiRegex.ReplaceAllString(input, "")
	if got != input {
		t.Fatalf("expected unchanged text, got: %q", got)
	}
}

// ─────────────────────────────────────────────────────────────────────
// isExecutable coverage for Windows-style extensions (on any platform)
// ─────────────────────────────────────────────────────────────────────

func TestIsExecutable_WindowsExtensions(t *testing.T) {
	// On non-Windows, isExecutable checks mode bits, not extensions.
	// On Windows, it checks extensions. This test verifies the extension
	// check logic is correct by testing the code paths directly.
	t.Parallel()

	if runtime.GOOS == "windows" {
		// On Windows, test .com, .bat, .cmd — all should be executable
		dir := t.TempDir()
		for _, ext := range []string{".exe", ".com", ".bat", ".cmd"} {
			path := filepath.Join(dir, "test"+ext)
			if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(path)
			if !isExecutable(info) {
				t.Errorf("expected %s to be executable on Windows", ext)
			}
		}
		// .ps1 and .sh should NOT be executable
		for _, ext := range []string{".ps1", ".sh", ".py"} {
			path := filepath.Join(dir, "test"+ext)
			if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(path)
			if isExecutable(info) {
				t.Errorf("expected %s to NOT be executable on Windows", ext)
			}
		}
	} else {
		// On Unix, mode bits determine executability — test various modes
		dir := t.TempDir()

		// No execute bits
		noExec := filepath.Join(dir, "no-exec")
		if err := os.WriteFile(noExec, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		infoNoExec, _ := os.Stat(noExec)
		if isExecutable(infoNoExec) {
			t.Error("expected file without exec bits to not be executable")
		}

		// User execute bit only
		userExec := filepath.Join(dir, "user-exec")
		if err := os.WriteFile(userExec, []byte("x"), 0100); err != nil {
			t.Fatal(err)
		}
		infoUserExec, _ := os.Stat(userExec)
		if !isExecutable(infoUserExec) {
			t.Error("expected file with user exec bit to be executable")
		}

		// Group execute bit only
		groupExec := filepath.Join(dir, "group-exec")
		if err := os.WriteFile(groupExec, []byte("x"), 0010); err != nil {
			t.Fatal(err)
		}
		infoGroupExec, _ := os.Stat(groupExec)
		if !isExecutable(infoGroupExec) {
			t.Error("expected file with group exec bit to be executable")
		}

		// Other execute bit only
		otherExec := filepath.Join(dir, "other-exec")
		if err := os.WriteFile(otherExec, []byte("x"), 0001); err != nil {
			t.Fatal(err)
		}
		infoOtherExec, _ := os.Stat(otherExec)
		if !isExecutable(infoOtherExec) {
			t.Error("expected file with other exec bit to be executable")
		}
	}
}

// ─────────────────────────────────────────────────────────────────────
// registry_unix.go / registry_windows.go coverage
// ─────────────────────────────────────────────────────────────────────

func TestScriptCommand_KillProcessGroup_NilProcess(t *testing.T) {
	t.Parallel()
	// killProcessGroup should handle cmd.Process == nil gracefully
	sc := newScriptCommand("test", "/nonexistent")
	execCmd := &exec.Cmd{} // Process is nil (never started)
	sc.killProcessGroup(execCmd)
	// Should not panic — early return
}
