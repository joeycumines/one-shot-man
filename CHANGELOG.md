# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.9.1] - 2026-11-20

### Changed
- `osm:exec` `execv` result objects now include a boolean `truncated` field across all return paths (including pre-child validation errors) to distinguish incomplete output capture structurally from normal exit codes.
- `osm:exec` `child.wait()` now observes caller/engine context cancellation via `WaitContext(ctx)`: a caller abort before reap rejects with the context error, while a recorded exit status wins over a concurrent cancellation.
- `osm:bubbletea` runs headless (`tea.WithoutRenderer()`, bracketed-paste escapes gated on TTY) when stdin/stdout is not a terminal, binds program lifecycle and `WaitForProgram` to the engine's context (so `Engine.Close()` and unlistened signals tear down idle programs promptly), unwraps file-backed non-TTY input directly to `*os.File` (preserving piped stdin while bypassing `go-prompt`/`go-tty` on Windows), and drops defaulted `nil` input.
- `internal/termmux` `CaptureSession.Resize` serializes concurrent resizes without holding the session mutex across the PTY ioctl and propagates PTY resize errors (including `ErrClosed` and `ErrNotSupported`) while leaving stored dimensions unchanged on failure.

### Fixed
- `osm:exec` `child.wait()` reports the child's exit status even when tearing down its process tree failed, including a clean `exit 0`; `runExecCommand` reads `ProcessState` directly so post-reap context cancellation or post-EOF pipe-close errors preserve the real exit code instead of degrading to `-1`; `pumpPipe` treats a post-clean-reap Windows job-close read error as clean EOF when no drain abort or tree-close failure occurred.
- `osm pr-split` classifies unknown baseline and split verdicts (`execFailed`, `truncated`, `cancelled`) as "no information" in both CLI and TUI verification paths so failing splits are never forgiven as pre-existing and dependency guards stay intact; sync and async timeout results preserve both `truncated: true` and timeout attribution (`timedOut: true`).
- `osm pr-split` `verifySplitAsync` computes its cleanup budget in `finally` (decoupled from `verifyTimeoutMs`, clearing all bounded-await timers), captures abnormal-exit errors in `catch` before `finally` so `cleanupFailed`, `worktreeRetained`, `worktreeDir`, and cleanup diagnostics always attach, and awaits bounded kill + child exit before removing the worktree (retaining it if termination is unconfirmed).
- `osm pr-split` reports non-cancellation `child.wait()` rejections with the failure reason instead of collapsing to `exit -1`, and reports rejecting stream reads as cut-short captures rather than unhandled rejections.
- `internal/termmux/pty` ConPTY runtime probe fails the Windows test suite on broken `Spawn`/`Wait`/`Close` (skipping only `STATUS_DLL_INIT_FAILED` and absent pseudoconsole APIs), caches verdicts including failures, runs `cmd.exe /d /c exit 0` with a 4096-byte rolling output tail, and includes exit code and output tail on close failures.
- `internal/termmux` and `internal/termmux/pty` test reliability: `TestProcess_Close_CustomGracePeriod` now uses `testing/synctest` for exact fake-clock verification of grace-period timing; `TestSessionManager_Pipeline_EOFTransition` and sibling event tests wait on the event channel with a deadline and verify post-exit state; ConPTY smoke output deadline raised to 60s.
- `osm sync load` strips YAML frontmatter from files with CRLF line endings and no longer truncates an entry whose frontmatter value contains `---`; `osm sync --help` lists `config-push`/`config-pull`.
- CI pins `actions/checkout` and `actions/setup-go` by commit SHA, installs the exact Go patch from `go.mod` with `GOTOOLCHAIN=local` so the compiler stays aligned with pinned `staticcheck`, and `NewEngine` cancels its derived context on initialization errors.
- `osm pr-split` test harness pins prompt-ready knobs via `_setNumberKnob`, keeps a 2-minute default `EvalTimeout` with a 10-minute opt-in for the 10,000-file grouping tests, and verifies the TUI palette's WCAG AA contrast ratios.

## [v0.9.0] - 2026-11-15

