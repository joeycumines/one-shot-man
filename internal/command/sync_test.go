package command

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// requireGit skips the test if git is not available.
func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// setupBareRepo creates a bare git repo and returns its path.
func setupBareRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	cmd := exec.Command("git", "init", "--bare", bare)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("failed to create bare repo: %v", err)
	}
	return bare
}

// setupCloneWithCommit clones the bare repo, creates an initial commit, and
// pushes it. Returns the clone path.
func setupCloneWithCommit(t *testing.T, bareURL string) string {
	t.Helper()
	clone := filepath.Join(t.TempDir(), "clone")
	cmds := [][]string{
		{"git", "clone", bareURL, clone},
		{"git", "-C", clone, "config", "user.email", "test@test.com"},
		{"git", "-C", clone, "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
			t.Fatalf("setup cmd %v failed: %v", c, err)
		}
	}
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	addCommitPush := [][]string{
		{"git", "-C", clone, "add", "-A"},
		{"git", "-C", clone, "commit", "-m", "init"},
		{"git", "-C", clone, "push", "origin", "HEAD"},
	}
	for _, c := range addCommitPush {
		if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
			t.Fatalf("setup cmd %v failed: %v", c, err)
		}
	}
	return clone
}

func TestSyncCommand_NoSubcommand(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for no subcommand")
	}
	if !strings.Contains(err.Error(), "no subcommand specified") {
		t.Fatalf("expected 'no subcommand' error, got %q", err.Error())
	}
	if !strings.Contains(stderr.String(), "save") {
		t.Fatalf("expected stderr to mention subcommands, got %q", stderr.String())
	}
}

func TestSyncCommand_UnknownSubcommand(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"nope"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown sync subcommand: nope") {
		t.Fatalf("expected 'unknown sync subcommand' error, got %q", err.Error())
	}
}

func TestSyncCommand_SaveRequiresTitle(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"save", "--body", "hello"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "--title is required") {
		t.Fatalf("expected '--title is required' error, got %q", err.Error())
	}
}

func TestSyncCommand_SaveRequiresBody(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"save", "--title", "hello"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing body")
	}
	if !strings.Contains(err.Error(), "--body is required") {
		t.Fatalf("expected '--body is required' error, got %q", err.Error())
	}
}

func TestSyncCommand_SaveCreatesEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := NewSyncCommand(nil, dir)

	// Fix time so filenames are deterministic.
	fixedTime := time.Date(2025, 3, 15, 10, 30, 0, 0, time.UTC)
	cmd.TimeNow = func() time.Time { return fixedTime }

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{
		"save",
		"--title", "My Code Review",
		"--tags", "review,go",
		"--body", "Review the auth module changes.",
	}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("save returned error: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Saved notebook entry:") {
		t.Fatalf("expected confirmation message, got %q", output)
	}

	// Verify the file was created.
	expectedPath := filepath.Join(dir, "2025", "03", "2025-03-15-my-code-review.md")
	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read saved entry: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "---") {
		t.Fatal("expected YAML frontmatter delimiters")
	}
	if !strings.Contains(content, "date: 2025-03-15T10:30:00Z") {
		t.Fatalf("expected date in frontmatter, got %q", content)
	}
	if !strings.Contains(content, "tags: [review, go]") {
		t.Fatalf("expected tags in frontmatter, got %q", content)
	}
	if !strings.Contains(content, `title: "My Code Review"`) {
		t.Fatalf("expected title in frontmatter, got %q", content)
	}
	if !strings.Contains(content, "# My Code Review") {
		t.Fatalf("expected markdown heading, got %q", content)
	}
	if !strings.Contains(content, "Review the auth module changes.") {
		t.Fatalf("expected body text, got %q", content)
	}
}

