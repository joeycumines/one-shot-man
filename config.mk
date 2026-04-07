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

.PHONY: test-termmux
test-termmux: ## Run all termmux tests including slow passthrough tests (with race detection)
	$(GO) -C . test -race -v -count=1 -timeout=5m ./internal/termmux/...

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

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`

endif

