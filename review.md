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

## 2. Critical Defects (Blocking Merge)

### A. Violation of "Input Growth" Requirement

* **Source:** `AGENTS.md` ("textedit MUST be allowed to grow... entire page should scroll").
* **Defect:** `renderInput` sets a **fixed height** on the textarea: `s.contentTextarea.setHeight(DESIGN.textareaHeight)` (hardcoded to 6).
* **Result:** The textarea has an internal scrollbar while the outer page scrollbar is never used.
* **Fix:** Calculate height dynamically: `Math.max(minHeight, s.contentTextarea.lineCount())` and update the viewport content accordingly.

### B. Missing Cursor Tracking (Input Mode)

* **Defect:** While `PgUp`/`PgDn` scroll the input page, typing text that pushes the cursor off the bottom of the screen does **not** auto-scroll the viewport.
* **Result:** Users end up typing blindly into a "black void" off-screen.
* **Fix:** Logic must be added to compare the cursor's Y position with the viewport's Y offset and scroll the outer `inputVp` automatically during text entry.

### C. Missing Styling ("Black Void")

* **Defect:** No style configuration is applied to the textarea.
* **Result:** On many terminal themes, the default cursor and text colors result in poor contrast or invisibility.
* **Fix:** Explicitly inject `s.contentTextarea.focusedStyle` configurations in `initialState`.

**WARNING:** This may actually be a bug - can't tell, it's just a black void.

### D. Command Name Mismatch

* **Source:** `AGENTS.md` ("consolidating doc-list into JUST list").
* **Defect:** The command is still registered as `"doc-list"`, and that command (as implemented) **fails to include the critical context element ids**
* **Fix:** Rename registry key and usage to `"list"`. **N.B. FIX the summary to actually use the baseline list command but EXTEND it, a la `osm prompt-flow`.**

---

## 3. Required Actions

To resolve this PR, the developer must:

1. **Rename Command:** Change registry from `"doc-list"` to `"list"`. **N.B. FIX the summary to actually use the baseline list command but EXTEND it, a la `osm prompt-flow`.**
2. **Delete Artifacts:** Remove `review.md` and `WIP.md`.
3. **Fix Click Targets:** Adjust `handleMouse` logic to match `buildLayoutMap` (Remove button is at index 2).
4. **Implement Auto-Expanding Textarea:** Remove fixed height; allow textarea to expand and drive the outer viewport scrolling.
5. **Wire Styles:** Apply theme colors to the textarea cursor and text.
6. **Add Input Scrollbar:** Ensure the outer `inputVp` renders a visible scrollbar when content exceeds screen height.
7. **Fix terminal state reset:** Ensure that the bubbletea integration properly restores the terminal state on exit, e.g. on click of quit.
8. **ADDRESS ALL OTHER ITEMS IDENTIFIED IN `AGENTS.md`**: The PR must fully comply with all requirements specified in `AGENTS.md`, including but not limited to button layout, navigation behavior, and event handling.
