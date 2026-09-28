# termmux JS API Reference

The `osm:termmux` module exposes terminal session management to
JavaScript via `require('osm:termmux')`. It provides factories for
creating sessions and managers, plus event constants.

## Module Exports

| Export | Type | Description |
|--------|------|-------------|
| `newCaptureSession(cmd, args?, opts?)` | factory | Create a standalone PTY session |
| `newSessionManager(opts?)` | factory | Create a new SessionManager. Options: `{rows?, cols?, requestBuffer?, outputBuffer?, title?}`. |
| `newBoundedSession(opts)` | factory | Create a CaptureSession + SessionManager in one call. Opts: `{cmd, args?, dir?, rows?, cols?, env?, envReplace?, name?, kind?, remainOnExit?}`. Returns a Promise for `{mgr, session, sid}`. |
| `EXIT_TOGGLE` | `"toggle"` | Passthrough ended by toggle key |
| `EXIT_CHILD_EXIT` | `"childExit"` | Passthrough ended by child process exit |
| `EXIT_CONTEXT` | `"context"` | Passthrough ended by context cancellation |
| `EXIT_ERROR` | `"error"` | Passthrough ended by error |
| `SIDE_OSM` | `"osm"` | Constant for the OSM side identifier |
| `SIDE_AGENT` | `"agent"` | Constant for the agent side identifier |
| `DEFAULT_TOGGLE_KEY` | `29` (0x1D) | Ctrl+] key code |
| `EVENT_EXIT` | `"exit"` | Exit event name |
| `EVENT_RESIZE` | `"resize"` | Resize event name |
| `EVENT_FOCUS` | `"focus"` | Focus event name |
| `EVENT_BELL` | `"bell"` | Bell event name |
| `EVENT_OUTPUT` | `"output"` | Output event name |
| `EVENT_REGISTERED` | `"registered"` | Session registered event name |
| `EVENT_ACTIVATED` | `"activated"` | Session activated event name |
| `EVENT_CLOSED` | `"closed"` | Session closed event name |
| `EVENT_TERMINAL_RESIZE` | `"terminal-resize"` | Terminal resize event name |
| `EVENT_ACTIVITY` | `"activity"` | Session activity detected (new output) |
| `EVENT_SILENCE` | `"silence"` | Session silence detected (no output for a period) |
| `EVENT_TITLE` | `"title"` | Session title changed |
| `EVENT_WORKING_DIRECTORY` | `"cwd"` | Working directory changed |
| `EVENT_CWD` | `"cwd"` | Alias for `EVENT_WORKING_DIRECTORY` |
| `EVENT_CLIPBOARD` | `"clipboard"` | Clipboard event |
| `LAYOUT_TILED` | `"tiled"` | Tiled layout mode |
| `LAYOUT_STACKED` | `"stacked"` | Stacked layout mode |
| `LAYOUT_HORIZONTAL` | `"horizontal"` | Horizontal split layout mode |
| `LAYOUT_VERTICAL` | `"vertical"` | Vertical split layout mode |
| `LAYOUT_MAIN_HORIZONTAL` | `"main-horizontal"` | Main-horizontal layout mode |
| `LAYOUT_MAIN_VERTICAL` | `"main-vertical"` | Main-vertical layout mode |
| `enableMouseForward(config)` | function | Build an async mouse forwarder; call the returned `(msg) => Promise<void>` function for each mouse event |
| `mouseDrag()` | function | Create a stateful mouse-drag object; `.handle({manager,msg})` returns a Promise |
| `handleMouseDrag({ manager, msg })` | function | Handle one mouse event; returns `Promise<{handled, cmd}>` |
| `newControlRouter(opts?)` | function | Create a control router for key dispatch |
| `newPrefixKeyHandler(opts?)` | function | Create a prefix key handler |
| `handlePrefixKey({ manager, key })` | function | Execute a prefix action; returns `Promise<{action, consumed, description, result, listKeys}>` |
| `keyToTermBytes(key, appCursor?, appKeypad?)` | function | Convert a key to terminal byte sequence (`string\|null`) |
| `renderMessageBar(text, row?, cols?)` | function | Render a one-line ANSI message bar |
| `mouseToSGR(event, offsetRow?, offsetCol?)` | function | Convert a mouse event to SGR bytes (`string\|null`) |
| `splitLayout(...)` | function | Split layout operation |

---

## CaptureSession

Created via `newCaptureSession(command, args?, opts?)`.

