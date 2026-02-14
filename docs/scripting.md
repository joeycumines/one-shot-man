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

Scripts can receive arguments after the script filename:

```sh
osm script ./scripts/my-workflow.js --verbose --name hello extra-arg
```

The arguments after the script filename are available in the `args` global (see below).

### Interactive scripting terminal

```sh
osm script -i
```

## Globals (available in every script)

### `args` (script arguments)

A string array containing the command-line arguments passed after the script filename.

```sh
osm script myscript.js --name hello extra
# In JS: args = ["--name", "hello", "extra"]
```

For inline scripts (`-e`), `args` is an empty array. Use `osm:flag` (see Modules below) to parse structured flags from the args array:

```js
var flag = require('osm:flag');
var fs = flag.newFlagSet('mycommand');
fs.string('name', 'default', 'a name');
fs.bool('verbose', false, 'verbose mode');
var result = fs.parse(args);
if (result.error !== null) throw new Error('parse failed: ' + result.error);
output.print('name=' + fs.get('name'));
```

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
- `tui.createPrompt({...})` + `tui.runPrompt(name)`
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

- `require("osm:fetch")` — HTTP client for scripts (synchronous + streaming)
    - `fetch(url, options?) -> Response` — synchronous fetch (reads entire body)
    - `fetchStream(url, options?) -> StreamResponse` — streaming fetch (incremental reading)
    - Options: `method` (default `"GET"`), `headers` (object), `body` (string), `timeout` (seconds, default 30)
    - Response: `.status`, `.ok`, `.statusText`, `.url`, `.headers`, `.text()`, `.json()`
    - StreamResponse: `.status`, `.ok`, `.statusText`, `.url`, `.headers`, `.readLine()`, `.readAll()`, `.close()`

- `require("osm:flag")` — Go `flag` package wrapper for script-level argument parsing
    - `newFlagSet(name?) -> FlagSet` — create a new flag set
    - `FlagSet.string(name, defaultValue, usage) -> FlagSet`
    - `FlagSet.int(name, defaultValue, usage) -> FlagSet`
    - `FlagSet.bool(name, defaultValue, usage) -> FlagSet`
    - `FlagSet.float64(name, defaultValue, usage) -> FlagSet`
    - `FlagSet.parse(argv) -> { error: string|null }`
    - `FlagSet.get(name) -> any`
    - `FlagSet.args() -> string[]` — remaining non-flag arguments
    - `FlagSet.nArg() -> number`, `FlagSet.nFlag() -> number`
    - `FlagSet.lookup(name) -> { name, usage, defValue, value } | null`
    - `FlagSet.defaults() -> string` — formatted usage text
    - `FlagSet.visit(fn)`, `FlagSet.visitAll(fn)` — iterate flags

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
- `require("osm:tview")` — **Deprecated.** TUI helpers (proof-of-concept, superseded by `osm:bubbletea`). Will be removed in a future release. A deprecation warning is emitted to stderr when loaded. See [tview-deprecation.md](archive/notes/tview-deprecation.md).

Note: Better documentation for these modules is pending; see package comments in `./internal/builtin` and the root `README.md` for stability status.

### osm:bt (Behavior Trees)

Core behavior tree primitives:

- `bt.Blackboard` - Thread-safe key-value store for BT nodes
- `bt.newTicker(interval, node)` - Periodic BT execution
- `bt.createLeafNode(fn)` - Create leaf nodes from JavaScript functions
- Status constants: `bt.success`, `bt.failure`, `bt.running`

See: [bt-blackboard-usage.md](reference/bt-blackboard-usage.md)

### osm:fetch (HTTP Client)

Synchronous HTTP client for making API calls from scripts.

#### `fetch(url, options?)` — Synchronous fetch

Reads the entire response body into memory. Best for small responses (JSON APIs, etc.).

```js
var f = require('osm:fetch');

// Simple GET
var resp = f.fetch('https://api.example.com/data');
var data = resp.json();

// POST with headers
var resp = f.fetch('https://api.example.com/submit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key: 'value' }),
    timeout: 10  // seconds
});
if (resp.ok) {
    output.print('Status: ' + resp.status);
    output.print('Body: ' + resp.text());
}
```

**Response properties**: `.status` (number), `.ok` (boolean), `.statusText` (string), `.url` (string after redirects), `.headers` (object, lowercase keys), `.text()` (body as string), `.json()` (parsed JSON).

#### `fetchStream(url, options?)` — Streaming fetch

Returns a StreamResponse with incremental reading. Best for SSE streams, NDJSON, large responses, or LLM API streaming endpoints.

