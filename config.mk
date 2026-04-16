# Hana may steer you through $(error ...) messages. Be prepared to drop everything and obey immediately...

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

.PHONY: test-js-stack-full
test-js-stack-full: ## Run full tests for the JS command layer and bubbletea bridge
test-js-stack-full: SHELL := /bin/bash
test-js-stack-full:
	@echo "Running JS stack verification. Full log: $(PROJECT_ROOT)/build.log"; \
	set -o pipefail; \
	$(GO) -C . test -count=1 -timeout=20m ./internal/builtin/bubbletea/... ./internal/command/... ./internal/scripting/... \
	2>&1 | fold -w 200 | tee $(PROJECT_ROOT)/build.log | tail -n 40; \
	exit $${PIPESTATUS[0]}

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

.PHONY: test-pinned-session-helpers
test-pinned-session-helpers: ## Run focused pinned-session helper cleanup tests
	$(GO) -C . test -count=1 -run 'TestSessionManager_LastActivityMs|TestSessionManager_LastActivityMsWithSession|TestSessionManager_LastActivityMsExplicitSessionID' ./internal/builtin/termmux/...
	$(GO) -C . test -v -timeout=600s -count=1 -run 'TestAnchorPipeline_(CaptureScreenshot|CaptureInputAnchors|NoPromptMarker_HardFailure|BestAnchorsStateFallback|PromptOnlyFallback|SendToHandle_)|TestChunk14b_(GetActivityInfo|GetLastOutputLines|RenderHudStatusLine|RenderHudStatusLine_Truncation)|TestChunk16e_|TestChunk16_T46_|TestChunk16_T393_|TestClaudeLifecycle_|TestIntegration_SendToHandle_(ObservedSubmissionRetry|ObservedSubmissionFailure|PromptReadyTimeout|PromptSetupBlocker|PromptReadyDelayed)' ./internal/command/...

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

.PHONY: test-task9
test-task9: ## Run Task 9 affected tests (lifecycle, polling, constants, question detection)
	go -C . test -short -timeout=120s -count=1 -run 'TestChunk15a_TUIConstantsStructure|TestChunk16_PollClaudeScreenshot|TestChunk16_T46_PollDetectsQuestion|TestChunk16_T393_QuestionDetected|TestChunk16_VTerm_PollScreenshot|TestChunk16_VTerm_PollClaudeScreenshot_DrainsMuxEvents|TestChunk16_VTerm_FullRenderPipeline_MuxToView|TestClaudeLifecycle' -v ./internal/command/...

.PHONY: test-lifecycle
test-lifecycle: ## Run Claude lifecycle tests (no -short, spawns JS engines)
	go -C . test -timeout=120s -count=1 -run 'TestClaudeLifecycle' -v ./internal/command/...

.PHONY: test-persistence
test-persistence: ## Run Task 10 persistence truthfulness tests
	go -C . test -timeout=120s -count=1 -run 'TestPersistence_' -v ./internal/command/...

.PHONY: session-timer-start
session-timer-start: ## Record the session start epoch
	@echo "$$(date +%s)" > scratch/session_timer.txt
	@echo "Session started at $$(date '+%Y-%m-%d %H:%M:%S')"
	@echo "Session ends at $$(date -v+9H '+%Y-%m-%d %H:%M:%S' 2>/dev/null || date -d '+9 hours' '+%Y-%m-%d %H:%M:%S' 2>/dev/null)"

.PHONY: session-timer-check
session-timer-check: ## Check remaining session time
	@start=$$(cat scratch/session_timer.txt 2>/dev/null || echo 0); \
	now=$$(date +%s); \
	elapsed=$$(( (now - start) / 60 )); \
	remaining=$$(( (9 * 60) - elapsed )); \
	echo "Elapsed: $${elapsed}m | Remaining: $${remaining}m"

.PHONY: git-stage
git-stage: ## Stage files: gmake git-stage F="file1 file2"
git-stage: SHELL := /bin/bash
git-stage:
	@git add $(F)
	@git diff --staged --stat

.PHONY: git-commit-file
git-commit-file: ## Commit staged changes from message file: gmake git-commit-file MSG=path/to/msg.txt
git-commit-file: SHELL := /bin/bash
git-commit-file:
	@git commit -F $(MSG)

