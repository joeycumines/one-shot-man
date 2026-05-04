# TUI Lifecycle and I/O Propagation Reference

Defines the strategy and implementation details for terminal state management and `io.Reader`/`io.Writer` propagation across bubbletea, bubbles, and go-prompt subsystems.

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
    - **Bubbletea:** Creates its own terminal handle. Manages its own raw-mode lifecycle.

## Lifecycle Management

- **No Shared Ownership:** Each subsystem (go-prompt, bubbletea) manages its own terminal access independently. They do NOT share a single `TerminalIO` for raw-mode operations.
- **Save/Restore Pattern:** Subsystems acquiring raw mode persist the previous state and guarantee `Restore(saved)` on all exit paths (via deferred execution or explicit cleanup).
- **State Integrity:** Restores are localized to the owning subsystem.
- **Engine Cleanup:** `Engine.Close()` does NOT close `TerminalIO`. The terminal (stdin/stdout) is process-owned and should never be closed by the engine. go-prompt manages its own reader cleanup when the prompt loop exits.

## I/O Architecture

### go-prompt
- Receives `TUIReader`/`TUIWriter` via `prompt.WithReader()` and `prompt.WithWriter()` options.
- These lazily initialize to `prompt.NewStdinReader()`/`prompt.NewStdoutWriter()`.
- Manages its own lifecycle - no external cleanup required.

### Bubbletea
- Creates its own terminal handle via `tea.NewProgram()` options.
- Uses `tea.WithInput()` and `tea.WithOutput()` for custom I/O in tests.

## Mode Switching

- **Configuration:** `shell` flags determine the active subsystem.
- **Visual TUI Execution:** Visual mode activates only when interactive. Bubbletea creates its own program, runs, and cleans up before returning.
- **Subsystem Transition:** Each subsystem fully cleans up its terminal state before the next can acquire it. go-prompt runs with its own I/O.

## Testing Support

- `newTestTUIReader`/`newTestTUIWriter`: Bypass lazy init for pre-configured `go-prompt` instances (unexported, same-package test use only).
- `NewTUIReaderFromIO`/`NewTUIWriterFromIO`: Wrap basic `io.Reader`/`io.Writer` for output capture.

## Verification

- **MakeRaw/Restore Balance:** Mock terminal validates idempotent Close and absence of double-restores.
- **Panic Recovery:** Subsystem panics trigger terminal restoration.
- **Sequential Acquisition:** Integration tests validate sequential raw-mode acquisition between subsystems.
- **Reference Tests:** `internal/scripting/tui_io_test.go`, `prompt_flow_unix_integration_test.go`.

## Source Code References

- **`internal/scripting/tui_io.go`:** Primary abstractions (`TerminalOps`, `TUIReader`, `TUIWriter`, `TerminalIO`).
- **`internal/scripting/tui_manager.go`:** go-prompt injection (`NewTUIManagerWithConfig`).
- **`internal/builtin/bubbletea/bubbletea.go`:** Bubbletea manager.
