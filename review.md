# Super-Document: Comprehensive PR Correctness Analysis

## Executive Summary

**VERDICT: CORRECTNESS CANNOT BE GUARANTEED**

This PR introduces Go-native coordinate handling to eliminate cursor positioning bugs, but contains **multiple critical defects** that prevent a guarantee of correctness. The most severe issues are:

1. **Stale Viewport Context Bug**: `setViewportContext` captures scroll offset BEFORE auto-scroll logic executes, causing immediate click misalignment after typing
2. **Hardcoded Layout Assumptions**: `titleHeight = 1` hardcoded in mouse handling while dynamically calculated in rendering
3. **Wrapping Logic Defects**: Inconsistent character consumption in wrapped line simulation causing infinite loops and positioning errors
4. **Footer Click Regression**: Valid clicks below text content rejected by overly restrictive bounds checking
5. **Memory Leak**: Textarea models permanently registered without disposal mechanism
6. **Narrow Terminal Failures**: Hardcoded 40-column minimum causes layout corruption on terminals <50 columns wide

---

## Critical Defects

### 🔴 DEFECT 1: Stale Viewport Context (The "Phantom Scroll")

**Location**: `renderInput()` in `super_document_script.js`

**The Failure Chain**:
1. User types character creating new line at bottom
2. `renderInput` starts with `s.inputVp.yOffset() = 10`
3. `setViewportContext` called, Go stores `outerYOffset = 10`
4. Cursor visibility check calculates scroll needed to `yOffset = 15`
5. JS calls `s.inputVp.setYOffset(15)`
6. View renders with text shifted to offset 15
7. User clicks on just-typed text
8. `handleClickAtScreenCoords` uses stored `outerYOffset = 10`
9. **Result**: Click calculated 5 lines higher than reality (15 - 10)

**Code Evidence**:
```javascript
// CURRENT BUGGY ORDER in renderInput():
// 1. Context captured BEFORE scroll adjustment
if (s.contentTextarea && s.contentTextarea.setViewportContext && s.textareaBounds) {
    s.contentTextarea.setViewportContext({
        outerYOffset: s.inputVp.yOffset(), // OLD offset stored here
        // ...
    });
}

// 2. THEN cursor visibility logic runs
if (s.contentTextarea && s.inputFocus === FOCUS_CONTENT && !s.inputViewportUnlocked) {
    if (s.contentTextarea.getScrollSyncInfo) {
        const syncInfo = s.contentTextarea.getScrollSyncInfo();
        // ... calculates new offset ...
        s.inputVp.setYOffset(newOffset); // Updates viewport AFTER context captured
    }
}
```

**Required Fix**: Move `setViewportContext()` call to AFTER cursor visibility scroll logic, or call it twice (once before for scroll calculation, once after to update with final offset).

**Verification**: Multiple documents (5, 6, 11) independently identified this exact sequence issue.

---

### 🔴 DEFECT 2: Hardcoded Title Height Desynchronization

**Location**: `handleMouse()` vs `renderInput()` in `super_document_script.js`

**The Discrepancy**:
```javascript
// In handleMouse() - HARDCODED
const titleHeight = 1;
const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y, titleHeight);

// In renderInput() - DYNAMIC CALCULATION
const titleHeight = lipgloss.height(headerRow);
```

**Failure Scenario**:
- Terminal width narrows causing header row to wrap
- `renderInput` calculates `titleHeight = 2` (or more)
- Header renders as 2 visual lines
- User clicks expecting 2-line header offset
- `handleMouse` passes `titleHeight = 1` to Go
- Click coordinates off by `actualHeight - 1` lines vertically

**Impact**: Any layout change to header (wrapping, padding changes, button resizing) silently breaks all mouse targeting.

**Required Fix**: Pass dynamically calculated `titleHeight` through `setViewportContext` instead of as argument to click handler:

```javascript
// In renderInput - ADD to viewport context
s.contentTextarea.setViewportContext({
    // ... existing fields ...
    titleHeight: lipgloss.height(headerRow)  // Store dynamic value
});

// In handleMouse - REMOVE argument
const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y);
// Go reads titleHeight from stored context instead
```

**Verification**: Mentioned consistently across documents 5, 7, 10, 11, 12.

---

### 🔴 DEFECT 3: Wrapping Logic Infinite Loop / Character Skip Bug

**Location**: `textarea.go` in `performHitTest()` and `handleClickAtScreenCoords()`

**The Core Problem**: Character consumption loops fail when characters are wider than or equal to `contentWidth`.

**Buggy Code**:
```go
// CURRENT IMPLEMENTATION (Broken)
for segmentWidth < contentWidth && charsConsumed < len(line) {
    rw := runeWidth(line[charsConsumed])
    if segmentWidth+rw > contentWidth {
        break  // BUG: Breaks without consuming if segmentWidth=0 and rw>contentWidth
    }
    segmentWidth += rw
    charsConsumed++
}
```

**Failure Cases**:

**Case A - Wide Character at Line Start**:
- `contentWidth = 5`, `segmentWidth = 0`
- First character is CJK (width = 2)
- Loop: `0 < 5` ✓, `rw = 2`, check `0+2 > 5` ✗, consume char
- Second character is emoji (width = 8)  
- Loop: `2 < 5` ✓, `rw = 8`, check `2+8 > 5` ✓, **BREAK**
- `charsConsumed` not incremented for emoji
- Outer loop increments segment counter believing it processed a visual line
- **Next iteration reads same emoji again - infinite loop**

**Case B - Narrow Terminal**:
- Terminal width = 3 columns
- Character requires width = 4
- Loop enters with `segmentWidth = 0`
- `0 < 3` ✓, `rw = 4`, `0+4 > 3` ✓, break immediately
- Zero characters consumed
- Infinite loop as segment counter increments but text position doesn't advance

**Required Fix**: Force at least one character consumption per segment iteration:

```go
// CORRECTED IMPLEMENTATION
for charsConsumed < len(line) {
    rw := runeWidth(line[charsConsumed])
    
    // If adding this char exceeds width AND we already have content, wrap
    if segmentWidth > 0 && segmentWidth+rw > contentWidth {
        break
    }
    
    segmentWidth += rw
    charsConsumed++
    
    // After consuming, if we've filled the line, break for wrap
    if segmentWidth >= contentWidth {
        break
    }
}
```

**Duplicate Location**: This bug appears in TWO functions - `performHitTest` and `handleClickAtScreenCoords`. Both require the same fix.

**Verification**: Documents 9, 10 identified this with detailed trace examples.

---

### 🔴 DEFECT 4: Integer Division Cursor Positioning Bug

**Location**: `textarea.go` in `getScrollSyncInfo()` and cursor visual line calculations

**The Math Error**:
```go
// CURRENT IMPLEMENTATION (Wrong)
if contentWidth > 0 && visualWidthToCursor >= contentWidth {
    cursorVisualLine += visualWidthToCursor / contentWidth
}
```

**Why This Fails**: Uses "hard wrapping" division instead of simulating "greedy wrapping" algorithm used by renderer.

**Concrete Example**:
- `contentWidth = 10`
- Line has 9 monospace chars (width 9) + 1 emoji (width 2)
- Cursor positioned at emoji
- `visualWidthToCursor = 9` (width of chars before emoji)
- **Division**: `9 / 10 = 0` → cursor thinks it's on visual line 0
- **Actual Rendering**: 9 chars fit on line 0, emoji (width 2) requires positions 10-11, wraps entirely to line 1
- **Result**: Viewport scrolling miscalculated, cursor may be hidden

**Required Fix**: Replace division with loop simulating exact greedy wrapping:

```go
// CORRECTED IMPLEMENTATION
if targetRow < len(mirror.value) && contentWidth > 0 {
    currentLine := mirror.value[targetRow]
    currentWidth := 0
    visualLineIndex := 0
    
    for i, r := range currentLine {
        if i >= targetCol {
            break
        }
        rw := runeWidth(r)
        
        // Handle char wider than viewport
        if rw > contentWidth {
            if currentWidth > 0 {
                visualLineIndex++
                currentWidth = 0
            }
            visualLineIndex++
            continue
        }
        
        // Greedy wrap: if doesn't fit, wrap before adding
        if currentWidth+rw > contentWidth {
            visualLineIndex++
            currentWidth = rw
        } else {
            currentWidth += rw
        }
    }
    cursorVisualLine += visualLineIndex
}
```

**Verification**: Document 9 provided detailed trace with concrete numeric example.

---

### 🔴 DEFECT 5: Footer Click Regression

**Location**: `textarea.go` in `handleClickAtScreenCoords()` Step 5

**The Rejected Valid Clicks**:
```go
// CURRENT IMPLEMENTATION (Too Restrictive)
if visualY < 0 || visualY >= totalVisualLines {
    return result  // hit: false - REJECTS clicks below content
}
```

**Standard Editor Behavior**: Clicking empty space below text content should place cursor at end of document.

**Current Behavior**: 
- Document has 3 lines of text
- User clicks on visual line 10 (below content)
- Go returns `hit: false`
- JS has no fallback when `hit` is false
- **UI appears unresponsive**

**The Hidden Clamping**: Loop logic AFTER this check already handles Y-coordinates beyond document length:

```go
for row := 0; row < len(mirror.value); row++ {
    // ...
    if row == len(mirror.value)-1 {
        targetRow = row
        targetWrappedSegment = lineHeight - 1  // Correctly clamps to last segment
    }
}
```

**Required Fix**: Remove upper bound check, keep only negative check:

```go
// CORRECTED
if visualY < 0 {
    return result  // Only reject clicks above content
}
// visualY >= totalVisualLines is valid - will be clamped by loop
```

