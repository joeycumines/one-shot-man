# Hana may steer you through $(error ...) messages. Be prepared to drop everything and obey immediately...
# [CLEARED 2026-04-08] Full autopsy text read: 01_smoking_gun.md, 02_anatomy.md,
# 03_gap_analysis.md, 04_claim_verification.md, 05_honest_conclusions.md, README.md.
# All gaps catalogued. Task 40 directly addresses GAP-C01/C02.

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1
##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS := all GO_TEST_FLAGS=-timeout=20m

.PHONY: make-all-with-log
make-all-with-log: ## Run all targets with logging to build.log
make-all-with-log: SHELL := /bin/bash
make-all-with-log:
	@echo "Output limited to avoid context explosion. See $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log for full content."; \
set -o pipefail; \
$(MAKE) $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS) 2>&1 | fold -w 200 | tee $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log | tail -n 15; \
exit $${PIPESTATUS[0]}

.PHONY: make-all-in-container
make-all-in-container: ## Like `make make-all-with-log` inside a linux golang container
make-all-in-container: SHELL := /bin/bash
make-all-in-container:
	@echo "Output limited to avoid context explosion. See $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log for full content."; \
go_version="$$($(GO) -C $(PROJECT_ROOT) mod edit -print | awk '/^go / {print $$2}')"; \
echo "Running in container golang:$${go_version}."; \
set -o pipefail; \
docker run --rm -v $(PROJECT_ROOT):/work -w /work "golang:$${go_version}" bash -lc 'export PATH="/usr/local/go/bin:$$PATH" && export GOFLAGS=-buildvcs=false && { jobs="$$(nproc)" && [ "$$jobs" -gt 0 ] && jobs="-j $${jobs}" || jobs=''; } && set -x && make $${jobs} $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS)' 2>&1 | fold -w 200 | tee build.log | tail -n 15; \
exit $${PIPESTATUS[0]}

.PHONY: make-all-run-windows
make-all-run-windows: ## Run all targets with logging to build.log
make-all-run-windows: SHELL := /bin/bash
make-all-run-windows:
	@echo "Output limited to avoid context explosion. See $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log for full content."; \
set -o pipefail; \
hack/run-on-windows.sh moo make $(_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS) 2>&1 | fold -w 200 | tee $(or $(PROJECT_ROOT),$(error If you are reading this you specified the `file` option when calling `mcp-server-make`. DONT DO THAT.))/build.log | tail -n 15; \
exit $${PIPESTATUS[0]}

.PHONY: test-short
test-short: ## Run all tests with -short flag (skips slow integration/E2E tests)
	$(GO) -C . test -short -count=1 -timeout=5m ./...

.PHONY: test-termmux
test-termmux: ## Run all termmux tests including slow passthrough tests (with race detection)
	$(GO) -C . test -race -v -count=1 -timeout=5m ./internal/termmux/...

.PHONY: fuzz-session-router
fuzz-session-router: ## Fuzz the SessionManager router (30s default)
fuzz-session-router: FUZZTIME ?= 30s
fuzz-session-router:
	$(GO) test -run=^$$ -fuzz=FuzzSessionRouter -fuzztime=$(FUZZTIME) ./internal/termmux

##@ PR Split Test Targets

.PHONY: test-prsplit-fast
test-prsplit-fast: ## Run fast PR Split unit tests only (no slow/E2E)
	$(GO) -C . test -timeout=600s -count=1 -run 'TestViews|TestFocus|TestChrome|TestUpdate|TestKey|TestTab|TestVerify|TestShell|TestRenderVerify|TestMouseToTermBytes|TestInputRouting|TestE2E|TestCanSpawnInteractiveShell|TestSpawnShell' ./internal/command/...

.PHONY: test-prsplit-all
test-prsplit-all: ## Run ALL PR Split tests including slow/benchmark
	$(GO) -C . test -timeout=20m -count=1 ./internal/command/...

.PHONY: test-prsplit-e2e
test-prsplit-e2e: ## Run only E2E lifecycle tests
	$(GO) -C . test -timeout=300s -count=1 -run 'TestE2E_' ./internal/command/...

