## **Implementation Plan: Session Lifecycle Management & Cleanup**

### **Objective**

To implement a robust, safe, and configurable session lifecycle management system for `osm`. This system will resolve unbounded storage growth by enforcing retention policies (Time-To-Live, Count, Size) and provide a user-facing command suite (`osm session`) for inspection and manual intervention. The solution guarantees data safety via strict file locking protocols and atomic operations.

-----

## **Phase 1: Configuration & Global Metadata Infrastructure**

### **1.1 Configuration Schema Extension**

  - Extend the application configuration struct to support cleanup policies.
  - **File**: `internal/config/config.go`
  - **Struct Addition**:
    ```go
    type SessionConfig struct {
        MaxAgeDays           int  `json:"max_age_days" default:"90"`
        MaxCount             int  `json:"max_count" default:"100"`
        MaxSizeMB            int  `json:"max_size_mb" default:"500"`
        AutoCleanupEnabled   bool `json:"auto_cleanup_enabled" default:"true"`
        CleanupIntervalHours int  `json:"cleanup_interval_hours" default:"24"`
    }
    ```

### **1.2 Global Cleanup State**

  - Introduce a lightweight global state file to track the last cleanup execution timestamp. This prevents the cleanup logic from running on every command invocation (performance optimization).
  - **File**: `internal/storage/global_metadata.go`
  - **Path**: `~/.one-shot-man/metadata.json`
  - **Struct**:
    ```go
    type GlobalMetadata struct {
        LastCleanupRun time.Time `json:"last_cleanup_run"`
    }
    ```
  - **Functions**:
      - `LoadGlobalMetadata() (*GlobalMetadata, error)`
      - `UpdateLastCleanup(t time.Time) error` (Must use atomic file writing).

-----

## **Phase 2: Storage Backend Inspection & Locking Utilities**

### **2.1 Non-Blocking Lock Inspection**

  - Extend the storage backend utilities to support "TryLock" functionality. This is critical to determine if a session is currently in use without hanging the cleanup process.
  - **File**: `internal/storage/filelock_unix.go` / `_windows.go`
  - **Function**: `AcquireLockHandle(path string) (*os.File, bool, error)`
      Locking semantics
      -----------------

      This project uses short-lived lockfiles combined with OS-level file locks
      as the primary mutual exclusion mechanism for sessions. The implementation
      consciously mimics platform semantics rather than exposing identical
      behaviour across platforms.

      POSIX (unix-like):

      - Uses `flock(2)` which provides advisory locks on the file descriptor. On
        many Unix filesystems (including most local filesystems), locks are
        respected between processes. Note that `flock` may behave differently on
        NFS or other network filesystems and cannot be relied on for strict
        semantics in those environments.

      Windows:

      - Uses `LockFileEx` to acquire an exclusive byte-range lock on a file.
      - Windows prevents deleting a file while a handle is open. The code attempts
        to acquire the lock for orphaned lockfiles before removing them; however,
        other processes holding handles may prevent deletion—this condition is
        detected and treated as a skip rather than an unconditional removal.

      Notes and best-practices
      ------------------------

      - The cleaner will only remove an orphan lock if it can acquire the lock
        and successfully release (which removes the file). This prevents races
        where a slow process would still hold the original inode even after the
        lock_filename was removed.
      - On NFS/shared filesystems `flock` behavior can be non-atomic or may be
        implemented differently; if you plan to colocate sessions across a
        network filesystem take extra care and consider using a centralized
        lock service (e.g., etcd) for strict mutual exclusion.

      - **Unix**: Use `unix.Flock` with `unix.LOCK_EX | unix.LOCK_NB`.
      - **Windows**: Use `LockFileEx` with `LOCKFILE_FAIL_IMMEDIATELY`.
      - **Returns**: A cleanup function if locked successfully, a boolean indicating success, and an error.

### **2.2 Session Analysis Service**

  - Create a service to inspect session files without fully loading them (parsing headers only where possible, or `os.Stat` for metadata).
  - **File**: `internal/storage/inspector.go`
  - **Struct**: `SessionInfo`
    ```go
    type SessionInfo struct {
        ID        string
        Path      string
        LockPath  string
        Size      int64
        UpdatedAt time.Time
        CreatedAt time.Time
        IsActive  bool // True if file is currently locked
    }
    ```
  - **Function**: `ScanSessions(dir string) ([]SessionInfo, error)`
      - Iterates `~/.one-shot-man/sessions/*.session.json`.
      - Calls `AcquireLockHandle` on the corresponding lock file.
      - If lock acquisition fails (already locked), mark `IsActive = true`.
      - If lock acquisition succeeds, release immediately and mark `IsActive = false`.

-----

## **Phase 3: Core Cleanup Logic (The Policy Engine)**

### **3.1 Cleanup Service**

  - Implement the logic to select candidates for deletion based on policies.
  - **File**: `internal/storage/cleanup.go`
  - **Struct**: `Cleaner`
  - **Logic Flow**:
    1.  **Filter**: Exclude the *current* session ID (passed via context).
    2.  **Filter**: Exclude `IsActive == true` (sessions locked by other processes).
    3.  **Policy Check - Age**: Identify sessions where `Now - UpdatedAt > MaxAgeDays`.
    4.  **Policy Check - Count**: Sort remaining sessions by `UpdatedAt` (descending). Identify entries beyond index `MaxCount`.
    5.  **Policy Check - Size**: Calculate total size. If `> MaxSizeMB`, identify oldest sessions until under threshold.
    6.  **Defunct Detection**: Identify "Orphaned Locks". Check for `.lock` files that have no corresponding `.session.json` file and remove them.

### **3.2 Safe Deletion Execution**

  - Implement the deletion routine with a "Global Cleanup Lock" to prevent multiple `osm` instances from racing to delete files.
  - **Lock Path**: `~/.one-shot-man/cleanup.lock`
  - **Function**: `ExecuteCleanup(candidates []SessionInfo) Report`
      - Acquire Global Cleanup Lock (Wait or Fail).
      - For each candidate:
          - Attempt to acquire individual session lock (Blocking with timeout, or TryLock).
          - If locked: `os.Remove(sessionPath)` and `os.Remove(lockPath)`.
          - Release lock.

-----

### **Platform Notes: Windows file-handle and archive atomicity**

 - **Windows file deletion semantics:** On Windows a file cannot be deleted while there is an open handle to it (unless opened with FILE_SHARE_DELETE). This means any design that attempts to delete a session file while still holding a lock handle on the same file may fail with access denied errors. To remain cross-platform robust prefer "sidecar" lock files (e.g. `{id}.session.lock`) rather than locking the session file itself. If the code must lock the session file directly, ensure the lock handle is closed before attempting deletion or use an OS-specific strategy.

 - **Archive TOCTOU protection:** Archiving previously used a file existence check followed by `os.Rename` which is not atomic and can be silently overwritten on some platforms if a race occurs. Prefer creating the archive destination with exclusive-create semantics (O_EXCL|O_CREATE) or using temporary files + atomic rename when the destination is guaranteed unique. Implementations should surface `EEXIST` (destination exists) to the caller so higher-level code can retry with a different candidate path.


## **Phase 4: The `sessions` Command Implementation**

### **4.1 Command Structure**

  - Register the new top-level command.
  - **File**: `internal/command/session.go`
  - **Struct**: `SessionCommand`
  - **Usage**: `osm session [list|clean|delete|info|path]`

### **4.2 Subcommands**

1.  **`list`**:
      - Calls `ScanSessions`.
      - Renders a table: `ID | Age | Size | Status (Active/Idle)`.
2.  **`clean`**:
      - Manually triggers `Cleaner.ExecuteCleanup` using loaded config.
      - Flags: `--dry-run` (shows what would be deleted).
3.  **`delete <id>`**:
      - Targets specific session.
      - Enforces locking check (fails if session is active).
4.  **`info <id>`**:
  - Loads full JSON and pretty-prints metadata/history summary.
5.  **`path [id]`**:
  - Show the sessions storage directory, or the absolute path for a session when an ID is supplied.

-----

## **Phase 5: Automation & Integration**

### **5.1 Startup Hook**

  - Integrate the cleanup check into the application startup sequence, specifically within the `NewEngine` or `StateManager` initialization.
  - **File**: `internal/scripting/tui_manager.go`
  - **Logic**:
    ```go
    func (tm *TUIManager) CheckAutoCleanup() {
        // Run in background goroutine to avoid blocking TUI startup
        go func() {
            meta := global.LoadGlobalMetadata()
            if time.Since(meta.LastCleanupRun) < config.CleanupInterval {
                return
            }

            // Acquire global cleanup lock (non-blocking)
            // If acquired:
            //    Run Cleaner
            //    Update GlobalMetadata.LastCleanupRun
            //    Release lock
        }()
    }
    ```

-----

## **Verification & Guarantees**

### **Correctness Strategy**

1.  **Locking Hierarchy**: We strictly adhere to a locking hierarchy: Global Cleanup Lock -\> Individual Session Lock. This prevents race conditions between the cleanup service and active sessions.
2.  **Atomic Deletion**: The system verifies the lock *immediately* before deletion. A session cannot become active "in between" the check and the delete because the cleanup process holds the exclusive lock during the `os.Remove` call.
3.  **Active Session Protection**: The `IsActive` check relies on OS-level file locking. If a user has a terminal open, that process holds the lock. The cleanup service will fail to acquire the lock and unconditionally skip that session.

