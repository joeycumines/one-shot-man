package gitops

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v6"
	gitconfig "github.com/go-git/go-git/v6/config"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// initRepoWithCommit creates a non-bare repo with one committed file on the
// "main" branch. Returns the *git.Repository and its path.
func initRepoWithCommit(t *testing.T) (*git.Repository, string) {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	// Explicitly set HEAD to refs/heads/main so the initial commit creates
	// the "main" branch regardless of the system's default branch name.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName("main"),
	)); err != nil {
		t.Fatalf("SetReference HEAD: %v", err)
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

// initBareRepo creates a bare repo at the given path with HEAD pointing to
// refs/heads/main. Returns the *git.Repository.
func initBareRepo(t *testing.T, path string) *git.Repository {
	t.Helper()
	repo, err := git.PlainInit(path, true)
	if err != nil {
		t.Fatalf("PlainInit --bare: %v", err)
	}
	// Set HEAD to refs/heads/main so clones check out the "main" branch.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName("main"),
	)); err != nil {
		t.Fatalf("SetReference HEAD on bare repo: %v", err)
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
	// Normalize line endings for cross-platform comparison (Windows may use CRLF).
	content := string(data)
	if runtime.GOOS == "windows" {
		content = strings.ReplaceAll(content, "\r\n", "\n")
	}
	if content != "new content\n" {
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

// TestHasStagedChanges_StagedDeletion verifies that deleting a committed file
// and staging the deletion is detected as a staged change.
func TestHasStagedChanges_StagedDeletion(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Remove the committed file from the worktree.
	if err := os.Remove(filepath.Join(dir, "README.md")); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Stage the deletion via AddAll.
	if err := repo.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	has, err := repo.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if !has {
		t.Fatal("expected staged changes after staging a file deletion")
	}
}

// TestHasStagedChanges_MixedUntrackedAndStaged verifies correct behavior when
// both staged changes and untracked files exist simultaneously.
func TestHasStagedChanges_MixedUntrackedAndStaged(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Modify tracked file and stage it.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("modified\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := repo.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}

	// Create an untracked file after AddAll — this file is NOT staged.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("noise"), 0644); err != nil {
		t.Fatal(err)
	}

	// README.md is staged (modified); untracked.txt exists but is untracked.
	// Verify HasStagedChanges returns true despite the untracked file's presence.
	has, err := repo.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if !has {
		// Fetch status for debug output.
		wt, werr := repo.repo.Worktree()
		if werr != nil {
			t.Fatalf("Worktree: %v", werr)
		}
		status, serr := wt.Status()
		if serr != nil {
			t.Fatalf("Status: %v", serr)
		}
		t.Fatalf("expected staged changes; status: %v", status)
	}
}

// TestHasStagedChanges_OnlyUntrackedFiles verifies that only having untracked
// files (without staging them) does NOT count as staged changes.
func TestHasStagedChanges_OnlyUntrackedFiles(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Create untracked file without staging.
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("noise"), 0644); err != nil {
		t.Fatal(err)
	}

	has, err := repo.HasStagedChanges()
	if err != nil {
		t.Fatalf("HasStagedChanges: %v", err)
	}
	if has {
		t.Fatal("expected no staged changes with only untracked files")
	}
}

// TestCommit_SpecialCharactersInMessage verifies that commit messages with
// newlines, quotes, unicode, and other special characters succeed.
func TestCommit_SpecialCharactersInMessage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  string
	}{
		{"embedded newlines", "line1\nline2\nline3"},
		{"CRLF", "line1\r\nline2"},
		{"double quotes", `say "hello"`},
		{"single quotes", "it's a test"},
		{"unicode emoji", "🚀 ship it"},
		{"unicode CJK", "修正バグ"},
		{"backticks", "run `go test`"},
		{"mixed special", "fix: handle $VAR & \"edge\" 'cases' (100%)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, dir := initRepoWithCommit(t)
			repo, err := Open(dir)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			// Stage a unique file for this sub-test.
			fname := "special.txt"
			if err := os.WriteFile(filepath.Join(dir, fname), []byte(tc.msg), 0644); err != nil {
				t.Fatal(err)
			}
			if err := repo.AddAll(); err != nil {
				t.Fatalf("AddAll: %v", err)
			}

			when := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
			hash, err := repo.Commit(tc.msg, when)
			if err != nil {
				t.Fatalf("Commit: %v", err)
			}
			if hash == plumbing.ZeroHash {
				t.Fatal("expected non-zero hash")
			}

			// Verify the message was stored correctly by reading the commit.
			commit, err := repo.repo.CommitObject(hash)
			if err != nil {
				t.Fatalf("CommitObject: %v", err)
			}
			if commit.Message != tc.msg {
				t.Fatalf("commit message mismatch:\n  got:  %q\n  want: %q", commit.Message, tc.msg)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for branch inspection methods: OpenDetect, IsWorkTree, BranchExists,
// DefaultBranch, HeadBranchName.
// ---------------------------------------------------------------------------

// setupCloneWithOriginHEAD creates a source repo with a commit, pushes to a
// bare remote, and clones it. The clone will have origin/HEAD set to point
// to origin/main (go-git's clone does not set this automatically, so we set
// it manually). Returns (cloneDir, srcDir).
func setupCloneWithOriginHEAD(t *testing.T) (cloneDir, srcDir string) {
	t.Helper()

	srcRepo, srcDir := initRepoWithCommit(t)

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

	cloneDir = filepath.Join(t.TempDir(), "clone")
	cloneRepo, err := Clone(context.Background(), bareDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// go-git's clone does not set refs/remotes/origin/HEAD. Set it manually
	// so DefaultBranch() can detect it via the symbolic ref path.
	if err := cloneRepo.repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.NewRemoteHEADReferenceName("origin"),
		plumbing.NewRemoteReferenceName("origin", "main"),
	)); err != nil {
		t.Fatalf("SetReference origin/HEAD: %v", err)
	}

	return cloneDir, srcDir
}

// initRepoWithBranch creates a non-bare repo with one commit on the specified
// branch name. Returns the repo path.
func initRepoWithBranch(t *testing.T, branchName string) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}

	// Set the initial branch to the specified name.
	if err := repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName(branchName),
	)); err != nil {
		t.Fatalf("SetReference HEAD: %v", err)
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

	return dir
}

