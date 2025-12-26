# PR Review: Super-Document Analysis

## Executive Summary

**Verdict: REJECT.**

The Pull Request is **materially incorrect**, **architecturally flawed**, and **unsafe for production**. It fundamentally mishandles the distinction between **Visual Coordinates** (screen rows) and **Logical Coordinates** (data rows), guaranteeing data corruption and cursor desynchronization for any document containing soft-wrapped text (long lines) or multi-width characters (emojis, CJK).

Additionally, the implementation relies on an **"Infinite Height" anti-pattern** to delegate scrolling to an external viewport. This creates a catastrophic **O(N)** performance bottleneck by forcing the serialization of the entire document on every frame and leads to content truncation where the bottom of wrapped documents becomes invisible.

## Detailed Defect Analysis

### 1. Critical Failure: Hit Detection on Soft-Wrapped Lines

The most severe defect lies in the coordinate mapping logic in both JavaScript and Go. The implementation relies on the linear fallacy that **1 Logical Row = 1 Visual Line**.

* **The Bug (Go - `textarea.go`):** The `handleClick` implementation iterates over `mirror.value` (logical rows) and increments a visual counter by exactly 1 for each row.
* *Evidence:* The code explicitly comments: `// For simplicity, assume no soft-wrapping for now`.
* *Reality:* `bubbles/textarea` soft-wraps text based on `Width`. If Logical Row 0 wraps into 3 visual lines, it consumes 3 units of Y-space.
* *Consequence:* If a user clicks the 3rd visual line (part of Row 0), the PR logic maps this to **Logical Row 2**. The cursor jumps blindly to the wrong paragraph, destroying the editing context.


* **The Bug (JS - `super_document_script.js`):** The `handleMouse` function performs linear arithmetic: `textareaRow = contentRelativeY - contentTop`.
* *Reality:* This calculates a **Visual Row Index**. Passing this directly to `setPosition(row, col)` (which expects a Logical Row Index) guarantees corruption whenever wrapping occurs.
* *Consequence:* Clicks drift further from the target with every wrapped line in the document.



### 2. Critical Failure: Viewport Clipping & "Infinite Height"

The architecture attempts to disable the textarea's internal scrolling by forcing it to "Infinite Height" (`setHeight(lineCount + 1)`), delegating control to an outer `inputVp`.

* **The Bug (Clipping):** The JS calculates height using `lineCount()`.
* *Reality:* `lineCount()` returns the number of **newlines** (logical rows). It does not account for visual wrapping.
* *Consequence:* If 10 logical rows wrap to occupy 20 visual lines, the container height is set to 11. The remaining 9 visual lines are rendered by Go but **clipped** by the viewport bounds, making the bottom half of the document invisible and inaccessible.


* **The Bug (Performance):**
* *Reality:* `textarea.View()` generates a string for the *entire* document (with ANSI codes) every time the user types a character.
* *Consequence:* This is an **O(N)** operation on the critical render path. For "Large Documents" (the stated use case), this causes severe input lag and effectively freezes the UI.


* **The Bug (Desynchronization):**
* *Reality:* The outer viewport and the inner cursor logic maintain separate scroll states. The JS attempts to sync them manually but fails due to the unit mismatch (Logical vs Visual).
* *Consequence:* The "Double Scroll" effect—clicks position the cursor, but the viewport fails to scroll to the new location, leaving the cursor hidden off-screen.



### 3. Structural Fragility: Unsafe Mirroring

The code employs `unsafe.Pointer` to cast `*textarea.Model` to a local `textareaModelMirror` struct to access private fields (`viewport`, `cache`).

* **The Risk:** This relies on the **exact** memory layout of the upstream library.
* **Violation:** If `bubbles/textarea` changes its struct layout (e.g., adds a private boolean, reorders fields, or changes the `rsan` interface implementation), this code will silently read garbage data or corrupt memory. This violates the "production-grade" requirement.

