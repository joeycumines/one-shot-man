# WIP: T048-T051 COMPLETE — VTerm + Integration + Alt-Screen Race Fix

## Status
T048-T051 implemented and `make all` passes clean. Ready for Rule of Two.

### Completed This Session
- **T048**: VTerm VT100 virtual terminal buffer (vterm.go ~950 lines, vterm_test.go ~900 lines)
  - Full state machine parser: ground→escape→CSI/OSC
  - CUU/CUD/CUF/CUB/CUP/HVP cursor movement
  - ED/EL/ECH erase, IL/DL insert/delete line, ICH/DCH insert/delete chars
  - DECSTBM scroll regions, SGR (16/256/truecolor), DECSET/DECRST 1049 (alt-screen)
  - pendingWrap deferred autowrap matching real VT100 behavior
  - Render() produces minimal ANSI diff output
  - 29+ tests all passing

- **T049**: VTerm integrated into TUIMux
  - `childVterm *VTerm` field on TUIMux struct
  - Created on Attach(24x80), nil on Detach
  - Terminal dimension capture + VTerm.Resize() in RunPassthrough
  - VTerm Render() restoration for non-first swaps (toggle back to Claude)
  - Child→stdout goroutine tees to childVterm.Write()
  - childVterm captured under mutex before goroutine launch
  - Removed 6 deadcode accessor methods (RenderDirty/IsAltScreen/Rows/Cols/CursorPos/CellAt)
  - Added test-only helpers (vtRows/vtCols/vtCursorPos/vtIsAltScreen/vtCellAt)
  - Updated TestRunPassthrough_FirstSwapClearsScreen for VTerm restoration

- **T050**: Alt-screen race fix
  - Added synchronous `\x1b[?1049l` write BEFORE RunPassthrough
  - Added synchronous `\x1b[?1049h\x1b[2J\x1b[H` AFTER RunPassthrough
  - Both idempotent — safe regardless of BubbleTea's async processing timing

- **T051**: `make all` passes clean (build + vet + staticcheck + deadcode + all tests)

## Files Modified
- internal/termui/mux/vterm.go — NEW (VT100 virtual terminal buffer)
- internal/termui/mux/vterm_test.go — NEW (29+ tests)
- internal/termui/mux/mux.go — VTerm integration (childVterm field, Attach/Detach, RunPassthrough tee + restoration)
- internal/termui/mux/mux_test.go — Updated TestRunPassthrough_FirstSwapClearsScreen
- internal/command/pr_split.go — Synchronous alt-screen escape sequences around RunPassthrough
- blueprint.json — T048/T049/T050/T051 marked Done

## Remaining Tasks
- T046: TestIntegration_ClaudeSpawnSwitch (PTY integration test)
- T047: TestIntegration_RealClaude_ClassificationE2E (full AI pipeline test)
- T052: Update project.mk integration targets
- T053: Final WIP.md checkpoint

## Next Steps
1. Rule of Two verification for T048-T051
2. Commit
3. T046/T047/T052/T053
