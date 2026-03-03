# WIP: Rearchitecture triggered — Cycle 6 tests + replan ready to commit

## Status: Hana intervention addressed. Blueprint replanned with R01-R28. Cycle 6 tests pass. Full suite green.

### Commits (historical, all on split/api):
- db95f10: Cycle 5 (T107+T108+T110)
- b3e0529: Cycle 4 (T95-T100+T102+T103)
- (earlier cycles: T01-T94)

### Cycle 6 Changes (UNCOMMITTED):
- pr_split_session_cancel_test.go: T114 — TestVerifySplits_CancellationMidIteration
- pr_split_integration_test.go: T115 — TestClaudeCodeExecutor_SpawnHealthCheck_DeadProcess
- pr_split_autosplit_recovery_test.go: T117 — TestAutoSplit_WatchdogTimeout
- pr_split_conflict_retry_test.go: T116 — TestResolveConflictsWithClaude_SuccessfulFix
- blueprint.json: COMPLETELY REPLANNED per Hana intervention (R01-R28)
- config.mk: Blocking $(error) removed after compliance, run-current updated
- WIP.md: Updated for rearchitecture plan

### Rearchitecture Plan (R01-R28):
Hana intervention (scratch/pr-split-auto-split-completely-broken.md) mandates:
1. R01: Archive monolith to scratch/archive/pr-split-v1/
2. R02: Design chunk architecture (14 chunks)
3. R03: Chunk loading infrastructure in pr_split.go
4. R04-R17: Extract each chunk with independent tests
5. R18-R19: Wire test infra to chunks, remove monolith
6. R20-R22: Cull AI slop (JS, Go, tests)
7. R23-R26: Integration testing (mock MCP + REAL Claude)
8. R27-R28: Final verification + documentation

### Next Steps:
1. Rule of Two on cycle 6 changes
2. Commit cycle 6
3. Begin R01: Archive existing implementation
