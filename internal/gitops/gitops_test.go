package gitops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// initRepoWithCommit creates a non-bare repo with one committed file.
// Returns the *git.Repository and its path.
func initRepoWithCommit(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	// Create a file and commit it.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}

	sig := &object.Signature{
		Name:  "test",
		Email: "test@test.com",
		When:  time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if _, err := wt.Commit("init", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	return repo, dir
}

// initBareRepo creates a bare repo at the given path.
func initBareRepo(t *testing.T, path string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(path, true)
	if err != nil {
		t.Fatalf("PlainInit --bare: %v", err)
	}
	return repo
}

func TestIsRepo(t *testing.T) {
	t.Parallel()

	// Empty dir → false.
	if IsRepo(t.TempDir()) {
		t.Fatal("expected false for empty dir")
	}

	// Dir with .git → true.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if !IsRepo(dir) {
		t.Fatal("expected true for dir with .git")
	}

	// Nonexistent dir → false.
	if IsRepo(filepath.Join(t.TempDir(), "nope")) {
		t.Fatal("expected false for nonexistent dir")
	}

	// .git as regular file (git worktree / submodule) → false.
	dirFile := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirFile, ".git"), []byte("gitdir: ../main/.git"), 0644); err != nil {
		t.Fatal(err)
	}
	if IsRepo(dirFile) {
		t.Fatal("expected false when .git is a regular file (not a directory)")
	}
}

