# Unified Code Review: btbridge JavaScript Behavior Tree Integration

**Status:** BLOCKING — PR contains critical defects that MUST be fixed before merge
**Scope:** Complete correctness analysis synthesized from multiple review passes

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Critical Concurrency Defects](#2-critical-concurrency-defects)
3. [Resource Management & Lifecycle Bugs](#3-resource-management--lifecycle-bugs)
4. [BlockingJSLeaf Defects](#4-blockingjsleaf-defects)
5. [API Design Issues (Require Module)](#5-api-design-issues-require-module)
6. [Test Infrastructure Defects](#6-test-infrastructure-defects)
7. [Error Handling & Edge Cases](#7-error-handling--edge-cases)
8. [Documentation & Claims Verification](#8-documentation--claims-verification)
9. [Missing Dependencies & Build Issues](#9-missing-dependencies--build-issues)
10. [Required Corrections](#10-required-corrections)
11. [Implementation Verification Checklist](#11-implementation-verification-checklist)

---

## 1. Executive Summary

### 1.1 Verdict

**The PR is NOT CORRECT and MUST NOT be merged.** It contains multiple critical correctness defects that will cause data corruption, deadlocks, and undefined behavior in production.

### 1.2 Blocking Defects (Immediate Fix Required)

| ID | Defect | Severity | Location |
|----|--------|----------|----------|
| C1 | Stale callback overwrites state (race condition) | CRITICAL | `adapter.go` |
| C2 | Tick() race condition (double dispatch) | CRITICAL | `adapter.go:86-109` |
| C3 | BlockingJSLeaf permanent deadlock | CRITICAL | `adapter.go:268` |
| C4 | BlockingJSLeaf double-send bug | CRITICAL | `adapter.go` |
| C5 | Missing go.mod dependencies | BUILD FAILURE | `go.mod` |
| C6 | Cancellation fails to invalidate pending callbacks | CRITICAL | `adapter.go` |

### 1.3 High-Priority Defects (Must Fix)

| ID | Defect | Severity | Location |
|----|--------|----------|----------|
| H1 | Context leak in JSLeafAdapter | HIGH | `adapter.go:71` |
| H2 | Unbounded Promise lifetime (no timeout) | HIGH | `adapter.go:173` |
| H3 | Require module registry inaccessible | HIGH | `require.go` |
| H4 | Manager type completely untested | HIGH | `require.go:24-44` |
| H5 | Non-deterministic tests (timing-based) | HIGH | `*_test.go` |
| H6 | Unsafe type assertions in tests | HIGH | `integration_test.go` |
| H7 | Bridge.Done() channel missing | HIGH | `bridge.go` |

### 1.4 Additional Defects (All Must Be Addressed)

| ID | Defect | Location |
|----|--------|----------|
| A1 | Event loop termination leaves adapter stuck | `adapter.go` |
| A2 | Type impedance mismatch (Go int vs JS string) | `require.go` |
| A3 | newNode expects bt.Tick which JS cannot construct | `require.go` |
| A4 | exposeBlackboard type assertion brittle | `require.go` |
| A5 | bridge can be nil in Require | `require.go` |
| A6 | ctx parameter unused in Manager | `require.go` |
| A7 | Ignored errors from obj.Set | `blackboard.go` |
| A8 | Magic strings without constants | Multiple files |
| A9 | Error wrapping inconsistent (no %w) | Multiple files |
| A10 | Bridge shutdown race with callbacks | `bridge.go` |
| A11 | Test count mismatch with WIP.md | WIP.md |
| A12 | Goroutine leak verification absent | Tests |
| A13 | Performance claims unverified | doc.go |
| A14 | One-shot context trap undocumented | `adapter.go` |

---

## 2. Critical Concurrency Defects

### 2.1 C1: Stale Callback Overwrites State (Race Condition)

**Location:** `internal/builtin/btbridge/adapter.go`

**The Bug:** `JSLeafAdapter` has no mechanism to associate a callback with the specific execution that created it. When a running node is cancelled and immediately restarted, the pending JavaScript callback from the first execution will fire and overwrite the state of the second execution.

**Failure Scenario:**
1. `Tick` (Generation 1) calls `dispatchJS`. State → `Running`.
2. `Cancel` is invoked. `Tick` resets state → `Idle`.
3. `Tick` (Generation 2) calls `dispatchJS`. State → `Running`.
4. **BUG:** JS Promise from Generation 1 resolves. Callback calls `finalize`. State → `Completed`. `lastStatus` contains Gen 1 result.
5. `Tick` (Generation 2 context) sees `Completed`, returns **stale result** from Gen 1.

**Impact:** Severe logic errors. A "MoveTo" action could succeed instantly because a previous cancelled "MoveTo" finished. Tree execution becomes non-deterministic.

**Required Fix:** Implement generation counting:
- Add `generation uint64` field to `JSLeafAdapter`
- Increment on every `dispatchJS` call
- Capture generation in closure
- Verify generation matches in `finalize`; discard stale callbacks

**This fix is mandatory. Without generation counting, the adapter is fundamentally broken.**

### 2.2 C2: Tick() Race Condition (Double Dispatch)

**Location:** `adapter.go:86-109`

**The Bug:** The mutex is released before the state transition occurs:

```go
func (a *JSLeafAdapter) Tick(children []bt.Node) (bt.Status, error) {
    a.mu.Lock()
    currentState := a.state
    a.mu.Unlock()  // ← UNLOCKED HERE

    switch currentState {
    case StateIdle:
        // ← RACE WINDOW: Another goroutine reads StateIdle
        a.dispatchJS()  // ← Both goroutines dispatch
```

**Proof:** Thread A reads `StateIdle`, unlocks. Thread B reads `StateIdle`, unlocks. Both call `dispatchJS()`. Both callbacks will call `finalize()` and overwrite each other.

**Impact:** Double execution of JavaScript code, state corruption, unpredictable behavior.

**Documentation Contradiction:** Line 36-37 claims "Thread Safety: The adapter is safe for concurrent access" — **this is false**.

**Required Fix:** Transition state atomically before unlocking:

```go
case StateIdle:
    a.state = StateRunning  // ← Set BEFORE unlock
    a.generation++
    gen := a.generation
    a.mu.Unlock()
    a.dispatchJSWithGen(gen)
    return bt.Running, nil
```

**This fix is mandatory. The current implementation allows double dispatch under concurrent access.**

### 2.3 C6: Cancellation Fails to Invalidate Pending Callbacks

**Location:** `adapter.go` (StateRunning case in Tick)

**The Bug:** When cancellation occurs, the code resets state to `Idle` but does **NOT** increment the generation counter. The pending callback holds the old generation value, which still matches the current generation. The callback will execute and corrupt state.

**Failure Scenario:**
1. `dispatchJS` sets `generation = 1`
2. Callback closure captures `gen = 1`
3. Cancellation: `state = Idle` (generation still 1)
4. Callback fires, checks `gen (1) == generation (1)` → match
5. Callback sets `state = Completed` → **corruption**

**Required Fix:** Cancellation **MUST** increment generation:

```go
case StateRunning:
    select {
    case <-a.ctx.Done():
        a.mu.Lock()
        a.state = StateIdle
        a.generation++  // ← CRITICAL: Invalidate pending callback
        a.mu.Unlock()
        return bt.Failure, errors.New("execution cancelled")
    default:
    }
```

**This fix is mandatory. Cancellation without generation increment leaves the race condition open.**

---

## 3. Resource Management & Lifecycle Bugs

### 3.1 H1: Context Leak in JSLeafAdapter

**Location:** `adapter.go:71`

```go
childCtx, cancel := context.WithCancel(ctx)
adapter := &JSLeafAdapter{..., ctx: childCtx, cancel: cancel}
```

**The Bug:** The child context and cancel function are never cleaned up. The `Cancel()` method exists but:
- The tree manager does not know about this special method
- After tree completion, the context remains active until parent cancellation
- This leaks resources

**Required Fix:** Either:
1. Call `cancel()` in `finalize()` after terminal status, OR
2. Document mandatory manual cleanup in API docs, OR
3. Tie adapter lifecycle to bridge lifecycle

**This fix is mandatory. Resource leaks are unacceptable in production.**

### 3.2 H2: Unbounded Promise Lifetime

**Location:** `adapter.go:173`

**The Bug:** There is no timeout mechanism for JavaScript Promise resolution. A misbehaved JS leaf that never resolves its Promise permanently leaves the adapter in `StateRunning`.

**Impact:** 
- Goroutine leak (the Tick loop continues polling)
- State permanently stuck
- Memory leak
- Tree never completes

**Required Fix:** Add per-leaf timeout:

Option A: In `runLeaf` JavaScript helper:
```javascript
globalThis.runLeaf = function(fn, ctx, args, callback, timeoutMs = 30000) {
    const timer = setTimeout(() => callback("failure", "timeout"), timeoutMs);
    Promise.resolve()
        .then(() => fn(ctx, args))
        .then(
            (status) => { clearTimeout(timer); callback(String(status), null); },
            (err) => { clearTimeout(timer); callback("failure", String(err)); }
        );
};
```

Option B: In Go adapter via context with deadline.

**This fix is mandatory. Promise-finalization guarantees require explicit timeout handling.**

### 3.3 H7: Bridge.Done() Channel Missing

**Location:** `bridge.go`

**The Bug:** `BlockingJSLeaf` and other callers need to detect bridge termination while waiting. Currently, if the bridge stops after `RunOnLoop` returns true but before callback executes, callers block forever.

**Required Fix:** Add `Done()` method to Bridge:

```go
type Bridge struct {
    // ... existing fields ...
    ctx    context.Context
    cancel context.CancelFunc
}

func (b *Bridge) Done() <-chan struct{} {
    return b.ctx.Done()
}

func (b *Bridge) Stop() {
    // ... existing logic ...
    b.cancel()  // Signal Done channel
}
```

**This fix is mandatory. All blocking operations must be cancellable.**

### 3.4 A1: Event Loop Termination Leaves Adapter Stuck

**Location:** `adapter.go:144-146`

**The Bug:** If `bridge.Stop()` is called while adapter is in `StateRunning`:
1. `RunOnLoop` callback is queued but never executes
2. `finalize()` is never called
3. Adapter is stuck in `StateRunning` forever

**Current Code:** Only handles `RunOnLoop` returning false, not mid-execution stop.

**Required Fix:** Use `Bridge.Done()` channel (H7) combined with generation counting (C1). In `finalize`, also check bridge state.

**This fix is mandatory. Graceful shutdown requires proper cleanup.**

### 3.5 A10: Bridge Shutdown Race with Callbacks

**Location:** `bridge.go`

**The Bug:** `bridge.Stop()` is idempotent but `RunOnLoop` callbacks may be in flight when stopped. The `finalize()` path doesn't verify the bridge is still running, risking writes to a blackboard that might be disposed.

**Required Fix:** Add bridge state check in finalize or document that blackboard outlives bridge.

**This fix must be addressed.**

---

## 4. BlockingJSLeaf Defects

### 4.1 C3: BlockingJSLeaf Permanent Deadlock

**Location:** `adapter.go:268`

```go
ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
    // ... Promise setup ...
})
if !ok { return bt.Failure, errors.New("event loop terminated") }
r := <-ch  // ← BLOCKS FOREVER
```

**The Bug:** If `bridge.Stop()` executes after `RunOnLoop` returns `true` but before the Promise callback fires, the channel receive blocks indefinitely.

**Impact:** 
- Calling goroutine leaks
- Deadlock in production
- Violates "event-driven bridge" guarantee

**Required Fix:** Use select with Bridge.Done():

```go
select {
case r := <-ch:
    return r.status, r.err
case <-bridge.Done():
    return bt.Failure, errors.New("bridge stopped")
}
```

**This fix is mandatory. BlockingJSLeaf is unsafe without this.**

### 4.2 C4: BlockingJSLeaf Double-Send Bug

**Location:** `adapter.go` (BlockingJSLeaf implementation)

**The Bug:** Multiple code paths can send to the result channel:

```go
_, err := runLeafFn(...)
if err != nil {
    ch <- result{bt.Failure, err}
    // ← NO RETURN HERE
}
// callback may also send later
```

**Impact:**
- If `runLeafFn` returns error AND callback fires, two sends occur
- Second send blocks on event loop goroutine (buffer size 1, receiver already drained first)
- Event loop goroutine wedged forever
- Future event loop tasks never execute

**Required Fix:** Use `sync.Once` to guarantee single send:

```go
var once sync.Once
send := func(r result) {
    once.Do(func() { ch <- r })
}

// All paths use send():
callback := func(call goja.FunctionCall) goja.Value {
    send(result{mapJSStatus(statusStr), err})
    return goja.Undefined()
}

if err != nil {
    send(result{bt.Failure, err})
    return  // ← MUST RETURN
}
```

**This fix is mandatory. The double-send bug is a latent deadlock.**

### 4.3 BlockingJSLeaf: No Context/Timeout Support

**The Bug:** `BlockingJSLeaf` has no context parameter. If the JS Promise never settles:
- Goroutine blocks forever on `<-ch`
- No way to cancel from caller
- No timeout mechanism

**Required Fix:** Accept `context.Context` parameter:

```go
func BlockingJSLeafWithContext(ctx context.Context, bridge *Bridge, fnName string, getCtx func() any) bt.Node {
    // ... setup ...
    select {
    case r := <-ch:
        return r.status, r.err
    case <-ctx.Done():
        return bt.Failure, ctx.Err()
    case <-bridge.Done():
        return bt.Failure, errors.New("bridge stopped")
    }
}
```

**This fix is mandatory. All blocking operations must be cancellable.**

---

## 5. API Design Issues (Require Module)

### 5.1 H3: Require Module Registry Inaccessible

**Location:** `bridge.go:42`, `require.go`

**The Bug:** `Require()` returns `require.ModuleLoader` but the internal registry is never exposed:

```go
// bridge.go:42
reg := require.NewRegistry()  // Private, never exposed
```

Users **cannot register** the `osm:bt` module. The `Require()` function is dead code.

**Required Fix:** Either:
1. Expose the registry from Bridge, OR
2. Register `osm:bt` module internally during Bridge initialization, OR
3. Remove the dead code

**This fix is mandatory. Dead code that appears functional is a maintenance hazard.**

### 5.2 A3: newNode Expects bt.Tick Which JS Cannot Construct

**Location:** `require.go`

```go
tickVal := call.Arguments[0].Export()
tick, ok := tickVal.(bt.Tick)
```

**The Bug:** A JavaScript function exported from goja will NOT naturally `Export()` to a Go `bt.Tick`. JS callables export as `func(goja.FunctionCall) goja.Value`, which does not implement `bt.Tick`.

The only viable "tick" values are Go ticks injected into JS (e.g., `bt.Sequence`). This means `newNode` is only for assembling Go nodes inside JS, not for defining JS tick functions.

**Impact:** API is misleading. Users expecting to pass JS functions will get cryptic type assertion failures.

**Required Fix:** Either:
1. Rename to `newGoNode` and document limitation, OR
2. Accept JS callables and wrap them via goja function calls (more complex), OR
3. Provide clear error message when type assertion fails

**This fix must be addressed.**

### 5.3 A5: bridge Can Be nil in Require

**Location:** `require.go`

```go
_ = exports.Set("createLeafNode", func(fnName string) bt.Node {
    return NewJSLeafAdapter(bridge, fnName, nil)  // bridge may be nil
})
```

**The Bug:** If `bridge` is nil, `dispatchJS` will panic on `a.bridge.RunOnLoop`.

**Required Fix:** Validate `bridge != nil` at loader creation:

```go
func Require(ctx context.Context, bridge *Bridge) require.ModuleLoader {
    if bridge == nil {
        panic("btbridge: bridge cannot be nil")
    }
    // ...
}
```

**This fix must be addressed.**

### 5.4 A4: exposeBlackboard Type Assertion Brittle

**Location:** `require.go`

```go
bb, ok := call.Arguments[0].Export().(*Blackboard)
```

**The Bug:** Depending on value crossing semantics, you might not get `*Blackboard` back. Could be `map[string]any`, `goja.Object`, etc.

**Required Fix:** Document that only blackboards created via `createBlackboard()` work, or add defensive type handling.

**This fix must be addressed.**

### 5.5 A6: ctx Parameter Unused in Manager

**Location:** `require.go:24-44`

```go
type Manager struct {
    ctx context.Context  // ← Never used after construction
    // ...
}
```

**The Bug:** `ctx` is stored but never referenced. Either it should control Manager lifecycle or be removed.

**Required Fix:** Either use ctx for lifecycle management or remove it.

**This fix must be addressed.**

### 5.6 A2: Type Impedance Mismatch (Go int vs JS string)

**Location:** `require.go`

**The Bug:** The `Require` module exposes `bt.Sequence` (Go implementation). Go `Tick` returns `(int, error)`. `goja` converts to JS `number`. The embedded `bt.js` logic checks `if (status === "running")`. Since `1 !== "running"`, JS composites misinterpret Go `Running` as `Failure`.

**Impact:** Users cannot mix Go composites and JS composites reliably.

**Required Fix:** Either:
1. Wrap Go nodes to return string status, OR
2. Document this limitation explicitly, OR
3. Remove Go composite exposure from JS API

**This fix must be addressed under the Go-Centric architecture decision.**

---

## 6. Test Infrastructure Defects

### 6.1 H5: Non-Deterministic Tests (Timing-Based)

**Locations:** `adapter_test.go:18`, `integration_test.go:20`, multiple files

```go
func waitForCompletion(...) {
    ticker := time.NewTicker(5 * time.Millisecond)  // TIMING-DEPENDENT
    // ...
    case <-ctx.Done(): t.Fatal("timeout")
}
```

**The Bug:** The specification explicitly requires: *"tests MUST be deterministic, and they ALL must pass RELIABLY"*. Timeout-based helpers contradict this requirement.

**Additional Issues:**
- `TestBridge_WithContext` uses `time.Sleep(10 * time.Millisecond)` then asserts stopped
- `TestJSLeafAdapter_Cancellation` uses `setTimeout` in JS (may not exist in all goja configurations)
- Under CI load, timing-based tests become flaky

**Required Fix:** Replace all timing-based waits with synchronization primitives:
- Use channels signaled by the code under test
- Use externally-resolved Promises controlled from Go
- Use `sync.WaitGroup` or condition variables

**This fix is mandatory. Non-deterministic tests are explicitly disallowed.**

### 6.2 H6: Unsafe Type Assertions in Tests

**Location:** `integration_test.go:149`, `integration_test.go:284`

```go
count := bb.Get("counter").(int64)  // Panics if JS never ran
require.True(t, bb.Get("task1").(bool))  // Panics if nil
```

**The Bug:** Initial `bb.Set("counter", 0)` stores Go `int`, not `int64`. If JS doesn't execute (e.g., event loop issue), the assertion panics with a type error instead of a useful test failure.

**Required Fix:** Use safe type assertions:

```go
countVal := bb.Get("counter")
require.IsType(t, int64(0), countVal)
count := countVal.(int64)
```

Or use type switch:

```go
switch v := bb.Get("counter").(type) {
case int64:
    require.Equal(t, expected, v)
case int:
    require.Equal(t, expected, int64(v))
default:
    t.Fatalf("unexpected type %T", v)
}
```

**This fix is mandatory. Tests must fail gracefully with useful messages.**

### 6.3 A12: Goroutine Leak Verification Absent

**Location:** All test files

**The Bug:** No test verifies goroutine counts before and after execution. Combined with the deadlock bugs (C3, C4), leaked goroutines will go undetected.

**Required Fix:** Add `goleak` verification:

```go
import "go.uber.org/goleak"

func TestMain(m *testing.M) {
    goleak.VerifyTestMain(m)
}
```

Or manual verification:

```go
func TestNoGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()
    // ... run test ...
    after := runtime.NumGoroutine()
    require.Equal(t, before, after)
}
```

**This fix is mandatory. Goroutine leaks must be detected.**

### 6.4 H4: Manager Type Completely Untested

**Location:** `require.go:24-44`

**The Bug:** `NewManager`, `Close()`, and the entire "osm:bt" module loader are public API with **zero test coverage**.

**Impact:** Unknown correctness. May be broken. Dead code if never used.

**Required Fix:** Either:
1. Add comprehensive tests for Manager, OR
2. Remove Manager if unused

**This fix is mandatory. Untested public API is unacceptable.**

### 6.5 A11: Test Count Mismatch

**Location:** WIP.md

**The Bug:** WIP.md claims "27 passing tests". Actual count appears to be 30 tests.

**Required Fix:** Update documentation to match reality.

**This fix must be addressed.**

---

## 7. Error Handling & Edge Cases

### 7.1 A7: Ignored Errors from obj.Set

**Location:** `blackboard.go:99-104`

```go
_ = obj.Set("get", b.Get)
_ = obj.Set("set", b.Set)
```

**The Bug:** `goja.Object.Set` returns an error which is discarded. While unlikely to fail for valid method names, this pattern is repeated 20+ times.

**Required Fix:** Either handle errors or add comment explaining why they're safe to ignore:

```go
// Set cannot fail for these keys as they are valid identifiers
_ = obj.Set("get", b.Get)
```

Or propagate errors:

```go
if err := obj.Set("get", b.Get); err != nil {
    return nil, fmt.Errorf("failed to expose get: %w", err)
}
```

**This fix must be addressed.**

### 7.2 A8: Magic Strings Without Constants

**Locations:** `adapter.go`, `bridge.go`, `require.go`

**The Bug:** Status strings ("running", "success", "failure") are hardcoded across multiple files without shared constants.

```go
// adapter.go
case "running": return bt.Running

// bridge.go
globalThis.bt = { running: "running", ... }
```

**Required Fix:** Define constants:

```go
const (
    StatusRunning = "running"
    StatusSuccess = "success"
    StatusFailure = "failure"
)
```

**This fix must be addressed.**

### 7.3 A9: Error Wrapping Inconsistent

**Locations:** Multiple files

**The Bug:** Most errors use `errors.New()` or `fmt.Errorf()` without `%w` wrapping, hindering error inspection with `errors.Is` and `errors.As`.

```go
// Current:
return bt.Failure, fmt.Errorf("panic in JS leaf %s: %v", a.fnName, r)

// Should be:
return bt.Failure, fmt.Errorf("panic in JS leaf %s: %w", a.fnName, panicErr)
```

**Required Fix:** Use `%w` for wrapped errors where appropriate.

**This fix must be addressed.**

### 7.4 A14: One-Shot Context Trap Undocumented

**Location:** `adapter.go`

**The Bug:** `NewJSLeafAdapterWithContext` derives a child context. Once `Cancel()` is called, the context remains cancelled forever. The node will always return `Failure` and cannot be reused.

This is standard `context` behavior but surprising for BT users who expect nodes to be resettable.

**Required Fix:** Document this limitation explicitly in API docs:

```go
// NewJSLeafAdapterWithContext creates a JSLeafAdapter with cancellation support.
// IMPORTANT: Once cancelled, this specific Node instance cannot be reused effectively
// as the context remains cancelled. Create a new Node for retries.
func NewJSLeafAdapterWithContext(...)
```

**This fix must be addressed.**

---

## 8. Documentation & Claims Verification

### 8.1 Premature "Ready for Release" Declaration

**Location:** WIP.md

**The Bug:** WIP.md states "Status: COMPLETE ✅ Ready for release" while:
- Critical race conditions exist (C1, C2, C6)
- BlockingJSLeaf deadlock exists (C3, C4)
- Tests are non-deterministic (H5)
- Manager is untested (H4)
- Performance claims unverified (no benchmarks)
- Dead code exists (Require module)

**Required Fix:** Update status to reflect actual state. Remove "Ready for release" claim.

**This fix is mandatory.**

### 8.2 Thread Safety Documentation False

**Location:** `adapter.go:36-37`

**The Bug:** Documentation claims "Thread Safety: The adapter is safe for concurrent access" but the implementation has race conditions (C2).

**Required Fix:** Either fix the races OR update documentation to reflect actual thread safety guarantees.

**This fix is mandatory.**

### 8.3 A13: Performance Claims Unverified

**Location:** `doc.go:91`

**Claim:** "~50-200µs per leaf execution" overhead.

**The Bug:** No benchmarks are provided. This claim cannot be verified.

**Required Fix:** Either:
1. Add benchmarks to verify claim, OR
2. Remove specific latency claims

**This fix must be addressed.**

### 8.4 bt.js Compatibility Deviation Undocumented

**Location:** `doc.go`

**The Bug:** The specification emphasizes "bt.js API Fidelity" as a goal. The implementation abandons bt.js composites entirely, exposing only leaf execution. This deviation is mentioned in WIP.md but not in the package documentation.

**Required Fix:** Document this clearly in `doc.go`:

```go
// This package implements Variant C.2 (Go-centric with JS leaves).
// Full bt.js composite compatibility is NOT provided.
// Use go-behaviortree composites (Sequence, Selector, etc.) in Go.
// JavaScript is used only for leaf behaviors.
```

**This fix must be addressed.**

### 8.5 .deadcodeignore Modifications

**Location:** `.deadcodeignore`

**The Bug:** The diff adds patterns to `.deadcodeignore` to suppress warnings rather than fixing dead code. This masks potential issues.

**Required Fix:** Verify each ignored pattern is intentional and document why in comments.

**This fix must be addressed.**

---

## 9. Missing Dependencies & Build Issues

### 9.1 C5: Missing go.mod Dependencies

**Location:** `go.mod`

**The Bug:** The source code imports `github.com/dop251/goja` and `github.com/dop251/goja_nodejs`, but these are missing from the `require` block.

**Impact:** Build fails immediately.

**Required Fix:** Add dependencies:

```go
require (
    // ... existing ...
    github.com/dop251/goja v0.0.0-20231027120936-3c313456ea40
    github.com/dop251/goja_nodejs v0.0.0-20231102000113-582965616336
)
```

**This fix is mandatory. The build cannot succeed without this.**

---

## 10. Required Corrections

### 10.1 JSLeafAdapter Complete Fix

Apply these changes to `internal/builtin/btbridge/adapter.go`:

**Add generation field:**
```go
type JSLeafAdapter struct {
    // ... existing fields ...
    generation uint64  // Prevents stale callbacks from corrupting state
}
```

**Fix Tick() for atomic state transition and generation:**
```go
func (a *JSLeafAdapter) Tick(children []bt.Node) (bt.Status, error) {
    a.mu.Lock()
    currentState := a.state
    
    switch currentState {
    case StateIdle:
        select {
        case <-a.ctx.Done():
            a.mu.Unlock()
            return bt.Failure, errors.New("execution cancelled")
        default:
        }
        a.state = StateRunning  // ATOMIC: Set before unlock
        a.generation++
        gen := a.generation
        a.mu.Unlock()
        a.dispatchJSWithGen(gen)
        return bt.Running, nil

    case StateRunning:
        select {
        case <-a.ctx.Done():
            a.state = StateIdle
            a.generation++  // CRITICAL: Invalidate pending callback
            a.mu.Unlock()
            return bt.Failure, errors.New("execution cancelled")
        default:
        }
        a.mu.Unlock()
        return bt.Running, nil

    case StateCompleted:
        status, err := a.lastStatus, a.lastError
        a.state = StateIdle
        a.lastStatus = 0
        a.lastError = nil
        a.mu.Unlock()
        return status, err
    }
    
    a.mu.Unlock()
    return bt.Failure, errors.New("invalid async state")
}
```

**Update dispatchJS to accept and use generation:**
```go
func (a *JSLeafAdapter) dispatchJSWithGen(gen uint64) {
    ok := a.bridge.RunOnLoop(func(vm *goja.Runtime) {
        defer func() {
            if r := recover(); r != nil {
                a.finalize(gen, bt.Failure, fmt.Errorf("panic: %v", r))
            }
        }()
        // ... existing function lookup ...
        
        callback := func(call goja.FunctionCall) goja.Value {
            statusStr := call.Argument(0).String()
            var err error
            if !goja.IsNull(call.Argument(1)) && !goja.IsUndefined(call.Argument(1)) {
                err = fmt.Errorf("%s", call.Argument(1).String())
            }
            a.finalize(gen, mapJSStatus(statusStr), err)
            return goja.Undefined()
        }
        // ... rest of implementation ...
    })

    if !ok {
        a.finalize(gen, bt.Failure, errors.New("event loop terminated"))
    }
}
```

**Update finalize to verify generation:**
```go
func (a *JSLeafAdapter) finalize(gen uint64, status bt.Status, err error) {
    a.mu.Lock()
    defer a.mu.Unlock()
    
    // Discard stale callbacks from cancelled/previous runs
    if gen != a.generation {
        return
    }
    
    a.lastStatus = status
    a.lastError = err
    a.state = StateCompleted
}
```

### 10.2 BlockingJSLeaf Complete Fix

```go
func BlockingJSLeaf(bridge *Bridge, fnName string, getCtx func() any) bt.Node {
    return BlockingJSLeafWithContext(context.Background(), bridge, fnName, getCtx)
}

func BlockingJSLeafWithContext(ctx context.Context, bridge *Bridge, fnName string, getCtx func() any) bt.Node {
    return func() (bt.Tick, []bt.Node) {
        return func(children []bt.Node) (bt.Status, error) {
            type result struct {
                status bt.Status
                err    error
            }
            ch := make(chan result, 1)
            var once sync.Once
            send := func(r result) {
                once.Do(func() { ch <- r })
            }

            ok := bridge.RunOnLoop(func(vm *goja.Runtime) {
                defer func() {
                    if r := recover(); r != nil {
                        send(result{bt.Failure, fmt.Errorf("panic: %v", r)})
                    }
                }()

                fnVal := vm.Get(fnName)
                if _, ok := goja.AssertFunction(fnVal); !ok {
                    send(result{bt.Failure, fmt.Errorf("function '%s' not callable", fnName)})
                    return  // MUST RETURN
                }

                runLeafVal := vm.Get("runLeaf")
                runLeafFn, ok := goja.AssertFunction(runLeafVal)
                if !ok {
                    send(result{bt.Failure, errors.New("runLeaf helper not found")})
                    return  // MUST RETURN
                }

                callback := func(call goja.FunctionCall) goja.Value {
                    statusStr := call.Argument(0).String()
                    var err error
                    if arg1 := call.Argument(1); !goja.IsNull(arg1) && !goja.IsUndefined(arg1) {
                        err = fmt.Errorf("%s", arg1.String())
                    }
                    send(result{mapJSStatus(statusStr), err})
                    return goja.Undefined()
                }

                var ctxVal goja.Value
                if getCtx != nil {
                    ctxVal = vm.ToValue(getCtx())
                } else {
                    ctxVal = goja.Undefined()
                }

                _, err := runLeafFn(
                    goja.Undefined(),
                    fnVal,
                    ctxVal,
                    goja.Undefined(),
                    vm.ToValue(callback),
                )
                if err != nil {
                    send(result{bt.Failure, err})
                    return  // MUST RETURN
                }
            })

            if !ok {
                return bt.Failure, errors.New("event loop terminated")
            }

            // Wait with cancellation support
            select {
            case r := <-ch:
                return r.status, r.err
            case <-ctx.Done():
                return bt.Failure, ctx.Err()
            case <-bridge.Done():
                return bt.Failure, errors.New("bridge stopped")
            }
        }, nil
    }
}
```

### 10.3 Bridge.Done() Implementation

Add to `internal/builtin/btbridge/bridge.go`:

```go
type Bridge struct {
    loop    *eventloop.EventLoop
    mu      sync.RWMutex
    started bool
    stopped bool
    
    // Lifecycle context
    ctx    context.Context
    cancel context.CancelFunc
}

func NewBridgeWithContext(ctx context.Context) (*Bridge, error) {
    reg := require.NewRegistry()
    loop := eventloop.NewEventLoop(
        eventloop.WithRegistry(reg),
        eventloop.EnableConsole(true),
    )

    childCtx, cancel := context.WithCancel(context.Background())
    b := &Bridge{
        loop:   loop,
        ctx:    childCtx,
        cancel: cancel,
    }
    // ... rest of initialization ...
}

func (b *Bridge) Done() <-chan struct{} {
    return b.ctx.Done()
}

func (b *Bridge) Stop() {
    b.mu.Lock()
    if b.stopped {
        b.mu.Unlock()
        return
    }
    b.stopped = true
    b.mu.Unlock()
    
    b.cancel()  // Signal Done channel BEFORE stopping loop
    b.loop.Stop()
}
```

### 10.4 go.mod Fix

Add to `go.mod`:

```go
require (
    // ... existing dependencies ...
    github.com/dop251/goja v0.0.0-20231027120936-3c313456ea40
    github.com/dop251/goja_nodejs v0.0.0-20231102000113-582965616336
)
```

### 10.5 Status Constants

Add to `internal/builtin/btbridge/adapter.go`:

```go
const (
    JSStatusRunning = "running"
    JSStatusSuccess = "success"
    JSStatusFailure = "failure"
)

func mapJSStatus(s string) bt.Status {
    switch s {
    case JSStatusRunning:
        return bt.Running
    case JSStatusSuccess:
        return bt.Success
    case JSStatusFailure:
        return bt.Failure
    default:
        return bt.Failure
    }
}
```

Update JS helper to use same constants:

```go
fmt.Sprintf(`globalThis.bt = {
    running: %q,
    success: %q,
    failure: %q
};`, JSStatusRunning, JSStatusSuccess, JSStatusFailure)
```

---

## 11. Implementation Verification Checklist

**The following checklist MUST be completed. Every item MUST pass. No exceptions.**

### 11.1 Critical Fixes (ALL MANDATORY)

- [ ] C1: Generation counter implemented in JSLeafAdapter
- [ ] C2: State transition atomic (before unlock) in Tick()
- [ ] C3: BlockingJSLeaf uses select with bridge.Done()
- [ ] C4: BlockingJSLeaf uses sync.Once for single send
- [ ] C5: go.mod includes goja dependencies
- [ ] C6: Cancellation increments generation counter

### 11.2 High-Priority Fixes (ALL MANDATORY)

- [ ] H1: Context leak addressed (cancel called or documented)
- [ ] H2: Promise timeout implemented
- [ ] H3: Require module registered or dead code removed
- [ ] H4: Manager tests added or Manager removed
- [ ] H5: All tests use deterministic synchronization (no time.Sleep waits)
- [ ] H6: All type assertions use safe patterns
- [ ] H7: Bridge.Done() channel implemented

### 11.3 Additional Fixes (ALL MANDATORY)

- [ ] A1: Event loop termination handling verified
- [ ] A2: Type impedance documented or fixed
- [ ] A3: newNode limitation documented or fixed
- [ ] A4: exposeBlackboard type handling improved
- [ ] A5: bridge nil check added to Require
- [ ] A6: Manager ctx parameter used or removed
- [ ] A7: obj.Set errors handled or documented
- [ ] A8: Status constants defined and used
- [ ] A9: Error wrapping uses %w where appropriate
- [ ] A10: Bridge shutdown race addressed
- [ ] A11: Test count in WIP.md corrected
- [ ] A12: Goroutine leak testing added
- [ ] A13: Performance claims verified or removed
- [ ] A14: One-shot context trap documented

### 11.4 Test Cases (ALL MUST PASS RELIABLY)

- [ ] Leaf returns success immediately
- [ ] Leaf returns failure immediately
- [ ] Leaf returns running then success
- [ ] Leaf returns running multiple times then failure
- [ ] Leaf throws exception → failure
- [ ] Leaf Promise rejects → failure
- [ ] Tree cancelled mid-execution → failure, no state corruption
- [ ] Event loop terminated while nodes pending → clean failure
- [ ] Concurrent ticks on same tree → no race (or documented single-threaded)
- [ ] Concurrent ticks on different trees (single runtime)
- [ ] Memory leak testing (long-running simulation)
- [ ] Panic recovery in JS callback
- [ ] **NEW:** Cancelled node restarted immediately → correct result (not stale)
- [ ] **NEW:** BlockingJSLeaf with bridge.Stop() mid-execution → clean failure
- [ ] **NEW:** BlockingJSLeaf with context cancellation → clean failure
- [ ] **NEW:** Goroutine count before == goroutine count after

### 11.5 Documentation Updates (ALL MANDATORY)

- [ ] Remove "Ready for release" from WIP.md
- [ ] Fix thread safety claims in adapter.go
- [ ] Document one-shot context behavior
- [ ] Document bt.js compatibility limitations
- [ ] Document or remove performance claims
- [ ] Update test count in WIP.md

---

## Final Verdict

**This PR requires significant corrections before merge.** The core design is sound (Variant C.2 architecture, event loop usage, Go-owned blackboard), but the implementation contains critical concurrency bugs that will cause data corruption and deadlocks in production.

**Minimum requirements for merge approval:**
1. All items in Section 11.1 (Critical Fixes) completed
2. All items in Section 11.2 (High-Priority Fixes) completed  
3. All items in Section 11.4 (Test Cases) passing reliably
4. All items in Section 11.5 (Documentation Updates) completed

**All fixes are mandatory. All tests must pass reliably. All documentation must be accurate. No exceptions. No deferrals. No "minor" issues. Complete everything.**