func TestSyncCommand_SaveDeduplicates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := NewSyncCommand(nil, dir)

	fixedTime := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	cmd.TimeNow = func() time.Time { return fixedTime }

	// Save first entry.
	var stdout1, stderr1 bytes.Buffer
	err := cmd.Execute([]string{"save", "--title", "Duplicate", "--body", "first"}, &stdout1, &stderr1)
	if err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	// Save second entry with same title.
	var stdout2, stderr2 bytes.Buffer
	err = cmd.Execute([]string{"save", "--title", "Duplicate", "--body", "second"}, &stdout2, &stderr2)
	if err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	// Verify both files exist.
	first := filepath.Join(dir, "2025", "06", "2025-06-01-duplicate.md")
	second := filepath.Join(dir, "2025", "06", "2025-06-01-duplicate-2.md")
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("first entry missing: %v", err)
	}
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("second entry missing: %v", err)
	}

	// Verify contents differ.
	d1, _ := os.ReadFile(first)
	d2, _ := os.ReadFile(second)
	if !strings.Contains(string(d1), "first") {
		t.Fatalf("first file should contain 'first', got %q", string(d1))
	}
	if !strings.Contains(string(d2), "second") {
		t.Fatalf("second file should contain 'second', got %q", string(d2))
	}
}

func TestSyncCommand_ListEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := NewSyncCommand(nil, dir)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "No notebook entries found.") {
		t.Fatalf("expected empty message, got %q", stdout.String())
	}
}

func TestSyncCommand_ListNonexistentDir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nonexistent")
	cmd := NewSyncCommand(nil, dir)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}
	if !strings.Contains(stdout.String(), "No notebook entries found.") {
		t.Fatalf("expected empty message for nonexistent dir, got %q", stdout.String())
	}
}

