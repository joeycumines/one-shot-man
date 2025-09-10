# Project-specific configuration for one-shot-man

# Tool packages - filter out grit and betteralign, enable deadcode
GO_TOOLS := $(filter-out $(GO_PKG_GRIT) $(GO_PKG_BETTERALIGN),$(GO_TOOLS_DEFAULT)) $(GO_PKG_DEADCODE) $(GO_PKG_SIMPLE_COMMAND_OUTPUT_FILTER)

# Enable deadcode tool for all modules - need to set this to "root" for single module  
GO_MODULE_SLUGS_USE_DEADCODE := root

# Exclude modules from betteralign targets (since we're filtering it out)
GO_MODULE_SLUGS_NO_BETTERALIGN := root

# Set deadcode ignore patterns file
DEADCODE_IGNORE_PATTERNS_FILE := .deadcodeignore

# Treat unignored deadcode as errors
DEADCODE_ERROR_ON_UNIGNORED := true

# Override tool paths to use installed binaries
STATICCHECK := $(shell which staticcheck || echo $(shell go env GOPATH)/bin/staticcheck)
DEADCODE := $(shell which simple-command-output-filter || echo $(shell go env GOPATH)/bin/simple-command-output-filter) -v -e on-content -f $(DEADCODE_IGNORE_PATTERNS_FILE) -- $(shell which deadcode || echo $(shell go env GOPATH)/bin/deadcode)