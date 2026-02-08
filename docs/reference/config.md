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
  - **Reserved for future use.** Parsed and validated but no automatic cleanup scheduler exists yet.
  - Accepts: `true`, `false`, `1`, `0`, `yes`, `no`, `on`, `off` (case-insensitive).
- `cleanupIntervalHours` (int, default `24`)
  - **Reserved for future use.** Parsed and validated but no automatic cleanup scheduler exists yet.
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

## Global keys you may see

`osm init` writes a few global keys such as `verbose` and `color`.

Today these are largely placeholders: they load successfully, but most user-visible behavior is driven by:
- discovery keys (script/goal)
- prompt colors (`prompt.color.*`)

## Environment variables (behavioral)

These aren’t config-file keys, but they change behavior:

- `OSM_CONFIG`: config file path override
- `OSM_SESSION`: override session ID selection
- `OSM_STORE`: set storage backend name (`fs` or `memory`)
- `OSM_CLIPBOARD`: override clipboard-copy command (used by `osm:os`)
- `VISUAL` / `EDITOR`: editor command used when a workflow opens an editor
