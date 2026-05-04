# termmux JS API Reference

The `osm:termmux` module exposes terminal session management to
JavaScript via `require('osm:termmux')`. It provides factories for
creating sessions and managers, plus event constants.

## Module Exports

| Export | Type | Description |
|--------|------|-------------|
| `newCaptureSession(cmd, args?, opts?)` | factory | Create a standalone PTY session |
| `newSessionManager(opts?)` | factory | Create a new SessionManager |
| `EXIT_TOGGLE` | `"toggle"` | Passthrough ended by toggle key |
| `EXIT_CHILD_EXIT` | `"childExit"` | Passthrough ended by child process exit |
| `EXIT_CONTEXT` | `"context"` | Passthrough ended by context cancellation |
| `EXIT_ERROR` | `"error"` | Passthrough ended by error |
| `SIDE_OSM` | `"osm"` | Constant for the OSM side identifier |
| `SIDE_CLAUDE` | `"claude"` | Constant for the Claude side identifier |
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
| `start()` | `CaptureSession.Start()` | — | `undefined` | throws |
| `interrupt()` | `CaptureSession.Interrupt()` | — | `undefined` | throws |
| `kill()` | `CaptureSession.Kill()` | — | `undefined` | throws |
| `pause()` | `CaptureSession.Pause()` | — | `undefined` | throws |
| `resume()` | `CaptureSession.Resume()` | — | `undefined` | throws |
| `isPaused()` | `CaptureSession.IsPaused()` | — | `boolean` | silent |
| `resize(rows, cols)` | `CaptureSession.Resize()` | `number, number` | `undefined` | throws |
| `wait()` | `CaptureSession.Wait()` | — | `{code, error?}` | error field |
| `sendEOF()` | `CaptureSession.SendEOF()` | — | `undefined` | throws |
| `close()` | `CaptureSession.Close()` | — | `undefined` | throws |
| `pid()` | `CaptureSession.Pid()` | — | `number` | silent |
| `exitCode()` | `CaptureSession.ExitCode()` | — | `number` | silent |
| `isDone()` | channel select on `Done()` | — | `boolean` | silent |
| `passthrough(opts?)` | `CaptureSession.Passthrough()` | `{toggleKey?}` | `{reason, error?}` | error field |
| `reader()` | `InteractiveSession.Reader()` | — | `string\|null` | blocks; null on close |
| `readAvailable()` | drain `Reader()` channel | — | `string\|null` | non-blocking; null on close |
| `write(data)` | `InteractiveSession.Write()` | `string` | `undefined` | throws |

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
| `close()` | `SessionManager.Close()` | — | `undefined` | silent |

### Session Management

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `register(session, opts?)` | `SessionManager.Register()` | `InteractiveSession, {name?,kind?,id?}` | `number` (session ID) | throws |
| `unregister(id)` | `SessionManager.Unregister()` | `number` | `undefined` | throws |
| `activate(id)` | `SessionManager.Activate()` | `number` | `undefined` | throws |
| `attach(handle)` | `Register() + Activate()` | `InteractiveSession\|StringIO\|map` | `number` (session ID) | throws |
| `detach()` | `Unregister()` | — | `undefined` | silent no-op if none active |

### State Queries

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `activeID()` | `SessionManager.ActiveID()` | — | `number` | silent |
| `sessions()` | `SessionManager.Sessions()` | — | `[{id,target,state,isActive}]` | silent |
| `snapshot(id)` | `SessionManager.Snapshot()` | `number` | `{gen,plainText,...}\|null` | null if not found |
| `lastActivityMs(id?)` | `SessionManager.Snapshot() + time.Since()` | `number?` (session ID) | `number` | `-1` if session/snapshot missing |
| `eventsDropped()` | `SessionManager.EventsDropped()` | — | `number` | silent |

### I/O and Display

