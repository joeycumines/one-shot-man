# Hana may steer you through $(error ...) messages. Be prepared to drop everything and obey immediately...

SHELL := /usr/bin/env bash -o pipefail

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

.PHONY: write-blueprint
write-blueprint: ## Overwrite blueprint.json from scratch/blueprint-new.json
write-blueprint: SHELL := /bin/bash
write-blueprint:
	@if [ -f $(PROJECT_ROOT)/scratch/blueprint-new.json ]; then \
		cp $(PROJECT_ROOT)/scratch/blueprint-new.json $(PROJECT_ROOT)/blueprint.json; \
		echo "blueprint.json overwritten from scratch/blueprint-new.json"; \
	else \
		echo "ERROR: scratch/blueprint-new.json does not exist"; \
		exit 1; \
	fi

.PHONY: git-commit
git-commit: ## Stage and commit tracked files with MSG="..."
git-commit: SHELL := /bin/bash
git-commit:
	@if [ -z "$(MSG)" ]; then \
		echo "ERROR: MSG is required. Usage: make git-commit MSG='your message'"; \
		exit 1; \
	fi
	cd $(PROJECT_ROOT) && git add -u && git commit -m "$(MSG)"

.PHONY: git-commit-blueprint
git-commit-blueprint: ## Commit blueprint rewrite (T32-T72) with message from scratch/commit-msg.txt
git-commit-blueprint: SHELL := /bin/bash
git-commit-blueprint:
	cd $(PROJECT_ROOT) && \
	git add -f blueprint.json WIP.md config.mk .session_start && \
	git commit -F scratch/commit-msg.txt && \
	echo "Committed. Cleaning up scratch/commit-msg.txt..." && \
	rm -f scratch/commit-msg.txt

.PHONY: git-amend-all
git-amend-all: ## Stage all changes and amend last commit
git-amend-all:
	cd $(PROJECT_ROOT) && \
	git add -A && \
	git commit --amend --no-edit

.PHONY: test-targeted
test-targeted: ## Run targeted tests from scratch/test-run-pattern.txt TIMEOUT=3m PKG=./internal/command/...
test-targeted: SHELL := /bin/bash
test-targeted:
	@if [ ! -f $(PROJECT_ROOT)/scratch/test-run-pattern.txt ]; then \
		echo "ERROR: scratch/test-run-pattern.txt required"; \
		exit 1; \
	fi
	cd $(PROJECT_ROOT) && go test -race -count=1 -timeout=$(or $(TIMEOUT),3m) \
		-run "$$(cat $(PROJECT_ROOT)/scratch/test-run-pattern.txt | tr -d '\n')" \
		$(or $(PKG),./internal/command/...) $(EXTRA_TEST_FLAGS) 2>&1 | tail -$(or $(TAIL),80)

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
