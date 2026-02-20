package command

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/storage"
)

// ─────────────────────────────────────────────────────────────────────
// session.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestSessionCommand_SetupFlags_RegistersDryRunAndYes(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	if fs.Lookup("dry-run") == nil {
		t.Fatal("expected -dry-run flag to be registered")
	}
	if fs.Lookup("y") == nil {
		t.Fatal("expected -y flag to be registered")
	}
}

func TestSessionCommand_Info_PrintsContent(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	defer storage.ResetPaths()

	id := "info-test"
	content := `{"mode":"code-review","files":["main.go"]}`
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"info", id}, &out, &out); err != nil {
		t.Fatalf("info failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	if got != content {
		t.Fatalf("expected %q, got %q", content, got)
	}
}

func TestSessionCommand_Info_RequiresID(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"info"}, &out, &out)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "info requires a session id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSessionCommand_Info_NonExistent(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	defer storage.ResetPaths()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"info", "nonexistent"}, &out, &out)
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestSessionCommand_Info_Help(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"info", "-h"}, &out, &out)
	if err != nil {
		t.Fatalf("info -h should return nil: %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage in help, got: %q", out.String())
	}
}

func TestSessionCommand_Execute_NoArgs_RunsList(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	defer storage.ResetPaths()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err != nil {
		t.Fatalf("expected nil error for no args, got: %v", err)
	}
	// With no sessions, list prints "No sessions found"
	if !strings.Contains(stdout.String(), "No sessions found") {
		t.Fatalf("expected 'No sessions found' output, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSessionCommand_Execute_UnknownSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"bogus"}, &out, &out)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown subcommand") {
		t.Fatalf("expected 'unknown subcommand', got: %v", err)
	}
}

func TestSessionCommand_List_InvalidFormat(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	defer storage.ResetPaths()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"list", "-format", "xml"}, &out, &out)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Fatalf("expected 'invalid format', got: %v", err)
	}
}

func TestSessionCommand_List_InvalidSort(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)
	defer storage.ResetPaths()

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"list", "-sort", "bogus"}, &out, &out)
	if err == nil {
		t.Fatal("expected error for invalid sort")
	}
	if !strings.Contains(err.Error(), "invalid sort") {
		t.Fatalf("expected 'invalid sort', got: %v", err)
	}
}

func TestSessionCommand_Path_Help(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	err := cmd.Execute([]string{"path", "-h"}, &out, &out)
	if err != nil {
		t.Fatalf("path -h should return nil: %v", err)
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Fatalf("expected usage in help, got: %q", out.String())
	}
}

// ─────────────────────────────────────────────────────────────────────
// sync.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestSyncCommand_SetupFlags_EmptyBody(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(config.NewConfig())
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)
	// SetupFlags is empty; calling it exercises the basic block.
}

func TestSyncCommand_SubcommandHelp(t *testing.T) {
	t.Parallel()

	for _, sub := range []string{"save", "list", "init"} {
		t.Run(sub, func(t *testing.T) {
			t.Parallel()
			cmd := NewSyncCommand(config.NewConfig(), t.TempDir())
			var stdout, stderr bytes.Buffer
			err := cmd.Execute([]string{sub, "-h"}, &stdout, &stderr)
			if err != nil {
				t.Fatalf("%s -h should return nil: %v", sub, err)
			}
		})
	}
}

func TestSyncCommand_TimeNow_Default(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(config.NewConfig())
	now := cmd.timeNow()
	if time.Since(now) > time.Second {
		t.Fatalf("expected recent time, got %v ago", time.Since(now))
	}
}

// writeFakeGit creates a shell script at path that behaves as a fake git
// binary according to the provided script body. Unix only.
func writeFakeGit(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-git")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncCommand_PullConflictDetection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script for fake git")
	}
	t.Parallel()

	dir := t.TempDir()
	fakeGit := writeFakeGit(t, dir, `echo 'CONFLICT (content): Merge conflict in file.txt' >&2
exit 1
`)

	syncRoot := filepath.Join(dir, "sync")
	if err := os.MkdirAll(filepath.Join(syncRoot, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", syncRoot)
	cmd := NewSyncCommand(cfg)
	cmd.GitBin = fakeGit

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from conflicting pull")
	}
	if !strings.Contains(err.Error(), "merge conflicts") {
		t.Fatalf("expected 'merge conflicts' error, got: %v", err)
	}
}