func TestOpenDetect_NotInRepo(t *testing.T) {
	t.Parallel()

	// An empty temp dir is not a repo.
	_, err := OpenDetect(t.TempDir())
	if err == nil {
		t.Fatal("expected error for non-repo dir")
	}
	if !errors.Is(err, ErrNotRepo) {
		t.Fatalf("expected ErrNotRepo, got: %v", err)
	}
}

func TestOpenDetect_ExactPath(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := OpenDetect(dir)
	if err != nil {
		t.Fatalf("OpenDetect at exact repo root: %v", err)
	}
	_ = repo
}

func TestOpenDetect_Subdirectory(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)

	// Create a subdirectory and open from there.
	subDir := filepath.Join(dir, "sub", "deep")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	repo, err := OpenDetect(subDir)
	if err != nil {
		t.Fatalf("OpenDetect from subdirectory: %v", err)
	}

	// Verify we can use the repo.
	isWT, err := repo.IsWorkTree()
	if err != nil {
		t.Fatalf("IsWorkTree: %v", err)
	}
	if !isWT {
		t.Fatal("expected worktree=true for repo opened from subdirectory")
	}
}

func TestIsWorkTree_NormalRepo(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	isWT, err := repo.IsWorkTree()
	if err != nil {
		t.Fatalf("IsWorkTree: %v", err)
	}
	if !isWT {
		t.Fatal("expected IsWorkTree=true for normal repo")
	}
}

func TestIsWorkTree_BareRepo(t *testing.T) {
	t.Parallel()

	bareDir := filepath.Join(t.TempDir(), "bare.git")
	bareGit := initBareRepo(t, bareDir)
	r := &Repo{repo: bareGit}

	isWT, err := r.IsWorkTree()
	if err != nil {
		t.Fatalf("IsWorkTree on bare repo returned error: %v", err)
	}
	if isWT {
		t.Fatal("expected IsWorkTree=false for bare repo")
	}
}

func TestBranchExists_Local(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// The default branch is "main" (set by initRepoWithCommit).
	exists, err := repo.BranchExists("main")
	if err != nil {
		t.Fatalf("BranchExists main: %v", err)
	}
	if !exists {
		t.Fatal("expected main branch to exist locally")
	}
}

func TestBranchExists_NotFound(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	exists, err := repo.BranchExists("nonexistent-branch-xyz")
	if err != nil {
		t.Fatalf("BranchExists: %v", err)
	}
	if exists {
		t.Fatal("expected nonexistent branch to return false")
	}
}

