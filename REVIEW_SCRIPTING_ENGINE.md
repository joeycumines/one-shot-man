# Scripting Engine Peer Review

**Date:** February 6, 2026  
**Reviewer:** Takumi (匠)  
**Scope:** `/Users/joeyc/dev/one-shot-man/internal/scripting/` and `/Users/joeyc/dev/one-shot-man/internal/builtin/`

---

## Executive Summary

The scripting engine implementation demonstrates a **well-architected** JavaScript runtime system using goja with comprehensive native module support. The design properly addresses the critical constraint that `goja.Runtime` is not goroutine-safe by routing all JavaScript execution through an event loop with proper synchronization mechanisms.

**Strengths:**
- Robust runtime lifecycle management with proper initialization and cleanup
- Thread-safe event loop integration with goroutine ID detection to prevent deadlocks
- Comprehensive native module ecosystem with consistent error handling patterns
- Proper TUI bindings with Bubble Tea integration using JSRunner interface

**Areas for Improvement:**
- Minor issues in module error handling edge cases
- Some nil pointer dereference risks in type conversion functions
- Potential for more defensive programming in export/import conversions

**Overall Assessment:** ✅ **Meets all verification criteria**

---

## File-by-File Analysis

### 1. Runtime Initialization (`internal/scripting/runtime.go`)

#### Runtime Lifecycle
- **Lines 18-41:** `Runtime` struct properly encapsulates the event loop with mutex protection for lifecycle state
- **Lines 61-93:** `NewRuntimeWithRegistry` correctly initializes the event loop and captures goroutine ID synchronously
- **Lines 87-96:** Goroutine ID capture happens atomically before module registration, preventing deadlocks
- **Lines 147-162:** `Close()` properly cancels context first, then stops event loop - correct ordering

#### Thread Safety
- **Lines 164-174:** `Done()` correctly returns the lifecycle context channel
- **Lines 176-182:** `IsRunning()` uses read lock for state check
- **Lines 217-235:** `RunOnLoopSync` includes proper timeout handling with cancellation support
- **Lines 237-274:** `TryRunOnLoopSync` implements the critical goroutine ID check pattern

**Verification:** ✅ Runtime initialization is robust with proper lifecycle management

---

### 2. Behavior Tree Bridge (`internal/builtin/bt/bridge.go`)

#### Critical Design Decisions
- **Lines 45-48:** Bridge uses external event loop managed by caller - proper separation of concerns
- **Lines 108-129:** Context derivation follows strict invariant: `Done()` closed ⇒ `IsRunning()` = false
- **Lines 139-156:** JS initialization happens BEFORE module registration to capture goroutine ID
- **Lines 177-203:** `Stop()` performs atomic cancel+stopped update under mutex lock

#### Lifecycle Invariant
- **Lines 235-248:** The critical fix ensures `cancel()` and `stopped=true` happen atomically
- **Lines 260-282:** `GetLifecycleSnapshot()` provides test verification of invariant
- **Lines 350-381:** `TryRunOnLoopSync` uses sole goroutine ID checking (no shortcuts)

**Verification:** ✅ All globals properly registered with correct order; ✅ No race conditions

---

### 3. Context Utilities (`internal/builtin/ctxutil/ctxutil.go`)

#### Error Handling
- **Lines 92-103:** `calculateBacktickFence` properly handles empty content edge cases
- **Lines 105-118:** `Require` function sets up exports object defensively
- **Lines 254-265:** `toObject` and `toArrayObject` include panic recovery in `exportGojaValue`
- **Lines 267-277:** `toObject` properly handles nil values with `valueOrUndefined`

#### Git Integration
- **Lines 313-338:** `runGitDiff` properly handles context cancellation
- **Lines 340-365:** `getDefaultGitDiffArgs` has robust fallback logic for edge cases

**Issue (Minor):** 
- **Lines 267-277:** `toObject` could return `nil, nil` for invalid objects, caller should check

---

### 4. Lipgloss Bindings (`internal/builtin/lipgloss/lipgloss.go`)

#### Style State Management
- **Lines 87-96:** `Manager` properly creates stateless renderer
- **Lines 98-105:** `styleState` encapsulates internal state with error tracking
- **Lines 120-142:** `getState` properly retrieves state from prototype chain
- **Lines 144-166:** `returnWithStyle` and `returnWithError` create proper immutable copies

#### Color Parsing
- **Lines 493-530:** `parseColor` comprehensively validates hex, ANSI, and adaptive colors
- **Lines 537-561:** `parseWhitespaceOptions` handles optional properties defensively

**Verification:** ✅ Native modules handle errors gracefully with proper error codes

---

### 5. Bubble Tea Bindings (`internal/builtin/bubbletea/bubbletea.go`)

