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

##@ Test Targets

.PHONY: test-prsplit-fast
test-prsplit-fast: ## Run PR Split tests in fast mode (skips slow/E2E via -short)
	$(GO) test -timeout=600s -race -short ./internal/command/... 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-prsplit-all
test-prsplit-all: ## Run ALL PR Split tests including slow/E2E
	$(GO) test -timeout=900s -race ./internal/command/... 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-prsplit-e2e
test-prsplit-e2e: ## Run only E2E PR Split tests
	$(GO) test -timeout=300s -race ./internal/command/... -run 'TestBinaryE2E' 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-osm-cmd
test-osm-cmd: ## Run all cmd/osm tests
	$(GO) test -timeout=120s -race -v ./cmd/osm/... 2>&1 | fold -w 200 | tail -n 80

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

.PHONY: test-key-forwarding
test-key-forwarding: ## Run key forwarding and reserved keys tests
	$(GO) test -timeout=120s -race -run 'TestChunk16_VTerm_KeyToTermBytes|TestKeyToTermBytes_SpecialKeys_T386|TestInteractiveReservedKeys_T386' ./internal/command/... -v 2>&1 | tail -n 50

.PHONY: test-ask-claude
test-ask-claude: ## Run Ask Claude and question detection tests
	$(GO) test -timeout=120s -race -run 'TestChunk16_ClaudeConvo|TestChunk16_T46|TestChunk16_Claude|TestConfirmCancel|TestChunk16_T393' ./internal/command/... -v 2>&1 | tail -n 60

##@ Build Targets

.PHONY: build-cross-platform
build-cross-platform: ## Verify cross-platform builds (linux, darwin, windows)
	@echo "=== Building for linux/amd64 ==="
	GOOS=linux GOARCH=amd64 $(GO) build -o /dev/null ./cmd/osm/
	@echo "=== Building for darwin/amd64 ==="
	GOOS=darwin GOARCH=amd64 $(GO) build -o /dev/null ./cmd/osm/
	@echo "=== Building for darwin/arm64 ==="
	GOOS=darwin GOARCH=arm64 $(GO) build -o /dev/null ./cmd/osm/
	@echo "=== Building for windows/amd64 ==="
	GOOS=windows GOARCH=amd64 $(GO) build -o /dev/null ./cmd/osm/
	@echo "=== Vet for linux/amd64 ==="
	GOOS=linux GOARCH=amd64 $(GO) vet ./...
	@echo "=== Vet for windows/amd64 ==="
	GOOS=windows GOARCH=amd64 $(GO) vet ./...
	@echo "=== All cross-platform builds + vet succeeded ==="

##@ Utility Targets

.PHONY: git-amend
git-amend: ## Amend last commit with staged changes (no message edit)
git-amend: SHELL := /bin/bash
git-amend:
	cd $(PROJECT_ROOT) && git add -A && git commit --amend --no-edit && git log --oneline -1

.PHONY: git-commit-file
git-commit-file: ## Stage all and commit using .commit-msg.txt, then clean up
git-commit-file: SHELL := /bin/bash
git-commit-file:
	cd $(PROJECT_ROOT) && git add -A && git commit -F .commit-msg.txt && rm -f .commit-msg.txt && git log --oneline -1

.PHONY: git-commit-staged
git-commit-staged: ## Commit only staged files using .commit-msg.txt
git-commit-staged: SHELL := /bin/bash
git-commit-staged:
	cd $(PROJECT_ROOT) && git commit -F .commit-msg.txt && rm -f .commit-msg.txt && git log --oneline -1

.PHONY: clean-gocache
clean-gocache: ## Remove task-specific Go caches (.gocache-*)
	rm -rf $(PROJECT_ROOT)/.gocache-*

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
