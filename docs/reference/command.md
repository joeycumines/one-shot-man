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

Split a large PR into reviewable stacked branches. Supports heuristic grouping strategies and AI-powered classification via Claude Code or Ollama.

- Usage: `osm pr-split [options]`
- Flags:
  - `-i` / `-interactive`: start interactive TUI mode (default true)
  - `-base <branch>`: base branch to split against (default `main`)
  - `-strategy <name>`: grouping strategy: `directory`, `directory-deep`, `extension`, `chunks`, `auto` (default `directory`)
  - `-max <n>`: maximum files per split (default `10`)
  - `-prefix <prefix>`: branch name prefix for splits (default `split/`)
  - `-verify <command>`: command to verify each split (default `make test`)
  - `-dry-run`: show plan without executing
  - `-ai`: use AI-powered classification and planning (requires provider)
  - `-provider <name>`: AI provider: `ollama`, `claude-code` (default `ollama`)
  - `-model <id>`: model identifier for AI provider
  - `-json`: output results as JSON
  - `-test`: enable test mode
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend

Config keys (in `[pr-split]` section or global):
  - `pr-split.base`, `pr-split.strategy`, `pr-split.max`, `pr-split.prefix`
  - `pr-split.verify`, `pr-split.dry-run`, `pr-split.ai`
  - `pr-split.provider`, `pr-split.model`

Interactive TUI commands:
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
  - `run --ai` — full workflow with AI classification
  - `classify` — classify files with AI
  - `connect` — connect to AI provider registry
  - `disconnect` — disconnect from AI provider
  - `set <key> <val>` — set runtime config
  - `copy` — copy plan to clipboard
  - `report` — output current state as JSON
  - `help` — show available commands

### `osm mcp`

Start an MCP (Model Context Protocol) server over stdio. Exposes osm's context management and prompt building as MCP tools for integration with Claude Desktop, VS Code Copilot, and other MCP-compatible clients.

- Usage: `osm mcp`
- No flags

The server provides 15 tools:

| Tool | Description |
|------|-------------|
| `addFile` | Add a file or directory to the prompt context |
| `addDiff` | Add a unified diff to the prompt context |
| `addNote` | Add a freeform text note to the prompt context |
| `removeFile` | Remove a file or directory from the prompt context |
| `listContext` | List all files, diffs, and notes currently in context |
| `clearContext` | Remove all files, diffs, and notes from the prompt context |
| `buildPrompt` | Build the complete prompt from current context (optionally with a goal) |
| `getGoals` | List all available goals with their descriptions |
| `registerSession` | Register a new agent session with capabilities |
| `reportProgress` | Report progress from an agent session |
| `reportResult` | Report task completion from an agent session |
| `requestGuidance` | Request guidance from the human operator |
| `getSession` | Get session info and drain queued events |
| `listSessions` | List all registered agent sessions |
| `heartbeat` | Update session heartbeat timestamp (keepalive) |

