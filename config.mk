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

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`

endif