.PHONY: cross-build
cross-build: ## Cross-compile for Linux, macOS, Windows
	GOOS=linux GOARCH=amd64 $(GO) -C . build ./...
	GOOS=darwin GOARCH=amd64 $(GO) -C . build ./...
	GOOS=darwin GOARCH=arm64 $(GO) -C . build ./...
	GOOS=windows GOARCH=amd64 $(GO) -C . build ./...

##@ Tilde Path Regression Tests

.PHONY: test-tilde-paths-in-container
test-tilde-paths-in-container: ## Run tilde path tests inside Linux golang container
test-tilde-paths-in-container: SHELL := /bin/bash
test-tilde-paths-in-container:
	@echo "Running tilde path tests in Linux container..."; \
	set -o pipefail; \
	go_version="$$($(GO) -C $(PROJECT_ROOT) mod edit -print | awk '/^go / {print $$2}')"; \
	docker run --rm -v $(PROJECT_ROOT):/work -w /work "golang:$${go_version}" bash -c 'export PATH="/usr/local/go/bin:$$PATH" && export GOFLAGS=-buildvcs=false && make test-tilde-paths' 2>&1 | fold -w 200 | tee $(or $(PROJECT_ROOT),.)/build.log | tail -n 15; \
	exit $${PIPESTATUS[0]}

.PHONY: test-tilde-paths-on-windows
test-tilde-paths-on-windows: ## Run tilde path tests on remote Windows
	hack/run-on-windows.sh moo make test-tilde-paths

.PHONY: commit-task40
commit-task40: ## Commit Task 40: session() wrapper write/resize
	git add internal/builtin/termmux/module.go internal/builtin/termmux/module_test.go
	git commit -F scratch/commit-msg-task40.txt

.PHONY: commit-task51
commit-task51: ## Commit Task 51: fix printf-style log.debug violations
	git add internal/command/pr_split_13_tui.js internal/command/pr_split_16c_tui_handlers_verify.js
	git commit -m "Fix printf-style log.debug calls in verify phase handlers" -m "Replace two log.debug calls using printf-style %s arguments with" -m "structured logging attrs objects. The jsLogDebug signature expects" -m "(msg: string, attrs?: map[string]any) — positional string arguments" -m "caused TypeErrors that silently broke the verify phase state machine." -m "" -m "Also convert messages to lowercase, punctuation-free, event-phrased" -m "format per project structured logging conventions."

.PHONY: commit-meta
commit-meta: ## Commit meta changes (blueprint, config, etc)
	git add blueprint.json config.mk scratch/
	git commit -m "Update blueprint and process artifacts for Tasks 40/51"

.PHONY: commit-task41
commit-task41: ## Commit Task 41: E2E keystroke test
	git add internal/command/pr_split_inline_e2e_test.go
	git commit -m "Add E2E test: keystroke reaches PTY via SessionManager" \
		-m "Exercise the full pr-split command engine stack (PrepareEngine +" \
		-m "setupEngineGlobals + 30 chunked JS scripts) and prove that" \
		-m "session().write('hello') delivers bytes through SessionManager" \
		-m "to a recording StringIOSession mock. Also tests resize and ANSI" \
		-m "escape round-trips." \
		-m "" \
		-m "Includes newPrSplitEvalWithMgr helper that exposes the" \
		-m "SessionManager for Go-side session registration in tests."

.PHONY: test-inline-e2e
test-inline-e2e: ## Run the inline keystroke E2E test with race detector
	go -C . test -race -timeout=120s -count=1 -run TestInlineKeystrokeReachesPTY -v ./internal/command/...

.PHONY: commit-task42
commit-task42: ## Commit Task 42: Fix architecture doc inaccuracies (GAP-H04)
	git add -f scratch/termmux-architecture.md
	git commit -m "Fix 31 inaccuracies in termmux architecture doc" \
		-m "Cross-reference scratch/termmux-architecture.md against actual" \
		-m "source code in internal/termmux/ and correct every factual" \
		-m "discrepancy. Fixes span method signatures, struct fields, state" \
		-m "transitions, shutdown ordering, passthrough mode mechanics," \
		-m "concurrency model, functional option types, and Event.Data" \
		-m "documentation." \
		-m "" \
		-m "Verified: 2 contiguous Rule of Two passes (independent reviewer" \
		-m "cross-referenced all 13 public methods, 35+ struct fields," \
		-m "5 state transitions, worker select loop, both shutdown paths," \
		-m "and passthrough architecture against Go source). Zero remaining" \
		-m "inaccuracies that would mislead an implementer."

