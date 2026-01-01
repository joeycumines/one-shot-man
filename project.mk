GO_TOOLS ?= $(filter-out $(GO_PKG_GRIT) $(GO_PKG_BETTERALIGN),$(GO_TOOLS_DEFAULT))
GO_MODULE_SLUGS_USE_DEADCODE ?= $(GO_MODULE_SLUGS)
GO_MODULE_SLUGS_NO_BETTERALIGN ?= $(GO_MODULE_SLUGS)
DEADCODE_IGNORE_PATTERNS_FILE ?= .deadcodeignore
DEADCODE_ERROR_ON_UNIGNORED ?= true

##@ Project Targets

.PHONY: generate-tapes-and-gifs
generate-tapes-and-gifs: ## Generate all recording tapes and GIFs
	@echo "Generating all recording tapes and GIFs..."
	$(GO) -C $(PROJECT_ROOT)/internal/scripting test -v -count=1 -timeout=10m -run "^TestRecording_" -record -execute-vhs

# ---