### Added
- `osm pr-split` agent tab is a live interactive terminal surface (clipped-pane termpane embedding with working cursor) instead of polled screenshots; non-reserved keys and inside-pane clicks reach the agent PTY with screenshot fallback when detached.
- `osm pr-split` preserves the agent session plus MCP callback on classification and heartbeat-stale timeouts: captures evidence, writes a transcript under the storage session directory (same family as `pr-split-mux.state.json`), and offers ERROR-state `a`/`f`/`d` actions (show, focus, discard-and-quit after teardown settles).
- `osm:termmux` `newKeymap({prefix?, passthroughKey?, keys?, chords?})`, a self-contained key-command mapper with an optional chord prefix, a direct key→action map, a chord map, and a passthrough key. `handleKey` reports handled/action/armed state, `<esc>` or a repeated prefix cancels an armed chord, and unbound chord keys reach the child. Bindings mutate at runtime through `bind`/`bindChord`/`unbind`/`unbindChord`/`setPrefix`/`setPassthroughKey`, and `listKeys` renders a deterministic aligned table.
- `osm:termmux` `keymapOverlay({keymap, tea, onClose?})`, a ready-to-run BubbleTea model (same shape as `chooseTree`) that renders a keymap's `listKeys` table as a centered popup and closes on any key.
- `osm:termmux` `inputStaged(id, data)` and `session.writeStaged(data)`: a staged/atomic session write that copies the payload once and delivers it to the PTY in a single `Write`, so large pastes are never seen by the child as several contiguous chunks. Ordering against regular input is preserved because both use the same queue.
- `.github/workflows/release.yml` publishes cross-platform binary assets for tagged releases.

### Changed
- Terminal session events are delivered by a lossless `ChanPubSub` fan-out instead of the previous best-effort bus: one shared unbuffered broadcast channel replaces per-subscriber buffered queues, so no control event is ever dropped when a consumer falls behind. Subscribers acknowledge every event before doing other work, and an unacknowledged subscriber blocks publishers by design rather than silently losing events. `SessionManager.Subscribe`/`Unsubscribe(id)`/`EventsDropped` and the JS `eventsDropped` binding are replaced by `SubscribeEvents`/`UnsubscribeEvents`/`AckEvent`; a drop counter no longer has meaning.
- Session output is no longer an event-bus signal. It is a conflated, stateful wake-up (`WatchOutput`/`WatchAnyOutput`/`TakeOutputDirty`) that consumers resolve against the screen snapshot, so a fast child process can no longer overrun a slow consumer and a burst collapses to a single pending notification. The JS `output` listener contract is unchanged.
- The `osm:termmux` JavaScript event bridge subscribes only while listeners exist and delivers at most 128 events or 5ms per event-loop turn, rescheduling the remainder, so a large backlog cannot monopolise the event loop. A manager with no JavaScript listener no longer holds a bridge subscription or a subscription goroutine.
- `termpane` forwards pastes as one staged write instead of 4096-byte chunks.
- Modified navigation and tilde keys now use the tmux-verified xterm modifier parameter `ESC[1;{mod}{letter}` (and `ESC[{num};{mod}~` for tilde keys), with `mod = 1 + shift(1)|alt(2)|ctrl(4)|meta(8)`; application cursor mode uses the SS3 form. Previously only shift/ctrl+shift prefixes were handled, so alt+left arrived as a double-ESC sequence whose trailing bytes surfaced as literal characters. `alt+<printable>` keeps the legacy ESC prefix.

### Fixed
- `osm pr-split` spawns the agent argv with exactly one `--mcp-config` by construction (empty provider `defaultArgs`, single builder, both `--mcp-config x` and `--mcp-config=x` user forms rejected); dead `agent-model`/`agent-config-dir` flags removed from flags, config, JS, docs, completions, and tests; help copy is honest about required agent-command, comma-rule agent-env, and non-bounding timeout.

## [v0.8.0] - 2026-11-10

### Added
- `userk8s` accepts `gemini` as a client surface. The GenerateContent surface is a shaper capability rather than an access protocol: chat, messages, and responses accesses can each serve it by transcoding, the surface is accepted for every shaper protocol, and it is rejected on a direct transport, which can only carry its own protocol's surface.

### Changed
- A trailing `/v1beta` on a gemini endpoint is normalized so the derived shaper mount prefix matches the other surfaces.

## [v0.7.1] - 2026-11-10

