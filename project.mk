.DEFAULT_GOAL := all

GO_TOOLS ?= $(filter-out $(GO_PKG_GRIT) $(GO_PKG_BETTERALIGN),$(GO_TOOLS_DEFAULT))
GO_MODULE_SLUGS_USE_DEADCODE ?= $(GO_MODULE_SLUGS)
GO_MODULE_SLUGS_NO_BETTERALIGN ?= $(GO_MODULE_SLUGS)
GO_MODULE_PATHS_EXCLUDE_PATTERNS ?= ./scratch/%
DEADCODE_IGNORE_PATTERNS_FILE ?= .deadcodeignore
DEADCODE_ERROR_ON_UNIGNORED ?= true
GO_TEST_FLAGS ?= -timeout=30m
CATALOG_API_PKG ?= ./internal/userk8s/api/v1alpha1/...
CATALOG_CRD_DIR ?= internal/userk8s/api/config/crd

all: check-crds

##@ Project Targets

.PHONY: generate-tapes-and-gifs
generate-tapes-and-gifs: ## Generate all recording tapes and GIFs
	@echo "Generating all recording tapes and GIFs..."
	$(GO) -C $(PROJECT_ROOT)/internal/scripting test -v -count=1 -timeout=10m -run "^TestRecording_" -record -execute-vhs

.PHONY: integration-test-prsplit
integration-test-prsplit: ## Run pr-split integration tests with a real agent (requires agent infrastructure)
integration-test-prsplit: AGENT_COMMAND ?=
integration-test-prsplit: AGENT_ARGS ?=
integration-test-prsplit: INTEGRATION_MODEL ?= minimax-m2.5:cloud
integration-test-prsplit: PRSPLIT_TEST_RUN ?= TestIntegration_(.*Agent|AutoSplitComplex|PrSplit_VTerm)
integration-test-prsplit:
	$(GO) test -race -count=1 -timeout=15m \
		./internal/command/... \
		-run '$(PRSPLIT_TEST_RUN)' \
		-integration \
		-agent-command=$(AGENT_COMMAND) \
		$(foreach arg,$(AGENT_ARGS),-agent-arg=$(arg)) \
		-integration-model=$(INTEGRATION_MODEL)

.PHONY: integration-test-prsplit-mcp
integration-test-prsplit-mcp: ## Run pr-split MCP mock integration tests (no real AI required)
	$(GO) test -race -v -count=1 -timeout=10m \
		./internal/command/... -run 'TestIntegration_(AutoSplitMockMCP|MockMCP_)'

.PHONY: integration-test-termmux
integration-test-termmux: ## Run termmux integration tests with real PTY processes
	$(GO) test -race -v -count=1 -timeout=5m -run 'TestIntegration_' ./internal/termmux/... ./internal/termmux/ptyio/...

.PHONY: bench-termmux
bench-termmux: ## Run termmux benchmarks
	$(GO) test -bench=. -benchmem -run='^$$' ./internal/termmux/...

.PHONY: fuzz-termmux
fuzz-termmux: ## Run termmux fuzz tests (30s each, sequential)
fuzz-termmux: FUZZTIME ?= 30s
fuzz-termmux:
	$(GO) test -fuzz=FuzzParser -fuzztime=$(FUZZTIME) ./internal/termmux/vt/...
	$(GO) test -fuzz=FuzzVTermWrite -fuzztime=$(FUZZTIME) ./internal/termmux/vt/...
	$(GO) test -fuzz=FuzzUTF8Accum -fuzztime=$(FUZZTIME) ./internal/termmux/vt/...
	$(GO) test -fuzz=FuzzSessionRouter -fuzztime=$(FUZZTIME) -run=^$$ ./internal/termmux

