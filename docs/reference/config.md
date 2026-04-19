# Configuration reference

This page is the deep reference for `osm` configuration: what keys exist today, what they do, and how discovery behaves.

If you just want the basics, start with [Configuration](../configuration.md).

## Location

- Default: `~/.osm/config`
- Override: `OSM_CONFIG=/path/to/config`

Create a starter config:

```sh
osm init
```

## File format

Each non-empty, non-comment line is:

```
optionName remainingLineIsTheValue
```

Command-specific sections:

```
[command-name]
optionName remainingLineIsTheValue
```

Notes:
- Lines starting with `#` are comments.
- Values are not quoted/escaped; the “value” is the remainder of the line.
## Subcommands

The `osm config` command supports the following subcommands:

| Subcommand | Description |
|------------|-------------|
| `config <key>` | Get the effective value for a key |
| `config <key> <value>` | Set a configuration value |
| `config validate` | Validate configuration against the schema |
| `config schema` | Show the full configuration schema |
| `config list` | List all values with their sources |
| `config diff` | Show only non-default values |
| `config reset <key>` | Reset a single key to its schema default |
| `config reset --all --force` | Reset all global keys to their defaults |

### `config reset`

Reset configuration keys to their schema defaults. The key is removed from
both the in-memory configuration and the config file on disk.

```sh
# Reset a single key
osm config reset color

# Reset all global keys (requires --force)
osm config reset --all --force
```

When resetting a single key, the key must be a known schema key (use
`osm config schema` to list all known keys). The command prints the default
value after the reset.

When resetting all keys with `--all`, the `--force` flag is required as a
safety measure. All global option lines are removed from the config file;
comments, section headers, and command-specific options are preserved.

## Prompt colors (`prompt.color.*`)

Interactive flows (`osm script -i`, `osm prompt-flow`, `osm code-review`, `osm goal -i`) support optional prompt color overrides.

Keys (mirrors go-prompt “color roles”):
- `prompt.color.input`
- `prompt.color.prefix`
- `prompt.color.suggestionText`
- `prompt.color.suggestionBackground`
- `prompt.color.selectedSuggestionText`
- `prompt.color.selectedSuggestionBackground`
- `prompt.color.descriptionText`
- `prompt.color.descriptionBackground`
- `prompt.color.selectedDescriptionText`
- `prompt.color.selectedDescriptionBackground`
- `prompt.color.scrollbarThumb`
- `prompt.color.scrollbarBackground`

Allowed values (named colors):

```
black,darkred,darkgreen,brown,darkblue,purple,cyan,lightgray,
darkgray,red,green,yellow,blue,fuchsia,turquoise,white
```

Example:

```text
prompt.color.input green
prompt.color.prefix cyan
prompt.color.selectedSuggestionBackground cyan
```

## Script command discovery

Script discovery controls “external script commands” that show up as `osm <name>`.

A script command is any executable file found in a discovered scripts directory.
- Unix: executable bit must be set.
- Windows: executable extensions are recognized (`.exe`, `.com`, `.bat`, `.cmd`).

When invoked, `osm <name> ...` launches the script as a child process.

### Keys

- `script.autodiscovery` (bool, default `false`)
  - Enables additional discovery beyond the legacy locations.
- `script.paths` (list)
  - Extra directories to search.
- `script.path-patterns` (list, default `scripts`)
  - Directory names to look for during traversal.
- `script.git-traversal` (bool, default `false`)
  - If enabled (and `script.autodiscovery` is true), traverse upwards looking for git repos and script dirs.
  - Requires `git` on `PATH`.
- `script.max-traversal-depth` (int, default `10`)
  - Limits upward traversal depth. Valid range is `1..100` (invalid values fall back to default).
- `script.disable-standard-paths` (bool, default `false`)
  - Disables the standard script search locations (executable-dir, config-dir, and `./scripts`).
  - Primarily useful for deterministic setups (e.g., tests).
