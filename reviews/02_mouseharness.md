# Mouseharness Package Review

## SUCCINCT SUMMARY

**CRITICAL FINDING**: The mouseharness package is **UNIX-ONLY** (all files use `//go:build unix`) - it **CANNOT be verified to work on Windows** as required by the project's cross-platform mandate (Linux, macOS, Windows). This is a **FAIL** condition.

**Secondary Findings**:
- Well-designed terminal buffer parsing with excellent ANSI escape sequence handling
- Good SGR mouse event encoding implementation
- Comprehensive unit tests for terminal parsing and mouse functions
- Integration tests exist but are Unix-only (unverified on Windows)
- Timing-dependent code exists (30ms delay between press/release, 50ms polling) - potential flakiness (unverified)
- UTF-8 handling uses placeholder character '*' instead of proper rendering
- Row 0 compensation hack (unverified if necessary)

## DETAILED ANALYSIS

### 1. terminal.go (364 lines)
**Purpose**: Virtual terminal emulator for parsing PTY output

**Key Components**:
- `parseTerminalBuffer()`: Simulates terminal output with cursor positioning, ANSI sequences, special chars
- `stripANSI()`: Removes ANSI escape codes (CSI, OSC, character set designation)
- `getBufferLineCount()`: Counts non-empty buffer lines
- `getVisibleTop()`: Calculates viewport offset for scrolling
- `bufferRowToViewportRow()`: Converts buffer-absolute to viewport-relative coordinates
- `GetBuffer()` / `GetBufferRaw()`: Buffer inspection methods

**Findings**:
- ✅ **ANSI parsing is robust**: Handles CSI sequences (H, J, K, A, B, C, D, m, h, l), OSC sequences with BEL/ST terminators, character set designation
- ✅ **Cursor movement comprehensive**: Implements up, down, forward, back, tab, backspace, carriage return, line feed
- ✅ **Alt screen support**: Correctly clears buffer on `?1049h` or `47h` mode switch
- ⚠️ **UTF-8 handling**: Multi-byte characters replaced with `'*'` placeholder (line 206-214) - loses unicode information
- ⚠️ **Screen sizing**: Grows dynamically but starts at 30x100 - hardcoded initial limits
- ⚠️ **Row 0 emptiness detection**: Used in element.go for viewport adjustment - unverified if this indicates a bug or expected behavior

**Verification Steps**:
```bash
# Parse operations unit tests (verified on macOS only):
go test -v ./internal/mouseharness/... -run TestParseTerminalBuffer
go test -v ./internal/mouseharness/... -run TestStripANSI
```

**Cross-Platform Issues**:
- ❌ **UNIX-ONLY**: `//go:build unix` restricts to Linux/macOS only
- ❌ **No Windows support**: No Windows PTY equivalent implemented
- ❌ **Verification unavailable**: Cannot verify on Windows per project requirements

---

### 2. mouse.go (180 lines)
**Purpose**: SGR mouse event generation and handling

**Key Components**:
- `MouseButton` type: Left (0), Middle (1), Right (2) - matches SGR encoding
- `ScrollDirection` type: Up (64), Down (65) - matches SGR encoding
- `Click()`: Convenience method for left click
- `ClickViewport()`: Alias emphasizing viewport-relative coordinates
- `ClickAtBufferPosition()`: Converts buffer-absolute to viewport-relative then clicks
- `ClickWithButton()`: Generic click method with button selection
- `ScrollWheel()`: String-based scroll (deprecated)
- `ScrollWheelWithDirection()`: Type-safe scroll
- `ScrollWheelOnElement`: Element-finding + scroll helpers

**Findings**:
- ✅ **SGR encoding correct**: Mouse sequences follow X11 spec: `\x1b[<Cb;Cx;CyM/m`
- ✅ **Button constants match spec**: Left=0, Middle=1, Right=2, Wheel Up=64, Wheel Down=65
- ✅ **Viewport vs buffer distinction**: Properly handles coordinate systems
- ✅ **Type safety**: New `ScrollWheelWithDirection()` replaces string-based version
- ⚠️ **Timing delay**: 30ms sleep between press and release (line 98) - potential flakiness (unverified)
- ⚠️ **Deprecated method**: `ScrollWheel()` with string "up"/"down" still exists

**Verification Steps**:
```bash
# SGR encoding tests (verified on macOS only):
go test -v ./internal/mouseharness/... -run TestSGRMouseEscapeSequences
go test -v ./internal/mouseharness/... -run TestScrollWheelEscapeSequences
```