### **Verification Plan**

1.  **Test: Concurrent Access**: Spawn a background process that holds a lock on `session-A`. Run `osm session clean`. Assert `session-A` is **not** deleted.
2.  **Test: Age Policy**: Create dummy sessions with `UpdatedAt` timestamps 365 days in the past. Run cleanup. Assert files are removed.
3.  **Test: Race Condition**: Run `osm session clean` in two separate terminals simultaneously. Assert that one waits or exits gracefully, and no panic or file corruption occurs.
4.  **Test: Orphaned Locks**: Create a `zombie.lock` with no session JSON. Run cleanup. Assert lock file is removed.

---

# Session Cleanup Mechanisms

## Current State

The storage system currently lacks any automatic cleanup mechanisms for session files. Sessions are persisted indefinitely as individual JSON files in the user's configuration directory (`{UserConfigDir}/one-shot-man/sessions/{sessionID}.session.json`). While history entries within each session are pruned to a maximum of 200 entries using a ring buffer implementation, the session files themselves accumulate without bound.

**Key Observations:**
- No TTL (time-to-live) or expiration mechanism for sessions
- No detection of "defunct" sessions (e.g., sessions tied to closed terminals or inactive users)
- No size-based cleanup when the number of session files becomes excessive
- No user-facing commands for manual session management or cleanup
- Session files may persist even after associated terminals or processes have terminated

**Potential Issues:**
- Disk space consumption over time as users create multiple sessions
- Accumulation of stale session files from temporary or one-off terminal instances
- No way to reclaim space from abandoned sessions
- Potential performance impact if the sessions directory contains thousands of files

## Identified Problem: Need for Session Cleanup Solution

The absence of session cleanup mechanisms represents a significant gap in the storage system's lifecycle management. This problem is actively asking for a comprehensive solution that addresses both automatic cleanup policies and manual management tools.

**Required Solution Components:**
1. **Automatic Cleanup Policies**: Implement configurable rules for session expiration based on:
  - Last access time (e.g., delete sessions not accessed in 30 days)
  - Session age (e.g., delete sessions older than 90 days)
  - Total session count limits (e.g., keep only the 100 most recent sessions)
  - Size-based thresholds (e.g., delete oldest sessions when total size exceeds 100MB)

2. **Defunct Session Detection**: Add logic to identify and clean up sessions that are no longer viable:
  - Terminal-specific sessions tied to non-existent terminals
  - Sessions with corrupted or invalid state
  - Sessions locked by crashed processes (stale lock files)

3. **Manual Management Tools**: Provide user commands for session administration:
  - `osm session list` - Show all existing sessions with metadata
  - `osm session delete <session-id>` - Manually remove specific sessions
  - `osm session clean` - Run automatic cleanup based on policies
  - `osm session info <session-id>` - Show detailed session information

  ### `osm session list` options

  `osm session list` supports two optional flags to control output format and ordering:

  - `-format <text|json>`
    - `text` (default): human-readable table lines, one session per line: `ID\tUpdatedAt\tSize\tstate`.
    - `json`: pretty-printed JSON array of `SessionInfo` objects (fields: `id`, `path`, `lockPath`, `size`, `updatedAt`, `createdAt`, `isActive`).
      Note: these are the actual JSON key names produced by the CLI — they are case-sensitive. For example, using `jq` you must access `.id` (not `.ID`).

  - `-sort <default|active>`
    - `default` (unchanged): preserves the scanner's ordering (filesystem discovery order).
    - `active`: sorts sessions to surface the most recently active entries first. The ordering rule is:
      1. Active sessions first (IsActive==true), idle sessions after.
      2. Within each group, sort by `UpdatedAt` descending (most recent first).
      3. Final tiebreaker: `ID` ascending.

  This makes it easy to quickly find the currently active sessions and the most recently-updated sessions when reviewing session state.

4. **Configuration Options**: Allow users to customize cleanup behavior via config file:
  - `session.max_age_days`: Maximum age for sessions
  - `session.max_count`: Maximum number of sessions to retain
  - `session.auto_cleanup_enabled`: Enable/disable automatic cleanup
  - `session.cleanup_interval_hours`: How often to run automatic cleanup

5. **Safe Cleanup Implementation**: Ensure cleanup operations are atomic and safe:
  - Proper file locking during cleanup operations
  - Backup mechanisms before deletion
  - Graceful handling of locked or in-use sessions
  - Logging of cleanup activities for audit purposes

This cleanup functionality should be implemented as a new built-in command integrated into the main command registry, with appropriate testing and documentation.

---

# Storage Backend Integration

This document provides a complete overview of how the storage backend (`./internal/storage`) is integrated into the `./cmd/osm` application, enabling persistent state management across terminal sessions and scripts.

## Executive Summary

The `osm` CLI features a sophisticated storage system designed for persistent state management across JavaScript-based TUI modes. The system successfully implements shared state contracts, enabling interoperability between commands such as `osm code-review`, `osm prompt-flow`, and `osm goal`. Users can share context (files, notes) across these commands when using the same session.

**Key Achievements:**
- Shared state contracts are fully supported via `tui.createState`
- ES6 Symbols enable cross-mode state sharing as originally designed
- Built-in commands support `--session` and `--store` flags for explicit state sharing
- Scripts utilize shared contracts for common data like `contextItems`
- Atomic writes, file locking, and session persistence ensure data integrity

**Architecture Highlights:**
- Pluggable backends (filesystem with locking, in-memory for testing)
- Session identification with configurable overrides
- State manager with shared and mode-specific state separation
- JavaScript integration through TUI API
- Comprehensive testing and error handling

## Overview

The storage system allows the one-shot-man (osm) application to persist JavaScript state, command history, and mode configurations across multiple terminal instances and script executions. This enables features like:

- **Session Persistence**: State survives terminal restarts and script re-runs
- **Multi-Terminal Sharing**: Different terminals can share state when configured
- **History Tracking**: Command history with full state snapshots
- **Shared State**: Global state accessible across all commands and modes
- **Contract Validation**: Ensures state compatibility across versions

## Architecture

### Core Components

1. **Storage Backend Interface** (`storage/backend.go`)
   - Defines the `StorageBackend` interface for persistence mechanisms
   - Supports pluggable backends (filesystem, in-memory)

2. **Backend Implementations**
   - **FileSystem Backend** (`storage/fs_backend.go`): Persistent file-based storage with atomic writes and locking
   - **In-Memory Backend** (`storage/memory_backend.go`): Ephemeral storage for testing

3. **State Manager** (`state_manager.go`)
   - Orchestrates persistence logic
   - Manages session lifecycle, shared and mode-specific state, and history
   - Provides atomic state snapshots and symbol-based access

4. **Session Schema** (`storage/schema.go`)
   - Defines the `Session` structure for serialized state
   - Includes shared state, script state, history, and metadata

### Integration Flow

#### 1. Application Entry Point (`cmd/osm/main.go`)

The main application registers commands that handle subcommands:

```go
registry.Register(command.NewScriptingCommand(cfg))
registry.Register(command.NewCodeReviewCommand(cfg))
registry.Register(command.NewPromptFlowCommand(cfg))
registry.Register(command.NewGoalCommand(cfg, registry))
```

#### 2. Command Execution

Each command accepts storage-related flags:

  - `--store`: Override storage backend (`fs` or `memory`)
  - `--session`: Override session ID for state persistence

When executed, commands create a scripting engine with explicit configuration:

```go
engine, err := scripting.NewEngineWithConfig(ctx, stdout, stderr, c.session, c.store)
```

#### 3. Engine Creation (`internal/scripting/engine_core.go`)

`NewEngineWithConfig` creates the TUI manager with session and storage parameters:

```go
engine.tuiManager = NewTUIManagerWithConfig(ctx, engine, os.Stdin, os.Stdout, sessionID, store)
```

#### 4. TUI Manager Initialization (`internal/scripting/tui_manager.go`)

`NewTUIManagerWithConfig` initializes the state manager:

```go
stateManager, err := initializeStateManager(actualSessionID, store)
```

#### 5. State Manager Setup (`internal/scripting/session_id_common.go`)

`initializeStateManager` determines the backend and creates the state manager:

1.  **Backend Selection** (precedence order):
       - `store` parameter (from `--store` flag)
       - `OSM_STORE` environment variable
       - Default: `"fs"` (filesystem backend)

2.  **Backend Creation**:
    ```go
    backend, err := storage.GetBackend(backendName, sessionID)
    ```

3.  **State Manager Initialization**:
    ```go
    stateManager, err := NewStateManager(backend, sessionID)
    ```

## Session Identification

Sessions are uniquely identified to enable state sharing and isolation. The discovery follows this precedence (from `session_id_common.go:discoverSessionID`):

1.  `overrideSessionID` parameter (from `--session` command-line flag)
2.  `OSM_SESSION` environment variable
3.  **Terminal multiplexer identifiers** (checked before generic terminal path):
    - **tmux**: `TMUX_PANE` + hash of `TMUX` socket path to ensure uniqueness across multiple tmux servers (format: `tmux-{socketHash}-{paneID}`)
    - **screen**: `STY` environment variable (format: `screen-{STY}`)
