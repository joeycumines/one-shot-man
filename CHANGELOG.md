# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Terminal emulator coverage for insert mode (SM 4), line feed/new line mode (SM 20), and private modes 1, 6, 7, 25, 47, 66, 1000, 1001, 1002, 1003, 1004, 1006, 1047, 1049, 2004 and 2026, plus device attributes (DA), status reports (DSR), cursor shape (DECSCUSR), window ops (XTWINOPS), soft reset (DECSTR), and mode queries (DECRQM, DECRQSS). Includes charset handling (G0/G1, line drawing), colon subparameters, OSC and DCS dispatch, scrollback with reflow, dirty-row tracking, batched ASCII output, search, copy mode, and alt-screen variants. See `internal/termmux/vt` and `docs/architecture-termmux.md`.
- Session and window management for the multiplexer: per-pane creation, focus, resize, swapping and zoom, per-window layout modes (tiled, stacked, horizontal, vertical, main-*), break and join between windows, copy mode key handling, search, monitoring for activity, silence and bell, remain-on-exit with respawn, pipe to file or command, message overlay, capture region, session lock, synchronize-panes, chooser, pane borders, and status bar positioning. See `internal/termmux/manager.go`, `pane_manager.go`, `window.go`, `layout.go`, `monitor.go` and `docs/termmux-architecture.md`.
- TermUI component suite in `internal/termui` and `osm:termui/*` bindings: box, compositor with generation caching, coordinate geometry, divider, label, layout helpers, list, modal, panel, split layout and split view, table, termpane, toast, and focus groups. Includes `internal/vtassert` helpers for cell and region checks and `internal/vhs` helpers for VHS tape and GIF generation. Example `scripts/example-13-split-pane.js` and updated `docs/scripting.md`.
- JS modules `osm:aimux` (generic process provider, parser, event stream, health monitor, registry), `osm:astpack` (Go parser plus heuristics for JS/TS/Python/Rust/C++ that packs symbols to 4000 tokens, `pack` and `packDiff` return promises), `osm:difftriage` (`triage` and `triageSummary`), and `osm:gitops` (go-git v6 reads `isRepo`, `open`, `headBranchName` and async writes `hasStagedChanges`, `addAll`, `commit`, `push`). Registered in `internal/builtin/register.go`.
- JS compliance harness in `internal/jscompliance` with tiers for adapter surface, binding contract, engine integration, ESM, global surface, plus `test262` at 1100 cases and `goja-compat` at 600 cases, overall report for 1700 cases, and reference `docs/reference/js-compliance-suite.md`. Adds `internal/aimuxcore` extraction of parser, provider, event stream and TUI state, and `internal/triage`.
- Documentation additions: `docs/architecture-termmux.md`, `docs/termmux-architecture.md`, `docs/reference/js-compliance-suite.md`, expanded `docs/reference/termmux-js-api.md` (bounded sessions, new events `activity`, `silence`, `title`, `cwd`, `clipboard`, layout helpers), `docs/scripting.md` notes on `osm:termui/*` and async `osm:gitops`/`osm:tokenizer`, and config renames in `docs/reference/config.md`.

### Changed
- JS bindings for file, clipboard, and process operations are now async where I/O can wait. `osm:os` `readFile`, `writeFile`, `appendFile`, `fileExists`, `openEditor`, `clipboardCopy` and `clipboardPaste` return promises, `osm:path` `glob` returns a promise, `osm:ctxutil` `buildContext` returns a promise, `osm:exec` now only `execv(argv)` returning a promise with `stdout`, `stderr`, `code`, and `spawn` streams where `read` and `wait` are promises, `osm:tokenizer` `loadFile` is async and renames `loadFromFile` to `loadFile` etc., `osm:bt` Bridge requires adapter and `Run`/`RunSync` renames, `osm:bubbletea` Runner renames, `osm:mcp`/`osm:mcpcallback`/`osm:grpc`/`osm:termmux` require `ctx` and `adapter`/`loop`. See `internal/builtin/register.go` and `docs/scripting.md`.
- Event loop surface updated to the 20260823 form. Strict microtask ordering is always on. Loop creation uses `WithLogger` and `WithMetrics`. Blocking work must use `adapter.Promisify` or `adapter.TrackPromise` with TrackedSettlement, and `adapter.Done` is the shutdown barrier that rejects pending promises with `ErrLoopTerminated`. See `internal/scripting/runtime.go` and `engine_core.go`.
- Go module fork moved from `github.com/dop251/goja` to `github.com/joeycumines/goja` across `go.mod` and all builtins, Go version raised to 1.27.0, and Charm dependencies updated (`bubbles`, `bubbletea`, `lipgloss`, `go-git`, `go-eventloop`, `goja-eventloop`, `mcp/go-sdk`, `x/sys`, `x/term`, `x/text`). Build helpers added in `config.mk` (`make-all-with-log`, `test-jscompliance`, `test-test262`, `report`, `test-engine`, `cover-engine`).
- Configuration keys that use `claude` are now `agent` (`agent-command`, `agent-arg`, `agent-model`, `agent-config-dir`, `agent-env`) and the `claude-mux` section is gone. Documentation updated in `docs/reference/config.md`, `docs/reference/command.md`, `docs/reference/tui-keymap.md`, `docs/architecture.md`, and `project.mk`.

