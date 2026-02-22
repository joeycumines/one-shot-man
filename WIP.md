# WIP — Current Session State

## Session
- **Started**: 2026-02-20T23:57:43Z (see .session-timer)
- **Branch**: wip (348+ commits ahead of main)
- **Build**: ALL GREEN — macOS PASS, Linux Docker PASS, Windows PASS
- **Blueprint**: T001-T025 DONE. Next: T026+ (claudemux audit)

## Completed Phases

### Phase 1: Ollama HTTP Deletion (T001-T023) — DONE
Deleted `osm:ollama` HTTP client module, `OllamaHTTPProvider`, all traces.
KEPT: `OllamaProvider` (PTY-based), `ollama()` JS factory, `model_nav.go`.

### Phase 2: Cross-Platform Verification (T024-T025) — DONE
- **T024**: Linux Docker via `make make-all-in-container` — GREEN
- **T025**: Windows via `make make-all-run-windows` — GREEN after two fixes:
  1. `splitCommand` deadcode: moved from `pty.go` to `pty_unix.go` (Windows never calls it)
  2. Windows path escaping: `filepath.ToSlash(dir)` in `module_bindings_test.go` line 191

### Cross-Platform Bug Fixes Applied
- `internal/builtin/pty/pty.go` — removed `splitCommand` (moved to pty_unix.go)
- `internal/builtin/pty/pty_unix.go` — received `splitCommand` + `"strings"` import
- `internal/builtin/pty/pty_splitcmd_test.go` — NEW file (`//go:build !windows`) with 18 test cases
- `internal/builtin/pty/pty_test.go` — removed `TestSplitCommand` (moved to pty_splitcmd_test.go)
- `internal/builtin/claudemux/module_bindings_test.go` — added `"path/filepath"` import, `filepath.ToSlash(dir)`

## Current Phase: Claudemux Audit (T026-T042) — STARTING
Audit every exported type, interface, function in claudemux package.

## Immediate Next Steps
1. **T026**: Audit claudemux API surface for fitness post-ollama removal
2. **T027**: Review ClaudeCodeProvider implementation
3. **T028**: Review parser.go for Claude Code patterns
4. Continue through T042 (full claudemux package review)

## Known Issues
- **create_file tool corruption**: Large files get corrupted — write to scratch/, assemble via make target.
- **Auto-commit ghost**: Something creates "test commit" commits periodically.
