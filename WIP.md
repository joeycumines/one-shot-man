# WIP: R28 IN PROGRESS — Test quality audit, assertion strengthening, dead code removal

## Status: R01-R27 Done. R28 In Progress. R29-R41 Not Started.

### Session: 2026-03-04 (continuing)

### What's been accomplished in R28:
1. **Deep test quality audit** completed via subagent:
   - 13 skips verified as legitimate (integration-flag or platform-gated)
   - Satellite test files verified as containing unique coverage (NOT safe to delete)
   - gitMockSetupJS pattern verified as complementary (not dangerous)

2. **Shallow assertions strengthened**:
   - `TestIntegration_HeuristicSplitEndToEnd`: Deep validation of analysis files, statuses, groups, splits
   - `TestIntegration_AutoSplitMockMCP_TUIObservation`: Step name + output content validation
   - `pr_split_04_validation_test.go`: 6 tests with error message content validation

3. **Dead PTY tests removed**:
   - Two permanently-skipped `t.Skip("legacy")` tests deleted from pr_split_pty_unix_test.go
   - `termtest` import removed (no longer needed)
   - File reduced from 519 lines to 146 lines

4. **Pre-existing test timeout bugs fixed**:
   - `verifyGoBuild`/`verifyGoTest` (pr_split_complex_project_test.go): Added 2-min context timeouts, suppressed race detector inheritance via GOFLAGS="" 
   - `NewPickAndPlaceHarness` (pick_and_place_harness_test.go): Per-pattern 15s timeouts instead of relying on harness-level context
   - `BuildPickAndPlaceTestBinary`: Added 3-min context timeout

5. **Verification results**:
   - `make all` — GREEN (675s, no timeouts)
   - `make lint` — GREEN (no dead code, no vet/staticcheck issues)
   - `make integration-test-vterm` — GREEN (VTermCleanExit 11.6s, VTermHeuristicRun 8.0s)

### What remains for R28:
- Linux container validation (`make make-all-in-container`)
- Windows validation (`make make-all-run-windows`)
- Run optional macOS integration tests (real Claude if available)
- Final verification all acceptance criteria met

### Key files modified in R28:
- `internal/command/pr_split_pty_unix_test.go` (dead tests removed, 519→146 lines)
- `internal/command/pr_split_integration_test.go` (deep assertion strengthening)
- `internal/command/pr_split_04_validation_test.go` (error message validation)
- `internal/command/pr_split_complex_project_test.go` (subprocess timeouts + race suppression)
- `internal/command/pick_and_place_harness_test.go` (per-pattern timeouts, build timeout)
- `config.mk` (test-audit-command target)

### Next Action:
Run Linux container validation, then continue with R29+ tasks.
