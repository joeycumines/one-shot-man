# 04 — Honest Conclusions

## TRUE — Verified by Code

1. **`KeyToTermBytes("space")` forwards the literal string "space" to the PTY** — traced through input.go:14-110, the function reaches the Unicode fallback at line 105-107 which returns `("space", true)`. The test at input_test.go:137-141 tests `KeyToTermBytes(" ")` (a literal space), NOT `KeyToTermBytes("space")` (the Bubble Tea key name), which is a different input entirely.

2. **The `msg.text` fallback in the JS script is unreachable for the spacebar** — `keyToTermBytes("space")` returns a non-null string, so the `else if (msg.text)` branch at line 520-521 is never executed. The text `" "` that would correctly represent a space is discarded.

3. **Ctrl+key combos bypass the handleControlKey function** — `handleControlKey("ctrl+s")` does not match `key === 's'` (strict equality). The function returns null, causing ctrl+s to be forwarded to the PTY as control character `\x13` instead of resizing the pane.

4. **The Unicode fallback in KeyToTermBytes is overly broad** — It matches any multi-character string without a "+" character, including key names like "space", "capslock", "num0". It should only match actual Unicode text input.

5. **The handleControlKey function only handles 5 specific key values** — "p", "b", "s", "q", "ctrl+c". All other keys (including ctrl+key variants of p/b/s/q) fall through to PTY forwarding.

6. **Bubble Tea v2 correctly produces "space" for spacebar** — The msgToJS function at bubbletea.go:998-1001 uses `msg.String()` which is the canonical Bubble Tea key name. The comment at line 999 explicitly notes: "Space bar produces 'space', not ' '. The text field provides the actual character."

## UNCERTAIN

1. **Whether other scripts have the same spacebar bug** — Other scripts using `termmux.keyToTermBytes(msg.key)` for key forwarding would have the same bug. The `KeyToTermBytes` function is shared infrastructure. Any script that forwards key events to a PTY would be affected. This was not investigated but is likely.

2. **Whether the user wants bare letter keys OR ctrl+letter keys OR a chord system for controls** — The script's UI shows `[B] Bigger`, `[S] Smaller`, `[P] Pause` implying bare keys. The user's request explicitly asks for "modifier key e.g. ctrl+x followed by s or b". These are different interaction patterns. The implementation needs to support both or choose one.

3. **Whether the Unicode fallback in KeyToTermBytes was intentionally designed for key names** — The code at input.go:104-107 has a comment "Multi-character unknown keys (e.g., Unicode) → send as-is." The intent is clearly for Unicode text, but the implementation doesn't distinguish between Unicode text and unrecognized key names. This may have been an oversight during the Bubble Tea v1→v2 migration.

## FALSE — Directly Contradicted by Code

1. **"The spacebar works correctly in the bouncing logo"** — FALSE. Pressing spacebar sends the literal string "space" to the PTY, not a space character.

2. **"KeyToTermBytes correctly handles all Bubble Tea key names"** — FALSE. It handles named keys in its switch statement, ctrl+letter, modifier+nav, and alt+key. It does NOT handle "space" or many other key names that Bubble Tea v2 produces.

3. **"The msg.text fallback handles the spacebar case"** — FALSE. The fallback is unreachable because keyToTermBytes returns a non-null value for "space".

## Recommended Fix Strategy

The fix requires changes at THREE layers:

### Layer 1: Go — `internal/termmux/input.go`
- Add `"space"` to the named keys switch → `return " ", true`
- Tighten the Unicode fallback to only match actual Unicode (non-ASCII multi-rune strings)
- For unrecognized ASCII-only key names, return `("", false)` so JS callers can use `msg.text` fallback

### Layer 2: Go — `internal/builtin/bubbletea/bubbletea.go` (optional but valuable)
- Consider exposing a `msg.parsedKey` or similar field that separates the key name from modifiers, making it easier for JS code to match control keys regardless of modifier state

### Layer 3: JS — `scripts/example-15-bouncing-logo.js`
- Add ctrl+key variants to handleControlKey: `key === 'ctrl+p'`, `key === 'ctrl+b'`, `key === 'ctrl+s'`, `key === 'ctrl+q'`
- Implement a key sequence state machine for the "ctrl+x then action" pattern the user requested
- Use `msg.mod` and `msg.text` fields more intelligently for key matching

### Layer 4: Tests
- Add `TestKeyToTermBytes_SpaceKey` testing `KeyToTermBytes("space")` → `(" ", true)`
- Add test for unrecognized key names returning false
- Consider integration test for key forwarding pipeline
