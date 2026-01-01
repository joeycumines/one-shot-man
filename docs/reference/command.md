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

Displays loaded configuration (global and command-specific).

- Usage: `osm config [-all|-global] [key] [value]`
- Flags:
  - `-all`: show global + command sections
  - `-global`: show only global

Important: `osm config <key> <value>` sets config *in-memory for that run*; it does not write back to the config file.

### `osm completion`

Prints shell completion scripts.

- Usage: `osm completion [shell]`
- Shells: `bash` (default), `zsh`, `fish`, `powershell` (alias: `pwsh`)

### `osm goal`

Lists goals or runs a goal. Goals are curated prompt templates/workflows.

- Usage: `osm goal [options] [goal-name]`
- Flags:
  - `-l`: list available goals
  - `-c <category>`: list by category
  - `-r <goal-name>`: run directly
  - `-i`: run interactively
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend

See also: [Goal reference](goal.md)

### `osm script`

Runs JavaScript in the embedded runtime (Goja), with built-in helpers for context management, editor/clipboard integration, and TUI.

- Usage: `osm script [options] [script-file]`
- Flags:
  - `-e <js>` / `-script <js>`: execute inline JavaScript
  - `-i` / `-interactive`: start interactive scripting terminal
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend

### `osm prompt-flow`

Interactive prompt builder: goal/context/template → meta-prompt → task prompt → final prompt.

- Usage: `osm prompt-flow [options]`
- Flags:
  - `-i` / `-interactive`: start interactive mode (default true; can disable via `-i=false`)
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend

### `osm code-review`

Interactive “single prompt” code review builder.

- Usage: `osm code-review [options]`
- Flags:
  - `-i` / `-interactive`: start interactive mode (default true; can disable via `-i=false`)
  - `-test`: enable test mode / verbose output
  - `-session <id>`: override session id
  - `-store <fs|memory>`: select storage backend

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
