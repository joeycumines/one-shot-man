# AGENTS.md / CLAUDE.md

This file provides guidance to AI agents.

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

# Run fast tests only (skips slow integration/E2E tests via -short flag)
go test -short -count=1 -timeout=5m ./...

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
- `log` - Logging (debug/info/warn/error/printf), backed by `slog`; same semantics, attrs as a plain object
- `tui` - Terminal UI integration (Bubble Tea, Lipgloss)

Native modules are available under `osm:` prefix (see `docs/scripting.md`).

**Log example**: `log.info("user authenticated", { userId: 42, method: "oauth" })`

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
- Tests must be isolated. Tests **must not mutate host system state** or depend on session/configuration state outside the test. This is a **zero-tolerance rule**. Tests that mutate host state cause flaky runs, order-dependent failures, and "works on my machine" issues in CI.
    - Forbidden test behaviors include:
        - Writing to files outside test-specific temporary directories
        - Modifying environment variables that persist beyond the test
        - Reading or depending on user-specific config files (`~/.gitconfig`, shell RC files, etc.)
        - Mutating OS-level session state (ssh-agent, gpg-agent, systemd user sessions)
        - Creating/modifying state in system directories
        - Relying on host-specific paths or configuration
    - Mitigations for these issues include:
        - Use `t.TempDir()` for all file operations in tests
        - Use `t.Setenv()` for environment variable isolation
        - Mock external services and config sources
        - Never assume a specific home directory or config location
        - Tests must be **fully self-contained** and portable across machines
        - Rare exceptions (e.g., `vhs` for recording) MUST use TestMain flags, documented Make targets, and skip gracefully when unavailable. See `generate-tapes-and-gifs` and `-execute-vhs` for the pattern.
- NEVER use build tags to segment tests. Prefer supporting "opting out of long tests" via the `go test -short` flag (`testing.Short()`) or use a `TestMain` with custom `flag` package parsing for to support "opt-in" test behavior (ONLY for exceptional cases).
- **Slow tests MUST use `testing.Short()` skip guards.** Any test that spawns JS runtimes (`scripting.NewEngineWithConfig`), uses bubbletea TUI, spawns subprocesses, or takes >2 seconds must call `skipSlow(t)` (in `internal/command`) or `if testing.Short() { t.Skip(...) }` at the top of the test function. Use `go test -short` for fast feedback.

### No "AI Slop"

When instructed to "clean up AI slop," this **does NOT mean** removal of AI-related code or features. AI integrations, embeddings, LLM interfaces, and similar functionality are valid parts of this codebase.

Instead, "AI slop" refers to code that meets ANY of these criteria:

- **Incoherent**: Code that doesn't read as intentional—contradictory logic, vestigial comments, or structures that serve no discernible function
- **Inconsistent**: Code that violates project conventions, naming patterns, or architectural patterns established elsewhere
- **Purpose-less**: Functions, types, or modules that exist but serve no actual role in the system
- **Untested**: Code paths that are never exercised by tests—either unit tests or integration tests
- **Unvalidated**: Code that has not been demonstrated to work through actual execution paths

The goal is a codebase where every line has a reason to exist, every feature is validated, and the overall system is coherent and maintainable.

## Important Conventions

1. **Clipboard-First**: Outputs go to clipboard by design—works in locked-down environments
2. **No API Calls**: Tool is fully local/offline (may be added to SPECIFIC commands and scripts later, but NEVER the default)
3. **Session Locking**: Always use proper session locking to prevent corruption
4. **Platform Compatibility**: Code must work identically on Linux, macOS, and Windows
5. **Script Discovery**: User scripts are auto-discovered from configured paths (experimental UX)
6. **Avoid Prepositions in Names**: No prepositions (From, Into, To, By, On, In, Of, For, etc.) in method, function, type, or variable names — especially public APIs. Prefer `LoadConfig` over `LoadFromConfig`, `SendEvent` over `SendEventTo`. **Allowed exception**: prepositions used with clear structural intent — e.g. `With*` option constructors convey a specific structural pattern (functional options); `ToJSON` matches an external API contract. The preposition must carry meaning beyond being part of a phrase that happens to be a symbol name.
7. **Go as Reusable Modules**: Go code must be exposed as reusable, modular implementations accessible via `osm script`. Never hardcode application-specific logic—deliver value for other implementations.
8. **JS for App-Specific Logic**: All application-specific functionality MUST be modeled as JavaScript. Don't do anything in Go that someone else couldn't do using OSM.
9. **Structured Logging**: Applies to ALL logging—Go's `log/slog` and the JS `log` API alike. All log calls must: use a lowercase, punctuation-free, event-phrased message; never use string concatenation; attach all context as camelCase key-value attributes (Go: alternating `key, value` pairs; JS: a plain object). Prefer passing `*slog.Logger` explicitly rather than relying on `slog.Default`. Use `log.printf` (JS) only when printf-style formatting is genuinely needed.

## Documentation

- `docs/README.md` - Documentation index
- `docs/architecture.md` - High-level architecture
- `docs/scripting.md` - JavaScript scripting guide
- `docs/reference/command.md` - Command reference
- `docs/reference/goal.md` - Goal system reference
