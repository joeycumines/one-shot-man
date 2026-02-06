# PTY Handling Code Peer Review

**Review Date:** February 6, 2026  
**Reviewer:** Peer Review (Takumi)  
**Files Reviewed:**
1. `/Users/joeyc/dev/one-shot-man/internal/command/pick_and_place_harness_test.go` - PTY Harness Implementation
2. `/Users/joeyc/dev/one-shot-man/internal/command/pick_and_place_unix_test.go` - PTY Integration Tests  
3. `/Users/joeyc/dev/one-shot-man/internal/command/pick_and_place_mouse_test.go` - Mouse Interaction Tests
4. `/Users/joeyc/dev/one-shot-man/internal/command/pick_and_place_error_recovery_test.go` - Error Recovery Tests
5. `/Users/joeyc/dev/one-shot-man/internal/cmd/test-pty/main.go` - PTY Test Utility
6. `/Users/joeyc/dev/one-shot-man/internal/mouseharness/mouse.go` - Mouse Harness Implementation
7. `/Users/joeyc/dev/one-shot-man/internal/command/testutils.go` - Shared Test Utilities

---

## Executive Summary

The PTY (pseudo-terminal) handling implementation in the one-shot-man project demonstrates **solid architectural foundations** with some areas requiring attention. The codebase correctly leverages the `github.com/joeycumines/go-prompt/termtest` package for PTY management and implements comprehensive test coverage for the pick-and-place simulation. However, several issues were identified that could lead to flaky tests, resource leaks, or race conditions in CI environments.

**Overall Assessment:** The implementation is **functionally correct** but has **medium-risk issues** around timing dependencies, error handling edge cases, and resource cleanup that should be addressed before production use.

### Key Strengths
- Correct use of `termtest.Console` for PTY management
- Comprehensive mouse event handling via SGR encoding
- Robust debug JSON parsing with ANSI code stripping
- Session isolation using UUID-based session IDs
- Comprehensive integration test coverage

### Key Concerns
- Fixed sleep durations introduce flakiness in CI
- Potential race conditions in frame synchronization
- Resource cleanup could be more defensive
- Some error paths lack proper error propagation

---

## 1. PTY Initialization Analysis

### 1.1 File: `pick_and_place_harness_test.go`

#### ✅ Correct Implementation: Lines 94-156
The PTY initialization correctly:
1. Sets the working directory to project root for correct script resolution
2. Configures appropriate terminal size (24 rows, 200 columns) to prevent JSON line-wrapping
3. Passes environment variables including `OSM_TEST_MODE=1`
4. Uses context with timeout for proper cancellation

```go
// Lines 101-122: Correct initialization pattern
h.console, err = termtest.NewConsole(h.ctx,
    termtest.WithCommand(h.binaryPath, args...),
    termtest.WithDefaultTimeout(h.timeout),
    termtest.WithEnv(testEnv),
    termtest.WithDir(projectDir),
    termtest.WithSize(24, 200),
)
```

#### ⚠️ Issue #1: No explicit PTY size negotiation (MINOR)
**Location:** `NewPickAndPlaceHarness()` - Line ~140  
**Severity:** Minor  
**Description:** The terminal size is set to 24x200, but there's no verification that the child process accepted this size. On some systems or under certain conditions (e.g., container environments), the PTY size might not be properly propagated.

**Impact:** Tests may fail silently or behave incorrectly if the terminal size is not accepted by the subprocess.

**Recommendation:** Add size verification after initialization:
```go
if err := h.console.Resize(h.ctx, 24, 200); err != nil {
    return nil, fmt.Errorf("failed to resize PTY: %w", err)
}
```

#### ⚠️ Issue #2: No error checking on `console.Expect()` (MINOR)
**Location:** Lines 143-175  
**Severity:** Minor  
**Description:** The startup detection uses `console.Expect()` in a loop but only logs when patterns are found. If the subprocess fails to start correctly, the error from the failed `Expect()` calls is discarded.

**Impact:** Potential for misleading error messages when startup fails.

