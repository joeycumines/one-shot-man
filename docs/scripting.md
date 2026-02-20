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

- `log.debug(msg)` — log at DEBUG level (only recorded when log.level ≤ debug)
- `log.info(msg)` — log at INFO level (default threshold)
- `log.warn(msg)` — log at WARN level
- `log.error(msg)` — log at ERROR level
- `log.printf(format, ...)` — log a formatted message at INFO level
- `log.getLogs()` — return all in-memory log entries (array of `{time, level, message, attrs}`)
- `log.searchLogs(query)` — search log entries by message or attribute content (case-insensitive)
- `log.clearLogs()` — clear all in-memory log entries

Logs are written to:
1. **In-memory ring buffer** (configurable size via `log.buffer-size`, default 1000) — accessible via `getLogs()`/`searchLogs()`
2. **JSON file** (when `log.file` is configured) — with size-based rotation (`log.max-size-mb`, `log.max-files`)

View logs externally with `osm log` (last N lines) or `osm log follow` / `osm log tail` (continuous tail).

### `tui` (modes, prompts, and terminal UI)

The `tui` object lets scripts register modes, commands, completions, and run prompts.

High-level calls:

- `tui.registerMode({...})` — supports `multiline: true` for Alt+Enter newline insertion
- `tui.switchMode(name)` / `tui.getCurrentMode()`
- `tui.registerCommand({...})`
- `tui.createState(commandName, definitions)`
- `tui.createPrompt({...})` + `tui.runPrompt(name)` — `createPrompt` supports `multiline: true` for Alt+Enter newline insertion
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
| `osm:os` | OS interactions (files, clipboard, editor, environment) | `readFile(path) → {content, error, message}`, `fileExists(path) → bool`, `writeFile(path, content, options?) → undefined` (options: `{mode?: number, createDirs?: boolean}`), `appendFile(path, content, options?) → undefined` (same options), `openEditor(nameHint, initialContent) → string`, `clipboardCopy(text)` (supports `OSM_CLIPBOARD` override), `getenv(key) → string` |
| `osm:exec` | Process execution | `exec(cmd, ...args) → {stdout, stderr, code, error, message}`, `execv(argv[]) → {stdout, stderr, code, error, message}` |
| `osm:flag` | Go `flag` package wrapper for argument parsing | `newFlagSet(name?) → FlagSet`; FlagSet methods: `.string(name, default, usage)`, `.int(…)`, `.bool(…)`, `.float64(…)`, `.parse(argv) → {error}`, `.get(name)`, `.args()`, `.nArg()`, `.nFlag()`, `.lookup(name)`, `.defaults()`, `.visit(fn)`, `.visitAll(fn)` |
| `osm:path` | Go `path/filepath` wrapper for path manipulation | `join(...args) → string`, `dir(path) → string`, `base(path) → string`, `ext(path) → string`, `abs(path) → {result, error}`, `rel(basepath, targpath) → {result, error}`, `clean(path) → string`, `isAbs(path) → bool`, `match(pattern, name) → {matched, error}`, `glob(pattern) → {matches, error}`, `separator`, `listSeparator` |
| `osm:regexp` | Go RE2 regular expressions | `match(pattern, str) → bool`, `find(pattern, str) → string\|null`, `findAll(pattern, str, n?) → string[]`, `findSubmatch(pattern, str) → string[]\|null`, `findAllSubmatch(pattern, str, n?) → string[][]`, `replace(pattern, str, repl) → string`, `replaceAll(pattern, str, repl) → string`, `split(pattern, str, n?) → string[]`, `compile(pattern) → RegexpObject` (with same methods bound). [Reference →](reference/regexp.md) |
| `osm:crypto` | Cryptographic hash functions | `sha256(input) → string`, `sha1(input) → string`, `md5(input) → string`, `hmacSHA256(key, message) → string`, `hmacSHA1(key, message) → string` — all return hex-encoded lowercase strings; input accepts strings or byte arrays |
| `osm:encoding` | Base64 and hex encoding/decoding | `base64Encode(input) → string`, `base64Decode(encoded) → string`, `base64URLEncode(input) → string`, `base64URLDecode(encoded) → string`, `hexEncode(input) → string`, `hexDecode(encoded) → string` — decode errors throw JS errors; input accepts strings or byte arrays. [Reference →](reference/encoding.md) |
| `osm:json` | JSON utilities | `parse(str) → any`, `stringify(value, indent?) → string`, `query(obj, path) → any` (dot-notation, `[n]`, `[*]` wildcard), `mergePatch(target, patch) → any` (RFC 7386), `diff(a, b) → [{op, path, value?, oldValue?}]` (JSON Pointer paths), `flatten(obj, sep?) → object`, `unflatten(obj, sep?) → object` |
| `osm:time` | Time utilities | `sleep(ms)` — synchronous sleep (milliseconds). [Reference →](reference/time.md) |
| `osm:argv` | Command-line string parsing | `parseArgv(cmdline) → string[]`, `formatArgv(argv[]) → string` |

