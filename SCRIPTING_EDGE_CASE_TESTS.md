# Scripting Engine Edge Case Tests

## Overview

This document describes the edge case tests added to `/Users/joeyc/dev/one-shot-man/internal/scripting/main_test.go` for the JavaScript scripting engine. These tests verify robustness under various edge conditions including error handling, concurrent access, and panic recovery.

## Test Coverage Summary

| Test Function | Subtests | Category |
|--------------|----------|----------|
| `TestRuntimeInitializationFailures` | 4 | Runtime |
| `TestGlobalRegistrationEdgeCases` | 6 | Globals |
| `TestNativeModuleErrorHandling` | 6 | Modules |
| `TestTUIBindingEdgeCases` | 9 | TUI Bindings |
| `TestConcurrentScriptExecution` | 3 | Concurrency |
| `TestScriptPanicRecovery` | 4 | Panic Handling |
| `TestScriptExecutionEdgeCases` | 6 | Script Execution |
| **Total** | **38** | |

## Detailed Test Descriptions

### TestRuntimeInitializationFailures

Tests JavaScript runtime initialization and edge cases around VM lifecycle:

1. **InvalidGojaOptions** - Tests that the runtime handles various JavaScript configurations:
   - Empty and whitespace-only scripts
   - Valid expressions
   - Undefined variable access
   - Syntax errors

2. **VMAccessedAfterClose** - Tests behavior when attempting to access VM after engine is closed:
   - Documents expected panic behavior
   - Verifies graceful handling

3. **RuntimeCloseIdempotent** - Tests that closing the runtime multiple times is safe:
   - No panics from multiple Close() calls
   - Idempotent cleanup

4. **RunOnLoopSyncAfterClose** - Tests RunOnLoopSync behavior after runtime closure:
   - Returns appropriate error when loop is stopped

### TestGlobalRegistrationEdgeCases

Tests global variable registration edge cases:

1. **DuplicateSymbolNames** - Tests that setting the same global multiple times:
   - Last value correctly overwrites previous values
   - No data corruption

2. **InvalidJSIdentifiers** - Tests setting globals with unusual identifier names:
   - Standard identifiers
   - Dollar sign prefix
   - Underscore prefix
   - Various casing patterns

3. **GlobalRegistrationOrderDependencies** - Tests that global access order doesn't matter:
   - Set globals in arbitrary order
   - Access in different order
   - All values preserved correctly

4. **ManyGlobalVariables** - Tests setting 100 global variables:
   - All values accessible
   - No performance degradation

5. **UnicodeGlobalNames** - Tests using unicode in global names:
   - Japanese characters
   - Emoji in identifiers
   - Proper value retrieval

6. **GlobalWithSpecialValues** - Tests setting special values:
   - nil/Go null
   - JS undefined behavior

### TestNativeModuleErrorHandling

Tests error handling for native Go modules:

1. **ModuleFunctionWithNilReceiver** - Tests requiring non-existent modules:
   - Error propagation to JS
   - No host crashes

2. **ModuleFunctionWithInvalidArguments** - Tests passing invalid arguments to modules:
   - Negative sleep duration
   - Non-numeric arguments
   - Error handling in time.sleep

3. **ModuleFunctionThatThrowsJSErrors** - Tests JS error throwing:
   - Try-catch correctly catches errors
   - Error messages preserved

4. **ModuleFunctionThatPanics** - Tests panic recovery:
   - Scripts cannot crash the host
   - ScriptPanicError correctly captured
   - Stack traces preserved

5. **NestedTryCatchAroundPanics** - Tests nested error handling:
   - Inner and outer catch blocks
   - Error propagation works

6. **AsyncModuleOperations** - Tests async module operations:
   - time.sleep works correctly
   - No async-related crashes

### TestTUIBindingEdgeCases

Tests TUI binding edge cases:

1. **TUIOperationsWhenTUINotActive** - Tests TUI functions without active session:
   - listModes() works
   - No crashes

2. **TUIOperationsWithInvalidMessageTypes** - Tests invalid TUI operations:
   - Null mode names
   - Error propagation

3. **TUIOperationsWithInvalidComponentIDs** - Tests accessing non-existent modes:
   - Proper error handling
   - No crashes

4. **TUIStateOperations** - Tests state creation and access:
   - createState works
   - get/set operations
   - Default values

5. **TUIContextOperations** - Tests context operations:
   - addPath
   - listPaths
   - No crashes

6. **TUILoggerOperations** - Tests all log levels:
   - debug, info, warn, error
   - No crashes

7. **TUICommandRegistration** - Tests command registration:
   - Valid command registration
   - Handler function requirement

8. **TUIExitRequestOperations** - Tests exit request flow:
   - requestExit
   - isExitRequested
   - clearExitRequest

9. **MultipleTUIOperations** - Tests multiple TUI operations in sequence:
   - Register mode
   - Switch mode
   - Get current mode
   - List modes

### TestConcurrentScriptExecution

Tests concurrent execution scenarios:

1. **QueueSetGlobalFromMultipleGoroutines** - Tests concurrent QueueSetGlobal:
   - 50 goroutines
   - 20 iterations each
   - All values accessible

2. **RapidEngineCreationAndClose** - Tests rapid engine creation:
   - 10 engines created and closed
   - No resource leaks
   - No crashes

3. **MixedSyncAndAsyncGlobalAccess** - Tests mixed sync/async access:
   - Alternate between sync and async
   - 20 operations
   - No race conditions

### TestScriptPanicRecovery

Tests panic handling and recovery:

1. **PanicWithVariousTypes** - Tests panicking with different types:
   - String panic
   - Number panic
   - Object panic
   - nil panic

2. **PanicRecoveryWithDefer** - Tests panic with deferred cleanup:
   - Deferred functions still run
   - Panic properly captured

3. **NestedPanicRecovery** - Tests nested try-catch:
   - Inner and outer errors
   - Proper error propagation

4. **PanicInDeferredFunction** - Tests panic in deferred function:
   - Deferred panic is caught
   - Host remains stable

### TestScriptExecutionEdgeCases

Tests additional script execution scenarios:

1. **VeryLongScript** - Tests execution of 1000-line script:
   - No performance issues
   - All lines execute

2. **DeepNesting** - Tests deeply nested function calls:
   - 10 levels of IIFEs
   - Correct return value

3. **ScriptWithUnicode** - Tests unicode in scripts:
   - Unicode strings
   - Unicode symbols

4. **ScriptWithSpecialCharacters** - Tests special characters:
   - Newlines, tabs, carriage returns
   - Template literals

5. **ScriptWithRegex** - Tests regex operations:
   - Pattern matching
   - match() method

## Verification Results

| Check | Status |
|-------|--------|
| Compilation | ✅ Pass |
| Tests | ✅ Pass |
| go vet | ✅ Pass |
| staticcheck | ✅ Pass |
| deadcode | ✅ Pass |

## Test Isolation

All tests:
- Use `t.Cleanup()` for proper resource cleanup
- Create isolated engine instances
- Do not share state between tests
- Properly close resources after completion

## Platform Compatibility

Tests are designed to work on all platforms (Linux, macOS, Windows) where the scripting engine is supported. No platform-specific tests were added.