### Factory Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `command` | `string` | required | Command to execute |
| `args` | `string[]` | `[]` | Command arguments |
| `opts.dir` | `string` | `""` | Working directory |
| `opts.rows` | `number` | `24` | Initial terminal rows |
| `opts.cols` | `number` | `80` | Initial terminal columns |
| `opts.env` | `object` | `{}` | Additional environment variables |
| `opts.name` | `string` | `""` | Session name metadata |
| `opts.kind` | `string` | `""` | Session kind metadata |

### Methods (17 total)

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `start()` | `CaptureSession.Start()` | — | `Promise<void>` | rejects on error |
| `interrupt()` | `CaptureSession.Interrupt()` | — | `Promise<void>` | rejects on error |
| `kill()` | `CaptureSession.Kill()` | — | `Promise<void>` | rejects on error |
| `pause()` | `CaptureSession.Pause()` | — | `Promise<void>` | rejects on error |
| `resume()` | `CaptureSession.Resume()` | — | `Promise<void>` | rejects on error |
| `isPaused()` | `CaptureSession.IsPaused()` | — | `boolean` | silent |
| `resize(rows, cols)` | `CaptureSession.Resize()` | `number, number` | `Promise<void>` | rejects on error |
| `wait()` | `CaptureSession.WaitContext()` | — | `Promise<{code, error?}>` | rejects on cancellation/error |
| `sendEOF()` | `CaptureSession.SendEOF()` | — | `Promise<void>` | rejects on error |
| `close()` | `CaptureSession.Close()` | — | `Promise<void>` | rejects on error |
| `pid()` | `CaptureSession.Pid()` | — | `number` | silent |
| `exitCode()` | `CaptureSession.ExitCode()` | — | `number` | silent |
| `isDone()` | channel select on `Done()` | — | `boolean` | silent |
| `passthrough(opts?)` | `CaptureSession.Passthrough()` | `{toggleKey?}` | `Promise<{reason, error?}>` | async, error field |
| `readAvailable()` | drain `Reader()` channel | — | `string\|null` | non-blocking; null on close |
| `write(data)` | `InteractiveSession.Write()` | `string` | `Promise<void>` | rejects on error |
| `sendKeys(...keys)` | `InteractiveSession.Write()` | `string[]` | `Promise<void>` | rejects on error |

**Removed from CaptureSession in Task 56:** `target()`, `setTarget()`,
`isRunning()` — these now live on the SessionManager `session()` wrapper
(see below).

---

## SessionManager (WrapSessionManager)

Created via `newSessionManager(opts?)` or injected by the host
application through `WrapSessionManager()`. The wrapped object is
typically available as `tuiMux` in pr-split scripts.

### Factory Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `opts.rows` | `number` | `24` | Initial terminal rows |
| `opts.cols` | `number` | `80` | Initial terminal columns |
| `opts.requestBuffer` | `number` | `256` | Request channel buffer size |
| `opts.outputBuffer` | `number` | `256` | Output channel buffer size |

### Lifecycle Methods

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `run()` | `SessionManager.Run()` | — | `undefined` | goroutine; errors ignored |
| `started()` | `SessionManager.Started()` | — | `boolean` | non-blocking channel check |
| `close()` | `SessionManager.Close()` | — | `Promise<void>` | rejects on shutdown error |

### Session Management

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `register(session, opts?)` | `SessionManager.Register()` | `InteractiveSession, {name?,kind?,id?}` | `Promise<number>` (session ID) | rejects on error |
| `unregister(id)` | `SessionManager.Unregister()` | `number` | `Promise<void>` | rejects on error |
| `activate(id)` | `SessionManager.Activate()` | `number` | `Promise<void>` | rejects on error |
| `attach(handle)` | `Register() + Activate()` | `InteractiveSession\|StringIO\|map` | `Promise<number>` (session ID) | rejects on error |
| `detach()` | `Unregister()` | — | `Promise<void>` | resolves when no active session or unregister completes |

### State Queries

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `activeID()` | `SessionManager.ActiveID()` | — | `number` | silent |
| `sessions()` | `SessionManager.Sessions()` | — | `Promise<[{id,target,state,isActive}]>` | rejects on manager error |
| `capture(id, opts?)` | `SessionManager.CaptureScreen()` | `number, {start?,end?,joinWrapped?}` | `{plain,ansi,fullScreen,gen,rows,cols,cursorRow,cursorCol,cursorVisible,mouseTracking,mouseSGR,locked,message,timestamp}\|null` | `null` if session missing or unpublished |
| `lastActivityMs(id?)` | `SessionManager.Snapshot() + time.Since()` | `number?` (session ID) | `number` | `-1` if session/snapshot missing |
| `eventsDropped()` | `SessionManager.EventsDropped()` | — | `number` | silent |

#### capture(id, opts?)

