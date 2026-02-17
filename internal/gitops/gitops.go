// Package gitops provides wrapper utilities for go-git/v6 git operations.
// It abstracts the go-git API to provide a simpler interface for common
// sync operations: Clone, Open, AddAll, HasStagedChanges, Commit, Push.
//
// Design constraints:
//   - go-git/v6 for clone, add, commit, push (no exec.Command)
//   - Pull with rebase is NOT supported by go-git; callers must use
//     exec.Command("git", "pull", "--rebase", ...) for that operation
//   - Auth is nil by default (works for local file:// repos)
package gitops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// Sentinel errors.
var (
	// ErrNotRepo is returned when the path is not a git repository.
	ErrNotRepo = errors.New("not a git repository")

	// ErrNothingToCommit is returned when there are no staged changes.
	ErrNothingToCommit = errors.New("nothing to commit")
)

// Repo wraps a go-git Repository with simplified operations.
type Repo struct {
	repo *git.Repository
}

// IsRepo reports whether path contains a git repository (.git directory).
func IsRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

// Clone clones a remote repository to the given local path.
// The directory must not already exist or must be empty.
// Auth is nil (suitable for local file:// repos and public repos).
func Clone(ctx context.Context, url, destPath string) (*Repo, error) {
	repo, err := git.PlainCloneContext(ctx, destPath, &git.CloneOptions{
		URL: url,
	})
	if err != nil {
		return nil, fmt.Errorf("clone %s: %w", url, err)
	}
	return &Repo{repo: repo}, nil
}

// Open opens an existing git repository at the given path.
func Open(path string) (*Repo, error) {
	if !IsRepo(path) {
		return nil, fmt.Errorf("%w: %s", ErrNotRepo, path)
	}

	repo, err := git.PlainOpen(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Repo{repo: repo}, nil
}

// AddAll stages all changes in the worktree (equivalent to "git add -A").
func (r *Repo) AddAll() error {
	wt, err := r.repo.Worktree()
	if err != nil {
		return fmt.Errorf("worktree: %w", err)
	}

	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return fmt.Errorf("add all: %w", err)
	}
	return nil
}

// HasStagedChanges reports whether the index has changes staged for commit.
func (r *Repo) HasStagedChanges() (bool, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return false, fmt.Errorf("worktree: %w", err)
	}

	status, err := wt.Status()
	if err != nil {
		return false, fmt.Errorf("status: %w", err)
	}

	for _, fs := range status {
		if fs.Staging != git.Unmodified && fs.Staging != git.Untracked {
			return true, nil
		}
	}
	return false, nil
}

// Commit creates a commit with the staged changes and the given message.
// The author/committer name and email are set to "osm" / "osm@local".
// Returns the commit hash. Returns ErrNothingToCommit if no changes are staged.
func (r *Repo) Commit(msg string, when time.Time) (plumbing.Hash, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("worktree: %w", err)
	}

	sig := &object.Signature{
		Name:  "osm",
		Email: "osm@local",
		When:  when,
	}

	hash, err := wt.Commit(msg, &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err != nil {
		if errors.Is(err, git.ErrEmptyCommit) {
			return plumbing.ZeroHash, ErrNothingToCommit
		}
		return plumbing.ZeroHash, fmt.Errorf("commit: %w", err)
	}
	return hash, nil
}

// Push pushes committed changes to the remote "origin".
// Auth is nil (suitable for local file:// repos).
func (r *Repo) Push(ctx context.Context) error {
	err := r.repo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
	})
	if err != nil {
		if errors.Is(err, git.NoErrAlreadyUpToDate) {
			return nil // nothing to push
		}
		return fmt.Errorf("push: %w", err)
	}
	return nil
}
