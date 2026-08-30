# 02 — Kill Conditions: What Actually Breaks

## KILL-001: Spacebar sends literal "space" to PTY

**Status in code**: TRUE — verified by tracing `KeyToTermBytes("space")` through input.go

**Scenario**: User presses spacebar while the bouncing logo is running. The nested PTY (running /bin/sh) receives the literal string "space" instead of a space character. The shell displays "space" on screen as if the user typed those five characters.

**Failure path**:
1. Terminal sends spacebar key event
2. Bubble Tea v2 produces `tea.KeyPressMsg` with `String() = "space"`
3. `msgToJS` creates `{ type: "Key", key: "space", text: " ", mod: [] }`
4. JS `handleControlKey(m, "space")` returns null (no match for "space")
5. JS calls `termmux.keyToTermBytes("space")`
6. Go `KeyToTermBytes("space")` reaches the Unicode fallback at line 105-107
7. Returns `("space", true)` — the literal 5-character string
8. Goja converts to JS string `"space"` (not null)
9. JS forwards `"space"` to PTY via `m.mgr.input("space")`
10. Shell receives "space" as typed input

**Probability**: CERTAIN — every spacebar press triggers this

**Severity**: CRITICAL — spacebar is one of the most common keys; the PTY is unusable for normal typing

**Mitigations in code**: NONE — no guard against this path

## KILL-002: Ctrl+letter bypasses control handler

**Status in code**: TRUE — verified by tracing `handleControlKey("ctrl+s")` and `handleControlKey("ctrl+b")`

**Scenario**: User presses Ctrl+S or Ctrl+B expecting to resize the bouncing pane. Instead, the control handler doesn't match (it checks `key === 's'` and `key === 'b'`, not `key === 'ctrl+s'`), falls through to `keyToTermBytes("ctrl+s")`, which converts it to control character `\x13` and forwards it to the PTY. The PTY receives a terminal control character instead of a resize command.

**Failure path**:
1. Terminal sends Ctrl+S key event
2. Bubble Tea v2 produces `tea.KeyPressMsg` with `String() = "ctrl+s"`
3. `msgToJS` creates `{ type: "Key", key: "ctrl+s", text: "", mod: ["ctrl"] }`
4. JS `handleControlKey(m, "ctrl+s")` — checks: `"ctrl+s" === 's'` → FALSE, `"ctrl+s" === 'b'` → FALSE
5. Returns null — key is not handled as a control action
6. JS calls `termmux.keyToTermBytes("ctrl+s")`
7. Go `KeyToTermBytes("ctrl+s")` matches the `ctrl+letter` case at line 72-80
8. Returns `("\x13", true)` — the XOFF control character
9. PTY receives XOFF — flow control or no-op depending on stty settings

**Probability**: CERTAIN — every ctrl+letter press for control keys triggers this

**Severity**: HIGH — the documented controls ([B] Bigger, [S] Smaller, [P] Pause) don't work when pressed with Ctrl

**User expectation mismatch**: The script's UI renders `[B] Bigger`, `[S] Smaller`, `[P] Pause` — these labels imply bare keypresses work (and they do). But the user's request explicitly asks for modifier key support (ctrl+x followed by s or b). The current code has NO mechanism for chorded/sequenced key input.

## KILL-003: All named keys not in KeyToTermBytes are forwarded as literal strings

**Status in code**: TRUE — the Unicode fallback at input.go:105-107 catches any multi-character key name not containing "+"

**Scenario**: Any Bubble Tea key name that is not explicitly handled in the `KeyToTermBytes` switch statement gets forwarded as its literal string. This includes:
- `"space"` → forwards "space"
- `"capslock"` → forwards "capslock"
- `"num0"` through `"num9"` → forwards "num0"..."num9"
- Any future key names added to Bubble Tea v2

**Probability**: CERTAIN for spacebar, HIGH for numpad keys

**Severity**: MEDIUM — affects less common keys but reveals a systemic design flaw

## KILL-004: KeyToTermBytes returns true for unrecognized keys

**Status in code**: TRUE — the Unicode fallback at input.go:105-107 returns `true` for any multi-character string without "+"

**The problem**: The Go function's contract says it returns `true` when the key was "recognized". But the Unicode fallback returns `true` for strings like "space" or "capslock" that are clearly NOT valid Unicode characters being forwarded intentionally. This makes it impossible for the JS caller to distinguish between "this key was intentionally forwarded as text" and "this key name wasn't recognized and was forwarded as a literal string by accident".

**Impact**: The `msg.text` fallback in the JS code is designed to handle this case, but it's unreachable because `KeyToTermBytes` claims to have recognized the key when it hasn't.