`capture` is the single terminal read surface. It returns all three
representations of one immutable snapshot plus that snapshot's metadata.
Representations are rendered once per call and cached on the snapshot.

Options are validated strictly; a malformed option throws a `TypeError`:

| Option | Type | Meaning |
|--------|------|---------|
| `start` | integral finite `number` | Zero-based inclusive first visible row; negative values clamp to `0`; fractional values throw `TypeError` |
| `end` | integral finite `number` | Zero-based exclusive last visible row; values `<= 0` mean the last visible row; fractional values throw `TypeError` |
| `joinWrapped` | `boolean` | Join wrapped continuation rows with no newline (`plain`/`ansi` only) |

Ranges are zero-based and end-exclusive. `fullScreen` never joins rows: each
selected row keeps its 1-based `CUP` coordinate plus `EL`, followed by the
cursor-position and visibility tail (`cursorRow`/`cursorCol` are 0-based,
`timestamp` is Unix milliseconds). Ranged captures preserve every metadata
field of the source snapshot; only the representation bytes are restricted to
the requested rows. An empty range yields empty strings. Scrollback content is
included whenever the session is scrolled back. At a range's start boundary a
wrapped continuation joins to its out-of-range predecessor only as a
separator decision (no out-of-range text is emitted).

`capture` returns `null` for an unknown session or a session with no published
snapshot — the documented "empty" result, not a throw.

```js
var termmux = require('osm:termmux');

(async function () {
  var runtime = await termmux.newBoundedSession({ cmd: '/bin/sh', args: ['-c', 'printf hello'] });
  var cap = runtime.mgr.capture(runtime.sid);
  output.print(cap.plain);                          // whole screen
  output.print(cap.fullScreen);                     // CUP+EL patch
  var line = runtime.mgr.capture(runtime.sid, {start: 0, end: 1, joinWrapped: true});
  output.print(line.plain);                         // first logical line
})();
```

### I/O and Display

`writeToChild(data)` and the `session()` wrapper operate on the current active
session via `SessionManager.ActiveID()`. They remain available for backwards
compatibility and ad-hoc scripts, but their mutating methods return Promises.
Production pr-split code should prefer pinned SessionID access: `capture(id)` /
`lastActivityMs(id?)` for reads and explicit `await activate(id)` +
`await input(data)` for writes.

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `input(data)` | `SessionManager.Input()` | `string` | `Promise<void>` | rejects on error |
| `resize(rows, cols)` | `SessionManager.Resize()` | `number, number` | `Promise<void>` | rejects on error |
| `writeToChild(data)` | `SessionManager.Input()` | `string` | `Promise<number>` (bytes) | rejects on error; active-session compatibility helper |
| `lastActivityMs(id?)` | `time.Since(snapshot)` | `number?` (session ID) | `number` (ms, -1 if none) | silent |

### Asynchronous query and interaction helpers

The following helpers consult the SessionManager worker and therefore return
Promises:

- `sessions()`
- `newChooser(activeSessionID)` → `Promise<chooser>`
- `chooseTree(options)` → `Promise<{model, selected, visible}>`
- `mouseDrag()` creates a stateful object whose `handle(options)` returns
  `Promise<{handled, cmd}>`
- `handleMouseDrag(options)` → `Promise<{handled, cmd}>`

Await these calls before reading their results. The chooser object's
`show`, `hide`, `visible`, `up`, `down`, `selected`, and `render` methods are
in-memory operations and remain synchronous after the chooser has resolved.


| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `passthrough(opts)` | `SessionManager.Passthrough()` | `{stdin?,stdout?,termFd?,toggleKey?,statusBar?,restoreScreen?,resizeFn?}` | `Promise<{reason, error?}>` | async, error field |
| `switchTo()` | `SessionManager.Passthrough()` | — | `Promise<{reason, error?, childOutput?}>` | async, error field |
| `hasChild()` | `ActiveID() != 0` | — | `boolean` | silent |

### Configuration

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `setStatus(s)` | `statusbar.SetStatus()` | `string` | `undefined` | silent |
| `setToggleKey(k)` | closure mutation | `number` | `undefined` | silent |
| `setStatusEnabled(b)` | closure mutation | `boolean` | `undefined` | silent |
| `setResizeFunc(fn)` | closure mutation | `function` | `undefined` | silent |

### Events

