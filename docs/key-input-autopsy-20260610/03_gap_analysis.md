# 03 — Gap Analysis: All Gaps Ranked by Severity

## GAP-001: KeyToTermBytes missing "space" named key

**Severity**: CRITICAL
**What's missing**: The `KeyToTermBytes` function in `internal/termmux/input.go` has no case for the key name "space". Bubble Tea v2 uses "space" as the canonical key name for the spacebar.
**Where it should be**: `internal/termmux/input.go:14-69` — the named special keys switch statement
**What the impact is**: Spacebar forwards the literal string "space" to PTY
**What a fix looks like**: Add `case "space": return " ", true` to the switch statement

## GAP-002: KeyToTermBytes Unicode fallback returns true for non-Unicode key names

**Severity**: CRITICAL
**What's wrong**: The fallback at input.go:105-107 returns `(key, true)` for any multi-character string without "+". This includes key names like "space", "capslock", "num0" etc. that are NOT valid Unicode input.
**Where it should be**: `internal/termmux/input.go:99-110` — the end of the function
**What the impact is**: The JS caller cannot distinguish between "recognized key" and "unrecognized key name forwarded as text". The `msg.text` fallback is unreachable.
**What a fix looks like**: The Unicode fallback should only match actual multi-byte Unicode characters (e.g., runes > 127). Key names that are ASCII-only and multi-character should return `("", false)`.

## GAP-003: handleControlKey does not handle ctrl+key variants of control keys

**Severity**: HIGH
**What's missing**: `handleControlKey` in the JS script matches bare letters ("p", "b", "s", "q") but not their ctrl+letter variants ("ctrl+p", "ctrl+b", "ctrl+s", "ctrl+q").
**Where it should be**: `scripts/example-15-bouncing-logo.js:539-569`
**What the impact is**: Ctrl+key combos bypass the control handler and get forwarded to the PTY as control characters
**What a fix looks like**: Add `key === 'ctrl+p'`, `key === 'ctrl+b'`, `key === 'ctrl+s'`, `key === 'ctrl+q'` cases to handleControlKey

## GAP-004: No key chord / prefix key support

**Severity**: HIGH (per user's explicit request)
**What's missing**: The script has no mechanism for chorded key input — e.g., pressing Ctrl+X followed by S to resize. The Bubble Tea v2 model only processes one key event at a time with no state machine for key sequences.
**Where it should be**: The JS update function and model state
**What the impact is**: The user's requested interaction pattern (modifier key prefix followed by action key) is impossible
**What a fix looks like**: Implement a key sequence state machine in the model. When a prefix key (e.g., ctrl+x) is detected, enter a "waiting for action" state. On the next key event, interpret it as an action. Reset state after action or timeout.

## GAP-005: KeyToTermBytes does not handle all Bubble Tea v2 key names

**Severity**: MEDIUM
**What's missing**: Bubble Tea v2 can produce key names that KeyToTermBytes doesn't handle:
- `"capslock"`, `"numlock"`, `"scrolllock"` — lock keys
- `"num0"` through `"num9"` — numpad number keys
- `"numenter"`, `"numadd"`, `"numsubtract"`, etc. — numpad operators
- `"printscreen"`, `"pause"` — system keys
- `"command"` / `"super"` / `"windows"` — super key
**Where it should be**: `internal/termmux/input.go:14-69`
**What the impact is**: These keys get forwarded as literal strings via the Unicode fallback
**What a fix looks like**: Add cases for commonly needed keys. For truly unrepresentable keys (capslock, etc.), return `("", false)` so the caller can handle them.

## GAP-006: No test coverage for the "space" key in KeyToTermBytes

**Severity**: MEDIUM
**What's missing**: `internal/termmux/input_test.go` tests `KeyToTermBytes(" ")` (a literal space character) but NOT `KeyToTermBytes("space")` (the key name that Bubble Tea v2 actually produces).
**Where it should be**: `internal/termmux/input_test.go`
**What the impact is**: The most common key input bug has no test protection
**What a fix looks like**: Add a test case for `KeyToTermBytes("space")` that expects `(" ", true)`

## GAP-007: No integration test for key event forwarding in bouncing logo

**Severity**: LOW
**What's missing**: There is no automated test that simulates key events through the full pipeline (Bubble Tea → msgToJS → handleControlKey → keyToTermBytes → PTY) and verifies correct PTY input.
**Where it should be**: A new test file or extended example_scripts_regression_test.go
**What the impact is**: Regressions in key handling can be introduced without detection
**What a fix looks like**: Integration test that simulates key presses and verifies PTY receives correct bytes
