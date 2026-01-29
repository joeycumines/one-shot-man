# Exhaustive Code Review: Scripting Engine Module

**Review Date:** 2026-01-30  
**Reviewer:** Takumi  
**Module:** `internal/scripting/`  
**Focus:** Deadlocks on stop, thread safety, goroutine leaks, event loop safety

---

## Executive Summary

The scripting engine module is the core JavaScript runtime integration layer. After exhaustive review of all files, I have identified **1 CRITICAL**, **3 HIGH**, **4 MEDIUM**, and **5 LOW** severity issues.

The CRITICAL issue is a **potential deadlock during shutdown** in the interaction between `Engine.Close()`, `Runtime.Close()`, and any pending `RunOnLoopSync` operations. The HIGH issues relate to event loop safety violations and potential goroutine leaks.

---

## CRITICAL Issues

### CRIT-1: Shutdown Ordering - Potential 5-Second Hang

**File:** `engine_core.go:360-396`  
**Severity:** CRITICAL (downgraded from DEADLOCK to HANG due to timeout protection)  
**Type:** Shutdown Hang

**Problem:**

The `Engine.Close()` method calls components in an order that can cause up to 5-second hang per stuck operation:

```go
func (e *Engine) Close() error {
    // 1. Close TUI manager (which closes StateManager and stops writer goroutine)
    if e.tuiManager != nil {
        if err := e.tuiManager.Close(); err != nil { ... }
    }
    // 2. Stop BT bridge (stops internal bt.Manager)
    if e.btBridge != nil {
        e.btBridge.Stop()
    }
    // 3. Close bubblezone manager
    if e.bubblezoneManager != nil {
        e.bubblezoneManager.Close()
    }
    // 4. Close runtime (stops event loop)
    if e.runtime != nil {
        e.runtime.Close()
    }
    ...
}
```

**The Hang Scenario:**

1. A BT ticker is in `RunOnLoopSync()` past the initial state check
2. It has scheduled work on the event loop via `b.loop.RunOnLoop()`
3. It's waiting in the select for: `errCh`, `b.Done()`, or 5-second timeout
4. `Engine.Close()` is called which calls `btBridge.Stop()`:
   - Sets `b.stopped = true`
   - Calls `b.manager.Stop()` which waits for ticker to complete
   - **BUT** `b.cancel()` is NOT called until AFTER `b.manager.Stop()` returns
5. The ticker's `RunOnLoopSync` won't see `b.Done()` fire because `b.cancel()` hasn't been called
6. The ticker waits for either `errCh` (event loop result) or 5-second timeout
7. **HANG**: If event loop is busy, shutdown hangs for 5 seconds

**Mitigating Factor:**

Both `Runtime` (runtime.go:64) and `Bridge` (bridge.go:57) have `DefaultSyncTimeout = 5 * time.Second`. This prevents indefinite deadlock but introduces unacceptable shutdown delay.

**Evidence:**

- `bt/bridge.go:240-258`: `Bridge.Stop()` calls `b.manager.Stop()` BEFORE `b.cancel()`
- `bt/bridge.go:337-351`: `RunOnLoopSync` waits on `b.Done()` which fires on `b.cancel()`
- The order means `b.Done()` never fires during `b.manager.Stop()` wait

**Fix Recommendation:**

Call `cancel()` BEFORE stopping dependent components:

```go
// In Bridge.Stop():
func (b *Bridge) Stop() {
    b.mu.Lock()
    if b.stopped {
        b.mu.Unlock()
        return
    }
    b.stopped = true
    b.mu.Unlock()

    // Cancel context FIRST to unblock any pending RunOnLoopSync
    b.cancel()
    
    // NOW stop the manager - pending calls will fail fast
    if b.manager != nil {
        b.manager.Stop()
    }
}
```

Same fix needed in `Engine.Close()` - cancel runtime context before stopping btBridge.

---

## HIGH Issues

### HIGH-1: Engine.SetGlobal/GetGlobal Bypass Event Loop

**File:** `engine_core.go:195-220`  
**Severity:** HIGH  
**Type:** Thread Safety Violation

**Problem:**

`Engine.SetGlobal()` and `Engine.GetGlobal()` directly access `e.vm` without going through the event loop:

```go
func (e *Engine) SetGlobal(name string, value interface{}) {
    e.globals[name] = value
    e.vm.Set(name, value)  // DIRECT VM ACCESS
}
```

The comments acknowledge this limitation but the methods are still public and callable from any goroutine.

**Risk:** Data race if called concurrently with script execution.

**Recommendation:**

1. Either make these methods private and document they're for internal use only
2. Or route through `e.runtime.SetGlobal()` which uses `RunOnLoopSync`

---

### HIGH-2: ExecuteScript Bypasses Event Loop

**File:** `engine_core.go:237-278`  
**Severity:** HIGH  
**Type:** Thread Safety Violation

**Problem:**

`Engine.ExecuteScript()` directly calls `e.vm.RunString()` without going through the event loop:

```go
func (e *Engine) ExecuteScript(script *Script) (err error) {
    // ...
    if _, runErr := e.vm.RunString(script.Content); runErr != nil {
        return fmt.Errorf("script execution failed: %w", runErr)
    }
    // ...
}
```

The comment documents this is for "synchronous script execution" but doesn't enforce it.

**Risk:** If called from a goroutine other than the main/event-loop goroutine while async operations are running, data races occur.

**Recommendation:**

Add a build-time or runtime assertion that this is called from the expected goroutine, or route through `e.runtime.LoadScript()`.

---

### HIGH-3: StateManager Listener Callbacks Can Block State Updates

**File:** `state_manager.go:556-578`  
**Severity:** HIGH  
**Type:** Potential Deadlock

**Problem:**

`notifyListeners()` is called synchronously after releasing `sm.mu`, but listeners are invoked in sequence:

```go
func (sm *StateManager) notifyListeners(key string) {
    sm.listenerMu.RLock()
    listeners := make([]builtin.StateListener, 0, len(sm.listeners))
    for _, fn := range sm.listeners {
        listeners = append(listeners, fn)
    }
    sm.listenerMu.RUnlock()
    
    for _, fn := range listeners {
        fn(key)  // SYNCHRONOUS - if slow, blocks all subsequent notifications
    }
}
```

The comment in `AddListener()` warns about this, but the implementation in `engine_core.go:153-158` spawns a goroutine:

```go
engine.tuiManager.stateManager.AddListener(func(key string) {
    go bubbleteaMgr.SendStateRefresh(key)  // Goroutine spawn
})
```

This is CORRECT but:
1. The pattern is not enforced
2. Other listeners added elsewhere may not follow this pattern

**Risk:** A slow listener can delay state updates for all other listeners.

**Recommendation:**

Consider always spawning goroutines for listener invocations in `notifyListeners()` itself, or document more prominently.

---

## MEDIUM Issues

### MED-1: TUIManager.scheduleWriteAndWait - No Timeout

**File:** `tui_manager.go:166-183`  
**Severity:** MEDIUM  
**Type:** Potential Hang

**Problem:**

`scheduleWriteAndWait()` waits indefinitely on `resultCh` or shutdown signals, but there's no timeout:

```go
select {
case err := <-resultCh:
    return err
case <-tm.writerStop:
    return errors.New("manager shutting down")
case <-tm.writerDone:
    return errors.New("manager shutting down")
}
```

If the writer goroutine hangs processing a task, this will hang forever.

**Recommendation:** Add a configurable timeout with sensible default.

---

### MED-2: ContextManager Mutex Contention

**File:** `context.go`  
**Severity:** MEDIUM  
**Type:** Performance

**Problem:**

`ContextManager` uses a single `sync.RWMutex` for all operations. `ToTxtar()` reads files from disk while holding the read lock:

```go
func (cm *ContextManager) ToTxtar() *txtar.Archive {
    cm.mutex.RLock()
    defer cm.mutex.RUnlock()
    
    // ...
    for k, cp := range cm.paths {
        // ...
        data, err := os.ReadFile(absPath)  // DISK I/O under lock
        // ...
    }
}
```

**Risk:** Slow disk I/O blocks other read operations.

**Recommendation:** Read file list under lock, release lock, then read files, re-acquire lock only if needed for mutations.

---

### MED-3: TUILogger.PrintToTUI - Potential Blocking

**File:** `logging.go:131-144`  
**Severity:** MEDIUM  
**Type:** Potential Hang

**Problem:**

`PrintToTUI()` holds a read lock while potentially writing to the TUI sink:

```go
func (l *TUILogger) PrintToTUI(msg string) {
    // ...
    l.sinkMu.RLock()
    defer l.sinkMu.RUnlock()
    
    if l.tuiSink != nil {
        l.tuiSink(msg)  // Could block if sink is slow
        return
    }
    // ...
}
```

If `tuiSink` blocks, it holds `sinkMu.RLock()` preventing `SetTUISink()` from completing.

**Recommendation:** The code comment justifies holding the lock, but the sink callback should be guaranteed non-blocking or have a timeout.

---

### MED-4: Terminal State Cleanup on Panic

**File:** `terminal.go:31-73`  
**Severity:** MEDIUM  
**Type:** Resource Leak

**Problem:**

`Terminal.Run()` saves terminal state and sets up signal handling correctly, but if a panic occurs in the signal handling goroutine before the TUI starts, terminal state may not be restored:

```go
go func() {
    defer close(done)
    t.tuiManager.Run()  // If this panics after terminal state changes
}()
```

The terminal state restoration happens after the `done` channel receives, but panics cause goroutine termination without defer execution in the parent.

**Recommendation:** Add panic recovery in the goroutine itself:

```go
go func() {
    defer close(done)
    defer func() {
        if r := recover(); r != nil {
            // Log but don't propagate - let main goroutine restore terminal
        }
    }()
    t.tuiManager.Run()
}()
```

---

## LOW Issues

### LOW-1: TUILogHandler - Unlock Before File Handler Call