4.  Controlling terminal device path (via `getTerminalID()` - POSIX systems)
5.  **Platform-specific stable variables**:
    - macOS: `TERM_SESSION_ID` (direct value)
    - X11: `WINDOWID` (format: `x11-{WINDOWID}`)
6.  Generated UUID (fallback if no other stable ID exists)

This allows:

  - **Terminal-specific sessions**: Each terminal window has its own state
  - **Multiplexer-aware sessions**: Tmux panes and screen windows get stable, unique IDs across server restarts
  - **Shared sessions**: Multiple terminals can share state via environment variables or explicit `--session` flag
  - **Testing isolation**: Tests can specify custom session IDs

## Storage Backends

### FileSystem Backend

  - **Purpose**: Production persistent storage
  - **Location**: Session files are stored under the user's config directory. The code uses `os.UserConfigDir()` and places files in `{UserConfigDir}/one-shot-man/sessions/` with filenames of the form `{sessionID}.session.json` (for example, on macOS the base config dir is typically `~/Library/Application Support`).
  - **Features**:
      - Atomic writes using temporary files and rename operations
      - Exclusive file locking to prevent concurrent access (Unix: flock, Windows: LockFileEx)
      - JSON serialization with schema versioning
  - **Platform Support**: Cross-platform with OS-specific locking implementations

### In-Memory Backend

  - **Purpose**: Testing and ephemeral sessions
  - **Storage**: Global in-memory map (shared across instances for testing)
  - **Features**:
      - No persistence (state lost on process exit)
      - Thread-safe with mutex protection
      - Deep copying to prevent concurrent modification issues

## State Persistence

### What Gets Persisted

1.  **Shared State**: Global state accessible across all commands and modes
2.  **Script State**: Per-command state (e.g., mode-specific configurations)
3.  **History**: Command execution log with full state snapshots
4.  **Session Metadata**: Version, timestamps, session ID

### Persistence Triggers

  - **In-memory capture after commands**: When a mode has `TUIConfig.EnableHistory == true`, `tui_manager.go:executeCommand` calls `captureHistorySnapshot` after successful command execution. This serializes the complete state and calls `StateManager.CaptureSnapshot()`, which adds an entry to the history ring buffer (in-memory only at this point).
  - **Manual disk persistence**: Via `StateManager.PersistSession()`, which atomically writes the session to the configured backend
  - **Automatic backend fallback**: If the requested backend (e.g., filesystem) fails to initialize, the TUI manager falls back to the memory backend and prints a warning
  - **Shutdown / Exit**: The TUI manager's `Close()` method calls `StateManager.Close()`, which persists the session one final time before closing the backend

### State Structure

```json
{
  "version": "1.0.0",
  "session_id": "/dev/ttys001",
  "created_at": "2025-10-25T10:00:00Z",
  "updated_at": "2025-10-25T10:30:00Z",
  "shared_state": {
    "contextItems": [...]
  },
  "script_state": {
    "code-review": {...},
    "prompt-flow": {...}
  },
  "history": [...]
}
```

## JavaScript Integration

The storage system is exposed to JavaScript scripts through the TUI API via `engine_core.go:jsCreateState`.

### tui.createState API

**Signature:** `tui.createState(commandName, definitions)` → `{get(symbol), set(symbol, value)}`

**Parameters:**
- `commandName`: String identifier for the command (e.g., "code-review")
- `definitions`: Object with Symbol-keyed properties, each value optionally has `{defaultValue: ...}`

**Implementation Details:**

1. **Symbol Extraction**: Uses JavaScript `Object.getOwnPropertySymbols()` to find all Symbol-keyed properties. String-keyed properties are ignored.

2. **Symbol Classification**: For each Symbol, checks if it's a shared symbol via `StateManager.IsSharedSymbol()`:
   - **Shared symbols** (from `osm:sharedStateSymbols`): Stored directly by canonical name (e.g., `"contextItems"`)
   - **Command-specific symbols**: Namespaced as `commandName:normalizedDescription`

3. **Symbol Normalization** (from `js_state_accessor.go:normalizeSymbolDescription`):
   - Strips `Symbol(...)` wrapper if present
   - Unquotes the description string (handles escape sequences)
   - Returns empty string for missing/invalid descriptions (causes `createState` to panic)

4. **Default Value Initialization**: For each Symbol with `defaultValue`:
   - Calls `StateManager.GetState(persistentKey)` to check if key exists
   - If key doesn't exist, calls `StateManager.SetState(persistentKey, defaultValue)`
   - If key exists, skips initialization (preserves previously persisted values)

5. **Closure Capture**: The returned accessor object captures:
   - `persistentKeyMap`: Maps runtime Symbol string representation → persistent string key
   - `defaultValues`: Maps Symbol string → default value
   - Both maps are closed over and used by `get()` and `set()` methods

**Return Value:** Object with methods:
- `get(symbol)`: Returns state value or default value if unset
- `set(symbol, value)`: Updates state via StateManager

**State Access** (no automatic fallback in new architecture):
- `get(symbol)` with a shared symbol directly accesses SharedState
- `set(symbol, value)` with a shared symbol directly updates SharedState
- Command-specific symbols access ScriptState[commandName][localKey]
- No fallback between zones; each symbol accesses its designated zone only

Example JavaScript usage:

```javascript
// Import shared symbols
const shared = require('osm:sharedStateSymbols');

// Create state accessor with shared and command-specific keys
const state = tui.createState("code-review", {
    [shared.contextItems]: { defaultValue: [] },
    localCounter: Symbol("code-review:localCounter")
});

// Access state - each symbol accesses its designated zone
const items = state.get(shared.contextItems);  // Accesses SharedState["contextItems"]
const counter = state.get(localCounter);       // Accesses ScriptState["code-review"]["code-review:localCounter"]

// Update state
state.set(shared.contextItems, [...items, newItem]);
state.set(localCounter, counter + 1);
```

## Relevant Files

The following files are relevant to the storage backend integration:

```
internal/storage/backend.go
internal/storage/fs_backend.go
internal/storage/memory_backend.go
internal/storage/schema.go
internal/storage/registry.go
internal/storage/paths.go
internal/storage/atomic_write.go
internal/storage/atomic_write_unix.go
internal/storage/atomic_write_windows.go
internal/storage/filelock_unix.go
internal/storage/filelock_windows.go
internal/scripting/state_manager.go
internal/scripting/tui_manager.go
internal/scripting/session_id_common.go
internal/scripting/js_state_accessor.go
internal/scripting/engine_core.go
internal/command/scripting_command.go
internal/command/code_review.go
internal/command/prompt_flow.go
internal/command/goal.go
internal/command/code_review_script.js
internal/command/goal.js
internal/command/prompt_flow_script.js
internal/builtin/ctxutil/contextManager.js
internal/builtin/shared_symbols.go
```

## Detailed File Analysis


  - **internal/storage/backend.go**: Defines the `StorageBackend` interface:
    ```go
    type StorageBackend interface {
        // LoadSession retrieves a session by its unique ID.
        // It MUST return (nil, nil) if the session does not exist.
        LoadSession(sessionID string) (*Session, error)

        // SaveSession atomically persists the entire session state.
        SaveSession(session *Session) error

        // Close performs any necessary cleanup of backend resources, such as releasing file locks.
        Close() error
    }
    ```

  - **internal/storage/fs_backend.go**: Implements the filesystem backend:
    - Uses `NewFileSystemBackend(sessionID string)` to create the backend and acquire an exclusive lock.
    - Locking is performed via `acquireFileLock(lockPath)` (see platform-specific filelock files).
    - Loads session with `LoadSession(sessionID string)`, checks for session file existence, and deserializes JSON.
    - Version validation is deferred to `StateManager`.

  - **internal/storage/memory_backend.go**: In-memory backend for testing with global state sharing and mutex protection.

  - **internal/storage/schema.go**: Defines the session schema:
    ```go
    type Session struct {
        Version     string                            `json:"version"`
        ID          string                            `json:"id"`
        CreatedAt   time.Time                         `json:"created_at"`
        UpdatedAt   time.Time                         `json:"updated_at"`
        History     []HistoryEntry                    `json:"history"`
        ScriptState map[string]map[string]interface{} `json:"script_state"`
        SharedState map[string]interface{}            `json:"shared_state"`
    }

    type HistoryEntry struct {
        EntryID    string    `json:"entry_id"`
        ModeID     string    `json:"mode_id"`
        Command    string    `json:"command"`
        Timestamp  time.Time `json:"timestamp"`
        FinalState string    `json:"finalState"`
    }
    ```

  - **internal/storage/registry.go**: Registry for backend factories, registers "fs" and "memory" backends.

  - **internal/storage/paths.go**: Path utilities for session directories, files, and lock files with cross-platform support.

  - **internal/storage/atomic_write.go**: Implements atomic file writing:
    - `AtomicWriteFile(filename string, data []byte, perm os.FileMode) error`:
      - Creates a temp file in the target directory.
      - Writes data, syncs to disk.
      - Atomically renames temp file to target file.
      - Cleans up temp file on error.

  - **internal/storage/atomic_write_unix.go**: Unix-specific atomic rename (uses os.Rename).

  - **internal/storage/atomic_write_windows.go**: Windows-specific atomic rename using MoveFileEx.

  - **internal/storage/filelock_unix.go**: Unix file locking (platform implementation):
    - Implementation: uses `unix.Flock` to acquire an exclusive, non-blocking advisory lock on the lock file descriptor.
    - Behaviour: lock file is created/opened, locked, and later unlocked + removed on release.

  - **internal/storage/filelock_windows.go**: Windows file locking (platform implementation):
    - Implementation: uses Windows `LockFileEx` to acquire an exclusive byte-range lock via `golang.org/x/sys/windows`.
    - Behaviour: lock file is created/opened, locked, and later unlocked + removed on release.

  - **internal/scripting/state_manager.go**: Implements `StateManager`:
    - Loads or initializes session, handles schema migration.
    - Maintains symbol maps for shared state identification.
    - Uses a fixed-size history ring buffer (`maxHistoryEntries = 200`).
    - Ensures `ScriptState` and `SharedState` are always initialized.

