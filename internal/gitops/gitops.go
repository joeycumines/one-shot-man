// Package gitops provides wrapper utilities for go-git/v6 git operations.
// It abstracts the go-git API to provide a simpler interface for common
// sync operations: Clone, Open, AddAll, HasStagedChanges, Commit, Push,
// plus branch inspection (DefaultBranch, BranchExists, IsWorkTree,
// HeadBranchName).
//
// Design constraints:
//   - go-git/v6 for clone, add, commit, push, branch/ref queries (no exec.Command)
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

	// ErrDetachedHead is returned when HEAD is detached (not on a branch).
	ErrDetachedHead = errors.New("gitops: detached HEAD")
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

// OpenDetect opens a git repository from the given path, walking up parent
// directories until a .git directory or file is found (like git's
// core.discoveryAcrossFs behaviour). This mirrors the semantics of
// `git rev-parse --show-toplevel` — the caller may be in a subdirectory.
//
// As a fallback, if no .git is found in any ancestor, the path itself is
// checked for a "HEAD" file — this handles bare repositories where the
// directory contains HEAD/objects/refs directly (no .git subdirectory).
//
// Returns ErrNotRepo when no repository is found in the path or any ancestor.
func OpenDetect(path string) (*Repo, error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			// The path might be a bare repo (no .git subdir, but the dir
			// itself contains HEAD/objects/refs). Check for a HEAD file as
			// a reliable indicator, then try PlainOpen without detection.
			if _, statErr := os.Stat(filepath.Join(path, "HEAD")); statErr == nil {
				if repo, err = git.PlainOpen(path); err == nil {
					return &Repo{repo: repo}, nil
				}
			}
			return nil, fmt.Errorf("%w: %s", ErrNotRepo, path)
		}
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &Repo{repo: repo}, nil
}

// IsWorkTree reports whether the repository has a working tree (i.e. is not
// bare). Bare repositories cannot be used for pr-split because they lack a
// worktree for checkout operations.
func (r *Repo) IsWorkTree() (bool, error) {
	wt, err := r.repo.Worktree()
	if err != nil {
		if errors.Is(err, git.ErrIsBareRepository) {
			return false, nil
		}
		return false, fmt.Errorf("worktree: %w", err)
	}
	_ = wt // wt is non-nil for non-bare repos
	return true, nil
}

// BranchExists reports whether a branch with the given short name exists
// locally (refs/heads/<name>) or as an origin remote tracking ref
// (refs/remotes/origin/<name>).
func (r *Repo) BranchExists(name string) (bool, error) {
	// Check local branch first.
	_, err := r.repo.Reference(plumbing.NewBranchReferenceName(name), false)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, fmt.Errorf("check local branch %q: %w", name, err)
	}

	// Fall back to origin remote tracking ref.
	_, err = r.repo.Reference(plumbing.NewRemoteReferenceName("origin", name), false)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("check remote branch %q: %w", name, err)
}

// DefaultBranch auto-detects the default branch of the repository.
//
// Detection order (first match wins):
//  1. refs/remotes/origin/HEAD symbolic ref — set automatically by `git clone`,
//     points to the remote's actual default branch (e.g. refs/remotes/origin/main).
//  2. Common branch names (main, master, develop) checked via BranchExists,
//     which looks for both local (refs/heads/<name>) and remote tracking
//     (refs/remotes/origin/<name>) refs.
//  3. Ultimate fallback: "main".
//
// This uses only go-git plumbing — zero exec.Command calls.
func (r *Repo) DefaultBranch() (string, error) {
	// 1. Try the symbolic ref refs/remotes/origin/HEAD (unresolved).
	headRef, err := r.repo.Reference(plumbing.NewRemoteHEADReferenceName("origin"), false)
	if err == nil && headRef.Type() == plumbing.SymbolicReference {
		target := headRef.Target()
		if target.IsRemote() {
			// target is like "refs/remotes/origin/main" — strip the
			// "refs/remotes/origin/" prefix to get the bare branch name.
			short := strings.TrimPrefix(target.String(), "refs/remotes/origin/")
			if short != target.String() && short != "" && short != "HEAD" {
				return short, nil
			}
		}
	}

	// 2. Check common branch names (local and remote) via BranchExists.
	commonNames := []string{"main", "master", "develop"}
	for _, name := range commonNames {
		exists, err := r.BranchExists(name)
		if err != nil {
			return "", fmt.Errorf("default branch detection: %w", err)
		}
		if exists {
			return name, nil
		}
	}

	// 3. Ultimate fallback — "main" is the modern convention.
	return "main", nil
}

// HeadBranchName returns the short name of the branch that HEAD currently
// points to (e.g. "main", "feature/foo"). Returns ErrDetachedHead when HEAD
// is not on a branch (detached HEAD state).
func (r *Repo) HeadBranchName() (string, error) {
	head, err := r.repo.Head()
	if err != nil {
		return "", fmt.Errorf("head: %w", err)
	}

	name := head.Name()
	if !name.IsBranch() {
		return "", ErrDetachedHead
	}

	return name.Short(), nil
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
