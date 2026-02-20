# Session: State per _logical_ terminal

Many `osm` workflows are interactive. Sessions are how `osm` persists state *locally* so you can resume a flow later (or keep separate flows per terminal).

## What a session is

A session is a small JSON file plus a lock file in the sessions directory:

- `<id>.session.json`
- `<id>.session.lock`

Get your sessions directory:

```sh
osm session path
```

List sessions:

```sh
osm session list
```

## Session IDs (tying state to terminals)

By default, `osm` tries hard to choose a stable session id for “this terminal”, so you don’t have to think about it.

Overrides, in priority order:

1. `--session <id>` flag (supported on `script`, `prompt-flow`, `code-review`, `goal`)
2. `OSM_SESSION=<id>` environment variable

To see what id would be used:

```sh
osm session id
```

The `id` subcommand also accepts `--session <id>` to test overrides:

```sh
osm session id --session my-custom-id
```

For a deep dive into how session ids are determined, see:

- [Sophisticated session-id auto-determination](reference/sophisticated-auto-determination-of-session-id.md)

The auto-determination hierarchy (highest to lowest priority):

1. **Explicit override** (`--session` flag or `OSM_SESSION` env)
2. **Multiplexer** (tmux, GNU Screen)
3. **SSH context** (`SSH_CONNECTION`)
4. **macOS GUI terminal** (`TERM_SESSION_ID`, darwin only)
5. **Deep Anchor** — process tree walk (Linux via `/proc`, macOS via `sysctl`, Windows via `CreateToolhelp32Snapshot`)
6. **UUID fallback**

## Subcommands

### `session list`

Show all existing sessions with metadata (ID, update time, size, active/idle status).

```sh
osm session list
osm session list -format json
osm session list -sort active
```

**Flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `-format <text\|json>` | `text` | Output format. `text` prints tab-separated lines; `json` prints a pretty JSON array of session objects. |
| `-sort <default\|active>` | `default` | Sorting behavior. `default` uses filesystem discovery order. `active` surfaces active sessions first, then orders by update time (newest first). |

When no sessions exist, text format prints `"No sessions found"` and json format prints `[]`.

### `session info`

Show the raw JSON data for a specific session.

```sh
osm session info <session-id>
```

Requires exactly one session ID argument. Prints the full contents of the session JSON file to stdout.

### `session delete`

Remove specific sessions from storage. **This is irreversible** — deleted sessions cannot be recovered.

```sh
# Preview what would be deleted
osm session delete -dry-run <session-id>

# Delete with confirmation prompt
osm session delete <session-id>

# Delete without confirmation
osm session delete -y <session-id>

# Delete multiple sessions at once
osm session delete -y session-1 session-2 session-3
```

**Flags:**

| Flag | Description |
|------|-------------|
| `-dry-run` | Don't actually delete; show what would be deleted. |
| `-y` | Assume yes to confirmation prompts. |

**Behavior:**

- Requires at least one session ID.
- Refuses to delete sessions that are currently active (locked by another process).
- When deleting multiple sessions, a single confirmation prompt is shown (unless `-y`).
- Flags can appear before or after session IDs (e.g., `delete id -y` works).
- Use `--` to separate flags from session IDs that start with a hyphen.

### `session clean`

Run automatic cleanup based on configured retention policies (`maxAgeDays`, `maxCount`, `maxSizeMB` under `[sessions]`).

```sh
osm session clean -dry-run   # Preview
osm session clean -y         # Clean without confirmation
osm session clean            # Clean with confirmation prompt
```

**Flags:** `-dry-run`, `-y` (same as delete).

### `session purge`

Permanently purge sessions, **ignoring** configured retention policies. Removes all non-active sessions.

```sh
osm session purge -dry-run   # Preview
osm session purge -y         # Purge without confirmation
```

**Flags:** `-dry-run`, `-y` (same as delete).

### `session path`

Show the sessions directory or a specific session file path.

```sh
osm session path                  # Show sessions directory
osm session path <session-id>     # Show full path to session file
```

### `session id`

Resolve and print the session ID that would be used for this terminal.

```sh
osm session id
osm session id --session override-id
```

## Cleanup and deletion

Safe defaults:

- `clean` uses built-in retention policies (and asks for confirmation unless `-y`)
- `purge` ignores retention policies (also asks for confirmation)
- `delete` deletes explicit IDs

Always start with dry run:

```sh
osm session clean -dry-run
osm session purge -dry-run
osm session delete -dry-run <session-id>
```

### Automatic cleanup

When `autoCleanupEnabled` (under `[sessions]`) is `true` (the default), commands that create sessions (`script`, `prompt-flow`, `code-review`, `goal`) start a background cleanup scheduler. Cleanup runs on startup and then at the interval configured by `cleanupIntervalHours` (default: 24).

Configuration keys (under `[sessions]` section):

| Key | Default | Description |
|-----|---------|-------------|
| `maxAgeDays` | 90 | Remove sessions older than this |
| `maxCount` | 100 | Keep at most this many sessions |
| `maxSizeMB` | 500 | Total size cap for all sessions |
| `autoCleanupEnabled` | true | Enable background cleanup |
| `cleanupIntervalHours` | 24 | Hours between cleanup runs |

## Storage backends

Most commands that persist state accept:

- `-store fs` (default)
- `-store memory` (non-persistent; useful for tests)

You can also set `OSM_STORE=memory`.

The storage backend is selected at engine creation time. The `fs` backend writes to the sessions directory (platform-specific; see `osm session path`). The `memory` backend is ephemeral and loses all data when the process exits.

See: [internal/storage/registry.go](../internal/storage/registry.go) for backend registration.