**Recommendation:** Capture and log all Expect errors for debugging:
```go
var expectErrors []error
for _, pattern := range debugPatterns {
    if err := h.console.Expect(h.ctx, snap, termtest.Contains(pattern), "debug overlay"); err != nil {
        expectErrors = append(expectErrors, fmt.Errorf("pattern %q: %w", pattern, err))
        continue
    }
    found = true
    break
}
if !found {
    return nil, fmt.Errorf("simulator startup failed: %v", expectErrors)
}
```

---

## 2. Input Handling Analysis

### 2.1 Keyboard Input (`SendKey`)

#### ✅ Correct Implementation: Lines 741-751
The `SendKey()` method correctly:
1. Uses raw character input via `WriteString()` without newline
2. Properly handles bubbletea's requirement for raw keypresses
3. Checks for nil console before writing

```go
func (h *PickAndPlaceHarness) SendKey(key string) error {
    if h.console == nil {
        if err := h.Start(); err != nil {
            return err
        }
    }
    _, err := h.console.WriteString(key)
    return err
}
```

#### ⚠️ Issue #3: No input buffering or flow control (MAJOR)
**Location:** `SendKey()` - Lines 741-751  
**Severity:** Major  
**Description:** `SendKey()` writes directly to the PTY without any flow control or acknowledgment. When tests send rapid keystrokes, there's no guarantee the previous keystroke was processed before the next is sent.

**Impact:** Race conditions in tests that send multiple keys rapidly. This is evident in tests like `TestPickAndPlaceE2E_ManualModeMovement` which loops sending keys with fixed delays.

**Current Mitigation:** Tests use `WaitForFrames()` between keypresses, but this is timing-dependent.

**Recommendation:** Implement input acknowledgment:
```go
func (h *PickAndPlaceHarness) SendKeyWithAck(key string) error {
    if err := h.SendKey(key); err != nil {
        return err
    }
    // Wait for a frame to ensure processing
    h.WaitForFrames(1)
    return nil
}
```

### 2.2 Mouse Input (`ClickGrid`, `ClickWithButton`)

#### ✅ Correct Implementation: Lines 661-702
The mouse handling correctly:
1. Uses proper SGR (Select Graphic Rendition) extended mouse mode encoding
2. Handles coordinate translation from grid to terminal coordinates
3. Sends both press and release events for proper click simulation
4. Uses 1-indexed coordinates as per terminal convention

```go
// SGR mouse encoding: ESC [ < Cb ; Cx ; Cy M (press) / m (release)
// Cb = button number (0=left, 1=middle, 2=right)
mousePress := fmt.Sprintf("\x1b[<%d;%d;%dM", button, x, y)
mouseRelease := fmt.Sprintf("\x1b[<%d;%d;%dm", button, x, y)
```

#### ⚠️ Issue #4: Missing delay between press and release (MINOR)
**Location:** `ClickWithButton()` - Lines 692-702  
**Severity:** Minor  
**Description:** The harness sends press and release events back-to-back without any delay. While this may work on most terminals, some TUI applications expect a minimum dwell time for click detection.

**Comparison:** The `mouseharness/mouse.go` file includes a 30ms delay (line 79):
```go
// Small delay between press and release for realism
time.Sleep(30 * time.Millisecond)
```

**Impact:** Mouse clicks may be missed or interpreted incorrectly by the TUI.

**Recommendation:** Add configurable delay:
```go
const defaultClickDelay = 30 * time.Millisecond

func (h *PickAndPlaceHarness) ClickWithButton(x, y, button int, delay time.Duration) error {
    // ... press ...
    if delay > 0 {
        time.Sleep(delay)
    }
    // ... release ...
}
```

#### ⚠️ Issue #5: Potential integer overflow in coordinate calculation (MINOR)
**Location:** `ClickGrid()` - Lines 666-675  
**Severity:** Minor  
**Description:** The coordinate calculation uses `state.TotalWidth` and `state.SpaceWidth` directly without bounds checking. If these values are unexpected (e.g., from a corrupted debug state), the coordinates could be incorrect.

```go
spaceX := (state.TotalWidth - state.SpaceWidth) / 2
return h.Click(x+spaceX+2, y+1)
```

**Impact:** Click coordinates could be negative or overflow terminal bounds.

