# WIP: T04a+T21-T26 Committed — Async sendToHandle, Error Feedback, osm:pty Removal

## Status: COMMITTED ✅ — Rule of Two PASSED

### Committed Changes (T04a + T21-T26):

1. **T04a (async sendToHandle)** — sendToHandle converted to async function.
   10ms delay between text/newline via `await new Promise(resolve => setTimeout(resolve, 10))`.
   All callers (resolveConflicts, automatedSplit, heuristicFallback, resolveConflictsWithClaude,
   step(), claude-fix strategy, command handlers) converted to async/await.
   SEND_TEXT_NEWLINE_DELAY_MS constant added.

2. **T21 (Already implemented)** — finishTUI already emits resume instructions.

3. **T22 (Test written)** — TestAutoSplit_ErrorFeedback_ResumeInstructions.

4. **T24 (Comments fixed)** — Stale sendToHandle comment block corrected.

5. **T25 (osm:pty JS removed)** — module.go deleted, register.go updated, 7 tests removed,
   docs updated.

### Next Steps:
1. T27: Real AI integration test — check `which ollama`, `which claude`
2. T28: Cross-platform validation: Windows
3. T29: Documentation: developer guide for pr-split integration tests
4. T30: Final validation suite
5. T31: Indefinite cycle expansion
6. T04b: Real E2E test with Claude
7. T04c: Windows verification