**Cross-Platform Issues**:
- ❌ **UNIX-ONLY**: `//go:build unix` restricts to Linux/macOS only
- ❌ **No Windows PTY**: Cannot send mouse events to Windows console
- ❌ **Verification unavailable**: Cannot verify on Windows per project requirements

---

### 3. console.go (106 lines)
**Purpose**: Console wrapper and mouseharness public API

**Key Components**:
- `Console` type: Wraps external `*termtest.Console`
- `consoleConfig`: Builder pattern for console creation
- `New()`: Constructor with functional options
- `Height()` / `Width()`: Terminal dimensions
- `String()`: Buffer content access
- `Snapshot()` / `WriteString()`: termtest.Console passthrough
- `TermtestConsole()`: Access to underlying console

**Findings**:
- ✅ **Design pattern**: Clean separation of concerns - wraps external dependency
- ✅ **Builder pattern**: Functional options for flexible configuration
- ✅ **Validation**: Enforces non-nil constraints for required fields
- ✅ **Default values**: 24x80 terminal size matches common defaults
- ✅ **Delegation**: Proper passthrough to termtest.Console methods

**Verification Steps**:
```bash
# No direct unit tests (uses mock termtest.Console in integration tests)
```

**Cross-Platform Issues**:
- ❌ **UNIX-ONLY**: `//go:build unix` restricts to Linux/macOS only
- ❌ **termtest dependency**: Inherits termtest platform limitations (Unix-only PTY)
- ❌ **Verification unavailable**: Cannot verify on Windows per project requirements

---

### 4. element.go (155 lines)
**Purpose**: Element location finding and interaction

**Key Components**:
- `ElementLocation` type: Row, Col, Width, Height, Text of found element
- `FindElement()`: Searches current buffer
- `FindElementInBuffer()`: Searches specific buffer string
- `ClickElement()`: Finds element then clicks with viewport conversion
- `ClickElementAndExpect()`: Click + wait verification (atomic operation)
- `Require*` variants: Fail-fast helpers
- `GetElementCenter()`: Coordinate calculation
- `DebugBuffer()`: Diagnostic logging

**Findings**:
- ✅ **ANSI stripping**: Searches parsed buffer (stripped ANSI) vs raw buffer - correct for visible text
- ✅ **Viewport conversion**: Properly uses `bufferRowToViewportRow()` for scrolled content
- ✅ **Polling with timeout**: Context-based element discovery with 50ms interval
- ✅ **Atomic operations**: `ClickElementAndExpect()` guarantees sequence
- ⚠️ **Row 0 compensation hack** (lines 104-110): Empty row 0 detection triggers viewportY++ - indicates potential rendering issue
- ⚠️ **Debug logging**: Uses `[CLICK DEBUG]` prefix - useful but hardcoded
- ⚠️ **Polling timing**: 50ms ticker interval - potential flakiness (unverified)

**Critical Code** (Row 0 Hack):
```go
// Check if row 0 is empty in the parsed buffer - this indicates a render issue
screen := parseTerminalBuffer(c.cp.String())
row0Empty := len(screen) > 0 && strings.TrimSpace(screen[0]) == ""
if row0Empty {
    viewportY++ // Compensate for missing title line
    c.tb.Logf("[CLICK DEBUG] Row 0 is empty, adjusting viewportY from %d to %d", viewportY-1, viewportY)
}
```

**Verification Steps**:
```bash
# Element finding tests (verified on macOS only):
go test -v ./internal/mouseharness/... -run TestFindElement
go test -v ./internal/mouseharness/... -run TestBufferRowToViewportRow
go test -v ./internal/mouseharness/... -run TestClickElement_AccountsForViewportOffset
```

**Cross-Platform Issues**:
- ❌ **UNIX-ONLY**: `//go:build unix` restricts to Linux/macOS only
- ❌ **Row 0 hack unverified**: Unknown if this is platform-specific rendering artifact
- ❌ **Verification unavailable**: Cannot verify on Windows per project requirements

---

### 5. options.go (78 lines)
**Purpose**: Functional options pattern for Console configuration

**Key Components**:
- `Option` interface: `applyOption(*consoleConfig) error`
- `optionFunc`: Concrete implementation
- `WithTermtestConsole()`: REQUIRED - external console
- `WithTestingTB()`: REQUIRED - test context
- `WithHeight()` / `WithWidth()`: Terminal dimensions

