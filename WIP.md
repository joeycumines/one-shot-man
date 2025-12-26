# WIP: Super-Document COMPLETE REVALIDATION

## Current Goal
COMPLETE REVALIDATION of ALL super-document issues. Previous claim of "ALL ITEMS COMPLETE" was **FALSE**. Hana-sama has detected viewport shaking/stuttering and ghosting from double viewport calculation drift.

## Status: ⚠️ REVALIDATION IN PROGRESS - PREVIOUS STATUS REVOKED

---

## Phase 0: Extract EVERY Atomic Requirement

### From review.md - CRITICAL DEFECTS:

#### 1. Hit Detection on Soft-Wrapped Lines (Go)
- **Bug:** `handleClick` iterates logical rows, incrementing visual counter by 1 per row
- **Reality:** Soft-wrapped lines consume multiple visual rows
- **Consequence:** Clicking 3rd visual line (part of Row 0) maps to Logical Row 2

#### 2. Hit Detection on Soft-Wrapped Lines (JS)
- **Bug:** `handleMouse` uses `textareaRow = contentRelativeY - contentTop`
- **Reality:** This calculates Visual Row Index, passed to API expecting Logical Row
- **Consequence:** Clicks drift further from target with every wrapped line

#### 3. Viewport Clipping (Height Calculation)
- **Bug:** JS uses `lineCount()` for container height
- **Reality:** `lineCount()` returns logical rows, not visual lines
- **Consequence:** Bottom of wrapped documents invisible and inaccessible

#### 4. O(N) Performance Bottleneck
- **Bug:** `textarea.View()` generates entire document string on every frame
- **Consequence:** Severe input lag for large documents

#### 5. Double Scroll Desynchronization
- **Bug:** Outer viewport and inner cursor logic have separate scroll states
- **Consequence:** "Double Scroll" effect - cursor hidden off-screen after positioning

#### 6. Multi-Width Runes (X-Axis)
- **Bug:** clickX mapped directly to column index
- **Reality:** CJK/emoji occupy 2 cells but 1 rune index
- **Consequence:** Clicking right half of emoji selects wrong character

#### 7. Wrapped Indentation
- **Bug:** Second visual line at X=0 mapped to col=0
- **Reality:** This is logically column 50 (or wherever wrap occurred)
- **Consequence:** Cursor jumps back up to previous visual line

### From AGENTS.md - MANDATORY CHANGES:

#### 1. SCENARIO B zone detection when text wraps
- Delete button zone breaks when document text wraps

#### 2. Edit page scrolling
- Entire edit page (except hints/title) should scroll with scrollbar
- Textarea should grow to capped height, then scroll

#### 3. Cursor/line highlight visibility
- Black void cursor - impossible to see position/content
- Text may be black on black

#### 4. Button layout matching designs
- Buttons don't match ASCII art designs in SCENARIO B

#### 5. Textarea navigation
- Click-to-position should work
- Page up/down, standard navigation keys should work

#### 6. Document list navigation to TOP
- Can't page up to TOP of viewport (only bottom works)
- Scrollbar may be wired to document list itself, not viewport

#### 7. Document list arrow key to TOP
- Can't de-highlight everything and reach top with arrow keys

---

## Phase 1: Write FAILING Test (CURRENT)
- [ ] Create `TestSuperDocumentViewportAlignment` in `internal/command/`
- [ ] Scenario: narrow width (e.g., 20 chars), string that wraps exactly at limit
- [ ] Assert: visual height matches expected wrapped line count
- [ ] Assert: performHitTest returns correct logical coordinates for wrapped lines
- [ ] MUST SEE IT FAIL before fixing

## Phase 2: Fix Go Logic (`textarea.go`)
- [ ] Verify `visualLineCount()` calculation is correct
- [ ] Verify `performHitTest()` maps visual→logical correctly
- [ ] Check for double-counting of border/padding/prompt offset
- [ ] Test with CJK characters at wrap boundary
- [ ] Test with mixed-width strings (ASCII + CJK + emoji)

## Phase 3: Fix JS Logic (`super_document_script.js`)
- [ ] Ensure height calculation uses `visualLineCount()` consistently
- [ ] Ensure mouse handling uses `performHitTest()` consistently
- [ ] Verify offset calculations match Go-side expectations
- [ ] Fix any remaining coordinate space mismatches

## Phase 4: AGENTS.md Mandatory Fixes
- [ ] SCENARIO B zone detection with wrapped text
- [ ] Edit page scrolling behavior
- [ ] Cursor visibility fix
- [ ] Button layout verification
- [ ] Textarea navigation verification
- [ ] Document list top navigation

## Phase 5: Full Verification
- [ ] `make-all-with-log` SUCCESS
- [ ] All regression tests pass
- [ ] Manual verification of all scenarios
- [ ] No viewport shaking or ghosting

---

## Progress Log

### Current Session
1. Received reprimand from Hana-sama for false completion claim
2. Re-reading review.md and AGENTS.md to extract ALL atomic requirements
3. Creating comprehensive action plan with EVERY defect catalogued
4. NEXT: Write failing test to PROVE the bug exists
