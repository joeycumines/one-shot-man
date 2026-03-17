# Hana may steer you through $(error ...) messages. Be prepared to drop everything and obey immediately...
# RESOLVED: syncMainViewport + prompt anchor issues tracked as T123 and T000 in blueprint.json.

# RESOLVED: Cross-platform build verification tracked as T302b in blueprint.json.

SHELL := /usr/bin/env bash -o pipefail

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1
##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

.PHONY: test-only
test-only: ## Run tests only (no lint) with logging
test-only: SHELL := /bin/bash
test-only:
	@echo "Running tests..."; \
set -o pipefail; \
$(MAKE) test GO_TEST_FLAGS=-timeout=20m 2>&1 | fold -w 200 | tee $(or $(PROJECT_ROOT),$(error))/test.log | tail -n 40; \
exit $${PIPESTATUS[0]}

.PHONY: test-command-unit
test-command-unit: ## Run command package unit tests (new T200+ tests)
test-command-unit: SHELL := /bin/bash
test-command-unit:
	@echo "Running command unit tests..."; \
set -o pipefail; \
$(GO) -C $(PROJECT_ROOT) test -v -timeout=120s -run 'TestViews_NavBar|TestAnchorPipeline_(BestAnchors|PromptOnly|NoPrompt)' ./internal/command/ 2>&1 | tee $(or $(PROJECT_ROOT),$(error))/test-command.log | tail -n 60; \
exit $${PIPESTATUS[0]}

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

.PHONY: record-session-start
record-session-start: ## Record session start time
record-session-start: SHELL := /bin/bash
record-session-start:
	@date +%s > $(PROJECT_ROOT)/.session_start
	@echo "Session started at $$(date -r $$(cat $(PROJECT_ROOT)/.session_start) '+%Y-%m-%d %H:%M:%S')"

.PHONY: check-session-time
check-session-time: ## Check elapsed session time
check-session-time: SHELL := /bin/bash
check-session-time:
	@if [ -f $(PROJECT_ROOT)/.session_start ]; then \
		start=$$(cat $(PROJECT_ROOT)/.session_start); \
		now=$$(date +%s); \
		elapsed=$$((now - start)); \
		hours=$$((elapsed / 3600)); \
		minutes=$$(( (elapsed % 3600) / 60 )); \
		seconds=$$((elapsed % 60)); \
		remaining=$$((32400 - elapsed)); \
		rem_hours=$$((remaining / 3600)); \
		rem_minutes=$$(( (remaining % 3600) / 60 )); \
		echo "Elapsed: $${hours}h $${minutes}m $${seconds}s | Remaining: $${rem_hours}h $${rem_minutes}m (of 9h mandate)"; \
	else \
		echo "No session start recorded. Run 'make record-session-start' first."; \
		exit 1; \
	fi

.PHONY: test-binary-e2e-pty
test-binary-e2e-pty: ## Run PTY binary E2E tests (FullFlow, VerifyPTY, PlanEditor, CancelDuringVerify)
test-binary-e2e-pty: SHELL := /bin/bash
test-binary-e2e-pty:
	@echo "Running PTY binary E2E tests..."; \
set -o pipefail; \
$(GO) -C $(PROJECT_ROOT) test -v -timeout=300s -tags=prsplit_slow -run 'TestBinaryE2E_(FullFlowToExecution|VerifyPTYLive|PlanEditorFlow|CancelDuringVerify)' ./internal/command/ 2>&1 | tee $(or $(PROJECT_ROOT),$(error))/test-e2e-pty.log | tail -n 80; \
exit $${PIPESTATUS[0]}

.PHONY: test-prsplit-views
test-prsplit-views: ## Run PR Split views tests (chunk 15 + chunk 13 view tests)
test-prsplit-views: SHELL := /bin/bash
test-prsplit-views:
	@echo "Running PR Split views tests..."; \
set -o pipefail; \
$(GO) -C $(PROJECT_ROOT) test -v -timeout=300s -run 'TestViews_|TestChunk13_View' ./internal/command/ 2>&1 | tee $(or $(PROJECT_ROOT),$(error))/test-views.log | tail -n 80; \
exit $${PIPESTATUS[0]}

.PHONY: test-prsplit-fast
test-prsplit-fast: ## Fast PR Split tests — excludes slow integration/E2E (no prsplit_slow tag)
test-prsplit-fast: SHELL := /bin/bash
test-prsplit-fast:
	@echo "Running fast PR Split tests (excluding prsplit_slow)..."; \