### Changed
- `osm version` reads the CLI version from `runtime/debug` build info instead of a hard-coded constant, so tagged releases and VCS pseudo-versions are reported accurately and missing build metadata falls back to `devel`.
- Shaper route validation canonicalizes endpoint paths and removes the surface API suffix before deriving the mount prefix, which accepts the renderer's real layouts while rejecting API-only endpoints, repeated slashes, and dot segments that could hide a duplicate or conflicting mount.

### Fixed
- The rendered personal profile fixture and its count assertions match the current registry output.

## [v0.7.0] - 2026-11-02

### Added
- `userk8s` catalog: `Model` describes only canonical identity and capabilities; provider offers move to `ModelServing`, fallback chains to `ModelSelection`, and entity access to `ModelRoute`. A serving carries the provider's wire id and narrowing overrides, a route carries the access, credential binding, exposed offers, surfaces, and transport, and `LocalSecretBinding` declares its own credential scheme. Consumers read `serving.effective` rather than merging model and serving facts themselves.
- The userk8s JavaScript surface exposes servings, routes, selections, and bindings, resolves credentials by binding, projects by serving, and returns promises for every catalog operation.
- Signal-delivery failures are reported on stderr, so an undelivered signal is distinguishable from a missing listener.

### Fixed
- The files backend matches the API server: it applies CRD defaults while decoding and validates the same CEL invariants in Go, including reasoning facts, tool settings against effective capabilities, route surfaces, offer ids, selection cycles, and shaper mount collisions.
- Bubblezone markers survive the compositor: they are transcoded to APC carriers that the ultraviolet cell parser keeps, only balanced marker pairs are restored after render, and hit tests use a geometry-only layer stack.

### Changed
- Documented that client-facing `Model` context and output token limits may be lower than a provider's hard limits, so a safety-buffered budget does not imply an upstream ceiling.
- Bumped dependencies including the Kubernetes libraries.

## [v0.6.0] - 2026-10-08

### Fixed
- `exec` submits the lifetime task before starting the loop, closing the startup race that could strand a child when auto-exit terminated the loop before admission.
- The pr-split empty-merge-base guard covers the async paths and the diff-stats path, and the regression tests use `GitMockSetupJS`.
- Scripting suppresses the post-exit callback race when `process.exit` terminates the event loop.
- ConPTY test probing retries three times with backoff, so a transient `exit 1` on a loaded runner no longer reads as a broken host; a genuinely broken host is still detected.
- The MCP init watcher no longer leaks goroutines: `WatchForInit` returns a deregistering cancel function, all call sites use the leak-safe `injectOnMCPInit` helper, and bespoke pr-split MCP-inject goroutines are fully done-interruptible.

### Changed
- `GO_TEST_FLAGS` default timeout is 40 minutes, and the pr-split `EvalJS` default timeout is 2 minutes, both to accommodate heavy real-git evaluations on loaded parallel CI runners.

## [v0.5.0] - 2026-09-17

