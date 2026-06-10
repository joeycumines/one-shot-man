# WIP.md — Session: Tue Jun 10 2026 — Bouncing Logo Input Fix (EXPANDED SCOPE)

## Current Status
IN PROGRESS. Code changes complete for example-15. Two critical bugs found and fixed by first review gate. Full suite passes with -short. VT audit found 25 findings (1 CRITICAL, 6 HIGH, 9 MEDIUM). First review gate found 10 findings (1 CRITICAL fixed, 1 HIGH fixed). Remaining: second review gate, then expanded scope to termmux package production-readiness.

## Changes Made (unstaged)
1. `scripts/example-15-bouncing-logo.js`:
   - handleControlKey: removed bare letter shortcuts, only ctrl+letter now
   - Chord mode: FIXED — now passes 'ctrl+s'/'ctrl+b'/'ctrl+p' to handleControlKey (was passing bare 's'/'b'/'p')
   - buttonIdToKey: returns 'ctrl+p'/'ctrl+b'/'ctrl+s'/'ctrl+q'
   - Button labels: [^P] Pause, [^B] Bigger, [^S] Smaller, [^Q] Quit

2. `internal/example/bouncelogo/bouncelogo_pty_test.go`:
   - Updated 3 tests to use ctrl+letter bytes
   - Added TestBouncingLogo_BareLettersForward, TestBouncingLogo_SpacebarForward
   - FIXED TestBouncingLogo_ChordMode: uses buffer.Contains instead of delta-based expect

3. `internal/termmux/input.go` (prior session): space case + Unicode fallback fix
4. `internal/termmux/input_test.go` (prior session): SpaceKey + UnrecognizedKeyNames tests
5. `scripts/README.md`: updated example-15 description

## Review Gate Status
- FIRST PASS: DONE. Found 10 issues. Fixed F-001 (CRITICAL: chord mode bare letter), F-002 (HIGH: chord test timing).
- SECOND PASS: NOT STARTED. Must run before commit.
- Remaining findings to address in expanded scope: F-003 through F-010

## VT Audit Findings (termmux/vt/) — EXPANDED SCOPE
The VT audit found 25 findings. Most impactful for production readiness:

### CRITICAL
- F1: ActiveScreen() leaks mutable *Screen outside mutex → data races

### HIGH (6)
- F2: CSI cursor commands don't clear PendingWrap → rendering glitches
- F3: ESC cursor commands don't clear PendingWrap → rendering glitches
- F18: CSI u (restore cursor) no bounds check → panic after resize
- F19: ESC 8 (DECRC) no bounds check → panic after resize
- F25: switchToPrimary restores cursor without bounds check → panic after resize
- F10: Missing DECOM, DSR, DECSCUSR, many DECSET modes → programs break

### MEDIUM (9)
- F4: No VT mouse tracking unit tests
- F5: InsertLines/DeleteLines don't clear PendingWrap
- F12/24: Resize doesn't clear PendingWrap → stale wrap after grow
- F20: EraseDisplay no CurRow bounds check
- F6: OSC discarded (no title storage)
- F7: Parser conflates private prefix with intermediate bytes
- F8: DCS data entirely discarded (no Sixel support)
- F22: RenderFullScreen loses scroll region, tab stops, mouse modes
- F10: Missing DECOM origin mode → scroll region cursor positioning wrong

## Next Session Instructions
1. Read WIP.md and blueprint.json FIRST
2. Run `go test -short -count=1 -timeout=5m ./...` to verify current state
3. Run SECOND review gate (new subagent)
4. Address remaining review findings (F-003 through F-010)
5. Expand blueprint with termmux/vt production-readiness tasks based on VT audit
6. Fix PendingWrap clearing across all cursor movement operations (F2, F3, F5, F12)
7. Fix cursor restore bounds checking (F18, F19, F25)
8. Consider removing ActiveScreen() or making it safe (F1)
9. Run full test suite after each fix
10. DO NOT COMMIT until second review gate passes

## Key File Locations
- Bouncing logo script: scripts/example-15-bouncing-logo.js
- PTY tests: internal/example/bouncelogo/bouncelogo_pty_test.go
- Mock shell: internal/example/bouncelogo/mock_shell.sh
- Go key forwarding: internal/termmux/input.go
- Go tests: internal/termmux/input_test.go
- VT parser: internal/termmux/vt/vt.go
- VT screen: internal/termmux/vt/screen.go
- CSI handler: internal/termmux/vt/csi.go
- ESC handler: internal/termmux/vt/esc.go
- Parser: internal/termmux/vt/parser.go
- Render: internal/termmux/vt/render.go
- Session manager: internal/termmux/manager.go
- JS bindings: internal/builtin/termmux/module.go
- Prior autopsy: docs/key-input-autopsy-20260610/