**Recommendation:** Add bounds validation:
```go
func (h *PickAndPlaceHarness) ClickGrid(x, y int) error {
    state := h.GetDebugState()
    spaceX := (state.TotalWidth - state.SpaceWidth) / 2
    
    // Validate coordinates are within reasonable bounds
    terminalX := x + spaceX + 2
    terminalY := y + 1
    if terminalX < 1 || terminalX > int(state.TotalWidth) ||
       terminalY < 1 || terminalY > 80 { // Assuming 80x24 minimum
        return fmt.Errorf("click coordinates (%d, %d) out of bounds", terminalX, terminalY)
    }
    return h.Click(terminalX, terminalY)
}
```

---

## 3. Frame Synchronization Analysis

### 3.1 `WaitForFrames()` Implementation

#### ⚠️ Issue #6: Race condition in frame detection (CRITICAL)
**Location:** `WaitForFrames()` - Lines 771-795  
**Severity:** Critical  
**Description:** The `WaitForFrames()` method has a race condition in how it detects frame updates:

```go
// Lines 782-794
for time.Now().Before(deadline) {
    currentState := h.GetDebugState()
    currentBufferLen := len(h.GetScreenBuffer())
    if retries%20 == 0 {
        // ... logging ...
    }
    prevBufferLen = currentBufferLen
    retries++
    if currentState.Tick >= initialTick+int64(frames) {
        return
    }
    time.Sleep(50 * time.Millisecond)
}
```

**Problems:**
1. `GetDebugState()` may return cached state if parsing fails
2. Buffer length comparison is unreliable (buffer may grow without frame changes)
3. No synchronization with actual frame render completion

**Impact:** Tests may proceed before frames are fully rendered, causing timing-dependent failures.

**Recommendation:** Implement frame-synchronized waiting:
```go
func (h *PickAndPlaceHarness) WaitForFrames(frames int64) error {
    initialTick := h.GetDebugState().Tick
    targetTick := initialTick + frames
    deadline := time.Now().Add(5 * time.Second)
    
    // First wait for at least one valid frame
    for time.Now().Before(deadline) {
        state := h.GetDebugState()
        if state.Tick > initialTick {
            break
        }
        time.Sleep(50 * time.Millisecond)
    }
    
    // Then wait for target frame
    for time.Now().Before(deadline) {
        state := h.GetDebugState()
        if state.Tick >= targetTick {
            return nil
        }
        // Add small delay to avoid CPU spinning
        time.Sleep(25 * time.Millisecond)
    }
    return fmt.Errorf("timeout waiting for frames: expected tick >= %d, got %d", 
        targetTick, h.GetDebugState().Tick)
}
```

#### ⚠️ Issue #7: No timeout handling in critical tests (MAJOR)
**Location:** `WaitForFrames()` - Line 795  
**Severity:** Major  
**Description:** When `WaitForFrames()` times out, it only logs a message but doesn't return an error. This causes tests to continue with potentially stale state.

```go
h.t.Logf("WaitForFrames: timeout reached, last tick=%d", initialTick)
// Returns implicitly with no error
```

**Impact:** Tests may proceed with incorrect assumptions about the system state.

**Recommendation:** Return error and let tests decide how to handle:
```go
func (h *PickAndPlaceHarness) WaitForFrames(frames int64) error {
    // ... existing code ...
    
    if time.Now().After(deadline) {
        err := fmt.Errorf("WaitForFrames(%d) timeout: initialTick=%d, currentTick=%d",
            frames, initialTick, h.GetDebugState().Tick)
        h.t.Logf("ERROR: %v", err)
        return err
    }
    return nil
}
```

### 3.2 `GetDebugState()` Implementation

#### ✅ Correct Implementation: Lines 832-855
The `GetDebugState()` method correctly:
1. Handles nil console by calling Start()
2. Caches last state for recovery on parse failures
3. Returns zero state when buffer is empty

#### ⚠️ Issue #8: Silent fallback to cached/zero state (MAJOR)
**Location:** `GetDebugState()` - Lines 848-855  
**Severity:** Major  
**Description:** When debug state parsing fails, the method falls back to cached state or zero state without signaling the caller:

```go
if err != nil {
    h.t.Logf("Warning: Could not parse debug state: %v", err)
    // ... log buffer for debugging ...
    if h.lastDebugState != nil {
        return h.lastDebugState  // Silent fallback!
    }
    return &PickAndPlaceDebugJSON{}  // Silent fallback!
}
```

