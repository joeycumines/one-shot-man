# WIP — Session 2026-02-17 (T214+T215)

## Current State

- **T200-T213**: Done (committed in prior sessions)
- **T214+T215**: Done — combined in one step due to deadcode constraint.
  - Created `internal/gitops/gitops.go` with simplified go-git/v6 wrapper
  - Functions: Clone, Open, AddAll, HasStagedChanges, Commit, Push, IsRepo
  - No auth layer (file:// repos only, auth deferred to T235)
  - Wired into `sync.go`: executeInit (clone), executePush (add+commit+push), executePull (fallback clone), isGitRepo→gitops.IsRepo
  - Wired into `sync_startup.go`: isGitRepo→gitops.IsRepo
  - Deleted private `isGitRepo()` from sync.go (replaced by gitops.IsRepo)
  - Pull --rebase still uses exec.Command (go-git has no rebase support)
  - Updated `util_cmd_coverage_gaps_test.go`: PushErrorPaths, InitCloneFails, PullCloneFailure no longer use fake git binaries
  - Updated `sync_test.go`: removed TestIsGitRepo, replaced isGitRepo→gitops.IsRepo in assertions
  - Created `gitops_test.go`: 12 tests all pass
  - `make make-all-with-log` passes fully (build, lint, deadcode, vet, staticcheck, all tests)

## Immediate Next Step

1. Run Review Gate (Rule of Two) for T214+T215
2. Commit T214+T215
3. Proceed to T216: Replace git shell-outs in sync_startup.go

## Files Modified This Session

- `internal/gitops/gitops.go` — simplified wrapper (no auth, no Path, no Pull)
- `internal/gitops/gitops_test.go` — 12 tests
- `internal/command/sync.go` — wired in gitops for clone/add/commit/push/isGitRepo
- `internal/command/sync_startup.go` — wired in gitops.IsRepo
- `internal/command/sync_test.go` — removed TestIsGitRepo, replaced isGitRepo calls
- `internal/command/util_cmd_coverage_gaps_test.go` — rewrote PushErrorPaths/InitCloneFails/PullCloneFailure for go-git
- `blueprint.json` — T214+T215 marked Done
- `config.mk` — added build-check, test-sync targets
- `WIP.md` — this file

## Key Technical Decisions

1. **No auth**: Simplified API — no AuthProvider/WithAuth/HTTPTokenAuth. Auth deferred to T235.
2. **No Path()**: Removed from Repo struct — dead code. Callers already know the path.
3. **No Pull**: go-git v6 Pull lacks rebase. executePull still uses exec.Command.
4. **Hybrid approach**: go-git for clone/add/commit/push, exec.Command for pull --rebase.
5. **Combined T214+T215**: Deadcode checker (DEADCODE_ERROR_ON_UNIGNORED=true) requires all exports to be reachable from main. Can't have standalone gitops package without wiring.