### StateManager Architecture

The StateManager maintains:
- **Session data** (ScriptState, SharedState, History, metadata)
- **Symbol maps** (`sharedSymbolToString`, `sharedStringToSymbol`) for identifying shared symbols at runtime
- **History ring buffer** (fixed-size physical buffer with maxHistoryEntries = 200 entries) for efficient O(1) append and space-bounded memory
- **Thread safety** via sync.Mutex for session modifications and sync.RWMutex for symbol map reads

**State Organization:** State is partitioned into two zones:
- **Shared State** (`SharedState` map): Keys without colons (e.g., `"contextItems"`) are persisted directly
- **Script State** (`ScriptState` map): Keys with colons (e.g., `"code-review:localKey"`) are namespaced by command and stored in `ScriptState[commandName][localKey]`

**Core Methods:**
- `GetState(persistentKey)`: Retrieves state, checking SharedState for keys without colons, ScriptState for namespaced keys
- `SetState(persistentKey, value)`: Stores state in the appropriate zone based on key format
- `CaptureSnapshot(modeID, command, stateJSON)`: Records an immutable history entry with complete serialized state
- `PersistSession()`: Writes session to backend via atomic operations
- `GetSessionHistory()`: Returns a chronological copy of the command history
- `SerializeCompleteState()`: JSON-encodes both ScriptState and SharedState for history entries
- `SetSharedSymbols(symbolToString, stringToSymbol)`: Called by `osm:sharedStateSymbols` module to register shared symbol identities
- `IsSharedSymbol(symbol)`: Checks if a Symbol is a known shared symbol; returns its canonical string key if true
- `Close()`: Persists session and closes backend before cleanup

**History Ring Buffer:** History uses fixed-size O(1) append:
- Physical buffer: `historyBuf` (size = 200 entries)
- Logical pointers: `historyStart` (oldest entry index) and `historyLen` (count of valid entries)
- When buffer fills, oldest entry is overwritten; pointers advance to maintain chronology
- `getFlatHistoryInternal()` reconstructs chronological order handling wrap-around
- Serialization of ring buffer to flat slice occurs only when persisting to avoid allocation on every read

  - **internal/scripting/tui_manager.go**: TUI manager initialization with state manager, rehydration of shared context, and command execution.

  - **internal/scripting/session_id_common.go**: Session ID discovery logic with flag/env/terminal precedence.

  - **internal/scripting/js_state_accessor.go**: Symbol mapping and JS API:
    - `normalizeSymbolDescription(symbolDesc string)` strips `Symbol(...)` wrappers and unquotes descriptions for persistent key mapping.
    - `jsCreateState` uses JavaScript to extract Symbol-keyed properties from definitions, mapping them to persistent keys and default values.

  - **internal/scripting/engine_core.go**: Engine creation with TUI manager and session configuration.

  - **internal/command/scripting_command.go**: Scripting command with session and storage backend flags.

  - **internal/command/code_review.go**: Code review command with session and storage backend support.

  - **internal/command/prompt_flow.go**: Prompt flow command with session and storage backend support.

  - **internal/command/goal.go**: Goal command with session and storage backend support.

  - **internal/command/code_review_script.js**: Uses shared `contextItems` via `shared.contextItems` symbol.

  - **internal/command/goal.js**: Uses shared `contextItems` for all goal modes.

  - **internal/command/prompt_flow_script.js**: Uses shared `contextItems` and local state keys.

  - **internal/builtin/ctxutil/contextManager.js**: Context management factory that accepts custom getItems/setItems for shared or mode-specific state.

  - **internal/builtin/shared_symbols.go**: Provides the `osm:sharedStateSymbols` module for JavaScript scripts to access shared state symbols.
    ```go
    // sharedStateKeys defines the canonical string keys for all shared state.
    var sharedStateKeys = []string{
        "contextItems",
        // Add other future shared keys here.
    }

    // GetSharedSymbolsLoader returns a loader function compatible with require.RegisterNativeModule.
    func GetSharedSymbolsLoader(stateManagerProvider StateManagerProvider) func(*goja.Runtime, *goja.Object) {
        // Creates symbols like Symbol("osm:shared/contextItems") and exports them as {contextItems: symbol}
        // Registers mappings with StateManager for shared symbol identification
    }
    ```

## Additional Implementation Details

### `fs_backend.go`
- Implements the `FileSystemBackend` for persistent storage.
- Key methods:
  - `NewFileSystemBackend(sessionID string)`: Creates a new backend and acquires an exclusive lock.
  - `LoadSession(sessionID string)`: Reads and deserializes session data from the filesystem.
  - `SaveSession(session *Session)`: Serializes and writes session data atomically.
- Uses `SessionDirectory`, `SessionFilePath`, and `SessionLockFilePath` for path resolution.
- Ensures atomic writes using `AtomicWriteFile`.

### `registry.go`
- Maintains a registry of storage backends.
- Registers `fs` (filesystem) and `memory` (in-memory) backends.
- Provides `GetBackend(name, sessionID string)` to retrieve a backend instance.

### `memory_backend.go`
- Implements the `InMemoryBackend` for ephemeral storage.
- Key methods:
  - `NewInMemoryBackend(sessionID string)`: Creates a new backend with a global in-memory store.
  - `LoadSession(sessionID string)`: Retrieves a deep copy of the session from memory.
  - `SaveSession(session *Session)`: Stores a deep copy of the session in memory.
- Includes `ClearAllInMemorySessions()` for testing.

### `schema.go`
- Defines the `Session` structure for serialized state:
  - `Version`: Schema version for compatibility.
  - `SessionID`: Unique identifier for the session.
  - `CreatedAt`, `UpdatedAt`: Timestamps for session lifecycle.
  - `History`: Chronological log of commands.
  - `ScriptState`: Per-command state.
  - `SharedState`: Global shared state.
- Includes `HistoryEntry` for immutable command logs.

### `atomic_write.go`
- Provides `AtomicWriteFile` for safe file writes:
  - Writes data to a temporary file.
  - Ensures data is synced to disk.
  - Atomically renames the temporary file to the target file.
- Includes `RenameError` for detailed error reporting.
- Supports crash simulation via `testHookCrashBeforeRename` (for testing).

### Testing Notes
- `fs_backend.go` and `memory_backend.go` include compile-time checks to ensure they implement `StorageBackend`.
- `atomic_write.go` ensures cross-platform atomic writes with OS-specific implementations.

## Shared State Architecture

The shared state architecture successfully enables cross-command state sharing through:

1. **Shared State Contracts**: Scripts use `tui.createState(commandName, definitions)` with shared symbols from `osm:sharedStateSymbols` to access shared `contextItems` via `shared.contextItems`.

2. **Session-Based Persistence**: Commands with `--session` flags persist state to configurable backends (filesystem or in-memory).

3. **State Manager Integration**: `state_manager.go` treats keys without a colon (:) as shared and stores them in the session's `SharedState` (persisted under the JSON key `shared_state`). Script-specific keys are namespaced as `commandName:localKey` and are stored in `ScriptState`.

4. **Context Rehydration**: On mode switches, `tui_manager.go:SwitchMode` calls `rehydrateContextManager()` which retrieves persisted `contextItems` from SharedState, re-adds file-type items to ContextManager (removing stale references), and ensures the session remains consistent across mode switches.

5. **Cross-Platform Atomic Writes**: Ensures data integrity with temp files, renames, and OS-specific file locking.

6. **Session Discovery**: Automatic session ID resolution via flags, environment, multiplexer identifiers, terminal path, or UUID generation.

## Implementation Details Reference

This section provides implementation facts from inspecting the actual codebase to enable accurate understanding of specific mechanisms.

### `js_state_accessor.go`
- **normalizeSymbolDescription(symbolDesc string)**: Converts goja.Symbol string representation to persistent key format
  - Input: `"Symbol(\"osm:shared/contextItems\")"` or `"Symbol(\"code-review:localKey\")"` or empty
  - Strips `Symbol(` prefix and `)` suffix if present
  - Unquotes the description string (handles escape sequences)
  - Returns empty string for missing/invalid descriptions
