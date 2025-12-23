### Succinct Summary

The PR is **incorrect and must be rejected** in its current form. While the submission logic fixes (specifically the `nil` pointer crash) and the shell mode renaming are verified as correct, the PR introduces **critical functional regressions** regarding UI usability and input handling.

The most severe issues render the application unusable on smaller terminals:

1. **Input Form Unusable:** The input viewport re-instantiates on every frame, resetting scroll position to zero and making bottom buttons ("Submit", "Cancel") unreachable.
2. **Mouse Logic Broken:** A hardcoded header height creates an off-by-one error in click detection, breaking document selection.
3. **Missing Input Routing:** There is no logic to route scroll events (mouse wheel or keys) to the input viewport.

---

### Detailed Analysis

#### 1. Critical Regression: Input Viewport & Scrolling

The implementation of the scrollable input form (`renderInput`) is functionally broken due to a combination of state mismanagement and missing event routing.

**A. Ephemeral Viewport State (Infinite Reset Loop)**
In `internal/command/super_document_script.js`, the viewport logic is placed directly inside the render loop:

```javascript
// renderInput
if (scrollableContentHeight <= scrollableHeight) {
    visibleContent = scrollableContent;
} else {
    // BUG: new() is called inside the render loop.
    // This creates a fresh viewport with yOffset=0 on every single frame/update.
    const inputVp = viewportLib.new(termWidth - scrollbarWidth, scrollableHeight);
    inputVp.setContent(scrollableContent);
    visibleContent = inputVp.view();
}

```

* **The Flaw:** `viewportLib.new` is called on every render cycle (every keystroke, cursor blink, or state change).
* **The Result:** The viewport instance is not persisted in the application state. Consequently, the `yOffset` resets to `0` immediately after any update.
* **Impact:** Users cannot scroll down. On terminals where the form height exceeds the screen height, the "Submit" and "Cancel" buttons are permanently clipped and unreachable.

**B. Missing Input Event Routing**
Even if the viewport state were persisted, the PR lacks the necessary logic to control it:

* **Mouse Wheel:** The `handleMouse` function explicitly guards scroll events for `MODE_LIST`. There is no corresponding block for `MODE_INPUT`.
* **Keyboard:** `handleKeys` for `MODE_INPUT` handles text entry and focus cycling but does not map navigation keys (`PgUp`, `PgDn`) to the `inputVp`.

**C. Missing Visual Indicators**
While the internal `textarea` has a scrollbar, the outer `inputVp` (which handles the overflow of the whole form) lacks a visual scrollbar implementation, leaving users unaware that content is hidden below the fold.

**Correction Required:**
The `inputVp` must be instantiated in `initialState` (persisted like `s.vp`) and updated in the render loop. Additionally, `handleMouse` and `handleKeys` must be updated to route scroll events to this viewport when in `MODE_INPUT`.

#### 2. Critical Bug: Mouse Coordinate Off-By-One Error

The hit-testing logic in `handleMouse` is decoupled from the rendering logic in `renderList`, leading to misalignment.

**The Code:**

```javascript
// internal/command/super_document_script.js - handleMouse
const headerHeight = 4; // Hardcoded assumption
const vpTop = headerHeight;
const viewportRelativeY = clickY - vpTop;

```

**The Reality:**
`renderList` produces a header of only **3 lines**:

1. Title Line
2. Gap (Empty String)
3. Docs Line (Count)

**Consequences:**

* Clicks on **Row 3** (the first line of the first document) are ignored because the logic expects the list to start at **Row 4**.
* Clicks on **Row 4** correspond to `relativeY = 0` (top of document), but visually the user is clicking the *second* line of the first document.
* This breaks "Remove" button targets and document selection.

**Correction Required:**
Update `handleMouse` to use `const headerHeight = 3;` or align `renderList` to match the hardcoded expectation.

#### 3. Verified Fixes and Improvements

Despite the regressions, the following changes in the PR are confirmed **correct**:

* **Submit Crash Fix:** The PR correctly resolves the panic where `s.contentTextarea` is accessed while `null`. The added safe access check (`if (s.inputFocus === FOCUS_CONTENT && s.contentTextarea)`) and the forced focus transition to `FOCUS_SUBMIT` before handling `ctrl+enter` are sound.
* **Shell Mode:** The transition from `replMode` to `shellMode` (Go struct, Flags, and JS injection) is consistent.
* **UI Cleanup:** The removal of `btn-edit`, `btn-view`, and `btn-generate` aligns with directives.
