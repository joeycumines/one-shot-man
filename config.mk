# This is an example config.mk file, to support local customizations.

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1
##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS := all GO_TEST_FLAGS=-timeout=30m GO_FLAGS=-p=1

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

.PHONY: test-prsplit-binary
test-prsplit-binary: ## Run real-binary E2E tests for pr-split (compiles osm, runs subprocess)
	$(GO) -C . test -timeout=10m -count=1 -run 'TestBinaryE2E_' ./internal/command/...

.PHONY: test-prsplit-agent-mock
test-prsplit-agent-mock: ## Run pr-split tests with mockagent binary (no real AI required)
	$(GO) -C . test -timeout=10m -count=1 -run 'TestBinaryE2E_Agent|TestIntegration_MockMCP|TestMockMCP' ./internal/command/...

.PHONY: test-prsplit-recovery
test-prsplit-recovery: ## Run pr-split resume/error-recovery/cleanup tests
	$(GO) -C . test -timeout=10m -count=1 -run 'TestBinaryE2E_(Rerun|Deleted|DryRun|FullPipeline|Cleanup|ConfigFile|JSONOutput|ComplexProject|Compilable)' ./internal/command/...

.PHONY: test-prsplit-strategies
test-prsplit-strategies: ## Run pr-split tests for all strategy variations
	$(GO) -C . test -timeout=10m -count=1 -run 'TestBinaryE2E_Strategy' ./internal/command/...

.PHONY: cross-build
cross-build: ## Cross-compile for Linux, macOS, Windows
	GOOS=linux GOARCH=amd64 $(GO) -C . build ./...
	GOOS=darwin GOARCH=amd64 $(GO) -C . build ./...
	GOOS=darwin GOARCH=arm64 $(GO) -C . build ./...
	GOOS=windows GOARCH=amd64 $(GO) -C . build ./...

##@ JS Compliance Suite

.PHONY: test-jscompliance
test-jscompliance: ## Run the FAST tier of the JS Runtime compliance suite (always-on subset, excludes fork-blocked)
	$(GO) -C . test -race -count=1 -timeout=300s \
		-run 'TestHarness|TestEngine_Integration|TestModuleContract|TestESM|TestResolution|TestSecurity|TestGlobalSurface|TestConsole|TestCoreES$$|TestCorePromises|TestCoreMicrotask|TestCoreTimers|TestCoreAbort$$' \
		./internal/jscompliance/...

.PHONY: test-jscompliance-fork-blocked
test-jscompliance-fork-blocked: ## Run ONLY the fork-blocked compliance tests (expected to FAIL)
	JS_COMPLIANCE_FORK_BLOCKED=1 $(GO) -C . test -race -count=1 -timeout=60s \
		-run 'TestCoreES_ForkBlocked' \
		./internal/jscompliance/...

.PHONY: test-jscompliance-all
test-jscompliance-all: ## Run the FULL JS Runtime compliance suite (fast + slow behavioral tiers, excludes fork-blocked via skip)
	$(GO) -C . test -race -count=1 -timeout=20m ./internal/jscompliance/...

.PHONY: test-test262
test-test262: ## Run test262 quantified suite (fast tier, go:embed, 1000+ cases)
	$(GO) -C . test -race -count=1 -timeout=300s -run 'TestTest262' ./internal/jscompliance/test262/...

.PHONY: test-goja-compat
test-goja-compat: ## Run goja compat quantified suite (slow tier, 500+ cases)
	$(GO) -C . test -race -count=1 -timeout=300s -run 'TestGojaCompat' ./internal/jscompliance/goja_compat/...

.PHONY: fuzz
fuzz: ## Run fuzz targets for 30s each
	$(GO) -C . test -run Fuzz -fuzz=. -fuzztime=30s ./internal/jscompliance/...

.PHONY: report
report: ## Generate quantified compliance report (test262 + goja compat vs goja baseline)
	@mkdir -p scratch
	@$(GO) -C . test -run TestTest262 -count=1 -timeout=300s ./internal/jscompliance/test262/... -json 2>&1 | tee scratch/report-test262-raw.json | tail -n 5
	@$(GO) -C . test -run TestGojaCompat -count=1 -timeout=300s ./internal/jscompliance/goja_compat/... -json 2>&1 | tee scratch/report-goja-compat-raw.json | tail -n 5
	@$(GO) -C . run ./internal/jscompliance/report/... 2>&1 | tee scratch/report.json | tail -n 20
	@echo "report generated: scratch/report.json scratch/report.md"

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