set -o pipefail; \
start=$$(date +%s); \
$(GO) -C $(PROJECT_ROOT) test -timeout=600s -count=1 ./internal/command/... 2>&1 | fold -w 200 | tail -n 40; \
rc=$$?; \
elapsed=$$(( $$(date +%s) - start )); \
echo "Fast tests completed in $${elapsed}s (exit $$rc)"; \
exit $$rc

.PHONY: test-prsplit-all
test-prsplit-all: ## All PR Split tests — fast + slow (includes prsplit_slow tag)
test-prsplit-all: SHELL := /bin/bash
test-prsplit-all:
	@echo "Running ALL PR Split tests (fast + slow)..."; \
set -o pipefail; \
start=$$(date +%s); \
$(GO) -C $(PROJECT_ROOT) test -timeout=900s -count=1 -tags=prsplit_slow ./internal/command/... 2>&1 | fold -w 200 | tail -n 60; \
rc=$$?; \
elapsed=$$(( $$(date +%s) - start )); \
echo "All tests completed in $${elapsed}s (exit $$rc)"; \
exit $$rc

.PHONY: cross-build
cross-build: ## Verify build succeeds on Linux, macOS, and Windows
cross-build: SHELL := /bin/bash
cross-build:
	@echo "Cross-platform build verification..."; \
set -e; \
for pair in 'linux/amd64' 'darwin/amd64' 'windows/amd64'; do \
	os=$${pair%%/*}; arch=$${pair##*/}; \
	echo "  Building GOOS=$$os GOARCH=$$arch..."; \
	GOOS=$$os GOARCH=$$arch $(GO) -C $(PROJECT_ROOT) build ./... || { echo "FAIL: $$os/$$arch"; exit 1; }; \
	echo "  OK: $$os/$$arch"; \
done; \
echo "All platforms build successfully."

.PHONY: git-stage-all
git-stage-all: ## Stage all changes (git add -A)
	@git -C $(PROJECT_ROOT) add -A && echo "All changes staged."

.PHONY: git-rm-old-chunks
git-rm-old-chunks: ## Delete leftover pre-split chunk files
	@git -C $(PROJECT_ROOT) rm -f internal/command/pr_split_10_pipeline.js 2>/dev/null || true
	@git -C $(PROJECT_ROOT) rm -f internal/command/pr_split_15_tui_views.js 2>/dev/null || true
	@git -C $(PROJECT_ROOT) rm -f internal/command/pr_split_16_tui_core.js 2>/dev/null || true
	@echo "Old chunk files removed."

.PHONY: git-diff-cached
git-diff-cached: ## Show staged diff (for review)
	@git -C $(PROJECT_ROOT) diff --cached --stat

.PHONY: git-commit-t312
git-commit-t312: ## Commit T312 split
	@git -C $(PROJECT_ROOT) commit -m "Split pr_split_10_pipeline.js into 4 chunk files$$(printf '\n\nSplit the 2097-line pipeline chunk into 4 files to keep each under\n1000 lines:\n\n  10a_pipeline_config.js (246 lines)\n    Constants (AUTOMATED_DEFAULTS, SEND_*), pure utility functions\n    (resolveNumber, resolveSendConfig, getCancellationError,\n    classificationToGroups, cleanupExecutor, isTransientError).\n\n  10b_pipeline_send.js (483 lines)\n    PTY send pipeline: captureScreenshot, prompt detection,\n    anchor stability, sendToHandle with chunked writes.\n\n  10c_pipeline_resolve.js (400 lines)\n    IPC wait layer (waitForLogged), heuristicFallback, and\n    resolveConflictsWithClaude with exponential backoff.\n\n  10d_pipeline_orchestrator.js (995 lines)\n    Main automatedSplit orchestrator.\n\nCross-chunk wiring:\n- 10b imports resolveSendConfig, getCancellationError from 10a\n- 10c imports AUTOMATED_DEFAULTS, isTransientError, sendToHandle,\n  resolveNumber from 10a/10b\n- 10d late-binds 9 deps from 10a/10b/10c in automatedSplit\n\nUpdated pr_split.go embed directives and prSplitChunks array.\nUpdated 5 test files with new chunk name arrays.\nOriginal pr_split_10_pipeline.js deleted.')"

