package prsplittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// RunGitCmd executes a git command in dir, failing on error.
// Returns the combined stdout+stderr output.
func RunGitCmd(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %s", args, dir, string(out))
	}
	return string(out)
}

// GitBranchList returns all local branch names in the given repo directory.
func GitBranchList(t testing.TB, dir string) []string {
	t.Helper()
	raw := RunGitCmd(t, dir, "branch", "--list", "--format=%(refname:short)")
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches
}

// FilterPrefix returns only the strings that start with the given prefix.
func FilterPrefix(ss []string, prefix string) []string {
	var out []string
	for _, s := range ss {
		if strings.HasPrefix(s, prefix) {
			out = append(out, s)
		}
	}
	return out
}

// InitTestRepo creates a temp git repo with main + feature branch for
// pr-split end-to-end tests. Returns the repo directory.
//
// The repo has:
//   - main branch with pkg/types.go, cmd/main.go, README.md
//   - feature branch with pkg/impl.go, cmd/run.go, docs/guide.md, docs/api.md
func InitTestRepo(t testing.TB, opts ...InitTestRepoOption) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	cfg := initTestRepoConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	dir := t.TempDir()

	// Initialize repo on main.
	RunGitCmd(t, dir, "init")
	RunGitCmd(t, dir, "symbolic-ref", "HEAD", "refs/heads/main")
	RunGitCmd(t, dir, "config", "user.email", "test@test.com")
	RunGitCmd(t, dir, "config", "user.name", "Test User")

	// Create initial files.
	initialFiles := []struct{ path, content string }{
		{"pkg/types.go", "package pkg\n\ntype Foo struct{}\n"},
		{"cmd/main.go", "package main\n\nfunc main() {}\n"},
		{"README.md", "# Test Project\n"},
	}
	for _, f := range initialFiles {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	RunGitCmd(t, dir, "add", "-A")
	RunGitCmd(t, dir, "commit", "-m", "initial commit")

	if cfg.noFeatureBranch {
		return dir
	}

	// Create feature branch with changes in multiple directories.
	RunGitCmd(t, dir, "checkout", "-b", "feature")
	featureFiles := []struct{ path, content string }{
		{"pkg/impl.go", "package pkg\n\nfunc Bar() string { return \"bar\" }\n"},
		{"cmd/run.go", "package main\n\nfunc run() {}\n"},
		{"docs/guide.md", "# Guide\n\nUsage instructions.\n"},
		{"docs/api.md", "# API\n\nAPI reference.\n"},
	}
	for _, f := range featureFiles {
		full := filepath.Join(dir, f.path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	RunGitCmd(t, dir, "add", "-A")
	RunGitCmd(t, dir, "commit", "-m", "feature work")

	return dir
}

// InitTestRepoOption configures [InitTestRepo].
type InitTestRepoOption func(*initTestRepoConfig)

type initTestRepoConfig struct {
	noFeatureBranch bool
}

// WithNoFeatureBranch creates only the main branch, skipping the feature branch.
func WithNoFeatureBranch() InitTestRepoOption {
	return func(c *initTestRepoConfig) {
		c.noFeatureBranch = true
	}
}