.PHONY: commit-meta2
commit-meta2: ## Commit meta changes (blueprint, config, review artifacts) for Tasks 41/42
	git add blueprint.json config.mk
	git add -f scratch/task42-r2-pass1d.md scratch/task42-r2-pass1e.md scratch/task42-r2-pass1f.md scratch/task42-r2-pass2f.md 2>/dev/null || true
	git commit -m "Update blueprint and process artifacts for Tasks 41/42"

.PHONY: commit-meta3
commit-meta3: ## Commit meta changes for Tasks 43-50
	git add blueprint.json config.mk internal/termmux/capture.go
	git add -f WIP.md scratch/task50-r2-pass1.md scratch/task50-r2-pass2.md 2>/dev/null || true
	git commit -m "Update blueprint and process artifacts for Tasks 43-50" \
		-m "Mark Tasks 43-50 as Done in blueprint.json. Add Make targets" \
		-m "for commit-task43 through commit-task49, test helpers, and" \
		-m "session timer management. Update WIP.md progress notes."

.PHONY: commit-task52
commit-task52: ## Commit Task 52: Address error-discarding patterns in SessionManager
	git add internal/termmux/manager.go
	git commit -m "Log all discarded errors in SessionManager worker" \
		-m "Replace every silently-discarded error in manager.go with" \
		-m "structured slog calls:" \
		-m "" \
		-m "- session.Close() errors: slog.Warn (3 sites: unregister," \
		-m "  immediate-exit, shutdown)" \
		-m "- session.Resize() errors: slog.Warn with sessionID + dimensions" \
		-m "- passthrough tee Write() errors: slog.Warn" \
		-m "- VTerm Write() errors: slog.Debug (in-memory, unlikely)" \
		-m "" \
		-m "Removes the _ = id iteration suppression in handleResize since" \
		-m "id is now consumed by the error log. A grep for '_ = ms.session'" \
		-m "and '_, _ =' in manager.go now returns zero results."

.PHONY: commit-task50
commit-task50: ## Commit Task 50: EventBus dropped event metrics
	git add internal/termmux/eventbus.go \
		internal/termmux/eventbus_test.go \
		internal/termmux/manager_test.go \
		internal/builtin/termmux/module.go
	git commit -m "Add dropped event counter to EventBus" \
		-m "Track events that could not be delivered because a subscriber's" \
		-m "channel buffer was full. Uses atomic.Int64 for lock-free reads." \
		-m "" \
		-m "- EventBus.Publish(): increment counter + slog.Debug on drop" \
		-m "- EventBus.DroppedCount(): returns cumulative drop count" \
		-m "- SessionManager.EventsDropped(): delegates to EventBus" \
		-m "- JS binding: eventsDropped() → number" \
		-m "" \
		-m "Four unit tests (InitiallyZero, CountsDropped, MultipleSubscribers," \
		-m "NoDropsWithLargeBuffer) plus one integration test via" \
		-m "SessionManager exercising the full pipeline."

.PHONY: commit-meta4
commit-meta4: ## Commit meta changes for Tasks 50-53
	git add -f WIP.md blueprint.json config.mk
	git rm --cached -f scratch/task42-r2-pass1d.md scratch/task42-r2-pass1e.md \
		scratch/task42-r2-pass1f.md scratch/task42-r2-pass2f.md \
		scratch/task50-r2-pass1.md scratch/task50-r2-pass2.md 2>/dev/null || true
	git add scratch/task53-r2-pass1.md scratch/task53-r2-pass2.md \
		scratch/task52-r2-pass1.md scratch/task52-r2-pass2.md 2>/dev/null || true
	git commit -m "Update blueprint and meta files for Tasks 50-53" \
		-m "Mark Tasks 52 and 53 as Done in blueprint.json. Add Make" \
		-m "targets commit-task50, commit-task53, commit-meta4. Update" \
		-m "WIP.md progress notes. Clean up stale scratch review files."

