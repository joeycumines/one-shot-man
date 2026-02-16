# Architecture

This document describes the internal architecture of `osm` — how the major subsystems connect, how data flows from CLI invocation to clipboard output, and where each component lives in the codebase.

---

## Entry point

`cmd/osm/main.go` is the sole entry point. It performs all top-level wiring in a single `run()` function:

1. **Configuration loading.** `config.Load()` reads the dnsmasq-style config file (default: `~/.one-shot-man/config`). If the file doesn't exist, an empty `Config` is used.

2. **Command registry.** `command.NewRegistryWithConfig(cfg)` creates the registry that maps command names to `Command` implementations.

3. **Goal subsystem.** `command.NewGoalDiscovery(cfg)` builds the discovery engine, then `command.NewDynamicGoalRegistry(builtIns, discovery)` merges built-in goals with user-discovered goals.

4. **Command registration.** All 14 built-in commands are registered:
   `help`, `version`, `config`, `init`, `script`, `session`, `prompt-flow`, `code-review`, `super-document`, `completion`, `goal`, `sync`, `log`, plus the completion command which receives both the registry and goal registry for tab-completion data.

5. **Flag parsing.** A global `FlagSet` parses top-level `-h`/`-help`; remaining args identify the command name and its arguments. Each command gets its own `FlagSet` (with `ContinueOnError`) so `SetupFlags` can register command-specific flags before `fs.Parse(cmdArgs)`.

6. **Execution.** `cmd.Execute(fs.Args(), os.Stdout, os.Stderr)` runs the command. Errors are printed to stderr with a non-zero exit code.

Source: [cmd/osm/main.go](../cmd/osm/main.go)

---

## Command system

### Command interface

Every command implements the `Command` interface:

```go
type Command interface {
    Name() string
    Description() string
    Usage() string
    SetupFlags(fs *flag.FlagSet)
    Execute(args []string, stdout, stderr io.Writer) error
}
```

### BaseCommand

`BaseCommand` provides the name/description/usage triplet and a no-op `SetupFlags`. All commands embed `*BaseCommand` for these fields.

Source: [internal/command/base.go](../internal/command/base.go)

### scriptCommandBase

Five commands execute JavaScript through the scripting engine: `script`, `prompt-flow`, `code-review`, `goal`, and `super-document`. These all embed `scriptCommandBase`, which provides:

- **Shared flags:** `--test`, `--session`, `--store`, `--log-level`, `--log-file`, `--log-buffer`
- **`RegisterFlags(fs)`:** Registers all shared flags on the command's FlagSet.
- **`PrepareEngine(ctx, stdout, stderr)`:** Creates a fully configured scripting engine with:
  - Logging (file + in-memory buffer)
  - Background session cleanup scheduler
  - Test mode toggle
  - Config-defined hot-snippet injection
  - Returns the engine + a cleanup function (released in reverse-acquisition order)

Source: [internal/command/script_command_base.go](../internal/command/script_command_base.go)

### Command registry

`Registry` maps command names to `Command` implementations. Supports `Register(cmd)`, `Get(name)`, `List()`, and `GetCommands()`. The help command uses the registry to enumerate all commands.

Source: [internal/command/registry.go](../internal/command/registry.go)

---

## Scripting engine