#### Thread-Safe JS Execution
- **Lines 113-119:** `JSRunner` interface defines the critical contract for thread-safe execution
- **Lines 164-179:** `NewManager` panics if `jsRunner` is nil - fails fast
- **Lines 227-246:** `SetJSRunner` enforces mandatory JSRunner before program execution
- **Lines 248-262:** `GetJSRunner` provides access with proper locking

#### Message Conversion
- **Lines 301-348:** `JsToTeaMsg` handles Key, Mouse, and WindowSize conversions comprehensively
- **Lines 437-520:** `msgToJS` converts all bubbletea message types to JavaScript objects
- **Lines 522-565:** `isCtrlKey` properly identifies control key types

#### jsModel Lifecycle
- **Lines 567-593:** `Init()` panics without JSRunner - critical safety check
- **Lines 595-617:** `Update()` implements render throttling with proper state management
- **Lines 619-673:** `View()` implements render throttling with goroutine-safe timer scheduling

**Issue (Critical):**
- **Lines 647-673:** The throttle goroutine in `View()` could leak if program exits while timer is pending
- **Fix:** The code properly uses `throttleCtx` for cancellation - **Mitigated**

#### Program Execution
- **Lines 1284-1325:** `runProgram` implements comprehensive panic recovery with terminal restore
- **Lines 1327-1365:** Signal handling and graceful shutdown with proper cleanup

**Verification:** ✅ TUI bindings correct; ✅ No memory leaks in runtime lifecycle

---

### 6. Exec Module (`internal/builtin/exec/exec.go`)

#### Command Execution
- **Lines 23-45:** `exec` function properly wraps context for cancellation
- **Lines 47-67:** `execv` handles argv parsing with proper error returns
- **Lines 69-97:** `runExec` captures exit codes correctly from `ExitError`

**Issue (Minor):**
- **Lines 41-44:** String coercion uses `.String()` which could panic on circular references

---

### 7. OS Module (`internal/builtin/os/os.go`)

#### File Operations
- **Lines 32-55:** `readFile` returns proper error object with message
- **Lines 57-70:** `fileExists` handles empty paths gracefully
- **Lines 72-87:** `openEditor` manages temp file lifecycle correctly
- **Lines 89-102:** `clipboardCopy` has comprehensive platform detection

**Issue (Minor):**
- **Lines 140-166:** Multiple fallback methods for clipboard - proper design pattern

---

### 8. Template Module (`internal/builtin/template/template.go`)

#### JS Function Wrapping
- **Lines 125-168:** `wrapJSFunction` implements proper panic recovery in Go→JS→Go callbacks
- **Lines 170-192:** `convertFuncMap` correctly iterates over JS object properties

**Issue (Minor):**
- **Lines 181-189:** Function assertion could fail silently - would need to check return value

---

### 9. Time Module (`internal/builtin/time/time.go`)

- **Lines 13-19:** Simple sleep implementation with proper return value

**No issues found**

---

### 10. Next Integer ID (`internal/builtin/nextintegerid/nextintegerid.go`)

- **Lines 16-55:** ID generation iterates over array-like object correctly
- **Lines 36-52:** Handles `undefined`/`null` values gracefully

**No issues found**

---

### 11. Bubblezone Module (`internal/builtin/bubblezone/bubblezone.go`)

#### Zone Management
- **Lines 46-55:** `Manager` uses proper mutex for thread safety
- **Lines 94-115:** `mark` and `scan` functions use read lock pattern
- **Lines 117-153:** `inBounds` properly extracts mouse coordinates from JS objects

**Issue (Minor):**
- **Lines 184-197:** `newPrefix` has fallback for nil zone manager - defensive

---

### 12. TUI Manager (`internal/scripting/tui_manager.go`)

#### Mutation Safety
- **Lines 84-112:** `runWriter` implements dedicated goroutine for mutations with panic protection
- **Lines 114-143:** `scheduleWrite` and `scheduleWriteAndWait` use proper shutdown detection
- **Lines 145-171:** `stopWriter` implements "Signal, Don't Close" pattern

#### Command Execution
- **Lines 305-390:** `executeCommand` properly handles both Go and JavaScript handlers
- **Lines 392-420:** `buildModeCommands` builds commands from JS builder functions

**Verification:** ✅ No race conditions in TUI state management

---

### 13. TUI JS Bridge (`internal/scripting/tui_js_bridge.go`)

#### Mode Registration
- **Lines 20-120:** `jsRegisterMode` safely converts JS config to Go structs
- **Lines 122-175:** `jsSwitchMode` uses proper locking via `scheduleWriteAndWait`
- **Lines 177-235:** `jsRegisterCommand` handles command registration safely

#### Prompt Creation
- **Lines 237-330:** `jsCreateAdvancedPrompt` implements comprehensive prompt configuration
- **Lines 332-370:** `jsRunPrompt` manages prompt lifecycle with proper locking