func TestSyncCommand_PullGitFailure_NonConflict(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix shell script for fake git")
	}
	t.Parallel()

	dir := t.TempDir()
	fakeGit := writeFakeGit(t, dir, `echo 'fatal: not a git repository' >&2
exit 128
`)

	syncRoot := filepath.Join(dir, "sync")
	if err := os.MkdirAll(filepath.Join(syncRoot, ".git"), 0755); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", syncRoot)
	cmd := NewSyncCommand(cfg)
	cmd.GitBin = fakeGit

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from git pull failure")
	}
	if !strings.Contains(err.Error(), "git pull failed") {
		t.Fatalf("expected 'git pull failed' error, got: %v", err)
	}
}

func TestSyncCommand_PullCloneFailure(t *testing.T) {
	t.Parallel()

	// No .git dir → gitops.IsRepo returns false → clone path.
	syncRoot := filepath.Join(t.TempDir(), "sync")

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", syncRoot)
	cfg.SetGlobalOption("sync.repository", "/nonexistent/repo.git")
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from clone failure")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Fatalf("expected 'git clone failed' error, got: %v", err)
	}
}

func TestSyncCommand_PushErrorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(t *testing.T) string // returns syncRoot
		errMsg string
	}{
		{
			name: "open_fails_not_real_repo",
			setup: func(t *testing.T) string {
				t.Helper()
				syncRoot := filepath.Join(t.TempDir(), "sync")
				if err := os.MkdirAll(filepath.Join(syncRoot, ".git"), 0755); err != nil {
					t.Fatal(err)
				}
				return syncRoot
			},
			errMsg: "opening sync repo",
		},
		{
			name: "push_fails_no_remote",
			setup: func(t *testing.T) string {
				t.Helper()
				requireGit(t)
				syncRoot := filepath.Join(t.TempDir(), "sync")
				cmds := [][]string{
					{"git", "init", syncRoot},
					{"git", "-C", syncRoot, "config", "user.email", "test@test.com"},
					{"git", "-C", syncRoot, "config", "user.name", "Test"},
				}
				for _, c := range cmds {
					if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
						t.Fatalf("setup cmd %v failed: %v", c, err)
					}
				}
				// Create initial commit so repo is valid.
				if err := os.WriteFile(filepath.Join(syncRoot, "init.txt"), []byte("init"), 0644); err != nil {
					t.Fatal(err)
				}
				for _, c := range [][]string{
					{"git", "-C", syncRoot, "add", "-A"},
					{"git", "-C", syncRoot, "commit", "-m", "init"},
				} {
					if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
						t.Fatalf("setup cmd %v failed: %v", c, err)
					}
				}
				// Create a new file so push has changes to commit.
				if err := os.WriteFile(filepath.Join(syncRoot, "new.txt"), []byte("new"), 0644); err != nil {
					t.Fatal(err)
				}
				return syncRoot
			},
			errMsg: "git push failed",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			syncRoot := tc.setup(t)

			cfg := config.NewConfig()
			cfg.SetGlobalOption("sync.local-path", syncRoot)
			cmd := NewSyncCommand(cfg)
			cmd.TimeNow = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }

			var stdout, stderr bytes.Buffer
			err := cmd.Execute([]string{"push"}, &stdout, &stderr)
			if err == nil {
				t.Fatalf("expected error: %s", tc.errMsg)
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Fatalf("expected %q in error, got: %v", tc.errMsg, err)
			}
		})
	}
}

func TestSyncCommand_InitCloneFails(t *testing.T) {
	t.Parallel()

	syncRoot := filepath.Join(t.TempDir(), "sync")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", syncRoot)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"init", "/nonexistent/repo.git"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error from clone failure")
	}
	if !strings.Contains(err.Error(), "git clone failed") {
		t.Fatalf("expected 'git clone failed' error, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────
// log_tail.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestLogCommand_SetupFlags_AllRegistered(t *testing.T) {
	t.Parallel()
	cmd := NewLogCommand(config.NewConfig())

	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	for _, name := range []string{"f", "follow", "n", "file"} {
		if fs.Lookup(name) == nil {
			t.Errorf("expected -%s flag to be registered", name)
		}
	}
}

func TestLogCommand_TailFollow_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping as root")
	}
	t.Parallel()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "noperm.log")
	if err := os.WriteFile(logPath, []byte("data\n"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(logPath, 0644) })

	cmd := NewLogCommand(config.NewConfig())
	cmd.file = logPath
	cmd.follow = true

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for permission denied file")
	}
}

// ─────────────────────────────────────────────────────────────────────
// diff_splitter.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestExtractFileName_NoPrefix(t *testing.T) {
	t.Parallel()
	line := "not a diff line"
	got := extractFileName(line)
	if got != line {
		t.Fatalf("expected %q, got %q", line, got)
	}
}