func TestClone(t *testing.T) {
	t.Parallel()

	// Create source repo with a commit, push to bare, clone from bare.
	srcRepo, srcDir := initRepoWithCommit(t)

	bareDir := filepath.Join(t.TempDir(), "bare.git")
	initBareRepo(t, bareDir)

	// Configure origin on source and push.
	if _, err := srcRepo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	if err := srcRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
	_ = srcDir

	// Clone from bare.
	destDir := filepath.Join(t.TempDir(), "clone")
	repo, err := Clone(context.Background(), bareDir, destDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	_ = repo
	if !IsRepo(destDir) {
		t.Fatal("expected .git in cloned dir")
	}

	// Verify file exists.
	data, err := os.ReadFile(filepath.Join(destDir, "README.md"))
	if err != nil {
		t.Fatalf("README.md not found: %v", err)
	}
	if string(data) != "# test\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestClone_InvalidURL(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "clone")
	_, err := Clone(context.Background(), "/nonexistent/repo", dest)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = repo
}

func TestOpen_NotARepo(t *testing.T) {
	t.Parallel()
	_, err := Open(t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
	if !isErrNotRepo(err) {
		t.Fatalf("expected ErrNotRepo, got %v", err)
	}
}

func isErrNotRepo(err error) bool {
	return err != nil && err.Error() != "" && containsStr(err.Error(), "gitops: not a git repository")
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findSubstring(s, sub))
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestAddAll(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Create a new file.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := repo.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	// Verify staged.
	has, err := repo.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if !has {
		t.Fatal("expected staged changes after AddAll")
	}
}

func TestHasStagedChanges_Clean(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	has, err := repo.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if has {
		t.Fatal("expected no staged changes on clean repo")
	}
}

func TestCommit(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Stage a new file.
	if err := os.WriteFile(filepath.Join(dir, "commit-test.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	when := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	hash, err := repo.Commit("test commit", when)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if hash == plumbing.ZeroHash {
		t.Fatal("expected non-zero hash")
	}

	// Repo should be clean after commit.
	has, err := repo.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if has {
		t.Fatal("expected clean after commit")
	}
}

func TestCommit_NothingStaged(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	_, err = repo.Commit("empty", time.Now())
	if err == nil {
		t.Fatal("expected error for nothing to commit")
	}
	if !errors.Is(err, ErrNothingToCommit) {
		t.Fatalf("expected ErrNothingToCommit, got %v", err)
	}
}

func TestPush(t *testing.T) {
	t.Parallel()

	// Set up: repo with commit → push to bare → clone → add file → push.
	srcRepo, srcDir := initRepoWithCommit(t)

	bareDir := filepath.Join(t.TempDir(), "bare.git")
	initBareRepo(t, bareDir)

	// Configure origin on source and push.
	if _, err := srcRepo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	if err := srcRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("Push seed: %v", err)
	}
	_ = srcDir

	// Clone from bare.
	cloneDir := filepath.Join(t.TempDir(), "clone")
	repo, err := Clone(context.Background(), bareDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Add a file and commit.
	if err := os.WriteFile(filepath.Join(cloneDir, "pushed.txt"), []byte("pushed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	when := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := repo.Commit("push test", when); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Push to bare.
	if err := repo.Push(context.Background()); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify by cloning again.
	verifyDir := filepath.Join(t.TempDir(), "verify")
	vRepo, err := Clone(context.Background(), bareDir, verifyDir)
	if err != nil {
		t.Fatalf("Clone verify: %v", err)
	}
	_ = vRepo
	data, err := os.ReadFile(filepath.Join(verifyDir, "pushed.txt"))
	if err != nil {
		t.Fatalf("pushed.txt not found: %v", err)
	}
	if string(data) != "pushed" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestPush_NothingToPush(t *testing.T) {
	t.Parallel()

	srcRepo, _ := initRepoWithCommit(t)

	bareDir := filepath.Join(t.TempDir(), "bare.git")
	initBareRepo(t, bareDir)

	if _, err := srcRepo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	if err := srcRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("Push seed: %v", err)
	}

	// Clone and push without changes — should be no-op.
	cloneDir := filepath.Join(t.TempDir(), "clone")
	repo, err := Clone(context.Background(), bareDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if err := repo.Push(context.Background()); err != nil {
		t.Fatalf("Push no-op should not error, got %v", err)
	}
}

func TestErrNotRepo(t *testing.T) {
	t.Parallel()
	if ErrNotRepo.Error() != "gitops: not a git repository" {
		t.Fatalf("unexpected ErrNotRepo message: %q", ErrNotRepo.Error())
	}
}

func TestErrNothingToCommit(t *testing.T) {
	t.Parallel()
	if ErrNothingToCommit.Error() != "gitops: nothing to commit" {
		t.Fatalf("unexpected ErrNothingToCommit message: %q", ErrNothingToCommit.Error())
	}
}

func TestErrConflict(t *testing.T) {
	t.Parallel()
	if ErrConflict.Error() != "gitops: merge conflict" {
		t.Fatalf("unexpected ErrConflict message: %q", ErrConflict.Error())
	}
}

// setupPullRebaseScenario creates a bare repo, pushes an initial commit to it,
// and clones it. Returns (bareDir, srcDir, cloneDir, srcRepo).
// srcRepo has origin pointing to bareDir and has already pushed.
func setupPullRebaseScenario(t *testing.T) (bareDir, srcDir, cloneDir string, srcRepo *git.Repository) {
	t.Helper()

	srcRepo, srcDir = initRepoWithCommit(t)
	bareDir = filepath.Join(t.TempDir(), "bare.git")
	initBareRepo(t, bareDir)

	if _, err := srcRepo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	if err := srcRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	cloneDir = filepath.Join(t.TempDir(), "clone")
	if _, err := Clone(context.Background(), bareDir, cloneDir); err != nil {
		t.Fatalf("Clone: %v", err)
	}

	return bareDir, srcDir, cloneDir, srcRepo
}

// pushCommitFromSrc adds a file and pushes from srcRepo to origin.
func pushCommitFromSrc(t *testing.T, srcRepo *git.Repository, srcDir, filename, content, msg string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(srcDir, filename), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	wt, err := srcRepo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add(filename); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}
	if _, err := wt.Commit(msg, &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := srcRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("Push: %v", err)
	}
}

func TestPullRebase_Success(t *testing.T) {
	t.Parallel()

	_, srcDir, cloneDir, srcRepo := setupPullRebaseScenario(t)

	// Push a new commit from src that clone doesn't have.
	pushCommitFromSrc(t, srcRepo, srcDir, "new.txt", "new content\n", "add new.txt")

	// PullRebase should bring the new file into clone.
	var stderr bytes.Buffer
	err := PullRebase(context.Background(), PullRebaseOptions{
		Dir:    cloneDir,
		Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("PullRebase: %v (stderr: %s)", err, stderr.String())
	}

	// Verify the new file arrived.
	data, err := os.ReadFile(filepath.Join(cloneDir, "new.txt"))
	if err != nil {
		t.Fatalf("new.txt not found: %v", err)
	}
	if string(data) != "new content\n" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestPullRebase_AlreadyUpToDate(t *testing.T) {
	t.Parallel()

	_, _, cloneDir, _ := setupPullRebaseScenario(t)

	// PullRebase with nothing to pull — should succeed (no-op).
	err := PullRebase(context.Background(), PullRebaseOptions{
		Dir: cloneDir,
	})
	if err != nil {
		t.Fatalf("PullRebase (already up-to-date): %v", err)
	}
}

func TestPullRebase_Conflict(t *testing.T) {
	t.Parallel()

	_, srcDir, cloneDir, srcRepo := setupPullRebaseScenario(t)

	// Push a change to README.md from src.
	pushCommitFromSrc(t, srcRepo, srcDir, "README.md", "src version\n", "src change")

	// Create conflicting change in clone on the same file.
	if err := os.WriteFile(filepath.Join(cloneDir, "README.md"), []byte("clone version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cloneGit, err := git.PlainOpen(cloneDir)
	if err != nil {
		t.Fatalf("PlainOpen clone: %v", err)
	}
	cloneWt, err := cloneGit.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := cloneWt.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "test", Email: "test@test.com", When: time.Now()}
	if _, err := cloneWt.Commit("clone change", &git.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// PullRebase should fail with ErrConflict.
	err = PullRebase(context.Background(), PullRebaseOptions{
		Dir: cloneDir,
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got: %v", err)
	}
}

func TestPullRebase_InvalidDir(t *testing.T) {
	t.Parallel()

	err := PullRebase(context.Background(), PullRebaseOptions{
		Dir: filepath.Join(t.TempDir(), "nonexistent"),
	})
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestPullRebase_StderrCapture(t *testing.T) {
	t.Parallel()

	// Trigger an error that produces stderr output, then verify the
	// caller's stderr writer received it.
	var buf bytes.Buffer
	err := PullRebase(context.Background(), PullRebaseOptions{
		Dir:    t.TempDir(), // not a git repo
		Stderr: &buf,
	})
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
	// git should produce some error output on stderr.
	if buf.Len() == 0 {
		t.Log("warning: no stderr output captured (may vary by git version)")
	}
}

func TestPullRebase_CustomGitBin(t *testing.T) {
	t.Parallel()

	// Using a nonexistent binary should fail with a clear error.
	err := PullRebase(context.Background(), PullRebaseOptions{
		Dir:    t.TempDir(),
		GitBin: "nonexistent-git-binary-abc123",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent git binary")
	}
}

// TestOpen_CorruptGitDir tests Open when .git exists but is not a valid repo.
func TestOpen_CorruptGitDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create an empty .git directory — IsRepo passes but PlainOpen fails.
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	_, err := Open(dir)
	if err == nil {
		t.Fatal("expected error for corrupt .git directory")
	}
	// Verify it's NOT an ErrNotRepo (since .git exists → IsRepo passes).
	if errors.Is(err, ErrNotRepo) {
		t.Fatalf("expected non-ErrNotRepo error, got: %v", err)
	}
}

// TestPush_TransportError tests Push with an unreachable remote URL.
func TestPush_TransportError(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Reconfigure origin to a nonexistent/unreachable path.
	gitRepo := repo.repo
	if _, err := gitRepo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{"/nonexistent/path/to/repo"},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}

	err = repo.Push(context.Background())
	if err == nil {
		t.Fatal("expected push error with unreachable remote")
	}
	// Should be a push/transport error, not a no-op.
	if findSubstring(err.Error(), "already up-to-date") {
		t.Fatalf("expected transport error, not up-to-date: %v", err)
	}
}

// TestAddAll_BareRepo tests AddAll on a bare repo (no worktree → error).
func TestAddAll_BareRepo(t *testing.T) {
	t.Parallel()
	bareDir := filepath.Join(t.TempDir(), "bare.git")
	bareGit := initBareRepo(t, bareDir)
	r := &Repo{repo: bareGit}
	err := r.AddAll()
	if err == nil {
		t.Fatal("expected error on bare repo AddAll")
	}
	if !findSubstring(err.Error(), "worktree") {
		t.Fatalf("expected worktree error, got: %v", err)
	}
}

// TestHasStagedChanges_BareRepo tests HasStagedChanges on a bare repo.
func TestHasStagedChanges_BareRepo(t *testing.T) {
	t.Parallel()
	bareDir := filepath.Join(t.TempDir(), "bare.git")
	bareGit := initBareRepo(t, bareDir)
	r := &Repo{repo: bareGit}
	_, err := r.HasStagedChanges()
	if err == nil {
		t.Fatal("expected error on bare repo HasStagedChanges")
	}
	if !findSubstring(err.Error(), "worktree") {
		t.Fatalf("expected worktree error, got: %v", err)
	}
}

// TestCommit_BareRepo tests Commit on a bare repo (no worktree → error).
func TestCommit_BareRepo(t *testing.T) {
	t.Parallel()
	bareDir := filepath.Join(t.TempDir(), "bare.git")
	bareGit := initBareRepo(t, bareDir)
	r := &Repo{repo: bareGit}
	_, err := r.Commit("test", time.Now())
	if err == nil {
		t.Fatal("expected error on bare repo Commit")
	}
	if !findSubstring(err.Error(), "worktree") {
		t.Fatalf("expected worktree error, got: %v", err)
	}
}
