# T394: termmux Claude Terminal Input Audit

## Executive Summary

Audit of the complete termmux pipeline for Claude terminal input in `osm pr-split`.
**Critical bug found and fixed:** Ctrl+] passthrough had stdin contention between
BubbleTea's cancelreader goroutine and RunPassthrough's stdin reader.

## Complete Input Chain

### Split-View Mode (Key-by-Key)

```
BubbleTea KeyMsg (user presses key)
  → pr_split_16e_tui_update.js: wizardUpdateImpl
  → checks: splitViewEnabled && splitViewFocus === 'claude' && !CLAUDE_RESERVED_KEYS[k]
  → pr_split_16d_tui_handlers_claude.js: keyToTermBytes(key)
  → Maps BubbleTea key string → terminal byte sequence
  → pr_split_16e_tui_update.js: tuiMux.writeToChild(bytes)
  → internal/builtin/termmux/module.go: Go binding
  → internal/termmux/termmux.go: Mux.WriteToChild([]byte(data))
  → internal/termmux/stringio.go: stringIOAdapter.Write → inner.Send(string(p))
  → internal/builtin/claudemux/claude_code.go: ptyAgentHandle.Send → proc.Write(input)
  → internal/builtin/claudemux/pty/pty.go: Process.Write → ptyFile.Write
  → KERNEL: PTY master fd → slave fd → Claude CLI stdin
```

**Status: ✅ CORRECT** — Fully wired, each hop has appropriate error handling.

### Full Passthrough Mode (Ctrl+])

**BEFORE T394 (BROKEN):**
```
BubbleTea KeyMsg (Ctrl+])
  → wizardUpdateImpl: k === 'ctrl+]'
  → tuiMux.switchTo('claude')                    ← BLOCKS in RunPassthrough
  → RunPassthrough starts goroutine reading m.stdin
  → BubbleTea's cancelreader ALSO reading stdin  ← STDIN CONTENTION BUG
```

**AFTER T394 (FIXED):**
```
BubbleTea KeyMsg (Ctrl+])
  → toggleModel.Update intercepts (Go level)
  → toggleModel.toggleCmd returns tea.Cmd
  → BubbleTea cmd goroutine executes:
    1. p.ReleaseTerminal()     ← stops cancelreader
    2. write \x1b[?1049l       ← exit alt-screen
    3. RunJSSync: prSplit._onToggle()
       → tuiMux.switchTo()    ← blocks in RunPassthrough (exclusive stdin)
    4. write \x1b[?1049h\x1b[2J\x1b[H  ← enter alt-screen
    5. p.RestoreTerminal()     ← restart cancelreader
  → ToggleReturn msg sent to JS update for notifications
```

**Status: ✅ FIXED** — No stdin contention. BubbleTea properly released before passthrough.

### Mouse Forwarding

```
BubbleTea MouseMsg
  → wizardUpdateImpl: splitViewEnabled && splitViewFocus === 'claude'
  → motion/release/wheel: mouseToTermBytes(msg, offsetRow, offsetCol) → writeMouseToPane
  → press: handleMouseClick first (zone detection) → fallback → mouseToTermBytes → writeMouseToPane
  → writeMouseToPane: tuiMux.writeToChild(bytes).
```

**Status: ✅ CORRECT** — SGR mouse encoding handles buttons, modifiers, 1-based coordinates.

### Claude Output Path

```
Claude CLI stdout → PTY slave → PTY master (readable)
  → ptyio.BufferedReader.ReadLoop: Read → output channel
  → termmux.teeLoop: drains channel → VTerm.Write (always) + stdout (passthrough only)
  → pollClaudeScreenshot (500ms tick): tuiMux.childScreen() → VTerm.ContentANSI()
  → BubbleTea view renders s.claudeScreen in split-view bottom pane
```

**Status: ✅ CORRECT** — Output always captured via VTerm.

## Bug Found: Ctrl+] Stdin Contention (CRITICAL)

**Root Cause:** `startWizard` called `tea.run(_wizardModel, {altScreen: true, mouse: true})`
without `toggleKey`/`onToggle` options. The Ctrl+] handler in JS directly called
`tuiMux.switchTo('claude')` from within BubbleTea's Update function (via RunJSSync).
This blocked the event loop while RunPassthrough started its own stdin reader goroutine.
BubbleTea's cancelreader goroutine was still active, creating two concurrent stdin readers
on the same fd. Keystrokes were non-deterministically split between the two readers.

**Fix:** Wire `toggleKey: 0x1D` and `onToggle: prSplit._onToggle` in `tea.run()` options.
The Go-level `toggleModel` wrapper intercepts Ctrl+] and calls `p.ReleaseTerminal()`
(stopping cancelreader) before invoking `onToggle` (which calls `tuiMux.switchTo()`).
After passthrough exits, `p.RestoreTerminal()` resumes BubbleTea. The manual Ctrl+]
handler in JS was removed; a `ToggleReturn` message handler was added for notifications.

## Files Modified

1. `pr_split_16f_tui_model.js`: Wire toggleKey/onToggle in startWizard, extract _onToggle
2. `pr_split_16e_tui_update.js`: Remove manual Ctrl+] handler, add ToggleReturn handler
3. `pr_split_16_ctrl_bracket_test.go`: Rewrite tests for _onToggle + ToggleReturn
4. `pr_split_16_keyboard_crash_test.go`: Update SwitchTo_WithChild/NoChild tests
5. `pr_split_16_focus_nav_edge_test.go`: Update CtrlBracketTermmux test

## Design Notes

- **CLAUDE_RESERVED_KEYS** blocks arrow keys from Claude in split-view (intentional — used for viewport scroll). Full interaction requires Ctrl+] passthrough.
- **INTERACTIVE_RESERVED_KEYS** (Shell tab) only blocks pane-management keys — better for interactive terminals. Claude tab may benefit from this in future.
- **pollClaudeScreenshot** polls at 500ms — acceptable latency for a read-only viewport.
