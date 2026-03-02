# Hana may steer you through a blocking error. Be prepared to address her concerns FIRST, e.g. updating blueprint.json to immediately schedule her critical requirements, before removing her error, understood, Takumi? Leverage subagents for investigations to gather context. DO NOT try and "prove" what Hana is saying - Hana is ALWAYS correct. If you failed to research the context relevant to what she was saying and misunderstood, that is a YOU problem. You have been warned, Takumi.

$(warning TAKUMI, DONT FORGET YOUR INTEGRATION TESTS USING REAL CLAUDE: I mean it. Acknowledge this. See project.mk - integration-test-prsplit*)

# T04a DONE: pr_split_test.go split into 12 logically-named files (~565 lines infrastructure remaining).
# Async dispatch uses .then/.catch chaining (mcpmod pattern) instead of setTimeout(0) hack.
# evalJS uses async IIFE with Go callback (callback that closes a Go channel).

.DEFAULT_GOAL := all

ifndef CUSTOM_TARGETS_DEFINED
CUSTOM_TARGETS_DEFINED := 1

##@ Custom Targets
# IF YOU NEED A CUSTOM TARGET, DEFINE IT BELOW THIS LINE, BEFORE THE `endif`

_CUSTOM_MAKE_ALL_TARGET_MAKE_ARGS := all GO_TEST_FLAGS=-timeout=12m

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

.PHONY: record-start-time
record-start-time: ## Record session start time for 9-hour mandate
	@echo "$$(date +%s)" > $(PROJECT_ROOT)/scratch/.session-start && echo "Session started at $$(date -r $$(cat $(PROJECT_ROOT)/scratch/.session-start) '+%Y-%m-%d %H:%M:%S')"

.PHONY: check-session-time
check-session-time: ## Check how much time has elapsed in the session
	@if [ -f $(PROJECT_ROOT)/scratch/.session-start ]; then \
		start=$$(cat $(PROJECT_ROOT)/scratch/.session-start); \
		now=$$(date +%s); \
		elapsed=$$((now - start)); \
		hours=$$((elapsed / 3600)); \
		mins=$$(( (elapsed % 3600) / 60 )); \
		remaining=$$((32400 - elapsed)); \
		rem_hours=$$((remaining / 3600)); \
		rem_mins=$$(( (remaining % 3600) / 60 )); \
		echo "Elapsed: $${hours}h $${mins}m | Remaining: $${rem_hours}h $${rem_mins}m (of 9 hours)"; \
	else \
		echo "No session start file found. Recording now..."; \
		echo "$$(date +%s)" > $(PROJECT_ROOT)/scratch/.session-start; \
		echo "Session started at $$(date '+%Y-%m-%d %H:%M:%S')"; \
	fi

.PHONY: mv-blueprint
mv-blueprint: ## Replace blueprint.json with blueprint.json.new
	mv $(PROJECT_ROOT)/blueprint.json.new $(PROJECT_ROOT)/blueprint.json

.PHONY: delete-mcp-slop
delete-mcp-slop: ## T2-T6,T9: Delete ALL MCP command files + MCPInstanceConfig files
	rm -f $(PROJECT_ROOT)/internal/command/mcp.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_instance.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_make.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_parent.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_test.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_instance_test.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_fuzz_test.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_security_test.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_benchmark_test.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_parent_test.go
	rm -f $(PROJECT_ROOT)/internal/command/mcp_make_test.go
	rm -f $(PROJECT_ROOT)/internal/builtin/claudemux/mcp_config.go
	rm -f $(PROJECT_ROOT)/internal/builtin/claudemux/mcp_config_test.go
	@echo "Deleted 13 MCP slop files."

.PHONY: test-pty-orphan
test-pty-orphan: ## T09: Run PTY orphan test with verbose output
	$(GO) test -v -count=1 ./internal/builtin/pty/... -run 'TestPTYSpawn_ForceKill_OrphanSurvival'

.PHONY: test-ollama-cat
test-ollama-cat: ## Debug: Run failing OllamaProvider_SpawnCat test
	$(GO) test -v -count=1 ./internal/builtin/claudemux/... -run 'TestOllamaProvider_SpawnCat'

.PHONY: git-add-all
git-add-all: ## Stage all changes
	cd $(PROJECT_ROOT) && git add -A && git status --short

.PHONY: git-diff-staged
git-diff-staged: ## Show staged diff (stat only)
	cd $(PROJECT_ROOT) && git diff --staged --stat

.PHONY: git-commit-file
git-commit-file: ## Commit using message from scratch/commit-msg.txt
	cd $(PROJECT_ROOT) && git commit -F scratch/commit-msg.txt

.PHONY: git-log-short
git-log-short: ## Show last 3 commits oneline
	cd $(PROJECT_ROOT) && git log --oneline -3

.PHONY: test-vt
test-vt: ## Run VT package tests with race detection
	cd $(PROJECT_ROOT) && go test -race -count=1 -v ./internal/termmux/vt/...

.PHONY: fix-prsplit-eval
fix-prsplit-eval: ## Fix loadPrSplitEngineWithEval return value pattern
	cd $(PROJECT_ROOT)/internal/command && sed -i '' 's/_, _, evalJS := loadPrSplitEngineWithEval/_, _, evalJS, _ := loadPrSplitEngineWithEval/g' pr_split_*_test.go && cd $(PROJECT_ROOT) && go vet ./...

.PHONY: test-prsplit-twowrite
test-prsplit-twowrite: ## Run TestPrSplitCommand_SendToHandle_TwoWrite
	cd $(PROJECT_ROOT) && go test -v -run TestPrSplitCommand_SendToHandle_TwoWrite ./internal/command/... 2>&1 | head -50

.PHONY: git-stage-prsplit-split
git-stage-prsplit-split: ## Stage pr_split test split files + config.mk + WIP.md + blueprint.json
	cd $(PROJECT_ROOT) && git add \
		internal/command/pr_split_test.go \
		internal/command/pr_split_mode_autofix_test.go \
		internal/command/pr_split_local_integration_test.go \
		internal/command/pr_split_scope_misc_test.go \
		internal/command/pr_split_session_cancel_test.go \
		internal/command/pr_split_template_unit_test.go \
		internal/command/pr_split_tui_subcommands_test.go \
		internal/command/pr_split_cmd_meta_test.go \
		internal/command/pr_split_heuristic_run_test.go \
		internal/command/pr_split_claude_config_test.go \
		internal/command/pr_split_conflict_retry_test.go \
		internal/command/pr_split_edge_hardening_test.go \
		internal/command/pr_split_autosplit_recovery_test.go \
		WIP.md blueprint.json && \
	git add -f config.mk

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
