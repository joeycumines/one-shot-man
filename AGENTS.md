# AGENTS.md

This file steers agent behavior. For build commands, architecture, and API details, use `gmake help` and the `docs/` directory.

## Code Quality Standards

ALL checks must pass on ALL platforms (ubuntu-latest, windows-latest, macos-latest). Never accept failing tests—fix them, never paper over them. Cross-platform testing targets (`make-all-in-container`, `make-all-run-windows`) must also pass.

### Linting

Never add entries to `.deadcodeignore`. This project is a CLI, not a library—all implementations MUST be wired up in `main.go` or their respective registries. If `deadcode` fails, wire up the code or delete it.

### Error Handling

- Consistent error handling with proper exit codes.
- No swallowing errors or "best effort" approaches.
- All commands must handle platform differences (Unix vs Windows).

### Internal API Discipline

When modifying internal code (anything under `internal/`, unreleased features, experimental code):

- **No Shims**: Do NOT retain shims, wrappers, or backwards-compatibility stubs "just in case".
- **One Implementation**: Do NOT accumulate variants (e.g., `Foo()` and `FooV2()`). Choose ONE and migrate all call sites.
- **Update Everything**: Update ALL test code and ALL call sites—unit tests, integration tests, all packages, any scripts that depend on the behavior.
- **Delete Boldly**: Remove deprecated functions entirely. Accumulated dead code is worse than temporary re-creation.

### Testing

- Tests run with race detection.
- Tests must be isolated. Tests **must not mutate host system state** or depend on session/configuration state outside the test. Zero-tolerance rule.
  - Use `t.TempDir()` for all file operations.
  - Use `t.Setenv()` for environment variable isolation.
  - Mock external services and config sources.
  - Never assume a specific home directory or config location.
- NEVER use build tags to segment tests. Use `testing.Short()` skip guards or `TestMain` with custom `flag` parsing.
- Slow tests (JS runtimes, TUI, subprocesses, >2s) must call `skipSlow(t)` or guard with `if testing.Short() { t.Skip(...) }`.

### No "AI Slop"

Code must be intentional, consistent, purposeful, tested, and validated. Remove contradictory logic, vestigial comments, and unused structures. AI integrations and LLM interfaces are valid; "AI slop" refers to incoherent, inconsistent, purpose-less, untested, or unvalidated code.

## Important Conventions

1. **Clipboard-First**: Outputs go to clipboard by design.
2. **No API Calls**: Fully local/offline by default. Network only in specific commands/scripts, never the default.
3. **Session Locking**: Always use proper session locking.
4. **Platform Compatibility**: Code must work identically on Linux, macOS, and Windows.
5. **Script Discovery**: User scripts are auto-discovered from configured paths (experimental UX).
6. **Avoid Prepositions in Names**: No prepositions (From, Into, To, By, On, In, Of, For) in public APIs. Prefer `LoadConfig` over `LoadFromConfig`. Exception: `With*` option constructors, `ToJSON` matching external API contracts.
7. **Go as Reusable Modules**: Go code must be exposed as reusable, modular implementations accessible via `osm script`.
8. **JS for App-Specific Logic**: All application-specific functionality MUST be modeled as JavaScript.
9. **Structured Logging**: All log calls must use lowercase, punctuation-free, event-phrased messages; never use string concatenation; attach all context as camelCase key-value attributes. Prefer passing `*slog.Logger` explicitly.

## Concurrency Model

The embedded JavaScript runtime (Goja) is single-threaded with a shared event loop. All JS execution happens on one goroutine. The `goja.Runtime` is not goroutine-safe—never touch it from outside the event loop. Bindings that perform I/O offload work to goroutines and resolve Promises back on the event loop via `adapter.Loop().Submit()`.

## JS Binding Contract

JS bindings are guests on the event loop. No binding may monopolize it.

- **Non-blocking I/O**: Any binding that does network, disk, subprocess, clipboard, or any I/O that can wait must return a Promise and run the work off the event loop via `adapter.Loop().Promisify()`. Synchronous file reads/writes, synchronous process execution, and synchronous waits are forbidden in new code.
- **Context propagation**: Capture the base `context.Context` passed to `Require` and thread it into every async goroutine. Do not use `context.Background()` inside a binding—it defeats cancellation. If the underlying Go function accepts `context.Context`, blocking synchronously is truly egregious—always async.
- **Consistent constructors**: Every module's `Require` must accept `context.Context` as its first parameter. Modules needing async support take the event-loop adapter next.
- **No synchronous sleeps**: `time.Sleep` in a binding stalls all JavaScript. Use the event loop's timer/Promise machinery instead.
- **Confirm sync is appropriate**: Even bindings without I/O must be confirmed non-blocking. If a binding does anything beyond pure CPU/memory computation, it must be async.
