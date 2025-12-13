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

For a deep dive into how session ids are determined, see:

- [Sophisticated session-id auto-determination](reference/sophisticated-auto-determination-of-session-id.md)

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

## Storage backends

Most commands that persist state accept:

- `-store fs` (default)
- `-store memory` (non-persistent; useful for tests)

You can also set `OSM_STORE=memory`.
