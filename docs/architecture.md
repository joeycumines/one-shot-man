# Architecture (high level)

This is a conceptual map of how `osm` hangs together.

## Entry point

`cmd/osm/main.go` wires:

- config loading
- the command registry
- goal discovery/registry (see [Goal reference](reference/goal.md))
- built-in commands

## Key components

### Command registry

Top-level CLI commands are implemented in Go under `internal/command/`.

### Scripting engine (Goja)

Several commands (`script`, `prompt-flow`, `code-review`, `goal`) run JavaScript inside an embedded runtime.

The engine:

- registers global helpers (`ctx`, `context`, `output`, `log`, `tui`)
- registers native modules under the `osm:` prefix (see [scripting](scripting.md))

### Sessions + storage

Interactive flows persist state to a session store.

- Default backend: filesystem (`fs`)
- Alternative: `memory` (mainly for tests)

Session IDs are auto-determined (with overrides), and sessions are locked to avoid concurrent corruption.

See:

- [Sessions](session.md)
- [Sophisticated session-id auto-determination](reference/sophisticated-auto-determination-of-session-id.md)

## Visuals

- [docs/visuals/architecture.md](visuals/architecture.md)
- [docs/visuals/workflows.md](visuals/workflows.md)

## Native Modules

Native modules are registered under the `osm:` prefix and provide Go implementations accessible from JavaScript.

