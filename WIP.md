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

### Completed this session (session 3):
- **test-commit**: VERIFIED — no tests mutate real repo (all use t.TempDir)
- **T101**: Fix txtar context root formatting (metadata outside code fence)
- **T102**: Fix txtar path ambiguity (verified already fixed in prior sessions)
- **T103**: Fix MCP auto-config (HTTP→stdio command/args format)
- **T104**: Implement osm mcp-instance subcommand (stdio MCP server)
- **T105**: ReadableStream type for fetch Response.body (Go types + JS wrappers)
- **T106**: Promise-based reader.read() (adapter threading, async resolution)
- **T107**: SSE parser for ReadableStream (W3C spec, 16 Go tests)
- **T108**: E2E Goja integration tests (7 JS tests with httptest SSE servers)

### Files created/modified this session:
- `internal/command/mcp_instance.go` — NEW: MCPInstanceCommand
- `internal/command/mcp_instance_test.go` — NEW: 4 tests + stubGoalRegistry
- `cmd/osm/main.go` — Added mcp-instance registration
- `internal/builtin/fetch/readable_stream.go` — NEW: ReadableStream + JS wrappers
- `internal/builtin/fetch/readable_stream_test.go` — NEW: 15 Go-level tests
- `internal/builtin/fetch/sse.go` — NEW: SSEParser + JS wrapper
- `internal/builtin/fetch/sse_test.go` — NEW: 16 Go-level tests
- `internal/builtin/fetch/fetch.go` — Modified: body property, sseReader, adapter threading
- `internal/builtin/fetch/fetch_test.go` — Modified: 12 new tests (5 body + 7 E2E)
- `blueprint.json` — Updated through T108
- `internal/builtin/ctxutil/ctxutil.go` — T101 metadata extraction
- `internal/builtin/ctxutil/ctxutil_test.go` — T101 tests
- `internal/builtin/claudemux/mcp_config.go` — T103 rewrite
- `internal/builtin/claudemux/mcp_config_test.go` — T103 rewrite
- `internal/builtin/claudemux/module.go` — T103 JS bindings
- `internal/builtin/claudemux/instance.go` — T103 state writing

### Build state: GREEN (all 41 packages pass)

### Next task: T109
macOS process hierarchy fallback for session ID — session_darwin.go

### Known pre-existing flaky tests:
- TestRecording_Goal (TUI timing)
- TestRecording_PromptFlow (TUI timing)
- TestPickAndPlace_MousePick_HoldingItem (PTY mouse timing)
- TestSessionsListAndClean (TempDir cleanup race)
- TestSuperDocument_ViewportUnlocksOnScrollSnapsBackOnTyping (PTY hang)
