# AGENTS.md

This file steers agent behavior. For build commands, architecture, and API details, use `gmake help` and the `docs/` directory.

## Code Quality Standards

ALL checks must pass on ALL platforms (ubuntu-latest, windows-latest, macos-latest). Never accept failing tests—fix them, never paper over them. Cross-platform testing targets (`make-all-in-container`, `make-all-run-windows`) must also pass.

### Linting

Never add entries to `.deadcodeignore`. This project is a CLI, not a library—all implementations MUST be wired up in `main.go` or their respective registries. If `deadcode` fails, wire up the code or delete it.

### Error Handling

- Consistent error handling with proper exit codes.
- No swallowing errors or "best effort" approaches.
- All commands must handle platform differences (Unix vs Windows).

### Internal API Discipline

When modifying internal code (anything under `internal/`, unreleased features, experimental code):

- **No Shims**: Do NOT retain shims, wrappers, or backwards-compatibility stubs "just in case".
- **One Implementation**: Do NOT accumulate variants (e.g., `Foo()` and `FooV2()`). Choose ONE and migrate all call sites.
- **Update Everything**: Update ALL test code and ALL call sites—unit tests, integration tests, all packages, any scripts that depend on the behavior.
- **Delete Boldly**: Remove deprecated functions entirely. Accumulated dead code is worse than temporary re-creation.

### Testing

- Tests run with race detection.
- Tests must be isolated. Tests **must not mutate host system state** or depend on session/configuration state outside the test. Zero-tolerance rule.
  - Use `t.TempDir()` for all file operations.
  - Use `t.Setenv()` for environment variable isolation.
  - Mock external services and config sources.
  - Never assume a specific home directory or config location.
- NEVER use build tags to segment tests. Use `testing.Short()` skip guards or `TestMain` with custom `flag` parsing.
- Slow tests (JS runtimes, TUI, subprocesses, >2s) must call `skipSlow(t)` or guard with `if testing.Short() { t.Skip(...) }`.

### No "AI Slop"

Code must be intentional, consistent, purposeful, tested, and validated. Remove contradictory logic, vestigial comments, and unused structures. AI integrations and LLM interfaces are valid; "AI slop" refers to incoherent, inconsistent, purpose-less, untested, or unvalidated code.

## Important Conventions

1. **Clipboard-First**: Outputs go to clipboard by design.
2. **No API Calls**: Fully local/offline by default. Network only in specific commands/scripts, never the default.
3. **Session Locking**: Always use proper session locking.
4. **Platform Compatibility**: Code must work identically on Linux, macOS, and Windows.
5. **Script Discovery**: User scripts are auto-discovered from configured paths (experimental UX).
6. **Avoid Prepositions in Names**: No prepositions (From, Into, To, By, On, In, Of, For) in public APIs. Prefer `LoadConfig` over `LoadFromConfig`. Exception: `With*` option constructors, `ToJSON` matching external API contracts.
7. **Go as Reusable Modules**: Go code must be exposed as reusable, modular implementations accessible via `osm script`.
8. **JS for App-Specific Logic**: All application-specific functionality MUST be modeled as JavaScript.
9. **Structured Logging**: All log calls must use lowercase, punctuation-free, event-phrased messages; never use string concatenation; attach all context as camelCase key-value attributes. Prefer passing `*slog.Logger` explicitly.

## Concurrency Model

The embedded JavaScript runtime (Goja) is single-threaded with a shared event loop. All JS execution happens on one goroutine. The `goja.Runtime` is not goroutine-safe—never touch it from outside the event loop. Bindings that perform I/O offload work to goroutines and resolve Promises back on the event loop via `adapter.Submit(func(*goja.Runtime))` (fire-and-forget) or `adapter.NewPromise()` + `PromiseSettler` (owner-only creation, settler is safe to call from another goroutine via `settler.Resolve/Reject(func(*goja.Runtime) any) error`). The `*goeventloop.Loop` is not exposed via `Adapter` (no `Adapter.Loop()`/`JS()`/`Runtime()`); hold `*Loop` separately only where lifecycle ownership requires it (e.g., `internal/scripting/runtime.go` bootstrap token via `loop.Promisify`). Strict microtask ordering is always-on in the 20260823 surface (no `WithStrictMicrotaskOrdering` option).

## JS Binding Contract

JS bindings are guests on the event loop. No binding may monopolize it.

- **Non-blocking I/O**: Any binding that does network, disk, subprocess, clipboard, or any I/O that can wait must return a native `goja.Promise` (via `adapter.NewPromise()`) and run the work off the event loop in a goroutine, settling via `settler.Resolve`/`Reject` which internally `Submit`s under the logical owner. Synchronous file reads/writes, synchronous process execution, and synchronous waits are forbidden in new code. Where `*goeventloop.Loop` is directly held, `loop.Promisify` may be used, but `Adapter` itself no longer exposes `Loop.Promisify` — prefer `NewPromise`+`Settler`.
- **Context propagation**: Capture the base `context.Context` passed to `Require` and thread it into every async goroutine. Do not use `context.Background()` inside a binding—it defeats cancellation. If the underlying Go function accepts `context.Context`, blocking synchronously is truly egregious—always async.
- **Consistent constructors**: Every module's `Require` must accept `context.Context` as its first parameter. Modules needing async support take the event-loop adapter next. Do not store `loop` or `promisify` fields on structs — use `adapter` directly and create promises owner-safely via `NewPromise`.
- **No synchronous sleeps**: `time.Sleep` in a binding stalls all JavaScript. Use the event loop's timer/Promise machinery (`delay`, `setTimeout`) instead.
- **Confirm sync is appropriate**: Even bindings without I/O must be confirmed non-blocking. If a binding does anything beyond pure CPU/memory computation, it must be async.
- **Owner-safe settlement**: `PromiseSettler` result callbacks receive the exact owner `*goja.Runtime`; returning a `goja.Value` from another runtime rejects with `TypeError`. Return `rt.NewGoError(err)` or `rt.ToValue(v)` inside the callback when conversion is needed. Handle `ErrAdapterInvalid`/`ErrPromiseSettled` where settlement may race with shutdown. `adapter.Done()` is the goroutine-safe terminal barrier; `adapter.TrackAbortSignal` is the owner-only abort integration.
- **Go-native blocking workloads require tracked promises**: Any binding whose work blocks or does I/O in Go (disk, network, subprocess, clipboard, timers, or any Go code that can wait) — i.e., work *not* routed through JS-host APIs — must surface that work as a JS `Promise` that participates in loop lifecycle. The ergonomic trap is `adapter.NewPromise()` (owner-only) + `go func(){ ...; settler.Resolve/Reject(...) }` with a bare, untracked goroutine: graceful `Shutdown` will not join it, auto-exit can fire mid-I/O, and if the loop terminates first the settler's `Submit` is silently dropped (the JS `Promise` never settles). For such workloads bindings MUST use `adapter.Promisify(ctx, fn)` (sugar for `func(ctx context.Context) (any,error)`) or `adapter.TrackPromise(ctx, run)` with `TrackedSettlement` — both implemented in goja-eventloop atop `goeventloop.Loop.Promisify` (promisifyWg shutdown join, liveness participation, Goexit/cancel guard) plus an owner-side terminal sweep that rejects any still-pending bridged promise with `ErrLoopTerminated`. `loop.Promisify` directly remains correct only where the `*Loop` is held and no JS `Promise` value is needed (no bridge). Pure CPU/memory bindings that never block may stay synchronous and need no promise; do not reintroduce bare-goroutine settlements for blocking Go work.
