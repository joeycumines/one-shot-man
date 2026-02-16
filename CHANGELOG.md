# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `osm goal paths` subcommand: displays all resolved goal discovery paths with source annotations (`standard`/`custom`/`autodiscovered`), existence status (`✓`/`✗`), and config validation warnings for missing custom paths
- `osm script paths` subcommand: displays all resolved script discovery paths with the same annotated format
- `AnnotatedPath` type in discovery subsystem with `Path`, `Source`, and `Exists` fields
- Shell completion for `paths` subcommand in `osm goal` and `osm script` (bash, zsh, fish, powershell)
- Built-in goal `pii-scrubber` (category: data-privacy): redacts personally identifiable information from code, logs, and data with three redaction levels (strict/moderate/minimal) and deterministic placeholder mapping
- Built-in goal `prose-polisher` (category: writing): 7-step copyediting pipeline (structural review, clarity, consistency, concision, correctness, tone alignment, final polish) with four target styles (technical/casual/academic/marketing) and `hot-expand-section` snippet
- Built-in goal `data-to-json` (category: data-transformation): extracts structured JSON from unstructured text, logs, or data with four extraction modes (auto/tabular/log/document) and JSON Schema output
- Built-in goal `cite-sources` (category: research): generates answers with numbered inline citations from provided source material, with three citation formats (numbered/author-date/footnote) and `hot-challenge-claims` snippet
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
- Fuzz tests for config parser, diff splitter, buildContext, and Goja runtime (zero panics across 2.4M+ executions)
- Security test suites: 34 input sanitization tests and 18 JS sandbox boundary tests
- `docs/security.md` documenting JavaScript sandbox boundaries and threat model
- Performance benchmarks across engine creation, filesystem, PA-BT planning, bubbletea, and 8 additional categories (60+ new benchmarks total)
- Test coverage expanded across 25+ packages with notable gains: bubblezone 0→98.7%, lipgloss 58→99%, tview 68.5→96.4%, bubbletea 75.8→91.2%, viewport 73.3→97.3%, overall cmd/osm 91.4→94.8%

### Changed
- Renamed `pabt.ModuleLoader` to `pabt.Require` for API consistency
- Moved `CONFIG_HOT_SNIPPETS` auto-detection into `contextManager.js` reducing per-script boilerplate
- Unexported 14 internal symbols across scripting, command, storage, and builtin packages
- Refactored txtar collision handling to use full relative paths instead of filename-only deduplication

### Removed
- `osm:tview` native module and entire `internal/builtin/tview/` package (~2,100 lines): superseded by `osm:bubbletea`
- `TViewManagerProvider` interface and `GetTViewManager()` method from scripting engine
- `rivo/tview` and `gdamore/tcell/v2` Go module dependencies
- Deprecated `tui.createAdvancedPrompt` alias; use `tui.createPrompt` instead
- Deprecated `GetStateViaJS`/`SetStateViaJS` aliases from scripting API
- `ContextCommand` interface from command package
- `BTBridge` alias from builtin package
- 3 unused benchmark threshold constants
- Stale `internal/termtest/*` entries from `.deadcodeignore`

### Fixed
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
- 2 Windows test failures: echo builtin and tview Console API tests skip on unsupported platforms

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