- `script.debug-discovery` (bool, default `false`)
  - Enables debug logging for script discovery. Useful for troubleshooting why a script is or isn't being found.
- `script.module-paths` (path-list, default empty)
  - Additional module search paths for `require()` in JavaScript scripts.
  - Uses the platform list separator (`:` on Unix, `;` on Windows) or comma separation.

Lists accept comma separation or the platform list separator (`:` on Unix, `;` on Windows).

Example:

```text
script.autodiscovery true
script.git-traversal true
script.max-traversal-depth 5
script.path-patterns scripts,tools
script.paths ~/my-scripts:/opt/shared-scripts
```

### Search order (high level)

`osm` always starts with a small set of “legacy” locations, then optionally adds configured and autodiscovered paths:

1. `scripts/` next to the `osm` executable
2. `scripts/` next to the config file (e.g. `~/.osm/scripts/`)
3. `./scripts/` in the current working directory
4. `script.paths` (if set)
5. Autodiscovery results (if enabled)

Discovered paths are deduplicated and then ranked roughly by “closeness” to your current working directory, then config dir, then executable dir.

## Goal discovery

Goal discovery controls custom goal definitions (JSON), in addition to the built-in goals.

See also: [Goal reference](goal.md).

### Keys

- `goal.autodiscovery` (bool, default `true`)
- `goal.disable-standard-paths` (bool, default `false`)
  - Disables the standard search locations (config-dir, executable-dir, and `./osm-goals`).
  - Primarily useful for deterministic setups (e.g., tests), but available in normal runs.
- `goal.paths` (list)
- `goal.path-patterns` (list, default `osm-goals,goals`)
- `goal.max-traversal-depth` (int, default `10`)
- `goal.debug-discovery` (bool, default `false`)
  - Enables debug logging for goal discovery. Useful for troubleshooting why a goal is or isn't being found.

Environment override:
- `OSM_DISABLE_GOAL_AUTODISCOVERY=true` disables goal autodiscovery.

### Standard search locations

Unless disabled, `osm` searches these standard locations:

1. `goals/` next to the config file (e.g. `~/.osm/goals/`)
2. `goals/` next to the `osm` executable
3. `./osm-goals/` in the current working directory

When autodiscovery is enabled, it also traverses upward looking for directories matching `goal.path-patterns`.

## Session cleanup (`[sessions]`)

The `[sessions]` section controls session retention and cleanup thresholds.

