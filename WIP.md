# WIP — Takumi Session State

## Quick Resume Checklist
1. Read blueprint.json → find first "Not Started" task → execute it
2. Run `make check-session-time` to verify session timer
3. Run `make find-test-failures` if in doubt about state
4. NEVER stop. NEVER declare "done". EXPAND and ITERATE.

---

## Session Identity
- **Branch**: wip (251 commits ahead of main at session start; grows as we commit)
- **Session timer file**: `.session-timer`
- **Session start**: 2026-02-18T14:49:00Z
- **Session end**: 2026-02-18T23:49:00Z (9 hours)
- **Timer check**: `make check-session-time`

---

## Current State (2026-02-18)

### Completed this session:
- **P001**: macOS baseline passes (`make make-all-with-log` green)
- **P002**: Session timer initialized (`.session-timer` = 2026-02-18T14:49:00Z)
- **P003**: WIP.md created (this file)
- **Blueprint rewritten**: Complete architectural planning phase done. blueprint.json rewritten with flat, sequential P001–P042 task list, no priorities, no estimates.

### Immediately next (P004):
Run Linux baseline:
```
make make-all-in-container
```
Then check: `make find-test-failures`

### After that (P005):
Run Windows baseline:
```
make make-all-run-windows
```

### Current blueprint path:
`/Users/joeyc/dev/one-shot-man/blueprint.json` — P001-P003 Done, P004-P042 Not Started.

---

## Key Files for Cold Start

| File | Purpose |
|------|---------|
| `blueprint.json` | THE sacred task tracker — read first |
| `WIP.md` | This file — session state |
| `.session-timer` | 9-hour session start timestamp |
| `config.mk` | Custom make targets (make check-session-time, make find-test-failures, etc.) |
| `build.log` | Output of make-all-with-log (gitignored) |
| `internal/builtin/orchestrator/` | Orchestrator package — needs rename to claudemux (P009) |
| `docs/architecture-ai-orchestrator.md` | Needs rewrite to architecture-claude-mux.md (P008) |
| `scripts/bt-templates/orchestrator.js` | Needs rename to claude-mux.js (P010) |

---

## Architecture Context

### The "orchestrator" → "claudemux" rename
The word "orchestrator" is RETIRED. All code/docs must use "claudemux" (Go) or "claude-mux" (user-facing) or "osm:claudemux" (JS module).

Current state of the rename (NOT YET DONE, pending P009):
- Package: `internal/builtin/orchestrator/` — 7 Go files
- JS module name: `osm:orchestrator` (registered in register.go as `orchestratormod`)
- JS template: `scripts/bt-templates/orchestrator.js` (uses `require('osm:orchestrator')`)
- Tests: templates_test.go, parser_test.go, pr_split_test.go, provider_test.go

### What P009 requires (rename):
1. `mv internal/builtin/orchestrator internal/builtin/claudemux`
2. Change `package orchestrator` → `package claudemux` in all 7 Go files
3. Update `register.go`: import path + `osm:orchestrator` → `osm:claudemux`
4. Update `register_test.go`: module name string
5. Update `templates_test.go`, `parser_test.go`, `pr_split_test.go`, `provider_test.go`: package name
6. Update `module.go` comment
7. Update `scripts/bt-templates/orchestrator.js`: `require('osm:orchestrator')` → `require('osm:claudemux')`
8. Run make

### What P010 requires (template rename):
1. `mv scripts/bt-templates/orchestrator.js scripts/bt-templates/claude-mux.js`
2. Update `templatePath()` in templates_test.go (now in claudemux pkg): `orchestrator.js` → `claude-mux.js`
3. Update scripts/README.md
4. Update header comment in the file

---

## Test Suite Notes
- All tests currently pass on macOS (fresh run confirms)
- Linux/Windows not yet verified this session (P004/P005)
- Tests in orchestrator package: templates_test.go skips on Windows (uses sh -c)
- Tests in orchestrator package: pr_split_test.go skips on Windows (uses sh -c)
- No .DS_Store files tracked in git (confirmed by `make check-ds-store`)

---

## Task Dependencies Quick Reference
```
P008 (architecture doc) → P009 (rename) → P010 (template rename) → P011 (parser ext)
P013 (MCP feedback) → P014 (dynamic MCP config) → P015 (session isolation)
P015 + P016 (guard rails) → P017 (error recovery)
P015 + P018 (instance mgmt) → P024 (main entry point)
P024 → P025 (integration tests)
```

---

## Hostile Reviewer Notes
(Things to check carefully before committing any batch)
- `make deadcode` — ensure no dead code lurks after renames
- `make betteralign` — struct alignment
- `make staticcheck` — catching subtle issues
- `go test -race ./...` — data race detection
- All three platforms must pass