.PHONY: commit-task57
commit-task57: ## Commit Task 57: Module isolation test for termmux
	git add internal/termmux/isolation_test.go
	git commit -m "Add module-extraction boundary test for termmux" \
		-m "Enforce that internal/termmux/... has zero imports from osm" \
		-m "internal packages outside the termmux subtree. Uses go list" \
		-m "-deps to enumerate all transitive dependencies and rejects" \
		-m "any matching internal/* but not internal/termmux/*." \
		-m "" \
		-m "Dynamic module path discovery via go env GOMOD + go list -m" \
		-m "avoids hardcoded paths. Includes testing.Short() guard and" \
		-m "t.Parallel(). Prevents accidental coupling that would block" \
		-m "extracting termmux as a standalone module."

.PHONY: commit-task56
commit-task56: ## Commit Task 56: Remove transitional CaptureSession methods
	git add internal/termmux/capture.go \
		internal/termmux/capture_test.go \
		internal/termmux/session_test.go \
		internal/builtin/termmux/module.go \
		internal/builtin/termmux/module_capture_test.go
	git commit -m "Remove Target, SetTarget, IsRunning from CaptureSession" \
		-m "Purge transitional methods and their JS bindings that are no" \
		-m "longer used. All JS call sites route through SessionManager" \
		-m "wrappers (tuiMux.session()) for target/identity and liveness" \
		-m "checks. The verify proxy uses isDone(), not isRunning()." \
		-m "" \
		-m "Deleted from CaptureSession: Target(), SetTarget(), IsRunning()" \
		-m "methods plus the target field and kind initialization block." \
		-m "Deleted from WrapCaptureSession: target(), setTarget()," \
		-m "isRunning() JS bindings and the targetFromJS helper." \
		-m "" \
		-m "Updated AUDIT comment (20->17 methods), wrapInteractiveSession" \
		-m "docstring, and module_capture_test.go (method list + absence" \
		-m "checks). Replaced IsRunning() in ConcurrentOutput test with" \
		-m "Done() channel select."

.PHONY: commit-task58
commit-task58: ## Commit Task 58: JS→Go boundary audit + reference doc + tests
	git add docs/reference/termmux-js-api.md \
		internal/builtin/termmux/module_session_test.go
	git commit -m "Add JS→Go API reference doc and SessionManager binding tests" \
		-m "Create docs/reference/termmux-js-api.md documenting every" \
		-m "method exposed by WrapSessionManager (33 methods + session()" \
		-m "wrapper with 8 methods), WrapCaptureSession (17 methods)," \
		-m "wrapInteractiveSession (6 shared base methods), module" \
		-m "exports (18 constants/factories), and the event system." \
		-m "Tables list JS method names, Go counterparts, parameters," \
		-m "return types, and error handling patterns." \
		-m "" \
		-m "Add module_session_test.go with 31 test functions covering" \
		-m "all SessionManager JS binding methods: lifecycle (run/" \
		-m "started/close with both Go and JS entry points), session" \
		-m "management (register/unregister/activate/attach/detach)," \
		-m "state queries, I/O, passthrough/switchTo/fromModel edge" \
		-m "cases, session() wrapper, events, config setters, error" \
		-m "paths, and a comprehensive method presence check."

.PHONY: commit-task60
commit-task60: ## Commit Task 60: Documentation refresh for termmux and pr-split
	git rm docs/reference/mux-architecture.md
	git add docs/README.md docs/scripting.md docs/architecture.md \
		docs/reference/termmux-js-api.md
	git commit -m "Refresh documentation for SessionManager architecture" \
		-m "Delete obsolete docs/reference/mux-architecture.md which" \
		-m "described the removed Mux type. Update docs/README.md to" \
		-m "link to termmux-js-api.md instead." \
		-m "" \
		-m "Update docs/scripting.md: replace stale newMux() module" \
		-m "table entry with newSessionManager() API showing all 33" \
		-m "methods, update CaptureSession section to reflect removal" \
		-m "of screen()/output()/target()/setTarget()/isRunning() and" \
		-m "addition of reader()/readAvailable()/passthrough()/wait()." \
		-m "" \
		-m "Update docs/architecture.md split-view pipeline section:" \
		-m "CaptureSession no longer has VTerm; output flows through" \
		-m "SessionManager worker goroutine with snapshot() for reads." \
		-m "" \
		-m "Clarify termmux-js-api.md removal note: methods moved from" \
		-m "CaptureSession to SessionManager session() wrapper."