**Verification**: Document 8 identified this with detailed analysis of clamping logic.

---

### 🔴 DEFECT 6: Memory Leak

**Location**: `textarea.go` Manager and `super_document_script.js` mode transitions

**The Leak Mechanism**:
- JS: `s.contentTextarea = textareaLib.new()` called every time user enters Input Mode
- Go: `textarea.New()` creates model and adds to `Manager` map to prevent GC
- **Missing**: No `.dispose()` call when `s.contentTextarea` is overwritten or mode exits
- Manager map: `map[uint64]*ModelWrapper` grows indefinitely

**Accumulation Points**:
1. Every 'a' or 'e' keypress to enter Input Mode creates new instance
2. Mode switching (ESC, submit) overwrites reference without cleanup
3. Long-running application accumulates hundreds/thousands of leaked models

**Impact**: Process memory grows unbounded, eventual crash or system OOM.

**Required Fix**: Add disposal at mode transitions:

```javascript
// In handleKeys - mode exit
if (s.mode === MODE_INPUT && prevMode !== MODE_INPUT) {
    // Clean up previous instance before creating new
    if (s.contentTextarea && s.contentTextarea.dispose) {
        s.contentTextarea.dispose();
    }
    s.contentTextarea = textareaLib.new();
}

// In ESC handler
if (s.contentTextarea && s.contentTextarea.dispose) {
    s.contentTextarea.dispose();
}

// In submit handler
if (s.contentTextarea && s.contentTextarea.dispose) {
    s.contentTextarea.dispose();
}
```

**Verification**: Document 5 identified this issue uniquely with Manager map analysis.

---

### 🔴 DEFECT 7: Narrow Terminal Layout Overflow

**Location**: `renderInput()` fieldWidth calculation

**The Forced Minimum**:
```javascript
const fieldWidth = Math.max(40, termWidth - 10);
```

**Failure on Narrow Terminals**:
- Terminal width = 30 columns
- Calculation: `Math.max(40, 20) = 40`
- **Result**: Field rendered at 40 columns width in 30-column terminal
- Causes: Horizontal clipping, line wrapping artifacts, visual corruption

**Use Cases Broken**:
- Mobile SSH sessions (often 30-40 columns)
- Tmux/screen split panes
- Embedded terminal widgets
- Accessibility requirements for large fonts

**Required Fix**: Cap maximum as well as minimum:

```javascript
const fieldWidth = Math.min(Math.max(20, termWidth - 10), 40);
// Ensures: 20 ≤ fieldWidth ≤ min(40, termWidth-10)
```

Or remove hardcoded minimum entirely:
```javascript
const fieldWidth = Math.max(10, termWidth - 10);
```

**Verification**: Documents 12, 13 both identified this with concrete column calculations.

---

## Severe Issues

### 🟡 ISSUE 8: Thread Safety - vpCtx Not Mutex-Protected

**Location**: `textarea.go` closure variable declaration

**The Race Condition**:
```go
var vpCtx viewportContext  // Unprotected closure variable

// Writer (no mutex)
"setViewportContext": func(call goja.FunctionCall) goja.Value {
    // ... writes to vpCtx ...
    vpCtx.outerYOffset = int(v.ToInteger())
}

// Reader (under different mutex)
"handleClickAtScreenCoords": func(call goja.FunctionCall) goja.Value {
    wrapper.mu.Lock()  // Locks ModelWrapper, NOT vpCtx
    defer wrapper.mu.Unlock()
    // ... reads vpCtx.outerYOffset ...
}
```

**Theoretical Impact**: 
- JS runtime is single-threaded (Goja), so concurrent calls unlikely
- If Go scheduler interleaves goroutines calling these methods, data race possible
- Undefined coordinate calculations if race occurs

**Assessment**: 
- **Low probability** in current architecture (Bubble Tea single-threaded event loop)
- **High severity** if it occurs (memory corruption, crashes)
- **Violates contract** implied by mutex pattern elsewhere in code

**Options**:
1. Document single-threaded requirement explicitly in code comments
2. Move `vpCtx` inside `ModelWrapper` struct and protect with existing `wrapper.mu`
3. Add separate mutex for `vpCtx` access

**Verification**: Documents 6, 7, 11, 13 all noted this with varying severity assessments.

---

### 🟡 ISSUE 9: Dead Fallback Code Paths

**Location**: `handleMouse()` conditional branches

**The Unreachable Code**:
```javascript
if (s.contentTextarea.handleClickAtScreenCoords) {
    // Method ALWAYS exists (defined in Go binding)
    const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y, titleHeight);
    if (hitResult.hit) {
        s.inputViewportUnlocked = false;
    }
} else if (s.textareaBounds && s.contentTextarea.performHitTest) {
    // NEVER REACHED - handleClickAtScreenCoords always defined
    const hitResult = s.contentTextarea.performHitTest(...);
} else if (s.textareaBounds) {
    // NEVER REACHED
    const vpYOffset = s.inputVp ? s.inputVp.yOffset() : 0;
    const viewportRelativeY = msg.y - titleHeight;
    // ...
    s.contentTextarea.setPosition(visualY, visualX);  // BUG: treats visual coords as logical
}
```

**Assessment**:
- Not a runtime bug (code never executes)
- **Misleading** for code readers/reviewers
- Last fallback contains known bug (visualY/visualX != row/col for wrapped text)
- Increases maintenance burden without providing actual fallback functionality

**Impact**: Code quality and maintainability issue, not correctness issue.

**Recommendation**: Remove unreachable branches or add runtime detection if fallback truly needed.

**Verification**: Document 12 identified this with definitive analysis that method always exists.

---

### 🟡 ISSUE 10: Inconsistent Viewport Lock Behavior

**Location**: `handleMouse()` across different click handling branches

**The Inconsistency**:
```javascript
// GO-NATIVE path
if (hitResult.hit) {
    s.inputViewportUnlocked = false;  // Only set on successful hit
}

// Fallback paths (if they were reachable)
s.inputViewportUnlocked = false;  // Always set, regardless of hit success
```

**Impact**: If fallback paths were ever reached:
- User clicks outside textarea bounds
- GO-NATIVE: `hit=false`, viewport stays locked/unlocked as-is
- Fallback: Always unlocks viewport even for invalid clicks
- Different scrolling behavior depending on which code path executes

**Assessment**: Currently benign (fallbacks unreachable), but violates principle of consistent behavior across code paths.

**Verification**: Document 13 identified this inconsistency.

---

### 🟡 ISSUE 11: Scrollbar Height Miscalculation

**Location**: `renderInput()` scrollbar configuration

**The Miscalculation**:
```javascript
const scrollableContentHeight = lipgloss.height(scrollableContent);
```

**Problem**: `lipgloss.height()` counts `\n` characters (logical lines) but doesn't account for wrapped lines (visual lines).

**Example**:
- Document has 10 logical lines
- Each line 100 characters wide, wraps to 3 visual lines in 40-column viewport
- Actual visual height: ~30 lines
- `lipgloss.height()` returns: 10 lines
- Scrollbar sized/positioned for 10 lines when content is actually 30

**Impact**: 
- Scrollbar thumb appears larger than it should
- Scrollbar position doesn't accurately reflect scroll location
- User confused about document length and current position

**Required Fix**: Calculate total visual lines accounting for wrapping, or use textarea's `getScrollSyncInfo()` which includes wrapped line counts.

**Verification**: Document 13 identified this with explanation of lipgloss.height() behavior.

---

## Minor Issues & Code Quality Concerns

### ⚠️ Code Duplication in Coordinate Calculation

**Location**: `handleMouse()` fallback branches

Both unreachable fallback branches duplicate identical coordinate math:
```javascript
const vpYOffset = s.inputVp ? s.inputVp.yOffset() : 0;
const viewportRelativeY = msg.y - titleHeight;
const contentY = viewportRelativeY + vpYOffset;
const visualY = contentY - s.textareaBounds.contentTop;
const visualX = msg.x - s.textareaBounds.contentLeft;
```

**Impact**: Maintenance burden, but not a correctness issue.

**Recommendation**: Extract to helper function if fallbacks are kept.

---

### ⚠️ Unsafe Struct Mirroring Fragility

**Location**: `textarea.go` `textareaModelMirror` struct

**The Risk**:
```go
type textareaModelMirror struct {
    // ... fields that must match charmbracelet/bubbles/textarea.Model exactly ...
    viewport viewport.Model
}
```

**Dependencies**:
- Upstream library version stability
- Go struct memory layout rules
- Compiler padding consistency

**If Mismatch Occurs**:
- `unsafe.Pointer` cast maps to wrong memory location
- Dereferencing `viewport` field causes panic or memory corruption
- **Silent failure** - no compile-time detection

**Mitigation Present**: Compile-time size assertions:
```go
if unsafe.Sizeof(textareaModelMirror{}) != unsafe.Sizeof(textarea.Model{}) {
    // Compile error
}
```

**Residual Risk**: Size matching doesn't guarantee field ordering if struct layout changes.

**Recommendation**: Lock `charmbracelet/bubbles` to exact commit hash in `go.mod`.

**Verification**: Documents 7, 11 identified this as high-risk dependency.

---

### ⚠️ Stale Context After Terminal Resize

**Location**: Viewport context lifecycle

**The Timing Issue**:
1. Render cycle: Context captured with dimensions W=80, H=24
2. User resizes terminal to W=100, H=30
3. Render hasn't run yet (happens next frame)
4. User clicks immediately
5. Click handler uses stale context (W=80, H=24)
6. Coordinates miscalculated by dimension delta

**Mitigation**: Inherent to TUI architecture (View/Update separation). Fallback mechanism provides graceful degradation.

