# Unified Correctness & Completion Specification for `btbridge`

This document is the **canonical, exhaustive specification** for bringing the `btbridge` implementation up to the level required by the architecture, by the behavior-tree libraries, and by the analyses in all attached documents. It **coalesces all identified issues** and prescribes **concrete, non-optional fixes**. Nothing in this document is optional: every point **MUST** be implemented and verified.

The document is organized into the following sections:

1. [Scope and Non-Negotiable Goals](#1-scope-and-non-negotiable-goals)  
2. [JSLeafAdapter: Final Correctness Specification](#2-jsleafadapter-final-correctness-specification)  
3. [BlockingJSLeaf: Final Correctness Specification](#3-blockingjsleaf-final-correctness-specification)  
4. [Bridge and Event Loop Lifecycle](#4-bridge-and-event-loop-lifecycle)  
5. [Require Module, Manager, and Cross-Language API](#5-require-module-manager-and-cross-language-api)  
6. [Blackboard and State Exposure](#6-blackboard-and-state-exposure)  
7. [Testing, Determinism, and Leak Detection](#7-testing-determinism-and-leak-detection)  
8. [Documentation, Contracts, and Architecture Conformance](#8-documentation-contracts-and-architecture-conformance)  
9. [Promises, Cancellation, and Resource Management](#9-promises-cancellation-and-resource-management)  
10. [Dependencies, Dead Code, and Tooling](#10-dependencies-dead-code-and-tooling)  
11. [Exhaustive Implementation Checklist](#11-exhaustive-implementation-checklist)  

Every issue described in the source documents is integrated somewhere in these sections. If something is not implemented exactly as required here, the implementation is **not** acceptable.

---

## 1. Scope and Non-Negotiable Goals

1. The target architecture is the **Go-centric with JS leaves, event-driven bridge (Variant C.2)**, as described in the architecture context:
   - `go-behaviortree` is the canonical runtime for tree structure and composites.
   - JavaScript is used **only for leaf behaviors**.
   - All JS runs inside a single-threaded `goja` runtime driven by `goja_nodejs/eventloop` (or equivalent), and **all** VM interactions occur via `RunOnLoop`.

2. The implementation **must** be:
   - **Thread-safe** according to its own API claims (e.g., if `JSLeafAdapter` claims to be safe for concurrent `Tick()` calls, it must be correct under that usage).
   - **Race-free** with respect to adapter state; stale completions must never corrupt new runs.
   - **Deadlock-safe** under all documented lifecycle operations (`Bridge.Stop`, leaf cancellation, context cancellation).

3. **All analyses and concerns** raised in the five review documents must be addressed:
   - Every race condition, deadlock, context leak, type mismatch, test flakiness, and documentation discrepancy must be resolved or explicitly designed around.
   - No issue may be silently ignored or reclassified as “minor”.

4. Tests **must be deterministic**:
   - No reliance on arbitrary sleeps or timing assumptions for core correctness.
   - Cancellation, completion, and shutdown must be testable via synchronization, not wall-clock.

5. **Promises and async behavior**:
   - All async JS paths invoked via the bridge must either complete or be safely ignored; no path may leave the adapter in a permanently inconsistent state.
   - You must not rely on unverifiable external guarantees about code quality of user JS; defensive design is required.

---

## 2. JSLeafAdapter: Final Correctness Specification

`JSLeafAdapter` is the heart of the async-to-sync bridge. All issues identified in the documents must be resolved here.

### 2.1 State Machine and Fields

The adapter must maintain the following internal fields:

- `state AsyncState` with values:
  - `StateIdle`: ready to start a new JS execution.
  - `StateRunning`: a JS execution is in flight.
  - `StateCompleted`: a JS execution has finished and a result is available.
- `lastStatus bt.Status`
- `lastError error`
- `generation uint64` — **monotonic dispatch identifier**.
- `ctx context.Context` and `cancel context.CancelFunc` for per-adapter lifecycle (one-shot).
- `mu sync.Mutex` guarding **all** above fields.

All modifications to these fields must be done under `mu`. There must be **no reading or writing of `state`, `lastStatus`, `lastError`, or `generation` outside the lock** except as explicitly allowed below.

### 2.2 Concurrency Guarantees: No Double-Dispatch

The existing implementation had:

```go
a.mu.Lock()
currentState := a.state
a.mu.Unlock()

switch currentState {
case StateIdle:
    a.dispatchJS()
    return bt.Running, nil
```

This is unsafe under concurrent `Tick()` calls because multiple goroutines can see `StateIdle` and both call `dispatchJS`. The corrected pattern is:

- **Inside `Tick`**, for `StateIdle`:

  ```go
  a.mu.Lock()
  if a.state != StateIdle {
      // Re-read state and fall through to the correct case
      currentState := a.state
      a.mu.Unlock()
      return a.tickNonIdle(currentState) // helper handling Running/Completed
  }
  // Transition to running and bump generation atomically:
  a.generation++
  gen := a.generation
  a.state = StateRunning
  a.mu.Unlock()

  a.dispatchJS(gen)
  return bt.Running, nil
  ```

- The essential property: **the transition to `StateRunning` and increment of `generation` occur while still holding the lock**, before any unlock. This ensures at most one call to `dispatchJS` per transition from `Idle`.

If you choose to keep the switch structure, you must still set `state = StateRunning` and increment `generation` inside the lock before unlocking and calling `dispatchJS`.

### 2.3 Generation Guard Against Stale Results

The race identified in multiple documents is that **late completions from an older run** can overwrite the state of a newer run.

The adapter must:

- Increment `generation` on each new dispatch (handled above).
- Pass this `gen` identifier into `dispatchJS(gen)`.
- The JS callback must call `finalize(gen, status, err)`.
- **`finalize` must ignore results for non-current generations**.

Correct pattern:

```go
func (a *JSLeafAdapter) finalize(gen uint64, status bt.Status, err error) {
    a.mu.Lock()
    defer a.mu.Unlock()

    if gen != a.generation {
        // Stale completion from a cancelled or superseded run; ignore entirely.
        return
    }

    a.lastStatus = status
    a.lastError = err
    a.state = StateCompleted
}
```

This ensures that:

- Any JS completion from an earlier generation has **no observable effect**.
- **State corruption by stale completions is impossible**, regardless of timing.

### 2.4 Cancellation and Generation Invalidations

Multiple documents highlight that **cancellation must invalidate the currently running generation** to prevent stale completions from being accepted.

Within `Tick`, for the `StateRunning` case:

```go
case StateRunning:
    select {
    case <-a.ctx.Done():
        a.mu.Lock()
        a.state = StateIdle      // We no longer consider this run "running"
        a.generation++           // CRITICAL: invalidate pending callbacks for this run
        a.mu.Unlock()
        return bt.Failure, errors.New("execution cancelled")
    default:
    }
    return bt.Running, nil
```

Key invariants:

- When cancellation is observed, `generation` is incremented.
- Any in-flight callback captured with the old `gen` will fail the `gen != a.generation` check in `finalize` and be ignored.
- The node is now logically idle, but **its context is cancelled**, so future ticks will treat it as non-reusable (see below).

### 2.5 One-Shot Context Semantics

`NewJSLeafAdapterWithContext` creates a child context:

```go
childCtx, cancel := context.WithCancel(parentCtx)
adapter := &JSLeafAdapter{ctx: childCtx, cancel: cancel, ...}
```

The semantics must be:

- Once `childCtx` is cancelled (by `adapter.Cancel()` or by the parent), **this adapter instance is permanently cancelled**.
- Any subsequent `Tick` must detect `ctx.Done()` in the `StateIdle` path and immediately return `bt.Failure` with `execution cancelled` (or equivalent message). It must not start new JS work when `ctx.Err() != nil`.

So for the `StateIdle` case:

```go
case StateIdle:
    select {
    case <-a.ctx.Done():
        return bt.Failure, errors.New("execution cancelled")
    default:
    }
    // proceed with dispatch as above
```

This matches the “one-shot” contract explicitly described: a cancelled adapter is not reusable. The documents accept that, provided we avoid stale state corruption.

### 2.6 Context Lifetime and Leak Avoidance

A potential leak was identified: if no one calls `Cancel()`, the adapter’s internal context may never be cancelled, and resources may linger.

To avoid leaks:

- `adapter.Cancel()` must be called as part of BT teardown or as part of any explicit cancel path. You must:
  - Document that when using `NewJSLeafAdapterWithContext`, the caller is responsible for calling `Cancel()` when the node is no longer needed.
  - Or, if you want stronger guarantees, call `a.cancel()` when the adapter reaches a terminal, non-reused state such as in `finalize` after `StateCompleted` followed by some external tree teardown; but because the adapter might be reused across ticks, **do not auto-cancel in `finalize`** unless you explicitly want “complete once, and never run again”.

Given prior documents, the simplest internally consistent rule is:

- `Cancel()` is a manual hook. It must be invoked by the BT owner when that subtree is no longer used.
- `NewJSLeafAdapter` (without context arg) uses `context.Background()` and is not cancellable externally; cancellation of its work is purely via generation invalidation and ignoring stale completions.

Whichever policy you adopt, you must:

- Avoid unbounded growth of contexts.
- Clearly document the one-shot behavior of `NewJSLeafAdapterWithContext`.
- Ensure that cancellation invalidates generation, as in 2.4.

### 2.7 `dispatchJS` Implementation Requirements

`dispatchJS` must:

1. **Not modify state** except via the call to `finalize`, which is generation-guarded.
2. Capture the `gen` argument and use it consistently in all error and completion paths.
3. Respect event loop termination.

Pseudocode (simplified; you must adapt):

```go
func (a *JSLeafAdapter) dispatchJS(gen uint64) {
    ok := a.bridge.RunOnLoop(func(vm *goja.Runtime) {
        defer func() {
            if r := recover(); r != nil {
                a.finalize(gen, bt.Failure, fmt.Errorf("panic in JS leaf %s: %v", a.fnName, r))
            }
        }()

        fnVal := vm.Get(a.fnName)
        if _, ok := goja.AssertFunction(fnVal); !ok {
            a.finalize(gen, bt.Failure, fmt.Errorf("JS function '%s' not callable", a.fnName))
            return
        }

        runLeafVal := vm.Get("runLeaf")
        runLeafFn, ok := goja.AssertFunction(runLeafVal)
        if !ok {
            a.finalize(gen, bt.Failure, errors.New("runLeaf helper not found"))
            return
        }

        callback := func(call goja.FunctionCall) goja.Value {
            statusStr := call.Argument(0).String()
            var err error
            if arg1 := call.Argument(1); !goja.IsNull(arg1) && !goja.IsUndefined(arg1) {
                err = fmt.Errorf("%s", arg1.String())
            }
            a.finalize(gen, mapJSStatus(statusStr), err)
            return goja.Undefined()
        }

        var ctxVal goja.Value
        if a.getCtx != nil {
            ctxVal = vm.ToValue(a.getCtx())
        } else {
            ctxVal = goja.Undefined()
        }

        _, err := runLeafFn(
            goja.Undefined(),
            fnVal,
            ctxVal,
            goja.Undefined(),    // args, if any
            vm.ToValue(callback),
        )
        if err != nil {
            a.finalize(gen, bt.Failure, fmt.Errorf("runLeaf failed for '%s': %w", a.fnName, err))
        }
    })

    if !ok {
        a.finalize(gen, bt.Failure, errors.New("event loop terminated"))
    }
}
```

Key requirements:

- **All code paths** that represent a failure to start or execute JS must call `finalize(gen, ...)`.
- `RunOnLoop` returning `false` must be treated as a terminal failure and must call `finalize`.

### 2.8 `mapJSStatus` and Status String Constants

`mapJSStatus` must map status strings from JS to `bt.Status`. The strings `"running"`, `"success"`, `"failure"` must be defined in a **single place** (e.g., a `const` block) and reused across adapter, bridge, and require APIs to avoid divergent spelling.

```go
const (
    jsStatusRunning = "running"
    jsStatusSuccess = "success"
    jsStatusFailure = "failure"
)

func mapJSStatus(s string) bt.Status {
    switch s {
    case jsStatusRunning:
        return bt.Running
    case jsStatusSuccess:
        return bt.Success
    case jsStatusFailure:
        return bt.Failure
    default:
        return bt.Failure
    }
}
```

This removes status “magic strings” scattered through files.

### 2.9 Thread Safety Claim

After the above:

- The adapter can legitimately be documented as **safe for concurrent `Tick()` calls** because:
  - All state transitions are guarded by `mu`.
  - Generation-based guards prevent stale completions.
  - No double-dispatch is possible from concurrent `Tick()` calls in `StateIdle`.

This must be reflected in documentation: if you state “thread-safe”, you must implement these behaviors.

---

## 3. BlockingJSLeaf: Final Correctness Specification

`BlockingJSLeaf` is the simpler, synchronous adapter. Multiple analyses highlight that it can **deadlock forever** and that it can **double-send** on its result channel.

You must make the following changes.

### 3.1 Single-Delivery Guarantee with `sync.Once`

All sends to the result channel must be guarded with `sync.Once` to avoid double-send and associated deadlocks.

Pattern:

```go
ch := make(chan result, 1)
var once sync.Once

send := func(r result) {
    once.Do(func() {
        ch <- r
    })
}
```

Then:

- In the callback: `send(result{status: mapJSStatus(...), err: err})`
- On synchronous `runLeafFn` error: `send(result{bt.Failure, err}); return` (and you must `return` to avoid the callback also trying to send).
- On panic recovery: `send(result{bt.Failure, fmt.Errorf("panic: %v", r)})`

This guarantees at most one send.

### 3.2 Avoid Infinite Block: Use Context or `Bridge.Done`

You must not do a raw `r := <-ch` with no alternative. There are two required protections:

1. Select against `bridge.Done()` (a new channel described in section 4):

   ```go
   select {
   case r := <-ch:
       return r.status, r.err
   case <-bridge.Done():
       return bt.Failure, errors.New("bridge stopped")
   }
   ```

2. Preferably, also allow a caller-provided `context.Context` to bound wait time. If you choose not to add context to `BlockingJSLeaf`, at least the `bridge.Done()` select is required; but adding context is more robust.

The fix must ensure:

- If the event loop is stopped after `RunOnLoop` returns `true` but before the callback executes, the calling goroutine does not hang forever.
- If the JS Promise never settles, the consumer has a way to time out (either via external context or via application-level design).

### 3.3 No Blocking in Event Loop Goroutine

The callback running on the event loop goroutine must never block indefinitely on sending to `ch`. With the `sync.Once` and buffer of 1, and considering that the receiving goroutine is always listening via `select` as above, you avoid unbounded blocking.

You must ensure:

- There is no execution path where `send` is called after the receiver goroutine has returned and no one listens to `ch` anymore. `sync.Once` plus careful control of when goroutines exit must enforce this.

---

## 4. Bridge and Event Loop Lifecycle

The `Bridge` type must correctly manage:

- Event loop lifecycle (`Start`, `Stop`).
- A `Done()` channel for consumers.
- Safe `RunOnLoop` scheduling and termination behavior.
- Module registry wiring.

### 4.1 Bridge Fields and Context

The `Bridge` must have:

```go
type Bridge struct {
    loop   *eventloop.EventLoop
    mu     sync.RWMutex
    started bool
    stopped bool

    ctx    context.Context
    cancel context.CancelFunc
}
```

In `NewBridge` / `NewBridgeWithContext`:

- Create a `childCtx, cancel := context.WithCancel(context.Background())` (or derived from caller’s context) and store it in the bridge.
- **Do not** leave `ctx` nil.

### 4.2 `Done()` Channel

Implement:

```go
func (b *Bridge) Done() <-chan struct{} {
    return b.ctx.Done()
}
```

This must be used by `BlockingJSLeaf` (and any other long-blocking operations) as described.

### 4.3 Stop Semantics

`Stop()` must:

- Be idempotent (as currently intended).
- Set `stopped = true`.
- Call `b.cancel()` to close the `Done` channel.
- Then call `b.loop.Stop()`.

Pattern:

```go
func (b *Bridge) Stop() {
    b.mu.Lock()
    if !b.started || b.stopped {
        b.mu.Unlock()
        return
    }
    b.stopped = true
    b.mu.Unlock()

    b.cancel()
    b.loop.Stop()
}
```

This guarantees that **any goroutine waiting on `Bridge.Done()` will unblock** when the bridge stops.

### 4.4 `RunOnLoop` and `RunOnLoopSync` Guarantees

You must enforce:

- `RunOnLoop` returns `false` when the bridge is not running or is stopping.
- For synchronous variants (if present, e.g., `RunOnLoopSync`), ensure they:

  ```go
  done := make(chan struct{})
  var runErr error

  ok := b.loop.RunOnLoop(func(vm *goja.Runtime) {
      defer close(done)
      runErr = fn(vm)
  })
  if !ok {
      return errors.New("event loop not running")
  }

  select {
  case <-done:
      return runErr
  case <-b.Done():
      return errors.New("bridge stopped before completion")
  }
  ```

This prevents a scenario where:

- The event loop is stopped **after** the job is scheduled but **before** it runs, leaving `RunOnLoopSync` blocked forever on `done`.

### 4.5 Module Registry Wiring

The `Require`-based module loader for `"osm:bt"` must actually be registered:

- In `NewBridgeWithContext` (or similar initialization), after creating `reg := require.NewRegistry()`, you must:

  ```go
  reg.RegisterNativeModule("osm:bt", Require(ctx, b))
  ```

- Otherwise, the `Require()` helper is effectively dead code because there is no way for JS to `require('osm:bt')` successfully.

If the module is intended for external use rather than automatic registration, that must be **explicitly documented** and cross-referenced; otherwise, automatic registration is required.

---

## 5. Require Module, Manager, and Cross-Language API

The `"osm:bt"` module and `Manager` must be internally consistent with the Go-centric design. Several issues were identified.

### 5.1 `Require(ctx, bridge *Bridge)` and Nil Bridge

`Require` must validate that `bridge` is non-nil:

- If you allow `bridge == nil`, any attempt to use `createLeafNode` will panic.
- Therefore, `Require` must either:
  - Panic early with a clear error if `bridge == nil`, or
  - Export functions that throw descriptive JS exceptions when invoked.

A simple, safe behavior:

```go
if bridge == nil {
    panic("btbridge.Require: bridge must not be nil")
}
```

This prevents silent misuse and undefined behavior.

### 5.2 `newNode` and `newTicker` Type Semantics

`newNode` currently expects:

```go
tickVal := call.Arguments[0].Export()
tick, ok := tickVal.(bt.Tick)
...
child, ok := call.Arguments[i].Export().(bt.Node)
```

This means:

- Only **Go values** of type `bt.Tick` and `bt.Node` can be passed from JS (i.e., previously exported Go functions/values).
- Raw JS-defined functions cannot become `bt.Tick` or `bt.Node` via `Export()`.

This is consistent with the **Go-centric** approach but must be documented:

- `osm:bt.newNode` is a helper for assembling **Go** composites and nodes **from JS**, not for defining JS composites.
- Mixing JS-defined composites (bt.js-style) with these Go nodes is not supported.

You must:

- Update documentation (see section 8) to explain that `newNode` and `newTicker` **do not** accept arbitrary JS functions; they accept Go node values passed into JS.
- Ensure tests cover the expected, supported usage patterns.

### 5.3 Status Type Mismatch: bt.js vs Go

The architecture documents note that bt.js expects string statuses `"running"|"success"|"failure"`, whereas Go composites from `go-behaviortree` use integer statuses.

When exposing Go composites or Go ticks via the `Require` module:

- **Do not** pass them directly into a bt.js-like JS composite that expects strings, unless there is a translation layer.
- Either:
  - Restrict the module so that Go composites are only used by Go (or by JS code that is aware of int statuses), and clearly document that `bt.js` composites must not be mixed with these, or
  - Provide wrappers that translate `bt.Status` ints to strings and vice versa.

Given the Go-centric design and the analysis, the simplest and internally consistent approach is:

- **Explicitly document** that:
  - `"osm:bt"` is for assembling **Go** trees.
  - The embedded bt.js is not used directly; bt.js-style composites are not supported in the same runtime.
  - Leaf functions may still follow bt.js-like leaf contracts (string statuses), but composite logic is not shared.

### 5.4 Manager Type and Tests

The `Manager` type exposed via `require.go`:

- Must be either:
  - Fully tested with integration tests demonstrating creation, execution, and closure via the JS module, or
  - Clearly documented as experimental/internal and not part of the supported public API.

You must:

- Either write tests that build a tree via `"osm:bt"`, run it, and verify blackboard updates via `Manager`, or
- Mark `Manager` as internal and remove its exposure from the JS module until it is tested.

---

## 6. Blackboard and State Exposure

The `Blackboard` type is the Go-owned, mutex-protected state container exposed to JS.

### 6.1 Thread Safety and API Surface

The blackboard must:

- Use `sync.RWMutex` to guard `data map[string]any`.
- Provide `Get`, `Set`, `Has`, `Delete`, and `Keys` (or similar).
- Be safe to call from:
  - Go goroutines.
  - JS functions when invoked via event loop (calls come back as Go calls).

All existing methods that satisfy these constraints are acceptable.

### 6.2 `ExposeToJS` Error Handling

Existing code:

```go
_ = obj.Set("get", b.Get)
_ = obj.Set("set", b.Set)
...
```

You must either:

- Handle errors returned by `obj.Set` (e.g., log or propagate), or
- Add a comment explicitly acknowledging that `Set` cannot fail for exported methods in this context and that ignoring errors is intentional.

To be consistent and explicit, best is:

```go
if err := obj.Set("get", b.Get); err != nil {
    panic(fmt.Sprintf("failed to expose get: %v", err))
}
```

or similar.

### 6.3 Mapping Types in Tests

Tests currently do:

```go
count := bb.Get("counter").(int64)
```

This is unsafe if JS did not run (initial value may be an `int`, not `int64`, or `nil`).

You must:

- Either set the initial type to `int64` explicitly in tests (`bb.Set("counter", int64(0))`), or
- Use a type switch or `require.IsType` before assertion:

  ```go
  val := bb.Get("counter")
  require.IsType(t, int64(0), val)
  count := val.(int64)
  ```

The same applies to boolean fields; do not blindly assert `. (bool)` without ensuring non-nil, correct type.

### 6.4 Ordering Assumptions

`Keys()` returns a slice of keys based on map iteration order, which is non-deterministic. Tests already use `ElementsMatch`, which is correct. This must be kept.

---

## 7. Testing, Determinism, and Leak Detection

Tests must be rock-solid and deterministic. Every identified issue in the analysis must be addressed.

### 7.1 Eliminate Timing-Based Flakiness

Remove or replace patterns like:

```go
time.Sleep(10 * time.Millisecond)
...
waitForCompletion(ctx, ticker, ...)
```

with:

- Synchronization via channels or conditions that are signaled from within the code under test.
- Polling loops are acceptable only if bound by a **generous timeout** and used for non-critical metrics, but core correctness (e.g., “the adapter never returns stale results”) must not rely on short sleeps.

Examples:

- To test cancellation:
  - Have the JS leaf call back into Go or update blackboard in a deterministic way once started.
  - Cancel the context immediately after you know the leaf has started (via a Go channel).
  - Verify that subsequent `Tick` returns `Failure` and that no stale result is ever reported.

### 7.2 Avoid Dependence on `setTimeout` and Timers in Tests

Tests that rely on `setTimeout` existing and being correctly integrated with `goja_nodejs/eventloop` are fragile.

You must:

- Prefer Promises you resolve explicitly from Go (e.g., JS keeps a global `resolve` function that Go calls via `RunOnLoop`), enabling fully deterministic control over when completion happens.
- If you do test timers, add a separate suite that is clearly labeled and uses large timeouts; but core correctness tests should not depend on timers.

### 7.3 Add Goroutine Leak Detection

You must:

- Add tests (or use `go.uber.org/goleak` or similar) that verify no goroutines leak after running a suite of BT executions and cancellations.
- At minimum:
  - Capture `runtime.NumGoroutine()` before and after a series of runs with a buffer; fail if the count grows unbounded.
  - Ensure `BlockingJSLeaf` and `JSLeafAdapter` do not leave goroutines stuck on dead channels.

### 7.4 Validate Event Loop Shutdown Scenarios

Tests must cover:

- `Bridge.Stop()` called while adapters are in `StateRunning` (non-blocking leaf).
- `Bridge.Stop()` called while `BlockingJSLeaf` is waiting on `ch`:
  - Confirm that `BlockingJSLeaf` unblocks via `bridge.Done()` and returns failure.
- `RunOnLoop` returning `false`:
  - Adapters must treat this as terminal error and not remain stuck in `StateRunning`.

### 7.5 Test Count and Assertions

- The earlier claim of “27 passing tests” must be updated to reflect actual numbers; you must not rely on outdated counts in documentation.
- Every test that expects a ticker to finish must check `ticker.Err()` and assert against expected error conditions (`nil`, `context.DeadlineExceeded`, etc.), not ignore it.

Example correction:

```go
<-ticker.Done()
require.True(t, errors.Is(ticker.Err(), context.DeadlineExceeded))
```

---

## 8. Documentation, Contracts, and Architecture Conformance

All public-facing documentation, especially `doc.go` and `WIP.md` (or equivalent), must be aligned with actual behavior and the Go-centric architecture.

### 8.1 bt.js Compatibility Claims

You must:

- Clearly state that:
  - Full bt.js composite API (sequence, fallback, parallel, etc.) is **not** implemented on top of goja in this bridge.
  - The bridge supports **bt.js-style leaves** (async functions returning string statuses) but uses **Go-native composites** via `go-behaviortree`.
- Remove or correct any claims that suggest full bt.js tree execution in JS (unless you actually implement it and test it, which is currently not the case).

### 8.2 Thread Safety Claims

Where documentation (e.g., comments near `JSLeafAdapter`) claims “thread-safe adapter for concurrent access”, you must ensure:

- The code matches those claims (see section 2).
- Tests exercise concurrent `Tick()` calls (even though the typical `Ticker` only calls from one goroutine).

### 8.3 Performance Claims

Statements like “~50–200µs per leaf execution” must be:

- Either supported by actual benchmarks (and included in `bench_test.go` or docs), or
- Softened to avoid implying experimental numbers are guaranteed.

You must not present unverified performance numbers as factual guarantees.

### 8.4 WIP and Release Status

If `WIP.md` or doc headers say “Status: COMPLETE ✅ Ready for release”, these must be updated only **after**:

- All fixes in this document are implemented.
- Tests, linters, staticcheck, and deadcode (with justified ignores) pass.
- At least basic stress tests / load tests have to be run and reported.

Until then, documentation must not overstate readiness.

### 8.5 go-pabt Integration Claims

If documentation mentions integration with `go-pabt` (planner), but the code does not implement any such integration:

- Either implement the described integration (wrapping JS leaves in planner actions) and test it, or
- Remove or downgrade such claims to “future work / not yet implemented”.

---

## 9. Promises, Cancellation, and Resource Management

Promises and async JS form a major part of the bridge; they must not leak resources or remain in unresolved states that break invariants.

### 9.1 Promise Lifetime Guarantees

You must guarantee:

- The Go side never **depends** on a JS Promise resolving to make progress after cancellation:
  - Stale completions are ignored via generation guard.
  - `JSLeafAdapter` and `BlockingJSLeaf` do not block forever waiting for unresolved Promises.

You are not responsible for ensuring user JS never leaks Promises, but you must ensure:

- **Your** code never hangs indefinitely regardless of JS leaf behavior.

### 9.2 Optional Timeouts

While not strictly required for safety, you are strongly encouraged to:

- Add optional per-leaf timeouts:
  - E.g., supply `context.Context` to `NewJSLeafAdapterWithContext` and have JS leaf consult an abort API (`execCtx.IsCancelled()` or an AbortSignal) directly.
- What is mandatory is that the Go runtime never permanently blocks; optional addition is to propagate cancellation signals into JS logic.

### 9.3 Abort Signaling to JS (Optional but Recommended)

An optional but desirable extension is:

- Expose an `AbortController` / `AbortSignal`-like construct in JS:
  - `signal.Aborted()` reads a Go `context.Context`.
  - JS leaves can periodically check `signal.Aborted()` to stop work.

If you choose to implement this (as the architecture allowed), you must:

- Hook it to the same context used by `JSLeafAdapter`.
- Document that cancellation means JS might still do work unless it cooperatively checks the signal.

### 9.4 Side Effects After Cancellation

Even with generation guards, JS can still mutate the Go blackboard after cancellation unless you:

- Check `ctx.Err()` inside each callback before applying effect, or
- Use a separate context passed into the JS leaf’s `ctx` argument and have it check cancellation before, e.g., writing to state.

At a minimum:

- `finalize` must not update the adapter’s internal state after cancellation (covered by generation).
- Blackbox side effects authored by users are beyond your direct control; but documentation should warn that JS leaves that continue running after cancellation might still mutate shared state unless written carefully.

---

## 10. Dependencies, Dead Code, and Tooling

### 10.1 go.mod Dependencies for goja

`go.mod` must include:

```txt
require (
    github.com/dop251/goja vX.Y.Z
    github.com/dop251/goja_nodejs vA.B.C
    ...
)
```

Concrete versions may differ, but the point is:

- The project **must** declare these dependencies; missing them causes build failure.

### 10.2 deadcode and `.deadcodeignore`

The `.deadcodeignore` file currently ignores a set of functions, including some in `btbridge`. You must:

- Review every ignored symbol and justify:
  - It is part of a public API used only in tests/examples, or
  - It is compiled under specific build tags.
- Ensure that `deadcode` is not blindly suppressing genuinely unused production code.

If any `btbridge` functions are truly unused or misdesigned, you must either:

- Remove them, or
- Use them in tests or examples such that they are reachable and not considered dead.

### 10.3 Static Analysis and CodeQL Claims

Claims like:

- “vet, staticcheck, deadcode all pass”
- “CodeQL security check passed”

cannot be treated as established until:

- You update them after the current set of fixes.
- You actually run those tools in CI.

Documentation must not assert these as current truths until re-verified.

---

## 11. Exhaustive Implementation Checklist

This section enumerates a concrete, ordered list of tasks that must all be completed. No item is optional. Many items restate previous sections to **eliminate any ambiguity**.

### 11.1 JSLeafAdapter

1. Add `generation uint64` field to `JSLeafAdapter`.
2. In `Tick`, for `StateIdle`:
   - Under lock: verify `state == StateIdle`.
   - Increment `generation`.
   - Set `state = StateRunning`.
   - Capture `gen := generation`.
   - Unlock.
   - Call `dispatchJS(gen)`.
3. In `Tick`, for `StateRunning` + cancelled context:
   - Under lock: set `state = StateIdle`.
   - Increment `generation` (to invalidate pending callbacks).
   - Unlock.
   - Return `bt.Failure, errors.New("execution cancelled")`.
4. In `Tick`, for `StateIdle` path before dispatch:
   - Check `<-a.ctx.Done()`; if closed, return cancellation failure immediately.
5. Modify `dispatchJS` to accept `gen uint64` parameter and pass `gen` to all `finalize` calls (panic recovery, missing function, missing `runLeaf`, runLeaf error, event loop termination).
6. Implement `finalize(gen, status, err)`:
   - Lock.
   - If `gen != a.generation`, return without modifying state.
   - Otherwise, set `lastStatus`, `lastError`, and `state = StateCompleted`.
   - Unlock.
7. Define constants for JS status strings and use them everywhere (`mapJSStatus`, JS initialization, `Require` module).

### 11.2 BlockingJSLeaf

8. Wrap writes to the result channel `ch` in a `sync.Once`-protected helper:

   ```go
   var once sync.Once
   send := func(r result) {
       once.Do(func() { ch <- r })
   }
   ```

9. In each place where you would send to `ch` (callback, error path, panic recovery), call `send(...)` and return immediately if appropriate to avoid double-sends.
10. After `RunOnLoop`, instead of `r := <-ch`, use:

    ```go
    select {
    case r := <-ch:
        return r.status, r.err
    case <-bridge.Done():
        return bt.Failure, errors.New("bridge stopped")
    }
    ```

11. Optionally, add a `context.Context` parameter or use a context associated with the BT to bound wait time; if you do, integrate it into the select.

### 11.3 Bridge

12. Add `ctx context.Context` and `cancel context.CancelFunc` to `Bridge`.
13. In `NewBridge` / `NewBridgeWithContext`, initialize `ctx` and `cancel`.
14. Implement `Done() <-chan struct{}` to return `b.ctx.Done()`.
15. In `Stop()`:
    - Ensure you set `stopped = true` under lock exactly once.
    - Call `b.cancel()` before or after `loop.Stop()` (order is up to you, but both must be called).
16. Ensure `RunOnLoop` returns `false` when the loop is not running.
17. If you have `RunOnLoopSync` or equivalent, wrap the wait in a select that also listens on `b.Done()` to avoid indefinite block if the loop stops before executing the callback.

### 11.4 Require Module and Manager

18. In `Require(ctx, bridge)`, assert that `bridge != nil` and fail early if not.
19. Register the `"osm:bt"` module (`Require`) in the registry in `NewBridgeWithContext`:

    ```go
    reg.RegisterNativeModule("osm:bt", Require(ctx, bridge))
    ```

20. Document in `doc.go` or module docs that:
    - `newNode` and `newTicker` operate only on **Go** `bt.Node` and `bt.Tick` values exported into JS.
    - They do not accept arbitrary JS functions as ticks or nodes.
21. Decide whether `Manager` is part of supported API:
    - If yes, add integration tests that:
      - Create a `Manager` via `"osm:bt"`.
      - Build a simple tree.
      - Run it and verify glyph updates on blackboard.
    - If not, hide or deprecate `Manager` until tested.

### 11.5 Blackboard

22. Update tests to avoid unsafe type assertions:
    - Either store `int64` from the start (`Set("counter", int64(0))`).
    - Or use `IsType` / type switches before asserts.
23. In `ExposeToJS`, either:
    - Handle errors from `obj.Set` explicitly, or
    - Add a clear comment that failures are impossible given goja contract and ignore them intentionally.
24. Keep using `ElementsMatch` for key lists; do not rely on iteration order.

### 11.6 Testing & Determinism

25. Remove or refactor tests that rely on short `time.Sleep` durations as core correctness checks.
26. Replace timer-based assertions with channel-based synchronization from the code under test.
27. Avoid reliance on `setTimeout` for JS tests that check adapter semantics:
    - Use Promises whose resolution you control from Go via `RunOnLoop`.
28. Add leak checks:
    - Either via `runtime.NumGoroutine()` before/after a test suite or by integrating `goleak`.
29. Add tests for:
    - `JSLeafAdapter` cancellation not returning stale results (explicit “race on cancel” test).
    - `BlockingJSLeaf` not deadlocking when `Bridge.Stop()` is called while waiting.
    - Event loop termination causing adapters to return failure and never stay stuck in `StateRunning`.

### 11.7 Documentation & Claims

30. Update `doc.go` and `WIP.md` (or equivalents) to:
    - Clarify Go-centric design and JS-leaves-only support.
    - Remove or modify any full bt.js compatibility claims.
    - Remove or hedge unverified performance numbers.
    - Update test and lint status only after reruns.
31. Document “one-shot” semantics of `NewJSLeafAdapterWithContext`:
    - Once its context is cancelled, this adapter instance cannot be reused.
32. Document that JS cancellation is cooperative:
    - Go will stop waiting and ignore stale completions.
    - JS code must explicitly handle cancellation if it needs to avoid side effects after cancel.

### 11.8 Promises & Resource Management

33. Ensure all paths in `dispatchJS` call `finalize(gen, ...)` when the JS function cannot be started or fails synchronously.
34. Ensure the callback from JS never blocks indefinitely sending to the channel (for `BlockingJSLeaf`) or updating adapter state (for `JSLeafAdapter`).
35. Optionally implement and expose cancellation signals or Abort-like constructs to JS, tied to `context.Context`.

### 11.9 Dependencies and Dead Code

36. Add `github.com/dop251/goja` and `github.com/dop251/goja_nodejs` to `go.mod`.
37. Verify `.deadcodeignore`:
    - Ensure every ignored symbol is legitimately unreachable in production (tests-only, debug-only, etc.).
    - If any `btbridge` symbol is truly unused, either remove it or ensure test coverage.

### 11.10 Final Validation

38. After implementing all the above:
    - Run the full test suite.
    - Run race detector (`go test -race ./...`).
    - Run linters (vet, staticcheck).
    - Run deadcode with the current `.deadcodeignore` and verify only justified symbols are ignored.
39. Only then update any “Ready for release” statements or similar claims.

---

By following this specification exactly, you will:

- Eliminate the stale-state race in `JSLeafAdapter`.
- Eliminate deadlocks and double-send bugs in `BlockingJSLeaf`.
- Make `Bridge` lifecycle robust under shutdown and failures.
- Clarify and harden the `Require` module semantics.
- Ensure blackboard and tests are safe and deterministic.
- Bring documentation and claims into alignment with reality.
- Satisfy the architectural constraints of a Go-centric, JS-leaf, event-driven behaviors bridge.