**Impact:** Tests may continue with stale state, leading to misleading test results.

**Recommendation:** Add state freshness tracking:
```go
type DebugState struct {
    Data        *PickAndPlaceDebugJSON
    Timestamp   time.Time
    ParseError  error
    IsStale     bool
}

func (h *PickAndPlaceHarness) GetDebugState() *DebugState {
    // ... existing parsing ...
    
    if err != nil {
        return &DebugState{
            Data:       h.lastDebugState,
            Timestamp:  time.Now(),
            ParseError: err,
            IsStale:    true,
        }
    }
    return &DebugState{
        Data:      state,
        Timestamp: time.Now(),
        IsStale:   false,
    }
}
```

---

## 4. Test Harness Implementation Analysis

### 4.1 Binary Build Process

#### ✅ Correct Implementation: Lines 220-241
The `BuildPickAndPlaceTestBinary()` function correctly:
1. Uses `runtime.Caller()` for robust path resolution in parallel tests
2. Sets correct working directory for the build command
3. Captures stderr for error diagnostics

### 4.2 Session Isolation

#### ✅ Correct Implementation: Lines 260-270
Session isolation is properly implemented using `NewTestSessionID()` which:
1. Generates unique UUID-based session IDs
2. Sanitizes test names for filesystem safety
3. Creates isolated clipboard and store paths

```go
func NewPickAndPlaceTestProcessEnv(tb testing.TB) []string {
    sessionID := testutil.NewTestSessionID("pickplace", tb.Name())
    clipboardFile := filepath.Join(tb.(*testing.T).TempDir(), sessionID+"-clipboard.txt")
    return []string{
        "OSM_SESSION=" + sessionID,
        "OSM_STORE=memory",
        "OSM_CLIPBOARD=cat > " + clipboardFile,
    }
}
```

### 4.3 Debug JSON Parsing

#### ⚠️ Issue #9: Regex matching could match stale data (MINOR)
**Location:** `parseDebugJSON()` - Lines 1001-1065  
**Severity:** Minor  
**Description:** The regex matches the last JSON object in the buffer, which could be stale if the TUI hasn't updated:

```go
rawMatches := pickPlaceRawJSONRegex.FindAllString(normalizedBuffer, -1)
if len(rawMatches) > 0 {
    jsonStr = rawMatches[len(rawMatches)-1]  // Could be stale!
}
```

**Impact:** In edge cases where the TUI renders multiple JSON states, we might parse an old state.

**Recommendation:** Add timestamp verification if available:
```go
// Try to find the most recent JSON by looking for markers that indicate fresh output
if len(rawMatches) > 1 {
    // Prefer JSON that appears after "place_debug_start" marker
    if strings.Contains(buffer, "__place_debug_start__") {
        // Find last match after the marker
        markerIdx := strings.LastIndex(buffer, "__place_debug_start__")
        for i := len(rawMatches) - 1; i >= 0; i-- {
            matchIdx := strings.LastIndex(buffer, rawMatches[i])
            if matchIdx > markerIdx {
                jsonStr = rawMatches[i]
                break
            }
        }
    }
}
```

#### ⚠️ Issue #10: Incomplete ANSI code stripping (MINOR)
**Location:** `testutils.go` - Line 8  
**Severity:** Minor  
**Description:** The ANSI regex only matches standard CSI sequences:
```go
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
```

This misses:
- OSC (Operating System Command) sequences: `\x1b]`
- DCS (Device Control String): `\x1bP`
- APC (Application Program Command): `\x1b_`
- PM (Privacy Message): `\x1b^`

**Impact:** Some ANSI codes may remain in parsed JSON, causing parse failures.