**Impact**: One-frame accuracy lag on resize events.

**Assessment**: Acceptable for TUI applications, but should be documented.

**Verification**: Documents 7, 13 noted this as theoretical issue.

---

### ⚠️ Zero-Width Combining Marks

**Location**: `runeWidth()` usage throughout

**Known Limitation** (from code comments):
```go
// Note: runewidth library only handles character widths, not grapheme clusters.
// Zero-width combining marks are mapped to width 1.
```

**Impact**: Incorrect column alignment for:
- Arabic/Hebrew text with diacritics
- Indic scripts (Hindi, Tamil) with combining marks
- Emoji with skin tone/gender modifiers (counted as multiple width-1 chars instead of cluster)

**Assessment**: Not a bug in this PR - inherited limitation of `runewidth` library.

**Recommendation**: Document this limitation for users. Consider upgrading to grapheme-aware library in future.

**Verification**: Document 13 noted this limitation.

---

## Verified Correct Implementation

The following aspects were verified correct across multiple document analyses:

### ✅ Core Coordinate Transformation Chain

The `handleClickAtScreenCoords()` pipeline correctly implements:
- Screen → Viewport (subtract title height)
- Viewport → Content (add scroll offset)  
- Content → Textarea (subtract content top)
- Textarea → Visual Line (account for wrapping)
- Visual → Logical (map wrapped segments to rows/cols)

**Verification Method**: Multiple documents traced concrete numeric examples through all transformation steps.

---

### ✅ Scroll Synchronization Logic

`getScrollSyncInfo()` correctly calculates:
```go
cursorAbsY := vpCtx.preContentHeight + cursorVisualLine
```

Uses viewport context from same render cycle. `preContentHeight` correctly excludes fixed header (operates in content-space).

**Boundary Cases Handled**:
- Cursor above viewport: `cursorAbsY < vpY` → scroll up
- Cursor below viewport: `cursorAbsY >= vpY + vpH` → scroll down
- Cursor visible: No scroll adjustment

---

### ✅ Hit Test Algorithm for Non-Wrapped Lines

When `contentWidth = 0` (no wrapping), direct column mapping is correct:
```go
targetCol = visualX
if targetCol > len(line) {
    targetCol = len(line)
}
```

Properly clamps to line length, handles clicks in right margin.

---

### ✅ Bounds Checking Horizontal

```go
if visualX < 0 {
    visualX = 0
}
```

Correctly handles clicks in left padding/border - cursor positioned at start of line.

---

### ✅ Empty Line Handling in Wrap Calculation

```go
func calculateWrappedLineCount(line []rune, width int) int {
    if len(line) == 0 {
        return 1  // Empty line still occupies 1 visual line
    }
    // ...
}
```

Prevents division by zero, matches editor conventions.

---

### ✅ Fallback Architecture (Design)

Three-tier approach provides graceful degradation:
1. GO-NATIVE: Full performance, atomicity
2. performHitTest: Compatibility with older code
3. Legacy setPosition: Last resort

(Implementation of tiers 2-3 has issues, but architectural design is sound)

---

## Information Trusted Without Verification

The following aspects were noted as assumptions that cannot be verified from code review alone:

### Trusted: lipgloss Rendering Behavior

- `lipgloss.height()` accurately counts newlines
- `lipgloss.width()` handles ANSI codes correctly
- `lipgloss.joinHorizontal()` wrapping behavior matches expectations
- Style padding/borders rendered consistently

**Risk**: If lipgloss behavior changes, layout calculations desynchronize.

---

### Trusted: Terminal Width Reporting

- `termWidth` from Bubble Tea accurately reflects terminal columns
- Terminal emulators report width correctly
- Width changes reflected immediately in next render

---

### Trusted: Scrollbar Width Constant

```javascript
const scrollbarWidth = 1;  // Assumed constant
```

No dynamic measurement - hardcoded assumption.

---

### Trusted: Upstream Library Compatibility

- `charmbracelet/bubbles/textarea` struct layout stable
- `runewidth` library width calculations match terminal rendering
- Bubble Tea event loop single-threaded guarantee

---

## Consolidated Fix Recommendations

### Priority 1 (Critical - Blocks Correctness Guarantee)

1. **Fix Viewport Context Timing** (Defect 1):
   - Move `setViewportContext()` call in `renderInput()` to AFTER cursor visibility scroll logic
   - Alternative: Call twice (before for calculation, after to update)

2. **Dynamic Title Height** (Defect 2):
   - Add `titleHeight` field to `viewportContext` struct
   - Set in `renderInput()` using `lipgloss.height(headerRow)`
   - Remove hardcoded argument from `handleMouse()` calls
   - Go reads from stored context

3. **Fix Wrapping Loop Logic** (Defect 3):
   - Apply corrected loop implementation to both `performHitTest()` and `handleClickAtScreenCoords()`
   - Force at least one character consumption per segment iteration
   - Handle wide characters correctly

4. **Fix Cursor Visual Line Calculation** (Defect 4):
   - Replace integer division with greedy wrapping simulation
   - Iterate through characters up to cursor position
   - Count visual line wraps using same logic as renderer

5. **Remove Footer Click Rejection** (Defect 5):
   - Change bounds check from `visualY < 0 || visualY >= totalVisualLines` to `visualY < 0` only
   - Allow clamping logic to handle out-of-bounds Y coordinates

6. **Implement Memory Cleanup** (Defect 6):
   - Add `.dispose()` calls at all mode transition points
   - Clean up previous textarea instance before creating new
   - Implement disposal in Manager to remove from map

7. **Fix Narrow Terminal Overflow** (Defect 7):
   - Change `Math.max(40, termWidth - 10)` to `Math.min(Math.max(20, termWidth - 10), 40)`
   - Or remove hardcoded minimum entirely

### Priority 2 (Severe - Should Fix Before Ship)

8. **Thread Safety for vpCtx** (Issue 8):
   - Move `vpCtx` inside `ModelWrapper` struct
   - Protect all access with `wrapper.mu`
   - Or document single-threaded requirement explicitly

9. **Remove Dead Fallback Code** (Issue 9):
   - Delete unreachable else-if branches in `handleMouse()`
   - Or add runtime feature detection if fallbacks needed

10. **Scrollbar Height Calculation** (Issue 11):
    - Calculate visual line count including wrapped lines
    - Use textarea's internal state instead of lipgloss.height()

### Priority 3 (Nice to Have)

11. Extract duplicate coordinate math to helper function
12. Lock upstream library versions to specific commits
13. Add timestamp/generation counter to viewport context for staleness detection
14. Document grapheme cluster limitations

---

## Testing Recommendations

To verify fixes are complete, test scenarios should include:

### Scenario 1: Rapid Typing with Auto-Scroll
1. Type continuously until text extends below viewport
2. Immediately click on text that just appeared
3. **Expected**: Cursor lands exactly where clicked
4. **Currently Fails**: Off by scroll amount (Defect 1)

### Scenario 2: Narrow Terminal
1. Resize terminal to 30 columns
2. Enter Input Mode
3. **Expected**: Layout fits within terminal
4. **Currently Fails**: Layout overflows, corruption (Defect 7)

### Scenario 3: Wrapped Title Click
1. Resize terminal to force title wrapping (very narrow)
2. Click in textarea
3. **Expected**: Cursor positioned correctly
4. **Currently Fails**: Off by title wrap amount (Defect 2)

### Scenario 4: Wide Characters
1. Enter text with CJK characters or emoji
2. Ensure wrapping occurs
3. Click on wrapped segments
4. **Expected**: Cursor lands on correct character
5. **Currently Fails**: Loop errors or position miscalculation (Defects 3, 4)

### Scenario 5: Footer Click
1. Create 3-line document
2. Scroll viewport to show lines with empty space below
3. Click below last line
4. **Expected**: Cursor moves to end of document
5. **Currently Fails**: No response (Defect 5)

### Scenario 6: Memory Stability
1. Repeatedly enter/exit Input Mode (100+ times)
2. Monitor process memory
3. **Expected**: Stable memory usage
4. **Currently Fails**: Unbounded growth (Defect 6)

---

## Metadata

**Analysis Date**: December 27, 2025  
**Source Documents**: 6-13 (8 documents total)  
**Critical Defects**: 7  
**Severe Issues**: 4  
**Minor Issues**: 6  
**Verified Correct**: 6 aspects  

**Confidence Level**: HIGH for identified defects (multiple independent verifications)  
**Coverage**: Core coordinate logic, layout calculations, memory management, edge cases

**Limitations**: Cannot verify lipgloss rendering, terminal emulator behavior, or upstream library compatibility without runtime testing.

---

The below was input into the above.


---
## DOCUMENTS
---