**Findings**:
- ✅ **Functional options**: Clean, idiomatic Go pattern
- ✅ **Validation**: Enforces non-nil constraints, positive dimensions
- ✅ **Error messages**: Clear, descriptive error messages
- ✅ **Defaults**: Sensible defaults (24 height, 80 width)
- ✅ **Consistency**: Error handling consistent across options

**Verification Steps**:
```bash
# Options tests (verified on macOS only):
go test -v ./internal/mouseharness/... -run TestWithHeight
go test -v ./internal/mouseharness/... -run TestWithWidth
go test -v ./internal/mouseharness/... -run TestNew_MissingTermtestConsole
```

**Cross-Platform Issues**:
- ✅ **No platform-specific code**: Pure configuration - theoretically portable
- ⚠️ **Depends on consumer**: Cross-platform support requires platform-compatible Console implementation
- ❌ **Build constraint**: `//go:build unix` prevents use on Windows even though code is portable

---

### 6. Test Files Analysis

#### terminal_test.go (185 lines)
**Coverage**:
- Plain text parsing ✅
- Cursor positioning (forward, back, up, down) ✅
- ANSI color codes ✅
- Alt screen switching ✅
- Erase operations ✅
- Special characters (tab, backspace, carriage return) ✅
- `stripANSI()` function ✅
- `isCSITerminator()` function ✅

**Findings**:
- ✅ **Comprehensive ANSI coverage**: Tests color, cursor, erase, alt screen
- ✅ **Edge cases**: Empty params, missing params, unknown sequences ignored
- ✅ **OSC sequences**: Tests BEL and ST terminators

#### mouse_test.go (148 lines)
**Coverage**:
- SGR mouse press/release sequences ✅
- Scroll wheel sequences (up/down) ✅
- MouseButton constants ✅
- ScrollDirection constants ✅
- Direction validation ✅

**Findings**:
- ✅ **Sequence correctness**: Verifies exact SGR encoding format
- ✅ **Button mapping**: Left=0, Middle=1, Right=2
- ✅ **Scroll encoding**: Up=64, Down=65

#### console_test.go (171 lines)
**Coverage**:
- `findElementInScreen()` with ANSI ✅
- `findElementInScreen()` without ANSI ✅
- Buffer row to viewport row conversion ✅ (8 test cases)
- Viewport offset correctness ✅

**Findings**:
- ✅ **ANSI stripped correctly**: Element finding works with colored text
- ✅ **Viewport math correct**: 8 comprehensive test cases including edge cases
- ✅ **Clamping logic**: Tests values outside viewport range

#### options_test.go (82 lines)
**Coverage**:
- `optionFunc` pattern ✅
- Height validation ✅
- Width validation ✅
- TestingTB nil check ✅
- TermtestConsole nil check ✅
- Multiple options application ✅
- Default config values ✅

**Findings**:
- ✅ **Negative validation**: Zero and negative values rejected
- ✅ **Nil checks**: Required fields enforced
- ✅ **Order independence**: Multiple options apply correctly

#### integration_test.go (268 lines) + main_test.go (87 lines)
**Coverage**:
- Console creation ✅
- Element finding ✅
- Element clicking ✅
- Scroll wheel events ✅
- Click with specific button ✅
- ClickElementAndExpect ✅
- GetElementCenter ✅
- DebugBuffer ✅
- RequireClick ✅

**Findings**:
- ⚠️ **Dummy program required**: Builds Go binary for testing Bubble Tea TUI
- ⚠️ **PTY timing**: Integration tests depend on external PTY behavior
- ⚠️ **Context timeouts**: All tests use 30s global timeout - very generous
- ⚠️ **TestMain complexity**: Complex setup/build/cleanup

**Dummy Binary** (`internal/dummy/main.go`):
- Simple Bubble Tea TUI
- Handles mouse events: left click, wheel up/down
- Displays: click state, scroll count, last position
- Keyboard controls: 'q' to quit, 'r' to reset

---

### 7. Cross-Platform Compatibility Analysis

#### Current Status:
```go
// All 11 source files:
//go:build unix
```

#### Platform-Specific Code Comparison:
Other packages in the repository provide Windows implementations:
- `internal/session/session_windows.go` ✅
- `internal/storage/windows_lock_delete_test.go` ✅
- `internal/storage/atomic_write_windows.go` ✅
- `internal/storage/filelock_windows.go` ✅

**Mouseharness**: ❌ **NO Windows implementation exists**