func TestBranchExists_RemoteOnly(t *testing.T) {
	t.Parallel()

	// Clone from bare — clone has origin/main as a remote tracking ref.
	cloneDir, _ := setupCloneWithOriginHEAD(t)

	repo, err := Open(cloneDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// The clone has both local main and origin/main.
	exists, err := repo.BranchExists("main")
	if err != nil {
		t.Fatalf("BranchExists main: %v", err)
	}
	if !exists {
		t.Fatal("expected main to exist (local or remote) in clone")
	}
}

func TestDefaultBranch_OriginHEAD(t *testing.T) {
	t.Parallel()

	// Clone from a bare repo — clone has origin/HEAD -> origin/main.
	cloneDir, _ := setupCloneWithOriginHEAD(t)

	repo, err := Open(cloneDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected 'main', got %q", branch)
	}
}

func TestDefaultBranch_OriginHEAD_Master(t *testing.T) {
	t.Parallel()

	// Create a source repo with a "master" branch, push to bare, clone.
	srcDir := initRepoWithBranch(t, "master")
	srcRepo, err := git.PlainOpen(srcDir)
	if err != nil {
		t.Fatalf("PlainOpen srcDir: %v", err)
	}

	bareDir := filepath.Join(t.TempDir(), "bare.git")
	bareRepo := initBareRepo(t, bareDir)
	// Set bare repo HEAD to master so clone checks out master.
	if err := bareRepo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.HEAD, plumbing.NewBranchReferenceName("master"),
	)); err != nil {
		t.Fatalf("SetReference HEAD on bare repo: %v", err)
	}

	if _, err := srcRepo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	}); err != nil {
		t.Fatalf("CreateRemote: %v", err)
	}
	if err := srcRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("Push seed: %v", err)
	}

	cloneDir := filepath.Join(t.TempDir(), "clone")
	cloneRepo, err := Clone(context.Background(), bareDir, cloneDir)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	// Set origin/HEAD -> origin/master so DefaultBranch detects "master"
	// via the symbolic ref path (not the common-name fallback).
	if err := cloneRepo.repo.Storer.SetReference(plumbing.NewSymbolicReference(
		plumbing.NewRemoteHEADReferenceName("origin"),
		plumbing.NewRemoteReferenceName("origin", "master"),
	)); err != nil {
		t.Fatalf("SetReference origin/HEAD: %v", err)
	}

	repo, err := Open(cloneDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "master" {
		t.Fatalf("expected 'master' via origin/HEAD, got %q", branch)
	}
}

func TestDefaultBranch_LocalOnly(t *testing.T) {
	t.Parallel()

	// A local-only repo with main branch (no remote).
	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected 'main' for local-only repo, got %q", branch)
	}
}

func TestDefaultBranch_MasterBranch(t *testing.T) {
	t.Parallel()

	// Create a repo with master branch (no main, no remote).
	dir := initRepoWithBranch(t, "master")
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "master" {
		t.Fatalf("expected 'master' for repo with master branch, got %q", branch)
	}
}

func TestDefaultBranch_Fallback(t *testing.T) {
	t.Parallel()

	// Create a repo with a non-standard branch name.
	dir := initRepoWithBranch(t, "custom-branch")
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// No main, master, develop, or origin/HEAD → falls back to "main".
	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "main" {
		t.Fatalf("expected 'main' fallback, got %q", branch)
	}
}

func TestDefaultBranch_DevelopBranch(t *testing.T) {
	t.Parallel()

	// Create a repo with develop branch (no main, no master).
	dir := initRepoWithBranch(t, "develop")
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	branch, err := repo.DefaultBranch()
	if err != nil {
		t.Fatalf("DefaultBranch: %v", err)
	}
	if branch != "develop" {
		t.Fatalf("expected 'develop' for repo with develop branch, got %q", branch)
	}
}

func TestHeadBranchName(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	name, err := repo.HeadBranchName()
	if err != nil {
		t.Fatalf("HeadBranchName: %v", err)
	}
	if name != "main" {
		t.Fatalf("expected 'main', got %q", name)
	}
}

func TestHeadBranchName_DetachedHead(t *testing.T) {
	t.Parallel()

	_, dir := initRepoWithCommit(t)
	gitRepo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}

	// Get the commit hash, then detach HEAD to that hash.
	head, err := gitRepo.Head()
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	commitHash := head.Hash()

	if err := gitRepo.Storer.SetReference(plumbing.NewHashReference(
		plumbing.HEAD, commitHash,
	)); err != nil {
		t.Fatalf("SetReference detached HEAD: %v", err)
	}

	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	_, err = repo.HeadBranchName()
	if err == nil {
		t.Fatal("expected error for detached HEAD")
	}
	if !errors.Is(err, ErrDetachedHead) {
		t.Fatalf("expected ErrDetachedHead, got: %v", err)
	}
}

func TestErrDetachedHead(t *testing.T) {
	t.Parallel()
	if ErrDetachedHead.Error() != "gitops: detached HEAD" {
		t.Fatalf("unexpected ErrDetachedHead message: %q", ErrDetachedHead.Error())
	}
}
