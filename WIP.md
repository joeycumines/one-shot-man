# WIP — Task 64 DONE, proceeding to remaining tasks

## Current Status: Tasks 40-53, 56-58, 60, 64, 65 DONE

### Session (2026-04-08, implementation sprint continued)

#### Completed tasks (committed)
- **Tasks 40-51**: Done in prior context windows
- **Task 52**: Log discarded errors in SessionManager — `5dada1e2`
- **Task 50**: EventBus dropped event counter (was uncommitted!) — `898b0af4`
- **Task 53**: Configurable drain timeout in CaptureSession — `6225c409`
- **Task 57**: Module-extraction boundary test for termmux — `8c2718d1`
- **Task 56**: Remove Target, SetTarget, IsRunning from CaptureSession — `15e73979`
- **Task 58**: JS→Go boundary audit, reference doc, SessionManager binding tests — `5b42965f`
- **Task 60**: Documentation refresh for SessionManager architecture — `740daaeb`
- **Task 65**: Scrollback buffer design document (scratch/scrollback-design.md, local only)
- **Task 64**: Wide-character boundary repair in VTerm — `a7dbb069`

#### Remaining tasks
- **Task 54**: ConPTY Windows support (LARGE — may defer)
- **Task 55**: E2E TUI tests (depends 40+44+48)
- **Task 59**: Session suspend/resume (depends 55)
- **Task 61**: Tab-based session switching UI (depends 55)
- **Task 62**: Performance profiling and optimization
- **Task 63**: Session persistence (depends 48+61)

#### Pre-existing test failures (NOT ours)
- `TestCaptureSession_Passthrough_ContextCancel`: expects ExitContext, gets EOF (PTY timing)
