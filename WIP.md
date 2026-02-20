# WIP — Takumi Session State

## Quick Resume Checklist
1. Read blueprint.json → find first "Not Started" task → execute it
2. Run `make make-all-with-log` to verify build is GREEN
3. NEVER stop. NEVER declare "done". EXPAND and ITERATE.

---

## Session Identity
- **Branch**: wip (30+ commits ahead of main)
- **Session timer file**: `.session-timer`

---

## Current State

### Completed this session (session 3+):
- **T101-T108**: SSE, ReadableStream, MCP, txtar fixes (prior in session)
- **T109**: macOS Deep Anchor (sysctl, committed f086711)
- **T110**: claude-mux run subcommand (PTY dispatch, committed ca9cd6e)
- **T111**: ManagedSession health tracking (guard events, ProcessCrash, committed 1d62cb1)
- **T112**: Panel TUI data layer wiring (output routing, health indicators, StatusBar, committed ab94850)
- **T113**: Integration tests for claudemux run (8 tests, 2 mock agents, committed 588c5d6)
- **T114**: Model auto-navigation (tryNavigateModel, sliding window, 5 tests, committed 81a7422)
- **T115**: mcp-make command (MCP-exposed Make tools, 12 tests, committed 779341b)
- **T116**: Control socket (ControlServer/ControlClient, Unix domain socket, JSON protocol, 17 tests, wired into run/submit/stop/status, committed dc51df5)

- **T117**: Dynamic task dispatch wiring (dynamicDispatchLoop, controlAdapter methods, 3 tests, committed b8ffd13)

### Next task: T118

### Build state: GREEN (all packages pass)

### Known pre-existing flaky tests:
- TestRecording_Goal (TUI timing)
- TestRecording_PromptFlow (TUI timing)
- TestPickAndPlace_MousePick_HoldingItem (PTY mouse timing)
- TestSessionsListAndClean (TempDir cleanup race)
- TestSuperDocument_ViewportUnlocksOnScrollSnapsBackOnTyping (PTY hang)
