# HUD/Rendering Review - `scripts/example-05-pick-and-place.js`

**Reviewer:** Takumi (code review)  
**Date:** 2025-01-25  
**Task:** RG-1-hud_rendering-review (from blueprint.json)  
**Status:** CRITICAL ISSUES FOUND

---

## Summary

The HUD positioning calculation is fundamentally broken. `hudX` ignores the `spaceX` centering offset, causing:
1. **Overlap on narrow terminals** (80x24): HUD overwrites play area columns 55-67
2. **Misplacement on wide terminals** (200+): HUD appears in left margin, not right of play area
3. **spaceWidth=55 is incompatible with 80-column terminals** even if fixed (needs 82+ columns)

The `getRenderBuffer()`, `clearBuffer()`, `getAllSprites()`, and sprite rendering logic are correct.

---

## Detailed Analysis

### 1. `renderPlayArea()` - HUD Positioning (CRITICAL BUG)

**Location:** Lines 1148-1205

**Bug:** HUD X-coordinate calculation ignores `spaceX`:

```javascript
const spaceX = Math.floor((width - state.spaceWidth) / 2);  // Line 1151
// ... sprites rendered at: sx = Math.floor(s.x) + spaceX + 1
const hudX = Math.min(state.spaceWidth + 2, width - 25);    // Line 1179 - IGNORES spaceX!
```

**Calculation trace for 80x24 terminal:**
| Variable | Calculation | Value |
|----------|-------------|-------|
| `width` | terminal width | 80 |
| `spaceWidth` | hardcoded | 55 |
| `spaceX` | `floor((80-55)/2)` | 12 |
| Play area left border | column at `spaceX` | 12 |
| Sprite max column | `54 + 12 + 1` | 67 |
| `hudX` | `min(57, 55)` | **55** |
| HUD range | `55` to `55+25` | **55-79** |
| **OVERLAP** | columns 55-67 | **12 columns** |

**Calculation trace for 200x24 terminal (test harness):**
| Variable | Calculation | Value |
|----------|-------------|-------|
| `width` | test harness width | 200 |
| `spaceX` | `floor((200-55)/2)` | 72 |
| Play area columns | 73 to 127 | 55 cols |
| `hudX` | `min(57, 175)` | **57** |
| HUD location | column 57 | **LEFT of play area** |

**Root Cause:** The formula `state.spaceWidth + 2` assumes play area starts at column 0. It does not.

**Correct formula:**
```javascript
const hudX = spaceX + state.spaceWidth + 2;  // Right of play area
```

But this creates a NEW problem: on 80-column terminals:
- Correct hudX = 12 + 55 + 2 = **69**
- HUD needs 25 columns → ends at 94
- **Overflows screen by 14 columns**

**DESIGN CONFLICT:** `spaceWidth=55` is fundamentally incompatible with 80x24 terminals when HUD requires 25 columns. Mathematical minimum: `55 + 2 + 25 = 82` columns.

---

### 2. `renderPlayArea()` - Sprite Rendering (CORRECT)

**Location:** Lines 1167-1175

```javascript
const sx = Math.floor(s.x) + spaceX + 1;
const sy = Math.floor(s.y);
if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
    buffer[sy * width + sx] = s.char;
}
```

**Verification:**
- ✅ Applies `spaceX` offset correctly
- ✅ Adds `+1` for border column
- ✅ Bounds-checks before writing
- ✅ Uses 1D array indexing correctly

---

### 3. `renderPlayArea()` - Goal Area Dots (CORRECT)

**Location:** Lines 1156-1166

```javascript
const sx = gx + spaceX + 1;  // Correctly applies offset
if (sx >= 0 && sx < width && gy >= 0 && gy < height) {
    const idx = gy * width + sx;
    if (buffer[idx] === ' ') buffer[idx] = '·';
}
```

