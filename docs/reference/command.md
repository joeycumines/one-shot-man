# Command reference

This is a *meaning-based* reference (what each command is for), plus the key flags that shape behavior.

Note: `osm help <command>` prints name, description, usage, and any defined flags. For full, verbatim command help (including flag formatting produced by the command), use `osm <command> -h`.

## Top-level commands

### `osm help`

Shows a command list, including discovered script commands.

- Usage: `osm help [command]`

### `osm version`

Prints the build/version string.

- Usage: `osm version`

### `osm init`

Creates the default config file (dnsmasq-style format).

- Usage: `osm init [-force]`
- Flags:
  - `-force`: overwrite existing config

### `osm config`

Manage configuration settings. Read, set, validate, and inspect the configuration schema.

- Usage: `osm config [-all|-global] [key] [value]`
- Flags:
  - `-all`: show global + command sections
  - `-global`: show only global

Subcommands:

- `osm config <key>` — get a configuration value (schema-aware: resolves env var → config → default)
- `osm config <key> <value>` — set a configuration value **persistently** (writes to the config file on disk). The value is validated against the schema before writing; unknown keys produce a warning and invalid values are rejected.
- `osm config validate` — validate the current configuration against the schema and report any issues
- `osm config schema` — print the full configuration schema with all known keys, types, defaults, and descriptions
- `osm config list` — list all configuration values with their sources (`default`, `config`, or `env`), formatted as a table
- `osm config diff` — show only non-default values (overridden via config file or environment variable)

### `osm completion`

Prints shell completion scripts.

- Usage: `osm completion [shell]`
- Shells: `bash` (default), `zsh`, `fish`, `powershell` (alias: `pwsh`)

### `osm goal`

Lists goals or runs a goal. Goals are curated prompt templates/workflows.

- Usage: `osm goal [options] [goal-name]`
- Subcommands:
  - `osm goal paths`: show resolved goal discovery paths with source annotations (`standard`, `custom`, `autodiscovered`) and existence status (`✓`/`✗`). Warns on stderr about missing configured paths.
- Flags:
  - `-l`: list available goals
  - `-c <category>`: list by category
  - `-r <goal-name>`: run directly
  - `-i`: run interactively
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend
  - `-log-level <level>`: log level (`debug`, `info`, `warn`, `error`; default `info`)
  - `-log-file <path>`: path to log file (JSON output)
  - `-log-buffer <n>`: size of in-memory log buffer (default `1000`)

See also: [Goal reference](goal.md)

### `osm script`

Runs JavaScript in the embedded runtime (Goja), with built-in helpers for context management, editor/clipboard integration, and TUI.

- Usage: `osm script [options] [script-file]`
- Subcommands:
  - `osm script paths`: show resolved script discovery paths with source annotations (`standard`, `custom`, `autodiscovered`) and existence status (`✓`/`✗`). Warns on stderr about missing configured paths.
- Flags:
  - `-e <js>` / `-script <js>`: execute inline JavaScript
  - `-i` / `-interactive`: start interactive scripting terminal
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend
  - `-log-level <level>`: log level (`debug`, `info`, `warn`, `error`; default `info`)
  - `-log-file <path>`: path to log file (JSON output)
  - `-log-buffer <n>`: size of in-memory log buffer (default `1000`)

### `osm prompt-flow`

Interactive prompt builder: goal/context/template → meta-prompt → task prompt → final prompt.

- Usage: `osm prompt-flow [options]`
- Flags:
  - `-i` / `-interactive`: start interactive mode (default true; can disable via `-i=false`)
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend
  - `-log-level <level>`: log level (`debug`, `info`, `warn`, `error`; default `info`)
  - `-log-file <path>`: path to log file (JSON output)
  - `-log-buffer <n>`: size of in-memory log buffer (default `1000`)

### `osm code-review`

Interactive “single prompt” code review builder.

- Usage: `osm code-review [options]`
- Flags:
  - `-i` / `-interactive`: start interactive mode (default true; can disable via `-i=false`)
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend
  - `-log-level <level>`: log level (`debug`, `info`, `warn`, `error`; default `info`)
  - `-log-file <path>`: path to log file (JSON output)
  - `-log-buffer <n>`: size of in-memory log buffer (default `1000`)

### `osm super-document`

TUI for merging documents into a single internally consistent super-document.

- Usage: `osm super-document [options]`
- Flags:
  - `-i` / `-interactive`: start interactive TUI mode (default true; can disable via `-i=false`)
  - `-shell`: use shell mode instead of visual TUI
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend
  - `-log-level <level>`: log level (`debug`, `info`, `warn`, `error`; default `info`)
  - `-log-file <path>`: path to log file (JSON output)
  - `-log-buffer <n>`: size of in-memory log buffer (default `1000`)

### `osm pr-split`

Split a large PR into reviewable stacked branches. Supports heuristic grouping strategies and dependency-aware grouping (Go import graph analysis). Output is styled with Lipgloss when available.