- **jsCreateState** orchestrates the full state accessor creation process:
  - Extracts Symbol-keyed properties via JavaScript `Object.getOwnPropertySymbols()`
  - Classifies each symbol as shared or command-specific
  - Normalizes symbol descriptions to persistent keys
  - Initializes default values only if key doesn't already exist
  - Captures `persistentKeyMap` and `defaultValues` in closure for accessor methods

### `session_id_common.go`
- **discoverSessionID(overrideSessionID)**: Implements the session ID precedence logic
  - Parameter from flag takes priority
  - `OSM_SESSION` environment variable as second priority
  - Tmux detection: uses `TMUX_PANE` + SHA256 hash of TMUX socket path (first 8 chars) for server uniqueness
  - Screen detection: uses `STY` environment variable
  - Falls back to terminal device path via `getTerminalID()`
  - Then tries `TERM_SESSION_ID` (macOS) or `WINDOWID` (X11)
  - Generates UUID as final fallback
- **initializeStateManager(sessionID, overrideBackend)**: Creates StateManager with backend fallback
  - Uses precedence: parameter > `OSM_STORE` env var > "fs" default
  - If backend creation fails, automatically falls back to memory backend with warning message
  - Returns initialized StateManager with either requested or fallback backend

### `registry.go`
- Maintains a registry of storage backend factories
- Registers `fs` (filesystem) and `memory` (in-memory) backends via `RegisterBackend(name, factory)`
- Provides `GetBackend(name, sessionID string)` to retrieve a backend instance
- Each backend factory takes a sessionID and returns a StorageBackend interface

### `tui_manager.go`
- **NewTUIManagerWithConfig**: Initializes TUI manager with explicit session and backend configuration
  - Calls `discoverSessionID()` to resolve actual session ID
  - Calls `initializeStateManager()` with fallback to memory backend on initialization failure
  - Initializes StateManager and extracts command history via `GetSessionHistory()`
  - Registers built-in commands
- **SwitchMode**: Handles mode switching including state rehydration
  - Calls current mode's OnExit callback before switching
  - Calls `rehydrateContextManager()` to restore file items from persisted contextItems
  - Builds mode commands on first entry if CommandsBuilder is defined
  - Calls mode's OnEnter callback
- **executeCommand**: Executes JavaScript command handler with execution context
  - Sets up ExecutionContext for the command
  - After successful execution, calls `captureHistorySnapshot()` if `TUIConfig.EnableHistory == true`
  - Runs deferred functions collected during execution
