# WIP: R27 IN PROGRESS — VTerm observation tests working, exit hang FIXED

## Status: R01-R26 Done. R27 In Progress. R28-R42 Not Started.

### Session: 2026-03-04 (continuing)

### What's been accomplished in R27:
1. **VTerm observation infrastructure** (pr_split_termmux_observation_test.go):
   - `vtermObserver` struct: wraps VTerm, collects periodic snapshots
   - `typeToPrompt()` helper: sends bytes one-by-one with 20ms delay
   - `snapshotsContain()`, `vtermObservationSummary()` helpers
   - `newVTermObserver()`, `pumpPTY()`, `startSnapshotter()`, etc.

2. **PTY input bug ROOT CAUSED AND FIXED**:
   - go-prompt's readBuffer reads in bulk from raw PTY
   - Multi-byte input treated as single unrecognized key → text insertion
   - Fix: `typeToPrompt()` sends byte-by-byte with delays
   - All 5 multi-byte PTY write sites updated

3. **Exit hang FIXED**:
   - Added OSM_EXIT_TRACE instrumentation to terminal.go (opt-in, zero-cost)
   - Added 5s timeout to stopWriter() in tui_manager.go (defense-in-depth)
   - Trace proves clean shutdown: tuiManager.Run() → session save → Close() → done

4. **Test results**:
   - `TestIntegration_PrSplit_VTermCleanExit` — **PASS** (exit in 32ms)
   - `TestIntegration_PrSplit_VTermHeuristicRun` — **PASS** (4 branches, equivalence OK)
   - `TestIntegration_AutoSplitClaude_VTermObservation` — NOT YET RUN (needs real Claude)
   - `make all` — **GREEN**
   - `make integration-test-vterm` — **GREEN** (both VTerm tests pass)

5. **Flaky test fix**: TestSessionDelete_HappyPath — added lock artifact cleanup

### What remains for R27:
- Run real-Claude `AutoSplitClaude_VTermObservation` test
- Strengthen all validation of input/output over TTY
- Verify broken.md issues fixed with HIGH CONFIDENCE via VTerm observation
- Deep validation of state — PROVE it actually works properly
- Consider strengthening existing real-Claude tests with VTerm capture

### Key files modified:
- `internal/command/pr_split_termmux_observation_test.go` (NEW, ~700 lines)
- `internal/scripting/terminal.go` (OSM_EXIT_TRACE instrumentation)
- `internal/scripting/tui_manager.go` (stopWriter timeout)
- `internal/command/coverage_gaps_batch10_test.go` (flaky test fix)
- `config.mk` (integration-test-vterm target)

### Next Action:
Run real-Claude integration test: `make integration-test-prsplit`
Then continue R27 deeper validation mandate.
