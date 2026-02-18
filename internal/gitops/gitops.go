// Package gitops provides wrapper utilities for go-git/v6 git operations.
// It abstracts the go-git API to provide a simpler interface for common
// sync operations: Clone, Open, AddAll, HasStagedChanges, Commit, Push.
//
// Design constraints:
//   - go-git/v6 for clone, add, commit, push (no exec.Command)
//   - Pull with rebase is NOT supported by go-git; PullRebase uses
//     exec.Command("git", "pull", "--rebase", ...) as a consolidated
//     shell-out wrapper — the only shell-out in this package
//   - Auth is nil by default (works for local file:// repos)
package gitops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

// Sentinel errors.
var (
	// ErrNotRepo is returned when the path is not a git repository.
	ErrNotRepo = errors.New("gitops: not a git repository")

	// ErrNothingToCommit is returned when there are no staged changes.
	ErrNothingToCommit = errors.New("gitops: nothing to commit")

	// ErrConflict is returned when a pull --rebase encounters merge conflicts.
	ErrConflict = errors.New("gitops: merge conflict")
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

// PullRebaseOptions configures a PullRebase operation.
type PullRebaseOptions struct {
	// Dir is the working directory (repository root). Required.
	Dir string

	// GitBin overrides the git binary path. Default: "git".
	GitBin string

	// Stderr receives git's stderr output. May be nil.
	Stderr io.Writer
}

// PullRebase executes "git pull --rebase origin HEAD" via shell-out.
// This is the ONLY shell-out in the gitops package — go-git v6 does not
// support rebase, so this operation cannot be implemented natively.
//
// Returns ErrConflict (wrapping the underlying exec error) if stdout or stderr
// contain conflict indicators. Returns nil on success.
func PullRebase(ctx context.Context, opts PullRebaseOptions) error {
	gitBin := opts.GitBin
	if gitBin == "" {
		gitBin = "git"
	}

	var stderrBuf, stdoutBuf bytes.Buffer
	var stderrWriter io.Writer = &stderrBuf
	if opts.Stderr != nil {
		stderrWriter = io.MultiWriter(opts.Stderr, &stderrBuf)
	}

	cmd := exec.CommandContext(ctx, gitBin, "pull", "--rebase", "origin", "HEAD")
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = stderrWriter

	if err := cmd.Run(); err != nil {
		// Combine stdout + stderr for conflict detection.  git may write
		// conflict information to either stream depending on version and
		// platform (e.g. Windows git sometimes writes CONFLICT markers to
		// stdout).  Use case-insensitive matching and handle CRLF endings.
		combined := strings.ToLower(stderrBuf.String() + stdoutBuf.String())
		if strings.Contains(combined, "conflict") || strings.Contains(combined, "could not apply") {
			return fmt.Errorf("%w: %w", ErrConflict, err)
		}
		return fmt.Errorf("pull --rebase: %w", err)
	}
	return nil
}