- **captureHistorySnapshot**: Records history entry with complete state
  - Serializes complete state via `StateManager.SerializeCompleteState()`
  - Calls `StateManager.CaptureSnapshot(modeID, commandString, stateJSON)`
  - Logs warning if snapshot capture fails (doesn't halt execution)
- **rehydrateContextManager**: Restores file items from persisted contextItems
  - Retrieves contextItems from SharedState
  - For each file-type item, calls `ContextManager.AddPath(label)`
  - Removes items with missing/inaccessible files from state to maintain consistency
  - Preserves non-file items (notes, diffs) regardless
- **Close**: Graceful shutdown
  - Calls `StateManager.Close()` which persists session and closes backend

## Alternative Solutions Considered

### Option 1: Global State Registry

**Description**: Instead of per-mode contracts, maintain a single global state object accessible to all modes.

**Pros**:

  - Simplifies state access (`state.get('contextItems')` works everywhere).
  - No contract registration needed.
  - Easier for scripts to share data.

**Cons**:

  - Violates mode isolation principle.
  - Potential for accidental overwrites between modes.
  - No schema validation per mode.
  - Harder to track which mode owns which data.

**Recommendation**: Rejected. While simpler, it undermines the mode-based architecture and increases coupling.

### Option 2: Automatic Shared Detection

**Description**: Modify `createState` to automatically detect common keys (like `contextItems`) and promote them to shared state.

**Pros**:

  - Backward compatible with existing scripts.
  - No script changes required.
  - Automatic interoperability for common patterns.

**Cons**:

  - Magic behavior that's hard to understand.
  - Unclear ownership of shared keys.
  - Potential conflicts if modes define different schemas for same key.
  - Violates explicit design principle.

**Recommendation**: Rejected. Introduces implicit behavior that could lead to bugs and confusion.

### Option 3: Context Manager Refactoring

**Description**: Modify `contextManager` to always use shared state for `contextItems`, regardless of provided getters/setters.

**Pros**:

  - Single change affects all commands.
  - Scripts continue to work unchanged.
  - Enforces shared context by design.

**Cons**:

  - Breaks encapsulation (contextManager assumes control of state access).
  - Modes lose ability to have private context.
  - Unexpected side effects for custom context managers.

**Recommendation**: Rejected. Violates the composable design of contextManager and could break existing or future custom implementations.

### Option 4: Command-Level State Sharing (Chosen)

**Description**: Explicitly use shared contracts in scripts and add session flags to commands.

**Pros**:

  - Explicit and intentional.
  - Maintains mode isolation for other state.
  - Follows existing architectural patterns.
  - Allows fine-grained control over what is shared.

**Cons**:

  - Requires changes to multiple files.
  - Scripts must be updated to use shared contracts.

**Recommendation**: Accepted. Aligns with the original Symbol-sharing design and provides the most robust solution.

## Testing Strategy for Interoperability Implementation

### Unit Tests

1.  **Shared State Contract Creation**: Verify `createState` creates symbols with correct descriptions and default values.
2.  **State Access Fallback**: Test `getStateBySymbol` prioritizes mode-specific state, falls back to shared.
3.  **Context Manager with Shared State**: Ensure `contextManager` works with shared getters/setters.
4.  **Session ID Override**: Test that commands accept and use `--session` flag correctly.

### Integration Tests

1.  **Cross-Command State Sharing**:
       - Run `osm code-review --session test-session`, add context.
       - Run `osm goal commit-message --session test-session`, verify context is available.
2.  **Session Persistence**: Ensure shared state persists across command invocations.
3.  **Mode Isolation**: Verify mode-specific state remains separate from shared state.
4.  **History Separation**: Confirm per-mode histories are maintained.

### End-to-End Tests

1.  **Workflow Simulation**:
       - Add files in `osm code-review`.
       - Switch to `osm prompt-flow`, verify files are available.
       - Generate prompt, copy to clipboard.
       - Switch to `osm goal commit-message`, use the prompt.
2.  **Multi-Terminal Sharing**: Test state sharing across different terminal windows using explicit session IDs.
3.  **Error Handling**: Verify graceful fallback when shared state is unavailable.

### Regression Tests

1.  **Existing Functionality**: Ensure all current features work unchanged.
2.  **Performance**: Verify no performance degradation in state access or persistence.
3.  **Concurrency**: Test concurrent access to shared state across commands.

## Migration Guide

### For Script Developers

When updating embedded JavaScript scripts to use shared state:

1.  **Identify Shared Keys**: Determine which state keys should be shared (e.g., `contextItems`, `currentGoal`).
2.  **Import Shared Symbols**: Use `const shared = require('osm:sharedStateSymbols')` to access shared symbols.
3.  **Create State Accessor**: Replace per-mode contract creation:
    ```javascript
    // Before
    const StateKeys = tui.createState(MODE_NAME, {
        contextItems: Symbol(MODE_NAME + ":contextItems")
    });

    // After
    const shared = require('osm:sharedStateSymbols');
    const state = tui.createState(MODE_NAME, {
        [shared.contextItems]: { defaultValue: [] },
        // mode-specific keys only
    });
    ```
4. **Update State Access**: Change all `StateKeys.contextItems` to `shared.contextItems`.
5. **Test Thoroughly**: Ensure mode-specific state still works and shared state is accessible.

### For Command Developers

When adding session support to built-in commands:

1.  **Add Fields**: Add `session` and `store` string fields to command struct.
2.  **Add Flags**: In `SetupFlags`, add:
    ```go
    fs.StringVar(&c.session, "session", "", "Session ID for state persistence")
    fs.StringVar(&c.store, "store", "", "Storage backend")
    ```
3.  **Update Engine Creation**: Replace `scripting.NewEngine(ctx, stdout, stderr)` with `scripting.NewEngineWithConfig(ctx, stdout, stderr, c.session, c.store)`.

### For Users

After implementation:

1.  **Explicit Sessions**: Use `--session shared-session` to share state across commands.
2.  **Cross-Command Workflows**: Build context in one command, access in another.
3.  **Multi-Terminal**: Share sessions across terminal windows for collaborative workflows.

### Rollback Plan

If issues arise:

1.  **Script Rollback**: Revert to per-mode contracts (functionality preserved, interoperability lost).
2.  **Command Rollback**: Remove session flags (commands work as before).
3.  **Data Migration**: No migration needed - existing sessions remain compatible.

## Future Considerations

### Advanced Interoperability Features

1.  **Selective State Sharing**: Allow modes to opt-in to sharing specific keys while keeping others private.
2.  **State Synchronization**: Real-time sync of shared state across multiple terminals.
3.  **Conflict Resolution**: Handle concurrent modifications to shared state with merge strategies.
4.  **State Versioning**: Track changes to shared state with conflict detection.

### Architectural Improvements

1.  **Plugin System**: Allow third-party modes to register shared state contracts.
2.  **State Observers**: Notify modes when shared state changes.
3.  **State Validation**: Cross-mode validation of shared state schemas.
4.  **Performance Optimization**: Lazy loading and caching for large shared state objects.

### User Experience Enhancements

1.  **Session Management UI**: TUI interface for managing and switching sessions.
2.  **State Diffing**: Show differences between current and persisted state.
3.  **Import/Export**: Ability to export shared state for backup or sharing.
4.  **Collaboration Features**: User identification and change attribution in shared sessions.

### Security Considerations

1.  **State Isolation**: Ensure private mode state cannot be accessed via shared contracts.
2.  **Session Security**: Prevent unauthorized access to shared sessions.
3.  **Data Sanitization**: Clean sensitive data before persistence.
4.  **Audit Logging**: Track state changes for compliance.

## Glossary

  - **State Contract**: A schema defining state keys, default values, and validation rules for a mode.
  - **Shared State Contract**: A contract whose keys are accessible across all modes. Shared keys are persisted inside the session's `SharedState` (JSON key `shared_state`). Symbol-based shared keys are exported by `osm:sharedStateSymbols`.
  - **Session ID**: Unique identifier for a state persistence context, typically tied to a terminal.
  - **Mode**: A self-contained UI and command set within the TUI (e.g., code-review, prompt-flow).
  - **Symbol Registry**: Global mapping of symbol descriptions to ES6 Symbol objects for serialization.
  - **Context Items**: Array of user-added data (files, notes, diffs) managed by contextManager.
  - **Backend**: Storage implementation (filesystem or memory) handling persistence.
  - **Interoperability**: Ability to share state across different OSM commands and modes.

## FAQ

### Why were Symbols chosen for state management?

Symbols provide unique, immutable identifiers that prevent key collisions while allowing descriptive metadata for persistence. They enable the core requirement of cross-mode state sharing.

### Do built-in commands support `--session` and `--store` flags?

Yes — built-in commands expose `--session` and `--store` flags and construct the engine with `NewEngineWithConfig(ctx, stdout, stderr, session, store)`. This lets users explicitly control session IDs and the storage backend (for persistent or in-memory sessions).

### Can shared state be corrupted by concurrent access?

The filesystem backend uses file locking to prevent concurrent writes. The in-memory backend uses mutexes. State validation ensures schema compliance.

### What happens to existing sessions after implementing shared state?

Existing sessions remain compatible. Mode-specific state continues to work. Shared state is additive - modes can access it if available.

### How does state fallback work?

When `state.get(symbol)` is called, the system first checks mode-specific state, then falls back to shared state if the key exists there.

### Are there performance implications for shared state?

Minimal. Shared state is stored in the same JSON structure, and access patterns remain similar. The main cost is the fallback check.

### Can modes have private contextItems?

Yes. Modes can define additional private state keys. Only keys explicitly put in shared contracts are accessible across modes.

### What if two modes define the same shared key differently?

Contract validation will detect schema mismatches and reset to defaults. Shared contracts should be designed collaboratively.

## References

### Code Locations

  - **Storage Backend Interface**: `internal/storage/backend.go`
  - **Shared State API**: `internal/scripting/js_state_accessor.go:jsCreateState`
  - **Shared Symbols Module**: `internal/builtin/shared_symbols.go`
  - **Session Discovery**: `internal/scripting/session_id_common.go:initializeStateManager`
  - **Context Manager**: `internal/builtin/ctxutil/contextManager.js`
  - **Built-in Commands**: `internal/command/code_review.go`, `internal/command/prompt_flow.go`, `internal/command/goal.go`
  - **Embedded Scripts**: `internal/command/code_review_script.js`, `internal/command/goal.js`, `internal/command/prompt_flow_script.js`

### Test Files

  - **Shared State Tests**: `internal/scripting/tui_manager_test.go`
  - **Integration Tests**: `internal/scripting/integration_persistence_test.go`
  - **Context Tests**: `internal/scripting/context_rehydration_integration_test.go`

### Related Documentation

  - **TUI State Management**: `docs/tui-state.md`
  - **JavaScript API**: `docs/goja-reference.md`
  - **Configuration**: `docs/config-reference.md`

## Code Examples

### Creating Shared State Contract

```javascript
// Import shared symbols
const shared = require('osm:sharedStateSymbols');

// Create state accessor with shared and mode-specific keys
const state = tui.createState("my-mode", {
    [shared.contextItems]: { defaultValue: [] },
    localCounter: Symbol("my-mode:localCounter")
});

// Register mode that can access shared state
tui.registerMode({
    name: "my-mode",
    commands: function() { return {
        addItem: ({ state }) => {
            const items = state.get(shared.contextItems);
            items.push("new item");
            state.set(shared.contextItems, items);
        }
    }; }
});
```

### Command with Session Support

```go
// In a command implementation
type MyCommand struct {
    *BaseCommand
    session        string
    store string
    // ... other fields
}

func (c *MyCommand) SetupFlags(fs *flag.FlagSet) {
    fs.StringVar(&c.session, "session", "", "Session ID for state persistence")
    fs.StringVar(&c.store, "store", "", "Storage backend")
}

func (c *MyCommand) Execute(args []string, stdout, stderr io.Writer) error {
    engine, err := scripting.NewEngineWithConfig(ctx, stdout, stderr, c.session, c.store)
    // ... rest of implementation
}
```

### Context Manager with Shared State

```javascript
// In a script
const shared = require('osm:sharedStateSymbols');

const ctxmgr = contextManager({
    getItems: () => state.get(shared.contextItems) || [],
    setItems: (v) => state.set(shared.contextItems, v),
    nextIntegerId: nextIntegerId,
    buildPrompt: () => { /* build prompt logic */ }
});

// Use ctxmgr.commands as usual
```

## Built-in Modes and Command Interactions

The application includes several built-in modes that leverage storage for state persistence. Each mode defines specific state keys and commands that interact with the storage system. Persistence occurs automatically after each command execution when history is enabled for the mode.

### Goal Modes

These modes are all powered by the generic `internal/command/goal.js` script, which interprets different `GOAL_CONFIG` objects.
Built-in goals provide pre-configured modes for common development tasks. Each goal mode persists its state across sessions, allowing users to resume work seamlessly.

#### Comment Stripper Mode (`comment-stripper`)

**State Keys:**

  - `contextItems`: Array of context items (files, notes, diffs) for analysis

**Command Interactions:**

  - `add [file...]`: Adds files to `contextItems` for comment analysis
  - `note [text]`: Adds notes to `contextItems` for additional context
  - `list`: Displays current `contextItems` without modification
  - `edit <id>`: Modifies existing items in `contextItems`
  - `remove <id>`: Removes items from `contextItems`
  - `show`: Displays current `contextItems` without modification
  - `copy`: Copies current `contextItems` to clipboard without modification
  - `run [file...]`: Adds files to `contextItems` and displays the analysis prompt

#### Doc Generator Mode (`doc-generator`)

**State Keys:**

  - `contextItems`: Array of context items for documentation generation
  - `type`: Documentation type (`comprehensive`, `api`, `readme`, `inline`, `tutorial`)

**Command Interactions:**

  - `add [file...]`: Adds files to `contextItems`
  - `note [text]`: Adds notes to `contextItems`
  - `list`: Displays current `contextItems`
  - `edit <id>`: Modifies items in `contextItems`
  - `remove <id>`: Removes items from `contextItems`
  - `show`: Displays current `contextItems`
  - `copy`: Copies current `contextItems` to clipboard
  - `type <type>`: Sets `type` to specified value

#### Test Generator Mode (`test-generator`)

**State Keys:**

  - `contextItems`: Array of context items for test generation
  - `type`: Test type (`unit`, `integration`, `e2e`, `performance`, `security`)
  - `framework`: Testing framework (`auto`, `jest`, `mocha`, `go`, `pytest`, `junit`, `rspec`)

**Command Interactions:**

  - `add [file...]`: Adds files to `contextItems`
  - `note [text]`: Adds notes to `contextItems`
  - `list`: Displays current `contextItems`
  - `edit <id>`: Modifies items in `contextItems`
  - `remove <id>`: Removes items from `contextItems`
  - `show`: Displays current `contextItems`
  - `copy`: Copies current `contextItems` to clipboard
  - `type <type>`: Sets `type` to specified value
  - `framework <fw>`: Sets `framework` to specified value

#### Commit Message Mode (`commit-message`)

**State Keys:**

  - `contextItems`: Array of context items (diffs, notes) for commit message generation

**Command Interactions:**

  - `add [file...]`: Adds files to `contextItems`
  - `diff [args...]`: Adds git diff to `contextItems`
  - `note [text]`: Adds notes to `contextItems`
  - `list`: Displays current `contextItems`
  - `edit <id>`: Modifies items in `contextItems`
  - `remove <id>`: Removes items from `contextItems`
  - `show`: Displays current `contextItems`
  - `copy`: Copies current `contextItems` to clipboard
  - `run [args...]`: Adds git diff to `contextItems` and displays the prompt

### Prompt Flow Mode (`flow`)

**State Keys:**

  - `phase`: Current workflow phase (`INITIAL`, `CONTEXT_BUILDING`, `META_GENERATED`, `TASK_PROMPT_SET`)
  - `goal`: The user's goal description
  - `template`: Meta-prompt template (defaults to embedded template)
  - `metaPrompt`: Generated meta-prompt
  - `taskPrompt`: User-provided task prompt
  - `contextItems`: Array of context items

**Command Interactions:**

  - `goal [text]`: Sets `goal` and advances `phase` to `CONTEXT_BUILDING` if not already
  - `add [file...]`: Adds files to `contextItems`
  - `diff [args...]`: Adds git diff to `contextItems`
  - `note [text]`: Adds notes to `contextItems`
  - `list`: Displays current state including `phase`, `goal`, and `contextItems`
  - `edit <target>`: Edits `goal`, `template`, `meta`, `prompt`, or context items
  - `remove <id>`: Removes items from `contextItems`
  - `template [edit]`: Modifies `template`
  - `generate`: Generates `metaPrompt` from `goal` and `contextItems`, sets `phase` to `META_GENERATED`
  - `use [text]`: Sets `taskPrompt` and advances `phase` to `TASK_PROMPT_SET`
  - `show [meta|prompt]`: Displays `metaPrompt` or `taskPrompt`
  - `copy [meta|prompt]`: Copies `metaPrompt`, `taskPrompt`, or final assembled prompt to clipboard

### Code Review Mode (`review`)

**State Keys:**

  - `contextItems`: Array of context items for code review

**Command Interactions:**

  - `add [file...]`: Adds files to `contextItems`
  - `diff [args...]`: Adds git diff to `contextItems`
  - `note [text|--goals]`: Adds notes to `contextItems` or pre-written goal-based notes
  - `list`: Displays current `contextItems`
  - `edit <id>`: Modifies items in `contextItems`
  - `remove <id>`: Removes items from `contextItems`
  - `show`: Displays the generated code review prompt
  - `copy`: Copies the code review prompt to clipboard

### Persistence Behavior

For all built-in modes:

  - In-memory snapshots are captured after each command execution when `enableHistory` is `true` (the `StateManager` stores immutable history entries in-memory).
  - History includes full state snapshots for each command (recorded in-memory and available via `GetSessionHistory`).
  - State survives terminal restarts and script re-runs
  - Commands that modify state (add, edit, remove, set operations) update in-memory state immediately; persistence to the configured backend occurs on explicit `PersistSession()` calls or on shutdown/exit paths.
  - Read-only commands (list, show) do not modify state but may be logged in history

## Interoperability Between Distinct Commands

The storage system successfully enables interoperability between distinct OSM commands through shared state contracts and session-based persistence. Users can seamlessly switch between commands like `osm code-review`, `osm prompt-flow`, and `osm goal` while maintaining shared context.

### How Interoperability Works

1. **Shared State Contracts**: All commands use `tui.createState(commandName, definitions)` with shared symbols from `osm:sharedStateSymbols` to access shared `contextItems` via `shared.contextItems`.

2. **Session Flags**: Commands support `--session` and `--store` flags for explicit session sharing.

3. **State Persistence**: Shared state is persisted to configurable backends (filesystem or in-memory) and restored across command invocations.

4. **Mode Isolation**: While `contextItems` are shared, mode-specific state remains isolated per command.

5. **History Separation**: Histories are logically separated by ModeID within the session file, maintaining per-mode command sequences.

### Cross-Command Workflows

Users can now perform workflows like:

- Add files in `osm code-review --session shared-workflow`
- Switch to `osm prompt-flow --session shared-workflow` and access the same files
- Generate prompts and copy to clipboard
- Switch to `osm goal commit-message --session shared-workflow` and use the generated prompt

### Session Management

- **Automatic Discovery**: Session IDs are automatically determined via terminal path, environment variables, or UUID fallback
- **Explicit Override**: Users can specify `--session custom-id` for cross-terminal sharing
- **Backend Selection**: `--store fs` or `--store memory` controls persistence mechanism

### Benefits

- **Unified Context**: Build context once, use across all commands
- **Seamless Workflows**: Transition between prompt building, code review, and goal execution without losing state
- **Persistent Sessions**: State survives across multiple command invocations and terminal sessions
- **Mode Flexibility**: Switch modes while preserving shared state
- **Separate Concerns**: Mode-specific state and histories remain isolated where appropriate

## Configuration

### Environment Variables

  - `OSM_STORE`: Default backend (`fs` or `memory`)
  - `OSM_SESSION`: Default session ID

### Command Flags

  - `--store`: Override backend for session
  - `--session`: Override session ID

### Config File

The application config (`~/.osm/config.json`) can include storage-related settings, though currently minimal integration exists.

## Error Handling

  - **Backend Failures**: Logged as warnings; TUI continues in ephemeral mode
  - **Lock Conflicts**: FileSystem backend fails if session locked by another process
  - **Schema Mismatches**: Version validation; falls back to fresh state
  - **Serialization Errors**: Command execution continues; persistence skipped

## Testing

The storage system includes comprehensive testing:

  - **Unit Tests**: Backend implementations, state manager logic
  - **Integration Tests**: Full persistence workflows
  - **Concurrency Tests**: Multi-terminal state sharing
  - **Fuzz Tests**: Input validation and edge cases

Tests use the in-memory backend for isolation and speed.

## Future Enhancements

Potential improvements to the storage system:

  - **Migration Support**: Automatic schema upgrades
  - **Compression**: Reduce storage footprint for large histories
  - **Encryption**: Secure sensitive state data
  - **Remote Backends**: Cloud storage integration
  - **History Pruning**: Configurable retention policies

## Session Cleanup Mechanisms

### Current State

The storage system currently lacks any automatic cleanup mechanisms for session files. Sessions are persisted indefinitely as individual JSON files in the user's configuration directory (`{UserConfigDir}/one-shot-man/sessions/{sessionID}.session.json`). While history entries within each session are pruned to a maximum of 200 entries using a ring buffer implementation, the session files themselves accumulate without bound.

**Key Observations:**
- No TTL (time-to-live) or expiration mechanism for sessions
- No detection of "defunct" sessions (e.g., sessions tied to closed terminals or inactive users)
- No size-based cleanup when the number of session files becomes excessive
- No user-facing commands for manual session management or cleanup
- Session files may persist even after associated terminals or processes have terminated

**Potential Issues:**
- Disk space consumption over time as users create multiple sessions
- Accumulation of stale session files from temporary or one-off terminal instances
- No way to reclaim space from abandoned sessions
- Potential performance impact if the sessions directory contains thousands of files

### Identified Problem: Need for Session Cleanup Solution

The absence of session cleanup mechanisms represents a significant gap in the storage system's lifecycle management. This problem is actively asking for a comprehensive solution that addresses both automatic cleanup policies and manual management tools.

**Required Solution Components:**
1. **Automatic Cleanup Policies**: Implement configurable rules for session expiration based on:
   - Last access time (e.g., delete sessions not accessed in 30 days)
   - Session age (e.g., delete sessions older than 90 days)
   - Total session count limits (e.g., keep only the 100 most recent sessions)
   - Size-based thresholds (e.g., delete oldest sessions when total size exceeds 100MB)

2. **Defunct Session Detection**: Add logic to identify and clean up sessions that are no longer viable:
   - Terminal-specific sessions tied to non-existent terminals
   - Sessions with corrupted or invalid state
   - Sessions locked by crashed processes (stale lock files)

3. **Manual Management Tools**: Provide user commands for session administration:
  - `osm session list` - Show all existing sessions with metadata
  - `osm session delete <session-id>` - Manually remove specific sessions
  - `osm session clean` - Run automatic cleanup based on policies
  - `osm session info <session-id>` - Show detailed session information

4. **Configuration Options**: Allow users to customize cleanup behavior via config file:
   - `session.max_age_days`: Maximum age for sessions
   - `session.max_count`: Maximum number of sessions to retain
   - `session.auto_cleanup_enabled`: Enable/disable automatic cleanup
   - `session.cleanup_interval_hours`: How often to run automatic cleanup

5. **Safe Cleanup Implementation**: Ensure cleanup operations are atomic and safe:
   - Proper file locking during cleanup operations
   - Backup mechanisms before deletion
   - Graceful handling of locked or in-use sessions
   - Logging of cleanup activities for audit purposes

This cleanup functionality should be implemented as a new built-in command integrated into the main command registry, with appropriate testing and documentation.

## **Reset Command — historical retention & safe renaming**

### **Problem statement**

The `reset` REPL command is used interactively to clear a session's in-memory and persisted state back to defaults. Today this operation clears state in-place which loses the previous session contents. We must ensure the reset operation:
- preserves the previous session file for historical/forensic purposes,
- keeps the active session ID and its filename semantics intact for the continuing REPL process,
- is atomic and safe with respect to OS-level file locks and concurrent processes,
- integrates sensibly with session cleanup/retention policies (archives are handled by the cleanup engine), and
- exposes predictable and auditable behavior for users and tests.

### **High level design**

When the user runs `reset` inside an active session the system will:

1. Ensure the current session file is persisted to disk (persist the most recent in-memory state).
2. Acquire the session's file lock (the same one the engine already holds for an in-process session); fail if the session is active on another process/owner and the lock can't be obtained.
3. Atomically rename (move) the existing on-disk session file into an archive location using a deterministic, session-ID-preserving filename (see naming rules below).
4. Reinitialize the in-memory session to defaults (new CreatedAt/UpdatedAt, empty history or 1 meta history entry indicating a reset), then write a brand new session file under the original `{sessionID}.session.json` filename using the atomic write primitives already present.
5. Record a small reset metadata entry (either appended to global metadata or written as a companion file) to make auditing and test verification trivial.

This preserves historical data, keeps the current session ID stable for subsequent persistence and sharing, and ensures other instances cannot race with the reset process.

### **Filename / archive conventions**

- Primary session filename (unchanged): `{sessionDir}/{sessionID}.session.json`
- Archive filename pattern: `{sessionDir}/archive/{sanitizedSessionID}--reset--{UTC-ISO8601}--{counter}.session.json`
  - `sanitizedSessionID` = sessionID with filesystem-unsafe characters replaced with `_` to preserve readability and avoid path traversal issues
  - `UTC-ISO8601` = timestamp accurate to seconds or milliseconds, e.g. `2025-11-26T14-03-00Z` (colons replaced to avoid cross-platform filename issues)
  - `counter` = small monotonic integer to avoid collisions if multiple resets happen within the same timestamp

Using an `archive/` subdirectory keeps the main sessions dir tidy and helps the cleanup engine treat historical archives differently (e.g., stricter retention) while still retaining the sessionID as a visible prefix in filenames.

### **Detailed step-by-step implementation (code locations & behaviour)**

1. Add helper & path utilities
  - `internal/storage/paths.go` — add `SessionArchiveDir(sessionID string)` and `ArchiveSessionFilePath(sessionID string, ts time.Time, counter int) string` helpers; add `SanitizeFilename(input string) string`.

2. Filesystem backend API extension
  - `internal/storage/backend.go` — add a new optional method `ArchiveSession(sessionID string, destPath string) error` to enable backends to archive their on-disk representation. `fs_backend.go` implements this; `memory_backend.go` can no-op or provide a memory-record for tests.

3. Implement safe rename in filesystem backend
  - `internal/storage/fs_backend.go` — implement `ArchiveSession` to:
    - Ensure target `archive/` dir exists (atomic mkdir semantics),
    - Validate session file exists; fsync to persist current content,
    - Use `os.Rename` when in the same filesystem (fast and atomic). If a cross-device rename is required, fall back to copy-then-sync-and-remove with careful error handling.
    - Preserve file permissions and atomically remove temp artifacts on failure.

4. Reset command workflow
  - `internal/scripting/tui_manager.go` — in `resetAllState` replace the simple clear-without-history behavior with the archive+reinitialize workflow described above.
    - Before mutating the in-memory state, ensure current state persisted and call backend.ArchiveSession(sessionID, archivePath) while holding the session lock.
    - Reinitialize the session in-memory (new CreatedAt/UpdatedAt, new or small history entry describing the reset), then `PersistSession()` to write a new `{sessionID}.session.json` atomically.
    - Update global metadata (e.g. `~/.one-shot-man/metadata.json`) with an entry describing the reset event for auditing.

5. Locking and concurrency rules
  - The reset operation must only be allowed from the process that owns the session lock (the normal case inside an interactive REPL). If invoked from another process, fail unless `--force` is explicitly provided.
  - `--force` behavior: ensure the command explicitly documents the risk; if forced, attempt to acquire the lock by breaking a stale lock only after verifying the lock owner process does not exist (best-effort; platform-specific). Use a separate `TryBreakStaleLock()` helper that requires a CLI flag and logs the action.

6. Config & flags
  - Add optional `--keep-archives` (or config `session.reset.keep_archives=true`) to control whether reset keeps old sessions in archive dir or deletes them immediately (default: keep).
  - Add `--force` flag to allow forced reset that will attempt to break stale/active locks (present with caution and only when user understands the risk).

7. Tests
  - Unit tests for `ArchiveSessionFilePath` & `SanitizeFilename`.
  - Integration tests: `TestResetCommand_EndToEnd` should be extended to assert that after reset:
    - An archive file exists whose filename starts with the original `sessionID` and contains a `reset` timestamp.
    - New current `{sessionID}.session.json` exists and is empty/preset to defaults.
    - Global metadata contains an appended reset record.
  - Concurrency tests: simulate a second process holding the lock; assert reset fails without `--force`; with `--force` and a simulated dead owner the reset succeeds.

8. Cleanup integration
  - The cleanup engine should consider `archive/` paths as separate buckets (configurable). Default behavior: archive entries shorter TTL and stricter count limits than live sessions. Add tests ensuring the cleanup service deletes archives first according to policy.

9. Observability
  - Emit an audit event (structured log) when reset/rename happens, including before/after filenames, timestamp, user/host, and whether `--force` was used.

### **Safety & correctness rationale**

- Atomic `os.Rename` ensures historical file is moved instantly in the sessions directory if the operation happens on the same filesystem; the code falls back to copy+sync+remove when necessary.
- Holding the session lock (or failing safely when it isn't available) prevents two processes from racing to rename/replace the same session file.
- Using an `archive/` location prevents pollution of the primary sessions directory and makes automatic retention straightforward.
- Sanitizing session IDs in filenames prevents path traversal, special char issues, and cross-platform filename problems.

### **Backward-compatibility & migration**

- Existing session files remain valid. Reset will simply move them into `archive/` with a readable preserved prefix matching the previous session ID.
- The system should provide a small migration utility (or `osm session restore <archive-filename>`) to allow users to restore an archived session back to active status if needed.

### **Summary: deliverables**

- `internal/storage/paths.go`: archive path helpers + sanitizer
- `internal/storage/backend.go`: optional `ArchiveSession` method
- `internal/storage/fs_backend.go`: archive implementation (atomic rename, copy fallback)
- `internal/scripting/tui_manager.go`: resetAllState updated to archive + reinitialize
- tests in `internal/scripting/*_test.go` covering unit & integration cases
- docs updated (this section) and `osm session` command extended to list/restore archives

These changes ensure the `reset` REPL command retains historical session data in a safe, auditable, testable manner while keeping the active session's filename and session ID semantics unchanged.

## Revision History

### Version 1.0 - Initial Documentation

  - Basic overview of storage backend integration
  - High-level architecture description
  - Session identification and backend details

### Version 1.1 - Symbol Handling and Mapping

  - Added section describing JavaScript Symbol handling for state keys
  - Documented how symbol descriptions map to persistent keys
  - Verified storage backend supports shared state when symbols are registered

### Version 1.2 - Deep Investigation and Analysis

  - Added relevant files list
  - Detailed file analysis with roles and responsibilities
  - Analysis of design choices (Symbols, session persistence, backends)
  - Technical debt evaluation and test coverage notes

### Version 1.3 - Solution Development

  - Alternative solutions analysis with pros/cons and recommendations
  - Complete implementation plan with code examples
  - Testing strategy covering unit, integration, and end-to-end tests
  - Migration guide for developers and users
  - Rollback procedures

### Version 1.4 - Comprehensive Enhancement

  - Future considerations for advanced features
  - Security considerations
  - Glossary of terms
  - FAQ addressing common questions
  - References to code locations and tests
  - Practical code examples for implementation

### Version 1.6 - Implementation Success Documentation

  - Updated document to reflect successful shared state implementation
  - Replaced failure analysis with working architecture description
  - Updated interoperability section to describe current functionality
  - Revised conclusion to highlight successful implementation
  - Maintained comprehensive documentation of the working system

### Version 1.7 - Recent Fixes and Improvements

  - **Tmux Session ID Enhancement**: Modified `session_id_common.go` to include a SHA256 hash of the TMUX socket path in tmux pane session IDs for uniqueness across multiple tmux servers (format: `tmux-{socketHash}-{paneID}`).
  - **Idempotent Context Removal**: Updated `contextManager.js` remove command to tolerate missing files, treating "path not found" errors as informational and still removing stale items from session state.
  - **Persistence on Engine Close**: Enhanced `engine_core.go` to explicitly close the TUI manager in `Engine.Close()`, ensuring state is persisted to disk on shutdown.
  - **Test Coverage**: Added unit tests in `session_id_common_test.go` for tmux pane precedence, `cm_test.go` for remove missing file behavior, and `persistence_test.go` for close-time persistence verification.

## Conclusion

The storage system successfully implements the original design intent of leveraging ES6 Symbols for seamless cross-command state sharing. The implementation provides:

**Successful Shared State Architecture**: Scripts use `tui.createState(commandName, definitions)` with shared symbols from `osm:sharedStateSymbols` for common data like `contextItems`, enabling access across all commands via `shared.contextItems`.

**Session-Based Persistence**: Commands support `--session` and `--store` flags, allowing users to explicitly share state across commands and terminals.

**Robust Backend Implementation**: Filesystem backend with atomic writes, cross-platform file locking, and in-memory backend for testing, all supporting shared state persistence.

**Cross-Command Interoperability**: Users can build context in one command (e.g., `osm code-review`) and seamlessly access it in another (e.g., `osm goal commit-message`) using the same session.

**Mode Isolation with Shared Concerns**: While mode-specific state remains isolated, shared state contracts enable unified context management across the entire application.

This implementation fulfills the core requirement of unified context management while maintaining appropriate separation of concerns.
