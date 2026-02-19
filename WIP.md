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

### Known pre-existing flaky tests:
- **TestRecording_Goal** (internal/scripting): TUI timing
- **TestRecording_PromptFlow** (internal/scripting): TUI timing
- **TestPickAndPlace_MousePick_HoldingItem** (internal/command): PTY mouse timing
- **TestSessionsListAndClean** (internal/command): TempDir cleanup race
- **TestSuperDocument_ViewportUnlocksOnScrollSnapsBackOnTyping** (internal/scripting): PTY hang under load
- All pass on re-run.

### Next task: T013
Expose claude-mux building blocks as JS API.

### No commits made yet this session. Rule of Two needed before committing.