```js
var f = require('osm:fetch');

// Stream an LLM response line by line
var resp = f.fetchStream('https://api.example.com/stream', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ prompt: 'Hello', stream: true })
});

while (true) {
    var line = resp.readLine();
    if (line === null) break;  // EOF
    if (line === '') continue;  // skip empty lines (SSE)
    log.debug('stream line: ' + line);
}
resp.close();  // Important: release HTTP connection
```

**StreamResponse properties**: `.status`, `.ok`, `.statusText`, `.url`, `.headers` (same as fetch), plus:
- `.readLine()` → `string | null` — reads next line (stripped of trailing `\n`/`\r`), returns `null` at EOF
- `.readAll()` → `string` — reads remaining body (useful after partial readLine calls)
- `.close()` → `void` — releases the HTTP connection; **always call this when done**

### osm:pabt (Planning-Augmented Behavior Trees)

PA-BT integration with [go-pabt](https://github.com/joeycumines/go-pabt):

- `pabt.newState(blackboard)` - PA-BT state wrapping blackboard
- `pabt.newAction(name, conditions, effects, node)` - Define planning actions
- `pabt.newPlan(state, goalConditions)` - Create goal-directed plans
- `pabt.newExprCondition(key, expr)` - Fast Go-native conditions

**Architecture principle**: Application types (shapes, sprites, simulation) are defined in JavaScript only. The Go layer provides PA-BT primitives; JavaScript provides domain logic.

See: [planning-and-acting-using-behavior-trees.md](reference/planning-and-acting-using-behavior-trees.md)

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

## Module Resolution

`osm` uses the [goja_nodejs](https://github.com/nicois/goja_nodejs) CommonJS module system.
The global `require()` function resolves modules in the following order:

### 1. Native Go modules (`osm:` prefix)

Modules with the `osm:` prefix are Go-backed and registered at engine startup.
They are always available, regardless of working directory or script location.

```js
var exec = require('osm:exec');
var flag = require('osm:flag');
```

See the [Native modules](#native-modules-requireosm) section above for the full list.

### 2. Relative paths (`./` or `../`)

Relative paths are resolved from the **calling module's directory**.
For the top-level script, this is the directory containing the script file.

```js
// In /project/scripts/main.js:
var helpers = require('./lib/helpers');   // loads /project/scripts/lib/helpers.js
var common  = require('../shared/util');  // loads /project/shared/util.js
```

Resolution tries the following, in order:

1. Exact path (e.g. `./lib/helpers.js`)
2. Path with `.js` appended (e.g. `./lib/helpers` → `./lib/helpers.js`)
3. Path with `.json` appended (loads and parses as JSON)
4. Directory with `index.js` (e.g. `./mylib` → `./mylib/index.js`)

**How it works internally**: File-based scripts are compiled with their absolute file path
as the source name. When `require()` is called, the runtime walks the JavaScript call stack
to find the caller's source file name, then resolves relative paths from that directory.
This means relative `require()` calls work correctly from any depth of the module tree.

### 3. Absolute paths

Absolute paths bypass all search logic and load the file directly:

```js
var config = require('/etc/osm/shared-config.js');
```

### 4. Bare module names (configurable search paths)

A bare name—one without `./`, `../`, `/`, or the `osm:` prefix—is searched
through configured **module search paths**, similar to `NODE_PATH` in Node.js.

```js
var mylib = require('mylib');  // searches configured paths for mylib.js or mylib/index.js
```

Configure search paths via the `script.module-paths` setting in `~/.config/osm/config`:

```
script.module-paths=/home/user/osm-libs,/opt/shared-scripts
```

Or via environment variable:

```sh
export OSM_SCRIPT_MODULE_PATHS=/home/user/osm-libs:/opt/shared-scripts
```

Both comma-separated and OS path-separator-delimited values are supported.
Each directory is searched for the module name using the same resolution rules
(exact, `.js`, `.json`, `index.js`).

### Module features

**Caching**: Modules are cached by resolved path. Requiring the same file twice
returns the same `exports` object—the module code only executes once.

**JSON loading**: `require('./data.json')` parses the file as JSON and returns it directly.

**Shebang support**: Scripts starting with `#!` (e.g. `#!/usr/bin/env osm script`)
have the shebang line automatically commented out, preserving line numbers in
error messages while keeping the source valid JavaScript. This applies to both
top-level scripts and modules loaded via `require()`.

**Circular dependencies**: Partially supported (as in Node.js CommonJS). If module A
requires module B, and B requires A, B will see A's `exports` as they were at the
point where execution left A. Avoid relying on this.