.PHONY: commit-task64
commit-task64: ## Commit Task 64: Unicode width handling in VTerm
	git add internal/termmux/vt/screen.go \
		internal/termmux/vt/wide_char_test.go
	git commit -m "Fix wide-character boundary repair in VTerm operations" \
		-m "Add repairWideBoundary helper that clears orphaned wide-char" \
		-m "halves at edges of cell-modifying operations. Wide chars use" \
		-m "two cells (rune + NUL placeholder); operations that split" \
		-m "this pair must blank the orphaned half." \
		-m "" \
		-m "Apply repair to six methods: PutChar (before write), EraseChars" \
		-m "(before erase), InsertChars (cursor + discard boundary)," \
		-m "DeleteChars (cursor + delete boundary), EraseLine (modes 0/1)," \
		-m "EraseDisplay (modes 0/1). Modes 2/3 replace entire rows so" \
		-m "need no boundary repair." \
		-m "" \
		-m "Add 25 tests covering all six operations with wide chars:" \
		-m "cursor-on-placeholder, end-splits-wide-char, consecutive" \
		-m "wide pairs, ASCII regression, and CSI integration tests" \
		-m "that exercise the full UTF-8 → dispatch → repair pipeline."

.PHONY: commit-task62
commit-task62: ## Commit Task 62: Copy/paste support in split-view panes
	git add internal/termmux/vt/vt.go \
		internal/termmux/vt/cursor_test.go \
		internal/termmux/manager.go \
		internal/termmux/manager_test.go \
		internal/builtin/termmux/module.go \
		internal/builtin/os/os.go \
		internal/builtin/os/os_test.go \
		internal/scripting/js_output_api.go \
		internal/scripting/engine_core.go \
		internal/command/pr_split_16f_tui_model.js \
		internal/command/pr_split_16d_tui_handlers_claude.js \
		internal/command/pr_split_16e_tui_update.js \
		internal/command/pr_split_15b_tui_chrome.js
	git commit -m "Add copy/paste and text selection to split-view panes" \
		-m "Implement keyboard and mouse text selection with clipboard" \
		-m "integration across Claude, Output, and Verify split-view" \
		-m "panes." \
		-m "" \
		-m "Go layer:" \
		-m "- VTerm.CursorPosition(): thread-safe cursor accessor" \
		-m "- ScreenSnapshot: add CursorRow/CursorCol fields" \
		-m "- ClipboardPaste(): multi-platform clipboard read (pbpaste," \
		-m "  PowerShell, wl-paste, xclip, xsel) with OSM_CLIPBOARD_PASTE" \
		-m "  env override" \
		-m "- output.fromClipboard JS binding via Engine" \
		-m "" \
		-m "JS layer:" \
		-m "- Selection helpers: start, extend, extract, clear with line" \
		-m "  wrapping and backward-selection normalization" \
		-m "- Keyboard: Shift+Arrow to select, Ctrl+Shift+C/V to copy/" \
		-m "  paste, Escape to clear selection" \
		-m "- Mouse: Shift+Click to start/extend, Shift+Drag to extend" \
		-m "- Rendering: reverse-video highlighting via applySelectionHighlight" \
		-m "  with column-level precision for plain-text panes" \
		-m "" \
		-m "10 new tests across 3 packages (cursor position, clipboard" \
		-m "paste, snapshot cursor fields)."

.PHONY: commit-meta5
commit-meta5: ## Commit meta changes for Tasks 56-58, 60, 64, 65
	git add blueprint.json WIP.md config.mk
	git commit -m "Update blueprint status for tasks 56-58, 60, 64, 65" \
		-m "Mark tasks 56, 57, 58, 60, 64, 65 as Done in blueprint.json." \
		-m "Update WIP.md with completed task list and remaining work." \
		-m "Add commit targets for tasks 56-58, 60, 64 and meta5."

.PHONY: commit-meta6
commit-meta6: ## Commit meta changes for Task 62
	git add blueprint.json WIP.md config.mk
	git add -f scratch/task62-r2-pass1.md scratch/task62-r2-pass2.md 2>/dev/null || true
	git commit -m "Update blueprint and meta files for Task 62" \
		-m "Mark Task 62 as Done in blueprint.json. Add commit-task62" \
		-m "and commit-meta6 targets to config.mk. Update WIP.md."