func TestExtractFileName_NoBSeparator(t *testing.T) {
	t.Parallel()
	line := "diff --git unusual-format-without-paths"
	got := extractFileName(line)
	if got != "unusual-format-without-paths" {
		t.Fatalf("expected %q, got %q", "unusual-format-without-paths", got)
	}
}

func TestCountLines_EdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  int
	}{
		{"empty", "", 0},
		{"single_no_newline", "hello", 1},
		{"single_with_newline", "hello\n", 1},
		{"two_no_trailing", "a\nb", 2},
		{"two_with_trailing", "a\nb\n", 2},
		{"three_lines", "a\nb\nc\n", 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := countLines(tc.input)
			if got != tc.want {
				t.Fatalf("countLines(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────
// prompt_file.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestFindPromptFiles_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping as root")
	}
	t.Parallel()

	unreadable := filepath.Join(t.TempDir(), "noperm")
	if err := os.MkdirAll(unreadable, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0755) })

	candidates, err := FindPromptFiles(unreadable, false)
	if err != nil {
		t.Fatalf("expected nil error for permission denied, got: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestLoadPromptFile_ParseError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.prompt.md")
	// Unclosed frontmatter triggers parse error.
	if err := os.WriteFile(path, []byte("---\nname: broken\nno closing"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadPromptFile(path)
	if err == nil {
		t.Fatal("expected error for parse failure")
	}
	if !strings.Contains(err.Error(), "failed to parse") {
		t.Fatalf("expected 'failed to parse' error, got: %v", err)
	}
}

func TestParsePromptFile_EmptyQuotedToolsValue(t *testing.T) {
	t.Parallel()
	// tools: "" → parseInlineYAMLList("\"\"") → unquote → empty → nil
	content := "---\ntools: \"\"\n---\nBody.\n"
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pf.Tools) != 0 {
		t.Errorf("expected empty tools for quoted empty value, got %v", pf.Tools)
	}
}

func TestParsePromptFile_LineWithoutColon(t *testing.T) {
	t.Parallel()
	// A frontmatter line without a colon is silently skipped.
	content := "---\nname: test\nthis line has no colon\ndescription: works\n---\nBody.\n"
	pf, err := ParsePromptFile([]byte(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pf.Name != "test" {
		t.Errorf("expected name %q, got %q", "test", pf.Name)
	}
	if pf.Description != "works" {
		t.Errorf("expected description %q, got %q", "works", pf.Description)
	}
}

func TestExpandPromptFileReferences_BrokenLink(t *testing.T) {
	t.Parallel()
	// Opening [ with ]( but no closing ) → left unchanged.
	body := "[text](no-close"
	result := expandPromptFileReferences(body, t.TempDir())
	if result != body {
		t.Fatalf("expected unchanged body, got: %q", result)
	}
}

func TestExpandPromptFileReferences_EmptyPath(t *testing.T) {
	t.Parallel()
	body := "[text]()"
	result := expandPromptFileReferences(body, t.TempDir())
	if result != body {
		t.Fatalf("expected unchanged body for empty path, got: %q", result)
	}
}

// ─────────────────────────────────────────────────────────────────────
// goal_loader.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestResolveGoalScript_PresetScript(t *testing.T) {
	t.Parallel()
	goal := &Goal{
		Name:        "test",
		Description: "Test",
		Script:      "// custom script",
	}
	if err := resolveGoalScript(goal, t.TempDir()); err != nil {
		t.Fatalf("resolveGoalScript: %v", err)
	}
	if goal.Script != "// custom script" {
		t.Fatalf("expected custom script preserved, got %q", goal.Script)
	}
}

func TestResolveGoalScript_DefaultFallback(t *testing.T) {
	t.Parallel()
	goal := &Goal{
		Name:        "test",
		Description: "Test",
	}
	if err := resolveGoalScript(goal, t.TempDir()); err != nil {
		t.Fatalf("resolveGoalScript: %v", err)
	}
	if goal.Script != goalScript {
		t.Fatal("expected default goalScript when Script empty")
	}
}

// ─────────────────────────────────────────────────────────────────────
// goal_registry.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestNewDynamicGoalRegistry_WithPromptFiles(t *testing.T) {
	dir := t.TempDir()

	pf := filepath.Join(dir, "my-check.prompt.md")
	if err := os.WriteFile(pf, []byte("---\nname: my-check\ndescription: A check\n---\nDo the check.\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.paths", dir)
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	builtins := []Goal{{Name: "builtin-1", Description: "B1", Script: "x"}}
	registry := NewDynamicGoalRegistry(builtins, discovery)

	// Prompt file goal should be discovered
	goal, err := registry.Get("my-check")
	if err != nil {
		t.Fatalf("Get my-check: %v; all names: %v", err, registry.List())
	}
	if goal.Category != "prompt-file" {
		t.Errorf("expected category 'prompt-file', got %q", goal.Category)
	}

	// Built-in should still be present
	if _, err := registry.Get("builtin-1"); err != nil {
		t.Fatalf("Get builtin-1: %v", err)
	}
}

func TestNewDynamicGoalRegistry_InvalidGoalFileSkipped(t *testing.T) {
	dir := t.TempDir()

	// Invalid JSON → LoadGoalFromFile logs warning and skips
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.paths", dir)
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	for _, n := range registry.List() {
		if n == "bad" {
			t.Fatal("invalid goal file should not appear in registry")
		}
	}
}

func TestNewDynamicGoalRegistry_InvalidPromptFileSkipped(t *testing.T) {
	dir := t.TempDir()

	// Unclosed frontmatter → LoadPromptFile returns error → skipped
	if err := os.WriteFile(filepath.Join(dir, "broken.prompt.md"), []byte("---\nname: broken\nunclosed"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.paths", dir)
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	for _, n := range registry.List() {
		if n == "broken" {
			t.Fatal("broken prompt file should not appear in registry")
		}
	}
}

func TestDynamicGoalRegistry_Reload_PromptFilePaths(t *testing.T) {
	dir := t.TempDir()
	promptDir := filepath.Join(dir, "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(promptDir, "extra.prompt.md"),
		[]byte("---\nname: extra\ndescription: Extra\n---\nBody"), 0644,
	); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("prompt.file-paths", promptDir)
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	goal, err := registry.Get("extra")
	if err != nil {
		t.Fatalf("expected 'extra' from prompt file paths: %v; names=%v", err, registry.List())
	}
	if goal.Category != "prompt-file" {
		t.Errorf("expected category 'prompt-file', got %q", goal.Category)
	}
}

func TestDynamicGoalRegistry_Reload_DuplicateGoalFirstWins(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Same goal name in both directories — first path wins
	goal1JSON := `{"name":"dupe","description":"From dir1","category":"first"}`
	goal2JSON := `{"name":"dupe","description":"From dir2","category":"second"}`
	if err := os.WriteFile(filepath.Join(dir1, "dupe.json"), []byte(goal1JSON), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "dupe.json"), []byte(goal2JSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("goal.paths", dir1+","+dir2)
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	discovery := NewGoalDiscovery(cfg)
	registry := NewDynamicGoalRegistry(nil, discovery)

	goal, err := registry.Get("dupe")
	if err != nil {
		t.Fatalf("Get dupe: %v", err)
	}
	if goal.Category != "first" {
		t.Errorf("expected first-path category 'first', got %q", goal.Category)
	}
}

// ─────────────────────────────────────────────────────────────────────
// goal_discovery.go coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestGoalDiscovery_AddPath_Empty(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	gd := NewGoalDiscovery(cfg)

	var paths []string
	seen := make(map[string]bool)
	gd.addPath(&paths, seen, "   ")
	if len(paths) != 0 {
		t.Fatalf("expected empty paths for whitespace candidate, got: %v", paths)
	}
}

func TestGoalDiscovery_DiscoverPromptFilePaths_Custom(t *testing.T) {
	dir := t.TempDir()
	promptDir := filepath.Join(dir, "my-prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Resolve symlinks so comparison works on macOS (/var -> /private/var).
	resolvedPromptDir, err := filepath.EvalSymlinks(promptDir)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.NewConfig()
	cfg.SetGlobalOption("goal.disable-standard-paths", "true")
	cfg.SetGlobalOption("prompt.file-paths", promptDir)
	t.Setenv("OSM_DISABLE_GOAL_AUTODISCOVERY", "true")

	gd := NewGoalDiscovery(cfg)
	paths := gd.DiscoverPromptFilePaths()

	found := false
	for _, p := range paths {
		if p == resolvedPromptDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected custom prompt dir %s in paths, got: %v", resolvedPromptDir, paths)
	}
}

// ─────────────────────────────────────────────────────────────────────
// script_discovery.go helper function coverage gaps
// ─────────────────────────────────────────────────────────────────────

func TestSplitPathSegments_Cases(t *testing.T) {
	t.Parallel()

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		if got := splitPathSegments(""); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("single", func(t *testing.T) {
		t.Parallel()
		got := splitPathSegments("scripts")
		if len(got) != 1 || got[0] != "scripts" {
			t.Fatalf("got %v, want [scripts]", got)
		}
	})
}

func TestHasDirPrefix_Cases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		dir  string
		want bool
	}{
		{"exact_match", filepath.FromSlash("/foo/bar"), filepath.FromSlash("/foo/bar"), true},
		{"empty_dir", filepath.FromSlash("/foo/bar"), "", false},
		{"proper_prefix", filepath.FromSlash("/foo/bar/baz"), filepath.FromSlash("/foo/bar"), true},
		{"strict_boundary", filepath.FromSlash("/foo/barista"), filepath.FromSlash("/foo/bar"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hasDirPrefix(tc.path, tc.dir)
			if got != tc.want {
				t.Fatalf("hasDirPrefix(%q, %q) = %v, want %v", tc.path, tc.dir, got, tc.want)
			}
		})
	}
}

func TestPathDepthRelative_Cases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		path string
		base string
		want int
	}{
		{"empty_base", filepath.FromSlash("/foo/bar"), "", 0},
		{"same_dir", filepath.FromSlash("/foo/bar"), filepath.FromSlash("/foo/bar"), 0},
		{"one_down", filepath.FromSlash("/foo/bar/baz"), filepath.FromSlash("/foo/bar"), 1},
		{"two_down", filepath.FromSlash("/foo/bar/baz/qux"), filepath.FromSlash("/foo/bar"), 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pathDepthRelative(tc.path, tc.base)
			if got != tc.want {
				t.Fatalf("pathDepthRelative(%q, %q) = %d, want %d", tc.path, tc.base, got, tc.want)
			}
		})
	}
}

func TestCollectDownSegments_Cases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, nil},
		{"only_up", []string{"..", "..", ".", ""}, nil},
		{"mixed", []string{"..", "..", "scripts", "js"}, []string{"scripts", "js"}},
		{"no_up", []string{"a", "b"}, []string{"a", "b"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := collectDownSegments(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("collectDownSegments(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("collectDownSegments(%v)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestScriptDiscovery_TraverseForGitRepos(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Parallel()

	// Create a directory structure: root/project/ (git repo) with scripts/ dir
	root := t.TempDir()
	repoDir := filepath.Join(root, "project")
	scriptsDir := filepath.Join(repoDir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Init a real git repo
	gitInit := exec.Command("git", "init")
	gitInit.Dir = repoDir
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	sd := &ScriptDiscovery{
		config: &ScriptDiscoveryConfig{
			EnableGitTraversal: true,
			MaxTraversalDepth:  5,
			ScriptPathPatterns: []string{"scripts"},
		},
	}

	// Traverse from the scripts dir (child of repo) upward
	paths := sd.traverseForGitRepos(scriptsDir)

	found := false
	for _, p := range paths {
		if p == scriptsDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scripts dir via git repo traversal, got: %v", paths)
	}
}

func TestScriptDiscovery_TraverseForGitRepos_PermissionDenied(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission tests not reliable on Windows")
	}
	if os.Getuid() == 0 {
		t.Skip("skipping as root")
	}
	t.Parallel()

	dir := t.TempDir()
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(blocked, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0755) })

	sd := &ScriptDiscovery{
		config: &ScriptDiscoveryConfig{
			EnableGitTraversal: true,
			MaxTraversalDepth:  2,
			ScriptPathPatterns: []string{"scripts"},
		},
	}

	// Should not panic — permission denied stops traversal gracefully
	paths := sd.traverseForGitRepos(blocked)
	if len(paths) != 0 {
		t.Fatalf("expected no paths for permission denied start, got: %v", paths)
	}
}

func TestScriptDiscovery_TraverseForGitRepos_NoGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	t.Parallel()

	// A temp dir with no git repos → should find nothing
	dir := t.TempDir()
	scriptsDir := filepath.Join(dir, "project", "scripts")
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}

	sd := &ScriptDiscovery{
		config: &ScriptDiscoveryConfig{
			EnableGitTraversal: true,
			MaxTraversalDepth:  3,
			ScriptPathPatterns: []string{"scripts"},
		},
	}

	paths := sd.traverseForGitRepos(scriptsDir)
	// The traversal may find scripts dirs within git repos that happen to be parents,
	// but in a temp dir there should be none.
	for _, p := range paths {
		if strings.HasPrefix(p, dir) {
			t.Fatalf("unexpected path within test dir: %s", p)
		}
	}
}