- Usage: `osm pr-split [options]`
- Flags:
  - `-i` / `-interactive`: start interactive TUI mode (default true)
  - `-base <branch>`: base branch to split against (default `main`)
  - `-strategy <name>`: grouping strategy: `directory`, `directory-deep`, `extension`, `chunks`, `dependency`, `auto` (default `directory`)
  - `-max <n>`: maximum files per split (default `10`)
  - `-prefix <prefix>`: branch name prefix for splits (default `split/`)
  - `-verify <command>`: command to verify each split (default `make test`)
  - `-dry-run`: show plan without executing
  - `-json`: output results as JSON
  - `-test`: enable test mode
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend
  - `-log-level <level>`: log level (`debug`, `info`, `warn`, `error`; default `info`)
  - `-log-file <path>`: path to log file (JSON output)
  - `-log-buffer <n>`: size of in-memory log buffer (default `1000`)
  - `-claude-command <path>`: Claude binary path (empty = auto-detect `claude` → `ollama`)
  - `-claude-arg <arg>`: additional Claude CLI argument (repeatable, e.g. `-claude-arg --verbose -claude-arg --no-color`)
  - `-claude-model <model>`: model name (provider-dependent)
  - `-claude-config-dir <dir>`: Claude config directory override
  - `-claude-env <vars>`: extra environment variables (`KEY=VALUE,KEY=VALUE`)

Config keys (in `[pr-split]` section or global):
  - `pr-split.base`, `pr-split.strategy`, `pr-split.max`, `pr-split.prefix`
  - `pr-split.verify`, `pr-split.dry-run`
  - `pr-split.claude-command`, `pr-split.claude-arg`, `pr-split.claude-model`
  - `pr-split.claude-config-dir`, `pr-split.claude-env`

#### Grouping strategies

| Strategy | Description |
|----------|-------------|
| `directory` | Group by top-level directory (default) |
| `directory-deep` | Group by full directory path |
| `extension` | Group by file extension |
| `chunks` | Split into equal-sized chunks |
| `dependency` | Parse Go import graph and merge packages that import each other within the changeset. Falls back to `directory` for non-Go projects. |
| `auto` | Automatically selects best strategy based on file count and project structure |

#### Interactive TUI commands

Workflow commands:
  - `analyze [base]` — analyze diff between current and base branch
  - `stats` — show addition/deletion counts per file
  - `group [strategy]` — group files by strategy
  - `plan` — create split plan from groups
  - `preview` — show detailed plan preview
  - `execute` — execute the split (create branches)
  - `verify` — run verify command on each branch
  - `equivalence` — check tree hash equivalence
  - `cleanup` — delete all split branches
  - `run` — full workflow: analyze → group → plan → execute → verify
  - `auto-split` — automated pipeline: spawn Claude → classify → plan → execute → verify → resolve (falls back to heuristic mode if Claude unavailable)

Plan editing commands:
  - `move <file> <from-index> <to-index>` — move a file between splits (1-based indexes)
  - `rename <index> <new-name>` — rename a split (1-based index)
  - `merge <index-a> <index-b>` — merge split B into split A (1-based indexes)
  - `reorder <index> <new-position>` — change split execution order (1-based)

Plan persistence:
  - `save-plan [path]` — save current plan to JSON file (default `.pr-split-plan.json`)
  - `load-plan [path]` — restore plan from saved JSON file

GitHub integration:
  - `create-prs [--draft] [--push-only]` — push branches and create stacked GitHub PRs via `gh` CLI
  - `fix` — auto-resolve common split conflicts (go mod tidy, go.sum regeneration)

General:
  - `set <key> <val>` — set runtime config (keys: `base`, `strategy`, `max`, `prefix`, `verify`, `dry-run`, `retry-budget`, `mode`)
  - `copy` — copy plan to clipboard
  - `report` — output current state as JSON
  - `help` — show available commands

#### Usage examples

**Heuristic mode** (default, no Claude):
```
$ osm pr-split -i --base main --strategy directory
> run
```

**Automated mode** (with Claude Code):
```
$ osm pr-split -i --base main --claude-command claude
> auto-split
```

**Mixed mode** — start automated, then refine manually:
```
$ osm pr-split -i --base main --claude-command claude
> auto-split
> preview
> move internal/util.go 3 1
> execute
```

#### Troubleshooting

- **"Claude unavailable"** — ensure `claude` (or `--claude-command`) is on PATH.
  Auto-split falls back to heuristic mode automatically.
- **Tree hash mismatch** — a file rename's old path wasn't deleted from the split
  branch. Use `fix` to attempt auto-repair, or manually adjust with `move`.
- **Retry budget exhausted** — increase with `set retry-budget 5` before `auto-split`,
  or use `fix` on individual splits after execution.

### `osm log`

View and tail log files.

- Usage: `osm log [tail|follow] [options]`
- Flags:
  - `-n <lines>`: number of lines to show from the end of the file (default `10`)
  - `-f` / `-follow`: follow the log file (like `tail -f`)
  - `-file <path>`: path to log file (overrides config `log.file`)

