# WIP — Current Session State

## Session
- **Started**: 2026-02-20T23:57:43Z (see .session-timer)
- **Branch**: wip (322 commits ahead of main)
- **Build**: GREEN
- **Blueprint**: Rewritten 2026-02-21. T101-T118 Done. Proceeding to T200.

## Current Task
- **T200**: Fix TUI regex for Ollama model selection character ▸
- **File**: internal/builtin/claudemux/model_nav.go
- **Issue**: reSelectedArrow regex only matches `>` and `❯`, need to add `▸`, `→`, `►`

## Next Steps After T200
1. T201: PTY command word-splitting (Unix)
2. T202: PTY command word-splitting (Windows)  
3. T203: Implement OllamaProvider
4. T204: Wire into resolveProvider
5. T205-T206: Wire SafetyValidator + MCPInstanceConfig

## Key Files
- blueprint.json — exhaustive task list
- DIRECTIVE.txt — session mandate
- config.mk — custom make targets
- .session-timer — 9-hour timer