**Recommendation:** Use comprehensive ANSI stripping:
```go
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\]|[^\x1b]|\x1bP|\x1b_|\x1b\^`)
```
Or better, use a proven library:
```go
import "github.com/mattn/go-colorable"
// Or use the strip-ansi-regex package
```

---

## 5. Error Handling Analysis

### 5.1 Error Recovery Tests

#### ✅ Good Practice: Lines 1-150 in `pick_and_place_error_recovery_test.go`
The error recovery tests demonstrate good practices:
1. Testing both expected errors and edge cases
2. Verifying error propagation
3. Testing null/undefined key handling

### 5.2 PTY Test Utility (`test-pty/main.go`)

#### ✅ Correct Implementation: Lines 1-185
The PTY test utility correctly:
1. Uses proper syscall operations for termios manipulation
2. Compares termios state before/after tests
3. Detects TTY state corruption

#### ⚠️ Issue #11: Incomplete termios flag checking (MINOR)
**Location:** `formatTermios()` - Lines 50-100  
**Severity:** Minor  
**Description:** The `formatTermios()` function only checks a subset of terminal flags. Some important flags like `ICANON`, `ECHO`, `ISIG` are checked, but others that could indicate corruption are not.

**Impact:** Some terminal state corruptions may go undetected.

**Recommendation:** Add comprehensive flag checking:
```go
func formatTermios(t *termios) string {
    if t == nil {
        return "<nil>"
    }
    var flags []string
    
    // Input flags
    if t.Iflag&syscall.IGNBRK != 0 { flags = append(flags, "IGNBRK") }
    if t.Iflag&syscall.BRKINT != 0 { flags = append(flags, "BRKINT") }
    if t.Iflag&syscall.ICRNL != 0 { flags = append(flags, "ICRNL") }
    if t.Iflag&syscall.INLCR != 0 { flags = append(flags, "INLCR") }
    if t.Iflag&syscall.IGNCR != 0 { flags = append(flags, "IGNCR") }
    
    // Output flags
    if t.Oflag&syscall.OPOST != 0 { flags = append(flags, "OPOST") }
    
    // Control flags
    if t.Cflag&syscall.CS8 != 0 { flags = append(flags, "CS8") }
    
    // Local flags
    if t.Lflag&syscall.ICANON != 0 { flags = append(flags, "ICANON") }
    if t.Lflag&syscall.ECHO != 0 { flags = append(flags, "ECHO") }
    if t.Lflag&syscall.ECHOE != 0 { flags = append(flags, "ECHOE") }
    if t.Lflag&syscall.ECHOK != 0 { flags = append(flags, "ECHOK") }
    if t.Lflag&syscall.ECHONL != 0 { flags = append(flags, "ECHONL") }
    if t.Lflag&syscall.ISIG != 0 { flags = append(flags, "ISIG") }
    if t.Lflag&syscall.IEXTEN != 0 { flags = append(flags, "IEXTEN") }
    if t.Lflag&syscall.PENDIN != 0 { flags = append(flags, "PENDIN") }
    
    return fmt.Sprintf("iflag=0x%x oflag=0x%x cflag=0x%x lflag=0x%x [%s]",
        t.Iflag, t.Oflag, t.Cflag, t.Lflag, strings.Join(flags, ","))
}
```

#### ⚠️ Issue #12: Error handling in `runTestInPTY()` (MINOR)
**Location:** `runTestInPTY()` - Lines 120-150  
**Severity:** Minor  
**Description:** The function discards errors from `getTermios()`:
```go
beforeState, _ = getTermios(int(ptmx.Fd()))
// ...
afterState, _ = getTermios(int(ptmx.Fd()))
```

**Impact:** Syscall errors are silently ignored, potentially masking issues.

**Recommendation:** Log errors:
```go
beforeState, err := getTermios(int(ptmx.Fd()))
if err != nil {
    fmt.Printf("  WARNING: Failed to get initial termios: %v\n", err)
}
// ...
afterState, err := getTermios(int(ptmx.Fd()))
if err != nil {
    fmt.Printf("  WARNING: Failed to get final termios: %v\n", err)
}
```

---

## 6. Resource Management Analysis

### 6.1 Console Lifecycle

#### ✅ Good: Defer-based cleanup in `Close()`
The `Close()` method properly cleans up:
```go
func (h *PickAndPlaceHarness) Close() {
    if h.console != nil {
        h.console.Close()
    }
    h.cancel()
}
```

#### ⚠️ Issue #13: Missing cleanup verification (MINOR)
**Location:** `Close()` and test defer statements  
**Severity:** Minor  
**Description:** There's no verification that cleanup completed successfully. If `console.Close()` fails, the error is silently lost.

**Impact:** Resource leaks may go undetected.

**Recommendation:** Add error logging:
```go
func (h *PickAndPlaceHarness) Close() error {
    var errs []error
    
    if h.console != nil {
        if err := h.console.Close(); err != nil {
            errs = append(errs, fmt.Errorf("console.Close(): %w", err))
        }
    }
    
    if h.cancel != nil {
        h.cancel()
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("Close() errors: %v", errs)
    }
    return nil
}
```

### 6.2 Test Parallelism

#### ⚠️ Issue #14: Potential parallelism issues with shared resources (MAJOR)
**Location:** Test file organization  
**Severity:** Major  
**Description:** Tests use `filepath.Join(t.TempDir(), ...)` for temporary files, which provides isolation per-test. However, some tests may not properly clean up their PTY consoles before starting new tests.

**Impact:** TTY state corruption when running tests in parallel.

**Note:** The `test-pty` utility with `-isolate` flag addresses this for CI, but regular `go test` may still have issues.

**Recommendation:** Add test isolation verification:
```go
func (h *PickAndPlaceHarness) VerifyCleanState() error {
    if h.console != nil {
        buffer := h.GetScreenBuffer()
        // Check for any leftover escape sequences or corrupted state
        if strings.Contains(buffer, "\x1b[?") && strings.Contains(buffer, "\x1b[") {
            return fmt.Errorf("possible terminal state corruption detected")
        }
    }
    return nil
}
```

---

## 7. CI-Specific Concerns

### 7.1 Timing Issues

#### ⚠️ Issue #15: Fixed sleep durations are CI-unfriendly (MAJOR)
**Locations:**
- `pick_and_place_unix_test.go`: Lines 78, 104, 156, 200, 300, etc.
- `pick_and_place_mouse_test.go`: Lines 55, 80, 120

Many tests use fixed sleep durations like:
```go
time.Sleep(500 * time.Millisecond)
time.Sleep(300 * time.Millisecond)
```

**Impact:** Tests may be flaky in CI environments with different CPU speeds, or unnecessarily slow on fast machines.

**Recommendation:** Implement adaptive timing:
```go
func adaptiveSleep(duration time.Duration) {
    // In CI, use base duration
    if os.Getenv("CI") != "" {
        time.Sleep(duration)
        return
    }
    // In local development, reduce delays for faster iteration
    time.Sleep(duration / 2)
}
```

Or better, use polling with exponential backoff:
```go
func waitForCondition(condition func() bool, timeout time.Duration) bool {
    deadline := time.Now().Add(timeout)
    backoff := 10 * time.Millisecond
    
    for time.Now().Before(deadline) {
        if condition() {
            return true
        }
        time.Sleep(backoff)
        backoff = min(backoff*2, 100*time.Millisecond)
    }
    return false
}
```

### 7.2 Test Reliability

#### ⚠️ Issue #16: Some tests have implicit assumptions (MINOR)
**Location:** Various tests in `pick_and_place_unix_test.go`  
**Severity:** Minor  
**Description:** Tests like `TestPickAndPlaceE2E_ModuleLoad` assume:
1. Module loads within 3 frames
2. Tick counter increments predictably
3. PTY buffer refreshes within 500ms

These assumptions may not hold in all environments.

**Recommendation:** Make tests more resilient:
```go
// Instead of:
h.WaitForFrames(3)

