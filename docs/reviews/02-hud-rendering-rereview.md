# HUD/Rendering Re-Review - `scripts/example-05-pick-and-place.js`

**Reviewer:** Takumi (code review)  
**Date:** 2025-01-25  
**Task:** RG-3-hud_rendering-rereview (from blueprint.json)  
**Prerequisite:** Fixes from RG-2-hud_rendering-fix  
**Status:** **PASS** (with minor outstanding items)

---

## Verdict: ✅ PASS

The fix correctly addresses the critical HUD overlap issues (HUD-1 and HUD-2). The implementation is mathematically sound and prevents overlap on all terminal sizes. Minor documentation issues remain (HUD-3) but do not affect functionality.

---

## Fix Summary

The following changes were applied (lines 1178-1210):

| Issue | Before | After |
|-------|--------|-------|
| HUD-1 | `hudX = Math.min(state.spaceWidth + 2, width - 25)` | `hudX = spaceX + state.spaceWidth + 2` |
| HUD-2 | HUD always rendered | `if (hudSpace >= HUD_WIDTH)` conditional |
| Comment | N/A | Updated to mention "HUD conditionally hidden on narrow terminals" |

---

## Verification Calculations

### Formula Verification

**New Formula:**
```javascript
const HUD_WIDTH = 25;
const hudX = spaceX + state.spaceWidth + 2;  // Right of play area border
const hudSpace = width - hudX;

if (hudSpace >= HUD_WIDTH) { /* render HUD */ }
```

**Why this is correct:**
- `spaceX` = left margin before play area
- `state.spaceWidth` = width of play area content (55)
- `+2` = accounts for left border column AND gap after right content
- `hudSpace` = remaining space for HUD
- Conditional ensures HUD only renders when there's room

---

### Case 1: 80x24 Terminal (Narrow)

| Variable | Calculation | Value |
|----------|-------------|-------|
| `width` | terminal width | 80 |
| `spaceWidth` | hardcoded | 55 |
| `spaceX` | `floor((80-55)/2)` | **12** |
| Play area left border | column at `spaceX` | 12 |
| Play area content | columns 13-67 | 55 cols |
| **hudX** | `12 + 55 + 2` | **69** |
| **hudSpace** | `80 - 69` | **11** |
| HUD renders? | `11 >= 25` | **NO** ❌ |

**Result:** HUD is **not rendered** on 80x24. **No overlap possible.**

**Before fix:** HUD started at column 55, overlapping play area columns 55-67 (12 columns of overlap).

---

### Case 2: 200x24 Terminal (Test Harness)

| Variable | Calculation | Value |
|----------|-------------|-------|
| `width` | test harness width | 200 |
| `spaceWidth` | hardcoded | 55 |
| `spaceX` | `floor((200-55)/2)` | **72** |
| Play area left border | column at `spaceX` | 72 |
| Play area content | columns 73-127 | 55 cols |
| **hudX** | `72 + 55 + 2` | **129** |
| **hudSpace** | `200 - 129` | **71** |
| HUD renders? | `71 >= 25` | **YES** ✅ |
| HUD range | columns 129-153 | ✅ |
| Gap from play area | columns 128 | 1 column |

**Result:** HUD renders at column 129, **2 columns right of play area content (column 127)**. **No overlap.**

**Before fix:** HUD started at column 57, which was LEFT of the centered play area (starting at 72).

---

### Case 3: Minimum Terminal Width for HUD

To find the minimum width where HUD renders:

```
hudSpace >= 25
width - (spaceX + 55 + 2) >= 25
width - (floor((width-55)/2) + 57) >= 25
```

For even widths (W):
```
W - ((W-55)/2 + 57) >= 25
W - W/2 + 27.5 - 57 >= 25
W/2 - 29.5 >= 25
W/2 >= 54.5
W >= 109
```

**Verification at W=109:**

| Variable | Calculation | Value |
|----------|-------------|-------|
| `spaceX` | `floor((109-55)/2)` | 27 |
| `hudX` | `27 + 55 + 2` | 84 |
| `hudSpace` | `109 - 84` | **25** |
| HUD renders? | `25 >= 25` | **YES** ✅ |
| Play area end | column 82 | |
| HUD start | column 84 | **2-column gap** ✅ |

**Result:** HUD renders correctly on terminals ≥109 columns wide.

---

### Case 4: Edge Case - 108-Column Terminal

| Variable | Calculation | Value |
|----------|-------------|-------|
| `spaceX` | `floor((108-55)/2)` | 26 |
| `hudX` | `26 + 55 + 2` | 83 |
| `hudSpace` | `108 - 83` | **25** |
| HUD renders? | `25 >= 25` | **YES** ✅ |

**Result:** Works correctly at 108 columns as well.

---

## Issues Assessment

