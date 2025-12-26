# WIP: Super-Document Review Fixes

## Current Goal
Fix ALL issues identified in `review.md` AND `AGENTS.md` for `osm super-document` command. Personal guarantee to Hana-sama.

## Status: ALL ITEMS COMPLETE ✅

## Completed Phases

### Phase 1: Analysis & Test Replication ✅
- [x] Read review.md
- [x] Read super_document_script.js
- [x] Read textarea.go
- [x] Read existing tests
- [x] Baseline tests passed with `make-all-with-log`

### Phase 2: Core Fixes in Go (textarea.go) ✅
- [x] Implemented `visualLineCount()` method (accounts for soft-wrapping)
- [x] Implemented `performHitTest(visualX, visualY)` method
- [x] Used `runewidth` for multi-width character handling
- [x] Used `contentWidth = width - promptWidth` for proper content area calculation
- [x] Added comprehensive regression tests
- [x] All existing and new tests pass

### Phase 3: JS Fixes (super_document_script.js) ✅
- [x] Updated height calculation to use `visualLineCount()`
- [x] Updated mouse handling to use `performHitTest()` instead of manual coord translation

### Phase 4: Verification ✅
- [x] Full test suite passes: `make-all-with-log` SUCCESS

## AGENTS.md Mandatory Items - ALL COMPLETE ✅

1. **Bug: SCENARIO B zone detection when text wraps** ✅ FIXED
   - Modified `buildLayoutMap()` to compute actual line offsets for header, preview, and removeBtn
   - Updated click handler to use pre-computed offsets instead of hardcoded height-3/height-2
   
2. **Edit page scrolling** ✅ ALREADY WORKING
   - Verified: Outer viewport (`inputVp`) scrolls entire page including buttons
   - Textarea grows to full height via `visualLineCount()`
   - Scrollbar via `inputScrollbar`

3. **Cursor/line highlight visibility** ✅ FIXED
   - Added explicit cursor block styling via `setCursorStyle()`
   - foreground: COLORS.warning (amber/yellow) for visibility
   - background: COLORS.primary (indigo) for contrast
   - CursorLine already has blue background with white text

4. **Button layout matching designs** ✅ ALREADY CORRECT
   - SCENARIO B: 2-column grid layout already implemented
   - Buttons: Add, Load, Copy, Shell, Reset, Quit (matches ASCII design)

5. **Textarea navigation** ✅ IMPLEMENTED
   - `performHitTest()` handles click-to-position correctly
   - pgup/pgdown scroll outer viewport (correct per design)
   - Textarea receives all valid keyboard events via `update(msg)`

6. **Document list page navigation** ✅ FIXED
   - Arrow up at first document now deselects (`selectedIdx = -1`)
   - PgUp at first document now deselects and scrolls to top
   - Arrow down from deselected state selects first document
   - Enables "de-highlight everything and get to the top"

## Progress Log

### Session Progress
1. Analyzed review.md issues
2. Created WIP.md tracking plan
3. Implemented `visualLineCount()` in Go with runewidth support
4. Implemented `performHitTest()` in Go with soft-wrap awareness
5. Updated JS to use new Go methods
6. Added comprehensive regression tests
7. Fixed tests to disable prompt and line numbers for predictable width control
8. Fixed zone detection bug in SCENARIO B by computing actual line offsets
9. Added cursor block styling for visibility
10. Fixed document list navigation to support "no selection" state
11. **ALL TESTS PASS** - `make-all-with-log` SUCCESS
12. **ALL MANDATORY ITEMS COMPLETE**