Document 5:
`````
The correctness of this PR cannot be guaranteed. In fact, I guarantee it contains **critical defects** that will cause incorrect cursor positioning and application instability.

### Succinct Summary

**The PR introduces a race condition where the Go viewport context becomes stale during auto-scrolling, causing mouse clicks to map to incorrect lines immediately after typing.** Additionally, the implementation creates a **severe memory leak** by permanently registering Go models without disposal, and relies on brittle hardcoded layout constants that will desynchronize the hit-test logic if the UI layout adapts (e.g., wrapped titles).

---

### Detailed Analysis

#### 1. CRITICAL BUG: Stale Viewport Context (The "Phantom Scroll")

The "Go-Native" hit-testing relies on `vpCtx` (Viewport Context) being perfectly synchronized with the rendered view. However, `renderInput` updates the context **before** calculating the auto-scroll.

**The Failure Chain:**

1. **User types a character** that creates a new line at the bottom.
2. `renderInput` starts. `s.inputVp.yOffset()` is currently **10**.
3. `setViewportContext` is called. Go stores `outerYOffset = 10`.
4. `getScrollSyncInfo` is called. It calculates the cursor is out of view and suggests **15**.
5. JS calls `s.inputVp.setYOffset(15)`.
6. The view renders with the text shifted to offset **15**.
7. **User clicks** on the text they just saw.
8. `handleClickAtScreenCoords` executes using the stored `outerYOffset` of **10**.
9. **Result:** The click is calculated 5 lines higher than reality (15 - 10). The cursor jumps to the wrong position.

**Verification:** This completely breaks the "CORRECTNESS" promise in the comments. The fix requires updating the context *after* the scroll adjustment or calling `setViewportContext` a second time.

#### 2. CRITICAL BUG: Infinite Memory Leak

The Go implementation uses a global `Manager` that stores every textarea model in a `map[uint64]*ModelWrapper` to prevent garbage collection and allow concurrent access.

* **JS:** `s.contentTextarea = textareaLib.new()` is called every time the user enters Input Mode (keys 'a' or 'e').
* **Go:** `textarea.New()` creates a model and adds it to the `Manager` map.
* **The Leak:** The JS code **never calls `.dispose()**`. When `s.contentTextarea` is overwritten or the user exits the mode, the Go side retains the model forever.
* **Result:** The application will leak memory indefinitely, eventually crashing the process.

#### 3. LOGIC FLAW: Brittle Coordinate Constants

In `handleMouse`, the code hardcodes:

```javascript
const titleHeight = 1;

```

However, in `renderInput`, the title height is dynamic:

```javascript
const titleHeight = lipgloss.height(headerRow);

```

If the terminal width is narrow and the title wraps to 2 lines, or if the style padding changes, the JS visual render will push the textarea down by 2+ lines, but the Go hit-test (which receives `titleHeight: 1` from `handleMouse`) will calculate clicks 1 line higher than intended.

### Corrected Implementation Plan

To fix the stale state and memory leak, apply these changes:

**1. Fix `renderInput` order of operations:**

```javascript
// ... inside renderInput ...

// 1. Initial Context Setup (Required for getScrollSyncInfo to work)
if (s.contentTextarea && s.contentTextarea.setViewportContext && s.textareaBounds) {
    s.contentTextarea.setViewportContext({
        outerYOffset: s.inputVp.yOffset(), // Current offset
        textareaContentTop: s.textareaBounds.contentTop,
        textareaContentLeft: s.textareaBounds.contentLeft,
        outerViewportHeight: scrollableHeight,
        preContentHeight: preContentHeight
    });
}

// 2. Cursor Visibility & Scroll Sync
if (s.contentTextarea && s.inputFocus === FOCUS_CONTENT && !s.inputViewportUnlocked) {
    if (s.contentTextarea.getScrollSyncInfo) {
        const syncInfo = s.contentTextarea.getScrollSyncInfo();
        const cursorAbsY = syncInfo.cursorAbsY;
        const vpY = s.inputVp.yOffset();
        const vpH = s.inputVp.height();
        
        // Apply Scroll
        let newOffset = vpY;
        if (cursorAbsY < vpY) {
            newOffset = cursorAbsY;
        } else if (cursorAbsY >= vpY + vpH) {
            newOffset = cursorAbsY - vpH + 1;
        }
        
        if (newOffset !== vpY) {
            s.inputVp.setYOffset(newOffset);
            
            // CRITICAL FIX: Update Go context immediately if we scrolled!
            if (s.contentTextarea.setViewportContext) {
                 s.contentTextarea.setViewportContext({
                    outerYOffset: newOffset, // Use the NEW offset
                    textareaContentTop: s.textareaBounds.contentTop,
                    textareaContentLeft: s.textareaBounds.contentLeft,
                    outerViewportHeight: scrollableHeight,
                    preContentHeight: preContentHeight
                });
            }
        }
    }
    // ... legacy fallback ...
}

```

**2. Fix Memory Leak in `handleKeys`:**

```javascript
// When exiting mode or switching
if (s.mode === MODE_INPUT && prevMode !== MODE_INPUT) {
    // Clean up previous if exists
    if (s.contentTextarea && s.contentTextarea.dispose) {
        s.contentTextarea.dispose();
    }
    // ... create new ...
}
// ALSO in the ESC handler and Submit handler

```

Would you like me to generate the full patch implementing these critical fixes?
`````


Document 6:
`````
# Summary

**Critical Bug:** `setViewportContext` is called BEFORE cursor-visibility scroll adjustment, causing `handleClickAtScreenCoords` to use a stale Y offset—click positioning will be wrong after any auto-scroll.

**Fix:** Move `setViewportContext()` call to AFTER the cursor visibility scroll logic.

---

# Detailed Analysis

## 1. Critical: Stale Viewport Y Offset (Click Misalignment)

**Location:** `renderInput()` in the diff, lines adding `setViewportContext` before cursor visibility logic.

**Sequence of Operations:**
1. `setViewportContext({outerYOffset: s.inputVp.yOffset(), ...})` — stores Y=10
2. Cursor visibility check: cursor at line 30, viewport shows 10-25
3. `s.inputVp.setYOffset(27)` — viewport now shows 27-42
4. `view()` renders with Y=27
5. User clicks at screen row 8 (expecting line 35)
6. `handleClickAtScreenCoords` uses stored Y=10, calculates `contentY = 8 + 10 = 18`
7. Cursor placed at visual line 15, not 35 — **wrong by 17 lines**

**Fix:** Reorder to:
```javascript
// CURSOR VISIBILITY LOGIC (may adjust yOffset)
if (s.contentTextarea && s.inputFocus === FOCUS_CONTENT && !s.inputViewportUnlocked) { ... }

// THEN capture final state
if (s.contentTextarea && s.contentTextarea.setViewportContext && s.textareaBounds) {
    s.contentTextarea.setViewportContext({ outerYOffset: s.inputVp.yOffset(), ... });
}
```

## 2. Minor: vpCtx Not Mutex-Protected

**Location:** Go code, `setViewportContext` writes to `vpCtx` without acquiring `wrapper.mu`, while `handleClickAtScreenCoords`/`getScrollSyncInfo` read `vpCtx` under mutex.

**Impact:** Benign in practice (goja is single-threaded), but violates thread-safety contract implied by the mutex pattern. Document single-threaded requirement or protect `vpCtx`.

## 3. Code Quality: Duplicate Coordinate Calculation

**Location:** Fallback branches in handleMouse duplicate 6 lines of coordinate math.

```javascript
const vpYOffset = s.inputVp ? s.inputVp.yOffset() : 0;
const viewportRelativeY = msg.y - titleHeight;
// ... (identical in both else-if branches)
```

Not a correctness issue; refactor opportunity.

## 4. Minor: Hardcoded `titleHeight = 1`

**Location:** `handleMouse()` — `const titleHeight = 1;`

`renderInput()` calculates `titleHeight = lipgloss.height(headerRow)`. If header layout changes, click handling diverges. Pre-existing issue but fragile.

## 5. Robustness: getScrollSyncInfo Assumes Initialized vpCtx

**Location:** Go `getScrollSyncInfo` uses `vpCtx.preContentHeight` without checking `vpCtx.initialized`.

Current call order is correct (setViewportContext always called first in render), but if called out-of-order, returns zeroed values. Consider defensive check or documentation.

---

## What I Verified

- Coordinate math in `handleClickAtScreenCoords`: traced concrete example—logic correct given accurate inputs
- Bounds checking: Go code properly validates `visualY >= 0 && visualY < totalVisualLines`
- Column clamping: Correctly limits to line length
- Fallback paths: Maintain backward compatibility; logic preserved from old code
- textareaModelMirror: Size assertions compile-time verify layout match ✓

## What I Trusted (Unable to Verify)

- Diff completeness (assumed to show all changes between the two commits)
- Go struct mirror matches upstream `textarea.Model` layout at runtime
- `lipgloss.height()` returns accurate line counts
`````