state := h.GetDebugState()
if state.Tick == 0 {
    h.WaitForFrames(5)
    state = h.GetDebugState()
}

// Use:
state := h.GetDebugState()
if state.Tick == 0 {
    if !h.WaitForFramesWithTimeout(5, 10*time.Second) {
        t.Fatalf("Module failed to load within timeout")
    }
    state = h.GetDebugState()
}
```

---

## 8. Issues Summary

### Critical Issues (Must Fix)

| ID | Issue | Location | Impact |
|----|-------|----------|--------|
| CR1 | Race condition in frame detection | `WaitForFrames()` | Tests may proceed before frames render |
| CR2 | Silent fallback to stale state | `GetDebugState()` | Tests may use incorrect state |

### Major Issues (Should Fix)

| ID | Issue | Location | Impact |
|----|-------|----------|--------|
| MJ1 | No flow control on rapid keystrokes | `SendKey()` | Race conditions in rapid input tests |
| MJ2 | No timeout on WaitForFrames | `WaitForFrames()` | Tests may hang indefinitely |
| MJ3 | Fixed sleep durations in CI | Multiple test files | Flaky tests, slow CI |
| MJ4 | Potential parallelism issues | Test organization | TTY state corruption |

### Minor Issues (Nice to Fix)

| ID | Issue | Location | Impact |
|----|-------|----------|--------|
| MN1 | No PTY size verification | `NewPickAndPlaceHarness()` | May fail silently |
| MN2 | No error collection on startup | Console startup detection | Misleading errors |
| MN3 | Missing mouse click delay | `ClickWithButton()` | Possible missed clicks |
| MN4 | Integer overflow potential | `ClickGrid()` | Out-of-bounds clicks |
| MN5 | Regex may match stale JSON | `parseDebugJSON()` | Stale state parsing |
| MN6 | Incomplete ANSI stripping | `testutils.go` | Parse failures |
| MN7 | Incomplete termios checking | `test-pty/main.go` | Undetected corruption |
| MN8 | Discarded syscall errors | `getTermios()` | Masked issues |
| MN9 | No cleanup verification | `Close()` | Undetected leaks |
| MN10 | Implicit test assumptions | Various tests | CI reliability |

---

## 9. Recommendations

### Priority 1: Fix Critical Issues

1. **CR1 - Frame Synchronization Race**
   - Implement proper frame-synchronized waiting
   - Add timestamp tracking for state freshness
   - Use exponential backoff for polling

2. **CR2 - State Fallback**
   - Add `IsStale` flag to debug state
   - Modify tests to check state freshness
   - Return errors instead of silently falling back

### Priority 2: Fix Major Issues

3. **MJ1 - Input Flow Control**
   - Add `SendKeyWithAck()` method
   - Update tests to use acknowledgment-based input
   - Consider implementing input buffering

4. **MJ2 - Timeout Handling**
   - Return errors from `WaitForFrames()`
   - Update tests to handle timeout errors
   - Add overall test timeout enforcement

5. **MJ3 - CI Timing**
   - Implement adaptive timing helper
   - Reduce delays in CI environment via env var
   - Use polling with backoff instead of fixed sleeps

6. **MJ4 - Parallelism Safety**
   - Add isolation verification in test setup
   - Document parallel test requirements
   - Consider adding test group boundaries

### Priority 3: Fix Minor Issues

7. Add comprehensive ANSI stripping
8. Improve termios checking in test-pty
9. Add cleanup verification
10. Add coordinate bounds checking
11. Document all public API methods
12. Add more unit tests for harness functions

---

## 10. Verification Criteria Assessment

| Criteria | Status | Notes |
|----------|--------|-------|
| ✅ PTY initialized correctly | PARTIAL | Size not verified, missing error collection |
| ✅ Input events properly transmitted | PARTIAL | Works but lacks flow control |
| ✅ Frame synchronization reliable | FAIL | Race condition exists |
| ✅ No race conditions in PTY handling | FAIL | Multiple race conditions identified |

---

## 11. Additional Observations

### 11.1 Code Quality Notes

The codebase demonstrates good practices:
- Proper use of `runtime.Caller()` for path resolution
- Good session isolation with UUID-based IDs
- Comprehensive error logging for debugging
- Clean separation of concerns

### 11.2 Documentation

Several public methods lack documentation comments:
- `NewPickAndPlaceHarness()` - Missing parameter descriptions
- `WaitForFrames()` - Missing return value documentation
- `ClickGrid()` - Missing error conditions

### 11.3 Testing Coverage

The test coverage is comprehensive but could be improved:
- Missing unit tests for `parseDebugJSON()`
- Missing tests for error paths in `SendKey()`
- Missing tests for concurrent PTY operations

---

## 12. Conclusion

The PTY handling implementation in one-shot-man is **structurally sound** but requires **significant improvements** to frame synchronization and timing handling before it can be considered production-ready. The critical race conditions and silent fallbacks to stale state pose the highest risk to test reliability.

**Overall Grade:** B- (Good implementation with significant reliability concerns)

**Next Steps:**
1. Fix CR1 and CR2 immediately (critical reliability)
2. Implement adaptive timing for CI (MJ3)
3. Add proper flow control for input (MJ1)
4. Add comprehensive error handling throughout
5. Add unit tests for harness internals

---

*Review completed: February 6, 2026*
