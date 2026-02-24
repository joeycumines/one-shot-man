# WIP — PR Split Consolidation

## Status: COVERAGE PUSH — Tests written, awaiting Rule of Two

### Session Context
- Branch: `wip`
- Previous commit: `55ec7e6` (composite BT fixes + behavioral tests — committed and verified)
- Current: Coverage tests added for claudemux package error paths

### Commit history
1. `7477aae` — Initial pr-split consolidation (had composite BT defects)
2. `55ec7e6` — Fixed all 4 composite functions, added 7 behavioral tests, docs, CHANGELOG

### What changed since 55ec7e6 (UNCOMMITTED)

#### Coverage tests for claudemux package (15 new tests across 3 files)

**guard_test.go** — 6 new tests for computeBackoff overflow edge cases:
- `TestGuard_ComputeBackoff_FloatOverflow_WithMaxDelay` — factor=+Inf → clamps to MaxDelay
- `TestGuard_ComputeBackoff_FloatOverflow_NoMaxDelay` — factor=+Inf → falls back to InitialDelay
- `TestGuard_ComputeBackoff_Int64Overflow_WithMaxDelay` — Duration overflow → clamps to MaxDelay
- `TestGuard_ComputeBackoff_Int64Overflow_NoMaxDelay` — Duration overflow → positive (saturated)
- `TestGuard_ComputeBackoff_NaNFactor` — direct computeBackoff with +Inf factor
- (No change to existing Overflow test which was already present)

**instance_test.go** — 4 new tests for Create error paths:
- `TestInstanceRegistry_Create_LongSessionID` — 80-char ID → dir basename truncated to 64
- `TestInstanceRegistry_Create_StateDirFail` — invalid baseDir → error, not registered
- `TestInstanceRegistry_Create_WriteStateFail` — read-only stateDir → error, not registered
- `TestInstanceRegistry_Create_LogsDirFail` — logs as regular file → error, not registered

**control_test.go** — 5 new tests for send/GetStatus error paths:
- `TestControl_Send_ServerClosesImmediately` — empty response → error
- `TestControl_Send_MalformedResponse` — garbage JSON → unmarshal error
- `TestControl_GetStatus_NonOKResponse` — ok=false → error propagation
- `TestControl_GetStatus_InvalidResultJSON` — ok=true but bad Result → decode error
- `TestControl_EnqueueTask_InvalidResultJSON` — ok=true but bad Result → decode error

#### Coverage improvements (before → after)
| Function | Before | After | Δ |
|----------|--------|-------|---|
| instance.Create | 57.1% | ≥90% | +33%+ |
| guard.computeBackoff | 68.4% | 84.2% | +15.8% |
| control.GetStatus | 66.7% | 80.0% | +13.3% |
| control.send | 68.4% | 78.9% | +10.5% |
| **Total package** | **94.2%** | **94.8%** | **+0.6%** |

### Build status (last verified)
- `go build ./...` PASS
- All claudemux tests PASS with `-race` (6.0s)
- All 15 new tests PASS

### Rule of Two (for this diff)
- Not yet run — PENDING

### Files touched (since 55ec7e6)
- `internal/builtin/claudemux/guard_test.go` — 6 new overflow tests
- `internal/builtin/claudemux/instance_test.go` — 4 new error-path tests + runtime/strings imports
- `internal/builtin/claudemux/control_test.go` — 5 new error-path tests + fmt/net/runtime imports

### Next steps
1. Rule of Two review gate on diff vs HEAD
2. Commit
3. Continue coverage push (T034-T039: remaining functions)
4. Integration test with ollama (T024-T027)
5. Cross-platform verification (T040-T042)
