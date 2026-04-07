# Platform Support Strategy for `osm pr-split`

This document defines the product-level contract for platform support across all interactive pane types used by the `osm pr-split` command. It serves as the authoritative reference for tasks 2–20 in `blueprint.json` and prevents mid-project scope creep on platform questions.

## Platform × Pane Support Matrix

Each cell specifies: **support level** (interactive, capture-only, or disabled), **rationale**, and **fallback**.

| Pane Type       | macOS (darwin/arm64, darwin/amd64) | Linux (linux/amd64)              | Windows (windows/amd64)             |
|-----------------|-------------------------------------|----------------------------------|--------------------------------------|
| **Claude pane** | Interactive (PTY via creack/pty) | Interactive (PTY via creack/pty) | Capture-only initially; interactive pending ConPTY (task 14) |
| **Verify shell** | Interactive (CaptureSession + PTY) | Interactive (CaptureSession + PTY) | Capture-only initially; interactive pending ConPTY (task 14) |
| **User shell** | Interactive (CaptureSession + PTY) | Interactive (CaptureSession + PTY) | Capture-only initially; interactive pending ConPTY (task 14) |
| **Output pane** | Read-only (rendered text) | Read-only (rendered text) | Read-only (rendered text) |

### Rationale

- **macOS and Linux** are the primary development platforms. Both support POSIX PTY allocation via `creack/pty`, SIGWINCH-based resize, and Unix process groups. These platforms receive full interactive support with no caveats.
- **Windows** currently lacks PTY spawning (see `internal/termmux/pty/pty_windows.go` — returns `ErrNotSupported`). Task 13 will research ConPTY viability. Until Windows PTY is implemented (task 14), Windows falls back to capture-only mode: output is displayed but the user cannot type into panes interactively. This is an acceptable minimum for launch because the core pr-split workflow (analysis, grouping, planning, execution) is non-interactive and works identically on all platforms.

### Fallback: Windows Capture-Only Mode

When PTY spawning is unavailable (Windows without ConPTY):

1. **Verify shell** runs verification commands in a subprocess with stdout/stderr captured and rendered in the output pane. The user cannot type into the shell interactively.
2. **Claude pane** renders Claude's output as streaming text. Interactive prompting is not available; Claude receives prompts via the pipeline orchestrator only.
3. **User shell** is disabled. Users needing interactive shell access must open a separate terminal window.
4. **The TUI clearly indicates** that interactive mode is unavailable and why (`"Interactive shells require ConPTY (Windows 10 1809+). Falling back to capture-only mode."`).

## Unix-Only Code Paths

The following files and code paths are Unix-specific (gated by `//go:build !windows` or `runtime.GOOS` checks):

### PTY Layer (`internal/termmux/pty/`)

| File | Purpose | Windows Counterpart |
|------|---------|---------------------|
| `pty_unix.go` | PTY allocation via `creack/pty`, process group setup (`Setpgid`), slave fd lifecycle | `pty_windows.go` — returns `ErrNotSupported` (stub) |
| `pty_signal_unix.go` | Defines `extraSignals` map (SIGSTOP, SIGCONT) for Unix-only signal support | `pty_signal_windows.go` — empty `extraSignals` map (SIGSTOP/SIGCONT not supported on Windows) |
| `pty_unix_test.go` | Unix-specific PTY tests (resize via ioctl, signal propagation) | No Windows equivalent yet |
| `pty_splitcmd_test.go` | Shell command splitting tests (Unix shell semantics) | No Windows equivalent yet |

### I/O Layer (`internal/termmux/ptyio/`)

| File | Purpose | Windows Counterpart |
|------|---------|---------------------|
| `blocking_unix.go` | `UnixBlockingGuard` — clears `O_NONBLOCK` via `fcntl` to ensure blocking reads | `blocking_windows.go` — `WindowsBlockingGuard` (basic implementation) |
| `blocking_unix_test.go` | Unix blocking guard tests | No Windows equivalent yet |

### Mux Layer (`internal/termmux/`)

| File | Purpose | Windows Counterpart |
|------|---------|---------------------|
| `platform_unix.go` | Returns `UnixBlockingGuard` as default | `platform_windows.go` — returns `WindowsBlockingGuard` |
| `resize_unix.go` | SIGWINCH handler for terminal resize events | `resize_windows.go` — no-op (Windows does not use SIGWINCH) |

### Command Layer (`internal/command/`)

| File | Purpose | Windows Counterpart |
|------|---------|---------------------|
| `process_alive_unix.go` | `kill -0` liveness check via `syscall.Kill(pid, 0)` | `process_alive_windows.go` — conservative `return true` (relies on lock age timeout) |
| `registry_unix.go` | Process group creation (`Setpgid`) and `SIGTERM`/`SIGKILL` group kill | `registry_windows.go` — `taskkill /F /T /PID` fallback |

### Exec Module (`internal/builtin/exec/`)

| File | Purpose | Windows Counterpart |
|------|---------|---------------------|
| `spawn_unix.go` | Process group setup (`Setpgid`) and group kill (`SIGKILL` via negative PID) for JS exec module | `spawn_windows.go` — no-op `setProcAttr`, direct `Process.Kill()` |