.PHONY: git-commit-t315
git-commit-t315: ## Commit T315 design document
	@git -C $(PROJECT_ROOT) commit -m "design(pr-split): T315 test package structure for isolation" -m "Document comprehensive design for PR Split test restructuring." -m "Key design decisions:" -m "- prsplittest helper package at internal/command/prsplittest/" -m "- No import of internal/command (avoids Go import cycle)" -m "- Engine creation via public scripting.NewEngineDetailed()" -m "- Chunk loading via filesystem reads with lexicographic ordering" -m "- Build tags (prsplit_slow) for slow/fast test splitting" -m "- Make targets for isolated test execution" -m "" -m "Design covers 68 test file audit, import cycle analysis, 8-file" -m "package structure with complete export signatures, migration plan" -m "for all 68 files across 4 phases, expected 83% fast-feedback" -m "speedup, and 5 documented blockers/compromises." -m "" -m "Validated against Go build constraints documentation."

.PHONY: git-commit-t316
git-commit-t316: ## Commit T316 prsplittest package
	@git -C $(PROJECT_ROOT) commit -m "test(pr-split): extract prsplittest helper package (T316)" \
		-m "Create internal/command/prsplittest/ (8 files) to provide" \
		-m "reusable test infrastructure that avoids the import cycle" \
		-m "between command → prsplittest → command." \
		-m "" \
		-m "Package API:" \
		-m "  - NewEngine / NewChunkEngine: Goja VM with production-parity" \
		-m "    config via scripting.NewEngineDetailed()" \
		-m "  - NewTUIEngine / NewTUIEngineWithHelpers: full chunk stack" \
		-m "    with TUI mocks injected between chunks 12 and 13" \
		-m "  - Chunk discovery: filesystem glob + lexicographic sort," \
		-m "    cached via sync.Once" \
		-m "  - ChunkNamesThrough / ChunkNamesAfter: prefix-based" \
		-m "    chunk selection (replaces ad-hoc slice constants)" \
		-m "  - SafeBuffer: thread-safe bytes.Buffer for output capture" \
		-m "  - InitTestRepo: git repo scaffolding with functional options" \
		-m "  - GitMockSetupJS / ChunkCompatShim: JS mock constants" \
		-m "" \
		-m "Migrations (4 test files):" \
		-m "  - pr_split_02_grouping_test.go (10 calls)" \
		-m "  - pr_split_07_prcreation_test.go (6 calls)" \
		-m "  - pr_split_08_conflict_test.go (13 calls)" \
		-m "  - pr_split_09_claude_test.go (11 calls)" \
		-m "" \
		-m "Cleanup:" \
		-m "  - Delete leftover pr_split_10_pipeline.js (2097 lines)" \
		-m "    that was already split into 10a-10d in T312" \
		-m "  - Fix claudemux/pr_split_test.go chunk list (10 → 10a-10d)" \
		-m "  - Update ADR-001 chunk table and prompt anchor stability doc" \
		-m "  - Add .deadcodeignore for prsplittest/*"

.PHONY: git-commit-t317
git-commit-t317: ## Commit T317 migration
	@git -C $(PROJECT_ROOT) commit -m "test(pr-split): migrate unit tests to prsplittest helpers (T317)" \
		-m "Migrate 13 unit test files (152 loadChunkEngine + 18" \
		-m "loadPrSplitEngineWithEval call sites) to use the prsplittest" \
		-m "package created in T316." \
		-m "" \
		-m "Migrated files:" \
		-m "  00_core (13), 01_analysis (6), 03_planning (13)," \
		-m "  04_validation (26), 05_execution (7), 06_verification (7)," \
		-m "  10_pipeline (21), 11_utilities (36), 12_exports (4)," \
		-m "  corruption (1), pipeline_smoke (2)," \
		-m "  template_unit (18 — uses NewFullEngine)" \
		-m "" \
		-m "New prsplittest exports:" \
		-m "  - NewFullEngine: loads all chunks + ChunkCompatShim" \
		-m "    (for template tests that reference monolith-era globals)" \
		-m "  - ChunkCompatShim: ~160-line Object.defineProperty proxy" \
		-m "    bridging chunk namespace to pre-split global names" \
		-m "" \
		-m "Cleanup:" \
		-m "  - Delete zombie pr_split_15_tui_views.js (2604 lines)" \
		-m "    and pr_split_16_tui_core.js (5126 lines) that reappeared" \
		-m "    on disk after T310/T311 splits" \
		-m "  - Update config.mk git-rm-old-chunks target" \
		-m "" \
		-m "Remaining loadChunkEngine callers (T318 scope):" \
		-m "  - Definition in 00_core_test.go (kept for 13_tui_test.go)" \
		-m "  - 2 calls in 13_tui_test.go"