### 4. Coordinate Precision Failures (X-Axis)

The horizontal mapping is naive and incorrect for production text.

* **Multi-Width Runes:** The code maps `clickX` directly to column index. CJK characters and emojis occupy 2 visual cells but 1 rune index. Clicking the right half of an emoji will map to the *next* character, splitting the byte sequence or selecting the wrong rune.
* **Tabs:** The logic ignores tab expansion.
* **Wrapped Indentation:** If a line wraps, the second visual line visually starts at X=0. Logically, this is (e.g.) Column 50. The current logic sets `col=0`, moving the cursor to the start of the logical line (jumping back up to the previous visual line).

---

## Root Causes

1. **Coordinate Space Mismatch:** The system treats the text area as a static grid of fixed-height lines (Visual) rather than a dynamic flow of wrapped text (Logical). The translation layer between these two spaces is missing.
2. **Architectural Conflict:** Both the internal `textarea.Model` and the external `inputVp` attempt to manage the "View". Forcing the internal model to abdicate view management (Infinite Height) fights the framework optimizations.
3. **Incomplete Implementation:** The Go helper `handleClick` admits to being incomplete (`// For simplicity...`), yet the feature relies entirely on it working correctly.

---

## Guaranteed Implementation Plan

To guarantee correctness, the following changes are mandatory.

### 1. Go-Side: Implement Soft-Wrap Aware Hit Testing

We must implement a `PerformHitTest` method in Go that uses the internal wrapping logic to map Visual Coordinates to Logical Coordinates.

**Required Logic:**

1. Iterate through logical rows (`m.value`).
2. For each row, calculate its **visual height** using the wrapping logic (`m.memoizedWrap` or a local simulation).
3. Accumulate visual height. When `currentVisualY + rowHeight > clickY`, the target logical row is found.
4. Within that row, determine which **wrapped segment** was clicked.
5. Calculate the column offset by summing the lengths of previous wrapped segments + the visual width of runes in the clicked segment (respecting `runewidth`).

**Reference Implementation (Snippet):**

```go
// VisualLineCount fixes the clipping bug by reporting true rendered height
func (m *Model) VisualLineCount() int {
    count := 0
    for _, runes := range m.value {
        wrapped := m.memoizedWrap(runes, m.width)
        count += len(wrapped)
    }
    return count
}

// PerformHitTest fixes the cursor jump bug
func (m *Model) PerformHitTest(visualX, visualY int) (row, col int) {
    currentVisualY := 0
    // Iterate logical rows to find the target visual area
    for r, runes := range m.value {
        wrapped := m.memoizedWrap(runes, m.width)
        height := len(wrapped)
        if visualY >= currentVisualY && visualY < currentVisualY+height {
            // Found the logical row. Now calculate column...
            // [Implementation details: iterate wrapped segments, sum runewidths]
            return r, calculatedCol
        }
        currentVisualY += height
    }
    return len(m.value) - 1, len(m.value[last]) // Clamp to end
}

```

### 2. JS-Side: Fix Height & Event Handling

1. **Abandon Manual Math:** Stop calculating rows in JS. Pass the relative visual coordinates (`clickX`, `clickY`) directly to the Go `handleClick` method.
2. **Fix Truncation:** If the "Infinite Height" architecture is retained, use the new `VisualLineCount()` (from Go) instead of `lineCount()` to set the container height.
```javascript
// Fixes the "bottom of document cut off" bug
const visualLines = Math.max(1, s.contentTextarea.visualLineCount());
s.contentTextarea.setHeight(visualLines + 1);

```



### 3. Architecture: Recommended Simplification

Ideally, **abandon the external viewport (`inputVp`) entirely**.

* Give the `textarea` the full available dimensions (`termHeight - header`).
* Let `bubbles/textarea` handle scrolling, wrapping, and viewport optimizations natively.
* This solves the **O(N)** performance issue and the **Double Scroll** desynchronization immediately without complex patching.
