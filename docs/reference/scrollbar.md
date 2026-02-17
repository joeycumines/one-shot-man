# osm:termui/scrollbar — Vertical Scrollbar Widget

A native module providing a thin vertical scrollbar component for TUI applications. Designed for use alongside `osm:bubbles/viewport` to indicate scroll position in content-heavy views.

```js
const scrollbar = require('osm:termui/scrollbar');
```

---

## Constructor

### `scrollbar.new(viewportHeight?)`

Creates a new scrollbar instance.

**Parameters:**

| Name             | Type   | Required | Description                                      |
|------------------|--------|----------|--------------------------------------------------|
| `viewportHeight` | number | No       | Initial height of the visible area in rows. If omitted, uses the internal default. |

**Returns:** `ScrollbarObject` — a stateful scrollbar model.

**Example:**

```js
var sb = require('osm:termui/scrollbar').new(10);
```

---

## ScrollbarObject Methods

All setter methods return the scrollbar object itself for **method chaining**, except when called with missing required arguments (returns `undefined`).

### `sb.setViewportHeight(h)`

Sets the height of the visible viewport area.

**Parameters:**

| Name | Type   | Required | Description                              |
|------|--------|----------|------------------------------------------|
| `h`  | number | Yes      | Viewport height in rows. Negative values are clamped to `0`. |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument provided.

---

### `sb.setContentHeight(h)`

Sets the total height of the content being scrolled.

**Parameters:**

| Name | Type   | Required | Description                              |
|------|--------|----------|------------------------------------------|
| `h`  | number | Yes      | Total content height in rows. Negative values are clamped to `0`. |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument provided.

---

### `sb.setYOffset(y)`

Sets the current vertical scroll offset.

**Parameters:**

| Name | Type   | Required | Description                              |
|------|--------|----------|------------------------------------------|
| `y`  | number | Yes      | Scroll offset in rows. Negative values are **not** clamped. |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument provided.

---

### `sb.viewportHeight()`

Returns the current viewport height.

**Returns:** `number`

---

### `sb.contentHeight()`

Returns the current content height.

**Returns:** `number`

---

### `sb.yOffset()`

Returns the current Y offset.

**Returns:** `number`

---

### `sb.setChars(thumb, track)`

Sets the characters used to render the thumb (scroll position indicator) and track (background).

**Parameters:**

| Name    | Type   | Required | Description                    |
|---------|--------|----------|--------------------------------|
| `thumb` | string | Yes      | Character for the thumb.       |
| `track` | string | Yes      | Character for the track.       |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if fewer than 2 arguments provided.

---

### `sb.setThumbForeground(color)`

Sets the foreground color of the thumb character.

**Parameters:**

| Name    | Type   | Required | Description                                          |
|---------|--------|----------|------------------------------------------------------|
| `color` | string | Yes      | Color string (e.g., `"#FF0000"`, `"9"` for ANSI).   |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument.

---

### `sb.setThumbBackground(color)`

Sets the background color of the thumb character.

**Parameters:**

| Name    | Type   | Required | Description                   |
|---------|--------|----------|-------------------------------|
| `color` | string | Yes      | Color string.                 |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument.

---

### `sb.setTrackForeground(color)`

Sets the foreground color of the track character.

**Parameters:**

| Name    | Type   | Required | Description                   |
|---------|--------|----------|-------------------------------|
| `color` | string | Yes      | Color string.                 |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument.

---

### `sb.setTrackBackground(color)`

Sets the background color of the track character.

**Parameters:**

| Name    | Type   | Required | Description                   |
|---------|--------|----------|-------------------------------|
| `color` | string | Yes      | Color string.                 |

**Returns:** `ScrollbarObject` (for chaining), or `undefined` if no argument.

---

### `sb.view()`

Renders the scrollbar as a multi-line string with ANSI escape codes for terminal display. The output has one line per viewport row, with thumb and track characters styled according to the configured colors.

**Returns:** `string` — rendered scrollbar with ANSI styling.

---

## Properties

### `sb._type`

Type tag string, always `"termui/scrollbar"`.

---

## Examples

**Basic usage:**

```js
var sb = require('osm:termui/scrollbar').new(10);
sb.setContentHeight(50);
sb.setYOffset(0);
var rendered = sb.view();
// rendered is a 10-line string with thumb and track characters
```

**Method chaining with custom styling:**

```js
var sb = require('osm:termui/scrollbar').new(8);
sb.setContentHeight(40)
  .setYOffset(5)
  .setChars('█', '░')
  .setThumbForeground('#00FF00')
  .setTrackForeground('#333333');

var view = sb.view();
// 8-line scrollbar with green thumb and grey track
```

**Integration with viewport:**

```js
var viewport = require('osm:bubbles/viewport').new(80, 20);
var sb = require('osm:termui/scrollbar').new(20);

viewport.setContent(longText);
sb.setContentHeight(viewport.totalLineCount())
  .setViewportHeight(20)
  .setYOffset(viewport.yOffset());

// Render side by side
var lines = viewport.view().split('\n');
var sbLines = sb.view().split('\n');
var combined = '';
for (var i = 0; i < lines.length; i++) {
  combined += lines[i] + (sbLines[i] || '') + '\n';
}
```

---

## Notes

- The scrollbar is a **stateful** model — setters mutate the internal state.
- The thumb position and size are calculated automatically based on `viewportHeight`, `contentHeight`, and `yOffset`.
- Color strings accept Lipgloss color formats: hex (`"#RRGGBB"`), ANSI 256 (`"9"`), or named ANSI colors.
- When `contentHeight ≤ viewportHeight`, the scrollbar renders as all-thumb (full-height thumb), a standard convention indicating "all content is visible; no scrolling needed."