**Verification:**
- ✅ Applies `spaceX + 1` offset consistently with sprites
- ✅ Only draws on empty cells (doesn't overwrite sprites)

---

### 4. `getRenderBuffer()` / `clearBuffer()` (CORRECT)

**Location:** Lines 1099-1117

```javascript
function getRenderBuffer(width, height) {
    if (_renderBuffer === null || _renderBufferWidth !== width || _renderBufferHeight !== height) {
        _renderBufferWidth = width;
        _renderBufferHeight = height;
        _renderBuffer = new Array(width * height);
        for (let i = 0; i < _renderBuffer.length; i++) {
            _renderBuffer[i] = ' ';
        }
    }
    return _renderBuffer;
}

function clearBuffer(buffer, width, height) {
    for (let i = 0; i < buffer.length; i++) {
        buffer[i] = ' ';
    }
}
```

**Verification:**
- ✅ Buffer reuse via global `_renderBuffer`
- ✅ Reallocates only when dimensions change
- ✅ Uses 1D array (efficient)
- ✅ `clearBuffer` iterates explicitly (no slice/fill allocation)

---

### 5. `getAllSprites()` (CORRECT with minor observation)

**Location:** Lines 1119-1138

```javascript
function getAllSprites(state) {
    const sprites = [];
    state.actors.forEach(a => {
        sprites.push({ x: a.x, y: a.y, char: '@', width: 1, height: 1 });
        if (a.heldItem) sprites.push({ x: a.x, y: a.y - 0.5, char: '◆', width: 1, height: 1 });
    });
    state.cubes.forEach(c => { ... });
    state.goals.forEach(g => { ... });
    return sprites;
}
```

**Verification:**
- ✅ Collects all entity types
- ✅ Held item rendered at `y - 0.5` (appears above actor after floor)
- ✅ Different chars per cube type
- Caller sorts by Y: `sprites.sort((a, b) => a.y - b.y)` - correct layering

**Observation:** `width` and `height` properties in sprite objects are unused. Minor dead code but harmless.

---

### 6. `view()` (CORRECT)

**Location:** Lines 1206-1230

```javascript
function view(state) {
    let output = renderPlayArea(state);
    if (state.debugMode) {
        // ... builds debug JSON
        output += '\n__place_debug_start__\n' + debugJSON + '\n__place_debug_end__';
    }
    return output;
}
```

**Verification:**
- ✅ Delegates to `renderPlayArea()`
- ✅ Debug overlay appended (not inserted into play area)
- ✅ Debug JSON structure matches `PickAndPlaceDebugJSON` Go struct

---

## Issues Found

| ID | Severity | Description | Impact |
|----|----------|-------------|--------|
| HUD-1 | **CRITICAL** | `hudX` ignores `spaceX` offset | HUD overlaps play area on 80x24 |
| HUD-2 | **MAJOR** | `spaceWidth=55` incompatible with 80x24 + 25-col HUD | Cannot fix HUD-1 without layout redesign |
| HUD-3 | **MINOR** | Test comment claims "42 wide" but spaceWidth=55 | Outdated comment (line 2376 of test) |
| HUD-4 | **MINOR** | Test harness uses 200 columns, masking HUD-1 | Bug not caught in tests |

---

## Recommendations

### Fix HUD-1 (Required)

Apply `spaceX` offset to HUD positioning:

```javascript
// BEFORE:
const hudX = Math.min(state.spaceWidth + 2, width - 25);

// AFTER:
const hudX = spaceX + state.spaceWidth + 2;
```

### Fix HUD-2 (Required - Choose One)

**Option A:** Reduce `spaceWidth` to fit 80 columns:
```javascript
spaceWidth: 40,  // 40 + 12 (spaceX) + 2 + 25 = 79, fits in 80
```

**Option B:** Reduce HUD width to ~12 columns (truncate control hints):
```javascript
const HUD_WIDTH = 12;
const hudX = Math.max(spaceX + state.spaceWidth + 2, width - HUD_WIDTH);
```

**Option C:** Conditionally hide HUD on narrow terminals:
```javascript
const hudSpace = width - (spaceX + state.spaceWidth + 2);
if (hudSpace >= 25) { /* draw HUD */ }
```

### Fix HUD-3 (Minor)

Update test comment:
```go
// BEFORE: Play space is approximately 42 wide
// AFTER:  Play space is spaceWidth=55 columns
```

### Fix HUD-4 (Test Coverage)

Add test with 80-column terminal:
```go
termtest.WithSize(24, 80),  // Standard terminal
```

---

## Verification Criteria Assessment

From `blueprint.json` task `fix_hud_layout`:

| Criterion | Status | Evidence |
|-----------|--------|----------|
| HUD positioned right of play area, accounting for `spaceX` | ❌ FAIL | HUD-1: `hudX` ignores `spaceX` |
| HUD does not occlude any map tiles | ❌ FAIL | HUD-1: overlaps columns 55-67 on 80x24 |
| HUD readable on 80x24 | ❌ FAIL | HUD-2: needs 82+ columns |

---

## Trust Declarations

- **Verified by code trace:** All coordinate calculations, buffer indexing, sprite rendering
- **Verified by test reference:** Debug JSON format matches Go struct
- **NOT independently verified (trusted):** That `termtest.WithSize(24, 200)` accurately emulates 200-column terminal (assumed correct per library documentation)

---

## Appendix: Full Coordinate Trace for 80x24

```
Terminal: 80 columns × 24 rows

spaceX = floor((80 - 55) / 2) = 12

Play Area Layout:
  Col 0-11:  Left margin (empty)
  Col 12:    Left border '│'
  Col 13-67: Play area content (sprites at x=0..54 map to col 13..67)
  Col 68-79: Right margin (should contain HUD)

Actual HUD Position:
  hudX = min(55 + 2, 80 - 25) = min(57, 55) = 55
  HUD draws to columns 55-79

Overlap Zone:
  Columns 55-67 (13 columns) contain BOTH sprites AND HUD text
```