.PHONY: commit-task53
commit-task53: ## Commit Task 53: Configurable drain timeout in CaptureSession
	git add internal/termmux/capture.go \
		internal/termmux/capture_test.go
	git commit -m "Make drain timeout configurable in CaptureSession" \
		-m "Add DrainTimeout field to CaptureConfig with a default of" \
		-m "5 seconds applied in NewCaptureSession when <= 0. Replace" \
		-m "the hardcoded time.After(5*time.Second) in Close() with" \
		-m "cs.cfg.DrainTimeout." \
		-m "" \
		-m "Two new tests verify the default (5s) and custom timeout" \
		-m "(100ms with sleep-60 process). All existing CaptureConfig" \
		-m "instantiations are compatible via zero-value defaulting."

.PHONY: test-passthrough-resize
test-passthrough-resize: ## Run passthrough resize tests
	go -C . test -race -timeout=60s -count=1 -run 'TestPassthrough_Resize' -v ./internal/termmux/...

.PHONY: commit-task46
commit-task46: ## Commit Task 46: Passthrough StatusBar integration tests
	git add internal/termmux/passthrough_test.go
	git commit -m "Add passthrough StatusBar integration tests" \
		-m "Three new tests cover previously-untested StatusBar code paths:" \
		-m "- ScrollRegionSetup: DECSTBM escape sequence, reset, render" \
		-m "- MouseRouting: SGR mouse click on status bar → ExitToggle" \
		-m "- RenderRestore: VTerm screen restore + post-restore re-render" \
		-m "" \
		-m "All tests use testing.Short() guards, t.Parallel(), and pass" \
		-m "under the race detector."

.PHONY: commit-task47
commit-task47: ## Commit Task 47: Passthrough resize interaction tests
	git add internal/termmux/passthrough_test.go
	git commit -m "Add passthrough resize interaction tests" \
		-m "Two new tests cover the resize-during-passthrough path:" \
		-m "- ResizeDuringPassthrough: mgr.Resize(24,80) propagates to session" \
		-m "- ResizeFnCallback: ResizeFn fires with StatusBar-adjusted dims (23,80)" \
		-m "" \
		-m "Both tests use testing.Short() guards, t.Parallel(), and pass" \
		-m "under the race detector."

.PHONY: commit-task48
commit-task48: ## Commit Task 48: Migrate verify sessions into SessionManager
	git add internal/command/pr_split_13_tui.js \
		internal/command/pr_split_16c_tui_handlers_verify.js \
		internal/command/pr_split_16f_tui_model.js
	git commit -m "Migrate verify sessions into SessionManager" \
		-m "Verify sessions (CaptureSession objects) are now registered with" \
		-m "SessionManager on creation via tuiMux.register(). Screen reads" \
		-m "route through tuiMux.snapshot() (COW snapshots) and cleanup uses" \
		-m "tuiMux.unregister() instead of direct session.close()." \
		-m "" \
		-m "Key changes:" \
		-m "- s.activeVerifySession stores SessionManager sessionID (number)" \
		-m "- _buildVerifyProxy() wraps sessionID with same API as CaptureSession" \
		-m "- getInteractivePaneSession() returns proxy for numbers, raw for objects" \
		-m "- _onToggle uses getInteractivePaneSession instead of direct access" \
		-m "- Persistent shell upgrade: unregister -> re-register flow" \
		-m "- Graceful fallback when tuiMux unavailable (raw session retained)" \
		-m "" \
		-m "All existing verify-related tests pass unmodified."

.PHONY: test-passthrough-statusbar
test-passthrough-statusbar: ## Run passthrough StatusBar tests
	go -C . test -race -timeout=60s -count=1 -run TestPassthroughStatusBar -v ./internal/termmux/...

.PHONY: commit-task45
commit-task45: ## Commit Task 45: Forward all EventBus events through muxEvents bridge
	git add internal/builtin/termmux/module.go
	git commit -m "Forward all 7 EventBus event kinds through muxEvents bridge" \
		-m "The EventBus→muxEvents bridge goroutine previously only forwarded" \
		-m "3 of 7 event types (exit, bell, output). Add cases for the" \
		-m "remaining 4: registered, activated, closed, terminal-resize." \
		-m "" \
		-m "Add 4 new JS event constants, validEvents entries, and EVENT_" \
		-m "exports. Existing event names (exit, bell, output, resize, focus)" \
		-m "are unchanged. EventTerminalResize (terminal-resize) is distinct" \
		-m "from EventResize (resize) which maps to JS SIGWINCH callbacks." \
		-m "" \
		-m "Also adds sessionId metadata to exit and bell event payloads."

