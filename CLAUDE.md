# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

`osm` (one-shot-man) is a Go-based CLI tool for constructing reproducible prompts from files, diffs, notes, and templates. It outputs to clipboard for pasting into any LLM UI—no API keys or network required. The tool includes an embedded JavaScript runtime (Goja) for scripting and extensibility.

## Build & Test Commands

Use GNU Make (`gmake` on macOS) for all development operations:

```bash
# Build, lint, and test everything (default)
make

# Build only
make build

# Run tests
make test

# Run all linters (vet, staticcheck, betteralign, deadcode)
make lint

# Format code
make fmt

# View all available targets
make help

```

**Critical Requirement**: ALL checks must pass on ALL platforms (ubuntu-latest, windows-latest, macos-latest). Never accept failing tests—even timing-dependent or "flaky" tests must be properly fixed.

### Running Single Tests

```bash
# Run tests in a specific package
go test ./internal/command/...

# Run a specific test
go test -v ./internal/session/... -run TestSessionLock
```

### Platform-Specific Testing

See `example.config.mk` for additional platform-specific targets:
- `make-all-in-container` - Test Linux behavior from macOS using Docker
- `make-all-run-windows` - Run all targets on Windows via `hack/run-on-windows.sh`

## Architecture

### Entry Point

`cmd/osm/main.go` wires configuration loading, the command registry, goal discovery, and built-in commands.

### Key Directories

- `internal/command/` - Go implementations of CLI commands
- `internal/scripting/` - Embedded JavaScript runtime (Goja) with native bindings
- `internal/storage/` - Session persistence backends (filesystem, memory)
- `internal/session/` - Session management and locking
- `internal/config/` - Configuration handling (dnsmasq-style format)
- `scripts/` - Example JavaScript scripts demonstrating capabilities

### Command Pattern

Most commands (`code-review`, `prompt-flow`, `goal`) execute JavaScript files through the embedded Goja runtime. The built-in commands themselves are implemented as scripts that can be inspected and modified.

### Scripting Globals

The JavaScript environment provides these globals:

- `ctx` / `context` - Context management (files, diffs, git state)
- `output` - Output formatting and clipboard
- `log` - Logging (debug API, subject to change)
- `tui` - Terminal UI integration (TView, Bubble Tea, Lipgloss)

Native modules are available under `osm:` prefix (see `docs/scripting.md`).

### Session Management

- Sessions persist state across workflow boundaries
- Session IDs are auto-determined with locking to prevent corruption
- Two storage backends: `fs` (default) and `memory` (for tests)
- See `docs/session.md` for details

## Configuration

Plain text format (dnsmasq-style) with command-specific sections and environment variable overrides. See `docs/configuration.md` and `docs/reference/config.md`.

## Code Quality Standards

### Linting

The `lint` target runs:

- `go vet` - Static analysis
- `staticcheck` - Strict static analysis with comprehensive checks
- `betteralign` - Struct field alignment optimization
- `deadcode` - Detects unused code (with optional ignore patterns)

**Never add ignores to `.deadcodeignore`**. This defeats the entire purpose of the checker. This project is a CLI, not a library—all implementations MUST be wired up in `main.go` or their respective registries. Ignoring dead code just lets it accumulate. If `deadcode` fails, wire up the code or delete it—don't hide the problem. See `make help` for available check targets.

### Error Handling

- Consistent error handling across all commands with proper exit codes
- No swallowing errors or "best effort" approaches
- All commands must handle platform differences (Unix vs Windows)

### Internal API Discipline

When modifying **internal code**—meaning any code that isn't depended on by external parties (includes all code under `internal/`, unreleased features, experimental code, etc.):

- **No Shims**: Do NOT retain shims, wrappers, or backwards-compatibility stubs "just in case"
- **One Implementation**: Do NOT accumulate variants of the same function (e.g., `Foo()` and `FooV2()`). Choose ONE and migrate all call sites
- **Update Everything**: Update ALL test code and ALL call sites - this includes:
  - Unit tests
  - Integration tests
  - All packages that call the function
  - Any scripts that depend on the behavior
- **Delete Boldly**: Remove deprecated functions entirely. If they're truly needed later, they can be re-added—but accumulated dead code is worse than temporary re-creation

**Lazy is not acceptable**: Keeping old variants "because tests might break" or "because it's easier" creates technical debt. Fix the tests, fix the call sites, delete the old code.

### Testing

- Tests run with race detection
- Coverage reports available via `make cover`
- Platform-specific variations must be tested on all platforms

## Important Conventions

1. **Clipboard-First**: Outputs go to clipboard by design—works in locked-down environments
2. **No API Calls**: Tool is fully local/offline (may be added to SPECIFIC commands and scripts later, but NEVER the default)
3. **Session Locking**: Always use proper session locking to prevent corruption
4. **Platform Compatibility**: Code must work identically on Linux, macOS, and Windows
5. **Script Discovery**: User scripts are auto-discovered from configured paths (experimental UX)

## Documentation

- `docs/README.md` - Documentation index
- `docs/architecture.md` - High-level architecture
- `docs/scripting.md` - JavaScript scripting guide
- `docs/reference/command.md` - Command reference
- `docs/reference/goal.md` - Goal system reference
