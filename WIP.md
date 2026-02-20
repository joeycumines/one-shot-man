# WIP — Takumi Session State

## Quick Resume Checklist
1. Read blueprint.json → find first "Not Started" task → execute it
2. Run `make make-all-with-log` to verify build is GREEN
3. NEVER stop. NEVER declare "done". EXPAND and ITERATE.

---

## Session Identity
- **Branch**: wip (300+ commits ahead of main)
- **Session timer file**: `.session-timer`

---

## Current State

### Completed this session (session 3+):
- **T101-T108**: SSE, ReadableStream, MCP, txtar fixes (prior in session)
- **T109**: macOS Deep Anchor (sysctl, committed f086711)
- **T110**: claude-mux run subcommand (PTY dispatch, committed ca9cd6e)

### Next task: T111
Wire Pool dispatch into claudemux run — round-robin task assignment,
health tracking via PTY output monitoring, graceful drain on shutdown.

### Build state: GREEN (all packages pass)

### Known pre-existing flaky tests:
- TestRecording_Goal (TUI timing)
- TestRecording_PromptFlow (TUI timing)
- TestPickAndPlace_MousePick_HoldingItem (PTY mouse timing)
- TestSessionsListAndClean (TempDir cleanup race)
- TestSuperDocument_ViewportUnlocksOnScrollSnapsBackOnTyping (PTY hang)
