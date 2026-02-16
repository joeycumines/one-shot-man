# Configuration reference

This page is the deep reference for `osm` configuration: what keys exist today, what they do, and how discovery behaves.

If you just want the basics, start with [Configuration](../configuration.md).

## Location

- Default: `~/.one-shot-man/config`
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
2. `scripts/` next to the config file (e.g. `~/.one-shot-man/scripts/`)
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

1. `goals/` next to the config file (e.g. `~/.one-shot-man/goals/`)
2. `goals/` next to the `osm` executable
3. `./osm-goals/` in the current working directory

When autodiscovery is enabled, it also traverses upward looking for directories matching `goal.path-patterns`.

## Session cleanup (`[sessions]`)

The `[sessions]` section controls session retention and cleanup thresholds.

These settings drive the `osm session cleanup` workflow, which removes sessions that exceed the configured limits.

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

## Global options

These keys can appear at the top level of the config file (outside any `[section]`).
All keys, types, defaults, and environment variable overrides are defined in the schema
(`DefaultSchema()` in `internal/config/schema.go`).

### Core options

| Key | Type | Default | Env var | Description |
|-----|------|---------|---------|-------------|
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

Accepts the platform list separator (`:` on Unix, `;` on Windows) or comma separation.

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

### Sync options (reserved)

These keys are parsed and validated but the sync feature is not yet implemented.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `sync.repository` | string | _(empty)_ | Git repository URL for sync |
| `sync.enabled` | bool | `false` | Enable git synchronisation |
| `sync.auto-pull` | bool | `false` | Auto-pull on startup |
| `sync.local-path` | string | _(empty)_ | Local path for sync repository |

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
| `OSM_CONFIG` | Config file path override (default: `~/.one-shot-man/config`) |
| `OSM_SESSION` | Override session ID selection |
| `OSM_STORE` | Storage backend name (`fs` default, or `memory`) |
| `OSM_CLIPBOARD` | Override clipboard-copy command (used by JS module `osm:os`) |
| `OSM_DISABLE_GOAL_AUTODISCOVERY` | Set to `true` to disable goal autodiscovery |
| `VISUAL` / `EDITOR` | Editor command used when a workflow opens an editor |
