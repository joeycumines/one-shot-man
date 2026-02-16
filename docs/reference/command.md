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

### `osm log`

View and tail log files.

- Usage: `osm log [tail] [options]`
- Flags:
  - `-n <lines>`: number of lines to show from the end of the file (default `10`)
  - `-f` / `-follow`: follow the log file (like `tail -f`)
  - `-file <path>`: path to log file (overrides config `log.file`)

Subcommands:

- `osm log` — print the last N lines of the log file
- `osm log tail` — alias for `osm log -f`; prints last N lines then follows for new output

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

Configuration keys: `sync.repository` (remote URL), `sync.local-path` (local sync root; default `~/.one-shot-man/sync`), `sync.auto-pull` (auto-pull on startup).

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

## Script commands (discovered)

Any executable file discovered in the configured script paths can appear as `osm <name>`.
On Unix, the executable bit must be set.

See [configuration](../configuration.md) for discovery rules.
