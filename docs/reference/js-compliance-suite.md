# JS Runtime Compliance Suite

`internal/jscompliance` is the production-grade compliance test suite for the
`osm` JavaScript runtime. It verifies that the runtime — the Goja engine, the
`osm:*` native modules, the global objects, the CommonJS module system, and the
event-loop/Promise/timer surface — behaves according to its contract, and that
**any deviation surfaces as a real test failure** (never a silent skip, except
for tracked, documented limitations).

`docs/scripting.md` is the contract authority this suite pins; where code and
docs disagree, the disagreement is recorded as a drift finding and resolved
(fixed in code, or documented with evidence).

## Shape

The suite is a **test-only** package — every `.go` file is `*_test.go`, so it
never enters the production build or `deadcode`. It is split into two tiers:

- **FAST tier** (always-on under `gmake all`): the data-driven module contract
  table, ESM rejection, the ES2020+ global-surface inventory, core ECMAScript
  semantics, module resolution, security, console, and the pure-module
  behavior gut-checks. No I/O, a couple of seconds, never mutates host state.
- **SLOW tier** (guarded by `skipSlow` / `testing.Short`): behavioral specs that
  do real I/O (file, process, HTTP via `httptest`, temp git repos), exercise
  timers, and the event-loop liveness check.

## Running

```sh
gmake test-jscompliance        # FAST tier only (~2s, always-on subset)
gmake test-jscompliance-all    # FAST + SLOW (incl. real I/O)
go test -race ./internal/jscompliance/...   # direct (full package, -race)
```

The FAST tier is automatically included in `gmake all` (and the Linux/Windows
gates), because `all` runs every package's tests.

## How it's built

Every test constructs the **real production engine** via a harness that mirrors
`internal/scripting.newTestEngine` verbatim (`scripting.NewEngine` + cleanup).
All JavaScript runs on the engine's single event-loop goroutine; the Go side
observes results by attaching Go-side handlers to Promises — it never blocks on
a Promise expression and a never-settling promise **fails** the test (it never
silently passes). One fresh engine is used per `t.Run` subtest (isolation);
`t.Parallel` is safe because each subtest owns a private engine/vm.

Assertions are authored in JavaScript (`internal/jscompliance/specs/*.spec.js`)
against a tiny dependency-free assert runtime (`specs/harness.js`). A Go driver
loads the harness + a spec, awaits the harness `__done` Promise, and maps each
recorded result to a `t.Run` subtest. A spec that registers **zero** tests
fails — a spec that asserts nothing is a false-confidence trap.

## What it covers

- **Module contract (`TestModuleContract` / `TestModuleSurface`):** every one of
  the 47 `osm:*` modules loads; headline exports exist; documented async
  exports are functions; pure modules are *invoked* with value-smokes (closing
  the "typeof passes but the function throws" trap). The live export surface is
  captured per module and logged for doc-drift triage. Adding a module or export
  = adding a row to `moduleContracts` in `modules_test.go`.
- **ESM rejection (`TestESM_Rejection`):** `import` / `export` / `export
  default` / dynamic `import()` all throw `SyntaxError` (CommonJS only).
- **Core ECMAScript semantics (`TestCoreES`):** typed arrays, well-known
  symbols, Number/Math/String correctness, error subclassing +
  `AggregateError`, Proxy/Reflect, generators, destructuring, optional
  chaining, nullish coalescing, BigInt, JSON edge cases (incl. `__proto__`
  non-pollution), the `module.exports` vs `exports` aliasing trap, and the
  Go↔JS marshalling precision boundary.
- **Promise / event loop (`TestCorePromises` / `Microtask` / `Timers` /
  `Abort`):** combinators, ES2024 `Promise.withResolvers`/`try`,
  `WithStrictMicrotaskOrdering`, `setTimeout`/`setInterval`/`setImmediate`,
  `AbortController` + ES2024 `AbortSignal.timeout`/`any`.
- **ES2020+ global surface (`TestGlobalSurface`):** the WHATWG/ES globals the
  adapter provides (TextEncoder, URL, Blob, Headers, FormData, DOMException,
  structuredClone, performance, the WHATWG `crypto`, …).
- **Module resolution + security (`TestResolution_*` / `TestSecurity_*`):**
  relative/absolute/bare/`osm:` resolution, JSON require, caching, the
  `exports={...}` aliasing trap, `__filename`/`__dirname`, and the path-traversal
  hardening (bare-name `..` escape blocked; relative requires intentionally not
  hardened — the documented exception).
- **Binding contract (`TestBindingContract_*`):** I/O exports return Promises
  (async, not sync), and the event loop is **not monopolized** by an async op
  (a timer fires during a long async exec).
- **Globals (`TestGlobals_*`):** `output`, `log`, `tui.createState`, `args`,
  and `console` (both method sets coexist).
- **Behavioral value checks (`TestModules_PureBehavior`, `TestSlow_Os_*`):**
  known crypto digests, encoding round-trips, RFC 7386 `mergePatch`, regexp
  groups, and the documented `os.readFile` response shape `{content,error,
  message}` (a regression to `{data:...}` fails).

## Known limitations (tracked, not silent)

- **goja lacks `for await…of` (async iteration).** `TestCoreES` pins
  `Symbol.asyncIterator` presence only; the parse-unsafe syntax is omitted so
  the spec file loads.
- **Unhandled Promise rejections are not yet observable (RISK-A).** The runtime
  does not wire the event loop's `WithUnhandledRejection` handler, so a
  rejected promise with no handler vanishes silently.
  `TestUnhandledRejection_Observability` is a tracked `t.Skip` with the full
  fix path (add a `SetUnhandledRejection` setter to the `go-eventloop` fork and
  call it on-loop in `runtime.go`); the assertion activates once the fix lands.
  `TestUnhandledRejection_DoesNotCrash` is the actionable stopgap (an unhandled
  rejection must not crash the runtime or stall the loop).
- **termmux `CaptureSession.wait()` is synchronous (WAIT-1).** Unlike
  `exec.spawn.wait()` (async), the termmux wait blocks the event loop. It has
  zero production callers (pr-split polls `isDone()`/`exitCode()` instead);
  making it async requires adding event-loop infrastructure to the termmux
  package tests. `TestBindingContract_TermmuxWaitShouldBeAsync` is a tracked
  `t.Skip` with the concrete fix path.

See `WIP.md` for the live drift register and resolutions.
