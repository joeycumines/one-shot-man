# WIP — PR Split Consolidation

## Status: FIXES APPLIED — Awaiting Rule of Two

### Session Context
- Branch: `wip`
- Previous commit: `7477aae` (initial consolidation, had composite BT function defects)
- Current: Composite functions fixed, behavioral tests added, docs updated

### What changed since 7477aae (UNCOMMITTED)

#### Fixed composite BT functions (5 behavioral deviations resolved)
1. **`spawnAndPrompt`** — Restored 3-step sequence (spawn → send → **wait**). Changed API to config-object style `(bb, registry, config)` where config has `.provider`, `.spawnOpts`, `.prompt` — matching original claude-mux.js API exactly.
2. **`verifyAndCommit`** — Fixed step order to tests → verify → commit (was verify → tests → commit). Restored default message to `'Automated commit'` (capital A).
3. **`spawnPromptAndReadResult`** — Now has proper 3-step sequence (spawn → send → wait). Uses positional parameter style `(bb, registry, providerName, opts)`.
4. **`createPlanningActions`** — Restored ALL 7 PA-BT actions with proper preconditions and effects for backchaining:
   - SpawnClaude: [] → agentSpawned=true
   - SendPrompt: agentSpawned → promptSent=true
   - WaitForResponse: promptSent → responseReceived=true
   - RunTests: responseReceived → testsPassed=true
   - VerifyOutput: testsPassed → verified=true
   - CommitChanges: testsPassed → committed=true
   - SplitBranch: committed → branchCreated=true

#### Added behavioral tests (7 new tests in templates_test.go, 23 total)
- `TestTemplates_SpawnAndPrompt_IncludesWaitForResponse`
- `TestTemplates_SpawnAndPrompt_ConfigObjectAPI`
- `TestTemplates_SpawnPromptAndReadResult_PositionalAPI`
- `TestTemplates_VerifyAndCommit_OrderTestsThenVerify`
- `TestTemplates_VerifyAndCommit_DefaultMessage`
- `TestTemplates_CreatePlanningActions_HasPreconditionsAndEffects`
- `TestTemplates_CreatePlanningActions_BackchainOrder`

#### Updated documentation
- `CHANGELOG.md` — Added entries for `osm pr-split` built-in command, removed entries for deleted files
- `docs/architecture-claude-mux.md` — Fixed 17 stale references to deleted files
- `internal/command/goal_test.go` — Fixed stale comment referencing deleted goal file

### Build status (last verified)
- `go build ./...` PASS
- `go vet ./...` PASS
- All 23 templates tests PASS
- All 15 pr_split command tests PASS
- All claudemux pr_split tests PASS

### Rule of Two (for this diff)
- Not yet run — PENDING

### Files touched (since 7477aae)
- `internal/command/pr_split_script.js` — composite functions rewritten
- `internal/builtin/claudemux/templates_test.go` — 7 new behavioral tests + message casing fixes
- `internal/command/goal_test.go` — stale comment fix
- `CHANGELOG.md` — pr-split entries added
- `docs/architecture-claude-mux.md` — stale references fixed
- `blueprint.json` — rewritten for new task set
- `WIP.md` — this file

### Next steps
1. Rule of Two review gate on diff vs HEAD
2. Commit
3. Coverage push (T034-T039)
4. Integration test with ollama (T024-T027)
5. Cross-platform verification (T040-T042)