#### Data & text processing

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:text/template` | Go `text/template` wrapper | `new(name) → Template`, `execute(text, data) → string`; Template methods: `.parse(text)`, `.execute(data) → string`, `.funcs(funcMap)`, `.name()`, `.delims(left, right)`, `.option(...opts)` |
| `osm:unicodetext` | Unicode text utilities | `width(s) → number` (monospace display width), `truncate(s, maxWidth, tail?) → string`. [Reference →](reference/unicodetext.md) |
| `osm:fetch` | Promise-based HTTP client (browser Fetch API) | `fetch(url, opts?) → Promise<Response>`; Options: `method`, `headers`, `body`, `timeout`, `signal`; Response: `.status`, `.ok`, `.statusText`, `.url`, `.headers` (Headers object with `.get()`, `.has()`, `.entries()`, `.keys()`, `.values()`, `.forEach()`), `.text() → Promise<string>`, `.json() → Promise<any>` |
| `osm:grpc` | Promise-based gRPC client and server (via [goja-grpc](https://github.com/joeycumines/goja-grpc)) | `createClient(service) → Client` (methods return `Promise`), `createServer(service, handler) → Server`, `dial(target, opts?) → Channel`, `status` (code constants: `OK`, `CANCELLED`, `NOT_FOUND`, etc.), `metadata`, `enableReflection(server)`, `createReflectionClient(channel)` |
| `osm:protobuf` | Protocol Buffers for goja (via [goja-protobuf](https://github.com/joeycumines/goja-protobuf)) | `loadDescriptorSet(bytes)` — loads binary `FileDescriptorSet` for use with `osm:grpc` |

#### Workflow & state

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:ctxutil` | Context building helpers (used by built-ins) | `buildContext(items, options?) → string`, `contextManager` (factory for reusable context management patterns) |
| `osm:nextIntegerID` | Simple ID generator | Default export is a function: `nextId(list) → number` — finds max `.id` in array and returns max+1. _(Deprecated alias: `osm:nextIntegerId`)_ [Reference →](reference/nextintegerid.md) |
| `osm:sharedStateSymbols` | Cross-mode shared state symbols | Exports Symbol properties (e.g., `contextItems`) for use with `tui.createState("__shared__", …)` |

