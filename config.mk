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
$(GO) -C $(PROJECT_ROOT) test -v -timeout=300s -run 'TestBinaryE2E_(FullFlowToExecution|VerifyPTYLive|PlanEditorFlow|CancelDuringVerify)' ./internal/command/ 2>&1 | tee $(or $(PROJECT_ROOT),$(error))/test-e2e-pty.log | tail -n 80; \
exit $${PIPESTATUS[0]}

.PHONY: test-prsplit-views
test-prsplit-views: ## Run PR Split views tests (chunk 15 + chunk 13 view tests)
test-prsplit-views: SHELL := /bin/bash
test-prsplit-views:
	@echo "Running PR Split views tests..."; \
set -o pipefail; \
$(GO) -C $(PROJECT_ROOT) test -v -timeout=300s -run 'TestViews_|TestChunk13_View' ./internal/command/ 2>&1 | tee $(or $(PROJECT_ROOT),$(error))/test-views.log | tail -n 80; \
exit $${PIPESTATUS[0]}

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

.PHONY: git-rm-old-chunk16
git-rm-old-chunk16: ## Delete the original pr_split_16_tui_core.js (T311 split)
	@git -C $(PROJECT_ROOT) rm -f internal/command/pr_split_16_tui_core.js 2>/dev/null || rm -f $(PROJECT_ROOT)/internal/command/pr_split_16_tui_core.js
	@echo "Old chunk 16 file removed."

.PHONY: git-diff-cached
git-diff-cached: ## Show staged diff (for review)
	@git -C $(PROJECT_ROOT) diff --cached --stat

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