.PHONY: git-stage-t318
git-stage-t318: ## Stage T318 TUI test migration files
	@git -C $(PROJECT_ROOT) add \
		internal/command/pr_split_00_core_test.go \
		internal/command/pr_split_13_tui_test.go \
		internal/command/pr_split_15_tui_views_test.go \
		internal/command/pr_split_16_analysis_hang_test.go \
		internal/command/pr_split_16_async_pipeline_test.go \
		internal/command/pr_split_16_auto_split_equiv_test.go \
		internal/command/pr_split_16_bench_test.go \
		internal/command/pr_split_16_claude_attach_test.go \
		internal/command/pr_split_16_config_output_test.go \
		internal/command/pr_split_16_ctrl_bracket_test.go \
		internal/command/pr_split_16_focus_nav_edge_test.go \
		internal/command/pr_split_16_helpers_test.go \
		internal/command/pr_split_16_keyboard_crash_test.go \
		internal/command/pr_split_16_overlays_test.go \
		internal/command/pr_split_16_preexisting_test.go \
		internal/command/pr_split_16_restart_claude_test.go \
		internal/command/pr_split_16_split_mouse_test.go \
		internal/command/pr_split_16_sync_avail_test.go \
		internal/command/pr_split_16_verify_expand_nav_test.go \
		internal/command/pr_split_16_vterm_claude_pane_test.go \
		internal/command/pr_split_16_vterm_key_forwarding_test.go \
		internal/command/pr_split_16_vterm_lifecycle_test.go \
		internal/command/pr_split_autosplit_recovery_test.go \
		internal/command/pr_split_edge_hardening_test.go \
		internal/command/pr_split_mode_autofix_test.go \
		internal/command/pr_split_scope_misc_test.go \
		internal/command/pr_split_tui_hang_test.go \
		internal/command/prsplittest/engine.go \
		internal/command/prsplittest/eval.go \
		internal/command/prsplittest/tui.go
	@echo "T318 files staged."

.PHONY: git-commit-t318
git-commit-t318: ## Commit T318 TUI test migration
	@git -C $(PROJECT_ROOT) commit -m "test(pr-split): migrate TUI tests to prsplittest helpers (T318)" \
		-m "Migrate ~30 TUI and related test files from local loadTUIEngine," \
		-m "loadTUIEngineWithHelpers, loadChunkEngine, and" \
		-m "loadPrSplitEngineWithEval helpers to the prsplittest package." \
		-m "" \
		-m "New prsplittest exports:" \
		-m "  - NewTUIEngineE: returns *Engine with full TUI stack loaded," \
		-m "    allowing access to ScriptingEngine() for event loop/VM" \
		-m "  - MakeEvalJS: exported wrapper for custom-timeout evalJS" \
		-m "    creation from a raw scripting.Engine" \
		-m "" \
		-m "Refactored existing prsplittest:" \
		-m "  - NewTUIEngine: now delegates to NewTUIEngineE" \
		-m "  - NewTUIEngineWithHelpers: now uses NewTUIEngineE + Chunk16Helpers" \
		-m "  - NewEngine: now sets args global for compatibility" \
		-m "" \
		-m "Deleted dead code:" \
		-m "  - loadChunkEngine + makeEvalJS from 00_core_test.go (~170 lines)" \
		-m "  - loadTUIEngine + setupTUIMocks from 13_tui_test.go (~80 lines)" \
		-m "  - loadTUIEngineWithHelpers + chunk16Helpers from 16_helpers_test.go" \
		-m "    (~100 lines)" \
		-m "  - loadTUIEngineRaw from tui_hang_test.go (~85 lines)" \
		-m "  - numVal from 16_helpers_test.go (replaced by prsplittest.NumVal)" \
		-m "" \
		-m "Migration scope (30 files, ~600 call-site replacements):" \
		-m "  Group 1 — loadTUIEngineWithHelpers to NewTUIEngineWithHelpers:" \
		-m "    19 files, 346 call sites" \
		-m "  Group 2 — loadTUIEngine to NewTUIEngine:" \
		-m "    3 files, 212 call sites" \
		-m "  Group 3 — loadPrSplitEngineWithEval to NewFullEngine:" \
		-m "    4 files (autosplit_recovery, edge_hardening, scope_misc," \
		-m "    mode_autofix), 55 call sites" \
		-m "  Special — tui_hang_test.go:" \
		-m "    NewTUIEngineE + MakeEvalJS for event-loop concurrent polling" \
		-m "" \
		-m "Net: 30 files changed, ~710 insertions(+), ~920 deletions(-)"

