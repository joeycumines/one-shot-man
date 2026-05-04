# osm:nextIntegerID — Sequential ID Generator

A native module that generates the next sequential integer ID from an array of objects. Useful for maintaining ordered, non-colliding IDs in state-managed lists.

```js
const nextId = require('osm:nextIntegerID');
```

> **Deprecated alias:** `require('osm:nextIntegerId')` still works but is deprecated. Use `osm:nextIntegerID` (capital `ID`) per Go naming conventions.

---

## Default Export

The module's default export is a single callable function (not an object with methods).

### `nextId(list)`

Scans an array of objects for the highest `.id` property and returns `max + 1`.

**Parameters:**

| Name   | Type                    | Required | Description                                      |
|--------|-------------------------|----------|--------------------------------------------------|
| `list` | `Array<{id?: number}>` | No       | Array of objects, each optionally having an `id` property. |

**Returns:** `number` — The next available integer ID.

**Behavior:**

- If called with **no arguments**, returns `1`.
- If `list` is `null` or `undefined`, returns `1`.
- If `list` is an **empty array** (`[]`), returns `1`.
- Iterates through the array, reading each element's `.id` property.
- Elements that are `null`, `undefined`, or lack an `.id` property are skipped.
- String `.id` values are coerced to integers via Goja's `ToInteger()` (non-numeric strings become `0`).
- Returns `max(all .id values) + 1`.
- If all elements are skipped (no valid `.id` found), returns `1` (since `max` starts at `0`).

**Examples:**

```js
const nextId = require('osm:nextIntegerID');

// No arguments — starts at 1
nextId();          // → 1

// Empty list — starts at 1
nextId([]);        // → 1

// Basic usage — finds max and adds 1
nextId([
  { id: 1, name: 'Alice' },
  { id: 3, name: 'Bob' },
  { id: 2, name: 'Charlie' }
]);                // → 4

// Gaps are preserved (does not fill gaps)
nextId([
  { id: 1 },
  { id: 100 }
]);                // → 101

// Items without id are skipped
nextId([
  { id: 5 },
  { name: 'no-id' },
  { id: 2 }
]);                // → 6

// Null/undefined items are skipped
nextId([null, undefined, { id: 7 }]);  // → 8
```

**Example — Managing a todo list:**

```js
const nextId = require('osm:nextIntegerID');

var todos = [
  { id: 1, text: 'Write docs', done: false },
  { id: 2, text: 'Run tests', done: true }
];

function addTodo(text) {
  todos.push({
    id: nextId(todos),
    text: text,
    done: false
  });
}

addTodo('Deploy');
// todos now includes { id: 3, text: 'Deploy', done: false }
```

**Example — State variable integration:**

```js
const nextId = require('osm:nextIntegerID');

// Works with tui.createState() arrays
var state = tui.createState('myState', { items: [] });

function addItem(label) {
  var items = state.get('items');
  items.push({ id: nextId(items), label: label });
  state.set('items', items);
}
```

---

## Notes

- The function operates on the `.id` property specifically — it does not support custom key names.
- IDs are always positive integers starting from `1`.
- The function does **not** fill gaps in the sequence. If IDs are `[1, 5, 10]`, the next ID is `11`, not `2`.
- The function is **stateless** — it computes the next ID from the array each time. It does not maintain an internal counter.
- Thread-safety: Each call to `require()` creates a fresh function binding. The function itself has no shared mutable state.
