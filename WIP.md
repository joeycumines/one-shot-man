# WIP — Current Session State

## Session
- **Started**: 2026-02-20T23:57:43Z (see .session-timer)
- **Branch**: wip (348+ commits ahead of main)
- **Build**: UNTESTED since Ollama HTTP deletion — need `make` run
- **Blueprint**: T001-T025 Ollama deletion phase ~85% complete

## Current Phase: Ollama HTTP Deletion (T001-T025)
Systematically removing the `osm:ollama` HTTP client module, `OllamaHTTPProvider`, and all traces.
**KEEP**: `OllamaProvider` (PTY-based), `ollama()` JS factory in `osm:claudemux`, `model_nav.go`, all PTY tests.

### Files DELETED
- `internal/builtin/ollama/` (entire directory — 10 files)
- `internal/builtin/claudemux/provider_ollama_http.go`
- `internal/builtin/claudemux/provider_ollama_http_test.go`
- `docs/reference/ollama.md`
- `docs/tool-calling.md`

### Files EDITED (Ollama HTTP removed)
- `internal/builtin/register.go` — removed ollamamod import + registration
- `internal/config/config.go` — removed 8 Ollama HTTP struct fields, 4 defaults, 7 switch cases
- `internal/config/schema.go` — removed 15 ollama schema entries
- `internal/config/config_test.go` — removed test data, assertions, 5 subtests
- `internal/command/claude_mux.go` — removed `case "ollama-http"` block
- `internal/builtin/claudemux/pr_split_test.go` — removed 6 Ollama test functions
- `scripts/orchestrate-pr-split.js` — removed ollama import, 5 functions, extractJSON, section header
- `CHANGELOG.md` — removed 11 Ollama HTTP entries, trimmed PR split line
- `docs/README.md` — removed ollama.md and tool-calling.md links

### Files VERIFIED KEPT (PTY — correct)
- `internal/builtin/claudemux/provider_ollama.go` (PTY-based OllamaProvider)
- `internal/builtin/claudemux/provider_ollama_test.go` (12 unit tests)
- `internal/builtin/claudemux/model_nav.go` (TUI navigation)
- `internal/builtin/claudemux/model_nav_test.go` (7 Ollama-named tests — generic TUI)
- `internal/builtin/claudemux/module.go` — ollama() factory creates PTY OllamaProvider
- `internal/builtin/claudemux/module_bindings_test.go` — ollama factory tests KEPT
- `internal/builtin/claudemux/integration_test.go` — all PTY provider refs

## Immediate Next Steps
1. Run `go mod tidy`
2. Run `make` — first build verification
3. Fix any compilation errors
4. Update blueprint.json statuses
5. Continue to T026+ (claudemux audit and refinement)

## Known Issues
- **create_file tool corruption**: Large files get corrupted with reversed content.
  - **Workaround**: Write parts to scratch/, assemble via make target.
- **Auto-commit ghost**: Something creates "test commit" commits periodically.
- **Env var advisory**: OSM_OLLAMA_ENDPOINT and OSM_OLLAMA_MODEL no longer in schema (deleted with HTTP config).