The compatibility helpers below (`screenshot()`, `childScreen()`,
`writeToChild(data)`, and `session()`) operate on the current active
session via `SessionManager.ActiveID()`. They remain available for
backwards compatibility and ad-hoc scripts, but production pr-split code
should prefer pinned SessionID access: `snapshot(id)` /
`lastActivityMs(id?)` for reads and explicit `activate(id)` +
`input(data)` for writes.

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `input(data)` | `SessionManager.Input()` | `string` | `undefined` | throws |
| `resize(rows, cols)` | `SessionManager.Resize()` | `number, number` | `undefined` | throws |
| `screenshot()` | `Snapshot()` → plainText | — | `string` | empty if no session; active-session compatibility helper |
| `childScreen()` | `Snapshot()` → ANSI | — | `string` | empty if no session; active-session compatibility helper |
| `writeToChild(data)` | `SessionManager.Input()` | `string` | `number` (bytes) | throws; active-session compatibility helper |
| `lastActivityMs(id?)` | `time.Since(snapshot)` | `number?` (session ID) | `number` (ms, -1 if none) | silent |

### Passthrough

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `passthrough(opts)` | `SessionManager.Passthrough()` | `{stdin?,stdout?,termFd?,toggleKey?,statusBar?,restoreScreen?,resizeFn?}` | `{reason, error?}` | error field |
| `switchTo()` | `SessionManager.Passthrough()` | — | `{reason, error?, childOutput?}` | error field |
| `hasChild()` | `ActiveID() != 0` | — | `boolean` | silent |

### Configuration

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `setStatus(s)` | `statusbar.SetStatus()` | `string` | `undefined` | silent |
| `setToggleKey(k)` | closure mutation | `number` | `undefined` | silent |
| `setStatusEnabled(b)` | closure mutation | `boolean` | `undefined` | silent |
| `setResizeFunc(fn)` | closure mutation | `function` | `undefined` | silent |

### Events

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `on(event, callback)` | listener registration | `string, function` | `number` (listener ID) | throws TypeError if invalid |
| `off(id)` | listener removal | `number` | `boolean` | silent |
| `pollEvents()` | drain event queue | — | `number` (events delivered) | silent |
| `subscribe(bufSize?)` | `EventBus.Subscribe()` | `number?` | `{id, pollEvents}` | silent |
| `unsubscribe(id)` | `EventBus.Unsubscribe()` | `number` | `boolean` | silent |

Valid event names: `exit`, `resize`, `focus`, `bell`, `output`,
`registered`, `activated`, `closed`, `terminal-resize`.

### BubbleTea Integration

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `fromModel(model, opts?)` | model wrapper | `any, {altScreen?,toggleKey?}` | `{model, options}` | throws TypeError if no model |
| `activeSide()` | hardcoded | — | `"osm"` | N/A |

---

## session() Wrapper

Accessed via `tuiMux.session()`. Provides a convenience API
operating on the active session. This wrapper is retained for
backwards compatibility and tests; production pr-split code should
prefer pinned SessionIDs over ActiveID-backed convenience access.

| Method | Go Function | Parameters | Return | Error Handling |
|--------|-------------|------------|--------|----------------|
| `isRunning()` | `ActiveID() != 0` | — | `boolean` | silent |
| `isDone()` | loop `Sessions()` | — | `boolean` | silent |
| `output()` | `Snapshot()` → plainText | — | `string` | empty if none |
| `screen()` | `Snapshot()` → ANSI | — | `string` | empty if none |
| `target()` | closure read | — | `{id, name, kind}` | silent |
| `setTarget(t)` | closure mutation | `{name?,kind?,id?}` | `undefined` | throws TypeError |
| `write(data)` | `SessionManager.Input()` | `string` | `undefined` | throws |
| `resize(rows, cols)` | `SessionManager.Resize()` | `number, number` | `undefined` | throws |

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

Mutation operations (write, resize, register, start, kill, etc.)
throw. Query operations (snapshot, sessions, activeID, etc.) use
silent returns. Compound operations (passthrough, wait, switchTo)
use error fields.
