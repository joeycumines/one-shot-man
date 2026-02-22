# WIP — Current Session State

## Session
- **Started**: 2026-02-20T23:57:43Z (see .session-timer)
- **Branch**: wip (348+ commits ahead of main)
- **Build**: ALL GREEN — macOS PASS, Linux Docker PASS, Windows PASS
- **Blueprint**: T001-T042 DONE. Next: T043+ (PR splitter review/rewrite)

## Completed Phases

### Phase 1: Ollama HTTP Deletion (T001-T023) — DONE
Deleted `osm:ollama` HTTP client module, `OllamaHTTPProvider`, all traces.
KEPT: `OllamaProvider` (PTY-based), `ollama()` JS factory, `model_nav.go`.

### Phase 2: Cross-Platform Verification (T024-T025) — DONE
- **T024**: Linux Docker via `make make-all-in-container` — GREEN
- **T025**: Windows via `make make-all-run-windows` — GREEN after two fixes:
  1. `splitCommand` deadcode: moved from `pty.go` to `pty_unix.go`
  2. Windows path escaping: `filepath.ToSlash(dir)` in `module_bindings_test.go`

### Phase 3: Full Claudemux Audit (T026-T042) — DONE
Deep audit of all 20+ Go files in claudemux package + claude_mux.go command.

**Bugs Fixed:**
- control.go (C1-C5): Connection deadlines 30s, DialTimeout, socket chmod 0700, scanner buffer 1MB, marshal error handling
- guard.go (G1/G2): timeoutFired flag prevents repeated emission, computeBackoff overflow protection
- mcp_guard.go (M2): noCallTimeoutFired flag prevents repeated emission
- recovery.go (R1): Removed dead maxRetries variable in decideRetryOrEscalate
- session_mgr.go: MCP guard timeout now transitions to SessionFailed
- module.go: Composite validator type assertion uses comma-ok
- panel.go: SetActive on empty panel returns clear error
- claude_mux.go: controlSockPath() returns (string, error) — no longer swallows UserHomeDir error

**Known Pre-existing Issues (documented, not regressions):**
- `stop` subcommand broken (`interruptFn` never wired) — scaffold
- Race conditions in pool.go/panel.go/session_mgr.go — low risk (Goja single-threaded)
- `receive()` in module.go swallows non-EOF errors
- `safety.go` silently drops invalid regex patterns
- `model_nav.go` ANSI regex doesn't match BEL-terminated OSC

## Current Phase: PR Splitter Review (T043+) — STARTING

## Immediate Next Steps
1. **T043**: Review goals/orchestrate-pr-split.json fitness
2. **T044-T049**: Redesign PR split to use claudemux (not deleted Ollama HTTP)
3. **T050-T053**: PTY harness improvements
4. **T054-T060**: Unit + integration tests for PR split
5. **T061-T077**: Coverage push to 100%
6. **T078-T080**: Final cross-platform verification
7. **T081-T128**: Docs, security, fuzz, benchmarks

## Known Issues
- **create_file tool corruption**: Large files get corrupted — write to scratch/, assemble via make target.
- **Auto-commit ghost**: Something creates "test commit" commits periodically.
