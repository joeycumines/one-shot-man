# Configuration

## Overview

`osm` is intentionally local-first. Configuration is plain text and loosely mirrors the style of tools like `git`/`kubectl`: a global section plus optional command-specific sections.

N.B. A small set of environment variables also affect behavior; see below.

## Location

- Default: `~/.one-shot-man/config`
- Override: `OSM_CONFIG=/path/to/config`

Create a starter config:

```sh
osm init
```

## File format

Each non-comment line is:

```
optionName remainingLineIsTheValue
```

Command-specific sections are:

```
[command-name]
optionName remainingLineIsTheValue
```

## What’s currently configurable

### Script discovery (adds `osm <script-name>` commands)

See the deep reference for the full set and defaults:
- [Configuration reference](reference/config.md)

Highlights:
- `script.autodiscovery` (bool, default false)
- `script.git-traversal` (bool, default false)
- `script.paths` (list)
- `script.path-patterns` (list, default `scripts`)
- `script.max-traversal-depth` (int)
- `script.module-paths` (path-list) — additional module search paths for bare `require()` in JavaScript scripts

### Goal discovery

Also documented in the deep reference:
- `goal.autodiscovery` (bool, default true)
- `goal.paths` (list)
- `goal.path-patterns` (list)
- `goal.max-traversal-depth` (int)

### Prompt colors

The interactive prompt supports configurable colors via keys like `prompt.color.*`.

Keys:
- `prompt.color.input`, `prompt.color.prefix`
- `prompt.color.suggestionText`, `prompt.color.suggestionBackground`
- `prompt.color.selectedSuggestionText`, `prompt.color.selectedSuggestionBackground`
- `prompt.color.descriptionText`, `prompt.color.descriptionBackground`
- `prompt.color.selectedDescriptionText`, `prompt.color.selectedDescriptionBackground`
- `prompt.color.scrollbarThumb`, `prompt.color.scrollbarBackground`

Allowed values are named colors:

```
black,darkred,darkgreen,brown,darkblue,purple,cyan,lightgray,
darkgray,red,green,yellow,blue,fuchsia,turquoise,white
```
### Prompt file paths

- `prompt.file-paths` (path-list) — additional directories to search for `.prompt.md` files

Accepts the platform list separator (`:` on Unix, `;` on Windows) or comma separation.

### Hot-snippets

The `[hot-snippets]` section defines named text snippets that can be quickly copied to the clipboard from interactive modes (e.g. `osm goal -i`, `osm prompt-flow`).

Each line defines a snippet or sets a description:

```text
[hot-snippets]
snippetName text of the snippet
snippetName.description Help text for the snippet
```

- **Name** is the first word on the line.
- **Text** is the remainder of the line after the name.
- Literal `\n` sequences in the text are converted to actual newlines.
- A `.description` suffix sets a description for the named snippet.
- The global option `hot-snippets.no-warning` (bool, default `false`) suppresses the warning when using embedded (builtin) hot-snippets.

Goals can also define their own hot-snippets in the goal JSON (`hotSnippets` field), which are merged with config-file hot-snippets at runtime.

See the [configuration reference](reference/config.md#hot-snippets-hot-snippets) for the full format specification.

### Logging

Structured JSON logging can be enabled by setting a log file path:

```text
log.file /tmp/osm.log
log.level debug
log.max-size-mb 25
log.max-files 3
```

Keys:
- `log.file` (string) — log file path; enables JSON logging when set (env: `OSM_LOG_FILE`)
- `log.level` (string, default `info`) — log level: `debug`, `info`, `warn`, `error` (env: `OSM_LOG_LEVEL`)
- `log.max-size-mb` (int, default `10`) — max log file size in MB before rotation
- `log.max-files` (int, default `5`) — max number of rotated log backup files
- `log.buffer-size` (int, default `1000`) — in-memory log buffer size (entries)

### Sync

Configure the git-based notebook synchronization:

- `sync.repository` (string) — Git repository URL for sync (used by `osm sync init` and auto-clone on `osm sync pull`)
- `sync.enabled` (bool, default `false`) — enable git synchronisation
- `sync.auto-pull` (bool, default `false`) — automatically run `git pull --rebase` on program startup when the sync repository is initialized
- `sync.local-path` (string) — local path for sync repository (default: `~/.one-shot-man/sync`)

When the sync directory contains `goals/` or `scripts/` subdirectories, those paths are automatically added to goal and script discovery paths at startup.

### Session cleanup

The `[sessions]` section controls session retention and cleanup thresholds:

```text
[sessions]
maxAgeDays 90
maxCount 100
maxSizeMB 500
autoCleanupEnabled true
cleanupIntervalHours 24
```

Keys:
- `maxAgeDays` (int, default `90`) — sessions older than this are eligible for cleanup
- `maxCount` (int, default `100`) — maximum number of sessions to retain
- `maxSizeMB` (int, default `500`) — maximum total size of all sessions in MB
- `autoCleanupEnabled` (bool, default `true`) — enables automatic session cleanup on startup; when true, the cleanup scheduler runs before each script-executing command
- `cleanupIntervalHours` (int, default `24`) — minimum hours between automatic cleanup runs; the scheduler skips cleanup if less than this interval has elapsed since the last run

All integer values must be non-negative (`cleanupIntervalHours` must be at least 1).
## Environment variables

### Schema-declared overrides

These environment variables override the corresponding config-file key:

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