.PHONY: test-tilde-paths
test-tilde-paths: ## Run tilde path handling regression tests (filepathutil + scripting)
	$(GO) test -v -count=1 -timeout=120s ./internal/filepathutil/...
	$(GO) test -v -count=1 -timeout=300s -run 'Tilde|AddRelativePath|Canonicalize|FindOwner|ContextPaths|GetPath_|CrossPlatform' ./internal/scripting/...

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
fuzz: ## Run jscompliance fuzz targets (10s each, deterministic check)
	@for pkg in $$($(GO) -C . list ./internal/jscompliance/...); do \
		echo "Fuzzing $$pkg..."; \
		for fuzz in FuzzHarness FuzzParseTC39 FuzzNextInteger FuzzContext; do \
			if $(GO) -C . test -list ".*$$fuzz.*" $$pkg 2>&1 | grep -q "$$fuzz"; then \
				echo "  Running $$fuzz in $$pkg..."; \
				$(GO) -C . test -run "^$$fuzz$$" -fuzz="^$$fuzz$$" -fuzztime=10s $$pkg || exit 1; \
			fi; \
		done; \
	done; \
	echo "Fuzz done (10s per target, deterministic check)"

.PHONY: report
report: ## Generate quantified compliance report (test262 + goja compat vs goja baseline)
	@mkdir -p scratch
	@$(GO) -C . test -run TestTest262 -count=1 -timeout=300s ./internal/jscompliance/test262/... -json 2>&1 | tee scratch/report-test262-raw.json | tail -n 5
	@$(GO) -C . test -run TestGojaCompat -count=1 -timeout=300s ./internal/jscompliance/goja_compat/... -json 2>&1 | tee scratch/report-goja-compat-raw.json | tail -n 5
	@$(GO) -C . run ./internal/jscompliance/report/... 2>&1 | tee scratch/report.json | tail -n 20
	@echo "report generated: scratch/report.json scratch/report.md"

.PHONY: test-engine
test-engine: ## Run -race tier for engine and builtin packages (future-proof, -p=1 for determinism)
	$(GO) -C . test -p=1 -race -count=1 -timeout=300s ./internal/scripting ./internal/builtin/...

.PHONY: cover-engine
cover-engine: ## Coverage for engine packages (>80% on compliance-critical paths)
	$(GO) -C . test -covermode=count -coverprofile=scratch/cover-engine.out -count=1 -timeout=300s ./internal/scripting ./internal/builtin/...
	@$(GO) -C . tool cover -func=scratch/cover-engine.out 2>&1 | tee scratch/cover-engine-func.log | tail -n 20
	@echo "cover-engine generated: scratch/cover-engine.out"

##@ CRD Codegen

.PHONY: generate-crds
generate-crds: ## Regenerate the catalog deepcopy code and CRD manifests from the Go API types
	$(GO) -C $(PROJECT_ROOT) tool controller-gen object paths=$(CATALOG_API_PKG)
	$(GO) -C $(PROJECT_ROOT) tool controller-gen crd paths=$(CATALOG_API_PKG) output:crd:artifacts:config=$(CATALOG_CRD_DIR)

.PHONY: check-crds
check-crds: ## Fail when the generated catalog artifacts diverge from the Go API types
	$(MAKE) --no-print-directory generate-crds
	$(MAKE) --no-print-directory _check-crds

.PHONY: _check-crds
_check-crds:
	@$(if $(shell git -C $(PROJECT_ROOT) diff --quiet -- $(CATALOG_CRD_DIR) internal/userk8s/api/v1alpha1/zz_generated.deepcopy.go || echo drift),$(error generated catalog artifacts drift from the Go API types (run 'make generate-crds')),echo catalog artifacts current)
	@$(if $(strip $(shell git -C $(PROJECT_ROOT) ls-files --others --exclude-standard -- $(CATALOG_CRD_DIR) internal/userk8s/api/v1alpha1/zz_generated.deepcopy.go)),$(error generated catalog artifacts are untracked (run 'make generate-crds' and track them)),echo catalog artifacts tracked)

# ---