`osm:termmux` wraps each `SessionManager` in a standard DOM-style
`EventTarget`. You may use either the modern `addEventListener` API or
the legacy `on`/`off` aliases; both register listeners on the same
underlying target. Listeners receive `CustomEvent` instances with a
`detail` object containing event-specific data.

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `addEventListener(event, callback)` | `EventTarget.addEventListener()` | `string, function` | `undefined` | throws TypeError if callback is not a function |
| `removeEventListener(event, callback)` | `EventTarget.removeEventListener()` | `string, function` | `undefined` | silent |
| `dispatchEvent(event)` | `EventTarget.dispatchEvent()` | `Event\|CustomEvent` | `boolean` | throws TypeError if the event is missing a `type` |
| `on(event, callback)` | legacy wrapper | `string, function` | `number` (listener ID) | throws TypeError if invalid event or callback |
| `off(id)` | legacy wrapper | `number` | `boolean` | silent |
| `pollEvents()` | compatibility no-op | — | `number` (always `0`) | silent |
| `subscribe(bufSize?)` | `EventBus.Subscribe()` | `number?` | `{id, pollEvents}` | silent |
| `unsubscribe(id)` | `EventBus.Unsubscribe()` | `number` | `boolean` | silent |

Valid legacy event names for `on`: `exit`, `resize`, `focus`, `bell`,
`output`, `registered`, `activated`, `closed`, `terminal-resize`,
`activity`, `silence`, `title`, `cwd`, `clipboard`.
`addEventListener` accepts any event `type`.

#### Event detail payload

| Event type | `detail` fields |
|------------|-----------------|
| `exit` | `{ sessionId: number, pane?: string }` |
| `resize` / `terminal-resize` | `{ sessionId: number, rows: number, cols: number }` |
| `focus` | Not emitted by the current EventBus bridge |
| `bell` | `{ sessionId: number, pane?: string }` |
| `output` | `{ sessionId: number, pane?: string, chunk?: string }` |
| `registered` | `{ sessionId: number }` |
| `activated` | `{ sessionId: number }` |
| `closed` | `{ sessionId: number }` |
| `terminal-resize` | `{ sessionId: number, rows: number, cols: number }` |
| `activity` | `{ sessionId: number }` |
| `silence` | `{ sessionId: number }` |
| `title` | `{ sessionId: number, data?: string }` |
| `cwd` | `{ sessionId: number, data?: string }` |
| `clipboard` | `{ sessionId: number, data?: string }` |

Example:

```js
var termmux = require('osm:termmux');
var bounded = await termmux.newBoundedSession({ cmd: '/bin/sh' });

bounded.mgr.addEventListener('output', function (e) {
  output.print('output from ' + e.detail.sessionId + ': ' + e.detail.chunk);
});
```

### BubbleTea Integration

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `fromModel(model, opts?)` | model wrapper | `any, {altScreen?,toggleKey?,onToggle?}` | `{model, options}` | throws TypeError if no model |
| `activeSide()` | passthrough state | — | `"osm"` or `"agent"` | N/A |
| `isPassthrough()` | passthrough state | — | `boolean` | N/A |

---

## session() Wrapper

Accessed via `tuiMux.session()`. Provides a convenience API
operating on the active session. This wrapper is retained for
backwards compatibility and tests; production pr-split code should
prefer pinned SessionIDs over ActiveID-backed convenience access.

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `isRunning()` | `ActiveID() != 0` | — | `boolean` | silent |
| `isDone()` | cached active-session completion state | — | `boolean` | silent |
| `output()` | `CaptureScreen()` → plain | — | `Promise<string>` | empty string if none |
| `screen()` | `CaptureScreen()` → ANSI | — | `Promise<string>` | empty string if none |
| `target()` | closure read | — | `{id, name, kind}` | silent |
| `setTarget(t)` | closure mutation | `{name?,kind?,id?}` | `undefined` | throws TypeError |
| `write(data)` | `SessionManager.Input()` | `string` | `Promise<void>` | rejects on error |
| `resize(rows, cols)` | `SessionManager.Resize()` | `number, number` | `Promise<void>` | rejects on error |

---

## Error Handling Patterns

Three patterns are used consistently:

1. **throws** — Go errors become JS exceptions via
   `panic(runtime.NewGoError(err))`. Use try/catch in JS.
2. **error field** — Return object includes optional `error` string
   field. Caller should check `result.error`.
3. **silent** — Method returns a sentinel value
   (`null`, `false`, `0`, empty string, `-1`). Used for queries
   where "not found" is a normal condition, not an error.

Mutation operations (write, resize, register, start, kill, etc.) and
worker-backed queries (such as `sessions`, `newChooser`, `chooseTree`, and
mouse-drag handling) return Promises and reject on failure. Lock-free queries
such as `capture`, `lastActivityMs`, `activeID`, and `isDone` use synchronous
sentinel returns. Compound operations (passthrough, wait, switchTo) use
Promises with error fields.