The scripting engine is the core runtime for JavaScript execution, backed by [Goja](https://github.com/dop251/goja) (a pure-Go ES5.1+ engine).

### Engine creation

`NewEngineDetailed(ctx, stdout, stderr, sessionID, store, logFile, bufferSize, logLevel, opts...)` is the full constructor. Simpler wrappers exist (`NewEngineWithConfig`).

Creation sequence:

1. Create `ContextManager` (file/diff/note management, rooted at CWD)
2. Create `TerminalIO` (shared terminal I/O for all TUI subsystems)
3. Create `require.Registry` with shebang-stripping loader + optional module paths
4. Create `Runtime` (wraps Goja's `eventloop.EventLoop` for async/Promise support)
5. Get a direct `*goja.Runtime` reference for sync operations
6. Register all native Go modules via `builtin.Register()`
7. Enable CommonJS `require()` on the runtime
8. Create `TUIManager` (mode/state/session management)
9. Wire `StateManager` → BubbleTea state refresh listener
10. Register `osm:sharedStateSymbols` module
11. Call `setupGlobals()` to expose global objects
12. Install context cancellation → VM interrupt

Source: [internal/scripting/engine_core.go](../internal/scripting/engine_core.go)

### EngineOption

Currently one option: `WithModulePaths(paths...)` configures additional module search paths for bare `require('mylib')` resolution (similar to `NODE_PATH`). Configured via `script.module-paths` in the config file.

### Event loop

The engine uses `goja_nodejs/eventloop` for async JavaScript execution. All JS code runs on the event loop goroutine. The `Runtime` wrapper provides:

- `RunOnLoopSync(fn)` — run synchronous code on the event loop, blocking until complete
- `RunOnLoop(fn)` — queue code asynchronously
- Event loop goroutine ID tracking for thread-safety assertions

### require() and module resolution

CommonJS `require()` is enabled via `goja_nodejs/require`. Resolution order:

1. **Native modules** (`osm:` prefix): Go-implemented modules registered via `RegisterNativeModule`
2. **Absolute paths**: `/path/to/module.js`
3. **Relative paths**: `./module.js`, `../lib/helper.js` (resolved from `__dirname`)
4. **Bare module names**: searched in configured `modulePaths` (from `WithModulePaths`)

A custom source loader strips `#!/...` shebang lines from scripts, matching Node.js behavior.

### Global objects

`setupGlobals()` exposes these objects to all scripts:

| Global | Purpose |
|--------|---------|
| `ctx` / `context` | Context manager API — add files, diffs, notes; build prompts |
| `output` | Output formatting — `print()`, `printf()`, clipboard operations |
| `log` | Logging API — `debug()`, `info()`, `warn()`, `error()` |
| `tui` | TUI manager — mode creation, state, prompts, keybindings |
| `args` | Command-line arguments passed to the script |

---

## Built-in modules

Native modules are registered via `builtin.Register()` in [internal/builtin/register.go](../internal/builtin/register.go). All use the `osm:` prefix for `require()`.

### Core utilities

| Module | Description |
|--------|-------------|
| `osm:argv` | Command-line argument parsing |
| `osm:exec` | Execute shell commands with stdout/stderr capture |
| `osm:flag` | Flag parsing with tab-completion support |
| `osm:os` | File I/O, environment variables, OS information |
| `osm:time` | Time utilities, sleep, duration formatting |
| `osm:fetch` | HTTP client (GET, POST, etc.) |
| `osm:grpc` | gRPC client with protobuf descriptor loading |

### Text and templates

| Module | Description |
|--------|-------------|
| `osm:text/template` | Go `text/template` style templating |
| `osm:unicodetext` | Unicode text utilities (width calculation, truncation) |

### TUI framework (Charm stack)

| Module | Description |
|--------|-------------|
| `osm:bubbletea` | BubbleTea TUI framework (Elm architecture) |
| `osm:lipgloss` | Terminal styling (colors, borders, layout) |
| `osm:bubblezone` | Zone-based mouse hit-testing for BubbleTea |
| `osm:bubbles/textarea` | Multi-line text input component |
| `osm:bubbles/viewport` | Scrollable content viewport |

### Other TUI components

| Module | Description |
|--------|-------------|
| `osm:termui/scrollbar` | Custom scrollbar widget |

### Behavior trees and planning

| Module | Description |
|--------|-------------|
| `osm:pabt` | Planning and Acting using Behavior Trees |
| `osm:ctxutil` | Context manager factory and REPL command helpers |

### State and identifiers

| Module | Description |
|--------|-------------|
| `osm:nextIntegerId` | Thread-safe monotonic integer ID generator |
| `osm:sharedStateSymbols` | Shared state symbol constants for cross-module state |

### Context management

The `osm:ctxutil` module provides the `contextManager` factory used by goals and interactive commands. It manages files, diffs, and notes as context items and provides the standard commands (`add`, `diff`, `note`, `list`, `edit`, `remove`, `show`, `copy`).

Source: [internal/builtin/ctxutil/contextManager.js](../internal/builtin/ctxutil/contextManager.js)

---

## Session and storage

### Session files

Sessions are JSON files stored in a platform-specific directory:

- **Linux/macOS:** `~/.local/share/osm/sessions/` (or `$XDG_DATA_HOME/osm/sessions/`)
- **Windows:** `%LOCALAPPDATA%\osm\sessions\`

Each session consists of:
- `<id>.session.json` — serialized state (context items, mode state, history)
- `<id>.session.lock` — advisory file lock preventing concurrent corruption

### Session ID resolution

Session IDs are auto-determined through a sophisticated hierarchy (see [session.md](session.md)):

1. `--session <id>` flag (highest priority)
2. `OSM_SESSION` environment variable
3. Terminal-specific auto-detection (TTY device, tmux pane, SSH session, etc.)
4. Fallback: `default`

Source: [internal/scripting/session_id_common.go](../internal/scripting/session_id_common.go)

### Storage backends

Two backends are registered in `storage.BackendRegistry`:

| Backend | Description |
|---------|-------------|
| `fs` | Filesystem-based persistence (default). Atomic writes with fsync. |
| `memory` | Ephemeral in-memory storage (for tests). |

Source: [internal/storage/registry.go](../internal/storage/registry.go)

### StateManager

`StateManager` persists and restores per-mode state across sessions. It tracks `contextItems`, prompt history, and arbitrary key-value state. State changes fire listeners (used to trigger BubbleTea re-renders).

### Cleanup

`storage.Cleaner` enforces retention policies: max age, max count, max total size. The `scriptCommandBase.PrepareEngine` starts a background scheduler when `sessions.auto-cleanup-enabled` is true.

Source: [internal/storage/cleaner.go](../internal/storage/cleaner.go)

---

## Configuration

### Format

Configuration uses a dnsmasq-style plaintext format:

```
# Global options
optionName value as rest of line

# Command-specific sections
[command-name]
optKey value

# Hot-snippets section
[hot-snippets]
my-snippet = This is the snippet text
```

Key characteristics:
- Option name is the first token; everything after the first space is the value.
- `[section]` headers scope options to specific commands.
- `[hot-snippets]` is a special section where `name = text` defines snippets.
- Comment lines start with `#`.

Source: [internal/config/config.go](../internal/config/config.go)

### Schema and validation

`config.DefaultSchema()` declares all known configuration keys with types, defaults, descriptions, and environment variable overrides. `config.ValidateAgainstSchema(cfg, schema)` checks for unknown keys and type mismatches.

The `osm config validate` command runs schema validation. `osm config schema` dumps the schema.

Source: [internal/config/schema.go](../internal/config/schema.go)

### Persistence

`osm config set <key> <value>` writes configuration values back to the config file using `config.SetKeyInFile()`, which performs a surgical in-place update preserving comments, ordering, and formatting. If the key doesn't exist, it's appended to the appropriate section.

### Environment variable overrides

Many config keys can be overridden via environment variables. Common overrides:

| Env Var | Config Key |
|---------|------------|
| `OSM_SESSION` | `--session` flag |
| `OSM_STORE` | `--store` flag |
| `OSM_LOG_LEVEL` | `log.level` |
| `OSM_DISABLE_GOAL_AUTODISCOVERY` | `goal.autodiscovery` |

---

## Goal system

### Overview

Goals are pre-written interactive workflows for common tasks. Each goal defines a TUI mode with prompt instructions, state variables, and commands.

### Sources (in precedence order)

1. **User-discovered JSON goals** — from goal directories (highest priority, override built-ins)
2. **User-discovered `.prompt.md` goals** — VS Code prompt files converted to goals
3. **Built-in goals** — 10 goals compiled into the binary (lowest priority)

### Discovery

`GoalDiscovery` scans multiple directories for goal files:

- Standard paths: `~/.one-shot-man/goals/`, `<exe-dir>/goals/`, `./osm-goals/`
- Configured paths: `goal.paths`
- Autodiscovery: upward traversal from CWD matching `goal.path-patterns` (default: `osm-goals,goals`)
- `.prompt.md` paths: `.github/prompts`, `prompt.file-paths`

Paths are scored by proximity to CWD and deduplicated.

Source: [internal/command/goal_discovery.go](../internal/command/goal_discovery.go)

### Registry

`DynamicGoalRegistry` merges built-in and discovered goals. User goals override built-ins on name collision. `Reload()` re-scans all paths.

Source: [internal/command/goal_registry.go](../internal/command/goal_registry.go)

### Execution

When a goal is invoked:

1. Goal config is marshalled to JSON
2. JSON is injected as `GOAL_CONFIG` into the JavaScript runtime
3. The embedded `goal.js` interpreter registers a TUI mode
4. For interactive runs, the TUI switches to the goal's mode

Source: [internal/command/goal.go](../internal/command/goal.go), [internal/command/goal.js](../internal/command/goal.js)

### Built-in goals

See [Goal reference](reference/goal.md) for the complete catalog of 10 built-in goals.

---

## Sync subsystem

The `sync` command provides local notebook save/list/load operations and git-based synchronization for prompt notebooks.

### Subcommands

| Subcommand | Description |
|------------|-------------|
| `save` | Save a prompt as a timestamped Markdown notebook entry |
| `list` | List saved notebook entries in reverse chronological order |
| `load` | Load a saved entry by slug, date, or partial match |
| `init` | Clone a git repository as the sync root |
| `push` | Stage, commit, and push notebooks to the remote |
| `pull` | Pull and rebase remote notebooks; auto-clones if configured |

### Startup integration

At program startup (in `main.go`), two sync-related hooks run before goal/script discovery:

1. **Auto-pull** (`SyncAutoPull`): if `sync.auto-pull = true` and the sync directory is an initialized git repo, runs `git pull --rebase` silently. Errors are logged to stderr but do not prevent startup.
2. **Discovery path injection** (`ApplySyncDiscoveryPaths`): if the sync directory contains `goals/` or `scripts/` subdirectories, those paths are appended to `goal.paths`, `script.paths`, and `script.module-paths` so they participate in goal/script autodiscovery.

### Architecture

- Notebooks are saved to `<sync-root>/notebooks/<YYYY>/<MM>/<date>-<slug>.md` with YAML frontmatter.
- The sync root defaults to `~/.one-shot-man/sync` and can be configured via `sync.local-path`.
- `init` clones a repository URL (from argument or `sync.repository` config key).
- `push`/`pull` use shell `git` commands for transport.
- `load` strips YAML frontmatter and outputs the entry body.

Source: [internal/command/sync.go](../internal/command/sync.go), [internal/command/sync_startup.go](../internal/command/sync_startup.go)

---

## Data flow

```
CLI args → main.go → Registry.Get(cmd)
                          ↓
                    cmd.SetupFlags(fs)
                    fs.Parse(args)
                    cmd.Execute(fs.Args())
                          ↓
              ┌───────────┴────────────┐
              │                        │
        Go commands              JS commands
     (config, session,      (script, prompt-flow,
      init, version,         code-review, goal,
      help, log,             super-document)
      completion)                  ↓
                           PrepareEngine()
                                  ↓
                          Engine created with:
                          - Goja VM + event loop
                          - 20 native modules
                          - TUI manager + state
                          - Session persistence
                          - Context manager
                                  ↓
                          ExecuteScript()
                                  ↓
                     ┌────────────┴─────────────┐
                     │                          │
              Interactive mode          Non-interactive
              (Terminal/REPL)           (script side-effects)
                     ↓
              User commands:
              add, diff, note,
              show, copy, etc.
                     ↓
              copy → clipboard
```

---

## Visuals

- [docs/visuals/architecture.md](visuals/architecture.md)
- [docs/visuals/workflows.md](visuals/workflows.md)
