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
- `autoCleanupEnabled` (bool, default `true`) — *reserved for future use*; parsed and validated but no automatic cleanup scheduler exists yet
- `cleanupIntervalHours` (int, default `24`) — *reserved for future use*; parsed and validated but no automatic cleanup scheduler exists yet

All integer values must be non-negative (`cleanupIntervalHours` must be at least 1).
## Environment variables

These are not “config file keys”, but they materially affect behavior:

- `OSM_CONFIG`: config file path
- `OSM_SESSION`: override auto session-id detection
- `OSM_STORE`: storage backend name (`fs` default, or `memory`)
- `OSM_CLIPBOARD`: override clipboard command (used by JS module `osm:os`)
- `VISUAL` / `EDITOR`: editor used when workflows open an editor
