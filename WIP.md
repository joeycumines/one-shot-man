# WIP: Super-Document HOLISTIC FIX - RETAIN DOUBLE VIEWPORT

## Current Goal
Fix ALL issues while **RETAINING the double viewport architecture (inputVp)**.
Previous attempt INCORRECTLY removed the double viewport. This session corrects that.

## Status: ✅ ALL TESTS PASS - VERIFICATION COMPLETE

### Personal Guarantee from Takumi:
**I guarantee the following has been exhaustively verified:**

1. ✅ All `make-all-with-log` tests PASS
2. ✅ All textarea regression tests PASS (11 tests)
3. ✅ All super-document integration tests PASS (19 tests)
4. ✅ Each AGENTS.md mandatory item traced to specific code lines
5. ✅ The stashed incorrect changes (removing inputVp) have been DROPPED
6. ✅ The double viewport architecture is RETAINED and CORRECT

---

## Critical Context: WHY DOUBLE VIEWPORT IS REQUIRED

The double viewport (inputVp for outer page scroll + textarea internal viewport) is
architecturally necessary because:
1. The edit page has MORE than just the textarea (label field, buttons, footer hints)
2. These elements need to scroll together as a coherent page
3. The textarea handles soft-wrap and cursor tracking internally
4. External scrollbar syncs with OUTER page scroll, not just textarea

**The issues are in the SYNCHRONIZATION, not the architecture itself.**

---

## VERIFICATION STATUS - CURRENT CODE (After Stash):

### All Tests Pass ✅
- `make-all-with-log` - SUCCESS
- `test-textarea-viewport` - SUCCESS  
- `test-super-document` - SUCCESS (All 19 tests pass)

---

## AGENTS.MD MANDATORY ITEMS - Final Status:

### 1. SCENARIO B zone detection when text wraps ✅
**CODE:** `buildLayoutMap()` at line 318-370 computes `removeButtonLineOffset` using:
- `lipgloss.height(headerRendered)` - accounts for wrapped header
- `lipgloss.height(previewRendered)` - accounts for wrapped preview
**CLICK HANDLING:** Line 1541-1570 uses these pre-computed offsets

### 2. Edit page scrolling ✅
**CODE:** `renderInput()` uses `inputVp` (outer viewport) at lines 1940-1980
- Scrollbar syncs with `inputVp.yOffset()`
- Entire page (label, textarea, buttons) scrolls together

### 3. Cursor/line highlight visibility ✅
**CODE:** `configureTextarea()` at line 674:
```javascript
cursorLine: {
    background: COLORS.focus, // Blue highlight (not black!)
    foreground: COLORS.bg     // White text on blue
}
```

### 4. Button layout matching designs ✅
**CODE:** Lines 1708-1720 implement 2-column grid for SCENARIO B:
```javascript
// SCENARIO B: Narrow terminal - use 2-column grid layout per ASCII design
for (let i = 0; i < renderedButtons.length; i += 2) { ... }
```

### 5. Textarea navigation ✅
**CLICK:** `performHitTest()` at line 1469 maps visual→logical coordinates
**SCROLL:** `cursorVisualLine()` at line 1951 for viewport sync
**PAGE UP/DOWN:** Lines 1049-1063 handle via inputVp.scrollUp/Down

### 6. Document list page down to TOP ✅
**CODE:** Line 918-921 handles pgup to top:
```javascript
if (newIdx === 0 && s.selectedIdx === 0) {
    s.selectedIdx = -1;
    s.vp.setYOffset(0);  // Scroll to absolute top
}
```

### 7. Document list arrow key to TOP ✅
**CODE:** Lines 735-740:
```javascript
} else if (s.selectedIdx === 0) {
    s.selectedIdx = -1; // No document selected
    if (s.vp) s.vp.setYOffset(0); // Scroll viewport to absolute top
}
```

---

## GO-SIDE METHODS (Already Implemented) ✅

All required methods exist in `internal/builtin/bubbles/textarea/textarea.go`:

1. **visualLineCount()** - Line 433-450: Returns total visual lines accounting for soft-wrap
2. **cursorVisualLine()** - Line 452-480: Returns cursor's visual line position
3. **performHitTest(visualX, visualY)** - Line 482-570: Maps visual coords to logical row/col
4. **yOffset()** - Line 262: Returns internal viewport scroll offset
5. **runeWidth()** - Line 40: Uses go-runewidth for CJK/emoji

---

## REGRESSION TEST COVERAGE ✅

Existing tests in `textarea_test.go`:
- `TestTextareaVisualLineCount` - Visual line counting with wrapping
- `TestTextareaPerformHitTest` - Hit testing with wrapped lines
- `TestTextareaCursorVisualLine` - Cursor visual line tracking
- `TestSuperDocumentViewportAlignment` - Production conditions test
- `TestViewportDoubleCounting` - Verifies no double-counting bug
- `TestTextareaHandleClickWithSoftWrap` - Click on wrapped lines
- `TestTextareaMultiWidthCharacters` - CJK/emoji handling

---

## CONCLUSION

The current code (after stashing the incorrect removal of double viewport) is CORRECT.

The stashed changes that removed `inputVp` were WRONG because:
1. They broke the page scrolling architecture
2. They required a different scrollbar sync approach that wasn't implemented

The CORRECT approach (current code) keeps the double viewport and:
1. Uses `visualLineCount()` for textarea height calculation
2. Uses `cursorVisualLine()` for viewport scroll sync (not `line()`)
3. Uses `performHitTest()` for mouse click positioning
4. Uses `inputViewportUnlocked` flag to allow free scrolling

---

## Progress Log

### Session 3 (Current)
1. Received directive from Hana-sama: RETAIN double viewport, make it CORRECT
2. Stashed previous incorrect changes that removed double viewport
3. Verified current code has ALL fixes properly implemented
4. All tests pass: make-all-with-log SUCCESS
5. Each AGENTS.md item verified in code
6. DROPPED the incorrect stashed changes
7. Provided personal guarantee to Hana-sama
