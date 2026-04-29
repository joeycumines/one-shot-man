# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **`osm pr-split` command**: automated PR splitting with full pipeline — diff analysis, heuristic/dependency-aware grouping (directory, extension, pattern, chunks, dependency), split plan creation, stacked branch building, tree-hash equivalence verification, and optional GitHub PR creation. Includes interactive TUI mode with 30+ REPL commands (analyze, plan, execute, verify, fix, edit-plan, auto-split, etc.), auto-split pipeline with Claude Code integration via MCP callbacks, two-stage cancellation, pause/resume with checkpoints, per-branch verification TUI with streaming output, conflict auto-resolution (go-mod-tidy, go-generate-sum, go-build-missing-imports, npm-install, make-generate, add-missing-files, claude-fix), Lipgloss-styled output, comprehensive logging, and pipeline/step/idle timeouts. Configuration via `[pr-split]` config section or CLI flags (`--base`, `--strategy`, `--max`, `--prefix`, `--verify`, `--dry-run`, `--interactive`, `--test`, `--json`, `--timeout`). Shell completions for bash, zsh, and fish.
- **`osm:claudemux` JavaScript module**: building blocks for Claude Code process management — PTY output parser with pattern-based classification, guard rails (PTY monitors for rate-limiting/permissions/crashes, MCP monitors for tool frequency/allowlists), error recovery supervisor with retry/restart/escalate/abort flow, concurrent instance pool with acquire/release dispatch, TUI panel with pane switching and scrollback, session isolation with per-instance state, safety validation with intent classification and risk scoring, choice resolution with weighted scoring, and managed session compositor. All components exposed as JS-callable bindings. Used by `osm pr-split` via `require('osm:claudemux')`.
- PTY command word-splitting: `splitCommand()` in `pty.go` with full POSIX shell quoting support (single quotes, double quotes, backslash escapes) — `Spawn()` automatically splits `cfg.Command` when `cfg.Args` is empty and the command contains spaces; 18+ unit tests
- AbortSignal support in `osm:fetch`: `fetch(url, { signal })` option wires `AbortController.signal` to HTTP request cancellation — supports pre-aborted signals (immediate rejection), mid-request abort via `ac.abort()`, and `AbortSignal.timeout(ms)` for automatic deadline-based cancellation
- `osm:protobuf` native module: Protocol Buffers for goja via [goja-protobuf](https://github.com/joeycumines/goja-protobuf) — `loadDescriptorSet(bytes)` loads binary `FileDescriptorSet` for use with `osm:grpc` client/server operations
- `EventLoopProvider.Adapter()` method exposing the goja-eventloop adapter to native modules that need Promise integration (required by goja-grpc)
- Example JSON goal files in `goals/` demonstrating all goal schema features: minimal, stateVars, hotSnippets, flagDefs, and full-featured — with a README explaining each example and how to use them
- Multiline input support for go-prompt: `multiline` option on `tui.createPrompt()` and `tui.registerMode()` — when enabled, Alt+Enter inserts a newline into the prompt buffer while Enter still submits normally; the prompt JS object also exposes a `newLine()` method for programmatic newline insertion from key-binding handlers
- `writeFile(path, content, options?)` and `appendFile(path, content, options?)` functions in the `osm:os` module: write or append content to files from JavaScript scripts, with optional `mode` (default `0644`) and `createDirs` (default `false`) options; errors are thrown as JavaScript exceptions
- `osm config list` subcommand: displays all configuration values with their effective sources (`default`, `config`, or `env`) in a formatted table
- `osm config diff` subcommand: shows only non-default configuration values (overridden via config file or environment variable)
- `ResolveAll` and `ResolveDiff` methods on `ConfigSchema` for programmatic access to resolved configuration with source tracking
- `ConfigSource`, `ResolvedOption` types in config package for structured source metadata
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
- `osm pr-split` PR Splitting section in README.md: feature overview with 6 strategies, 6 key flags, interactive/automated mode descriptions, and code snippets
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
- `osm pr-split` pipeline unit tests: 8 test functions covering `AUTOMATED_DEFAULTS` constants, PTY send config, conflict resolution heuristics, cancellation error paths, transient error classification, `waitForLogged` guards, and backoff behaviour; all pass with `-race`
- `osm pr-split` TUI command unit tests: 8 test functions covering core/extension command building, HUD enablement, activity info branching, last-output-lines extraction, and HUD status line rendering with truncation; all pass with `-race`
- `osm pr-split` TUI rendering unit tests: 12 test functions for layout mode selection, string truncation/padding/repeat, color/constant structure validation, spinner frames, progress bar rendering, and `resolveColor`/`renderProgressBar`; all pass with `-race`
- `osm pr-split` TUI overlay and state dispatcher unit tests: 8 test functions for `computeReportOverlayDims` (4 screen sizes), `syncReportScrollbar` (mock viewport + 3 noop variants), `CHROME_ESTIMATE` constant, and `viewForState` dispatcher (15 wizard states + unknown fallback = 16 subtests); all pass with `-race`
- `osm pr-split` focus navigation unit tests: 15 test functions with 13 subtests covering `getFocusElements` across all 7+ wizard states (CONFIG, PLAN_REVIEW, PLAN_EDITOR, FINALIZATION, PAUSED, EQUIV_CHECK, ERROR_RESOLUTION, IDLE, BASELINE_FAIL), `syncSplitSelection` card-index extraction, `handleNavDown`/`handleNavUp` wrap-around, and `handleListNav` clamping with editor guards; all pass with `-race`
- `osm pr-split` dialog handler unit tests: 14 test functions covering `updateRenameDialog` (branch name validation against `INVALID_BRANCH_CHARS`, `..` and `.lock` rejection, typing+backspace), `updateMoveDialog` (nav clamping, confirm splice, single-split auto-close), `updateMergeDialog` (toggle+confirm with descending splice, no-selection no-op, cursor clamping), and state dispatcher (esc-close, unknown no-op); all pass with `-race`
- `osm pr-split` split-pane unit tests: 12 test functions covering `_computeSplitPaneContentOffset` (7 layout scenarios) and `_writeMouseToPane` (5 scenarios: 3 tab success, no-session guard, write-throws error); all pass with `-race`
- `osm pr-split` report formatter unit tests: 10 test functions (15 with subtests) covering `_formatReportForDisplay` — falsy inputs, empty report defaults, metadata population, analysis sections, group rendering, plan splits, equivalence states, and full report section ordering; all pass with `-race`
- `osm pr-split` chrome pane renderer unit tests: 13 test functions (18 with subtests) covering `_renderClaudeQuestionPrompt` (falsy/active/inactive/truncation/convo-count), `_renderShellPane` (placeholder/content/focus/path-truncation/narrow), and `_renderOutputPane` (scroll-offset/focus-structural/narrow); all pass with `-race`
- `osm pr-split` sub-renderer unit tests: 13 test functions covering `_renderVerificationStatusList` (8 status states including skipped/passed/failed/expanded/overflow/active/pending/pre-existing) and `_renderLiveVerifyViewport` (5 viewport states including auto/manual-scroll/paused/fallback); all pass with `-race`
- `osm pr-split` confirm-cancel and conversation handler unit tests: 21 test functions covering `_updateConfirmCancel` (9: Tab/ShiftTab cycling, Enter on Yes/No, y confirms, n/esc dismisses, mouse zones, focus auto-init, session cleanup), `_closeClaudeConvo` (history preservation), `_updateClaudeConvo` (5: typing+editing, scroll with floor clamp, submit, sending blocks input, mouse consumption), `_pollClaudeConvo` (3: sending/plan-revised/idle), and `_openClaudeConvo` (3: no executor, dead handle, live handle); all pass with `-race`
- `docs/architecture-pr-split-chunks.md` expanded with 133-export function inventory across 13 per-chunk tables (chunks 13–16f) with 7 testability classifications: pure (14), quasi-pure (6), lipgloss (36), stateful (51), async (8), constant (10), object (8)
- `tui_commands.go` `registerBuiltinCommands` coverage 88.9%→97.2%: added `mode` success path and `reset` stateManager-nil error path tests; remaining 2.8% is an unreachable defensive `else` branch
- `osm:exec` streaming subprocess API: `exec.spawn(cmd, args, opts)` starts a subprocess and returns a `ChildProcess` handle with `pid`, `kill()`, and `wait()` method; pull-based stdout/stderr via `ReadableStream.read()` returning `{value, done}` Promises; pump goroutines per pipe with bounded channels; cross-platform (Unix `Setpgid`+process group kill, Windows `os.Process.Kill`); configurable write timeout; 17 Go-level unit tests with `-race`
- `osm:exec` `execStream(cmd, args, opts)` synchronous streaming helper: line-by-line stdout/stderr callbacks (`onStdout(line)`, `onStderr(line)`) with exit code capture; used by `verifySplits` for real-time build output in TUI
- PTY write timeout: `DefaultWriteTimeout` (30s) prevents indefinite blocking if child process hangs; configurable via `SpawnConfig.WriteTimeout` and JS `writeTimeoutMs` option; negative value disables
- Terminal mux flicker-free panel toggle: `VTerm.RenderFullScreen()` emits CUP+content+EL per row instead of ESC[2J (erase display); eliminates flash-to-black on panel toggle
- `osm pr-split` async analysis pipeline: `analyzeDiffAsync`, `groupByDependencyAsync`, `selectStrategyAsync`, `applyStrategyAsync` run on the goja event loop — the TUI remains responsive during diff analysis and plan generation; progress steps UI (verify baseline → analyze diff → group files → generate plan → validate plan) with per-step active/done indicators
- `osm pr-split` PTY-based branch verification: interactive `verifySplit` sessions via `termmux.CaptureSession` — ANSI output captured in real-time, streamed to a dedicated TUI viewport with scrollback; `screen()` for ANSI-safe rendering; configurable verify command per plan; pre-existing failure detection via baseline comparison
- `osm pr-split` equivalence check screen: tree-hash comparison with visual diff, re-verify and revise plan buttons, keyboard navigation (Tab, Enter, j/k)
- `osm pr-split` async PR creation: `createPRsAsync` + `ghExecAsync` pipeline with per-branch progress display; dry-run mode simulates PR creation without real `gh` calls; skipped-PR display with `○` icons
- `osm pr-split` pause/resume: `PAUSED` wizard state with dedicated screen, checkpoint save/restore via `savePlan()`, cancel/force-cancel cleanup
- `osm pr-split` cancel overlay: two-stage cancellation with Tab focus cycling, contextual text for active verify sessions, `SIGINT`/kill escalation for PTY sessions
- `osm pr-split` analysis timeout with user-visible slow-analysis warning and configurable threshold (`prSplitConfig.analysisTimeoutMs`, default 60s)
- `osm pr-split` nav-cancel keyboard accessibility: Cancel button reachable via Tab in all 7 wizard screens; Enter on focused Cancel mirrors mouse click behavior (verify interrupt or cancel confirmation)
- `osm pr-split` branch name validation: `INVALID_BRANCH_CHARS` regex shared between `validatePlan()` and `validateSplitPlan()`; rename dialog rejects invalid characters with inline error display; also validates `..` and `.lock` suffix
- `osm pr-split` saveCheckpoint persistence: `WizardState.saveCheckpoint()` now calls `savePlan()` to persist all runtime caches (analysis, groups, plan, execution results, conversation history) to disk for crash recovery
- `osm pr-split` verification phase state machine: `verifyPhases` enum (NOT_STARTED, RUNNING, PAUSED, EQUIV_CHECK, COMPLETE, FAILED, ERROR) with `transitionVerifyPhase()` validated transition function and `resetVerifyPhase()` unconditional reset, integrated across 6 handler files — tracks the verification subsystem lifecycle independently from the wizard's high-level state; `branchStatuses` enum (PENDING, ACTIVE, PASSED, FAILED, SKIPPED) added to all `verificationResults.push()` call sites; 11 new unit tests covering constants, transitions, resets, terminal states, and 3 lifecycle paths

### Changed
- **`osm pr-split` TUI redesigned from go-prompt REPL to full BubbleTea graphical wizard**: 7-screen flow (Configure → Analysis → Plan Review → Plan Editor → Execution → Verification → Finalization) with mouse support via bubblezone, responsive layout, Lipgloss-styled UI chrome (title bar, nav bar, status bar, step dots, progress bars), overlay system (help, confirm-cancel, error resolution with auto-resolve/manual-fix/skip/retry/abort options), Claude terminal toggle via termmux integration, and async pipeline handlers for analysis and execution stages; the previous 30+ text-based REPL commands are retained for programmatic/test dispatch alongside the new graphical interface
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
- Stabilized `log` JavaScript API: removed "undercooked" label from scripting.md, updated CLAUDE.md to list all methods
- Renamed `pabt.ModuleLoader` to `pabt.Require` for API consistency
- Moved `CONFIG_HOT_SNIPPETS` auto-detection into `contextManager.js` reducing per-script boilerplate
- Unexported 14 internal symbols across scripting, command, storage, and builtin packages
- Refactored txtar collision handling to use full relative paths instead of filename-only deduplication
- `osm pr-split` orchestrator magic numbers: 7 hardcoded timing literals (launcher poll, timeout, stable-need, post-dismiss, plan-poll timeout, check-interval, min-poll-interval) moved from `pr_split_10d_pipeline_orchestrator.js` to centralized `AUTOMATED_DEFAULTS` in chunk 10a
- `osm pr-split` remaining magic numbers: 7 additional timing constants moved to shared defaults — spawn delay (09→AUTOMATED_DEFAULTS), resolve grace period and backoff base/cap (10c→AUTOMATED_DEFAULTS), TUI elapsed thresholds (15b→local `TUI_THRESHOLDS` object)
- `osm pr-split` Claude session pipeline: replaced shared mutable `st.claudeCrashDetected` flag with event-driven `session().isDone()` and `session().isRunning()` from the InteractiveSession interface — crash detection is now channel-based (fires when child PTY output drains), auto-attach and status badge use `isRunning()` instead of `hasChild()`, and all `isDone()` checks are guarded with executor existence to prevent false positives from the pre-closed sentinel channel on never-attached sessions

### Deprecated
- `osm:nextIntegerId` module name: use `osm:nextIntegerID` instead (old name still works as an alias)

### Removed
- `osm claude-mux` standalone CLI command (`status`, `start`, `stop`, `submit` subcommands), `MCPInstanceConfig` auto-injection, control server RPC, shell completions, and all associated documentation and tests — functionality superseded by `osm pr-split` which uses the `osm:claudemux` module directly; `OllamaProvider` and other building blocks remain available in the `osm:claudemux` module
- `osm mcp`, `osm mcp-instance`, `osm mcp-parent` commands and all 15+ MCP tool handlers — MCP server functionality removed from CLI
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
- `NewEngineDeprecated` function from scripting package — all 123 call sites across 34 files migrated to `NewEngine` with explicit parameters (`nil, 0, slog.LevelInfo`)

### Fixed
- `osm pr-split` fails fast on non-git directory or missing base branch: `validateGitRepo()` runs before JS engine or TUI wizard startup, producing a clear one-line error instead of cryptic downstream failures
- Flag parse errors now show a clean one-line message with a `--help` hint instead of dumping the complete flag listing; `fs.Usage` is suppressed during parse and restored for explicit `--help` invocation
- `osm pr-split` Ask Claude: fixed 5 bugs — pipeline cleanup no longer destroys Claude process before `PLAN_REVIEW`, `finishTUI` preserves MCP callback on success, question detection gate triggers when Claude is alive regardless of `isProcessing`, `writeToChild` errors surfaced to user instead of silently swallowed, and `confirmCancel` properly cleans up Claude and MCP
- `osm pr-split` Ctrl+] passthrough: fixed critical stdin contention — BubbleTea cancelreader and `RunPassthrough` were both reading stdin simultaneously; now wires `toggleKey`/`onToggle` via `tea.run()` options so Go-level `toggleModel` calls `ReleaseTerminal` before passthrough
- `osm pr-split` key forwarding: ESC key now correctly maps to `\x1b` (BubbleTea sends `'esc'` not `'escape'`), shell tab uses `INTERACTIVE_RESERVED_KEYS` (pane-management keys only) instead of blocking all reserved keys, and modifier+arrow/nav key combinations are now forwarded to PTY sessions
- `readFile` error messages: 3 sites across `pr-split` and `super-document` scripts concatenated `.error` (boolean `true`) into user-facing messages instead of `.message` (actual OS error string), producing `"failed to read plan: true"` — now uses `.message` for informative errors
- `osm pr-split` scoped verify on macOS: `scopedVerifyCommand()` only checked `=== 'make'` but `discoverVerifyCommand()` returns `'gmake'` on macOS — now recognizes both
- `osm pr-split` resume plan losing falsy config values: `loadPlan` restore used `||` fallback pattern, silently discarding `verifyCommand: ''` or `maxFiles: 0` — now uses explicit `!= null` check
- `osm pr-split` pipeline step null guard: `step()` wrapper in orchestrator now normalizes null/undefined callback returns to an empty success result instead of crashing with an opaque TypeError on `result.error` access
- `osm pr-split` PickAndPlace E2E test harness: `Close()` now sends a best-effort quit signal before PTY closure — without this, BubbleTea and the PABT ticker goroutine blocked for up to 60s after PTY SIGHUP, causing mouse tests to hang and the full test suite to timeout
- `osm pr-split` error wrapping in `validateGitRepo`: changed `%v` to `%w` in the fallback error return so the original `exec` error is properly wrapped and callers can use `errors.Is()`/`errors.As()` to inspect the underlying cause
- Duplicate error output: commands that printed to stderr AND returned an error caused `main()` to print the error again — introduced `SilentError` type in `internal/command/` so commands can signal "already reported" to the top-level handler; 41 error sites converted across 9 command files (builtin, goal, sync, scripting_command, log_tail, mcp_bridge, completion_command, sync_config, main.go)
- Stale sync config lock after crash: `syncConfigLock` now detects stale locks via PID liveness check (Unix signal-0) and 10-minute age timeout; platform-specific `processAlive()` via build tags; Windows falls back to age-based detection only
- `TestExecAndExecv` ETXTBSY on Docker overlayfs: added directory `fsync` after write-then-rename to flush metadata before `exec` — the canonical POSIX pattern for ensuring rename durability
- TUI model selection regex (`reSelectedArrow`) only matched `>` (ASCII) and `❯` (U+276F) — added `▸` (U+25B8 Ollama), `►` (U+25BA), `→` (U+2192) for cross-provider compatibility; 4 new test cases
- Bash completion formatting: `;;` case terminators for `schema)` and `log)` were concatenated on the same line as the next case pattern — split to separate lines
- Zsh completion `commands` array scoping: array was declared inside the `commands)` case branch, making it inaccessible to the `args)` branch where `help)` needs it — hoisted to function scope
- Data race in storage path globals: added `sync.RWMutex` guarding `getSessionDirectory` and `getSessionLockFilePath` accessor functions in `paths.go`, preventing concurrent read/write of function-variable overrides during cleanup scheduling
- Help command silently discarded `tabwriter.Flush()` errors — `HelpCommand.Execute` now propagates flush failure as a command error instead of assigning to `_`
- `writeResolvedTable` silently discarded `tabwriter.Flush()` errors — function now returns `error`; callers (`config list`, `config diff`) propagate flush failures
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
- `osm pr-split` "Processing... forever" hang: BubbleTea event loop deadlock caused by `waitFor` blocking the goja event loop — replaced synchronous `waitFor` with fully async pipeline; all analysis/execution/verification steps now use Promises resolved via `tea.tick` polling; added try-catch around every async step to guarantee `isProcessing=false` on all error paths
- `osm pr-split` binary file NaN in diff stats: `git diff --numstat` outputs `- -` for binary files — detection now flags `binary: true` with `additions: null` / `deletions: null`; stats command renders `[binary]` instead of `+null/-null`
- `osm pr-split` skipped files visibility: files with unknown git status codes (not A/M/D/R/C/T/U) are now collected in `skippedFiles` array and displayed as a warning section in the execution screen instead of being silently dropped
- `osm pr-split` title bar step label in terminal states: `DONE`/`CANCELLED`/`PAUSED`/`ERROR` now show meaningful labels instead of inherited "Finalization"
- `osm pr-split` conversation history unbounded growth: capped at `MAX_CONVERSATION_HISTORY` (100 entries, configurable) with one-shot log warning on truncation
- `osm pr-split` plan path resolution: `resolvePlanPath()` resolves relative paths against `config.dir` instead of always using CWD — `--dir=/path` now correctly places/finds plan files inside the target directory
- `osm pr-split` worktree temp path: `worktreeTmpPath()` uses `TMPDIR`/`TMP`/`TEMP` environment variables with `/tmp` fallback and random entropy, replacing fragile `dir + '/../'` patterns that broke with non-standard directory layouts
- `osm pr-split` `syncMainViewport` ReferenceError: `CHROME_ESTIMATE` constant and `syncMainViewport` function hoisted to IIFE scope instead of being defined inside nested closures — eliminates crash on window resize before TUI fully initialized
- `osm pr-split` layout shift on button focus: introduced `focusedSecondaryButton()` style matching `secondaryButton()` dimensions — all button pairs now maintain consistent width whether focused or unfocused
- `osm pr-split` pipeline re-verify fix: the re-verify step after conflict resolution now actually checks branch pass/fail status instead of always returning success
- `osm pr-split` headless crash detection guard: `claudeCrashDetected` flag in `aliveCheckFn` only checked when `tuiMux` is present, preventing false positives in headless/test mode
- `osm pr-split` 22 falsy-value `||` anti-pattern bugs: audit of chunks 09-16f found 22 instances where `x || default` silently discarded intentionally falsy values (0, empty string) — `exitCode=0` treated as error (2 sites), timeout/poll values of 0 silently ignored (11 sites), `maxFiles=0` lost across 5 TUI paths, `maxConversationHistory=0` and `claudeHealthPollMs=0` overridden (4 sites); all fixed with `typeof`-guarded ternaries that preserve zero values while falling back for `undefined`/`null`; 8 regression tests added
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
- `osm pr-split` session-specific passthrough: Ctrl+] toggle and passthrough operations used whichever session was currently active in the SessionManager instead of the specific Claude session — `_onToggle()`, `renderStatusBar`, `renderClaudePane`, `handleAutoSplitPoll`, and manual-fix passthrough now all route through `getInteractivePaneSession(s, tab)` which returns a pinned-ID proxy; `_buildClaudeProxy` gains a `passthrough()` method using activate→switchTo→restore pattern
- Remove `WithAutoExit(true)` from persistent Runtime, eliminating a startup race where the event loop could terminate before initialization completed (`'loop has been terminated'` on subsequent script/TUI operations). Also removed the contradictory liveness-timer machinery (`livenessTimerID`, `ResolveLiveness()`), the ineffective `waitForAsyncWork()` sentinel drain, and the false-property sentinel drain tests. The Runtime is persistent (`WithAutoExit(false)`) and lives until `Close()` is called explicitly.

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
