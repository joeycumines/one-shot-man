# 01 — Core Anatomy: The Key Input Pipeline

## The Pipeline (End to End)

```
Terminal Key Press
    │
    ▼
Bubble Tea v2 (tea.KeyPressMsg)
    │  msg.String() → key name string
    │  msg.Key().Text → printable text
    │  msg.Key().Mod → modifier flags
    │
    ▼
jsModel.msgToJS() — internal/builtin/bubbletea/bubbletea.go:994-1013
    │  Converts tea.KeyPressMsg → JS map[string]any
    │  Produces: { type: "Key", key: "space", text: " ", mod: [] }
    │  Produces: { type: "Key", key: "ctrl+s", text: "", mod: ["ctrl"] }
    │  Produces: { type: "Key", key: "p", text: "p", mod: [] }
    │
    ▼
JS update() function — bouncingUpdate() in example-15-bouncing-logo.js:511-524
    │  Receives msg object
    │  Calls handleControlKey(m, msg.key)
    │  Falls through to termmux.keyToTermBytes(msg.key)
    │  Falls back to msg.text if keyToTermBytes returns null
    │
    ▼
handleControlKey() — example-15-bouncing-logo.js:539-569
    │  Checks: key === 'p', key === 'b', key === 's', key === 'q', key === 'ctrl+c'
    │  Returns null if no match → key gets forwarded to PTY
    │
    ▼
KeyToTermBytes() — internal/termmux/input.go:12-110
    │  Converts key name string → terminal byte sequence
    │  Handles: named keys, ctrl+letter, modifier+nav, alt+key, paste, single char, unicode
    │  Returns (string, bool) — bytes and whether key was recognized
    │
    ▼
mgr.input(termBytes) — forwarded to PTY
```

## The Three Key Representations

### Representation 1: Bubble Tea v2 KeyMsg.String()

Bubble Tea v2 produces key names via `tea.KeyPressMsg.String()`. Key examples:

| Physical Key | msg.key | msg.text | msg.mod |
|---|---|---|---|
| Space | `"space"` | `" "` | `[]` |
| 'p' | `"p"` | `"p"` | `[]` |
| Ctrl+s | `"ctrl+s"` | `""` | `["ctrl"]` |
| Ctrl+b | `"ctrl+b"` | `""` | `["ctrl"]` |
| Ctrl+c | `"ctrl+c"` | `""` | `["ctrl"]` |
| 's' (no modifier) | `"s"` | `"s"` | `[]` |
| Up arrow | `"up"` | `""` | `[]` |
| Shift+tab | `"shift+tab"` | `""` | `["shift"]` |
| Enter | `"enter"` | `""` | `[]` |

### Representation 2: handleControlKey() expectations

The JS control handler (`example-15-bouncing-logo.js:539-569`) checks:

```javascript
if (key === 'p') { ... }      // matches 'p' ✓
if (key === 'b') { ... }      // matches 'b' ✓
if (key === 's') { ... }      // matches 's' ✓
if (key === 'q') { ... }      // matches 'q' ✓
if (key === 'ctrl+c') { ... } // matches 'ctrl+c' ✓
```

**Critical gap**: When the user presses Ctrl+S, Bubble Tea sends `key = "ctrl+s"`, NOT `key = "s"`. The handler does NOT match "ctrl+s" — it only matches the bare letter "s".

### Representation 3: KeyToTermBytes() expectations

The Go function (`internal/termmux/input.go:12-110`) handles:

1. Named special keys: "enter", "tab", "backspace", "esc", "delete", arrow keys, function keys, etc.
2. **NO case for "space"** — this is a named key in Bubble Tea but not handled here
3. Ctrl+letter: "ctrl+a" through "ctrl+z" → control characters
4. Modifier+navigation: "shift+up", "ctrl+left", etc.
5. Alt+key: "alt+a", "alt+up", etc.
6. Paste: "[content]" → content
7. Single printable character (len == 1): send as-is
8. Multi-character without "+": send as-is (Unicode fallback)

**The "space" bug path**: `KeyToTermBytes("space")`:
- Not in the named keys switch (line 14-69)
- Not a ctrl+letter prefix (line 72)
- Not a modifier+nav (line 83)
- Not alt+key (line 88)
- Not paste (line 95)
- Not a single character (len("space") == 5, not 1) (line 100)
- Falls to the Unicode fallback (line 105): `"space"` has len > 1 and contains no "+", so it returns `("space", true)`
- **The literal string "space" is forwarded to the PTY**

**The correct path for space**: `KeyToTermBytes(" ")` (a single space character):
- Not in the named keys switch
- Not a ctrl+letter
- Not modifier+nav
- Not alt+key
- Not paste
- IS a single character (len == 1) → returns `(" ", true)`
- A space character is correctly forwarded to the PTY

## The msg.text Fallback is Unreachable for "space"

The JS code at line 517-521:
```javascript
var termBytes = termmux.keyToTermBytes(msg.key);
if (termBytes !== null) {
    try { m.mgr.input(termBytes); } catch (e) { logCatch('key-forward', e); }
} else if (msg.text) {
    try { m.mgr.input(msg.text); } catch (e) { logCatch('key-text', e); }
}
```

`keyToTermBytes("space")` returns the Go string `"space"` with `ok = true`, which Goja converts to a JS string (not null). So `termBytes !== null` is TRUE, and the `msg.text` fallback (which would correctly be `" "`) is NEVER reached.

## Source Code References

### msgToJS — bubbletea.go:994-1013
```go
case tea.KeyPressMsg:
    key := msg.Key()
    keyStr := msg.String()    // "space", "ctrl+s", "p", etc.
    text := key.Text          // " " for space, "" for ctrl+s, "p" for p
    mod := key.Mod
    return map[string]any{
        "type":        "Key",
        "key":         keyStr,
        "text":        text,
        "mod":         modToStrings(mod),
        // ...
    }
```

### KeyToTermBytes — input.go:12-110
The function returns `(string, bool)`. When `bool` is true, the JS binding returns a string (never null). The `bool = true` path for "space" comes from the Unicode fallback at line 105-107.

### JS keyToTermBytes binding — module.go:185-190
```go
_ = exports.Set("keyToTermBytes", func(key string) goja.Value {
    if s, ok := parent.KeyToTermBytes(key); ok {
        return runtime.ToValue(s)   // Returns the string, never null
    }
    return goja.Null()             // Only reached when ok = false
})
```
