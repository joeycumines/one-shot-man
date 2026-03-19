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

.PHONY: record-session-start
record-session-start: ## Record session start timestamp
	@date +%s > $(or $(PROJECT_ROOT),$(error PROJECT_ROOT required))/session-start.txt
	@echo "Session started at $$(date -r $$(cat $(PROJECT_ROOT)/session-start.txt) '+%Y-%m-%d %H:%M:%S')"
	@echo "End target: $$(date -r $$(( $$(cat $(PROJECT_ROOT)/session-start.txt) + 32400 )) '+%Y-%m-%d %H:%M:%S') (9 hours)"

.PHONY: check-session-time
check-session-time: ## Show elapsed/remaining time for 9-hour session
check-session-time: SHELL := /bin/bash
check-session-time:
	@if [ ! -f "$(PROJECT_ROOT)/session-start.txt" ]; then \
	    echo "ERROR: No session-start.txt found. Run 'make record-session-start' first."; \
	    exit 1; \
	fi; \
	start=$$(cat $(PROJECT_ROOT)/session-start.txt); \
	now=$$(date +%s); \
	elapsed=$$((now - start)); \
	remaining=$$((32400 - elapsed)); \
	elapsed_h=$$((elapsed / 3600)); elapsed_m=$$(( (elapsed % 3600) / 60 )); \
	if [ $$remaining -gt 0 ]; then \
	    remain_h=$$((remaining / 3600)); remain_m=$$(( (remaining % 3600) / 60 )); \
	    echo "Elapsed: $${elapsed_h}h $${elapsed_m}m | Remaining: $${remain_h}h $${remain_m}m"; \
	else \
	    over=$$(( -remaining )); over_h=$$((over / 3600)); over_m=$$(( (over % 3600) / 60 )); \
	    echo "Elapsed: $${elapsed_h}h $${elapsed_m}m | OVERTIME: $${over_h}h $${over_m}m (session ended)"; \
	fi

.PHONY: test-prsplit-fast
test-prsplit-fast: ## Run PR Split tests excluding slow tag (fast feedback loop)
	$(GO) test -timeout=600s -race ./internal/command/... 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-prsplit-all
test-prsplit-all: ## Run ALL PR Split tests including slow/E2E
	$(GO) test -timeout=900s -race -tags=prsplit_slow ./internal/command/... 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-prsplit-e2e
test-prsplit-e2e: ## Run only E2E PR Split tests (slow tag)
	$(GO) test -timeout=300s -race -tags=prsplit_slow ./internal/command/... -run 'TestBinaryE2E' 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-run
test-run: ## Run specific test(s): make test-run T=TestFoo
	$(GO) test -timeout=300s -race -v ./internal/command/... -run '$(T)' 2>&1 | fold -w 200 | tail -n 60

.PHONY: test-statusbar
test-statusbar: ## Run status bar and help overlay tests
	$(GO) test -timeout=300s -race -v ./internal/command/... -run 'TestStatusBar|TestHelpOverlay|TestChunk13_RenderStatusBar' 2>&1 | fold -w 200 | tail -n 60

.PHONY: test-verify-handlers
test-verify-handlers: ## Run verify handler and lifecycle tests
	$(GO) test -timeout=300s -race -v ./internal/command/... -run 'TestVerify|TestExecScreen|TestBranchBuilding' 2>&1 | fold -w 200 | tail -n 60

.PHONY: test-views
test-views: ## Run TUI view rendering tests
	$(GO) test -timeout=300s -race -v ./internal/command/... -run 'TestViews|TestChunk13_View' 2>&1 | fold -w 200 | tail -n 60

.PHONY: cross-build
cross-build: ## Build for Linux, macOS, and Windows
	@echo "Building for linux/amd64..."; GOOS=linux GOARCH=amd64 $(GO) build ./... 2>&1 | tail -n 5
	@echo "Building for darwin/amd64..."; GOOS=darwin GOARCH=amd64 $(GO) build ./... 2>&1 | tail -n 5
	@echo "Building for windows/amd64..."; GOOS=windows GOARCH=amd64 $(GO) build ./... 2>&1 | tail -n 5
	@echo "Cross-build complete."

.PHONY: git-add-commit
git-add-commit: ## Stage and commit T367 t.Parallel
	cd $(or $(PROJECT_ROOT),$(error PROJECT_ROOT required)) && \
	git add -A && \
	git commit -m 'test(pr-split): add t.Parallel() to 270 non-dependent unit tests' \
	  -m 'Add t.Parallel() to 270 tests across 16 test files to improve' \
	  -m 'test parallelism and enable race condition detection during' \
	  -m 'concurrent execution.' \
	  -m '' \
	  -m 'Tier 1 (9 files, 241 tests):' \
	  -m '  - pr_split_13_tui_test.go: 94 tests (per-test TUI engine)' \
	  -m '  - pr_split_11_utilities_test.go: 36 tests (pure functions)' \
	  -m '  - pr_split_04_validation_test.go: 26 tests (pure validation)' \
	  -m '  - pr_split_cmd_meta_test.go: 13 tests (flag/metadata tests)' \
	  -m '  - pr_split_00_core_test.go: 13 tests (core chunk tests)' \
	  -m '  - pr_split_08_conflict_test.go: 13 tests (conflict resolution)' \
	  -m '  - pr_split_03_planning_test.go: 13 tests (planning chunk)' \
	  -m '  - pr_split_02_grouping_test.go: 10 tests (grouping chunk)' \
	  -m '' \
	  -m 'Tier 2 (7 files, 29 tests):' \
	  -m '  - pr_split_09_claude_test.go: 8 tests' \
	  -m '  - pr_split_bt_test.go: 8 tests' \
	  -m '  - pr_split_10_pipeline_test.go: 7 tests' \
	  -m '  - pr_split_06_verification_test.go: 7 tests' \
	  -m '  - pr_split_01_analysis_test.go: 6 tests' \
	  -m '  - pr_split_07_prcreation_test.go: 6 tests' \
	  -m '  - pr_split_scope_misc_test.go: 6 tests' \
	  -m '  - pr_split_12_exports_test.go: 4 tests' \
	  -m '' \
	  -m 'Intentional exclusions:' \
	  -m '  - pr_split_tui_subcommands_test.go: uses os.Chdir via' \
	  -m '    chdirTestPipeline (process-global, not parallel-safe)' \
	  -m '  - TestParseClaudeEnv_MalformedInput: mutates slog.SetDefault' \
	  -m '' \
	  -m 'Verified: go test -race -count=3 passes clean (128s, zero races).'

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