### Storage Layer (`internal/storage/`)

| File | Purpose | Windows Counterpart |
|------|---------|---------------------|
| `filelock_unix.go` | File locking via `unix.Flock` (POSIX `flock`) for session persistence | `filelock_windows.go` — `LockFileEx`-based file locking |
| `atomic_write_unix.go` | Stub: prevents `atomicRenameWindows` from being called on Unix (returns error) | `atomic_write_windows.go` — atomic rename via `windows.MoveFileEx` with `MOVEFILE_REPLACE_EXISTING` |

### Test Files with `runtime.GOOS == "windows"` Skip Guards

These test files skip PTY-dependent tests on Windows at runtime:

- `internal/termmux/capture_test.go` — skips CaptureSession PTY tests
- `internal/termmux/pty/pty_test.go` — skips PTY spawn tests
- `internal/command/pr_split_edge_hardening_test.go` — 6 skip points
- `internal/command/pr_split_heuristic_run_test.go` — 7 skip points
- `internal/command/pr_split_corruption_test.go` — 1 skip point
- `internal/command/pr_split_mode_autofix_test.go` — 2 skip points
- `internal/command/pr_split_local_integration_test.go` — 18 skip points

### Test Files Excluded from Windows at Compile Time

These test files use build tags to exclude themselves entirely on Windows:

- `internal/command/pr_split_pty_unix_test.go` — `//go:build unix`
- `internal/command/pr_split_tui_pty_hang_test.go` — `//go:build unix`
- `internal/command/pr_split_termmux_observation_test.go` — `//go:build unix`
- `internal/command/pr_split_binary_e2e_test.go` — `//go:build unix`
- `internal/command/pr_split_16_e2e_lifecycle_test.go` — `//go:build !windows`
- `internal/command/pr_split_06b_shell_test.go` — `//go:build !windows`

## CI Platform Coverage

| Platform | CI Runner | Checks Run | Skip Conditions |
|----------|-----------|------------|-----------------|
| **macOS** | `macos-latest` | Full: `make` (build + lint + test) | None — full coverage |
| **Linux** | `ubuntu-latest` | Full: `make` (build + lint + test) | None — full coverage |
| **Windows** | `windows-latest` | Full: `make` (build + lint + test), PTY tests self-skip via build tags and `runtime.GOOS` guards | PTY-dependent tests excluded at compile time (`//go:build unix` / `//go:build !windows`) or skipped at runtime (`runtime.GOOS == "windows"` guard) |

All three platforms run identical CI steps (`make` with auto-detected parallelism). Windows test exclusions are self-contained — no CI-level skip logic is needed. Container verification (`make make-all-in-container`) is a developer-local target in `config.mk`. Integration tests (`make integration-test-*`) are defined in `project.mk` and are opt-in developer-local targets, not part of CI.

### Acceptable Skip Conditions

1. **Windows PTY tests** are skipped until ConPTY backend is implemented (task 14). This is acceptable because: (a) the PTY stub returns `ErrNotSupported` deterministically, (b) all non-PTY logic (analysis, grouping, planning, rendering, state machine) runs and is tested on Windows, and (c) the skip is guarded by an explicit `runtime.GOOS == "windows"` check, not a flaky condition.
2. **Integration tests** gated by `testing.Short()` and `runtime.GOOS` checks are opt-in via explicit make targets (`make integration-test-termmux`). PTY-dependent tests skip on Windows with a runtime check. Unit tests cover the same logic without external dependencies.

## Decision: Cross-Platform Verified

"Cross-platform verified" for this project means:

1. **Compilation succeeds** on all three platforms (`make cross-build` verifies `GOOS=linux`, `GOOS=darwin`, `GOOS=windows`).
2. **All non-PTY tests pass** on all three platforms (CI enforces this).
3. **PTY-dependent tests pass** on macOS and Linux (CI enforces this).
4. **Windows PTY tests** are explicitly skipped with documented rationale until ConPTY is implemented. Each skip site references this document.
5. **No scattered platform checks** — Unix-only behavior is isolated in `_unix.go`/`_windows.go` file pairs using build tags. Runtime `runtime.GOOS` checks in production code (not tests) are forbidden; platform differences are handled at compile time via build tags.

## Priority Code Paths for Redesign

The following code paths are the highest-value targets for the redesign (tasks 2–12), because they affect all three platforms:

1. **InteractiveSession interface** (`session.go`) — platform-agnostic contract
2. **CaptureSession lifecycle** (`capture.go`) — PTY-dependent but API is platform-agnostic
3. **TUI rendering and input routing** (`pr_split_15*.js`, `pr_split_16*.js`) — pure JavaScript, platform-agnostic
4. **Verification state machine** (`pr_split_13_tui.js`, `pr_split_16f_tui_model.js`) — pure JavaScript, platform-agnostic
5. **Pipeline orchestration** (`pr_split_10d_pipeline_orchestrator.js`) — platform-agnostic

PTY-dependent code paths (spawning, resize, signal handling) are isolated behind the `pty.Spawn` and `CaptureSession.Start` interfaces. The redesign should not introduce new platform-specific code paths outside these existing boundaries.
