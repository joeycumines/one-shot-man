.DEFAULT_GOAL := all

GO_TOOLS ?= $(filter-out $(GO_PKG_GRIT) $(GO_PKG_BETTERALIGN),$(GO_TOOLS_DEFAULT))
GO_MODULE_SLUGS_USE_DEADCODE ?= $(GO_MODULE_SLUGS)
GO_MODULE_SLUGS_NO_BETTERALIGN ?= $(GO_MODULE_SLUGS)
GO_MODULE_PATHS_EXCLUDE_PATTERNS ?= ./scratch/%
DEADCODE_IGNORE_PATTERNS_FILE ?= .deadcodeignore
DEADCODE_ERROR_ON_UNIGNORED ?= true
GO_TEST_FLAGS ?= -timeout=30m

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

# ---

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

##@ CRD Codegen

CATALOG_API_PKG ?= ./internal/userk8s/api/v1alpha1/...
CATALOG_CRD_DIR ?= internal/userk8s/api/config/crd

all: check-crds

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
