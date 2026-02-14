# TUI Lifecycle and I/O Propagation Reference

Defines the strategy and implementation details for terminal state management and `io.Reader`/`io.Writer` propagation across tcell/tview, bubbletea, bubbles, and go-prompt subsystems.

## Design Goals

- **State Protection:** Restore terminal state on exit, error, or panic.
- **Explicit Ownership:** Define specific component responsibility for raw-mode lifecycle and `MakeRaw`/`Restore` invocation.
- **Subsystem Interoperability:** Support sequential switching between visual TUI environments and shell-style prompts.
- **Testability:** Enable injection of `io.Reader`/`io.Writer` implementations.

## Core Abstractions

- **`TerminalOps`:** Low-level interface exposing `Fd()`, `MakeRaw()`, `Restore()`, `GetSize()`, `IsTerminal()`, and standard I/O methods.
- **`TUIReader` / `TUIWriter`:** Wrappers for `io.Reader`/`io.Writer` utilized by the TUI manager to enable subsystem injection. Used by go-prompt.
- **`TerminalIO`:** Aggregates reader/writer pairs for go-prompt. **Does NOT manage terminal state** - each subsystem handles its own lifecycle independently.
- **Subsystem Independence:**
    - **go-prompt:** Uses `TUIReader`/`TUIWriter` injected via options. Manages its own reader/writer lifecycle via stdin.
    - **tview/tcell:** Opens `/dev/tty` as a separate file descriptor via `NewDevTty()`. This allows tcell to be fully closed without affecting stdin, preventing zombie input loops.
    - **Bubbletea:** Creates its own terminal handle. Manages its own raw-mode lifecycle.

## Lifecycle Management

- **No Shared Ownership:** Each subsystem (go-prompt, tview, bubbletea) manages its own terminal access independently. They do NOT share a single `TerminalIO` for raw-mode operations.
- **Save/Restore Pattern:** Subsystems acquiring raw mode persist the previous state and guarantee `Restore(saved)` on all exit paths (via deferred execution or explicit cleanup).
- **State Integrity:** Restores are localized to the owning subsystem.
- **Engine Cleanup:** `Engine.Close()` does NOT close `TerminalIO`. The terminal (stdin/stdout) is process-owned and should never be closed by the engine. go-prompt manages its own reader cleanup when the prompt loop exits.

## I/O Architecture

### go-prompt
- Receives `TUIReader`/`TUIWriter` via `prompt.WithReader()` and `prompt.WithWriter()` options.
- These lazily initialize to `prompt.NewStdinReader()`/`prompt.NewStdoutWriter()`.
- Manages its own lifecycle - no external cleanup required.

### tview/tcell

> **Deprecated.** The tview/tcell subsystem is deprecated in favor of BubbleTea.
> It will be removed in a future release. See [tview-deprecation.md](../archive/notes/tview-deprecation.md) for the full plan.

- Does NOT call `app.SetScreen()` in production - lets tview handle screen creation internally.
- tview's `Run()` method calls `tcell.NewScreen()` which:
  - On Unix: Uses `NewTerminfoScreen()` -> opens `/dev/tty` internally with proper lifecycle.
  - On Windows: Falls back to `NewConsoleScreen()`.
- This ensures tcell manages the full TTY lifecycle (open, raw mode, polling, close) without external interference.
- The `TcellAdapter` exists only for testing scenarios where custom I/O is needed.

### Bubbletea
- Creates its own terminal handle via `tea.NewProgram()` options.
- Uses `tea.WithInput()` and `tea.WithOutput()` for custom I/O in tests.

## Mode Switching

- **Configuration:** `shell` flags determine the active subsystem.
- **Visual TUI Execution:** Visual mode activates only when interactive. tview creates its own screen, runs, and cleans up before returning.
- **GUI to Shell Transition:** tview `Fini()` restores terminal state. go-prompt then runs with its own I/O.

## Testing Support

- `newTestTUIReader`/`newTestTUIWriter`: Bypass lazy init for pre-configured `go-prompt` instances (unexported, same-package test use only).
- `NewTUIReaderFromIO`/`NewTUIWriterFromIO`: Wrap basic `io.Reader`/`io.Writer` for output capture.
- `TcellAdapter`: Wraps `TerminalOps` for tcell in test scenarios.
- `safeSimScreen`: Thread-safe tcell simulation screen for tests.

## Verification

- **MakeRaw/Restore Balance:** Mock terminal validates idempotent Close and absence of double-restores.
- **Panic Recovery:** Subsystem panics trigger terminal restoration.
- **Sequential Acquisition:** Integration tests validate sequential raw-mode acquisition between visual TUI and go-prompt.
- **Reference Tests:** `internal/scripting/tui_io_test.go`, `prompt_flow_unix_integration_test.go`, `internal/builtin/tview/tview_test.go`.

## Source Code References

- **`internal/scripting/tui_io.go`:** Primary abstractions (`TerminalOps`, `TUIReader`, `TUIWriter`, `TerminalIO`).
- **`internal/scripting/tui_manager.go`:** go-prompt injection (`NewTUIManagerWithConfig`).
- **`internal/builtin/tview/tview.go`:** tview manager and `TcellAdapter`.
- **`internal/builtin/tview/tview_unix.go`:** Platform-specific screen creation.
- **`internal/builtin/bubbletea/bubbletea.go`:** Bubbletea manager.