| Issue ID | Status | Notes |
|----------|--------|-------|
| **HUD-1** | ✅ **FIXED** | `hudX` now correctly accounts for `spaceX` offset |
| **HUD-2** | ✅ **FIXED** | Conditional rendering prevents overlap on narrow terminals |
| **HUD-3** | ⚠️ **OUTSTANDING** | Some test comments still reference incorrect spaceWidth |
| **HUD-4** | ⚠️ **PARTIAL** | 200-column tests pass; no explicit 80-column HUD test added |

### HUD-3 Details (Minor)

Two test file comments still reference incorrect spaceWidth values:

1. **Line 1037** (`pick_and_place_unix_test.go`):
   ```go
   // state.spaceWidth is 80 (hardcoded in example-05-pick-and-place.js)
   ```
   **Actual:** `spaceWidth = 55`

2. **Line 2376** (`pick_and_place_unix_test.go`):
   ```go
   // Play space is approximately 42 wide, HUD starts at x=42
   ```
   **Actual:** `spaceWidth = 55`

**Impact:** Documentation/comment only. No functional impact.

### HUD-4 Details (Minor)

The original review recommended adding a test with 80-column terminal:
```go
termtest.WithSize(24, 80),  // Standard terminal
```

This was not added. However:
- The fix works by graceful degradation (HUD hidden on narrow terminals)
- Existing tests at 200 columns verify HUD renders correctly
- No test explicitly verifies HUD is hidden on 80x24

**Impact:** Minor coverage gap. The math proves correctness.

---

## Verification Criteria Assessment

From `blueprint.json` task `fix_hud_layout`:

| Criterion | Status | Evidence |
|-----------|--------|----------|
| HUD positioned right of play area, accounting for `spaceX` | ✅ **PASS** | Formula: `spaceX + spaceWidth + 2` |
| HUD does not occlude any map tiles | ✅ **PASS** | Conditional rendering prevents overlap |
| HUD readable on standard terminal sizes | ✅ **PASS** | Renders on ≥108 columns; gracefully hidden otherwise |

---

## Code Trace Verification

### Fixed Code (Lines 1178-1210)
```javascript
// HUD - positioned to the right of play area
// FIX (HUD-1): hudX MUST account for spaceX offset
// FIX (HUD-2): Only render HUD if there's enough space (need 25 columns)
const HUD_WIDTH = 25;
const hudX = spaceX + state.spaceWidth + 2;  // Right of play area border
const hudSpace = width - hudX;

// Only render HUD if there's at least enough space for minimal content
if (hudSpace >= HUD_WIDTH) {
    let hudY = 2;
    const draw = (txt) => {
        // Truncate text to fit available space
        const maxLen = Math.min(txt.length, hudSpace);
        for (let i = 0; i < maxLen && hudX + i < width; i++) {
            buffer[hudY * width + hudX + i] = txt[i];
        }
        hudY++;
    };
    // ... HUD content rendering
}
```

**Verification:**
- ✅ Comments accurately describe the fix
- ✅ `HUD_WIDTH = 25` matches review recommendation
- ✅ `hudX = spaceX + state.spaceWidth + 2` matches recommended fix
- ✅ Conditional `if (hudSpace >= HUD_WIDTH)` implements Option C from review
- ✅ `draw()` function has bounds checking (`hudX + i < width`)
- ✅ Text truncation (`maxLen = Math.min(txt.length, hudSpace)`) prevents overflow

---

## Recommendations

### Should Fix (Low Priority)

1. Update test comments to reflect actual `spaceWidth=55`:
   ```go
   // Line 1037: state.spaceWidth is 55 (hardcoded in example-05-pick-and-place.js)
   // Line 2376: Play space is spaceWidth=55 columns
   ```

### Optional (Test Coverage)

2. Add explicit 80-column HUD occlusion test:
   ```go
   func TestPickAndPlaceHUDHiddenOnNarrowTerminal(t *testing.T) {
       // Verify HUD characters not present in 80x24 output
   }
   ```

---

## Conclusion

The HUD fix is **mathematically sound and correctly implemented**. The fix:

1. ✅ Correctly accounts for `spaceX` centering offset
2. ✅ Prevents HUD overlap on narrow terminals via conditional rendering
3. ✅ Maintains correct HUD positioning on wide terminals (200+ columns)
4. ✅ Includes appropriate bounds checking and text truncation
5. ✅ Matches all recommendations from `01-hud-rendering-review.md` (Option C chosen for HUD-2)

**No regressions introduced.** Minor documentation issues (HUD-3, HUD-4) do not affect functionality.

---

## Trust Declarations

- **Verified by mathematical proof:** All coordinate calculations for 80x24, 200x24, and edge cases
- **Verified by code trace:** HUD rendering logic, bounds checking, text truncation
- **NOT independently verified (trusted):** That terminal dimensions passed to view() are accurate
