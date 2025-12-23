# Consolidated PR Analysis: Super-Document Implementation

**Status:** 🔴 **REJECT / CHANGES REQUESTED**
**Review Consensus:** The PR correctly resolves several critical crashes and architectural issues but fails to meet specific UI/UX requirements defined in `AGENTS.md`. Furthermore, the PR includes stale/contradictory artifacts that misrepresent the code state.

---

## 1. Executive Summary

The Pull Request prevents the application from crashing and correctly implements the backend `shell` mode renaming. However, it cannot be merged in its current form due to **three categories of issues**:

1. **Process Failures:** Inclusion of contradictory files (`review.md`, `WIP.md`) and failure to consolidate the command name.
2. **UI/UX Requirement Violations:** The text editor has a fixed height (violating the "Must Grow" directive) and lacks cursor visibility styling.
3. **Functional Bugs:** Mouse click targets for document management are misaligned, and the input form lacks cursor-tracking scrolling.

---

## 2. False Positives (Correction of `review.md`)

The PR includes a file named `review.md` which claims the PR is broken for reasons that are **demonstrably false** upon code inspection. The reviewer must ignore `review.md` and focus on the actual code:

* **FALSE:** "Input Viewport Resets (Infinite Loop)."
* **Reality:** The code correctly initializes `inputVp` in `initialState` and persists it. The render loop reuses the instance. The scroll position is preserved.


* **FALSE:** "Mouse Coordinate Off-by-One / Header Height."
* **Reality:** The `headerHeight = 4` assumption in `handleMouse` is **correct**. `renderList` produces: Title (0) + Gap (1) + Count (2) + Gap (3) = 4 lines before content.


* **FALSE:** "Missing Input Event Routing."
* **Reality:** `handleKeys` and `handleMouse` both explicitly route `PgUp`, `PgDn`, and `Wheel` events to the input viewport when in `MODE_INPUT`.

---

## 3. Verified Fixes (Do Not Change)

The following implementations have been verified as **CORRECT** and should be preserved in the next iteration:

* ✅ **Nil Pointer Panic:** The crash on the "Submit" button is fixed by safely transitioning focus and checking for the textarea instance before access.
* ✅ **Viewport Persistence:** Moving the input viewport instantiation out of the render loop fixes the scroll reset bug.
* ✅ **Shell Mode Rename:** All references to `replMode` have been correctly updated to `shellMode` (Go struct, flags, JS injection).
* ✅ **Button Layout:** "View", "Edit", and "Generate" buttons were correctly removed. Remaining buttons are correctly placed within the scrollable content area.

---

## 4. Critical Defects (Blocking Merge)

### A. Violation of "Input Growth" Requirement

* **Source:** `AGENTS.md` ("textedit MUST be allowed to grow... entire page should scroll").
* **Defect:** `renderInput` sets a **fixed height** on the textarea: `s.contentTextarea.setHeight(DESIGN.textareaHeight)` (hardcoded to 6).
* **Result:** The textarea has an internal scrollbar while the outer page scrollbar is never used.
* **Fix:** Calculate height dynamically: `Math.max(minHeight, s.contentTextarea.lineCount())` and update the viewport content accordingly.

### B. Broken Document Click Targets

* **Source:** `internal/command/super_document_script.js`
* **Defect:** Logic Mismatch.
* **Visual Layout:** Line 0 (Header), Line 1 (Preview), Line 2 (Remove Button).
* **Click Handler:** Expects Rename at Line 1, Delete at Line 3.


* **Result:** Clicking the "Remove" button (Line 2) does nothing. Clicking the empty space below it (Line 3) triggers deletion.
* **Fix:** Align `handleMouse` targets to `relativeLine === 0` (Rename/Edit) and `relativeLine === 2` (Remove).

### C. Missing Cursor Tracking (Input Mode)

* **Defect:** While `PgUp`/`PgDn` scroll the input page, typing text that pushes the cursor off the bottom of the screen does **not** auto-scroll the viewport.
* **Result:** Users end up typing blindly into a "black void" off-screen.
* **Fix:** Logic must be added to compare the cursor's Y position with the viewport's Y offset and scroll the outer `inputVp` automatically during text entry.

### D. Missing Styling ("Black Void")

* **Defect:** No style configuration is applied to the textarea.
* **Result:** On many terminal themes, the default cursor and text colors result in poor contrast or invisibility.
* **Fix:** Explicitly inject `s.contentTextarea.focusedStyle` configurations in `initialState`.

### E. Command Name Mismatch

* **Source:** `AGENTS.md` ("consolidating doc-list into JUST list").
* **Defect:** The command is still registered as `"doc-list"`.
* **Fix:** Rename registry key and usage to `"list"`.

---

## 5. Required Actions

To resolve this PR, the developer must:

1. **Rename Command:** Change registry from `"doc-list"` to `"list"`.
2. **Delete Artifacts:** Remove `review.md` and `WIP.md`.
3. **Fix Click Targets:** Adjust `handleMouse` logic to match `buildLayoutMap` (Remove button is at index 2).
4. **Implement Auto-Expanding Textarea:** Remove fixed height; allow textarea to expand and drive the outer viewport scrolling.
5. **Wire Styles:** Apply theme colors to the textarea cursor and text.
6. **Add Input Scrollbar:** Ensure the outer `inputVp` renders a visible scrollbar when content exceeds screen height.
7. **Fix terminal state reset:** Ensure that the bubbletea integration properly restores the terminal state on exit, e.g. on click of quit.