These settings drive automatic session cleanup and the `osm session clean` command. When `autoCleanupEnabled` is true, cleanup runs automatically on startup of session-creating commands (`script`, `prompt-flow`, `code-review`, `goal`) at the configured interval. See [Session: Automatic cleanup](../session.md#automatic-cleanup) for operational details.

### Keys

- `maxAgeDays` (int, default `90`)
  - Maximum age of sessions in days. Sessions older than this are eligible for cleanup.
  - Must be non-negative (`0` means all sessions are eligible).
- `maxCount` (int, default `100`)
  - Maximum number of sessions to retain.
  - Must be non-negative (`0` means no limit on count).
- `maxSizeMB` (int, default `500`)
  - Maximum total size of all sessions in megabytes.
  - Must be non-negative (`0` means no limit on size).
- `autoCleanupEnabled` (bool, default `true`)
  - Enables automatic session cleanup on startup. When true, the cleanup scheduler runs before each script-executing command, applying `maxAgeDays`, `maxCount`, and `maxSizeMB` thresholds.
  - Accepts: `true`, `false`, `1`, `0`, `yes`, `no`, `on`, `off` (case-insensitive).
- `cleanupIntervalHours` (int, default `24`)
  - Minimum hours between automatic cleanup runs. The scheduler skips cleanup if less than this interval has elapsed since the last run.
  - Must be at least `1`.

Example:

```text
[sessions]
maxAgeDays 30
maxCount 50
maxSizeMB 250
autoCleanupEnabled true
cleanupIntervalHours 12
```

## PR splitting (`[pr-split]`)

The `[pr-split]` section configures default options for `osm pr-split`. All keys can be overridden by CLI flags; flags always take precedence over config values.

### Keys

| Key | Type | Default | CLI flag | Description |
|-----|------|---------|----------|-------------|
| `base` | string | `main` | `--base` | Base branch to diff against |
| `strategy` | string | `directory` | `--strategy` | Grouping strategy: `directory`, `directory-deep`, `extension`, `chunks`, `dependency`, `auto` |
| `max` | int | `10` | `--max` | Maximum files per split branch |
| `prefix` | string | `split/` | `--prefix` | Branch name prefix |
| `verify` | string | `make test` | `--verify` | Verification command run after each split |
| `dry-run` | bool | `false` | `--dry-run` | Plan without creating branches |
| `claude-command` | string | _(auto-detect)_ | `--claude-command` | Claude binary path. Auto-detects `claude` or `ollama` if empty |
| `claude-arg` | string (repeatable) | _(empty)_ | `--claude-arg` | Additional CLI argument for Claude (repeatable — one arg per entry) |
| `claude-model` | string | _(empty)_ | `--claude-model` | Model name (provider-dependent) |
| `claude-config-dir` | string | _(empty)_ | `--claude-config-dir` | Claude config directory override |
| `claude-env` | string | _(empty)_ | `--claude-env` | Extra environment variables (`KEY=VALUE,KEY=VALUE`) |

### Runtime-only settings

These settings are available via the interactive TUI `set` command but are not
persisted in the config file or exposed as CLI flags:

| Key | Type | Default | TUI command | Description |
|-----|------|---------|-------------|-------------|
| `retry-budget` | int | `3` | `set retry-budget N` | Max resolve attempts per failed split during `fix` / `auto-split` |
| `mode` | string | `heuristic` | `set mode <value>` | Splitting mode (`auto` or `heuristic`) |

### Example

```text
[pr-split]
base develop
strategy extension
max 8
prefix feature-split/
verify go test ./...
claude-command claude
claude-model sonnet
```

## Claude Code multiplexer (`[claude-mux]`)

The `[claude-mux]` section configures the Claude Code agent orchestration system used by the `mcp-bridge` command and `osm:claudemux` scripting module.

### Keys

- `provider` (string, default `claude-code`)
  - AI provider name. Determines how agents are spawned.
- `model` (string, default _(empty)_)
  - Model identifier passed to the provider. When empty, uses the provider's default.
- `work-dir` (string, default _(empty)_)
  - Working directory for spawned agents. When empty, uses the current working directory.
- `env-inherit` (bool, default `true`)
  - Whether agents inherit the parent process's environment variables.
  - Accepts: `true`, `false`, `1`, `0`, `yes`, `no`, `on`, `off` (case-insensitive).
- `env` (string, default _(empty)_)
  - Additional environment variable in `KEY=VALUE` format to set for agent processes.
- `env-profile` (string, default _(empty)_)
  - Active environment variable profile name. Profiles are defined as `[claude-mux.env.<profile>]` sections.
- `pre-spawn-hook` (string, default _(empty)_)
  - Path to a JavaScript file executed before each agent spawn. Useful for credential injection or dynamic environment setup.
- `permission-policy` (string, default `reject`)
  - How permission prompts from agents are handled. `reject` automatically denies permission requests; `ask` surfaces them to the user.
- `rate-limit-backoff-sec` (int, default `30`)
  - Initial backoff duration in seconds when an agent encounters API rate limits.
- `max-agents` (int, default `4`)
  - Maximum number of concurrent agents in the pool.
- `pty-rows` (int, default `24`)
  - Row count for agent PTY allocation.
- `pty-cols` (int, default `80`)
  - Column count for agent PTY allocation.
- `provider-command` (string, default _(empty)_)
  - Override the provider executable path. When empty, the provider is resolved via `$PATH`.
- `mcp-servers` (string, default _(empty)_)
  - Comma-separated MCP server commands to configure for agents.

Example:

```text
[claude-mux]
provider claude-code
model sonnet
max-agents 2
permission-policy ask
pty-rows 40
pty-cols 120
```

## Global options

These keys can appear at the top level of the config file (outside any `[section]`).
All keys, types, defaults, and environment variable overrides are defined in the schema
(`DefaultSchema()` in `internal/config/schema.go`).

### Core options

| Key | Type | Default | Env var | Description |
|-----|------|---------|---------|-------------|
| `config.schema-version` | int | `1` | — | Configuration schema version (do not edit manually) |
| `verbose` | bool | `false` | — | Enable verbose output |
| `color` | string | `auto` | — | Color mode: `auto`, `always`, `never` |
| `pager` | string | _(empty)_ | — | Pager program for long output |
| `format` | string | _(empty)_ | — | Default output format |
| `output` | string | _(empty)_ | — | Default output destination |
| `timeout` | duration | _(empty)_ | — | Default command timeout (e.g. `30s`, `5m`) |
| `editor` | string | _(empty)_ | `EDITOR` | Editor for interactive editing |
| `session.id` | string | _(empty)_ | `OSM_SESSION_ID` | Override session ID |
| `debug` | bool | `false` | — | Enable debug mode |
| `quiet` | bool | `false` | — | Suppress non-essential output |

Example:

```text
verbose true
color always
editor vim
timeout 30s
quiet false
```

### Prompt file paths

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `prompt.file-paths` | path-list | _(empty)_ | Additional directories to search for `.prompt.md` files |
| `prompt.recursive` | bool | `true` | Scan prompt directories recursively for `.prompt.md` files |

Accepts the platform list separator (`:` on Unix, `;` on Windows) or comma separation.

When `prompt.recursive` is enabled (the default), dedicated prompt file directories
(including `.github/prompts` and any paths from `prompt.file-paths`) are scanned
recursively up to 10 levels deep. Hidden directories (starting with `.`) are skipped,
and symlink cycles are detected and avoided. This matches VS Code's behavior of
searching subdirectories under `.github/prompts`.

Set `prompt.recursive false` to restrict scanning to top-level files only.

### Hot-snippet global option

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `hot-snippets.no-warning` | bool | `false` | Suppress warning when using embedded (builtin) hot-snippets |

See [Hot-snippets section](#hot-snippets-hot-snippets) below for the `[hot-snippets]` section format.

### Logging options

| Key | Type | Default | Env var | Description |
|-----|------|---------|---------|-------------|
| `log.file` | string | _(empty)_ | `OSM_LOG_FILE` | Log file path (JSON output) |
| `log.level` | string | `info` | `OSM_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` |
| `log.max-size-mb` | int | `10` | — | Max log file size in MB before rotation |
| `log.max-files` | int | `5` | — | Max number of rotated log backup files |
| `log.buffer-size` | int | `1000` | — | In-memory log buffer size (entries) |

Example:

```text
log.file /tmp/osm.log
log.level debug
log.max-size-mb 25
log.max-files 3
```

### Sync options

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `sync.repository` | string | _(empty)_ | Git repository URL for sync |
| `sync.auto-pull` | bool | `false` | Auto-pull on startup |
| `sync.local-path` | string | _(empty)_ | Local path for sync repository |
| `sync.config-sync` | bool | `false` | Enable shared config syncing |
| `sync.config-sha` | string | _(empty)_ | SHA256 of last synced shared config (internal) |

#### Shared config sync

The sync repository can store a shared configuration file at `config/shared.conf`. This enables syncing non-sensitive configuration keys across machines.

**Config push** (`osm sync config-push`): Writes shareable global config keys to the sync repo. Sensitive keys (`sync.*`, `log.file`, `session.*`) are automatically excluded.

**Config pull** (`osm sync config-pull [--force]`): Reads shared config from the sync repo and merges keys into the running configuration. Conflict handling:

- **First pull (no stored SHA):** Requires `--force` flag to prevent accidental overwrite of manually configured settings.
- **Already applied (SHA matches):** No-op — shared config is up to date.
- **Remote changed (SHA differs):** Auto-applies the new shared config.

The shared config file uses a versioned format:
```
# osm-shared-config-version 1
goal.autodiscovery true
prompt.template my-template
```

Schema version mismatches (newer version than supported) produce a clear error message.

## Hot-snippets (`[hot-snippets]`)

The `[hot-snippets]` section defines named text snippets that can be quickly copied to the clipboard from interactive modes (e.g. `osm goal -i`, `osm prompt-flow`).

### Format

Each line in the section defines a snippet or sets a description:

```
snippetName text of the snippet
snippetName.description Help text for the snippet
```

- **Name** is the first word on the line.
- **Text** is the remainder of the line after the name.
- Literal `\n` sequences in the text are converted to actual newlines.
- A `.description` suffix sets a description on the most recently defined snippet with that name.
- Duplicate names are allowed; `.description` applies to the *last* snippet with that name.
- Comments (`#` lines) and empty lines are ignored as usual.

### Example

```text
[hot-snippets]
# Quick follow-up prompt
followup Continue with the same context and approach.
followup.description Follow-up prompt for continuing a conversation

# Multi-line snippet using \n
kickoff You are an expert software engineer.\nPlease review the following code.
kickoff.description Kickoff prompt for code reviews

# Snippet with no text (e.g., used as a separator or placeholder)
blank
```

Goals can also define their own hot-snippets in the goal JSON (`hotSnippets` field), which are merged with config-file hot-snippets at runtime. Goal-defined snippets take precedence when names collide.

## Command-specific sections

Command sections override global options for a specific command. Any global key can be used inside a command section as a fallback. The following section-specific keys are also registered:

### `[help]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `pager` | string | _(empty)_ | Pager for help output |
| `format` | string | _(empty)_ | Help output format |
| `output` | string | _(empty)_ | Help output destination |

### `[version]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `format` | string | _(empty)_ | Version output format |
| `output` | string | _(empty)_ | Version output destination |

### `[prompt]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `template` | string | _(empty)_ | Default prompt template |
| `output` | string | _(empty)_ | Prompt output destination |
| `editor` | string | _(empty)_ | Editor for prompt editing |
| `add-context` | string | _(empty)_ | Auto-add context items |

### `[session]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `list` | string | _(empty)_ | Session list format |
| `delete` | string | _(empty)_ | Session deletion mode |
| `export` | string | _(empty)_ | Session export format |
| `import` | string | _(empty)_ | Session import format |

## Environment variables

### Schema-declared env var overrides

These environment variables override the corresponding config-file key. When set (even to an empty string), they take precedence over the config file value and the schema default.

| Env var | Overrides key | Description |
|---------|---------------|-------------|
| `OSM_SESSION_ID` | `session.id` | Override session ID |
| `EDITOR` | `editor` | Editor for interactive editing |
| `OSM_LOG_FILE` | `log.file` | Log file path |
| `OSM_LOG_LEVEL` | `log.level` | Log level |

### Behavioral env vars

These environment variables are not config-file keys but affect runtime behavior:

| Env var | Description |
|---------|-------------|
| `OSM_CONFIG` | Config file path override (default: `~/.osm/config`) |
| `OSM_SESSION` | Override session ID selection |
| `OSM_STORE` | Storage backend name (`fs` default, or `memory`) |
| `OSM_CLIPBOARD` | Override clipboard-copy command (used by JS module `osm:os`) |
| `OSM_DISABLE_GOAL_AUTODISCOVERY` | Set to `true` to disable goal autodiscovery |
| `VISUAL` / `EDITOR` | Editor command used when a workflow opens an editor |
