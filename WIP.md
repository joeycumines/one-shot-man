# WIP — Task 55+ Next

## Current Status: Tasks 40-53 DONE, proceeding to remaining tasks

### Session (2026-04-08, implementation sprint continued)

#### Completed tasks (committed)
- **Tasks 40-51**: Done in prior context windows
- **Task 52**: Log discarded errors in SessionManager — `5dada1e2`
- **Task 50**: EventBus dropped event counter (was uncommitted!) — `898b0af4`
- **Task 53**: Configurable drain timeout in CaptureSession — `6225c409`

#### Remaining tasks
- **Task 54**: ConPTY Windows support (LARGE — may defer)
- **Task 55**: E2E TUI tests (depends 40+44+48)
- **Task 56**: Purge transitional methods (depends 48+49)
- **Task 57**: Module isolation test (depends 42)
- **Task 58**: JS→Go boundary audit (depends 40+51)
- **Tasks 59-65**: Various cleanup, docs, and integration tasks

#### Pre-existing test failures (NOT ours)
- `TestCaptureSession_Passthrough_ContextCancel`: expects ExitContext, gets EOF (PTY timing)

#### Dead code note
- getClaudePaneSession() fallback wrapper in pr_split_13_tui.js (~lines 315-340) is dead code now that session() has write/resize. Should add a cleanup task to blueprint.
