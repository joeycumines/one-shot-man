# WIP — Ollama Rewrite + PR-Split AI Slop Removal

## Status: IN PROGRESS — Build verification needed

### What changed (UNCOMMITTED)

#### OllamaProvider Rewrite
- `provider_ollama.go` — runs `ollama launch claude` with `--model` flag; ExtraArgs replaces SubArgs; MCP=true
- `provider_ollama_test.go` — 12 tests updated for new behavior
- `module.go` — `ollama(opts?)` factory: subArgs→extraArgs, added model option
- `module_bindings_test.go` — subArgs→extraArgs, MCP assertion fix
- `claude_mux.go` — resolveProvider sets Model on OllamaProvider

#### PR-Split AI Slop Removal
- `pr_split.go` — removed aiMode/provider/model fields and flags
- `pr_split_script.js` — removed claudemux require, registry, AI functions, BT nodes, AI exports
- `pr_split_test.go` — deleted 4 AI tests, removed AI flag/field assertions
- `completion_command.go` — stripped --ai/--provider/--model from completions
- `docs/reference/command.md` — removed AI flags and ollama from pr-split
- `docs/reference/config.md` — removed ai/provider/model keys
- `CHANGELOG.md` — updated OllamaProvider, pr-split entries

### Next Steps
1. Build verification
2. Rule of Two
3. Commit
