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

### Module reference

All modules use the `osm:` prefix and are loaded via `require("osm:<name>")`.

#### Core utilities

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:os` | OS interactions (files, clipboard, editor, environment) | `readFile(path) → {content, error, message}`, `fileExists(path) → bool`, `openEditor(nameHint, initialContent) → string`, `clipboardCopy(text)` (supports `OSM_CLIPBOARD` override), `getenv(key) → string` |
| `osm:exec` | Process execution | `exec(cmd, ...args) → {stdout, stderr, code, error, message}`, `execv(argv[]) → {stdout, stderr, code, error, message}` |
| `osm:flag` | Go `flag` package wrapper for argument parsing | `newFlagSet(name?) → FlagSet`; FlagSet methods: `.string(name, default, usage)`, `.int(…)`, `.bool(…)`, `.float64(…)`, `.parse(argv) → {error}`, `.get(name)`, `.args()`, `.nArg()`, `.nFlag()`, `.lookup(name)`, `.defaults()`, `.visit(fn)`, `.visitAll(fn)` |
| `osm:time` | Time utilities | `sleep(ms)` — synchronous sleep (milliseconds) |
| `osm:argv` | Command-line string parsing | `parseArgv(cmdline) → string[]`, `formatArgv(argv[]) → string` |

#### Data & text processing

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:text/template` | Go `text/template` wrapper | `new(name) → Template`, `execute(text, data) → string`; Template methods: `.parse(text)`, `.execute(data) → string`, `.funcs(funcMap)`, `.name()`, `.delims(left, right)`, `.option(...opts)` |
| `osm:unicodetext` | Unicode text utilities | `width(s) → number` (monospace display width), `truncate(s, maxWidth, tail?) → string` |
| `osm:fetch` | HTTP client (synchronous + streaming) | `fetch(url, opts?) → Response`, `fetchStream(url, opts?) → StreamResponse`; Options: `method`, `headers`, `body`, `timeout`; Response: `.status`, `.ok`, `.statusText`, `.url`, `.headers`, `.text()`, `.json()`; StreamResponse adds: `.readLine()`, `.readAll()`, `.close()` |
| `osm:grpc` | gRPC client for proto-based services | `dial(target, opts?) → Connection`, `loadDescriptorSet(base64)`, `status` (code constants); Connection: `.invoke(method, request?) → object`, `.close()`, `.target` |

#### Workflow & state

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:ctxutil` | Context building helpers (used by built-ins) | `buildContext(items, options?) → string`, `contextManager` (factory for reusable context management patterns) |
| `osm:nextIntegerId` | Simple ID generator | Default export is a function: `nextId(list) → number` — finds max `.id` in array and returns max+1 |
| `osm:sharedStateSymbols` | Cross-mode shared state symbols | Exports Symbol properties (e.g., `contextItems`) for use with `tui.createState("__shared__", …)` |

#### TUI framework (Charm/BubbleTea stack)

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:bubbletea` | BubbleTea TUI framework bindings | `newModel(config) → Model`, `run(model, opts?)`, `isTTY() → bool`; Commands: `quit()`, `clearScreen()`, `batch(…cmds)`, `sequence(…cmds)`, `tick(ms, id?)`, `setWindowTitle(s)`, `hideCursor()`, `showCursor()`, `enterAltScreen()`, `exitAltScreen()`, `enableBracketedPaste()`, `disableBracketedPaste()`, `enableReportFocus()`, `disableReportFocus()`, `windowSize()`; Metadata: `keys`, `keysByName`, `mouseButtons`, `mouseActions`; Validation: `isValidTextareaInput(key, paste?)`, `isValidLabelInput(key, paste?)` |
| `osm:lipgloss` | Lipgloss terminal styling | `newStyle() → Style` (chainable, immutable); Style methods: `.bold()`, `.italic()`, `.foreground(color)`, `.background(color)`, `.padding(…)`, `.margin(…)`, `.width(n)`, `.height(n)`, `.border(type)`, `.align(pos)`, `.render(…strs) → string`, `.copy()`, `.inherit(other)`; Layout: `joinHorizontal(pos, …strs)`, `joinVertical(pos, …strs)`, `place(w, h, hPos, vPos, str)`, `width(str)`, `height(str)`, `size(str)`; Borders: `normalBorder()`, `roundedBorder()`, `doubleBorder()`, etc.; Constants: `Left`, `Center`, `Right`, `Top`, `Bottom` |
| `osm:bubblezone` | Zone-based mouse hit-testing | `mark(id, content) → string`, `scan(renderedView) → string`, `inBounds(id, mouseMsg) → bool`, `get(id) → {startX, startY, endX, endY, width, height}`, `newPrefix() → string`, `close()` |
| `osm:bubbles/viewport` | Scrollable viewport component | `new(width?, height?) → Viewport`; Viewport: `.setContent(s)`, `.setWidth(n)`, `.setHeight(n)`, `.scrollDown(n)`, `.scrollUp(n)`, `.gotoTop()`, `.gotoBottom()`, `.pageUp()`, `.pageDown()`, `.setYOffset(n)`, `.yOffset()`, `.scrollPercent()`, `.atTop()`, `.atBottom()`, `.totalLineCount()`, `.visibleLineCount()`, `.setStyle(lipglossStyle)`, `.update(msg)`, `.view()` |
| `osm:bubbles/textarea` | Multi-line text input component | `new() → Textarea`; Textarea: `.setValue(s)`, `.value()`, `.setWidth(n)`, `.setHeight(n)`, `.focus()`, `.blur()`, `.focused()`, `.insertString(s)`, `.setCursor(col)`, `.setPosition(row, col)`, `.lineCount()`, `.lineInfo()`, `.cursorVisualLine()`, `.visualLineCount()`, `.performHitTest(x, y)`, `.handleClickAtScreenCoords(x, y)`, `.getScrollSyncInfo()`, `.update(msg)`, `.view()` |
| `osm:termui/scrollbar` | Thin vertical scrollbar | `new(viewportHeight?) → Scrollbar`; Scrollbar: `.setViewportHeight(n)`, `.setContentHeight(n)`, `.setYOffset(n)`, `.setChars(thumb, track)`, `.setThumbForeground(color)`, `.setTrackForeground(color)`, `.view()` |

