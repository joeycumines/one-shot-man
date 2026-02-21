# WIP — Current Session State

## Session
- **Started**: 2026-02-20T23:57:43Z (see .session-timer)
- **Branch**: wip (322 commits ahead of main)
- **Build**: GREEN
- **Blueprint**: Rewritten 2026-02-21. T101-T118 Done. Proceeding to T200.

## Completed
- **T200**: Done. Regex fix + 4 tests. Build GREEN.

## Current Task
- **T201**: Fix PTY command word-splitting (Unix)
- **File**: internal/builtin/pty/pty_unix.go
- **Issue**: When cfg.Command has spaces and cfg.Args is empty, need shlex-style splitting

## Next Steps After T201
1. T202: PTY command word-splitting (Windows)
2. T203: Implement OllamaProvider
3. T204: Wire into resolveProvider
4. T205-T206: Wire SafetyValidator + MCPInstanceConfig

## Key Files
- blueprint.json — exhaustive task list
- DIRECTIVE.txt — session mandate
- config.mk — custom make targets
- .session-timer — 9-hour timer
