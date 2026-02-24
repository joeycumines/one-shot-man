# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **OllamaProvider**: `Provider` implementation for Ollama LLM backend — `Name()="ollama"`, `Capabilities={MCP:false, Streaming:true, MultiTurn:true, ModelNav:true}`, model selection via TUI navigation (not `--model` flag); configurable `Command` and `SubArgs`; 12 unit tests in `provider_ollama_test.go`
- `--safety` flag on `osm claude-mux run` wiring `SafetyValidator` into the `dispatchTask` output loop — PolicyBlock aborts task, PolicyConfirm treated as block (no interactive user in automated pipeline), PolicyWarn logs to stderr; default disabled
- MCPInstanceConfig auto-injection in `dispatchTask`: after `registry.Create()`, generates per-instance `.claude.json` for providers with `Capabilities().MCP`; appends `SpawnArgs()` to a defensive copy of `SpawnOpts`; defers `Close()` for cleanup; graceful fallback if config creation fails
- `ollama(opts?)` factory function in `osm:claudemux` JavaScript module with configurable `command` and `subArgs`
- `ModelNav` field in `ProviderCapabilities` indicating providers that require TUI-based model navigation post-spawn
- PTY command word-splitting: `splitCommand()` in `pty.go` with full POSIX shell quoting support (single quotes, double quotes, backslash escapes) — `Spawn()` automatically splits `cfg.Command` when `cfg.Args` is empty and the command contains spaces; 18+ unit tests
- **Claude-mux orchestration system**: multi-instance Claude Code management framework with building blocks for PTY output parsing, guard rails, MCP monitoring, error recovery, concurrent instance pooling, TUI multiplexing, safety validation, and choice resolution
- `osm claude-mux` command with `status`, `start`, `stop`, `submit` subcommands for lifecycle management, pool sizing (`-pool-size`), audit logging, and fail-closed security policy
- PTY output parser (`parser.go`): pattern-based classifier for Claude Code output — rate limits, permission prompts, tool calls, errors, model selection, cost updates, and text; extensible via `Parser.Patterns()`
- Guard rails — PTY monitors (`guard.go`): `Guard` pipeline with `GuardConfig` for rate-limit detection, permission policy (deny/allow), crash restart limits, and output timeout; emits `GuardEvent` actions (pause, restart, escalate, timeout)
- Guard rails — MCP monitors (`mcp_guard.go`): `MCPGuard` for tool call frequency limiting, repeat detection, no-call timeout, and tool allowlist enforcement
- Error recovery and cancellation (`recovery.go`): `Supervisor` state machine with retry→restart→escalate→abort flow, per-error-class strategies (PTY crash, MCP timeout, cancellation), context propagation, and graceful drain/shutdown
- Concurrent instance management (`pool.go`): `Pool` with acquire/release dispatch, round-robin scheduling, `sync.Cond` blocking, health tracking, `Drain`/`WaitDrained`/`Close` lifecycle
- TUI multiplexing (`panel.go`): `Panel` with Alt+1..9 pane switching, per-pane scrollback, PgUp/PgDown navigation, health indicators, and status bar
- Session isolation (`instance.go`): `InstanceRegistry` with per-instance state directories, `state.json` persistence, and `sync.Map`-based concurrent management
- Dynamic MCP config per instance (`mcp_config.go`): auto-port Unix socket/TCP listeners, session-scoped config JSON generation, and endpoint management
- MCP session coordination hardening: session ID validation (empty, >256 chars, control chars), sequence number deduplication, heartbeat tracking, 20+ new tests, and fuzz coverage
- Safety validation (`safety.go`): intent classification (read-only, code, destructive, credential, network), scope assessment (file, repo, infra, unknown), risk scoring (0.0–1.0), policy actions (allow, confirm, block, deny), composable `Validator` interface with `CompositeValidator`, `SafetyConfig` with blocked paths, sensitive patterns, and per-intent thresholds
- Choice resolution (`choice.go`): `ChoiceResolver` with `Candidate`/`Criterion`/`ChoiceConfig`, weighted scoring via `ScoreFunc`, ranked results with justification, and confirmation threshold
- Managed session compositor (`session_mgr.go`): `ManagedSession` composing Parser+Guard+MCPGuard+Supervisor into a unified pipeline with callbacks (`OnEvent`, `OnGuardAction`, `OnRecoveryDecision`) and thread-safe `Snapshot()`
- `osm:claudemux` JavaScript module: full JS bindings for all building blocks (parser, guard, MCP guard, supervisor, pool, panel, instance registry, safety, choice resolver, managed session) with `SESSION_IDLE`/`SESSION_ACTIVE`/`SESSION_PAUSED`/`SESSION_FAILED`/`SESSION_CLOSED` constants
- PR split rewrite: `orchestrate-pr-split.js` v3.0.0 with claudemux integration (selectStrategy+ChoiceResolver, conflict classification, equivalence verification with diff, createSelectStrategyNode BT leaf)
- **`osm pr-split` built-in command**: consolidated PR splitting from external scripts into a first-class embedded command (`internal/command/pr_split.go`) — 12 CLI flags (`--base`, `--strategy`, `--max`, `--prefix`, `--verify`, `--dry-run`, `--ai`, `--provider`, `--model`, `--interactive`, `--test`, `--json`); embedded JS runtime (v5.0.0) with all heuristic grouping strategies, AI classification via claudemux, BT workflow trees, PA-BT planning integration with full backchaining preconditions/effects, stacked branch creation, tree-hash equivalence verification, and interactive TUI mode; replaces `scripts/orchestrate-pr-split.js`, `scripts/bt-templates/claude-mux.js`, and `goals/orchestrate-pr-split.json`
- `osm pr-split` AI-assisted classification: `--ai` flag wires `classifyChangesWithClaudeMux` into the `run` workflow — creates provider registry, classifies changes, suggests split plan; falls back to heuristic grouping on failure; `classify` TUI command for manual AI classification; `connect`/`disconnect` TUI commands for provider lifecycle management
- `osm pr-split` elapsed time tracking: per-step timing `(Xms)` after each workflow step and total workflow duration at completion
- `osm pr-split` `[pr-split]` config section: 9 config keys (`base`, `strategy`, `max`, `prefix`, `verify`, `dry-run`, `ai`, `provider`, `model`) with flag override; documented in `docs/reference/config.md`
- `osm pr-split` shell completion for flags and strategy values in bash, zsh, and fish
- `osm pr-split --json` flag and `report` TUI command: structured JSON output including version, config, analysis, groups, plan, execution results, and equivalence verification
- `osm pr-split` documentation: full command reference in `docs/reference/command.md` (12 flags, 17 TUI commands, 9 config keys, quickstart examples)
- Composite BT workflow functions for pr-split: `spawnAndPrompt` (3-step: spawn→send→wait), `verifyAndCommit` (tests→verify→commit ordering), `spawnPromptAndReadResult` (positional API variant), `createPlanningActions` (7 PA-BT actions with preconditions and effects for planner backchaining)
- Behavioral tests for composite BT functions: config-object API verification, step ordering tests, PA-BT backchaining with goal planning, default commit message verification
- Shell completion for `claude-mux` subcommands (status/start/stop/submit) in bash, zsh, fish, and PowerShell
- Claude-mux documentation: `docs/reference/claude-mux.md` (full API reference), `docs/architecture-claude-mux.md` (11-section architecture doc), updates to `command.md`, `scripting.md`, and `README.md`
- Fuzz tests for claude-mux: `FuzzParseOutput`, `FuzzGuardRuleEval`, `FuzzMCPPayload`, `FuzzSafetyClassify` in `fuzz_test.go`
- Performance benchmarks for claude-mux: 8 benchmarks in `benchmark_test.go` covering parser, guard, MCP guard, safety, pool, managed session, panel, and choice resolver (all with `b.ReportAllocs()`)
- Security tests for MCP protocol: 20 tests across `claudemux/mcp_security_test.go` (guard injection, tool injection, privilege escalation, blocked paths, allowlist, disabled safety, sensitive patterns, concurrent guard, session isolation, instance registry IDs, frequency burst, repeat detection, composite validator) and `command/mcp_security_test.go` (session spoofing, ID validation, sequence replay, large payloads, concurrent manipulation, tool name injection, session overwrite)
- Integration testing infrastructure: TestMain with `-integration`, `-provider`, `-model` flags; 6 live agent tests (disabled by default); 4 simulated CI tests (full pipeline lifecycle, concurrent multi-session, error recovery escalation, safety-into-pipeline); `make integration-test-claudemux` target
- AbortSignal support in `osm:fetch`: `fetch(url, { signal })` option wires `AbortController.signal` to HTTP request cancellation — supports pre-aborted signals (immediate rejection), mid-request abort via `ac.abort()`, and `AbortSignal.timeout(ms)` for automatic deadline-based cancellation
- `osm:protobuf` native module: Protocol Buffers for goja via [goja-protobuf](https://github.com/joeycumines/goja-protobuf) — `loadDescriptorSet(bytes)` loads binary `FileDescriptorSet` for use with `osm:grpc` client/server operations
- `EventLoopProvider.Adapter()` method exposing the goja-eventloop adapter to native modules that need Promise integration (required by goja-grpc)
- Example JSON goal files in `goals/examples/` demonstrating all goal schema features: minimal, stateVars, hotSnippets, flagDefs, and full-featured — with a README explaining each example and how to use them
- Multiline input support for go-prompt: `multiline` option on `tui.createPrompt()` and `tui.registerMode()` — when enabled, Alt+Enter inserts a newline into the prompt buffer while Enter still submits normally; the prompt JS object also exposes a `newLine()` method for programmatic newline insertion from key-binding handlers
- `writeFile(path, content, options?)` and `appendFile(path, content, options?)` functions in the `osm:os` module: write or append content to files from JavaScript scripts, with optional `mode` (default `0644`) and `createDirs` (default `false`) options; errors are thrown as JavaScript exceptions
- `osm config list` subcommand: displays all configuration values with their effective sources (`default`, `config`, or `env`) in a formatted table
- `osm config diff` subcommand: shows only non-default configuration values (overridden via config file or environment variable)
- `ResolveAll` and `ResolveDiff` methods on `ConfigSchema` for programmatic access to resolved configuration with source tracking
- `ConfigSource`, `ResolvedOption` types in config package for structured source metadata
- `osm mcp` command: MCP (Model Context Protocol) server mode over stdio transport with 15 tools — `addFile`, `addDiff`, `addNote`, `removeFile`, `listContext`, `clearContext`, `buildPrompt`, `getGoals` (context management), `registerSession`, `reportProgress`, `reportResult`, `requestGuidance`, `getSession`, `listSessions`, `heartbeat` (session coordination) — enabling integration with Claude Desktop, VS Code Copilot, and other MCP clients
- `osm:json` native module: JSON utilities — `parse`, `stringify`, `query` (dot-notation/array-indexing/wildcard path queries), `mergePatch` (RFC 7386), `diff` (JSON Pointer paths), `flatten`/`unflatten` (nested↔flat conversion)
- `osm:crypto` native module: cryptographic hash functions wrapping Go's `crypto` package — `sha256`, `sha1`, `md5`, `hmacSHA256`, `hmacSHA1` — all return hex-encoded lowercase strings; input accepts strings or byte arrays
- `osm:path` native module: cross-platform path manipulation wrapping Go's `path/filepath` — `join`, `dir`, `base`, `ext`, `abs`, `rel`, `clean`, `isAbs`, `match`, `glob`, `separator`, `listSeparator`
- `osm:regexp` native module: Go RE2 regular expressions — `match`, `find`, `findAll`, `findSubmatch`, `findAllSubmatch`, `replace`, `replaceAll`, `split`, `compile` (returns `RegexpObject` with bound methods); invalid patterns throw JS errors
- `osm:encoding` native module: base64 and hex encoding/decoding — `base64Encode`, `base64Decode`, `base64URLEncode` (URL-safe, no padding), `base64URLDecode`, `hexEncode`, `hexDecode`; decode errors throw JS errors; input accepts strings or byte arrays
- `osm config reset <key>` subcommand: reset a single configuration key to its schema default, removing it from both in-memory config and the config file on disk
- `osm config reset --all --force` subcommand: reset all global configuration keys to their schema defaults; comments, section headers, and command-specific options are preserved; `--force` is required as a safety measure
- `DeleteKeyInFile` and `DeleteAllGlobalKeysInFile` functions in config package for removing global keys from the config file while preserving comments and sections
- Example script `example-07-flag-parsing.js`: demonstrates `osm:flag` argument parsing (typed flags, defaults, lookup, visit/visitAll, positional args)
- Example script `example-06-api-client.js`: demonstrates `osm:fetch` HTTP client API (GET, POST with JSON, streaming, error handling, timeouts, response headers)
- `osm log follow` subcommand as alias for `osm log tail` / `osm log -f` (continuous log tailing)
- Expanded `log` JavaScript API documentation: all 8 methods documented with parameter types, log destination details (in-memory ring buffer + JSON file rotation), and cross-reference to `osm log` command
- Warning log for unknown arg completer types in REPL completion (aids debugging custom goal definitions)
- `osm goal paths` subcommand: displays all resolved goal discovery paths with source annotations (`standard`/`custom`/`autodiscovered`), existence status (`✓`/`✗`), and config validation warnings for missing custom paths
- `osm script paths` subcommand: displays all resolved script discovery paths with the same annotated format
- `AnnotatedPath` type in discovery subsystem with `Path`, `Source`, and `Exists` fields
- Shell completion for `paths` subcommand in `osm goal` and `osm script` (bash, zsh, fish, powershell)
- Shell completion for `help` subcommand: `osm help <TAB>` now suggests available command names in bash, zsh, fish, and PowerShell
- Built-in goal `pii-scrubber` (category: data-privacy): redacts personally identifiable information from code, logs, and data with three redaction levels (strict/moderate/minimal) and deterministic placeholder mapping
- Built-in goal `prose-polisher` (category: writing): 7-step copyediting pipeline (structural review, clarity, consistency, concision, correctness, tone alignment, final polish) with four target styles (technical/casual/academic/marketing) and `hot-expand-section` snippet
- Built-in goal `data-to-json` (category: data-transformation): extracts structured JSON from unstructured text, logs, or data with four extraction modes (auto/tabular/log/document) and JSON Schema output
- Built-in goal `cite-sources` (category: research): generates answers with numbered inline citations from provided source material, with three citation formats (numbered/author-date/footnote) and `hot-challenge-claims` snippet
- Built-in goal `which-one-is-better` (category: decision-making): exhaustive comparative analysis of options, designs, or approaches with five comparison types (general/technology/architecture/strategy/design), weighted scoring matrices, confidence-rated recommendations, and `hot-deeper-analysis` / `hot-devils-advocate` follow-up snippets
- `osm sync load <slug-or-date>` subcommand: read saved notebook entries by exact date-slug, slug only, date only, or partial slug match, with YAML frontmatter stripped
- Auto-pull on startup: when `sync.auto-pull=true` and the sync repository is initialized, runs `git pull --rebase` before goal/script discovery
- Automatic sync discovery path injection: if the sync repo contains `goals/` or `scripts/` subdirectories, those paths are injected into `goal.paths`, `script.paths`, and `script.module-paths` for automatic discovery
- Summarized parameters in goal list: `osm goal -l` displays `[vars: key=val, ...]` suffix for non-nil state variable defaults
- Strict argument validation for all commands and subcommands with clear error messages
- Hot-snippets: `GoalHotSnippet` struct with `hot-` prefix convention and config-to-goal merge
- `scriptCommandBase` extracting shared `RegisterFlags()` and `PrepareEngine()` across 5 commands, eliminating ~120 lines of boilerplate
- Auto-generate meta-prompt on first copy via `autoGenerateOnCopy` state flag in prompt-flow
- Prompt-flow one-step mode: `copy` and `show` work without `goal`/`generate`, outputting raw context when no goal is set
- `PromptFooter` field in Goal struct with template variable interpolation, appended after generated meta-prompt in prompt-flow and goal CLI
- Schema-aware config validation via `ValidateOptionValue` before `config set`
- `computePathLCA` for txtar path disambiguation with context metadata in `ToTxtar` output
- PATH-based executable completion via `getExecutableSuggestions`
- Git ref completion with remote branches and recent commits
- `CommandFlagDef` struct and `FlagDefs` field for REPL flag completion
- Atomic `GetLifecycleSnapshot()` for behavior tree lifecycle state queries
- Fuzz tests for config parser, diff splitter, buildContext, and Goja runtime (zero panics across 2.4M+ executions); additionally FuzzGoalJSONParsing, FuzzSanitizeFilename, FuzzSanitizePayload, FuzzMCPBacktickFence, FuzzComputePathLCA covering goal loading, filesystem safety, session payload, Markdown fence, and path LCA correctness
- Security test suites: 34 input sanitization tests and 18 JS sandbox boundary tests
- `docs/security.md` documenting JavaScript sandbox boundaries and threat model
- Performance benchmarks across engine creation, filesystem, PA-BT planning, bubbletea, MCP tool latency/prompt building/backtick fencing, sanitizeFilename, computePathLCA, sanitizePayload, and 8 additional categories (90+ benchmark scenarios total)
- Test coverage expanded across 25+ packages with notable gains: bubblezone 0→98.7%, lipgloss 58→99%, tview 68.5→96.4%, bubbletea 75.8→91.2%, viewport 73.3→97.3%, overall cmd/osm 91.4→94.8%
- `tui_commands.go` `registerBuiltinCommands` coverage 88.9%→97.2%: added `mode` success path and `reset` stateManager-nil error path tests; remaining 2.8% is an unreachable defensive `else` branch
- `osm pr-split` dependency-aware grouping: `dependency` strategy parses Go import graph (go.mod module path + source file imports) and merges packages that import each other within the changeset using union-find; falls back to `directory` grouping for non-Go projects; added to `-strategy` flag, shell completions (bash/zsh/fish), and `auto` strategy selection
- `osm pr-split` plan persistence: `save-plan [path]` and `load-plan [path]` TUI commands serialize/restore plan, analysis, execution state, and metadata to/from JSON (default `.pr-split-plan.json`); uses `osm:os` `writeFile`/`readFile`
- `osm pr-split` GitHub PR creation: `create-prs [--draft] [--push-only]` TUI command pushes branches to remote and creates stacked GitHub PRs via `gh pr create` — each PR targets the previous split's branch for clean stacking; supports `--draft` for draft PRs and `--push-only` to skip PR creation
- `osm pr-split` conflict resolution: `fix` TUI command auto-resolves common split conflicts using sequential repair strategies — `go mod tidy` to fix module dependencies and `go mod download` to regenerate go.sum; re-verifies after each fix; reports fixed vs unresolved splits
- `osm pr-split` interactive plan editing: `move <file> <from-index> <to-index>`, `rename <index> <new-name>`, `merge <index-a> <index-b>`, `reorder <index> <position>` TUI commands for in-place plan modification
- `osm pr-split` Lipgloss styled output: all status messages use styled text when `osm:lipgloss` is available — success (green), error (red), warning (amber), info (blue), dim (gray), bold, and progress bar helpers; graceful degradation to plain text when Lipgloss is not loaded

### Changed
- **BREAKING:** Renamed internal "orchestrator" package to `claudemux` (Go) / `claude-mux` (user-facing) / `osm:claudemux` (JS module) — all imports, docs, and CLI references updated
- Consolidated two shell-out `git pull --rebase` call sites (`sync.go executePull`, `sync_startup.go SyncAutoPull`) into `gitops.PullRebase()` with `PullRebaseOptions` struct and `ErrConflict` sentinel — properly captures stderr, validates directory, and supports custom git binary path
- **BREAKING:** `osm:fetch` module reworked to browser Fetch API compliance — `fetch(url, opts?)` now returns `Promise<Response>` (async) instead of synchronous Response; Response.headers is now a proper Headers object with `.get()`, `.has()`, `.entries()`, `.keys()`, `.values()`, `.forEach()` methods; `.text()` and `.json()` now return Promises; HTTP requests run in goroutines with Promise resolution on the event loop
- **BREAKING:** Replaced `osm:grpc` synchronous API with Promise-based gRPC via [goja-grpc](https://github.com/joeycumines/goja-grpc) — `dial`/`loadDescriptorSet`/`invoke` replaced by `createClient`/`createServer`/`dial`/`status`/`metadata`/`enableReflection`/`createReflectionClient`; all RPC calls now return Promises supporting unary, server-streaming, client-streaming, and bidirectional streaming; protobuf descriptor loading moved to new `osm:protobuf` module (`loadDescriptorSet`); uses in-process gRPC channel (`go-inprocgrpc`) for zero-network-overhead internal communication
- Migrated JavaScript event loop from `dop251/goja_nodejs/eventloop` to `joeycumines/go-eventloop` + `joeycumines/goja-eventloop` — enables proper Promise/setTimeout/setInterval integration via adapter pattern; adds AbortController, TextEncoder/Decoder, URL, crypto, and process.nextTick as JS globals; console.log/warn/error/info/debug provided via goja_nodejs/console module with adapter-provided timer methods (console.time/timeEnd/timeLog)
- `osm:argv` `formatArgv` now applies POSIX shell quoting for arguments containing special characters (spaces, quotes, backslashes, glob chars, pipes, semicolons); arguments without special characters are passed through unquoted
- Migrated textarea `runeWidth` from `go-runewidth` to `uniseg.StringWidth` for correct Unicode grapheme cluster width — combining marks and control characters now correctly report zero width instead of being clamped to 1; extracted shared `hitTestColumn` helper eliminating 3× code duplication across `performHitTest`, `handleClick`, and `handleClickAtScreenCoords`
- Renamed `osm:nextIntegerId` native module to `osm:nextIntegerID` (Go naming convention); the old name is kept as a deprecated alias
- All user-visible strings updated from "one-shot-man" to "osm" — help text, version output, `osm init` messages, generated config file header, shell completion script comments, and temp directory prefixes now consistently use "osm"
- Default configuration directory migrated from `~/.one-shot-man/` to `~/.osm/` — existing `~/.one-shot-man/config` files are still read as a fallback if `~/.osm/` does not exist; new installations use `~/.osm/` by default
- Session storage directory migrated from `{UserConfigDir}/one-shot-man/sessions/` to `{UserConfigDir}/osm/sessions/`
- Upgraded `charmbracelet/bubbles` dependency from v0.21.1 to v1.0.0 (honorary release, zero API changes)
- Stabilized `log` JavaScript API: removed \"undercooked\" label from scripting.md, updated CLAUDE.md to list all methods
- Renamed `pabt.ModuleLoader` to `pabt.Require` for API consistency
- Moved `CONFIG_HOT_SNIPPETS` auto-detection into `contextManager.js` reducing per-script boilerplate
- Unexported 14 internal symbols across scripting, command, storage, and builtin packages
- Refactored txtar collision handling to use full relative paths instead of filename-only deduplication

### Deprecated
- `osm:nextIntegerId` module name: use `osm:nextIntegerID` instead (old name still works as an alias)

### Removed
- `scripts/orchestrate-pr-split.js` — consolidated into `osm pr-split` built-in command (`internal/command/pr_split_script.js`)
- `scripts/bt-templates/claude-mux.js` — BT template functions merged into `osm pr-split` embedded script
- `goals/orchestrate-pr-split.json` — goal definition replaced by `osm pr-split` CLI flags and embedded configuration
- `fetchStream()` from `osm:fetch` module — replaced by Promise-based `fetch()` which reads the full response body; streaming use cases should use standard async patterns with `await resp.text()`
- Old synchronous `osm:fetch` implementation — `fetch()` was synchronous (blocking the event loop), now runs HTTP requests in goroutines with Promise-based resolution
- Old synchronous `osm:grpc` implementation using raw `google.golang.org/grpc` — replaced entirely by goja-grpc thin wrapper with Promise-based API
- Direct dependency on `dop251/goja_nodejs/eventloop` — replaced by `joeycumines/go-eventloop` + `joeycumines/goja-eventloop` adapter
- Unused `sync.enabled` configuration key (was defined but never read)
- `osm:tview` native module and entire `internal/builtin/tview/` package (~2,100 lines): superseded by `osm:bubbletea`
- `TViewManagerProvider` interface and `GetTViewManager()` method from scripting engine
- `rivo/tview` and `gdamore/tcell/v2` Go module dependencies
- Deprecated `tui.createAdvancedPrompt` alias; use `tui.createPrompt` instead
- Deprecated `GetStateViaJS`/`SetStateViaJS` aliases from scripting API
- `ContextCommand` interface from command package
- `BTBridge` alias from builtin package
- 3 unused benchmark threshold constants
- Stale `internal/termtest/*` entries from `.deadcodeignore`
- 4 TODO comments in tui_completion.go: documented completion logic, resolved outdated arg completer precedence note, removed speculative types, added unknown-type warning
- Deprecated `ScrollWheel` and `ScrollWheelOnElement` string-based methods from mouseharness; use type-safe `ScrollWheelWithDirection` and `ScrollWheelOnElementWithDirection` instead

### Fixed
- `osm pr-split` TUI command dispatch error swallowing: `ErrCommandNotFound` sentinel error with `errors.Is` branching — handler errors now display to user instead of silently falling through to JS dispatch; panic recovery via named return + defer/recover in `executeCommand()`
- `osm pr-split` `analyzeDiff()` uses `--name-status` instead of `--name-only`: returns `fileStatuses` map (file → A/M/D/R/C/T); rename/copy track new path only; unmerged paths rejected with clear error; unknown status codes rejected via whitelist
- `osm pr-split` deleted file handling in `executeSplit`: `fileStatuses` is REQUIRED (no silent fallback); status `D` → `git rm --ignore-unmatch -f`; missing entries produce explicit error naming the file
- `osm pr-split` pre-existing branches: pre-flight loop deletes existing split branches before recreation, enabling `re-run` without manual cleanup
- `osm pr-split` run handler gracefully handles 0 changed files with a clear message instead of proceeding to empty plan
- Flaky `TestClaudeMux_Run_Integration_PoolConcurrency` on Linux: concurrency counter was tracked via a goroutine watching `agent.done`, but goroutine scheduling races allowed the pool to spawn the next agent before the decrement ran — replaced with `concurrencyTrackingAgent` wrapper that decrements in `Close()`, the actual synchronization point used by the pool
- `TestExecAndExecv` ETXTBSY on Docker overlayfs: added directory `fsync` after write-then-rename to flush metadata before `exec` — the canonical POSIX pattern for ensuring rename durability
- TUI model selection regex (`reSelectedArrow`) only matched `>` (ASCII) and `❯` (U+276F) — added `▸` (U+25B8 Ollama), `►` (U+25BA), `→` (U+2192) for cross-provider compatibility; 4 new test cases
- SafetyValidator `PolicyConfirm` assessment silently fell through in automated `dispatchTask` pipeline — now treated as block with distinct error message (`"safety blocked (would require confirmation)"`)
- Cross-platform safety validator: `filepath.Clean` on Windows converts `/etc/hosts` to `\etc\hosts` — added `filepath.ToSlash` normalization so system path detection works correctly on all platforms
- Bash completion formatting: `;;` case terminators for `schema)` and `log)` were concatenated on the same line as the next case pattern — split to separate lines
- Zsh completion `commands` array scoping: array was declared inside the `commands)` case branch, making it inaccessible to the `args)` branch where `help)` needs it — hoisted to function scope
- Data race in storage path globals: added `sync.RWMutex` guarding `getSessionDirectory` and `getSessionLockFilePath` accessor functions in `paths.go`, preventing concurrent read/write of function-variable overrides during cleanup scheduling
- `ScanSessions` incorrectly accepted non-session `.json` files (e.g. `notes.json`, `config.json`) — the filter used `filepath.Ext` (`.json`) then subtracted `.session.json` length, which could produce wrong session IDs or panic for short filenames; now uses `strings.HasSuffix(name, ".session.json")` with length-based slicing for base extraction
- Inconsistent `fmt.Fprint*` error handling: added `_, _ =` prefix to all unchecked calls across session, completion, scripting, terminal, and bubbletea source files for project-wide consistency
- Silently swallowed errors during log file rotation: `RotatingFileWriter.rotate()` now logs backup shift, rename, and cleanup failures to stderr instead of discarding them
- Flaky `TestSuperDocument_BacktabNavigation` PTY integration test: standardized inter-keystroke delay from inconsistent 4–20ms to a uniform 25ms constant (`ptyCharDelay`) across all character-typing loops in both PTY test files; under CPU load the previous delays caused the TUI to coalesce or drop keystrokes, producing garbled output
- macOS PTY data loss: slave fd is now kept alive in parent process until child exits, preventing buffered output from being lost on macOS when the slave fd closes before the master reads; also fixed `EvalSymlinks` for macOS `/var` → `/private/var` resolution
- VHS recording path remapping: replaced hardcoded `../../../` prefix with dynamic `filepath.Rel` computation from tape output directory to repository root; argument quoting now uses VHS-compatible `quoteVHSString` instead of Go-style `fmt.Sprintf("%q")`
- Data race in scripting engine: `context.AfterFunc` closure reading `engine.vm` while `Close()` sets nil; captured VM in local variable before closure
- Data race in bubbletea module via `syscall.Dup` file descriptor handling
- Context refresh failing for paths with trailing slashes or `./` prefixes: `RefreshPath` now normalizes input via `AddPath`'s pipeline
- TOCTOU race in mouseharness `ClickElement`: captures buffer once instead of three separate `cp.String()` calls
- 2 flaky behavior tree tests via atomic `GetLifecycleSnapshot()` replacing separate state queries
- Nil-dereference in ctxutil `Require` when `exportsVal` is Go nil
- 3 nil/undefined guard bugs in bubbletea module
- 2 nil-guard bugs for `mouseMsg` in bubblezone `inBounds`
- Nil guard in tview `Require` function
- 6 nil-guard and return-type bugs across template, unicodetext, and nextintegerid packages
- 3 error-wrapping format verbs corrected from `%v` to `%w` for proper error chain support
- Permission-based tests failing under Docker root: tests skip when running as UID 0
- Concurrent session archival race: `ArchiveSession` now uses mutex and checks if source file still exists before rename, preventing double-archive panics when multiple goroutines archive simultaneously
- `ETXTBSY` error on overlayfs (Docker): `exec` module uses atomic write-then-rename pattern instead of in-place modification for script execution on copy-on-write filesystems
- `ScanSessions` on Windows: added explicit directory check before `ReadDir`, preventing silent no-op on non-directory paths
- 2 Windows test failures: echo builtin and tview Console API tests skip on unsupported platforms
- `sanitizeFilename` compiled 3 regexes (`regexp.MustCompile`) on every call — hoisted to package-level vars for single compilation at init time
- Error message consistency: lowercased error string in `pabt/state.go` with `pabt:` prefix; added `gitops:` prefix to `ErrNotRepo`, `ErrNothingToCommit`, `ErrConflict` sentinel errors
- 5 documentation inaccuracies: MCP tool count 8→14 in `docs/reference/command.md`; session config key format (kebab-case→camelCase) in `docs/session.md`; stale event loop reference in `docs/architecture.md`; wrong config path (`~/.config/osm`→`~/.osm`) in `docs/scripting.md`; stale TView reference in `AGENTS.md`
- `slog.Handler` contract violation in `tuiLogHandler`: `WithAttrs`/`WithGroup` returned the same handler instead of a new instance — extracted shared state into `tuiLogHandlerShared` struct so each derived handler carries its own `preAttrs`/`groupPrefix` while sharing entries, mutex, and level
- `context.AfterFunc` stop handle leak in `bt/bridge.go`: missing capture of stop function caused GC to collect the AfterFunc registration prematurely — stored `stopParentCtx` field in bridge struct
- `deduplicatePath` in sync.go silently overwrote existing file on path name exhaustion — now returns `(string, error)` and propagates exhaustion as an explicit error to the caller
- `matchEntry` in sync.go mutated the caller's `[]fs.DirEntry` slice during sorting — now copies the slice via `make`+`copy` before `slices.SortFunc`
- `goalNameRE` regex recompiled on every `resolveGoalScript` call — hoisted to package-level `var` for single compilation at init time
- Flaky `FuzzMCPSessionTools`: fuzz iterations had no per-iteration timeout and blocking server cleanup, causing hangs when the fuzz engine's `-fuzztime` expired mid-iteration — added 10s `context.WithTimeout` and non-blocking `select` on server shutdown channel

## [v0.1.0] - 2026-02-10

### Added
- PA-BT (Planning-Augmented Behavior Trees) module for autonomous agent behaviors with planning capabilities
- `NewAction`, `NewActionGenerator`, `NewBlackboard`, `NewExprCondition` APIs for behavior tree planning
- `scripts/example-05-pick-and-place.js` demonstrating PA-BT for pick-and-place tasks
- `QueueGetGlobal(name, callback)` for thread-safe asynchronous global reads from scripting engine
- PA-BT documentation: API reference, demo script guide, blackboard usage guide
- Edge case test suites for commands, sessions, and platform-specific scenarios
- Performance benchmarks and regression tests
- Security test suite: 42 subtests covering path traversal, command injection, env sanitization, permissions, input validation, session isolation, and output sanitization

### Fixed
- Race condition in scripting engine: `GetGlobal()` now uses full `Lock()` for synchronization with `QueueSetGlobal()`
- Symlink vulnerability in config loading: `os.Lstat()` check rejects symlinks before opening config files

### Security
- Config file loading rejects symlinks to prevent path traversal attacks