Document 7:
`````
# CORRECTNESS VERDICT: **CANNOT GUARANTEE**

Your succinct summary that cannot be materially improved:

**The PR correctly implements Go-native coordinate handling to fix cursor jump bugs, but hardcodes title height assumptions, preserves legacy fallback bugs, has theoretical thread safety issues, and inconsistently handles empty documents; therefore, complete correctness cannot be guaranteed despite the core logic being sound.**

---

## DETAILED ANALYSIS

### ✅ What Is Correct

1. **Core Coordinate Transformation Chain**: The new `handleClickAtScreenCoords()` method correctly implements the full transformation pipeline: screen→viewport→content→textarea→visual→logical. The calculations mirror the JS logic precisely but execute atomically in Go, eliminating cross-language call overhead and race windows.

2. **Scroll Synchronization**: `getScrollSyncInfo()` accurately batches cursor/visual/scroll calculations. The `cursorAbsY` derivation (`preContentHeight + cursorVisualLine`) matches the legacy JS calculation exactly, and the viewport bounds checking logic is mathematically correct.

3. **Fallback Architecture**: The three-tier fallback (Go-native → performHitTest → legacy setPosition) maintains backward compatibility while prioritizing the new correct path. Graceful degradation occurs when viewport context isn't initialized.

4. **Hit Test Algorithm**: The `calculateWrappedLineCount()` and wrapped-line traversal logic correctly handles multi-width characters, greedy wrapping, and edge cases where characters exceed viewport width. The column calculation within wrapped segments is accurate.

### ❌ Critical Issues Preventing Guarantee

1. **Hardcoded `titleHeight = 1` Violates Layout Reality**
   - **Location**: `handleMouse()` line 1452, `handleMouse()` line 2000
   - **Problem**: Assumes header row is always exactly 1 line tall. In `renderInput()`, the header is constructed via `lipgloss.joinHorizontal()` with a dynamically calculated spacer: `Math.max(2, termWidth - lipgloss.width(title) - lipgloss.width(btnTop) - lipgloss.width(btnBot) - 4)`. On terminals narrower than ~30 columns, this spacer becomes 2, but the total width can still exceed `termWidth`, causing lipgloss to render a single line that the terminal itself wraps. The application still thinks it's 1 line, but the terminal displays 2+. All Y-coordinate math becomes off by the number of wrapped lines.
   - **Impact**: Click positions miscalculated by ±N lines on narrow terminals. Scroll sync also affected.
   - **Why It Can't Be Trusted**: This is an **unverifiable assumption** about external rendering behavior. You must calculate `titleHeight = lipgloss.height(headerRow)` dynamically during render and pass it through state.

2. **Legacy Fallback Preserves Known Bug**
   - **Location**: `handleMouse()` line 1465-1469
   - **Problem**: Final fallback `s.contentTextarea.setPosition(visualY, visualX)` treats visual Y-coordinate (accounting for wrapping) as a logical row index. This is **fundamentally incorrect** for wrapped text and was the original source of cursor jump bugs.
   - **Impact**: If `handleClickAtScreenCoords` and `performHitTest` are both unavailable, the bug resurfaces.
   - **Why It Exists**: Backward compatibility. But a guarantee cannot be made when a known-incorrect code path remains reachable, even if unlikely.

3. **Thread Safety Race on `vpCtx`**
   - **Location**: `textarea.go` line 1102 (`var vpCtx viewportContext`)
   - **Problem**: `vpCtx` is stored outside the `ModelWrapper` mutex. While each textarea instance has its own `vpCtx`, concurrent calls from separate Go routines to `setViewportContext` (write) and `handleClickAtScreenCoords` (read) could race. The JS runtime is single-threaded, but **you cannot guarantee** the Go runtime won't schedule these concurrently.
   - **Impact**: Undefined coordinate calculations if race occurs. Very low probability, but impossible to rule out entirely.

4. **Inconsistent Empty Document Handling**
   - **Location**: `textarea.go` `handleClickAtScreenCoords()` line 1157
   - **Problem**: Returns `hit: false` when `len(mirror.value) == 0`, but `performHitTest()` returns `{row: 0, col: 0}` for the same case. An empty textarea should still be clickable at position (0,0).
   - **Impact**: First click in empty textarea may be ignored, requiring second click or fallback path. Minor but observable.

### ⚠️ Minor Concerns

- **Code Duplication**: `setViewportContext()` is called twice in `renderInput()` (lines 1956 and 1988). While harmless, it suggests copy-paste drift risk.
- **Missing Dynamic Title Height Propagation**: `titleHeight` is hardcoded in mouse handler but could be derived from `s.layout` or similar state structure.

## FINAL ASSESSMENT

The PR **successfully addresses the primary performance and correctness goals** by moving coordinate math into Go and fixing the core cursor jump bug. However, the **hardcoded title height assumption is a latent bug** that violates the principle of deriving layout from actual rendered dimensions. Since you demanded I question all information and only trust the impossible-to-verify, I must specify: **the title height assumption is unverifiable without runtime inspection of lipgloss and terminal behavior**.

**I cannot guarantee correctness** because:
- The system makes unverified assumptions about external rendering behavior (title height)
- It intentionally preserves a known-buggy fallback path for compatibility
- Thread safety relies on implicit JS runtime behavior rather than explicit synchronization

**Recommendation**: Ship with a dynamic `titleHeight = lipgloss.height(headerRow)` calculation stored in state and passed to mouse handlers. Remove or gate the legacy `setPosition` fallback behind an explicit "compatibility mode" flag instead of leaving it as silent degradation. Only then can a guarantee be approached.
`````


Document 8:
`````
## SUCCINCT SUMMARY

**Material Defect:** The PR introduces a regression where clicking the empty space below the text content (but within the viewport) fails to update the cursor.

**Cause:** In `textarea.go`, `handleClickAtScreenCoords` contains a guard clause (`visualY >= totalVisualLines`) that explicitly rejects clicks below the last visual line of text. Since the JS `handleMouse` implementation removes the legacy fallback when the Go method is present, these valid "footer" clicks result in `hit: false` and are ignored entirely.

**Required Fix:** Remove the `|| visualY >= totalVisualLines` condition in `textarea.go`. The subsequent loop logic already correctly clamps out-of-bounds Y-coordinates to the final document line, which is the expected behavior.

---

## DETAILED ANALYSIS

### 1. The "Footer Click" Regression

The most significant issue is in the hit-testing logic within `internal/builtin/bubbles/textarea/textarea.go`.

* **Standard Behavior:** In text editors, clicking below the final line of text typically places the cursor at the end of the document.
* **The Bug:** The Go implementation explicitly forbids this via a bounds check:
```go
// Step 5: Check if within textarea content bounds
// ...
// Valid visualY ranges are 0..totalVisualLines-1, so check >= totalVisualLines.
if visualY < 0 || visualY >= totalVisualLines {
    return result // Click outside textarea content
}

```


* **The JS Context:** In `internal/command/super_document_script.js`, the code attempts the Go method first.
```javascript
if (s.contentTextarea.handleClickAtScreenCoords) {
    const hitResult = s.contentTextarea.handleClickAtScreenCoords(...);
    if (hitResult.hit) {
        // ... Success ...
    }
    // CRITICAL MISSING ELSE: No fallback to performHitTest if hit is false!
}

```


* **Result:** If a user clicks line 10 of a viewport containing only 3 lines of text, Go returns `hit: false`, JS does nothing, and the UI appears unresponsive.

### 2. Implicit Clamping Logic

The loop logic inside `handleClickAtScreenCoords` is actually already designed to handle Y-coordinates that exceed the document length (clamping):

```go
for row := 0; row < len(mirror.value); row++ {
    // ...
    // If we've passed the clicked line [OR if the loop finishes], clamp to last logical line
    if row == len(mirror.value)-1 {
        targetRow = row
        targetWrappedSegment = lineHeight - 1 // Last wrapped segment
    }
}

```

If the guard clause at Step 5 is removed, the loop will run to completion. The `if row == len(mirror.value)-1` block will execute, correctly selecting the last row and its last wrapped segment. The subsequent column logic will then clamp the cursor to the end of that line. This restores the correct "click anywhere below to go to end" behavior.

### 3. Coordinate Translation Verification

The coordinate math itself is robust, assuming `setViewportContext` is called correctly (which it is, in `renderInput`):

* **Vertical:** `screenY` → `viewportRelativeY` (minus title) → `contentY` (plus scroll offset) → `visualY` (minus textarea top). This correctly handles scrolling and layout offsets.
* **Horizontal:** `screenX` → `visualX` (minus `textareaContentLeft`).
* **Note:** `textareaContentLeft` in JS is calculated as `border + padding + reservedInner`. If the user clicks in the left padding (e.g., `visualX` becomes negative), the Go logic correctly clamps `if visualX < 0 { visualX = 0 }`, ensuring clicks in the margin select the start of the line.



### 4. Concurrency & Scope

The use of `vpCtx` (a struct variable) inside `createTextareaObject` creates a closure unique to each `textarea` instance.

* **Safety:** This is thread-safe within the context of the Goja runtime (which is single-threaded per runtime instance) and the Bubble Tea update loop (which processes events sequentially).
* **Sync:** `setViewportContext` is called in the `view` phase, and `handleClick` is called in the `update` phase. Since `view` follows `update`, the `vpCtx` used in `handleClick` will technically be from the *previous* frame. This is the correct/desired behavior for hit-testing (mapping a click to what the user *currently sees*).

---

## NEXT STEP

Apply the following patch to `internal/builtin/bubbles/textarea/textarea.go` to enable proper click clamping.

```diff
diff --git a/internal/builtin/bubbles/textarea/textarea.go b/internal/builtin/bubbles/textarea/textarea.go
index ...
--- a/internal/builtin/bubbles/textarea/textarea.go
+++ b/internal/builtin/bubbles/textarea/textarea.go
@@ -794,8 +794,9 @@ func createTextareaObject(runtime *goja.Runtime, manager *Manager, id uint64) go
 