Example MCP client configuration (Claude Desktop `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "osm": {
      "command": "osm",
      "args": ["mcp"]
    }
  }
}
```

#### Session coordination

The MCP session tools enable bidirectional communication between osm and AI agents. An agent registers a session, reports progress/results, and can request human guidance — all through the MCP protocol.

**Session lifecycle:**

1. Agent calls `registerSession` with a unique ID and capabilities list
2. Agent reports work via `reportProgress` (status updates) and `reportResult` (completion)
3. Agent calls `heartbeat` periodically to signal liveness
4. Orchestrator polls via `getSession` (drains queued events) or `listSessions` (summaries)
5. Agent can ask the human operator questions via `requestGuidance`

**Session ID validation:**

- Must be non-empty, max 256 characters
- No control characters (< 0x20) or DEL (0x7F)
- Spaces and printable Unicode are allowed
- Invalid IDs return an error immediately

**Sequence numbers (idempotency):**

The `reportProgress`, `reportResult`, and `requestGuidance` tools accept an optional `seq` field for idempotent operation. This prevents duplicate events when retrying after network failures.

- `seq` = 0 (or omitted): no deduplication, always processed
- `seq` > 0 and > last processed seq: processed, updates the session's `lastSeq`
- `seq` > 0 and ≤ last processed seq: skipped as duplicate, returns `"duplicate seq N (idempotent skip)"`

The `lastSeq` value is visible in `getSession` responses.

**Heartbeat:**

Call `heartbeat` with a `sessionId` to update the session's `lastHeartbeat` timestamp. This allows orchestrators to detect stale/dead agents by comparing `lastHeartbeat` against a timeout threshold.

**Tool schemas:**

`registerSession`:
```json
{
  "sessionId": "agent-1",
  "capabilities": ["code-review", "testing"]
}
```

`reportProgress`:
```json
{
  "sessionId": "agent-1",
  "status": "working",
  "progress": 45.0,
  "message": "Running test suite",
  "seq": 3
}
```
Status must be one of: `working`, `blocked`, `waiting`, `idle`. Progress is clamped to 0–100.

`reportResult`:
```json
{
  "sessionId": "agent-1",
  "success": true,
  "output": "All 42 tests passed",
  "filesChanged": ["internal/foo/bar.go", "internal/foo/bar_test.go"],
  "seq": 4
}
```

`requestGuidance`:
```json
{
  "sessionId": "agent-1",
  "question": "Should I refactor the error handling or keep the current approach?",
  "options": ["refactor", "keep current"],
  "context": "The current approach uses sentinel errors; refactoring would use error wrapping.",
  "seq": 5
}
```

`heartbeat`:
```json
{
  "sessionId": "agent-1"
}
```

`getSession`:
```json
{
  "sessionId": "agent-1"
}
```
Returns session state and **drains** all queued events (progress, result, guidance). Subsequent calls return an empty events array until new events arrive.

Response:
```json
{
  "sessionId": "agent-1",
  "capabilities": ["code-review", "testing"],
  "status": "working",
  "progress": 45.0,
  "lastUpdate": "2025-01-15T10:30:00Z",
  "lastHeartbeat": "2025-01-15T10:29:55Z",
  "lastSeq": 5,
  "events": [
    {
      "type": "progress",
      "timestamp": "2025-01-15T10:30:00Z",
      "data": {"status": "working", "progress": 45.0, "message": "Running test suite"}
    }
  ]
}
```

`listSessions` (no input):
Returns an array of session summaries with `sessionId`, `capabilities`, `status`, `progress`, `lastUpdate`, `lastHeartbeat`, and `eventCount`.

### `osm mcp-instance`

Start a per-instance MCP server over stdio, designed for use by individual Claude Code sessions. Provides the same tool set as `osm mcp`.

- Usage: `osm mcp-instance --session <session-id>`
- Flags:
  - `--session`: Session identifier for this MCP instance (required)

This command is typically invoked by the `claude-mux` orchestrator, not directly by users. Each spawned Claude Code instance gets its own `mcp-instance` server for isolated context management.

### `osm mcp-make`

Start an MCP server exposing GNU Make tools over stdio. Enables MCP clients to execute Makefile targets and query help.

- Usage: `osm mcp-make [--workdir <dir>] [--file <path>]`
- Flags:
  - `--workdir`: Default working directory for make invocations
  - `--file`: Path to Makefile (overrides auto-detection)

Tools:

| Tool | Description |
|------|-------------|
| `make` | Execute a make target with optional workdir and file overrides (5-minute timeout) |
| `make_help` | Display help from the Makefile's `help` target (cached 5 minutes) |

On macOS, prefers `gmake` (GNU Make via Homebrew) over the system `make`.

### `osm mcp-parent`

Start an MCP server for agent steering over stdio. Connects to a `claude-mux` orchestrator via a Unix domain socket and exposes task management tools.

- Usage: `osm mcp-parent --socket <path>`
- Flags:
  - `--socket`: Path to orchestrator control socket (required)

Tools:

| Tool | Description |
|------|-------------|
| `enqueue_task` | Submit a task description to the orchestrator queue |
| `interrupt_current` | Interrupt the currently active task |
| `get_status` | Get orchestrator status (active task, queue depth, queue entries) |

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
  - Initialize the orchestration infrastructure. Creates an instance registry, starts the pool, validates all building blocks (Guard, MCPGuard, Supervisor, ManagedSession) by processing a test event, and reports audit trail. Exits after validation; actual agent spawning requires `osm mcp parent` (planned).

- `osm claude-mux stop`
  - Shut down all managed instances. Currently a placeholder — reports no running instances since agent lifecycle management requires `osm mcp parent`.

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