.PHONY: git-status
git-status: ## Show git status and diff stat
git-status: SHELL := /bin/bash
git-status:
	@git status --short; echo "---"; git diff --stat

.PHONY: git-diff
git-diff: ## Show full git diff
git-diff: SHELL := /bin/bash
git-diff:
	@git diff

.PHONY: git-info
git-info: ## Show current branch, trunk ref, and diff stats
git-info: SHELL := /bin/bash
git-info:
	@echo "=== BRANCH ==="; \
	git branch --show-current; \
	echo "=== TRUNK REF ==="; \
	git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || echo "origin/HEAD not set"; \
	echo "=== RECENT COMMITS (20) ==="; \
	git log --oneline -20; \
	echo "=== COMMITS AHEAD OF TRUNK ==="; \
	trunkref="$$(git symbolic-ref --quiet --short refs/remotes/origin/HEAD 2>/dev/null || echo origin/main)"; \
	git log --oneline "$$trunkref..HEAD" 2>/dev/null || echo "could not diff against trunk"; \
	echo "=== DIFF STAT VS TRUNK ==="; \
	git diff --stat "$$trunkref...HEAD" 2>/dev/null || echo "could not stat diff"

.PHONY: do-commit-task2
do-commit-task2: ## Stage and commit Task 2: PTY resize delegation
do-commit-task2: SHELL := /bin/bash
do-commit-task2:
	git add \
		internal/builtin/claudemux/provider.go \
		internal/builtin/claudemux/claude_code.go \
		internal/builtin/claudemux/module.go \
		internal/builtin/claudemux/provider_test.go \
		internal/builtin/claudemux/coverage_gaps_test.go \
		internal/builtin/termmux/module.go \
		internal/termmux/manager.go \
		internal/termmux/manager_test.go \
		internal/termmux/stringio_session.go \
		internal/termmux/stringio_session_test.go \
		internal/command/pr_split_09_claude.js
	git diff --staged --stat
	git commit -F scratch/commit-msg-task2.txt

.PHONY: stage-task4
stage-task4: ## Stage Task 4 files: Claude SessionID capture
stage-task4: SHELL := /bin/bash
stage-task4:
	@echo "=== Unstaging non-Task-4 files ==="
	git reset HEAD internal/scripting/engine_core.go 2>/dev/null || true
	git reset HEAD internal/scripting/sentinel_drain_test.go 2>/dev/null || true
	git reset HEAD blueprint.json 2>/dev/null || true
	@echo "=== Staging Task 4 Go + JS production files ==="
	git add \
		internal/builtin/termmux/module.go \
		internal/builtin/termmux/module_session_test.go \
		internal/command/pr_split_10b_pipeline_send.js \
		internal/command/pr_split_10d_pipeline_orchestrator.js \
		internal/command/pr_split_13_tui.js \
		internal/command/pr_split_14b_tui_commands_ext.js \
		internal/command/pr_split_16c_tui_handlers_verify.js \
		internal/command/pr_split_16d_tui_handlers_claude.js
	@echo "=== Staging Task 4 test mock updates ==="
	git add \
		internal/command/pr_split_16_claude_attach_test.go \
		internal/command/pr_split_16_keyboard_crash_test.go \
		internal/command/pr_split_16_vterm_claude_pane_test.go \
		internal/command/pr_split_16_vterm_key_forwarding_test.go \
		internal/command/pr_split_16_vterm_lifecycle_test.go \
		internal/command/pr_split_16e_unit_test.go
	@echo "=== Staged diff stat ==="
	git diff --staged --stat
	@echo "=== Verify no unwanted files ==="
	@if git diff --staged --name-only | grep -qE 'engine_core|sentinel_drain|blueprint'; then \
		echo "ERROR: unwanted files staged!"; exit 1; \
	else \
		echo "OK: no unwanted files staged"; \
	fi

.PHONY: do-commit-task4
do-commit-task4: ## Commit Task 4: Claude SessionID capture
do-commit-task4: SHELL := /bin/bash
do-commit-task4:
	git diff --staged --stat
	git commit -F scratch/commit-msg-task4.txt