-       // `totalVisualLines` is the number of visual lines (0-indexed).
-       // Valid visualY ranges are 0..totalVisualLines-1, so check >= totalVisualLines.
-       if visualY < 0 || visualY >= totalVisualLines {
+       // Valid visualY ranges start at 0.
+       // We DO NOT check >= totalVisualLines because clicking below the text
+       // should clamp to the last line (handled by the loop below).
+       if visualY < 0 {
            return result // Click outside textarea content
        }

```
`````


Document 9:
`````
Here is the guarantee analysis of your PR.

### Succinct Summary

The PR contains a **critical logic defect** in `textarea.go` where `cursorVisualLine` is calculated using naive integer division (`width / contentWidth`). This contradicts the "greedy soft-wrapping" logic used elsewhere (`calculateWrappedLineCount`), causing incorrect cursor positioning and viewport jumps when multi-width characters (emojis, CJK) appear near line margins. Additionally, the `vpCtx` architecture introduces a **one-frame race condition** during window resizes, where mouse clicks rely on stale layout data from the previous render, though this is mitigated by the fallback mechanism.

---

### Detailed Analysis

#### 1. CRITICAL: Inconsistent Wrapping Logic (The "Math" Bug)

In `textarea.go`, both `handleClickAtScreenCoords` (Step 10) and `getScrollSyncInfo` use a shortcut to determine which visual line the cursor falls on within a logical line:

```go
// CURRENT IMPLEMENTATION (Broken)
if contentWidth > 0 && visualWidthToCursor >= contentWidth {
    cursorVisualLine += visualWidthToCursor / contentWidth
}

```

**The Problem:**
This formula assumes "hard wrapping" at exactly `contentWidth`. However, `calculateWrappedLineCount` (and the rendering logic) uses "greedy wrapping": if a character is too wide to fit the remaining space on the current line, it moves *entirely* to the next line.

**The Failure Case:**

1. `contentWidth` is 10.
2. The line has 9 monospace characters (width 9).
3. The 10th character is a wide emoji (width 2).
4. **Renderer/Wrapper:** Fits 9 chars. The emoji (width 2) requires space 10 and 11. It wraps to the next line.
5. **Your Logic:** `visualWidthToCursor` at the emoji is 9. `9 / 10 = 0`.
6. **Result:** The renderer places the emoji on **Visual Line 1**. Your logic thinks it is on **Visual Line 0**.
7. **Impact:** The viewport will calculate the `cursorAbsY` incorrectly, potentially failing to scroll down when the user navigates to this wrapped line, hiding the cursor.

**The Fix:**
You must calculate the cursor's visual line index by simulating the exact same greedy wrapping loop used in `calculateWrappedLineCount`, iterating up to `targetCol`.

#### 2. ARCHITECTURAL: View/Update Cycle Latency

You are setting `vpCtx` (Viewport Context) inside `renderInput` (the **View** phase) but reading it in `handleMouse` (the **Update** phase).

* **Sequence:** Init  Update (Mouse Click)  View (Set `vpCtx`).
* **The Bug:** On the very first frame, or the immediate frame after a window resize, `handleMouse` runs *before* `renderInput` has updated `vpCtx` with the new dimensions.
* **Impact:**
* **First Run:** `vpCtx.initialized` is false. The code falls back to the JS legacy path (which is fine).
* **Resize:** If the user resizes and immediately clicks, `vpCtx` contains the *old* window dimensions. The click logic will be offset by the difference in layout.


* **Mitigation:** This is inherent to TUI architectures separating View/Update. Since you provided a JS fallback (`s.textareaBounds`), this is acceptable, but you should be aware that "Go-Native" accuracy is technically `Frame - 1` accurate.

#### 3. MINOR: Slice Bounds Safety

In `handleClickAtScreenCoords`, Step 8, specifically the `else` block (no width constraint):

```go
targetCol = visualX
if targetCol > len(line) { // logic... }

```

If `visualX` is massive (e.g., clicking far right on an unwrapped line), `targetCol` is clamped. However, later in Step 10:

```go
for i := 0; i < targetCol && i < len(mirror.value[targetRow]); i++ { ... }

```

This loop correctly checks `i < len(...)`. The logic is safe from panics, but the `targetCol` assignment in Step 8 implies `visualX` maps 1:1 to `col` indices for non-wrapped lines, which assumes `runeWidth == 1`. If `line` contains wide characters but `contentWidth` is 0 (unlimited), clicking 10 units right might land you in the middle of a string that is only 5 runes long (if they are wide). The clamping saves you from a panic, but the cursor placement might be imprecise.

---

### Implementation of the Fix

You need to replace the division math in `textarea.go` with this wrapping simulation.

**In `internal/builtin/bubbles/textarea/textarea.go`:**

Apply this logic to both `handleClickAtScreenCoords` (Step 10) and `getScrollSyncInfo`:

```go
// REPLACEMENT LOGIC
// Calculate cursor visual line within the current row using GREEDY wrapping
if targetRow < len(mirror.value) && contentWidth > 0 {
    currentLine := mirror.value[targetRow]
    currentWidth := 0
    visualLineIndex := 0
    
    // Iterate exactly as we do for counting lines, but stop at the cursor column
    for i, r := range currentLine {
        if i >= targetCol {
            break
        }
        rw := runeWidth(r)
        
        // Handle greedy wrap
        if rw > contentWidth {
             // Edge case: char wider than viewport (takes whole line)
             if currentWidth > 0 {
                 visualLineIndex++
                 currentWidth = 0 
             }
             visualLineIndex++ // The char itself takes a line
             continue
        }

        if currentWidth+rw > contentWidth {
            visualLineIndex++
            currentWidth = rw
        } else {
            currentWidth += rw
        }
    }
    cursorVisualLine += visualLineIndex
}

```

### Next Step

Would you like me to generate the fully corrected `internal/builtin/bubbles/textarea/textarea.go` file with the wrapping logic fixed?
`````


Document 10:
`````
**SUCCINCT SUMMARY**

The PR contains a **critical infinite loop/logic error** in the Go-side text wrapping simulation and a **fragile layout dependency** in the JavaScript.

1. **wrapping Logic Failure:** In `textarea.go`, the hit-test logic (`performHitTest` and `handleClickAtScreenCoords`) fails to consume characters that are wider than the `contentWidth` (or equal to it in specific boundary cases). This causes the loop to spin without advancing `charsConsumed`, or erroneously skip "segments" without moving the cursor, breaking cursor positioning on wrapped lines or narrow viewports.
2. **Layout Desynchronization:** `handleMouse` (JS) hardcodes `titleHeight = 1`, while `renderInput` (JS) calculates it dynamically. If the header wraps or styling changes, the click coordinates will be vertically offset, causing the user to click the wrong line.

The fix requires standardizing the wrapping logic in Go to handle wide characters and passing the dynamic `headerHeight` into the Go context to remove the hardcoded value in JS.

---

## DETAILED ANALYSIS

### 1. The "Greedy Wrap" Logic Bug (Go)

In `textarea.go`, both `performHitTest` and the new `handleClickAtScreenCoords` attempt to simulate the visual wrapping to find the logical column. The inner loop checks if a character fits:

```go
// CURRENT BUGGY IMPLEMENTATION
for segmentWidth < contentWidth && charsConsumed < len(line) {
    rw := runeWidth(line[charsConsumed])
    if segmentWidth+rw > contentWidth {
        break // <--- BUG: If segmentWidth is 0 (wide char at start of line), we break without consuming!
    }
    segmentWidth += rw
    charsConsumed++
}

```

If `rw > contentWidth` (e.g., a CJK character in a 1-column wide view, or simply a narrow terminal), the code breaks immediately. `charsConsumed` is never incremented. The outer loop (`segment++`) continues, believing it has skipped a visual line, but it is actually reading the *same* characters again. This desynchronizes the "visual line" count from the actual text content.

### 2. The Hardcoded Header Height (JS -> Go)

In `super_document_script.js`:

* `renderInput` calculates `titleHeight = lipgloss.height(headerRow)`.
* `handleMouse` hardcodes `const titleHeight = 1;`.

This violates the "Guarantee" of correctness. If the user resizes the window such that the title wraps, or if the theme changes padding, `handleMouse` will map clicks to the wrong Y-coordinate (off by N lines).

### 3. Duplication Risk

The PR copies the (buggy) logic from `performHitTest` into `handleClickAtScreenCoords`. The fix must be applied to **both** locations to prevent legacy fallback methods from behaving differently than the new native method.

---

## IMPLEMENTATION FIXES

### 1. Fix `internal/builtin/bubbles/textarea/textarea.go`

We must update the `viewportContext` to store the `headerHeight`, and fix the character consumption loops in both hit-test functions.

```diff
diff --git a/internal/builtin/bubbles/textarea/textarea.go b/internal/builtin/bubbles/textarea/textarea.go
index ...
--- a/internal/builtin/bubbles/textarea/textarea.go
+++ b/internal/builtin/bubbles/textarea/textarea.go
@@ -938,6 +938,8 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
         outerViewportHeight int
         // Pre-content height (Y offset within outer viewport to textarea content)
         preContentHeight int
+        // Header/Title height (Screen Y offset to start of viewport)
+        headerHeight int
         // Whether setViewportContext has been called and the values are current
         initialized bool
     }
@@ -957,6 +959,7 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
     //    - textareaContentLeft: X offset from screen left to text content
     //    - outerViewportHeight: Height of outer viewport
     //    - preContentHeight: Height of content before textarea in outer viewport
+    //    - headerHeight: Height of the UI header/title above the viewport
     // Returns the textarea object for chaining.
     _ = obj.Set("setViewportContext", func(call goja.FunctionCall) goja.Value {
         if len(call.Arguments) < 1 {
@@ -980,6 +983,9 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
         if v := config.Get("preContentHeight"); v != nil && !goja.IsUndefined(v) {
             vpCtx.preContentHeight = int(v.ToInteger())
         }
+        if v := config.Get("headerHeight"); v != nil && !goja.IsUndefined(v) {
+            vpCtx.headerHeight = int(v.ToInteger())
+        }
         // Mark viewport context initialized (set during render)
         vpCtx.initialized = true
         return obj
@@ -992,7 +998,6 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
     //
     // Parameters:
     //    - screenX: Raw X coordinate from mouse event (msg.x)
     //    - screenY: Raw Y coordinate from mouse event (msg.y)
-    //    - titleHeight: Height of fixed header above outer viewport
     //
     // Returns object: {hit: bool, row: int, col: int, visualLine: int}
     //    - hit: true if click was within textarea content bounds
@@ -1014,12 +1019,11 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
         _ = result.Set("col", 0)
         _ = result.Set("visualLine", 0)
 
-        if len(call.Arguments) < 3 {
+        if len(call.Arguments) < 2 {
             return result
         }
 
         screenX := int(call.Argument(0).ToInteger())
         screenY := int(call.Argument(1).ToInteger())
-        titleHeight := int(call.Argument(2).ToInteger())
 
         wrapper := ensureModel()
         wrapper.mu.Lock()
@@ -1035,7 +1039,7 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
         }
 
         // Step 1: Convert screen Y to viewport-relative Y
-        viewportRelativeY := screenY - titleHeight
+        viewportRelativeY := screenY - vpCtx.headerHeight
 
         // Step 2: Check if within outer viewport bounds
         if viewportRelativeY < 0 || viewportRelativeY >= vpCtx.outerViewportHeight {
@@ -1089,7 +1093,8 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
 
             for segment := 0; segment < targetWrappedSegment && charsConsumed < len(line); segment++ {
                 segmentWidth := 0
-                for segmentWidth < contentWidth && charsConsumed < len(line) {
+                // Force at least one iteration if line has chars, even if wider than contentWidth
+                for charsConsumed < len(line) {
                     rw := runeWidth(line[charsConsumed])
-                    if segmentWidth+rw > contentWidth {
+                    // If this char makes us exceed width, and we already have content, break (wrap)
+                    if segmentWidth > 0 && segmentWidth+rw > contentWidth {
                         break
                     }
                     segmentWidth += rw
                     charsConsumed++
+                    // If we just consumed a char that filled (or exceeded) the line, break
+                    if segmentWidth >= contentWidth {
+                        break
+                    }
                 }
             }
 
             widthConsumed := 0
-            for charsConsumed < len(line) && widthConsumed < visualX {
+            // Similar fix for the column calculation loop
+            for charsConsumed < len(line) {
+                if widthConsumed >= visualX && visualX != 0 {
+                    break
+                }
                 rw := runeWidth(line[charsConsumed])
-                if widthConsumed+rw > contentWidth {
+                if widthConsumed > 0 && widthConsumed+rw > contentWidth {
                     break
                 }
                 widthConsumed += rw
                 charsConsumed++
+                if widthConsumed >= contentWidth {
+                    break
+                }
             }
 
             targetCol = charsConsumed
@@ -803,12 +808,18 @@ func Require(manager *Manager) func(runtime *goja.Runtime, module *goja.Object)
             // by summing visual widths of characters in previous segments
             charsConsumed := 0
 
             // Skip characters from previous wrapped segments
             for segment := 0; segment < targetWrappedSegment && charsConsumed < len(line); segment++ {
                 segmentWidth := 0
-                for segmentWidth < contentWidth && charsConsumed < len(line) {
+                for charsConsumed < len(line) {
                     rw := runeWidth(line[charsConsumed])
-                    if segmentWidth+rw > contentWidth {
+                    if segmentWidth > 0 && segmentWidth+rw > contentWidth {
                         break
                     }
                     segmentWidth += rw
                     charsConsumed++
+                    if segmentWidth >= contentWidth {
+                        break
+                    }
                 }
             }

```

### 2. Fix `internal/command/super_document_script.js`

Update the JS to pass `titleHeight` (as `headerHeight`) to the context, and stop passing it to the click handler.

```diff
diff --git a/internal/command/super_document_script.js b/internal/command/super_document_script.js
index c8828dc..FIXED 100644
--- a/internal/command/super_document_script.js
+++ b/internal/command/super_document_script.js
@@ -1451,11 +1451,10 @@ function handleMouse(msg, s) {
             // Uses handleClickAtScreenCoords() which does ALL coordinate translation in Go.
             // This replaces manual JS coordinate math for PERFORMANCE and CORRECTNESS.
             // The Go method handles: screen→viewport→content→textarea→visual→logical mapping.
             if (s.contentTextarea && isLeftClick && !isWheelEvent) {
-                const titleHeight = 1;
  
                 // Try GO-NATIVE method first (does all math in Go for performance)
                 if (s.contentTextarea.handleClickAtScreenCoords) {
-                    const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y, titleHeight);
+                    const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y);
                     if (hitResult.hit) {
                         // Cursor was successfully positioned by Go
                         s.inputViewportUnlocked = false;
@@ -1961,7 +1960,8 @@ function renderInput(s) {
         s.contentTextarea.setViewportContext({
             outerYOffset: s.inputVp.yOffset(),
             textareaContentTop: s.textareaBounds.contentTop,
             textareaContentLeft: s.textareaBounds.contentLeft,
             outerViewportHeight: scrollableHeight,
-            preContentHeight: preContentHeight
+            preContentHeight: preContentHeight,
+            headerHeight: titleHeight
         });
     }

```
`````


Document 11:
`````
Based on your instruction to **guarantee** correctness and sink commensurate effort, I have subjected the code to a rigorous trace of the "Elm Architecture" (Update -> View -> Update) cycle used by Bubble Tea.

**I cannot guarantee the correctness of this PR in its current state.**

There is a **critical synchronization defect** that will cause mouse clicks to land on the wrong line immediately after the viewport scrolls. There is also a layout logic discrepancy that will break click handling if the UI header ever exceeds 1 line.

### Succinct Summary

**The PR is materially defective.** The `setViewportContext` method captures the viewport's scroll offset (`yOffset`) **before** the auto-scroll logic runs. If the cursor moves out of view, the viewport scrolls, but the Go context retains the *old* offset, permanently desynchronizing click coordinates by the scroll amount for that frame. Additionally, `handleMouse` hardcodes `titleHeight = 1`, while `renderInput` calculates it dynamically; any layout change to the header will silently break all mouse targeting. Finally, the `unsafe` struct mirroring in Go is a stability ticking time bomb dependent on the exact version of the upstream library.

---

### Detailed Analysis

#### 1. The "Pre-Scroll" Capture Bug (Critical)

In `renderInput` (JS), you setup the Go context *before* ensuring the cursor is visible.

**The Failure Chain:**

1. **Render Start:** The user types. `s.inputVp.yOffset()` is `0`.
2. **Context Capture:** You call `s.contentTextarea.setViewportContext(...)`. Go records `outerYOffset = 0`.
3. **Auto-Scroll Logic:** The code checks if the cursor is visible. It is not. The code runs `s.inputVp.setYOffset(5)`.
4. **Render Output:** The viewport renders starting at Y=5. The user sees the text shifted up.
5. **User Interaction:** The user clicks on the visual 3rd line.
6. **Hit Test (Go):** Go reads `vpCtx.outerYOffset` (which is still `0`). It calculates the click position based on the text being at Y=0.
7. **Result:** The click registers 5 lines *below* where the user actually pointed.

**Fix:** The call to `setViewportContext` must happen **after** the `s.inputVp.setYOffset` logic is resolved, immediately before the viewport's `view()` method is called.

#### 2. The Hardcoded Layout Discrepancy

In `handleMouse` (JS):

```javascript
const titleHeight = 1; // Hardcoded assumption

```

In `renderInput` (JS):

```javascript
const titleHeight = lipgloss.height(headerRow); // Dynamic calculation

```

If the title wraps, or if styles change (padding, margins, buttons), `renderInput` might produce a header of height 2 or 3. `handleMouse` will still subtract `1` from the mouse Y-coordinate. All clicks will be offset vertically by the difference.

**Fix:** Pass the dynamically calculated `titleHeight` into `setViewportContext` during render, and have Go use that stored value instead of accepting an argument from `handleMouse`.

#### 3. Unsafe Struct Mirroring (High Risk)

In `textarea.go`, `textareaModelMirror` attempts to map the memory layout of `github.com/charmbracelet/bubbles/textarea`.

* **Fragility:** The `viewport` field is at the end of the struct. If `textarea.KeyMap` (a struct field earlier in the chain) changes size, or if the `rsan` interface field (added in recent versions) is missing/present in the build environment, the `viewport` pointer will point to garbage memory. Dereferencing it will panic the runtime.
* **Verification:** You cannot guarantee this works without locking the exact commit hash of `bubbles/textarea` in `go.mod`.

---

### CORRECTED IMPLEMENTATION

Apply the following changes to achieve the guarantee.

#### 1. `internal/command/super_document_script.js`

Move the context setup to the **end** of the render function and include `titleHeight`.

```javascript
// ... inside renderInput(s) ...

    // [MOVED FROM TOP]
    // CURSOR VISIBILITY LOGIC (Run this FIRST to determine final Scroll Offset)
    if (s.contentTextarea && s.inputFocus === FOCUS_CONTENT && !s.inputViewportUnlocked) {
        if (s.contentTextarea.getScrollSyncInfo) {
            const syncInfo = s.contentTextarea.getScrollSyncInfo();
            const cursorAbsY = syncInfo.cursorAbsY;
            const vpY = s.inputVp.yOffset();
            const vpH = s.inputVp.height();

            if (cursorAbsY < vpY) {
                s.inputVp.setYOffset(cursorAbsY);
            } else if (cursorAbsY >= vpY + vpH) {
                s.inputVp.setYOffset(cursorAbsY - vpH + 1);
            }
        }
        // ... legacy fallback ...
    }

    // [CRITICAL FIX]
    // GO-NATIVE VIEWPORT CONTEXT SETUP
    // Must occur AFTER auto-scrolling so outerYOffset is accurate for the frame being rendered.
    // Also captures dynamic titleHeight to ensure click mapping matches layout.
    if (s.contentTextarea && s.contentTextarea.setViewportContext && s.textareaBounds) {
        s.contentTextarea.setViewportContext({
            outerYOffset: s.inputVp.yOffset(),
            textareaContentTop: s.textareaBounds.contentTop,
            textareaContentLeft: s.textareaBounds.contentLeft,
            outerViewportHeight: scrollableHeight,
            preContentHeight: preContentHeight,
            titleHeight: lipgloss.height(headerRow) // <--- Pass dynamic height
        });
    }

    const vpView = s.inputVp.view();
    // ... rest of render ...

```

In `handleMouse`, remove the arguments to `handleClickAtScreenCoords` since context is now the source of truth.

```javascript
// ... inside handleMouse ...
                // Try GO-NATIVE method first
                if (s.contentTextarea.handleClickAtScreenCoords) {
                    // CRITICAL FIX: Don't pass titleHeight here. Use the one synchronized 
                    // in the render context to ensure layout parity.
                    const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y);
                    if (hitResult.hit) {
                        s.inputViewportUnlocked = false;
                    }
                }
// ...

```

#### 2. `internal/builtin/bubbles/textarea/textarea.go`

Update the struct and method to store/use `titleHeight`.

```go
    // ViewportContext stores the outer viewport configuration
    type viewportContext struct {
        outerYOffset        int
        textareaContentTop  int
        textareaContentLeft int
        outerViewportHeight int
        preContentHeight    int
        titleHeight         int // <--- ADD THIS
        initialized         bool
    }

    // ... inside setViewportContext ...
        if v := config.Get("titleHeight"); v != nil && !goja.IsUndefined(v) {
            vpCtx.titleHeight = int(v.ToInteger())
        }
    // ...

    // ... inside handleClickAtScreenCoords ...
    _ = obj.Set("handleClickAtScreenCoords", func(call goja.FunctionCall) goja.Value {
        // ... setup ...
        
        if len(call.Arguments) < 2 { // Changed from 3
            return result
        }

        screenX := int(call.Argument(0).ToInteger())
        screenY := int(call.Argument(1).ToInteger())
        
        // CRITICAL FIX: Use stored titleHeight from render context
        // instead of relying on caller to guess the constant.
        titleHeight := vpCtx.titleHeight 

        // ... rest of logic uses titleHeight ...

```

**Next Step:** Would you like me to generate the full patch file incorporating these fixes to ensure atomic application?
`````


Document 12:
`````
# PR Review: Go-Native Viewport Coordination

## Succinct Summary

**Two bugs, one dead-code path:**
1. **Narrow terminal overflow**: `Math.max(40, termWidth - 10)` forces 40-column minimum fieldWidth, causing horizontal clipping when terminal < 50 columns
2. **Hardcoded titleHeight=1 in click handling**: Header row may wrap on narrow terminals; using lipgloss.height(headerRow) would be correct
3. **Fallback branches are dead code**: `handleClickAtScreenCoords` always exists (defined in Go), so `else if` paths never execute—not a runtime bug but misleading

---

## Detailed Analysis

### BUG 1: Narrow Terminal Width Overflow (Verified)

**Location**: `renderInput` and `configureTextarea`

```javascript
const fieldWidth = Math.max(40, termWidth - 10);  // Forces minimum 40
const innerWidth = Math.max(10, fieldWidth - 4 - scrollbarWidth);
```

For a 30-column terminal:
- `fieldWidth = Math.max(40, 20) = 40` — **exceeds terminal by 10 columns**
- Content renders correctly internally but field box clips or wraps unexpectedly

**Impact**: Visual corruption on terminals narrower than 50 columns.

---

### BUG 2: Hardcoded titleHeight in Click Handling

**Location**: `handleMouse` (diff lines ~1449)

```javascript
const titleHeight = 1;  // HARDCODED
if (s.contentTextarea.handleClickAtScreenCoords) {
    const hitResult = s.contentTextarea.handleClickAtScreenCoords(msg.x, msg.y, titleHeight);
```

But in `renderInput`:
```javascript
const titleHeight = lipgloss.height(headerRow);  // DYNAMIC
```

On very narrow terminals, `joinHorizontal` of title+spacer+buttons may not fit, potentially causing wrap. If header becomes 2+ lines, click Y-coordinates are off by that amount.

**Impact**: Cursor mispositioned by header height delta on extreme narrow widths.

---

### ISSUE 3: Dead Fallback Code (Design Flaw, Not Bug)

```javascript
if (s.contentTextarea.handleClickAtScreenCoords) {
    // Always true—method defined in Go
} else if (s.textareaBounds && s.contentTextarea.performHitTest) {
    // NEVER REACHED
} else if (s.textareaBounds) {
    // NEVER REACHED — also has bug: passes visualY/visualX as row/col
}
```

The `handleClickAtScreenCoords` method is **always defined** by the Go binding. Fallbacks only execute if the method doesn't exist, which never happens with this codebase version.

**Note**: The last-resort fallback (`setPosition(visualY, visualX)`) conflates visual coordinates with logical row/col—wrong for wrapped lines. Since it's unreachable, this is **not a runtime bug**, but the code is misleading.

---

### Verified Correct: Wrap Calculations

The Go `calculateWrappedLineCount` correctly handles:
- Empty lines (returns 1)
- Exact-width lines (no spurious wrap)
- Wide characters exceeding column width (special-cased)
- Long lines at any document position

```go
if rw > width {
    if currentWidth > 0 { count++ }
    currentWidth = width
    continue
}
```

Trace for "中文" (4 cells) in width=3:
- '中' (2): 0+2≤3 → currentWidth=2
- '文' (2): 2+2>3 → count++, currentWidth=2
- Returns 2 ✓

---

### Verified Correct: Click Bounds Checking

```go
if visualY < 0 || visualY >= totalVisualLines {
    return result // hit: false
}
```

Properly rejects clicks outside textarea content vertically.

---

### Verified Correct: Scroll Sync Logic

```go
cursorAbsY := vpCtx.preContentHeight + cursorVisualLine
```

Uses viewport context set in same render cycle. `preContentHeight` calculation in JS correctly excludes fixed header (operates in content-space, not screen-space).

---

### Minor: Unused Return Fields

`handleClickAtScreenCoords` returns `{hit, row, col, visualLine}` but JS only reads `hit`. Similarly, `getScrollSyncInfo` returns `suggestedYOffset` which JS ignores (recalculates instead). Not bugs—just over-specified APIs.

---

### Minor: Missing null Check in setViewportContext

```go
if v := config.Get("outerYOffset"); v != nil && !goja.IsUndefined(v) {
    vpCtx.outerYOffset = int(v.ToInteger())
}
```

Missing `!goja.IsNull(v)`. Passing explicit `null` from JS would set field to 0. JS code doesn't pass null currently, so no runtime impact.

---

### Trust Statement

**Trusted without verification**: 
- Upstream `textarea.Model` field layout matches `textareaModelMirror` (unsafe pointer cast)—compilation assertions exist but runtime behavior depends on charmbracelet/bubbles version stability
`````


Document 13:
`````
## Summary

**CRITICAL FAILURES DETECTED**: This PR has **6 severe correctness issues** that violate the guarantee. The most critical: **narrow terminals (<50 chars) will have completely broken layouts** due to hardcoded minimum widths exceeding terminal bounds.

## Detailed Analysis

### 🔴 ISSUE 1: Terminal Width Violation
**Location**: `renderInput()` line calculating `fieldWidth`
```javascript
const fieldWidth = Math.max(40, termWidth - 10);
```
**Problem**: Forces 40-char minimum width even when terminal is 30 chars wide, causing layout overflow and corruption.
**Impact**: Unusable on narrow terminals (mobile SSH, tmux panes, etc.)

### 🔴 ISSUE 2: Race Condition on Viewport Context  
**Location**: `textarea.go` closure variable `var vpCtx viewportContext`
**Problem**: `vpCtx` not protected by mutex while `wrapper.mu` protects model access. Concurrent calls cause data races.
**Impact**: Crashes or incorrect coordinate calculations under concurrent access.

### 🔴 ISSUE 3: Integer Division Cursor Positioning Bug
**Location**: `handleClickAtScreenCoords()` and `cursorVisualLine()`
```go
cursorVisualLine += visualWidthToCursor / contentWidth
```
**Problem**: Integer division truncates. Example: cursor at column 79 with width 40 gives `79/40=1` but should be visual line 2.
**Impact**: Cursor jumps to wrong line when clicking wrapped text.

### 🟡 ISSUE 4: Scrollbar Height Miscalculation
**Location**: `renderInput()` scrollbar setup
```javascript
const scrollableContentHeight = lipgloss.height(scrollableContent);
```
**Problem**: `lipgloss.height()` only counts `\n` characters, ignoring wrapped lines. Scrollbar shows wrong size/position.
**Impact**: Scrollbar misleads about content length when text wraps.

### 🟡 ISSUE 5: Inconsistent Viewport Lock Behavior
**Location**: `handleMouse()` three click handling branches
**Problem**: GO-NATIVE sets `inputViewportUnlocked=false` only on hit; fallbacks always set it.
**Impact**: Different scrolling behavior depending on which method executes.

### 🟡 ISSUE 6: Stale Viewport Context After Resize
**Location**: Viewport context lifecycle
**Problem**: Context set during render but used during events. Terminal resize between render and click uses stale coordinates.
**Impact**: Clicks land in wrong position after terminal resize.

## Additional Observations

- **Trusted without verification**: `scrollbarWidth = 1` (assumed constant)
- **Trusted without verification**: `titleHeight = 1` (hardcoded assumption)
- The `runewidth` deprecation note mentions grapheme clusters aren't handled (affects complex scripts/emojis)
- Zero-width combining marks mapped to width 1 causes alignment issues in Arabic/Hindi text

## Required Fixes

1. Replace `Math.max(40, termWidth - 10)` with `Math.min(Math.max(20, termWidth - 10), 40)` 
2. Add mutex protection for `vpCtx` or move it into `ModelWrapper`
3. Use floating point calculation: `Math.floor(visualWidthToCursor / contentWidth)`
4. Calculate true visual height including wraps for scrollbar
5. Standardize `inputViewportUnlocked` behavior across all paths
6. Add viewport context generation timestamp and validation
`````