#### Windows PTY Challenge:
Windows console API differs fundamentally from Unix PTY:
- **Unix**: `os/exec`, pseudo-tty (`/dev/ptmx`)
- **Windows**: `conhost.exe`, Console API, Windows Terminal

**Potential Solutions** (not implemented):
1. Use `golang.org/x/crypto/ssh/terminal` (deprecated)
2. Write Windows console API wrapper
3. Drop Windows PTY support (accept limitation)

#### .deadcodeignore Entry:
```
# Mouseharness is a test-only package
internal/mouseharness/*
internal\mouseharness\*
```

**Analysis**:
- Correctly identified as test-only package (deadcode analysis would otherwise flag)
- **DOES NOT EXEMPT from cross-platform requirement** - test code must also work on all platforms

---

### 8. Timing and Reliability Issues

#### Timing-Dependent Code:

**1. Mouse Simulation Delay** (mouse.go:98):
```go
// Small delay between press and release for realism
time.Sleep(30 * time.Millisecond)
```
- **Risk**: TTY drivers may discard events sent too quickly
- **Unverified**: 30ms may be insufficient on slow systems
- **Unverified**: May cause flaky tests (no evidence yet)

**2. Element Polling Interval** (element.go:73):
```go
ticker := time.NewTicker(50 * time.Millisecond)
```
- **Risk**: Too fast - unnecessary CPU usage
- **Risk**: Too slow - tests timeout before element appears
- **Unverified**: No adaptive backoff or exponential sampling

**3. Integration Test Timeouts** (integration_test.go):
```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
```
- **Analysis**: Very generous - indicates potential timing uncertainty
- **Risk**: May hide slow render issues

#### Test Flakiness Assessment:
- **No flaky test evidence observed** (verified on macOS)
- **No explicit flakiness mitigations** (no retry logic, no tolerance windows)
- **No documented known timing issues**

---

### 9. Architectural Assessment

#### Strengths:
1. ✅ **Clean separation**: Termtest.Console (PTY) vs mouseharness (mouse utilities)
2. ✅ **Virtual terminal emulator**: Self-contained buffer parsing enables testing without real PTY
3. ✅ **Viewport abstraction**: Proper handling of scrolled content vs buffer content
4. ✅ **Type safety**: New ScrollDirection enum replaces string-based API
5. ✅ **Comprehensive tests**: High test coverage with good variety

#### Weaknesses:
1. ❌ **Unix-only build constraint**: BLOCKER for cross-platform requirement
2. ⚠️ **UTF-8 placeholder**: Loses unicode rendering information
3. ⚠️ **Row 0 compensation hack**: Workaround for unexplained rendering issue
4. ⚠️ **No Windows PTY strategy**: Unclear if Windows support is planned
5. ⚠️ **Test-only package placement**: In `internal/` namespace but not used by production code

