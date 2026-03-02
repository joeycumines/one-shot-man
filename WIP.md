# WIP: T32-T36 Complete — Awaiting Rule of Two + Commit

## Status: PRE-COMMIT — Rule of Two pending

### Last Commit: 629e94a (T04a+T21-T26)

### Uncommitted Changes (T27-T36):

#### T27-T31 (done earlier this session):
- T27: Real Claude test — infra works, timeout was config issue
- T28: GOOS=windows build passes
- T29: docs/pr-split-testing.md created
- T30: make all + integration tests pass
- T31: Scope expansion — T32-T36 added

#### T32: Configurable evalJS timeout
- `loadPrSplitEngineWithEval` extracts `_evalTimeout` from overrides
- Default 60s for unit tests, 10min for real Claude tests
- Both `evalJS` and `evalJSAsync` closures use configurable timeout
- Updated: pr_split_test.go, pr_split_integration_test.go, pr_split_complex_project_test.go

#### T33: Async EAGAIN retry
- `sendWithRetry` converted from sync to `async function`
- `timemod.sleep` replaced with `await new Promise(r => setTimeout(r, ms))`
- Dead `timemod` import removed from pr_split_script.js
- Callers use `await sendWithRetry(...)` 

#### T34: Handle dies between writes test
- TestPrSplitCommand_SendToHandle_HandleDiesBetweenWrites added
- Verifies broken pipe on newline write returns error (2 send calls)

#### T35: Newline EAGAIN retry tests
- TestPrSplitCommand_SendToHandle_NewlineEAGAINRetry (3 calls)
- TestPrSplitCommand_SendToHandle_NewlineEAGAINExhausted (5 calls)

#### T36: Timeout architecture docs
- Timeout Architecture section added to docs/pr-split-testing.md
- ASCII diagram of Go↔JS↔Claude interaction
- EAGAIN retry docs

### Verification: make make-all-with-log PASS (637s command pkg, 0 failures)

### Next Steps:
1. Rule of Two review (this changeset)
2. Commit
3. Indefinite cycle expansion
4. T04b: Real E2E test with Claude (deferred — infrastructure works)
4. T30: Final validation suite
5. T31: Indefinite cycle expansion
6. T04b: Real E2E test with Claude
7. T04c: Windows verification