Subcommands:

- `osm log` — print the last N lines of the log file
- `osm log tail` — alias for `osm log -f`; prints last N lines then follows for new output
- `osm log follow` — alias for `osm log -f`; same as `osm log tail`

The log file path is resolved from: `-file` flag → config key `log.file` → env var `OSM_LOG_FILE`. Follows log rotation automatically (detects file truncation/replacement).

### `osm sync`

Save, list, and load prompt notebook entries; sync via git.

- Usage: `osm sync <save|list|load|init|push|pull> [options]`

Subcommands:

- `osm sync save -title <title> -body <body> [-tags <tags>]`
  - Save a prompt notebook entry as a Markdown file with YAML frontmatter.
  - Flags:
    - `-title`: entry title (required; used in filename slug)
    - `-body`: prompt body text (required)
    - `-tags`: comma-separated tags
  - Files are written to `<sync-root>/notebooks/<YYYY>/<MM>/<date>-<slug>.md`.

- `osm sync list [-limit <n>]`
  - List saved notebook entries in reverse chronological order.
  - Flags:
    - `-limit`: maximum number of entries to show (0 = all)

- `osm sync load <slug-or-date>`
  - Load a saved notebook entry and output its body (YAML frontmatter stripped). The query can be a full date-slug (`2025-01-15-my-review`), slug only (`my-review`), date only (`2025-01-15`), or partial slug (`review`). When multiple entries match by slug, the most recent is returned.

- `osm sync init [<repo-url>]`
  - Clone a git repository as the sync root. The repository URL can be passed as an argument or read from the `sync.repository` config key.

- `osm sync push`
  - Stage all changes, commit with a timestamp message, and push to origin.

- `osm sync pull`
  - Fetch and rebase remote changes. If the sync directory is not initialized and `sync.repository` is configured, clones automatically. Reports merge conflicts with instructions to resolve.

Configuration keys: `sync.repository` (remote URL), `sync.local-path` (local sync root; default `~/.osm/sync`), `sync.auto-pull` (auto-pull on startup).

### `osm session`

Session lifecycle and inspection tools.

Top-level:
- Usage: `osm session [-dry-run] [-y] [list|clean|purge|delete|info|path|id]`
- Flags:
  - `-dry-run`: do not delete; show what would be deleted
  - `-y`: assume yes for confirmation

Subcommands:

- `osm session id [-session <id>]`
  - resolves and prints the session id for the current terminal
- `osm session list [-format text|json] [-sort default|active]`
  - lists sessions with metadata
- `osm session clean [-dry-run] [-y]`
  - policy-based cleanup (asks for confirmation unless `-y` or `-dry-run`)
- `osm session purge [-dry-run] [-y]`
  - aggressive cleanup (ignores retention policies)
- `osm session delete [-dry-run] [-y] <session-id>...`
  - deletes explicit sessions
- `osm session info <session-id>`
  - prints raw session JSON
- `osm session path [session-id]`
  - prints sessions directory, or a specific session file path

### `osm claude-mux`

Multi-instance Claude Code orchestration. Manages pools of Claude Code instances with guard rails, MCP integration, error recovery, and audit logging.

- Usage: `osm claude-mux <subcommand> [options]`
- Flags:
  - `-pool-size <n>`: maximum concurrent Claude instances (default `4`)

Subcommands:

- `osm claude-mux status`
  - Show current configuration and system health. Displays pool size, guard rail settings (rate-limit, permission policy, crash handling, output timeout), MCP guard settings (frequency limit, repeat detection, no-call timeout, tool allowlist), supervisor settings (max retries), and the fail-closed security policy.

- `osm claude-mux start`
  - Initialize the orchestration infrastructure. Creates an instance registry, starts the pool, validates all building blocks (Guard, MCPGuard, Supervisor, ManagedSession) by processing a test event, and reports audit trail.

- `osm claude-mux stop`
  - Shut down all managed instances. Currently a placeholder — reports no running instances.

- `osm claude-mux submit <task description>`
  - Submit a task for processing. Validates the task description is non-empty. Task queuing requires a running orchestrator.

Infrastructure wired by `start`:

| Component | Source | Purpose |
|-----------|--------|---------|
| InstanceRegistry | T007 | Isolated state directories per instance |
| Pool | T011 | Concurrent instance management with acquire/release |
| Guard | T008 | PTY output monitors (rate-limit, permission, crash, timeout) |
| MCPGuard | T009 | MCP call monitors (frequency, repeat, allowlist, timeout) |
| Supervisor | T010 | Error recovery state machine (retry, restart, escalate) |
| ManagedSession | T014 | Unified monitoring pipeline composing all guards |
| Safety | T015 | Intent/scope/risk classification and policy enforcement |
| ChoiceResolver | T016 | Multi-criteria decision analysis for strategy selection |

## Script commands (discovered)

Any executable file discovered in the configured script paths can appear as `osm <name>`.
On Unix, the executable bit must be set.

See [configuration](../configuration.md) for discovery rules.