#### Design Questions:
1. **Why test-only in internal/**? - Should be in internal/ or pkg/?
2. **PTY abstraction leak** - Is termtest.Console too tightly coupled?
3. **Unicode strategy** - Is `'*'` placeholder intentionally lossy?
4. **Row 0 issue** - Is this a termtest bug or expected behavior?

---

### 10. Code Quality Assessment

#### Static Analysis:
```bash
# Run per Makefile (verified on macOS only):
make vet.internal.mouseharness      # ✅ Pass
make staticcheck.internal.mouseharness  # ✅ Pass
make betteralign.internal.mouseharness  # ✅ Pass
```

#### Style and Maintainability:
- ✅ **Good documentation**: Comprehensive godoc comments
- ✅ **Clear naming**: Functions use descriptive names
- ✅ **Error handling**: Returns errors, validates inputs
- ✅ **Testing**: Well-structured tests with table-driven patterns
- ⚠️ **Magic numbers**: Some hardcoded values (30ms, 50ms, initial screen size)

#### Best Practices:
- ✅ **Builder pattern**: Functional options for Console construction
- ✅ **Interface segregation**: Small, focused interfaces
- ✅ **Immutable inputs**: Options copy config, don't mutate shared state
- ✅ **Test helpers**: `require()` variants fail fast appropriately

---

### 11. Integration Test Realism

#### Dummy Binary Design:
```go
type model struct {
    clicked  bool
    scrolled int
    lastX    int
    lastY    int
}
```

**Mouse Event Handling**:
- `tea.MouseButtonLeft` with `tea.MouseActionRelease` ✅
- `tea.MouseButtonWheelUp` / `WheelDown` ✅

**Assessment**:
- ✅ **Realistic behavior**: Matches Bubble Tea mouse API
- ✅ **State tracking**: Verifies click register, scroll count, position
- ✅ **Simple and effective**: Adequate for integration testing

#### Coverage Gaps:
- ⚠️ **No drag events**: Tests only click (press + release), not drag-and-drop
- ⚠️ **No double click**: Single click only tested
- ⚠️ **No modifier keys**: No shift-click, ctrl-click tests
- ⚠️ **No motion reporting**: `tea.WithMouseAllMotion()` not used in dummy

---

## CONCLUSION

### RECOMMENDATION: **FAIL**

### Justification:

#### **CRITICAL BLOCKERS**:

1. **Cross-Platform Requirement Violation** (BLOCKER):
   - All 11 files marked `//go:build unix`
   - **NO Windows implementation exists**
   - **Cannot verify on Windows** per project mandate (Linux, macOS, Windows)
   - Other packages (`internal/session`, `internal/storage`) provide Windows implementations - mouseharness does not

2. **PTY Dependency Architecture** (BLOCKER):
   - Tightly coupled to `github.com/joeycumines/go-prompt/termtest`
   - Termtest is Unix-only PTY library
   - No abstraction layer for Windows console
   - No documented Windows migration path

#### **SIGNIFICANT CONCERNS**:

3. **Unicode Rendering Loss**:
   - UTF-8 multi-byte characters replaced with `'*'` placeholder
   - Tests cannot verify unicode rendering correctness
   - May cause element finding failures on non-ASCII text

4. **Row 0 Compensation Hack**:
   - Hardcoded adjustment for "empty row 0" indicates unexplained rendering issue
   - May hide termtest bug or Bubble Tea viewport calculation error
   - Documented but not explained

5. **Timing-Dependent Test Risk** (Unverified):
   - 30ms delay between mouse press/release
   - 50ms polling interval for element discovery
   - No flakiness mitigations (retries, tolerances, adaptive backoff)
   - May cause intermittent test failures on slow systems

#### **POSITIVE ATTRIBUTES**:

1. **Excellent ANSI parsing** - Comprehensive coverage of escape sequences
2. **Good SGR mouse encoding** - Correct X11 protocol implementation
3. **Sensible API design** - Builder pattern, type safety, clear semantics
4. **High test coverage** - Unit tests, integration tests with dummy program
5. **Clean code style** - Good documentation, error handling, maintainability

### Required Fixes Before Approval:

**CRITICAL (Must Fix):**

1. **Windows Support Strategy** - One of:
   - Implement Windows console API wrapper (significant effort)
   - Document Windows exclusion and justify limitation (requires manager approval)
   - Move to `internal/` with Windows stubs that skip tests

2. **Build Constraint Review** - If Windows excluded:
   - Update AGENTS.md to document platform limitation
   - Update CI to reflect Windows skips
   - Update review checklist to clarify acceptance criteria

**MODERATE (Should Fix):**

3. **Unicode Strategy** - Either:
   - Implement proper UTF-8 cell width calculation
   - Document placeholder limitation with known impact
   - Add tests for unicode element finding

4. **Row 0 Issue Investigation** - Root cause analysis:
   - Is this a termtest bug?
   - Is this Bubble Tea viewport calculation error?
   - Is this expected alt-screen behavior?

5. **Timing Robustness** - Mitigations:
   - Add retry logic for element discovery
   - Use adaptive polling interval (exponential backoff)
   - Document acceptable timing ranges

**LOW (Nice to Have):**

6. **Test Coverage Expansion**:
   - Drag events
   - Double-click
   - Modifier keys (shift-click, ctrl-click)
   - Motion reporting

7. **Code Organization**:
   - Move test dummy binary to `testdata/` subdirectory
   - Consider `internal/testutil/mouseharness` instead of `internal/mouseharness`

### Verification Status:

| Platform | Tests Run | Result | Notes |
|----------|-----------|--------|-------|
| macOS    | All      | ✅ Pass | Manually verified |
| Linux    | All      | ⏸️ Unverified | Requires `make-all-in-container` |
| Windows  | All      | ❌ FAIL | Build constraint blocks compilation |

**Overall**: **FAIL** - Cannot meet "ALL platforms (ubuntu-latest, windows-latest, macos-latest)" requirement without Windows implementation or documented exclusion.

---

**Reviewer Note**: This package shows excellent engineering quality for Unix systems, but fundamentally fails the cross-platform mandate. The technical debt of Windows PTY support is significant and should be addressed early to prevent downstream consumer lock-in to Unix-only behavior.
