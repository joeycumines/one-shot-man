# WIP: Super-Document TUI Fixes

## Critical Bugs (from review.md)

1. **Input Viewport State Reset** (lines 1255-1264 in super_document_script.js)
   - BUG: `viewportLib.new()` is called inside `renderInput()` render loop
   - RESULT: Viewport scroll position resets to 0 on every frame
   - FIX: Move `inputVp` to `initialState` like `vp` for document list

2. **Missing Input Event Routing**
   - BUG: No scroll handling for MODE_INPUT in `handleMouse` or `handleKeys`
   - FIX: Add pgup/pgdown/wheel handling for input mode viewport

3. **Mouse Coordinate Alignment**
   - Verified headerHeight = 4 is correct (title + blank + docsLine + blank before viewport = 4 lines)

## Plan

- [x] Task 1: Move `inputVp` to initialState (persist viewport state)
- [x] Task 2: Wire up mouse wheel and keyboard navigation in MODE_INPUT
- [x] Task 3: Verify headerHeight is correct (4 lines, not 3)
- [x] Task 4: Update tests for removed buttons (Edit/View/Generate)
- [x] Task 5: Run full test suite

## Deviations

- The review.md claimed headerHeight should be 3, but after analyzing renderList(), it's actually 4 (header=3 lines + 1 blank before viewport). Kept as 4.