#### Behavior trees & planning

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:bt` | Behavior tree primitives ([go-behaviortree](https://github.com/joeycumines/go-behaviortree)) | Status: `success`, `failure`, `running`; Nodes: `node(tick, ...children)`, `createLeafNode(fn)`, `createBlockingLeafNode(fn)`; Composites: `sequence(children)`, `fallback(children)` / `selector(children)`, `fork()`; Decorators: `memorize(tick)`, `async(tick)`, `not(tick)`, `interval(ms)`; Execution: `tick(node)`, `newTicker(ms, node, opts?)`, `newManager()`; State: `new Blackboard()`, `exposeBlackboard(bb)` |
| `osm:pabt` | Planning-Augmented Behavior Trees ([go-pabt](https://github.com/joeycumines/go-pabt)) | `newState(blackboard) → State`, `newAction(name, conditions, effects, node) → Action`, `newPlan(state, goals) → Plan`, `newExprCondition(key, expr, value?) → Condition`; State: `.variable(key)`, `.get(key)`, `.set(key, value)`, `.registerAction(name, action)`, `.getAction(name)`, `.setActionGenerator(fn)`; Plan: `.node()`, `.running()` |

### osm:bt (Behavior Trees)

Core behavior tree primitives from [go-behaviortree](https://github.com/joeycumines/go-behaviortree).

**Status constants**: `bt.success`, `bt.failure`, `bt.running`

**Node creation**:
- `bt.node(tick, ...children)` — Create a node from a tick function and optional children
- `bt.createLeafNode(fn)` — Create a leaf node from a JS function (executed on event loop)
- `bt.createBlockingLeafNode(fn)` — Create a blocking leaf node (for already-on-loop contexts)

**Composites** (Go-native, children passed as arrays):
- `bt.sequence(children)` — Tick children in order until one fails
- `bt.fallback(children)` / `bt.selector(children)` — Tick children until one succeeds
- `bt.fork()` — Returns a tick that runs all children in parallel

**Decorators**:
- `bt.memorize(tick)` — Cache non-running status per execution
- `bt.async(tick)` — Wrap tick to run asynchronously
- `bt.not(tick)` — Invert tick result (success ↔ failure)
- `bt.interval(ms)` — Rate-limit tick to at most once per interval

**Execution**:
- `bt.tick(node) → status` — Tick a node and return status string
- `bt.newTicker(ms, node, opts?) → Ticker` — Create periodic ticker (opts: `{stopOnFailure: bool}`)
- `bt.newManager() → Manager` — Create a ticker manager for lifecycle grouping

**State**: `new bt.Blackboard()` — Thread-safe key-value store with `.get(key)`, `.set(key, val)`, `.has(key)`, `.delete(key)`, `.keys()`, `.clear()`, `.snapshot()`

**Architecture constraint**: Composite nodes (sequence, selector, fork) MUST use the Go primitives. JavaScript is used ONLY for leaf behaviors. This prevents deadlocks with the event loop.

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

Terminal UI framework based on [Charm BubbleTea](https://github.com/charmbracelet/bubbletea).

**Program lifecycle**:
- `tea.newModel(config)` — Create an Elm-architecture model (`config`: `{init, update, view}` functions; optional `renderThrottle` config)
- `tea.run(model, opts?)` — Run TUI program (blocks until exit); opts: `{altScreen, mouse, mouseCellMotion, bracketedPaste, reportFocus}`
- `tea.isTTY() → bool` — Check if terminal is a TTY

**Commands** (returned from update as second element of `[model, cmd]`):
- `tea.quit()`, `tea.clearScreen()`
- `tea.batch(...cmds)`, `tea.sequence(...cmds)` — Combine commands
- `tea.tick(ms, id?)` — Timer command (delivers `{type: 'Tick', id, time}` message)
- `tea.setWindowTitle(s)`, `tea.hideCursor()`, `tea.showCursor()`
- `tea.enterAltScreen()`, `tea.exitAltScreen()`
- `tea.enableBracketedPaste()`, `tea.disableBracketedPaste()`
- `tea.enableReportFocus()`, `tea.disableReportFocus()`
- `tea.windowSize()` — Query current terminal size

**Message types** (received in `update(msg, model)`):
- `Key` — `{type, key, runes, alt, ctrl, paste}`
- `Mouse` — `{type, x, y, button, action, alt, ctrl, shift}`
- `WindowSize` — `{type, width, height}`
- `Focus` / `Blur` — `{type}` (requires `reportFocus` option)
- `Tick` — `{type, id, time}` (from `tea.tick()` command)

**Metadata objects**: `tea.keys`, `tea.keysByName`, `tea.mouseButtons`, `tea.mouseActions` — lookup tables for key/mouse definitions.

**Input validation**: `tea.isValidTextareaInput(key, paste?)`, `tea.isValidLabelInput(key, paste?)` — whitelist-based input validators.

### osm:time

Time utilities:

- `time.sleep(ms)` — Synchronous sleep (milliseconds)

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
