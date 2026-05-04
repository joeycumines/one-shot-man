# osm:time — Time Utilities

A native module providing time-related utility functions for JavaScript scripts.

```js
const time = require('osm:time');
```

---

## Functions

### `time.sleep(ms)`

Synchronously pauses script execution for the specified duration.

**Parameters:**

| Name | Type   | Required | Description                  |
|------|--------|----------|------------------------------|
| `ms` | number | Yes      | Duration to sleep in milliseconds. Converted to an integer via `ToInteger()`. |

**Returns:** `undefined`

**Behavior:**

- Blocks the current JavaScript execution thread for exactly `ms` milliseconds.
- The sleep is synchronous — no other JavaScript code runs during the pause.
- A value of `0` returns immediately (no-op sleep).
- Negative values are treated as `0` by Go's `time.Duration`.
- Non-numeric values are coerced to `0` via Goja's `ToInteger()`.

**Examples:**

```js
const time = require('osm:time');

// Wait 1 second
time.sleep(1000);

// Brief pause for UI debouncing (100ms)
time.sleep(100);

// Polling loop with delay
let ready = false;
while (!ready) {
  ready = checkCondition();
  if (!ready) {
    time.sleep(500); // poll every 500ms
  }
}
```

**Example — Retry with backoff:**

```js
const time = require('osm:time');

function retryWithBackoff(fn, maxRetries, baseDelayMs) {
  for (var i = 0; i < maxRetries; i++) {
    var result = fn();
    if (result.error === undefined) {
      return result;
    }
    var delay = baseDelayMs * Math.pow(2, i);
    log.info('Attempt ' + (i + 1) + ' failed, retrying in ' + delay + 'ms');
    time.sleep(delay);
  }
  return fn(); // final attempt
}
```

---

## Notes

- This module wraps Go's `time.Sleep()` directly.
- Since the Goja JavaScript runtime is single-threaded, `sleep()` blocks all script execution — there is no concurrent JavaScript running during the sleep.
- For TUI applications using BubbleTea, prefer `tick()` commands over `sleep()` to avoid blocking the UI render loop.
