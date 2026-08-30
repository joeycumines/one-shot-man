# Key Input Autopsy: example-15-bouncing-logo.js

**Date**: 2026-06-10
**Scope**: Keyboard input handling pipeline — from terminal keypress through Bubble Tea v2, Go JS bridge, to PTY forwarding
**Boundary**: `internal/builtin/bubbletea/bubbletea.go` (msgToJS), `internal/termmux/input.go` (KeyToTermBytes), `scripts/example-15-bouncing-logo.js` (handleControlKey + key forwarding)

## One-Sentence Verdict

**The key input pipeline has a semantic mismatch at every layer: Bubble Tea v2 sends structured key names ("space", "ctrl+s") but the consumer code (handleControlKey and KeyToTermBytes) was written for a different key representation, causing spacebar to forward the literal string "space" to the PTY and ctrl+key combos to bypass the control handler entirely.**

## Critical Caveats

- This autopsy does NOT cover mouse input forwarding (which works correctly via SGR encoding)
- The `KeyToTermBytes` function itself is correctly implemented for its stated contract — the bug is in the contract, not the implementation
- The bug is not in Bubble Tea v2 — it correctly produces `"space"` for spacebar and `"ctrl+s"` for ctrl+s per its API

## Document Index

| # | File | Purpose |
|---|------|---------|
| 01 | `01_core_anatomy.md` | How the key input pipeline actually works (from code) |
| 02 | `02_kill_conditions.md` | The specific scenarios that cause catastrophic failure |
| 03 | `03_gap_analysis.md` | All gaps ranked by severity |
| 04 | `04_honest_conclusions.md` | True / Uncertain / False synthesis |

## Recommended Reading Order

1. `01_core_anatomy.md` — understand the pipeline
2. `02_kill_conditions.md` — see what breaks
3. `03_gap_analysis.md` — full inventory of gaps
4. `04_honest_conclusions.md` — the honest assessment