### Deprecated
- `osm:nextIntegerId` remains as an alias, the canonical name is `osm:nextIntegerID`.

### Removed
- `osm:time` (`sleep`) removed. Use `setTimeout` or `delay` instead. See `internal/builtin/time` deletion and `docs/reference/time.md` removal.
- `osm:claudemux` removed. Use `osm:aimux` with `processProvider` and `newRegistry` instead. All `internal/builtin/claudemux` files are gone and `internal/aimuxcore` is the new home for parser and provider logic.
- `Runtime.VM()` accessor, thread check mode, and `WithStrictMicrotaskOrdering` option removed.

### Fixed
- Erase display mode 3 (ED 3) now clears only scrollback, not the visible screen, and cursor positioning with origin mode now clamps to the scroll region. Wide character handling repairs orphaned halves on resize, and pending wrap is cleared on cursor movement. See `internal/termmux/vt/screen.go` and `csi.go`.
- PTY drain no longer pins the 32 KB read buffer, and save and restore (DECSC/DECRC) now covers charset, origin, wrap, cursor shape, focus, and highlight state. SGR matching ignores search highlight so rendering diffs remain stable. See `internal/termmux/vt/esc.go`, `sgr.go`, and `ptyio/reader.go`.
- Settlement during loop shutdown tolerates `ErrLoopTerminated`, `ErrAdapterInvalid`, and `ErrPromiseSettled` in `fetch`, `exec`, `tokenizer`, and `mcpcallback` so promises do not hang. Context extraction in `ctxutil` now runs on the loop via `Promisify` instead of blocking.

### Security
- JS sandbox now removes dangerous globals that `goja-eventloop` Bind had added (`process.exit`, `env`, `pid`, `Buffer`, `Deno`, `quit`), leaving only `process.nextTick` and emitter methods. See `internal/scripting/runtime.go` H0 block and `internal/jscompliance` checks.

## [v0.3.0] - 2026-06-16

### Changed
- `remove` in the context manager now accepts multiple IDs (`remove <id> [id ...]`) so you can delete several `contextItems` in one command instead of one by one. Updated `internal/builtin/ctxutil/contextManager.js` and all shipped goal templates in `goals`.

### Fixed
- Textarea cursor handling on wrapped lines could overshoot or clamp incorrectly. Now uses `max(..., 0)` in `findColumnInSegment`, `setPositionInternal`, `setRow`/`setCol` and visual line calculation in `internal/builtin/bubbles/textarea/textarea.go`.

## [v0.2.0] - 2026-05-15