func TestSyncCommand_ListShowsEntries(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := NewSyncCommand(nil, dir)

	// Create entries manually.
	jan := filepath.Join(dir, "2025", "01")
	feb := filepath.Join(dir, "2025", "02")
	if err := os.MkdirAll(jan, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(feb, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jan, "2025-01-10-first.md"), []byte("---\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(feb, "2025-02-20-second.md"), []byte("---\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// Non-md files should be ignored.
	if err := os.WriteFile(filepath.Join(jan, "readme.txt"), []byte("ignore"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"list"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}

	output := stdout.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 entries, got %d: %q", len(lines), output)
	}
	// Reverse chronological — second entry first.
	if !strings.Contains(lines[0], "2025-02-20") {
		t.Fatalf("expected newest entry first, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "second") {
		t.Fatalf("expected 'second' slug, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "2025-01-10") {
		t.Fatalf("expected older entry second, got %q", lines[1])
	}
}

func TestSyncCommand_ListWithLimit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := NewSyncCommand(nil, dir)

	// Create 3 entries.
	month := filepath.Join(dir, "2025", "01")
	if err := os.MkdirAll(month, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2025-01-01-aaa.md", "2025-01-02-bbb.md", "2025-01-03-ccc.md"} {
		if err := os.WriteFile(filepath.Join(month, name), []byte("---\n---\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"list", "--limit", "2"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("list returned error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 entries with limit, got %d: %q", len(lines), stdout.String())
	}
}

func TestSyncCommand_SaveAndList(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cmd := NewSyncCommand(nil, dir)

	fixedTime := time.Date(2025, 7, 4, 9, 0, 0, 0, time.UTC)
	cmd.TimeNow = func() time.Time { return fixedTime }

	// Save an entry.
	var stdout1, stderr1 bytes.Buffer
	if err := cmd.Execute([]string{"save", "--title", "Integration Test", "--body", "Testing save+list."}, &stdout1, &stderr1); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// List and verify it appears.
	var stdout2, stderr2 bytes.Buffer
	if err := cmd.Execute([]string{"list"}, &stdout2, &stderr2); err != nil {
		t.Fatalf("list failed: %v", err)
	}

	output := stdout2.String()
	if !strings.Contains(output, "2025-07-04") {
		t.Fatalf("expected date in list output, got %q", output)
	}
	if !strings.Contains(output, "integration-test") {
		t.Fatalf("expected slug in list output, got %q", output)
	}
}

func TestSyncCommand_Metadata(t *testing.T) {
	t.Parallel()
	if got := NewSyncCommand(nil).Name(); got != "sync" {
		t.Fatalf("expected name 'sync', got %q", got)
	}
	if got := NewSyncCommand(nil).Description(); got == "" {
		t.Fatal("expected non-empty description")
	}
	if got := NewSyncCommand(nil).Usage(); got == "" {
		t.Fatal("expected non-empty usage")
	}
}

// --- Git operations tests ---

func TestSyncCommand_InitFromArg(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	setupCloneWithCommit(t, bare) // need at least one commit in the bare repo

	localPath := filepath.Join(t.TempDir(), "sync-root")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", localPath)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"init", bare}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("init failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sync repository initialized:") {
		t.Fatalf("expected init confirmation, got %q", stdout.String())
	}
	if !isGitRepo(localPath) {
		t.Fatalf("expected .git directory in %s", localPath)
	}
}

func TestSyncCommand_InitFromConfig(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	setupCloneWithCommit(t, bare)

	localPath := filepath.Join(t.TempDir(), "sync-root")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.repository", bare)
	cfg.SetGlobalOption("sync.local-path", localPath)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"init"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("init from config failed: %v\nstderr: %s", err, stderr.String())
	}
	if !isGitRepo(localPath) {
		t.Fatalf("expected .git directory in %s", localPath)
	}
}

func TestSyncCommand_InitNoURL(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"init"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing repository URL")
	}
	if !strings.Contains(err.Error(), "repository URL required") {
		t.Fatalf("expected 'repository URL required', got %q", err.Error())
	}
}

func TestSyncCommand_InitAlreadyInitialized(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	setupCloneWithCommit(t, bare)

	localPath := filepath.Join(t.TempDir(), "sync-root")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", localPath)
	cmd := NewSyncCommand(cfg)

	// First init succeeds.
	var stdout1, stderr1 bytes.Buffer
	if err := cmd.Execute([]string{"init", bare}, &stdout1, &stderr1); err != nil {
		t.Fatalf("first init failed: %v", err)
	}

	// Second init fails.
	var stdout2, stderr2 bytes.Buffer
	err := cmd.Execute([]string{"init", bare}, &stdout2, &stderr2)
	if err == nil {
		t.Fatal("expected error for already initialized")
	}
	if !strings.Contains(err.Error(), "already initialized") {
		t.Fatalf("expected 'already initialized', got %q", err.Error())
	}
}

func TestSyncCommand_PushNoChanges(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	clone := setupCloneWithCommit(t, bare)

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", clone)
	cmd := NewSyncCommand(cfg)
	cmd.TimeNow = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("push failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Nothing to push") {
		t.Fatalf("expected 'Nothing to push', got %q", stdout.String())
	}
}

func TestSyncCommand_PushWithChanges(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	clone := setupCloneWithCommit(t, bare)

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", clone)
	cmd := NewSyncCommand(cfg)
	cmd.TimeNow = func() time.Time { return time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC) }

	// Create a new file in the clone.
	if err := os.WriteFile(filepath.Join(clone, "new-file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("push failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sync push complete.") {
		t.Fatalf("expected push confirmation, got %q", stdout.String())
	}

	// Verify the commit exists by checking log.
	logCmd := exec.Command("git", "-C", clone, "log", "--oneline", "-1")
	logOut, _ := logCmd.Output()
	if !strings.Contains(string(logOut), "osm sync:") {
		t.Fatalf("expected 'osm sync:' in commit log, got %q", string(logOut))
	}
}

func TestSyncCommand_PushNotInitialized(t *testing.T) {
	t.Parallel()

	localPath := filepath.Join(t.TempDir(), "empty")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", localPath)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"push"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for uninitialized directory")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("expected 'not initialized', got %q", err.Error())
	}
}

func TestSyncCommand_PullExisting(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	clone := setupCloneWithCommit(t, bare)

	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", clone)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"pull"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("pull failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sync pull complete.") {
		t.Fatalf("expected pull confirmation, got %q", stdout.String())
	}
}

func TestSyncCommand_PullCloneFromConfig(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	setupCloneWithCommit(t, bare) // need commits in the bare repo

	localPath := filepath.Join(t.TempDir(), "fresh-clone")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.repository", bare)
	cfg.SetGlobalOption("sync.local-path", localPath)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"pull"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("pull/clone failed: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Sync repository cloned:") {
		t.Fatalf("expected clone confirmation, got %q", stdout.String())
	}
	if !isGitRepo(localPath) {
		t.Fatalf("expected .git directory in %s", localPath)
	}
}

func TestSyncCommand_PullNotInitializedNoConfig(t *testing.T) {
	t.Parallel()

	localPath := filepath.Join(t.TempDir(), "empty")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", localPath)
	// No sync.repository configured.
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for no config and not initialized")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("expected 'not initialized', got %q", err.Error())
	}
}

