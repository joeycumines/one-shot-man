.DEFAULT_GOAL := all

GO_TOOLS ?= $(filter-out $(GO_PKG_GRIT) $(GO_PKG_BETTERALIGN),$(GO_TOOLS_DEFAULT))
GO_MODULE_SLUGS_USE_DEADCODE ?= $(GO_MODULE_SLUGS)
GO_MODULE_SLUGS_NO_BETTERALIGN ?= $(GO_MODULE_SLUGS)
GO_MODULE_PATHS_EXCLUDE_PATTERNS ?= ./scratch/%
DEADCODE_IGNORE_PATTERNS_FILE ?= .deadcodeignore
DEADCODE_ERROR_ON_UNIGNORED ?= true

##@ Project Targets

.PHONY: generate-tapes-and-gifs
generate-tapes-and-gifs: ## Generate all recording tapes and GIFs
	@echo "Generating all recording tapes and GIFs..."
	$(GO) -C $(PROJECT_ROOT)/internal/scripting test -v -count=1 -timeout=10m -run "^TestRecording_" -record -execute-vhs

.PHONY: integration-test-claudemux
integration-test-claudemux: ## Run claudemux integration tests (requires real agent infrastructure)
integration-test-claudemux: PROVIDER ?= ollama
integration-test-claudemux: MODEL ?= minimax-m2.5:cloud
integration-test-claudemux:
	$(GO) test -race -v -count=1 -timeout=10m \
		-integration -provider=$(PROVIDER) -model=$(MODEL) \
		./internal/builtin/claudemux/...

.PHONY: integration-test-prsplit
integration-test-prsplit: ## Run pr-split integration tests with real Claude/AI (requires agent infrastructure)
integration-test-prsplit: CLAUDE_COMMAND ?= claude
integration-test-prsplit: CLAUDE_ARGS ?=
integration-test-prsplit: INTEGRATION_MODEL ?= minimax-m2.5:cloud
integration-test-prsplit: PRSPLIT_TEST_RUN ?= TestIntegration_(.*Claude|AutoSplitComplex|PrSplit_VTerm)
integration-test-prsplit:
	$(GO) test -race -count=1 -timeout=15m \
		./internal/command/... \
		-run '$(PRSPLIT_TEST_RUN)' \
		-integration \
		-claude-command=$(CLAUDE_COMMAND) \
		$(foreach arg,$(CLAUDE_ARGS),-claude-arg=$(arg)) \
		-integration-model=$(INTEGRATION_MODEL)

.PHONY: integration-test-prsplit-mcp
integration-test-prsplit-mcp: ## Run pr-split MCP mock integration tests (no real AI required)
	$(GO) test -race -v -count=1 -timeout=10m \
		./internal/command/... -run 'TestIntegration_AutoSplitMockMCP'

# ---

.PHONY: integration-test-termmux
integration-test-termmux: ## Run termmux integration tests with real PTY processes
	$(GO) test -race -v -count=1 -timeout=5m -tags=integration ./internal/termmux/...

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