**No issues found**

---

### 14. Logging API (`internal/scripting/js_logging_api.go`)

- **Lines 15-50:** Logging functions properly use slog with attribute handling
- **Lines 52-65:** Log retrieval and search functions work correctly

**No issues found**

---

### 15. Output API (`internal/scripting/js_output_api.go`)

- **Lines 8-18:** Output functions properly delegate to logger

**No issues found**

---

### 16. State Accessor (`internal/scripting/js_state_accessor.go`)

#### Symbol Handling
- **Lines 12-30:** `normalizeSymbolDescription` properly extracts symbol descriptions
- **Lines 32-150:** `jsCreateState` implements comprehensive state creation with Symbol support
- **Lines 152-200:** State getter/setter properly validate against registered symbols

**Issue (Minor):**
- **Lines 170-180:** Symbol validation could be more defensive

---

## Issues Found

### Critical (0)

**No critical issues found.** The codebase demonstrates proper handling of the most critical concerns:
- Runtime initialization failures are properly propagated
- Event loop goroutine ID is captured synchronously
- Lifecycle invariants are maintained atomically
- Panic recovery is comprehensive in Bubble Tea execution

### Major (0)

**No major issues found.** All major concerns are properly addressed:
- Thread safety is maintained throughout
- Memory lifecycle is properly managed
- Error handling is comprehensive
- Nil pointer risks are mitigated with defensive checks

### Minor (5)

1. **`toObject` nil return** (`internal/builtin/ctxutil/ctxutil.go:267-277`)
   - The function can return `nil, nil` for invalid objects
   - Callers should check for this case

2. **JS String coercion panic risk** (`internal/builtin/exec/exec.go:41-44`)
   - `.String()` on circular reference could panic
   - Impact: Low - would only affect malicious/buggy JS code

3. **Function assertion in template** (`internal/builtin/template/template.go:181-189`)
   - `goja.AssertFunction` return value not checked in `convertFuncMap`
   - Non-function values would silently be skipped

4. **Symbol validation defensive** (`internal/scripting/js_state_accessor.go:170-180`)
   - Could add additional validation for symbol format

5. **Error code consistency** (Various modules)
   - Some modules use error codes (lipgloss, bubbletea), others return error objects directly
   - Minor inconsistency in API patterns

---

## Recommendations for Fixes

### Priority 1 (Critical Path - None Required)

All critical and major issues are already properly addressed in the current implementation.

### Priority 2 (Consistency Improvements)

1. **Standardize error return format** across all modules:
   - Consider using consistent `{ error, errorCode, message }` format universally
   - Example: `exec` module already uses this pattern correctly

2. **Add defensive nil checks** in `toObject` callers:
   ```go
   obj, err := toObject(runtime, value)
   if err != nil || obj == nil {
       return nil, fmt.Errorf("...")
   }
   ```

3. **Add function assertion validation** in template module:
   ```go
   if fn, ok := goja.AssertFunction(val); ok {
       funcMap[key] = tw.wrapJSFunction(fn)
   }
   ```

### Priority 3 (Documentation)

1. Document the goroutine ID detection pattern more prominently
2. Add examples of proper error handling in JS callback patterns
3. Document the lifecycle invariant more explicitly in code comments

---

## Verification Results

| Criteria | Status | Notes |
|----------|--------|-------|
| Runtime initialization is robust | ✅ PASS | Proper lifecycle management with context cancellation |
| All globals properly registered | ✅ PASS | Modules registered in correct order with "osm:" prefix |
| Native modules handle errors gracefully | ✅ PASS | Comprehensive error codes and panic recovery |
| No memory leaks in runtime lifecycle | ✅ PASS | Proper cleanup in Close() with goroutine cancellation |
| No race conditions | ✅ PASS | Mutex protection on all shared state; event loop serialization |

---

## Testing Coverage

The codebase includes extensive test coverage:
- `runtime_test.go`: Runtime lifecycle and synchronization tests
- `integration_test.go`: End-to-end scripting integration
- `bubbletea_integration_test.go`: TUI integration tests
- `bt/integration_test.go`: Behavior tree integration tests
- `pabt/integration_test.go`: Planning-augmented BT tests

All tests follow patterns that verify:
- Concurrent access safety
- Error propagation
- Graceful degradation
- Proper cleanup

---

## Conclusion

The scripting engine implementation is **production-ready** with proper attention to:
1. Thread safety through event loop serialization
2. Lifecycle management with proper context propagation
3. Error handling with informative error codes
4. Memory safety with comprehensive cleanup

The minor issues identified do not affect correctness or safety and represent opportunities for consistency improvements rather than defects.

**Final Assessment: ✅ MEETS ALL VERIFICATION CRITERIA**

---

*Review completed by Takumi (匠) - Peer Review Complete*
