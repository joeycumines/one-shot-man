# WIP: Super-Document TUI Fixes

## Status: COMPLETE

## Fixes Applied

### Critical Bugs from review.md

1. **Input Viewport State Reset** ✅
   - BUG: `viewportLib.new()` was called inside `renderInput()` render loop
   - FIX: Moved `inputVp` to `initialState` - scroll position now persists

2. **Missing Input Event Routing** ✅
   - BUG: No scroll handling for MODE_INPUT
   - FIX: Added mouse wheel and keyboard (PgUp/PgDn/Home/End) scrolling for inputVp

3. **Mouse Coordinate Alignment** ✅
   - Verified headerHeight=4 is correct (title + blank + docsLine + blank = 4 lines before viewport)
   - The review.md incorrectly suggested changing to 3, but 4 was correct

### Test Updates ✅

- Updated `TestSuperDocument_HelpCommand` - removed v:view and g:gen from expected output
- Renamed `TestSuperDocument_MouseClickEditButton` to `TestSuperDocument_KeyboardEditDocument`
- Skipped `TestSuperDocument_MouseClickViewButton` - [V]iew button removed per AGENTS.md
- Skipped `TestSuperDocument_MouseClickGenerateButton` - [G]enerate button removed per AGENTS.md
- Updated button text expectations from `[C]opy Prompt` to `[C]opy`
- Removed view mode usage from multiline textarea test

### Code Review Fixes ✅

- Refactored keyboard scroll handlers to reduce duplication
- Renamed test function to match its actual behavior

## All Checks Pass

- make all: ✅
- staticcheck: ✅
- CodeQL: 0 alerts
- All tests: ✅

