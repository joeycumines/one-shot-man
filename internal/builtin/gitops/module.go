// Package gitops provides a Goja module wrapping the internal/gitops Go
// package for JavaScript scripts. It is registered as "osm:gitops" and allows
// JS scripts to inspect git repositories (branch detection, worktree status,
// HEAD branch name) and perform sync operations (add, commit, push) using
// go-git/v6 — no git CLI binary required.
//
// Blocking operations (push, addAll, commit, hasStagedChanges) return Promises
// and run off the event loop via Promisify. Read-only operations (isRepo, open,
// openDetect, defaultBranch, branchExists, isWorkTree, headBranchName) are
// synchronous because they are fast, bounded, local ref/filesystem lookups.
package gitops

import (
	"context"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/one-shot-man/internal/builtin/async"

	"github.com/joeycumines/one-shot-man/internal/gitops"
)

// Require is the Goja module loader for osm:gitops.
//
// The base context is captured and threaded into every async goroutine so that
// engine shutdown cancels in-flight operations. The adapter provides Promise
// resolution and event-loop scheduling.
func Require(ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// Error constants (as strings for easy comparison from JS).
		_ = exports.Set("ERR_NOT_REPO", gitops.ErrNotRepo.Error())
		_ = exports.Set("ERR_NOTHING_TO_COMMIT", gitops.ErrNothingToCommit.Error())
		_ = exports.Set("ERR_CONFLICT", gitops.ErrConflict.Error())
		_ = exports.Set("ERR_DETACHED_HEAD", gitops.ErrDetachedHead.Error())

		// isRepo(path) -> bool
		// SYNCHRONOUS: single os.Stat call, O(1), no cancellation needed.
		_ = exports.Set("isRepo", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			return runtime.ToValue(gitops.IsRepo(path))
		})

		// open(path) -> Repo
		// SYNCHRONOUS: opens repo at exact path, reads .git config. Bounded local I/O.
		_ = exports.Set("open", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			repo, err := gitops.Open(path)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			return newRepoWrapper(runtime, repo, ctx, adapter, loop)
		})

		// openDetect(path) -> Repo
		// SYNCHRONOUS: walks parent dirs to find .git. Bounded local I/O.
		_ = exports.Set("openDetect", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			repo, err := gitops.OpenDetect(path)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			return newRepoWrapper(runtime, repo, ctx, adapter, loop)
		})

		// defaultBranch(path) -> string
		// SYNCHRONOUS: openDetect + ref lookups. Bounded local I/O.
		_ = exports.Set("defaultBranch", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			repo, err := gitops.OpenDetect(path)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			branch, err := repo.DefaultBranch()
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			return runtime.ToValue(branch)
		})

		// branchExists(path, name) -> bool
		// SYNCHRONOUS: openDetect + 1-2 ref lookups. Bounded local I/O.
		_ = exports.Set("branchExists", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			name := call.Argument(1).String()
			repo, err := gitops.OpenDetect(path)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			exists, err := repo.BranchExists(name)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			return runtime.ToValue(exists)
		})

		// isWorkTree(path) -> bool
		// SYNCHRONOUS: openDetect + worktree check. Bounded local I/O.
		_ = exports.Set("isWorkTree", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			repo, err := gitops.OpenDetect(path)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			isWT, err := repo.IsWorkTree()
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			return runtime.ToValue(isWT)
		})

		// headBranchName(path) -> string
		// SYNCHRONOUS: openDetect + HEAD ref read. Bounded local I/O.
		_ = exports.Set("headBranchName", func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			repo, err := gitops.OpenDetect(path)
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			name, err := repo.HeadBranchName()
			if err != nil {
				panic(runtime.NewTypeError("%s", err.Error()))
			}
			return runtime.ToValue(name)
		})
	}
}

// newRepoWrapper creates a Goja object wrapping a *gitops.Repo with methods
// that mirror the Go API.
//
// Read-only methods (defaultBranch, branchExists, isWorkTree, headBranchName)
// are synchronous—they do fast, bounded local ref lookups.
//
// Mutating/state-scanning methods (addAll, commit, push, hasStagedChanges)
// return Promises and run off the event loop via Promisify.
func newRepoWrapper(runtime *goja.Runtime, repo *gitops.Repo, ctx context.Context, adapter *gojaeventloop.Adapter, loop *goeventloop.Loop) goja.Value {
	obj := runtime.NewObject()

	// defaultBranch() -> string
	// SYNCHRONOUS: reads refs. Bounded local I/O.
	_ = obj.Set("defaultBranch", func(call goja.FunctionCall) goja.Value {
		branch, err := repo.DefaultBranch()
		if err != nil {
			panic(runtime.NewTypeError("%s", err.Error()))
		}
		return runtime.ToValue(branch)
	})

	// branchExists(name) -> bool
	// SYNCHRONOUS: reads 1-2 refs. Bounded local I/O.
	_ = obj.Set("branchExists", func(call goja.FunctionCall) goja.Value {
		name := call.Argument(0).String()
		exists, err := repo.BranchExists(name)
		if err != nil {
			panic(runtime.NewTypeError("%s", err.Error()))
		}
		return runtime.ToValue(exists)
	})

	// isWorkTree() -> bool
	// SYNCHRONOUS: checks worktree existence. Bounded local I/O.
	_ = obj.Set("isWorkTree", func(call goja.FunctionCall) goja.Value {
		isWT, err := repo.IsWorkTree()
		if err != nil {
			panic(runtime.NewTypeError("%s", err.Error()))
		}
		return runtime.ToValue(isWT)
	})

	// headBranchName() -> string
	// SYNCHRONOUS: reads HEAD ref. Bounded local I/O.
	_ = obj.Set("headBranchName", func(call goja.FunctionCall) goja.Value {
		name, err := repo.HeadBranchName()
		if err != nil {
			panic(runtime.NewTypeError("%s", err.Error()))
		}
		return runtime.ToValue(name)
	})

	// hasStagedChanges() -> Promise<bool>
	// ASYNC: walks worktree status, can be slow for large repos.
	_ = obj.Set("hasStagedChanges", func(call goja.FunctionCall) goja.Value {
		return async.PromiseTracked(adapter, loop, ctx, func(_ context.Context) (any, error) {
			has, err := repo.HasStagedChanges()
			if err != nil {
				return nil, err
			}
			return has, nil
		}, nil)
	})

	// addAll() -> Promise<void>
	// ASYNC: stages all files, walks worktree, writes index.
	_ = obj.Set("addAll", func(call goja.FunctionCall) goja.Value {
		return async.PromiseTracked(adapter, loop, ctx, func(_ context.Context) (any, error) {
			if err := repo.AddAll(); err != nil {
				return nil, err
			}
			return nil, nil
		}, nil)
	})

	// commit(msg) -> Promise<string>
	// ASYNC: creates commit, writes git objects to disk.
	_ = obj.Set("commit", func(call goja.FunctionCall) goja.Value {
		msg := call.Argument(0).String()
		return async.PromiseTracked(adapter, loop, ctx, func(_ context.Context) (any, error) {
			hash, err := repo.Commit(msg, time.Now())
			if err != nil {
				return nil, err
			}
			return hash.String(), nil
		}, nil)
	})

	// push() -> Promise<void>
	// ASYNC: network I/O to remote. Uses context for cancellation.
	_ = obj.Set("push", func(call goja.FunctionCall) goja.Value {
		return async.PromiseTracked(adapter, loop, ctx, func(opCtx context.Context) (any, error) {
			if err := repo.Push(opCtx); err != nil {
				return nil, err
			}
			return nil, nil
		}, nil)
	})

	return obj
}