func TestSyncCommand_PushPullRoundTrip(t *testing.T) {
	t.Parallel()
	requireGit(t)

	bare := setupBareRepo(t)
	clone1 := setupCloneWithCommit(t, bare)

	// Create second clone.
	clone2 := filepath.Join(t.TempDir(), "clone2")
	cmds := [][]string{
		{"git", "clone", bare, clone2},
		{"git", "-C", clone2, "config", "user.email", "test@test.com"},
		{"git", "-C", clone2, "config", "user.name", "Test"},
	}
	for _, c := range cmds {
		if err := exec.Command(c[0], c[1:]...).Run(); err != nil {
			t.Fatalf("setup cmd %v failed: %v", c, err)
		}
	}

	// Push a file from clone1.
	if err := os.WriteFile(filepath.Join(clone1, "shared.txt"), []byte("from clone1"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg1 := config.NewConfig()
	cfg1.SetGlobalOption("sync.local-path", clone1)
	cmd1 := NewSyncCommand(cfg1)
	cmd1.TimeNow = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }

	var out1, err1 bytes.Buffer
	if err := cmd1.Execute([]string{"push"}, &out1, &err1); err != nil {
		t.Fatalf("push from clone1 failed: %v", err)
	}

	// Pull into clone2.
	cfg2 := config.NewConfig()
	cfg2.SetGlobalOption("sync.local-path", clone2)
	cmd2 := NewSyncCommand(cfg2)

	var out2, err2 bytes.Buffer
	if err := cmd2.Execute([]string{"pull"}, &out2, &err2); err != nil {
		t.Fatalf("pull into clone2 failed: %v\nstderr: %s", err, err2.String())
	}

	// Verify shared.txt arrived.
	data, err := os.ReadFile(filepath.Join(clone2, "shared.txt"))
	if err != nil {
		t.Fatalf("shared.txt not found in clone2: %v", err)
	}
	if string(data) != "from clone1" {
		t.Fatalf("expected 'from clone1', got %q", string(data))
	}
}

func TestSyncCommand_SyncRootFromConfig(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "custom-sync")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", path)
	cmd := NewSyncCommand(cfg)

	root, err := cmd.syncRoot()
	if err != nil {
		t.Fatalf("syncRoot failed: %v", err)
	}
	if root != path {
		t.Fatalf("expected %q, got %q", path, root)
	}
}

func TestSyncCommand_SyncRootDefault(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil)

	root, err := cmd.syncRoot()
	if err != nil {
		t.Fatalf("syncRoot failed: %v", err)
	}
	if !strings.HasSuffix(root, filepath.Join(".one-shot-man", "sync")) {
		t.Fatalf("expected path ending in .one-shot-man/sync, got %q", root)
	}
}

func TestSyncCommand_NotebooksDirDerivedFromSyncRoot(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "custom-sync")
	cfg := config.NewConfig()
	cfg.SetGlobalOption("sync.local-path", path)
	cmd := NewSyncCommand(cfg)

	nbDir, err := cmd.notebooksDir()
	if err != nil {
		t.Fatalf("notebooksDir failed: %v", err)
	}
	expected := filepath.Join(path, "notebooks")
	if nbDir != expected {
		t.Fatalf("expected %q, got %q", expected, nbDir)
	}
}

func TestIsGitRepo(t *testing.T) {
	t.Parallel()

	// Not a repo.
	if isGitRepo(t.TempDir()) {
		t.Fatal("expected false for empty dir")
	}

	// Create a .git directory.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if !isGitRepo(dir) {
		t.Fatal("expected true for dir with .git")
	}
}

// --- Slugify unit tests ---

func TestSlugify(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"My Code Review", "my-code-review"},
		{"Hello, World!", "hello-world"},
		{"---dashes---", "dashes"},
		{"UPPER CASE", "upper-case"},
		{"a  b  c", "a-b-c"},
		{"", "untitled"},
		{strings.Repeat("a", 100), strings.Repeat("a", 50)},
		{"café résumé", "caf-rsum"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := slugify(tc.input)
			if got != tc.want {
				t.Fatalf("slugify(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseTags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  []string
	}{
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , b , c ", []string{"a", "b", "c"}},
		{"single", []string{"single"}},
		{",,empty,,", []string{"empty"}},
		{"", nil},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := parseTags(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("parseTags(%q) = %v, want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseTags(%q)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}