.PHONY: git-stage-t319
git-stage-t319: ## Stage T319 build tag splitting files
	@git -C $(PROJECT_ROOT) add \
		internal/command/pr_split_03_planning_test.go \
		internal/command/pr_split_06_verification_test.go \
		internal/command/pr_split_autosplit_recovery_test.go \
		internal/command/pr_split_benchmark_test.go \
		internal/command/pr_split_binary_e2e_test.go \
		internal/command/pr_split_bt_test.go \
		internal/command/pr_split_claude_config_test.go \
		internal/command/pr_split_complex_project_test.go \
		internal/command/pr_split_conflict_retry_test.go \
		internal/command/pr_split_corruption_test.go \
		internal/command/pr_split_edge_hardening_test.go \
		internal/command/pr_split_heuristic_run_test.go \
		internal/command/pr_split_integration_test.go \
		internal/command/pr_split_local_integration_test.go \
		internal/command/pr_split_mode_autofix_test.go \
		internal/command/pr_split_prompt_test.go \
		internal/command/pr_split_pty_unix_test.go \
		internal/command/pr_split_session_cancel_test.go \
		internal/command/pr_split_termmux_observation_test.go \
		internal/command/pr_split_test.go \
		internal/command/pr_split_tui_hang_test.go \
		internal/command/pr_split_tui_pty_hang_test.go \
		internal/command/pr_split_tui_subcommands_test.go \
		internal/command/pr_split_wizard_integration_test.go \
		internal/command/pr_split_16_helpers_test.go \
		project.mk \
		config.mk \
		blueprint.json \
		WIP.md
	@echo "T319 files staged."

.PHONY: git-commit-t319
git-commit-t319: ## Commit T319 build tag splitting
	@git -C $(PROJECT_ROOT) commit -m "test(pr-split): isolate slow tests with prsplit_slow build tag (T319)" \
		-m "Add //go:build prsplit_slow tag to 23 slow test files (integration," \
		-m "E2E, recovery, benchmark, binary builds) to enable fast-feedback" \
		-m "iteration via make test-prsplit-fast which excludes them." \
		-m "" \
		-m "Tagged files (19 non-unix + 4 unix):" \
		-m "  Non-unix: pr_split_{03_planning,06_verification,autosplit_recovery," \
		-m "    benchmark,bt,claude_config,complex_project,conflict_retry," \
		-m "    corruption,edge_hardening,heuristic_run,integration," \
		-m "    local_integration,mode_autofix,prompt,session_cancel," \
		-m "    tui_hang,tui_subcommands,wizard_integration}_test.go" \
		-m "  Unix: pr_split_{binary_e2e,tui_pty_hang,termmux_observation," \
		-m "    pty_unix}_test.go (unix && prsplit_slow)" \
		-m "" \
		-m "Relocated shared helpers to pr_split_test.go (untagged):" \
		-m "  initGitRepo, writeFile, gitCmd, escapeJSPath, jsString" \
		-m "  Previously in slow-tagged files, used by fast-path tests." \
		-m "" \
		-m "Build system changes:" \
		-m "  project.mk: GO_FLAGS/STATICCHECK_FLAGS = -tags=prsplit_slow" \
		-m "    (lint/vet/staticcheck see ALL code including slow files)" \
		-m "  config.mk: New targets test-prsplit-fast, test-prsplit-all" \
		-m "    Updated test-binary-e2e-pty with -tags=prsplit_slow" \
		-m "" \
		-m "Performance (internal/command package):" \
		-m "  Full suite: 1103s" \
		-m "  Fast target: 581s (47% reduction)" \
		-m "  Note: 300s target not achievable — non-pr-split tests" \
		-m "  (PTY, 180+ JS engine inits) consume ~400s."

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
