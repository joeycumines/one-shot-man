# Hana may steer you through a blocking error. Be prepared to address her concerns FIRST, e.g. updating blueprint.json to immediately schedule her critical requirements, before removing her error, understood, Takumi? Leverage subagents for investigations to gather context. DO NOT try and "prove" what Hana is saying - Hana is ALWAYS correct. If you failed to research the context relevant to what she was saying and misunderstood, that is a YOU problem. You have been warned, Takumi.

$(warning TAKUMI: T37-T41 scheduled per Hana mandate. Real Claude test MUST pass end-to-end. Edge cases MUST be verified. See blueprint.json T37-T41.)

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

.PHONY: run-single-test
run-single-test: ## Run a single test: make run-single-test TEST=TestName PKG=./internal/command/...
	$(GO) test -v -race -timeout=5m $(PKG) -run $(TEST)

.PHONY: clean-test-artifacts
clean-test-artifacts: ## Remove runtime test artifacts that should not be committed
	rm -f $(PROJECT_ROOT)/internal/command/.pr-split-plan.json

.PHONY: git-stage-all
git-stage-all: ## Stage all changes
	cd $(PROJECT_ROOT) && git add -A && git status --short

.PHONY: git-commit-staged
git-commit-staged: ## Commit staged changes with message from .git/COMMIT_MSG_TEMP
	cd $(PROJECT_ROOT) && git commit -F .git/COMMIT_MSG_TEMP

.PHONY: check-ai-tools
check-ai-tools: ## Check for available AI tools (claude, ollama, socat)
	@echo "=== AI Tool Availability ==="
	@which claude 2>/dev/null && claude --version 2>/dev/null || echo "claude: NOT FOUND"
	@which ollama 2>/dev/null && ollama --version 2>/dev/null || echo "ollama: NOT FOUND"
	@echo "=== MCP Dependencies ==="
	@which socat 2>/dev/null && socat -V 2>/dev/null | head -3 || echo "socat: NOT FOUND (required for MCP callback)"
	@echo "=== Claude Auth ==="
	@if [ -n "$$ANTHROPIC_API_KEY" ]; then echo "ANTHROPIC_API_KEY: SET (length=$${#ANTHROPIC_API_KEY})"; else echo "ANTHROPIC_API_KEY: NOT SET"; fi
	@claude -p "ping" --max-turns 1 2>&1 | head -3 || echo "claude -p: FAILED (need login or API key)"
	@echo "=== Windows Cross-Compile ==="
	@GOOS=windows go build ./... && echo "GOOS=windows build: OK" || echo "GOOS=windows build: FAILED"

.PHONY: run-real-claude-test
run-real-claude-test: ## Run real Claude integration test for pr-split (requires 'claude login')
	cd $(PROJECT_ROOT) && $(GO) test -race -v -count=1 -timeout=15m \
		./internal/command/... \
		-run 'TestIntegration_AutoSplitWithClaude_Pipeline' \
		-integration \
		-claude-command=claude

.PHONY: run-headless-claude-test
run-headless-claude-test: ## Run headless Claude MCP test (works with ANTHROPIC_API_KEY)
	cd $(PROJECT_ROOT) && $(GO) test -race -v -count=1 -timeout=5m \
		./internal/command/... \
		-run 'TestIntegration_ClaudeMCP_Headless' \
		-integration \
		-claude-command=claude

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

.PHONY: regen-diff
regen-diff: ## Regenerate scratch/review-diff.txt from HEAD
	cd $(PROJECT_ROOT) && git diff HEAD > scratch/review-diff.txt && wc -l scratch/review-diff.txt

.PHONY: test-bt-workflow-tree
test-bt-workflow-tree: ## Run TestBTNodeFactory_CreateWorkflowTree_Type
	cd $(PROJECT_ROOT) && $(GO) test -run TestBTNodeFactory_CreateWorkflowTree_Type -v ./internal/command/ -count=1 -timeout 120s 2>&1 | tail -20

.PHONY: test-bt-template-diff
test-bt-template-diff: ## Run BT/Template/Diff/Report tests (full output)
	cd $(PROJECT_ROOT) && $(GO) test -run 'TestBTNodeFactory|TestBTTemplate|TestRenderColorizedDiff|TestGetSplitDiff|TestBuildReport' -v ./internal/command/ -count=1 -timeout 300s 2>&1

.PHONY: test-groupby-dep
test-groupby-dep: ## Run TestGroupByDependency tests
	cd $(PROJECT_ROOT) && $(GO) test -run 'TestGroupByDependency' -v ./internal/command/ -count=1 -timeout 120s 2>&1

.PHONY: git-stage-t42-t48
git-stage-t42-t48: ## Stage T42-T48 changes
	cd $(PROJECT_ROOT) && git add \
		internal/command/pr_split_bt_test.go \
		internal/command/pr_split_script.js \
		internal/command/pr_split.go \
		internal/command/pr_split_planning_test.go \
		WIP.md blueprint.json && \
	git add -f config.mk

.PHONY: git-commit-t42-t48
git-commit-t42-t48: ## Commit T42-T48 batch
	cd $(PROJECT_ROOT) && git commit -F scratch/commit-msg-t42-t48.txt

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
