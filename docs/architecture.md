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

### osm:bt (Behavior Trees)

Core behavior tree primitives:

- `bt.Blackboard` - Thread-safe key-value store for BT nodes
- `bt.newTicker(interval, node)` - Periodic BT execution
- `bt.createLeafNode(fn)` - Create leaf nodes from JavaScript functions
- Status constants: `bt.success`, `bt.failure`, `bt.running`

See: [bt-blackboard-usage.md](reference/bt-blackboard-usage.md)

### osm:pabt (Planning-Augmented Behavior Trees)

PA-BT integration with [go-pabt](https://github.com/joeycumines/go-pabt):

- `pabt.newState(blackboard)` - PA-BT state wrapping blackboard
- `pabt.newAction(name, conditions, effects, node)` - Define planning actions
- `pabt.newPlan(state, goalConditions)` - Create goal-directed plans
- `pabt.newExprCondition(key, expr)` - Fast Go-native conditions

**Architecture principle**: Application types (shapes, sprites, simulation) are defined in JavaScript only. The Go layer provides PA-BT primitives; JavaScript provides domain logic.

See: [pabt.md](reference/pabt.md)

### osm:bubbletea (TUI Framework)

Terminal UI framework integration:

- `tea.newModel(config)` - Create Elm-architecture model
- `tea.run(model, opts)` - Run TUI application
- Message types: `Tick`, `Key`, `Resize`

### osm:time

Time utilities:

- `time.sleep(ms)` - Synchronous sleep
- `time.now()` - Current timestamp
