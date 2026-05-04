# osm:unicodetext — Unicode Text Utilities

A native module providing Unicode-aware text measurement and manipulation functions. Uses the [uniseg](https://github.com/rivo/uniseg) library for proper grapheme cluster handling and monospace width calculation.

```js
const unicodetext = require('osm:unicodetext');
```

---

## Functions

### `unicodetext.width(s)`

Returns the monospace display width of a string, accounting for wide characters (CJK), combining characters, zero-width characters, and emoji.

**Parameters:**

| Name | Type   | Required | Description             |
|------|--------|----------|-------------------------|
| `s`  | string | No       | The string to measure. Defaults to `""` if omitted. |

**Returns:** `number` — The display width in monospace terminal columns.

**Behavior:**

- ASCII characters have width `1`.
- CJK (Chinese, Japanese, Korean) characters have width `2`.
- Combining characters (e.g., accents like `e\u0301`) contribute `0` additional width.
- Zero-width characters (e.g., zero-width space `\u200B`) have width `0`.
- Emoji typically have width `2` (varies by terminal; uses Unicode standard widths).
- Complex emoji sequences (ZWJ, skin tones, flags) are measured as single grapheme clusters.
- An empty string returns `0`.

**Examples:**

```js
const unicodetext = require('osm:unicodetext');

// ASCII
unicodetext.width('hello');       // → 5

// CJK characters (width 2 each)
unicodetext.width('你好');         // → 4

// Combining accent: 'e' + combining acute = 1 grapheme
unicodetext.width('e\u0301');     // → 1

// Zero-width space between characters
unicodetext.width('a\u200Bb');    // → 2

// Emoji
unicodetext.width('😀');          // → 2

// Complex emoji (rainbow flag)
unicodetext.width('🏳️‍🌈');        // → 2

// Empty string
unicodetext.width('');            // → 0
unicodetext.width();              // → 0
```

---

### `unicodetext.truncate(s, maxWidth, tail?)`

Truncates a string so its display width does not exceed `maxWidth`, appending a tail indicator if truncation occurs. Operates on grapheme clusters to avoid splitting multi-byte characters.

**Parameters:**

| Name       | Type   | Required | Description                                        |
|------------|--------|----------|----------------------------------------------------|
| `s`        | string | Yes      | The string to truncate.                            |
| `maxWidth` | number | Yes      | Maximum display width (in monospace columns).      |
| `tail`     | string | No       | Suffix appended when truncation occurs. Default: `"..."`. |

**Returns:** `string` — The (possibly truncated) string with tail appended.

**Throws:** `Error` if fewer than 2 arguments are provided.

**Behavior:**

- If the string's display width is ≤ `maxWidth`, it is returned unchanged (no tail appended).
- If truncation is needed, the function:
  1. Calculates `targetWidth = maxWidth - width(tail)`.
  2. Iterates grapheme clusters, accumulating width until `targetWidth` would be exceeded.
  3. Appends `tail` to the accumulated content.
- **Edge case — tail wider than maxWidth:** If `width(tail) > maxWidth`, the tail itself is returned (best-effort behavior).
- Grapheme clusters are never split — a wide character that would exceed `targetWidth` is excluded entirely.

**Examples:**

```js
const unicodetext = require('osm:unicodetext');

// Basic truncation with default tail "..."
unicodetext.truncate('hello world', 5);
// → "he..."  (width: "he" = 2, "..." = 3, total = 5)

// No truncation needed
unicodetext.truncate('hello', 10);
// → "hello"  (fits within maxWidth, returned as-is)

// Exact fit — no truncation
unicodetext.truncate('hello', 5);
// → "hello"  (width 5 = maxWidth, no truncation)

// Custom tail
unicodetext.truncate('hello world', 4, '.');
// → "hel."  (width: "hel" = 3, "." = 1, total = 4)

// Empty tail — hard truncation
unicodetext.truncate('hello', 4, '');
// → "hell"  (no tail appended)

// Tail wider than maxWidth
unicodetext.truncate('abc', 2, '...');
// → "..."  (tail width 3 > maxWidth 2, returns tail as best-effort)
```

**Example — CJK truncation:**

```js
const unicodetext = require('osm:unicodetext');

// CJK: each char is width 2
// "你好世界" total width = 8
unicodetext.truncate('你好世界', 5, '.');
// → "你好."  ("你好" = 4, "." = 1, total = 5)

// Target width can't fit another CJK char
unicodetext.truncate('你好世界', 3, '.');
// → "你."  ("你" = 2, "." = 1, total = 3)
```

**Example — Emoji and combining characters:**

```js
const unicodetext = require('osm:unicodetext');

// Combining accent preserved as single grapheme
// 'é' (e + combining acute) has width 1
unicodetext.truncate('e\u0301abcd', 2, '.');
// → "e\u0301."  ("é" = 1, "." = 1, total = 2)

// Emoji (width 2) exceeds remaining target
unicodetext.truncate('😀bc', 2, '.');
// → "."  (😀 width 2 > target 1, skipped; "." = 1)

// Emoji at boundary
unicodetext.truncate('abc😀', 4, '.');
// → "abc."  ("abc" = 3, "." = 1, total = 4; 😀 would exceed)
```

**Example — Column-aligned table rendering:**

```js
const unicodetext = require('osm:unicodetext');
const lipgloss = require('osm:lipgloss');

function renderColumn(text, colWidth) {
  var truncated = unicodetext.truncate(text, colWidth, '…');
  var padding = colWidth - unicodetext.width(truncated);
  var pad = '';
  for (var i = 0; i < padding; i++) { pad += ' '; }
  return truncated + pad;
}

// Render aligned columns
var rows = [
  ['Name', 'Status'],
  ['very-long-service-name', 'running'],
  ['短い', 'stopped'],
];
rows.forEach(function(row) {
  log.printf('%s | %s', renderColumn(row[0], 15), renderColumn(row[1], 10));
});
```

---

## Notes

- This module uses [github.com/rivo/uniseg](https://github.com/rivo/uniseg) for Unicode grapheme cluster segmentation and width calculation.
- Width calculations follow the Unicode Standard Annex #29 (grapheme cluster boundaries) and East Asian Width properties.
- Terminal rendering may vary — some terminals render certain emoji differently than the Unicode standard width. The values returned by `width()` match the `uniseg` library's interpretation.
- For TUI applications, use `unicodetext.width()` instead of `string.length` to correctly measure display width for layout calculations.