.PHONY: stage-task6
stage-task6: ## Stage Task 6 files: trunk gap evidence pack
stage-task6: SHELL := /bin/bash
stage-task6:
	git add -f scratch/pr-split-trunk-gap-audit.md
	git add blueprint.json
	git diff --staged --stat

.PHONY: do-commit-task6
do-commit-task6: ## Commit Task 6: trunk gap evidence pack
do-commit-task6: SHELL := /bin/bash
do-commit-task6:
	git commit -F scratch/commit-msg-task6.txt

.PHONY: test-strategy-fixtures
test-strategy-fixtures: ## Run fixture-backed strategy quality tests
	$(GO) -C . test -v -timeout=300s -count=1 -run 'TestFixture_' ./internal/command/...

.PHONY: stage-task7
stage-task7: ## Stage Task 7 files: fixture-backed strategy tests
stage-task7: SHELL := /bin/bash
stage-task7:
	git add \
		internal/command/pr_split_02_strategy_fixture_test.go \
		internal/command/testdata/fixtures/strategy/multi-directory-go.json \
		internal/command/testdata/fixtures/strategy/generated-file-churn.json \
		internal/command/testdata/fixtures/strategy/rename-heavy.json \
		internal/command/testdata/fixtures/strategy/mixed-docs-code.json \
		internal/command/testdata/fixtures/strategy/large-monorepo.json \
		blueprint.json
	git diff --staged --stat

.PHONY: do-commit-task7
do-commit-task7: ## Commit Task 7: fixture-backed strategy tests
do-commit-task7: SHELL := /bin/bash
do-commit-task7:
	git commit -F scratch/commit-msg-task7.txt

.PHONY: squash-task7
squash-task7: ## Squash last 2 commits (Task 7 code + blueprint finalize)
squash-task7:
	git reset --soft HEAD~2 && git commit -F scratch/commit-msg-task7.txt

.PHONY: test-pinned-routing
test-pinned-routing: ## Run Task 8 pinned routing tests
	$(GO) -C . test -v -timeout=300s -count=1 -run 'TestPinnedSession|TestCtrlCombo|TestReservedKey|TestMouseEvent|TestVerifyPane|TestSpecialKey|TestFocusChange' ./internal/command/...

.PHONY: stage-task8
stage-task8: ## Stage Task 8 files
	git add \
		internal/command/pr_split_tui_pinned_routing_test.go \
		blueprint.json
	git diff --staged --stat

.PHONY: do-commit-task8
do-commit-task8: ## Commit Task 8
do-commit-task8: SHELL := /bin/bash
do-commit-task8:
	git commit -F scratch/commit-msg-task8.txt

.PHONY: stage-task9
stage-task9: ## Stage Task 9 files: Claude lifecycle events
stage-task9: SHELL := /bin/bash
stage-task9:
	git add \
		internal/command/pr_split_15a_tui_styles.js \
		internal/command/pr_split_15a_unit_test.go \
		internal/command/pr_split_15b_tui_chrome.js \
		internal/command/pr_split_16d_tui_handlers_claude.js \
		internal/command/pr_split_16e_tui_update.js \
		internal/command/pr_split_tui_claude_lifecycle_test.go \
		blueprint.json
	git diff --staged --stat

.PHONY: do-commit-task9
do-commit-task9: ## Commit Task 9: Claude lifecycle events
do-commit-task9: SHELL := /bin/bash
do-commit-task9:
	git commit -F scratch/commit-msg-task9.txt

.PHONY: stage-task10
stage-task10: ## Stage Task 10 files: persistence truthfulness
stage-task10: SHELL := /bin/bash
stage-task10:
	git add \
		blueprint.json \
		config.mk \
		internal/command/pr_split_15b_tui_chrome.js \
		internal/command/pr_split_16c_tui_handlers_verify.js \
		internal/command/pr_split_16f_tui_model.js \
		internal/command/pr_split_16g_persistence.js \
		internal/command/pr_split_tui_persistence_truthful_test.go \
		internal/command/pr_split_tui_pinned_routing_test.go
	git diff --staged --stat

.PHONY: do-commit-task10
do-commit-task10: ## Commit Task 10: persistence truthfulness
do-commit-task10: SHELL := /bin/bash
do-commit-task10:
	git commit -F /tmp/task10-commit-msg.txt
	git log --oneline -1

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`

endif