.PHONY: commit-task44
commit-task44: ## Commit Task 44: Fix resize path to call SessionManager.Resize()
	git add internal/command/pr_split_16e_tui_update.js
	git commit -m "Sync SessionManager VTerm dimensions on split-view resize" \
		-m "Add tuiMux.resize(paneRows, paneCols) call after per-session PTY" \
		-m "resizes in handleWindowResize. Without this, the SessionManager's" \
		-m "internal VTerms retained stale dimensions, causing childScreen()" \
		-m "and snapshot() to return incorrectly-sized ANSI output." \
		-m "" \
		-m "The call flows through mgr.Resize() which updates termRows/termCols," \
		-m "resizes every session's VTerm, and emits EventResize."

.PHONY: commit-task43
commit-task43: ## Commit Task 43: Fix stale MuxSession doc comment (GAP-M01)
	git add internal/termmux/session.go
	git commit -m "Fix stale MuxSession references in InteractiveSession doc" \
		-m "Replace deleted [MuxSession] type reference with [StringIOSession]" \
		-m "in the concrete types list. Remove stale '(or Attach for Mux" \
		-m "sessions)' parenthetical from the Reader() doc comment. Both" \
		-m "referenced types (CaptureSession, StringIOSession) now exist" \
		-m "in the package."

.PHONY: commit-task49
commit-task49: ## Commit Task 49: Remove VTerm from CaptureSession
	git add internal/termmux/capture.go \
		internal/termmux/session.go \
		internal/termmux/capture_test.go \
		internal/builtin/termmux/module.go \
		internal/builtin/termmux/module_capture_test.go \
		internal/command/pr_split_06b_shell_test.go
	git commit -m "Remove VTerm from CaptureSession" \
		-m "CaptureSession is now a pure PTY forwarder: raw bytes flow" \
		-m "from the PTY through outputCh to Reader() consumers." \
		-m "SessionManager maintains its own VTerm per session, so the" \
		-m "CaptureSession-level VTerm caused redundant double processing." \
		-m "" \
		-m "Key changes:" \
		-m "- Remove term *vt.VTerm field, vt import, cs.term.Write/Resize" \
		-m "- Delete Output() and Screen() methods from CaptureSession" \
		-m "- Remove output()/screen() JS bindings from WrapCaptureSession" \
		-m "- Add readAvailable() non-blocking channel drain to JS wrapper" \
		-m "- Add testOutputCollector helper for Go tests" \
		-m "- Update 5 JS shell tests to use readAvailable()" \
		-m "- Fix all stale VTerm/Output/Screen doc comments"

.PHONY: check-session-timer
check-session-timer: ## Verify session timer and show remaining time
	@START=$$(awk -F= '/^START=/{print $$2}' scratch/session_timer.txt 2>/dev/null); \
	if [ -z "$$START" ]; then echo "NO TIMER FOUND — resetting to NOW"; START=$$(date +%s); printf "START=%s\nDURATION=9h\n" "$$START" > scratch/session_timer.txt; fi; \
	NOW=$$(date +%s); \
	END=$$((START + 9*3600)); \
	ELAPSED=$$(( (NOW - START) / 60 )); \
	REMAINING=$$(( (END - NOW) / 60 )); \
	echo "Session started: $$(date -r $$START '+%Y-%m-%d %H:%M:%S' 2>/dev/null || date -d @$$START '+%Y-%m-%d %H:%M:%S' 2>/dev/null)"; \
	echo "Current time:    $$(date '+%Y-%m-%d %H:%M:%S')"; \
	echo "Elapsed: $${ELAPSED}m / 540m"; \
	echo "Remaining: $${REMAINING}m"; \
	if [ $$REMAINING -le 0 ]; then echo "⚠️  TIME EXCEEDED — session should have ended"; else echo "✅ Time remaining: $${REMAINING} minutes"; fi

.PHONY: reset-session-timer
reset-session-timer: ## Reset the 9-hour session timer to now
	@printf "START=%s\nDURATION=9h\n" "$$(date +%s)" > scratch/session_timer.txt
	@echo "Timer reset to $$(date '+%Y-%m-%d %H:%M:%S')"

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`

endif