**File:** `logging.go:72-91`  
**Severity:** LOW  
**Type:** Documentation

The code unlocks before calling file handler with a comment explaining why. This is correct but could be more explicit about the thread safety guarantees.

---

### LOW-2: Unused debugLockState Field in Release Builds

**File:** `tui_types.go:53`  
**Severity:** LOW  
**Type:** Code Quality

The `debugLockState` field exists in all builds but is only used in debug builds. The stub file works around staticcheck warnings with `var _ = ...` patterns which is somewhat hacky.

**Recommendation:** Use conditional compilation for the field itself, or document why it must exist in all builds.

---

### LOW-3: Magic Numbers in History Buffer

**File:** `state_manager.go:14`  
**Severity:** LOW  
**Type:** Maintainability

`maxHistoryEntries = 200` is defined as a package-level constant but not documented why 200 was chosen.

---

### LOW-4: Inconsistent Error Handling in jsCreateState

**File:** `js_state_accessor.go:41-150`  
**Severity:** LOW  
**Type:** API Consistency

`jsCreateState` uses `panic(runtime.NewTypeError(...))` for validation errors. While this is consistent with JS semantics (throwing), it differs from the error return pattern used elsewhere in the codebase.

---

### LOW-5: ContextManager.RemovePath Complexity

**File:** `context.go:187-274`  
**Severity:** LOW  
**Type:** Maintainability

The `RemovePath` function has multiple fallback resolution strategies making it complex. Consider breaking into smaller helper functions.

---

## Verified Safe Patterns

The following patterns were verified as correctly implemented:

1. **Writer Goroutine Pattern** (`tui_manager.go`): The "Signal, Don't Close" pattern for `writerStop` channel is correctly implemented, preventing panics from racing senders.

2. **TryRunOnLoopSync Goroutine ID Check** (`runtime.go`, `bt/bridge.go`): The goroutine ID check using `runtime.Stack()` parsing is done once at initialization and reused efficiently via atomic operations.

3. **StateManager Dual-Lock Strategy**: Using `sm.mu` for state and `sm.listenerMu` for listeners with proper unlock-before-notify ordering.

4. **Debug Assertions Build Tags**: Properly guarded with `//go:build debug` and stub implementations for release builds.

5. **TUIReader/TUIWriter Lazy Initialization**: Thread-safe with `sync.Once` and proper `isStdin`/`isStdout` tracking.

---

## Concurrency Model Summary

### Goroutines Spawned

| Component | Goroutine | Lifecycle | Stop Mechanism |
|-----------|-----------|-----------|----------------|
| Engine | Event Loop (via Runtime) | Engine lifetime | `runtime.Close()` → `loop.Stop()` |
| TUIManager | Writer Goroutine | TUIManager lifetime | `stopWriter()` closes `writerStop` |
| StateManager | None (listeners invoked synchronously) | N/A | N/A |
| BT Bridge | None directly (uses event loop) | Bridge lifetime | `Stop()` → `manager.Stop()` |
| Terminal | Signal handler goroutine | Terminal.Run() | Cleanup via `signal.Stop()` |

### Channels

| Channel | Type | Closed By | Pattern |
|---------|------|-----------|---------|
| `writerQueue` | `chan writeTask` | **NEVER** | "Signal, Don't Close" |
| `writerStop` | `chan struct{}` | `stopWriter()` | Signal channel |
| `writerDone` | `chan struct{}` | Writer goroutine | Completion signal |
| `rt.ctx.Done()` | `<-chan struct{}` | `rt.cancel()` | Context cancellation |

---

## Recommendations Summary

### Must Fix (CRITICAL + HIGH)

1. **CRIT-1**: Reorder `Engine.Close()` to cancel runtime context before stopping dependent components
2. **HIGH-1**: Make `Engine.SetGlobal`/`GetGlobal` private or route through event loop
3. **HIGH-2**: Add documentation/assertion for `ExecuteScript` thread safety
4. **HIGH-3**: Consider spawning goroutines for all listener invocations

### Should Fix (MEDIUM)

1. **MED-1**: Add timeout to `scheduleWriteAndWait`
2. **MED-2**: Reduce lock scope in `ContextManager.ToTxtar`
3. **MED-3**: Document non-blocking requirement for TUI sink
4. **MED-4**: Add panic recovery in Terminal goroutine

---

## Conclusion

The scripting engine is well-architected with proper attention to thread safety patterns. The deadlock regression tests (`tui_deadlock_regression_test.go`) demonstrate awareness of concurrency issues. However, the CRITICAL shutdown ordering issue represents a real risk that needs immediate attention.

The module would benefit from:
1. A formal shutdown sequence document
2. Integration tests that specifically exercise shutdown under load
3. Race detector runs with concurrent shutdown scenarios

**Trust Declarations:**
- I have NOT verified the behavior of `goja_nodejs/eventloop.Stop()` - I trust its documentation that it waits for pending jobs.
- I have NOT verified `context.AfterFunc` behavior - I trust Go stdlib documentation.

---

*Review completed by Takumi - may my Gunpla collection remain safe.*
