# Scripting

`osm script` embeds a JavaScript runtime (Goja) and exposes a small host API for building terminal workflows.

## Running scripts

### Inline snippets

```sh
osm script -e 'output.print("hello")'
```

### File-based scripts

```sh
osm script ./scripts/my-workflow.js
```

### Interactive scripting terminal

```sh
osm script -i
```

## Globals (available in every script)

### `ctx` (execution context)

A testing-style execution context:

- `ctx.run(name, fn)`
- `ctx.defer(fn)`
- `ctx.log(...)`, `ctx.logf(...)`
- `ctx.error(...)`, `ctx.errorf(...)`
- `ctx.fatal(...)`, `ctx.fatalf(...)`
- `ctx.failed()`
- `ctx.name()`

### `context` (context manager)

This is the core abstraction most workflows use.

Common functions:

- `context.addPath(path)`
- `context.removePath(path)`
- `context.listPaths()`
- `context.getPath(path)`
- `context.refreshPath(path)`
- `context.filterPaths(fn)`
- `context.getFilesByExt(ext)`
- `context.getStats()`
- `context.toTxtar()` / `context.fromTxtar(txtar)`

Example:

```sh
osm script -e 'context.addPath("go.mod"); output.print(context.toTxtar())'
```

### `output` (user-facing output)

- `output.print(text)`
- `output.printf(format, ...)`

### `log` (application logging)

- `log.debug/info/warn/error(...)`
- `log.printf(...)`
- `log.getLogs()`, `log.searchLogs(q)`, `log.clearLogs()`

Note: the logging API is functional, but currently considered undercooked (see docs/todo.md).

### `tui` (modes, prompts, and terminal UI)

The `tui` object lets scripts register modes, commands, completions, and run prompts.

High-level calls:

- `tui.registerMode({...})`
- `tui.switchMode(name)` / `tui.getCurrentMode()`
- `tui.registerCommand({...})`
- `tui.createState(commandName, definitions)`
- `tui.createAdvancedPrompt({...})` + `tui.runPrompt(name)`
- `tui.requestExit()` / `tui.isExitRequested()` / `tui.clearExitRequest()`
- `tui.registerCompleter(name, fn)` + `tui.setCompleter(promptName, completerName)`
- `tui.registerKeyBinding(key, fn)`

Built-ins like `prompt-flow` and `code-review` are embedded into the `osm` binary as scripts that use these APIs.

See also [reference/tui-api.md](./reference/tui-api.md), for more detailed documentation, including motivations and examples.

### `tui.createState` (persisted mode state)

`tui.createState` is the primary way to persist JavaScript workflow state into the session store.

At a high level, you define:

- a unique “command/mode name” (used as a namespace)
- a schema-like set of keys and default values

Important: keys are **Symbols**, not strings.

- You pass a definitions object whose keys are Symbols.
- You call `state.get(symbol)` / `state.set(symbol, value)` using those same Symbols.

Example:

```js
const symText = Symbol("text");
const symCount = Symbol("count");

const state = tui.createState("notes", {
    [symText]: {defaultValue: ""},
    [symCount]: {defaultValue: 0},
});

state.set(symText, "hello");
state.set(symCount, state.get(symCount) + 1);
output.print(state.get(symText));
```

There is also a small shared symbol set for cross-mode shared state:

```js
const shared = require("osm:sharedStateSymbols");

// shared.contextItems is a Symbol that maps to the canonical shared key "contextItems".
const sharedState = tui.createState("__shared__", {
    [shared.contextItems]: {defaultValue: []},
});
```

## Native modules (`require("osm:...")`)

### Overview

TODO: These should get their own (brief) summaries / sections.

`osm` registers a handful of Go-native modules:

- `require("osm:os")`
    - `readFile(path) -> { content, error, message }`
    - `fileExists(path) -> boolean`
    - `openEditor(nameHint, initialContent) -> string`
    - `clipboardCopy(text) -> void` (supports `OSM_CLIPBOARD` override)
    - `getenv(key) -> string`

- `require("osm:exec")`
    - `exec(cmd, ...args) -> { stdout, stderr, code, error, message }`
    - `execv(argvArray) -> { stdout, stderr, code, error, message }`

- `require("osm:text/template")` (Go `text/template` wrapper)
- `require("osm:time")`
- `require("osm:argv")`
- `require("osm:nextIntegerId")`
- `require("osm:ctxutil")` (workflow helpers used by built-ins)
- `require("osm:unicodetext")` — unicode helpers
- `require("osm:sharedStateSymbols")` — shared symbol helpers
- Bubbletea-related modules for building your own TUI components (experimental, introduced for `osm super-document`):
    - `require("osm:bubbletea")` — Bubbletea bindings (Charm JS API, WIP)
    - `require("osm:lipgloss")` — Lipgloss styling helpers
    - `require("osm:bubbles/viewport")`, `require("osm:bubbles/textarea")` — bubble components
    - `require("osm:bubblezone")` — zone/mouse helpers
    - `require("osm:termui/scrollbar")` — scrollbar helper
- `require("osm:tview")` — TUI helpers (proof-of-concept, overlaps with bubbletea)

Note: Better documentation for these modules is pending; see package comments in `./internal/builtin` and the root `README.md` for stability status.

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

## Where to look for examples

- The `scripts/` directory contains small demos and test drivers.
- Built-in workflows are implemented as embedded scripts/templates under `internal/command/`.