### Added
- `osm tokenize` CLI and `osm:tokenizer` JS module. Local, offline Hugging Face compatible pipeline in `internal/tokenizer` (BPE, Unigram, WordPiece, WordLevel, ByteLevel, Metaspace, Whitespace, Trie, Lattice) with serialization for `tokenizer.json`, vocab and merges. CLI is `osm tokenize -tokenizer <path> [-text <text> | -stdin] [-quiet] [-verbose]`. JS is `require('osm:tokenizer')` with `tokenize`, `count`, `byteCount`, `lineCount`, `loadFromFile`, `loadFromJSON` and model-specific loaders.
- Six utility JS modules wrapping Go stdlib in `internal/builtin/register.go`: `osm:crypto` (`sha256`, `sha1`, `md5`, `hmacSHA256`, `hmacSHA1`), `osm:encoding` (`base64*`, `hex*`), `osm:flag` (FlagSet parsing), `osm:json` (`parse`, `stringify`, `query`, `mergePatch`, `diff`, `flatten`), `osm:path` (`join`, `dir`, `base`, `ext`, `abs`, `glob`), `osm:regexp` (RE2 `match`, `find`, `replace`, `compile`). See `docs/reference/encoding.md` and `regexp.md`.
- `osm pr-split`. Split a large branch into stacked, reviewable PRs. New command in `internal/command/pr_split.go` with helpers `pr_split_00_*` through `pr_split_12_*` (diff analysis, grouping, plan, validation, git execution, verification, PR creation, conflict resolution, Claude integration) and an interactive Bubble Tea wizard `pr_split_13_*` to `pr_split_16*` (configure, plan review, execution, verification) plus headless mode. Strategies are `directory`, `directory-deep`, `extension`, `chunks`, `dependency`, `auto`. Flags include `-base`, `-strategy`, `-max`, `-verify`, `-resume`.
- Terminal multiplexer and VT stack. New `internal/termmux` (VT parser, screen, render, PTY, manager, persistence, SGR mouse, status bar) exposed as `osm:termmux` in `internal/builtin/termmux/module.go`, plus `osm:claudemux` building blocks and `osm:mcp`/`osm:mcpcallback` for Model Context Protocol.
- Async `osm:fetch` and `osm:grpc`/`osm:protobuf`. `osm:fetch` is promise based and browser compatible (`fetch(url, {method, headers, body, signal, timeout})` returns `Promise<Response>` with `Headers` and `text()`/`json()` promises, `AbortController` cancellation). `osm:grpc` and `osm:protobuf` are promise based via `goja-grpc`/`goja-protobuf` on `go-inprocgrpc`.
- Helpers, examples, and docs. `osm:format` (`formatNum`/`formatBytes`), example goals in `goals` and scripts `scripts/example-06-api-client.js` and `example-07-flag-parsing.js`, new reference pages `docs/reference/termmux-js-api.md`, `tui-keymap.md`, `security.md`, and `docs/architecture-pr-split-chunks.md`.

### Changed
- TUI and Go runtime upgraded. Go 1.25.7 to 1.26.2 and Charm stack `github.com/charmbracelet/{bubbles,bubbletea,lipgloss}` v1 to `charm.land/*` v2 (`bubbles/v2`, `bubbletea/v2`, `lipgloss/v2`, `bubblezone/v2`) plus `go-git/v6`, `modelcontextprotocol/go-sdk` and `x/*` bumps in `go.mod`.
- JS event loop replaced. `dop251/goja_nodejs/eventloop` to `joeycumines/go-eventloop` and `goja-eventloop`. The shared `goja.Runtime` runs single threaded behind an Adapter (`Loop()`, `Runtime()`, `Adapter()`, `Promisify()` on `EventLoopProvider` in `internal/builtin/register.go` and `internal/scripting/engine_core.go`). All I/O bindings return promises and timers, `AbortController`, `TextEncoder`, `URL` are available as globals.
- Behavior tree bridging tightened. `internal/builtin/bt.Bridge` now requires the event loop adapter at construction (`NewBridgeWithEventLoop(ctx, loop, runtime, registry)`), `pabt.ModuleLoader` renamed to `pabt.Require`, `Register` no longer takes a `tviewProvider`.

### Deprecated
- `osm:nextIntegerId` casing is deprecated in favor of `osm:nextIntegerID` (kept as alias).

### Removed
- `osm:tview` and the entire `internal/builtin/tview` package removed, along with `rivo/tview` and `gdamore/tcell/v2`. Superseded by `osm:bubbletea`.

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

[Unreleased]: https://github.com/joeycumines/one-shot-man/compare/v0.3.0...HEAD
[v0.3.0]: https://github.com/joeycumines/one-shot-man/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/joeycumines/one-shot-man/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/joeycumines/one-shot-man/releases/tag/v0.1.0