### Added
- `osm:freezeterm` and `internal/freezeterm`: render captured terminal text as SVG or PNG through the external [freeze](https://github.com/charmbracelet/freeze) CLI. The Go package discovers and supervises the binary (explicit `--output`, typed errors for unavailable/cancelled/non-zero runs, temporary-output cleanup) and never imports termmux; the JS module exposes async `info`, `render` and `renderText` with strict option validation. See `internal/freezeterm`, `internal/builtin/freezeterm`, and `scripts/example-16-freeze-capture.js`.

### Changed
- `osm:termmux` reads now go through one capture surface. `capture(id, opts?)` returns `{plain, ansi, fullScreen, gen, rows, cols, cursorRow, cursorCol, cursorVisible, mouseTracking, mouseSGR, locked, message, timestamp}` (or `null`), with `{start?, end?}` selecting a zero-based end-exclusive visible-row range and `joinWrapped` joining wrapped continuations. Ranged captures preserve every metadata field and clone render caches. The `snapshot`, `capturePane`, `screenshot` and `childScreen` JS methods and the Go `CapturePane`/`GetPlainText`/`GetANSI`/`GetFullScreen` accessors are gone; Go callers use `CaptureScreen` and `ScreenSnapshot.WriteCapture`. See `docs/reference/termmux-js-api.md`.
- Build helpers relocated out of the tracked tree. User-local make overrides live in the gitignored `config.mk`, the shared helpers stay in the `example.config.mk` template, and the project-wide runners (`cross-build`, `test-jscompliance`, `test-jscompliance-all`, `test-test262`, `test-goja-compat`, `fuzz`, `report`, `test-engine`, `cover-engine`) move to `project.mk`; see `Makefile` and `project.mk`. The personal `test-prsplit-*` runners and the fork-blocked fuzz shortcut are dropped, and the previously committed `config.mk` is removed.
- Documentation updated for the ranged capture surface and the `osm:freezeterm` composition across `docs/architecture-termmux.md`, `docs/reference/termmux-js-api.md`, `docs/scripting.md` and `docs/reference/js-compliance-suite.md`.

### Removed
- `renderRaster` and the terminal-to-PNG raster renderer (`RenderRaster`, `RenderRasterDefault`, `SaveRasterPNG` in `internal/termmux/vt`) are removed, with the `VTerm.RenderFullScreen`/`ContentANSI` wrappers and the `RenderContentANSI`/`RenderAll` helpers going with them. Terminal content is exposed through the terminal capture API instead.

### Fixed
- Viewport offset semantics fixed so offset 0 is the live tail (previously the oldest scrollback); selection, search and copy-mode math moved to absolute rows. `termui` split-layout and termpane pair content with its own generation cursor and honour `CursorVisible`.

## [v0.4.1] - 2026-09-13

### Added
- VT coverage for CSI REP (repeat preceding character), HPR/VPR (relative cursor movement) and HPA (absolute horizontal positioning), G2/G3 charset designation with save and restore across DECSC/DECRC and alternate-screen transitions, and mouse tracking preservation across alternate-screen switches. See `internal/termmux/vt`.
- `osm:os` gains `isAbsolute`, `join`, `platform` and `getenv` helpers.
- POSIX shell word splitting for multiplexer spawn: compound commands are parsed into binary plus args (`internal/termmux/pty/command.go`), with BSD-specific termios handling.
- JS compliance coverage for sandbox stripping of dangerous Node process globals and for the JS global surface.

### Changed
- Upstream `go-eventloop`, `goja-eventloop` and `goja-grpc` promoted to v0.1.0. The provider interface renames `Promise` to `Future`, `JSRunner`/`TrySyncJSRunner` take `context.Context`, the `eventlooputil` package is deleted in favour of inline state machines in `Bridge`, `Runtime` and `Engine`, and ownership checks use `loop.IsCallbackOwner()`. See `go.mod`.
- `osm:termmux` lifecycle is async: `start()`, `wait()` and `close()` return tracked promises via the new `lifecycle.go` instead of blocking the event loop. Embedded `pr_split` scripts updated for the new API.
- `osm:exec` spawn uses an unbounded synchronized pipe queue so slow JS consumers no longer lose output, and `osm:fetch` `ReadableStream` pumps integrate with the event loop via `Promisify`. `osm:astpack` token estimation and the tokenizer bindings are refactored.

### Fixed
- Multiplexer capture delivery no longer drops output when consumers are slow (the reader loop always queues copied chunks; the output loop delivers in order and owns passthrough activation), and manager shutdown no longer races request admission (close-channel handshake drains pending requests cleanly). PTY, `ptyio` and multiplexer tests use `t.TempDir()` isolation.
- Behavior-tree ticker admission race with `Bridge.Stop` closed: tickers are constructed and admitted under lock, orphaned untracked settlement is removed, and ticker/manager JS wrappers select on `ctx.Done()` so `.done()` settles after shutdown instead of hanging or leaking goroutines. `RunSync` call sites pass `context.Context`.
- Scripting engine `executeOnLoop` no longer uses the Goja runtime after return when cancellation fires while a callback owns it.
- VT rendering fixes: `MaxScrollOffset` formula corrected, dynamic tab stops for CHT/CBT, no deadlock on focus in/out responses, no spurious pending wrap on resize, private-ED guard against `CSI ? J` erasing the display, SGR colon subparameters, and scrollback wrap tracking for correct resize reflow.
- Storage cleanup hardening: lock parent directories are created, legacy locks are acquired for upgrade compatibility, and the session path is re-statted while holding the lock (TOCTOU fix).
- `osm pr-split` closes the termmux manager before `engine.Wait()` so tracked workers no longer block event-loop auto-exit; test harnesses use deterministic deadlines and failure logging. Windows PTY signal forwarding is real (SIGINT via `os/signal`) instead of a no-op stub.

## [v0.4.0] - 2026-08-30

### Added
- `osm:freezeterm` and `internal/freezeterm`: render captured terminal text as SVG or PNG through the external [freeze](https://github.com/charmbracelet/freeze) CLI. The Go package discovers and supervises the binary (explicit `--output`, typed errors for unavailable/cancelled/non-zero runs, temporary-output cleanup) and never imports termmux; the JS module exposes async `info`, `render` and `renderText` with strict option validation. See `internal/freezeterm`, `internal/builtin/freezeterm`, and `scripts/example-16-freeze-capture.js`.
- Terminal emulator coverage for insert mode (SM 4), line feed/new line mode (SM 20), and private modes 1, 6, 7, 25, 47, 66, 1000, 1001, 1002, 1003, 1004, 1006, 1047, 1049, 2004 and 2026, plus device attributes (DA), status reports (DSR), cursor shape (DECSCUSR), window ops (XTWINOPS), soft reset (DECSTR), and mode queries (DECRQM, DECRQSS). Includes charset handling (G0/G1, line drawing), colon subparameters, OSC and DCS dispatch, scrollback with reflow, dirty-row tracking, batched ASCII output, search, copy mode, and alt-screen variants. See `internal/termmux/vt` and `docs/architecture-termmux.md`.
- Session and window management for the multiplexer: per-pane creation, focus, resize, swapping and zoom, per-window layout modes (tiled, stacked, horizontal, vertical, main-*), break and join between windows, copy mode key handling, search, monitoring for activity, silence and bell, remain-on-exit with respawn, pipe to file or command, message overlay, capture region, session lock, synchronize-panes, chooser, pane borders, and status bar positioning. See `internal/termmux/manager.go`, `pane_manager.go`, `window.go`, `layout.go`, `monitor.go` and `docs/architecture-termmux.md`.
- TermUI component suite in `internal/termui` and `osm:termui/*` bindings: box, compositor with generation caching, coordinate geometry, divider, label, layout helpers, list, modal, panel, split layout and split view, table, termpane, toast, and focus groups. Includes `internal/vtassert` helpers for cell and region checks and `internal/vhs` helpers for VHS tape and GIF generation. Example `scripts/example-13-split-pane.js` and updated `docs/scripting.md`.
- JS modules `osm:aimux` (generic process provider, parser, event stream, health monitor, registry), `osm:astpack` (Go parser plus heuristics for JS/TS/Python/Rust/C++ that packs symbols to 4000 tokens, `pack` and `packDiff` return promises), `osm:difftriage` (`triage` and `triageSummary`), and `osm:gitops` (go-git v6 reads `isRepo`, `open`, `headBranchName` and async writes `hasStagedChanges`, `addAll`, `commit`, `push`). Registered in `internal/builtin/register.go`.
- JS compliance harness in `internal/jscompliance` with tiers for adapter surface, binding contract, engine integration, ESM, global surface, plus `test262` at 1100 cases and `goja-compat` at 600 cases, overall report for 1700 cases, and reference `docs/reference/js-compliance-suite.md`. Adds `internal/aimuxcore` extraction of parser, provider, event stream and TUI state, and `internal/triage`.
- Documentation additions: `docs/architecture-termmux.md`, `docs/reference/js-compliance-suite.md`, expanded `docs/reference/termmux-js-api.md` (bounded sessions, new events `activity`, `silence`, `title`, `cwd`, `clipboard`, layout helpers), `docs/scripting.md` notes on `osm:termui/*` and async `osm:gitops`/`osm:tokenizer`, and config renames in `docs/reference/config.md`.

### Changed
- `osm:termmux` reads now go through one capture surface. `capture(id, opts?)` returns `{plain, ansi, fullScreen, gen, rows, cols, cursorRow, cursorCol, cursorVisible, mouseTracking, mouseSGR, locked, message, timestamp}` (or `null`), with `{start?, end?}` selecting a zero-based end-exclusive visible-row range and `joinWrapped` joining wrapped continuations. Ranged captures preserve every metadata field and clone render caches. The `snapshot`, `capturePane`, `screenshot` and `childScreen` JS methods and the Go `CapturePane`/`GetPlainText`/`GetANSI`/`GetFullScreen` accessors are gone; Go callers use `CaptureScreen` and `ScreenSnapshot.WriteCapture`. See `docs/reference/termmux-js-api.md`.
- JS bindings for file, clipboard, and process operations are now async where I/O can wait. `osm:os` `readFile`, `writeFile`, `appendFile`, `fileExists`, `openEditor`, `clipboardCopy` and `clipboardPaste` return promises, `osm:path` `glob` returns a promise, `osm:ctxutil` `buildContext` returns a promise, `osm:exec` now only `execv(argv)` returning a promise with `stdout`, `stderr`, `code`, and `spawn` streams where `read` and `wait` are promises, `osm:tokenizer` `loadFile` is async and renames `loadFromFile` to `loadFile` etc., `osm:bt` Bridge requires adapter and `Run`/`RunSync` renames, `osm:bubbletea` Runner renames, `osm:mcp`/`osm:mcpcallback`/`osm:grpc`/`osm:termmux` require `ctx` and `adapter`/`loop`. See `internal/builtin/register.go` and `docs/scripting.md`.
- Event loop surface updated to the 20260823 form. Strict microtask ordering is always on. Loop creation uses `WithLogger` and `WithMetrics`. Blocking work must use `adapter.Promisify` or `adapter.TrackPromise` with TrackedSettlement, and `adapter.Done` is the shutdown barrier that rejects pending promises with `ErrLoopTerminated`. See `internal/scripting/runtime.go` and `engine_core.go`.
- Go module fork moved from `github.com/dop251/goja` to `github.com/joeycumines/goja` across `go.mod` and all builtins, Go version raised to 1.27.0, and Charm dependencies updated (`bubbles`, `bubbletea`, `lipgloss`, `go-git`, `go-eventloop`, `goja-eventloop`, `mcp/go-sdk`, `x/sys`, `x/term`, `x/text`). Local build helpers (`make-all-with-log`, `test-jscompliance`, `test-test262`, `report`, `test-engine`, `cover-engine`, `make-all-in-container`, `make-all-run-windows`) live in the tracked `example.config.mk` template (copy to the gitignored user-local `config.mk`); see `Makefile` and `project.mk`.
- Configuration keys that use `claude` are now `agent` (`agent-command`, `agent-arg`, `agent-model`, `agent-config-dir`, `agent-env`) and the `claude-mux` section is gone. Documentation updated in `docs/reference/config.md`, `docs/reference/command.md`, `docs/reference/tui-keymap.md`, `docs/architecture.md`, and `project.mk`.

### Deprecated
- `osm:nextIntegerId` remains as an alias, the canonical name is `osm:nextIntegerID`.

### Removed
- `renderRaster` and the terminal-to-PNG raster renderer (`RenderRaster`, `RenderRasterDefault`, `SaveRasterPNG` in `internal/termmux/vt`) are removed, with the `VTerm.RenderFullScreen`/`ContentANSI` wrappers and the `RenderContentANSI`/`RenderAll` helpers going with them. Terminal content is exposed through the terminal capture API instead.
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
- Helpers, examples, and docs. `osm:format` (`formatNum`/`formatBytes`), example goals in `goals` and scripts `scripts/example-06-api-client.js` and `example-07-flag-parsing.js`, and new reference pages `docs/reference/termmux-js-api.md`, `tui-keymap.md`, `security.md`.

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

[Unreleased]: https://github.com/joeycumines/one-shot-man/compare/v0.9.1...HEAD
[v0.9.1]: https://github.com/joeycumines/one-shot-man/compare/v0.9.0...v0.9.1
[v0.9.0]: https://github.com/joeycumines/one-shot-man/compare/v0.8.0...v0.9.0
[v0.8.0]: https://github.com/joeycumines/one-shot-man/compare/v0.7.1...v0.8.0
[v0.7.1]: https://github.com/joeycumines/one-shot-man/compare/v0.7.0...v0.7.1
[v0.7.0]: https://github.com/joeycumines/one-shot-man/compare/v0.6.0...v0.7.0
[v0.6.0]: https://github.com/joeycumines/one-shot-man/compare/v0.5.0...v0.6.0
[v0.5.0]: https://github.com/joeycumines/one-shot-man/compare/v0.4.1...v0.5.0
[v0.4.1]: https://github.com/joeycumines/one-shot-man/compare/v0.4.0...v0.4.1
[v0.4.0]: https://github.com/joeycumines/one-shot-man/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/joeycumines/one-shot-man/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/joeycumines/one-shot-man/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/joeycumines/one-shot-man/releases/tag/v0.1.0