#### TUI framework (Charm/BubbleTea stack)

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:bubbletea` | BubbleTea TUI framework bindings | `newModel(config) → Model`, `run(model, opts?)`, `isTTY() → bool`; Commands: `quit()`, `clearScreen()`, `batch(…cmds)`, `sequence(…cmds)`, `tick(ms, id?)`, `setWindowTitle(s)`, `hideCursor()`, `showCursor()`, `enterAltScreen()`, `exitAltScreen()`, `enableBracketedPaste()`, `disableBracketedPaste()`, `enableReportFocus()`, `disableReportFocus()`, `windowSize()`; Metadata: `keys`, `keysByName`, `mouseButtons`, `mouseActions`; Validation: `isValidTextareaInput(key, paste?)`, `isValidLabelInput(key, paste?)` |
| `osm:lipgloss` | Lipgloss terminal styling | `newStyle() → Style` (chainable, immutable); Style methods: `.bold()`, `.italic()`, `.foreground(color)`, `.background(color)`, `.padding(…)`, `.margin(…)`, `.width(n)`, `.height(n)`, `.border(type)`, `.align(pos)`, `.render(…strs) → string`, `.copy()`, `.inherit(other)`; Layout: `joinHorizontal(pos, …strs)`, `joinVertical(pos, …strs)`, `place(w, h, hPos, vPos, str)`, `width(str)`, `height(str)`, `size(str)`; Borders: `normalBorder()`, `roundedBorder()`, `doubleBorder()`, etc.; Constants: `Left`, `Center`, `Right`, `Top`, `Bottom` |
| `osm:bubblezone` | Zone-based mouse hit-testing | `mark(id, content) → string`, `scan(renderedView) → string`, `inBounds(id, mouseMsg) → bool`, `get(id) → {startX, startY, endX, endY, width, height}`, `newPrefix() → string`, `close()` |
| `osm:bubbles/viewport` | Scrollable viewport component | `new(width?, height?) → Viewport`; Viewport: `.setContent(s)`, `.setWidth(n)`, `.setHeight(n)`, `.scrollDown(n)`, `.scrollUp(n)`, `.gotoTop()`, `.gotoBottom()`, `.pageUp()`, `.pageDown()`, `.setYOffset(n)`, `.yOffset()`, `.scrollPercent()`, `.atTop()`, `.atBottom()`, `.totalLineCount()`, `.visibleLineCount()`, `.setStyle(lipglossStyle)`, `.update(msg)`, `.view()` |
| `osm:bubbles/textarea` | Multi-line text input component | `new() → Textarea`; Textarea: `.setValue(s)`, `.value()`, `.setWidth(n)`, `.setHeight(n)`, `.focus()`, `.blur()`, `.focused()`, `.insertString(s)`, `.setCursor(col)`, `.setPosition(row, col)`, `.lineCount()`, `.lineInfo()`, `.cursorVisualLine()`, `.visualLineCount()`, `.performHitTest(x, y)`, `.handleClickAtScreenCoords(x, y)`, `.getScrollSyncInfo()`, `.update(msg)`, `.view()` |
| `osm:termui/scrollbar` | Thin vertical scrollbar | `new(viewportHeight?) → Scrollbar`; Scrollbar: `.setViewportHeight(n)`, `.setContentHeight(n)`, `.setYOffset(n)`, `.viewportHeight()`, `.contentHeight()`, `.yOffset()`, `.setChars(thumb, track)`, `.setThumbForeground(color)`, `.setThumbBackground(color)`, `.setTrackForeground(color)`, `.setTrackBackground(color)`, `.view()`. [Reference →](reference/scrollbar.md) |

#### Behavior trees & planning

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:bt` | Behavior tree primitives ([go-behaviortree](https://github.com/joeycumines/go-behaviortree)) | Status: `success`, `failure`, `running`; Nodes: `node(tick, ...children)`, `createLeafNode(fn)`, `createBlockingLeafNode(fn)`; Composites: `sequence(children)`, `fallback(children)` / `selector(children)`, `fork()`; Decorators: `memorize(tick)`, `async(tick)`, `not(tick)`, `interval(ms)`; Execution: `tick(node)`, `newTicker(ms, node, opts?)`, `newManager()`; State: `new Blackboard()`, `exposeBlackboard(bb)` |
| `osm:pabt` | Planning-Augmented Behavior Trees ([go-pabt](https://github.com/joeycumines/go-pabt)) | `newState(blackboard) → State`, `newAction(name, conditions, effects, node) → Action`, `newPlan(state, goals) → Plan`, `newExprCondition(key, expr, value?) → Condition`; State: `.variable(key)`, `.get(key)`, `.set(key, value)`, `.registerAction(name, action)`, `.getAction(name)`, `.setActionGenerator(fn)`; Plan: `.node()`, `.running()` |

#### Claude-mux orchestration

| Module | Description | Key exports |
|--------|-------------|-------------|
| `osm:claudemux` | Multi-instance Claude Code orchestration building blocks | Parser: `newParser()`, `eventTypeName(type)`, `KEY_*` constants; Guard: `newGuard(config)`, `defaultGuardConfig()`, `guardActionName(action)`, `GUARD_ACTION_*`/`PERMISSION_POLICY_*` constants; MCPGuard: `newMCPGuard(config)`, `defaultMCPGuardConfig()`; Supervisor: `newSupervisor(config)`, `defaultSupervisorConfig()`, error/action/state constants; Pool: `newPool(config)`, `defaultPoolConfig()`; Panel: `newPanel(config)`, `defaultPanelConfig()`; Session: `createSession(id, config?)`, `defaultManagedSessionConfig()`, `managedSessionStateName(state)`, `SESSION_*` constants; Safety: `newSafetyValidator(config)`, `defaultSafetyConfig()`, `newCompositeValidator()`, intent/scope/risk/policy constants; Choice: `newChoiceResolver(config)`, `defaultChoiceConfig()`; Instance: `newInstanceRegistry(baseDir)` |

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

Promise-based HTTP client following the browser Fetch API. HTTP requests run asynchronously in a goroutine and resolve on the event loop.

#### `fetch(url, options?)` → `Promise<Response>`

Returns a Promise that resolves with a Response object once the HTTP request completes.

```js
const f = require('osm:fetch');

// Simple GET
const resp = await f.fetch('https://api.example.com/data');
const data = await resp.json();

// POST with headers
const resp = await f.fetch('https://api.example.com/submit', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ key: 'value' }),
    timeout: 10  // seconds
});
if (resp.ok) {
    output.print('Status: ' + resp.status);
    output.print('Body: ' + await resp.text());
}
```

**Options**: `method` (string, default `"GET"`), `headers` (object), `body` (string), `timeout` (number in seconds, default `30`), `signal` (AbortSignal for cancelling the request).

**Cancellation with AbortController**:
```js
const ac = new AbortController();
setTimeout(() => ac.abort(), 5000); // abort after 5s
const resp = await f.fetch(url, { signal: ac.signal });

// Or use AbortSignal.timeout() for a simpler timeout:
const resp = await f.fetch(url, { signal: AbortSignal.timeout(5000) });
```

**Response properties**: `.status` (number), `.ok` (boolean, true if 200-299), `.statusText` (string), `.url` (string, final URL after redirects).

**Response.headers** (Headers object): `.get(name) → string|null`, `.has(name) → boolean`, `.entries() → Array<[name, value]>`, `.keys() → Array<string>`, `.values() → Array<string>`, `.forEach(callback)`.

**Response methods**: `.text() → Promise<string>` (body as string), `.json() → Promise<any>` (parsed JSON, rejects on parse error).

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

### osm:claudemux (Claude-Mux Orchestration)

Building blocks for multi-instance Claude Code management. See [Claude-Mux Architecture](architecture-claude-mux.md) for the full design.

**Parser** — PTY output classification:
- `cm.newParser()` — Create a parser with built-in Claude Code output patterns
- `parser.parse(line)` — Classify a line → `{type, line, fields, pattern}`
- `parser.addPattern(name, regex, eventType, extractFn?)` — Register custom patterns
- `parser.patterns()` — List registered patterns
- `cm.eventTypeName(type)` — Human-readable event type name
- Constants: `cm.EVENT_TEXT`, `cm.EVENT_RATE_LIMIT`, `cm.EVENT_PERMISSION`, `cm.EVENT_MODEL_SELECT`, `cm.EVENT_SSO_LOGIN`, `cm.EVENT_COMPLETION`, `cm.EVENT_TOOL_USE`, `cm.EVENT_ERROR`, `cm.EVENT_THINKING`
- Key constants: `cm.KEY_UP`, `cm.KEY_DOWN`, `cm.KEY_ENTER`, `cm.KEY_Y`, `cm.KEY_N`

**Guard** — PTY output monitors:
- `cm.newGuard(config)` — Create a guard with rate-limit, permission, crash, and timeout monitors
- `guard.processEvent(event, now)` — Evaluate an output event → `{action, reason, details}` or null
- `guard.processCrash(exitCode, now)` — Report a crash → guard event
- `guard.checkTimeout(now)` — Check for output timeout → guard event or null
- `cm.defaultGuardConfig()` — Production defaults
- `cm.guardActionName(action)` — Human-readable action name
- Constants: `cm.GUARD_ACTION_NONE`, `cm.GUARD_ACTION_PAUSE`, `cm.GUARD_ACTION_REJECT`, `cm.GUARD_ACTION_RESTART`, `cm.GUARD_ACTION_ESCALATE`, `cm.GUARD_ACTION_TIMEOUT`; `cm.PERMISSION_POLICY_DENY`, `cm.PERMISSION_POLICY_ALLOW`

**MCPGuard** — MCP call monitors:
- `cm.newMCPGuard(config)` — Create an MCP guard with frequency, repeat, allowlist, and timeout monitors
- `mcpGuard.processToolCall(call)` — Evaluate a tool call → `{action, reason, details}` or null
- `mcpGuard.checkNoCallTimeout(now)` — Check for no-call timeout
- `cm.defaultMCPGuardConfig()` — Production defaults

**Supervisor** — Error recovery:
- `cm.newSupervisor(config)` — Create a supervisor state machine
- `supervisor.start()` — Transition to Running
- `supervisor.handleError(msg, errorClass)` — Get recovery decision
- `supervisor.shutdown()` — Initiate graceful drain
- `cm.defaultSupervisorConfig()` — Production defaults
- Error class constants: `cm.ERROR_CLASS_*`; Recovery action constants: `cm.RECOVERY_*`

**Pool** — Concurrent instance management:
- `cm.newPool(config)` — Create a pool with acquire/release dispatch
- `pool.start()`, `pool.addWorker(worker)`, `pool.acquire()`, `pool.tryAcquire()`, `pool.release(worker, err, now)`, `pool.drain()`, `pool.close()`, `pool.stats()`
- `cm.defaultPoolConfig()` — Default max size 4

**Panel** — TUI multi-instance display:
- `cm.newPanel(config)` — Create a panel with Alt+1..9 switching
- `panel.start()`, `panel.addPane(id, title)`, `panel.routeInput(key)`, `panel.appendOutput(id, text)`, `panel.updateHealth(id, health)`, `panel.statusBar()`, `panel.getVisibleLines(id, height)`, `panel.snapshot()`, `panel.close()`
- `cm.defaultPanelConfig()` — Default 9 panes, 10000 scrollback

**ManagedSession** — Unified monitoring pipeline:
- `cm.createSession(id, config?)` — Create a session composing Parser+Guard+MCPGuard+Supervisor
- `session.processLine(line, now)` → `{event, guardEvent, action}`
- `session.processCrash(exitCode, now)` → `{guardEvent, recoveryDecision}`
- `session.processToolCall(call)` → tool call result
- `session.shutdown()`, `session.close()`
- `session.onEvent(fn)`, `session.onGuardAction(fn)`, `session.onRecoveryDecision(fn)` — Callbacks
- `cm.defaultManagedSessionConfig()`, `cm.managedSessionStateName(state)`
- Constants: `cm.SESSION_IDLE`, `cm.SESSION_ACTIVE`, `cm.SESSION_PAUSED`, `cm.SESSION_FAILED`, `cm.SESSION_CLOSED`

**Safety** — Intent/scope/risk classification:
- `cm.newSafetyValidator(config)` — Create a rule-based safety validator
- `validator.validate(toolName, args)` → `{intent, scope, risk, riskLevel, action, reason}`
- `cm.defaultSafetyConfig()`, `cm.newCompositeValidator()`
- Constants: `cm.INTENT_*`, `cm.SCOPE_*`, `cm.RISK_*`, `cm.POLICY_*`

**Choice** — Multi-criteria decision analysis:
- `cm.newChoiceResolver(config)` — Create a resolver with configurable criteria
- `resolver.analyze(candidates, criteria?)` → `{recommendedID, rankings, justification, needsConfirm}`
- `cm.defaultChoiceConfig()` — Default 4 criteria (complexity/risk/maintainability/performance)

**Instance** — Session isolation:
- `cm.newInstanceRegistry(baseDir)` — Create a registry for isolated instance state
- `registry.create(id)`, `registry.get(id)`, `registry.close(id)`, `registry.closeAll()`, `registry.list()`, `registry.len()`, `registry.baseDir()`

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

Configure search paths via the `script.module-paths` setting in `~/.osm/config`:

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

### Module security (path-traversal protection)

The `osm` scripting engine enforces two layers of protection against path-traversal
attacks within bare module name resolution (global folder lookups only):

**1. Path resolver check (pre-read)**

Before the file is read, the path resolver verifies that the candidate file path stays
within the configured module search directories. If a bare module name like
`require('../../etc/passwd')` somehow resolves to a path outside the allowed directories,
the resolver returns a sentinel path (`.osm-blocked-traversal`) that will never exist,
causing `require()` to fail with a "module not found" error.

> Note: This check applies _only_ to bare module names looked up through global search
> folders. Relative requires (`./`, `../`) from within a script's own directory are
> **not** subject to this check, because the calling script's directory is trusted.

**2. Symlink escape check (post-read)**

Symlinks can bypass the pre-read check: a symlink inside an allowed directory might
point to a target outside it. After goja resolves the file path and just before reading
the file, the hardened source loader resolves the final path through `filepath.EvalSymlinks`
and verifies the real path is still within an allowed directory. If the real path has
escaped, the read is blocked with an explicit error:

```
path traversal blocked: "../../etc/passwd" resolves via symlink outside allowed module paths [/home/user/osm-libs]
```

**3. Path validation at startup**

All configured `script.module-paths` entries are validated at engine startup via
`os.Stat` + `filepath.EvalSymlinks`. Invalid entries (nonexistent, not a directory,
permission denied, unresolvable symlinks) are skipped with a warning rather than
failing hard. Only successfully validated paths are registered as global search folders.

**Scope**: Path-traversal protection applies to the global-folder (bare name) resolution
path only. Absolute `require('/path/to/script.js')` calls bypass all containment checks.
Users with `script.module-paths` configured should therefore not grant write access to
those directories to untrusted code.

### ES module syntax (ESM) limitations

The `osm` scripting runtime uses [Goja](https://github.com/dop251/goja), which
implements a CommonJS module system via [goja_nodejs](https://github.com/nicois/goja_nodejs).

**ESM is not supported.** The following ES module syntax will cause a parse error:

```js
// NOT SUPPORTED — will throw a SyntaxError at load time
import { exec } from 'osm:exec';
export function helper() { return 42; }
export default { key: 'value' };
```

Use CommonJS `require()` and `module.exports` instead:

```js
// SUPPORTED — CommonJS require/exports
var exec = require('osm:exec');
module.exports = { helper: function() { return 42; } };
module.exports = { key: 'value' };
```

**Note on `import()` expressions**: Dynamic `import()` expressions are also not
supported. Use `require()` for all module loading, including conditional or lazy loads.

**Why not ESM?** Goja implements the ES5.1 spec with many ES2015+ extensions, but its
module system predates WHATWG ESM and uses the CommonJS `require`/`module.exports`
conventions. ESM support (static `import`/`export` declarations) is tracked upstream
in goja but is not yet implemented. If ESM support is added to goja in a future
release, `osm` will evaluate adopting it.
