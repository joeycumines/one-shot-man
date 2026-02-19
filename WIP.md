# WIP — Takumi Session State

## Quick Resume Checklist
1. Read blueprint.json → find first "Not Started" task → execute it
2. Run `make check-session-time` to verify session timer
3. Run `make find-test-failures` if in doubt about state
4. NEVER stop. NEVER declare "done". EXPAND and ITERATE.

---

## Session Identity
- **Branch**: wip (256+ commits ahead of main)
- **Session timer file**: `.session-timer`
- **Session start**: 2026-02-20T08:00:00Z (reset)
- **Timer check**: `make check-session-time`

---

## Current State (2026-02-20, session 2+)

### Completed this session:
- **T001**: macOS baseline passes
- **T002**: Renamed orchestrator.js → claude-mux.js, updated all references
- **T003**: Model selection TUI parser extension (model_nav.go, enhanced parser, JS bindings)
- **T004**: MCP session coordination hardening (validation, seq numbers, heartbeat, 20 new tests, fuzz)
- **T005**: MCP session docs (command.md session coordination section, architecture-claude-mux.md §6)
- **T006**: Dynamic MCP config per instance (mcp_config.go, Unix socket/TCP, config JSON gen, JS bindings)
- **T007**: Session isolation (instance.go, InstanceRegistry sync.Map, isolated state dirs, tests with -race)
- **T008**: Guard rails — PTY monitors (guard.go, GuardAction/GuardConfig/Guard, JS bindings, 40+ tests)
- **T009**: Guard rails — MCP monitors (mcp_guard.go, MCPGuard, frequency/repeat/allowlist, JS bindings, 30+ tests)
- **T010**: Error recovery and cancellation (recovery.go, Supervisor state machine, ErrorClass/RecoveryAction/RecoveryDecision, context propagation, graceful shutdown, JS bindings, 30+ tests)
- **T011**: Concurrent instance management — pool (pool.go, Pool with acquire/release dispatch, round-robin, sync.Cond blocking, Drain/WaitDrained/Close, health tracking, JS bindings, 30+ tests)
- **T012**: TUI multiplexing — multi-instance panel (panel.go, Panel with Alt+1..9 switching, per-pane scrollback, PgUp/PgDown, health indicators, StatusBar, getVisibleLines, JS bindings, 40+ tests)
- **T013**: Expose building blocks as JS API (Parser.Patterns() + JS binding, example-08-claude-mux-api.js, scripts/README.md)
- **T014**: ManagedSession composing Parser+Guard+MCPGuard+Supervisor (session_mgr.go, 20+ tests, JS bindings with SESSION_* constants, full lifecycle)
- **T015**: Safety validation (safety.go, intent/scope/risk/policy classification, composable Validator interface, CompositeValidator, SafetyConfig, SafetyStats, JS bindings, 40+ tests)
- **T016**: Choice resolution (choice.go, Candidate+Criterion+ChoiceConfig+ChoiceResolver, 25+ tests, JS bindings). ALSO FIXED: ManagedSession race condition — Guard/MCPGuard calls now under s.mu in ProcessLine/ProcessCrash/ProcessToolCall/CheckTimeout/Snapshot (callbacks remain outside lock to prevent deadlock). 4,529 tests pass with -race.
- **T017**: PR split rewrite for claudemux (orchestrate-pr-split.json: category→claudemux, run cmd with flagDefs; orchestrate-pr-split.js v2.0.0: claudemux integration via selectStrategy+ChoiceResolver, conflict classification in executeSplit, verifyEquivalenceDetailed with diff, createSelectStrategyNode BT leaf; pr_split_test.go: +4 tests). 4,533 tests pass.

### Known pre-existing flaky tests:
- **TestRecording_Goal** (internal/scripting): TUI timing
- **TestRecording_PromptFlow** (internal/scripting): TUI timing
- **TestPickAndPlace_MousePick_HoldingItem** (internal/command): PTY mouse timing
- **TestSessionsListAndClean** (internal/command): TempDir cleanup race
- **TestSuperDocument_ViewportUnlocksOnScrollSnapsBackOnTyping** (internal/scripting): PTY hang under load
- All pass on re-run.

### Next task: T018
Main claude-mux entry point command.

### No commits made yet this session. Rule of Two needed before committing.
