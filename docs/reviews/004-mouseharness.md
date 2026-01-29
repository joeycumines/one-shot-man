# G3: Mouse Harness Module Review

**Reviewer:** Takumi  
**Date:** 2026-01-29  
**Status:** PASS (no issues)  
**Files Reviewed:**
- `internal/mouseharness/console.go`
- `internal/mouseharness/terminal.go`
- `internal/mouseharness/mouse.go`
- `internal/mouseharness/element.go`
- `internal/mouseharness/options.go`
- `internal/mouseharness/main_test.go`
- `internal/mouseharness/console_test.go`
- `internal/mouseharness/terminal_test.go`
- `internal/mouseharness/mouse_test.go`
- `internal/mouseharness/options_test.go`
- `internal/mouseharness/integration_test.go`
- `internal/mouseharness/internal/dummy/main.go`

---

## Module Purpose

The `mouseharness` package provides utilities for mouse interaction testing with terminal-based TUI applications via PTY. It wraps an externally-managed `*termtest.Console` and adds:

1. **SGR Mouse Event Generation** - Generates proper SGR extended mouse mode escape sequences
2. **Terminal Buffer Parsing** - Virtual terminal emulator that parses ANSI sequences
3. **Element Location Finding** - Locates UI elements by visible text content
4. **Viewport Coordinate Translation** - Converts buffer-absolute to viewport-relative coordinates

---

## Architecture Assessment

### Design Quality: **Excellent**

The module follows clean separation of concerns:

1. **Functional Options Pattern** (`options.go`) - Clean, extensible configuration
2. **Wrapper Pattern** (`console.go`) - Non-invasive extension of `termtest.Console`
3. **Pure Functions** (`terminal.go`) - Stateless parsing with exported variants
4. **Context-Aware Operations** (`element.go`) - Proper timeout and cancellation support

### Key Components

| File | Lines | Purpose |
|------|-------|---------|
| `console.go` | ~95 | Core Console wrapper with basic operations |
| `terminal.go` | ~356 | ANSI parsing, virtual terminal emulation |
| `mouse.go` | ~80 | SGR mouse event generation |
| `element.go` | ~145 | Element finding and click automation |
| `options.go` | ~65 | Functional options configuration |

---

## Code Quality Assessment

### Strengths

1. **Comprehensive ANSI Handling** (`terminal.go`)
   - CSI sequences (cursor positioning, colors, erase operations)
   - OSC sequences (window titles)
   - Alt screen handling (`?1049h`)
   - UTF-8 multi-byte character support

2. **Correct SGR Mouse Protocol** (`mouse.go`)
   - Proper format: `ESC [ < Cb ; Cx ; Cy M/m`
   - Button encoding: 0=left, 1=middle, 2=right, 64=wheel up, 65=wheel down
   - Press/release distinction (M vs m)

3. **Viewport Coordinate Handling** (`element.go`, `terminal.go`)
   - Correctly handles buffer vs viewport coordinates
   - Accounts for scrolled content beyond visible area
   - Proper clamping to valid viewport range

4. **Robust Test Infrastructure**
   - `TestMain` builds dummy BubbleTea app once for all integration tests
   - Proper cleanup of temp directories
   - Both unit tests (pure logic) and integration tests (full PTY)

5. **Thread Safety** - No shared mutable state; each Console has its own data

---

## Detailed Review: No Issues Found

### Verified Behaviors

1. **Element Finding** ✓
   - Strips ANSI codes before searching
   - Returns 1-indexed row/col coordinates
   - Handles emoji placeholders correctly

2. **Click Operations** ✓
   - 30ms delay between press and release (realistic timing)
   - Proper button parameter support
   - Context timeout for element polling

3. **Scroll Wheel** ✓
   - Correct SGR wheel encoding (64/65)
   - Scroll events are press-only (no release)

4. **Buffer Parsing** ✓
   - Handles all major ANSI sequences
   - Trims trailing spaces from lines
   - Grows screen dynamically as needed

---

## Test Coverage Assessment

| Test File | Coverage |
|-----------|----------|
| `terminal_test.go` | Comprehensive - cursor, ANSI, special chars |
| `console_test.go` | Good - element finding, viewport conversion |
| `mouse_test.go` | Good - SGR encoding verification |
| `options_test.go` | Full - all options, validation |
| `integration_test.go` | Excellent - full PTY workflows |

All 30+ tests pass consistently.

---

## Minor Observations (Not Issues)

1. **Placeholder for UTF-8** - Uses `*` placeholder for multi-byte characters (acceptable for text matching)

2. **Debug Logging** - `ClickElement` includes debug logging which may be verbose but aids troubleshooting

3. **Width Option Note** - Comment mentions "PTY API quirks" for width - documented limitation

---

## Conclusion

**Status: CLEAN - No changes required**

The mouseharness module is well-designed, thoroughly tested, and implements correct terminal mouse protocols. It provides essential testing infrastructure for BubbleTea TUI applications with proper ANSI parsing and SGR mouse event generation.

This is quality test infrastructure code.
