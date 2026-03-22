# Hana may steer you through $(error ...) messages. Be prepared to drop everything and obey immediately...
# RESOLVED: T393 (Ask Claude), T394 (termmux audit), T395 (test completion time) added to blueprint.

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
record-session-start: ## Record session start timestamp
	@date +%s > $(or $(PROJECT_ROOT),$(error PROJECT_ROOT required))/session-start.txt
	@echo "Session started at $$(date -r $$(cat $(PROJECT_ROOT)/session-start.txt) '+%Y-%m-%d %H:%M:%S')"
	@echo "End target: $$(date -r $$(( $$(cat $(PROJECT_ROOT)/session-start.txt) + 32400 )) '+%Y-%m-%d %H:%M:%S') (9 hours)"

.PHONY: check-session-time
check-session-time: ## Show elapsed/remaining time for 9-hour session
check-session-time: SHELL := /bin/bash
check-session-time:
	@if [ ! -f "$(PROJECT_ROOT)/session-start.txt" ]; then \
	    echo "ERROR: No session-start.txt found. Run 'make record-session-start' first."; \
	    exit 1; \
	fi; \
	start=$$(cat $(PROJECT_ROOT)/session-start.txt); \
	now=$$(date +%s); \
	elapsed=$$((now - start)); \
	remaining=$$((32400 - elapsed)); \
	elapsed_h=$$((elapsed / 3600)); elapsed_m=$$(( (elapsed % 3600) / 60 )); \
	if [ $$remaining -gt 0 ]; then \
	    remain_h=$$((remaining / 3600)); remain_m=$$(( (remaining % 3600) / 60 )); \
	    echo "Elapsed: $${elapsed_h}h $${elapsed_m}m | Remaining: $${remain_h}h $${remain_m}m"; \
	else \
	    over=$$(( -remaining )); over_h=$$((over / 3600)); over_m=$$(( (over % 3600) / 60 )); \
	    echo "Elapsed: $${elapsed_h}h $${elapsed_m}m | OVERTIME: $${over_h}h $${over_m}m (session ended)"; \
	fi

.PHONY: test-prsplit-fast
test-prsplit-fast: ## Run PR Split tests in fast mode (skips slow/E2E via -short)
	$(GO) test -timeout=600s -race -short ./internal/command/... 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-t395-acceptance
test-t395-acceptance: ## T395: Run with 300s timeout and -short (acceptance criteria)
	$(GO) test -timeout=300s -race -short ./internal/command/... 2>&1 | fold -w 200 | tail -n 10

.PHONY: test-t395-count
test-t395-count: ## T395: Count passed/skipped/failed tests
test-t395-count: SHELL := /bin/bash
test-t395-count:
	@out=$$($(GO) test -timeout=300s -race -short -v ./internal/command/... 2>&1); \
	passed=$$(echo "$$out" | grep -c -e 'PASS:' || true); \
	skipped=$$(echo "$$out" | grep -c -e 'SKIP:' || true); \
	failed=$$(echo "$$out" | grep -c -e 'FAIL:' || true); \
	echo "Passed: $$passed | Skipped: $$skipped | Failed: $$failed"

.PHONY: test-t395-profile
test-t395-profile: ## Profile test timing with JSON output
test-t395-profile: SHELL := /bin/bash
test-t395-profile:
	@$(GO) test -timeout=900s -race -short -json ./internal/command/... 2>&1 | \
	python3 -c "import sys,json; tests={}; \
[tests.update({d.get('Test',''):d.get('Elapsed',0)}) for line in sys.stdin if (d:=json.loads(line)).get('Action')=='pass' and d.get('Test')]; \
[print(f'{v:8.2f}s  {k}') for k,v in sorted(tests.items(), key=lambda x: -x[1])[:50]]"

.PHONY: test-key-forwarding
test-key-forwarding: ## Run key forwarding and INTERACTIVE_RESERVED_KEYS tests only
	$(GO) test -timeout=120s -race -run 'TestChunk16_VTerm_KeyToTermBytes|TestKeyToTermBytes_SpecialKeys_T386|TestInteractiveReservedKeys_T386' ./internal/command/... -v 2>&1 | tail -n 50

.PHONY: test-ask-claude
test-ask-claude: ## Run Ask Claude and question detection tests
	$(GO) test -timeout=120s -race -run 'TestChunk16_ClaudeConvo|TestChunk16_T46|TestChunk16_Claude|TestConfirmCancel|TestChunk16_T393' ./internal/command/... -v 2>&1 | tail -n 60

.PHONY: test-t393
test-t393: ## Run T393 Ask Claude keep-alive tests only
	$(GO) test -timeout=120s -race -run 'TestChunk16_T393' ./internal/command/... -v 2>&1

.PHONY: test-t393-broad
test-t393-broad: ## Run all chunk 16 + chunk 10 tests
	$(GO) test -timeout=300s -race -run 'TestChunk16_|TestChunk10_' ./internal/command/... -short 2>&1 | tail -n 20

.PHONY: test-prsplit-all
test-prsplit-all: ## Run ALL PR Split tests including slow/E2E
	$(GO) test -timeout=900s -race ./internal/command/... 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-prsplit-e2e
test-prsplit-e2e: ## Run only E2E PR Split tests
	$(GO) test -timeout=300s -race ./internal/command/... -run 'TestBinaryE2E' 2>&1 | fold -w 200 | tail -n 30

.PHONY: test-run
test-run: ## Run specific test(s): make test-run T=TestFoo
	$(GO) test -timeout=300s -race -v ./internal/command/... -run '$(T)' 2>&1 | fold -w 200 | tail -n 60

.PHONY: test-statusbar
test-statusbar: ## Run status bar and help overlay tests
	$(GO) test -timeout=300s -race -v ./internal/command/... -run 'TestStatusBar|TestHelpOverlay|TestChunk13_RenderStatusBar' 2>&1 | fold -w 200 | tail -n 60

.PHONY: test-verify-handlers
test-verify-handlers: ## Run verify handler and lifecycle tests
	$(GO) test -timeout=300s -race -v ./internal/command/... -run 'TestVerify|TestExecScreen|TestBranchBuilding' 2>&1 | fold -w 200 | tail -n 60

.PHONY: test-views
test-views: ## Run TUI view rendering tests
	$(GO) test -timeout=300s -race -v ./internal/command/... -run 'TestViews|TestChunk13_View' 2>&1 | fold -w 200 | tail -n 60

.PHONY: cross-build
cross-build: ## Build for Linux, macOS, and Windows
	@echo "Building for linux/amd64..."; GOOS=linux GOARCH=amd64 $(GO) build ./... 2>&1 | tail -n 5
	@echo "Building for darwin/amd64..."; GOOS=darwin GOARCH=amd64 $(GO) build ./... 2>&1 | tail -n 5
	@echo "Building for windows/amd64..."; GOOS=windows GOARCH=amd64 $(GO) build ./... 2>&1 | tail -n 5
	@echo "Cross-build complete."

.PHONY: run-t383-cli-observation
run-t383-cli-observation: ## Run real-binary Claude CLI flag observation test and capture full log
run-t383-cli-observation: SHELL := /bin/bash
run-t383-cli-observation:
	@set -o pipefail; \
	GOCACHE=$(PROJECT_ROOT)/.gocache-t383 $(GO) test -timeout=300s -v ./internal/command/... -run 'TestBinaryE2E_ClaudeCommandFlags$$' 2>&1 | tee $(PROJECT_ROOT)/scratch/t383-cli-run.log | fold -w 200 | tail -n 80; \
	exit $${PIPESTATUS[0]}

.PHONY: run-t383-cli-auto
run-t383-cli-auto: ## Run direct osm pr-split CLI observation with strategy=auto and mock Claude
run-t383-cli-auto: SHELL := /bin/bash
run-t383-cli-auto:
	@set -euo pipefail; \
	tmp_root="$$(mktemp -d)"; \
	repo="$$tmp_root/repo"; \
	mock_dir="$$tmp_root/mock"; \
	home_dir="$$tmp_root/home"; \
	bin_path="$$tmp_root/osm"; \
	arg_log="$(PROJECT_ROOT)/scratch/t383-auto-args.log"; \
	stdout_log="$(PROJECT_ROOT)/scratch/t383-auto-stdout.log"; \
	stderr_log="$(PROJECT_ROOT)/scratch/t383-auto-stderr.log"; \
	rm -f "$$arg_log" "$$stdout_log" "$$stderr_log"; \
	mkdir -p "$$repo" "$$mock_dir" "$$home_dir"; \
	cd "$$repo"; \
	git init -q; \
	git config user.name Test; \
	git config user.email test@example.com; \
	git checkout -b main >/dev/null 2>&1; \
	mkdir -p cmd api web; \
	printf 'package main\nfunc main(){}\n' > cmd/main.go; \
	printf 'package api\nconst Version = 1\n' > api/version.go; \
	printf '<html>base</html>\n' > web/index.html; \
	git add .; \
	git commit -q -m baseline; \
	git checkout -b feature >/dev/null 2>&1; \
	printf 'package main\nfunc main(){ println("feature") }\n' > cmd/main.go; \
	printf 'package api\nconst Version = 2\n' > api/version.go; \
	printf '<html>feature</html>\n' > web/index.html; \
	printf 'notes\n' > NOTES.md; \
	git add .; \
	git commit -q -m feature; \
	chmod -R u+rwX "$$tmp_root"; \
	printf '%s\n' '#!/bin/sh' "printf '%s\\n' \"\$$@\" > \"$$arg_log\"" 'sleep 0.1' 'exit 0' > "$$mock_dir/mock-claude"; \
	chmod +x "$$mock_dir/mock-claude"; \
	cd "$(PROJECT_ROOT)"; \
	GOCACHE="$(PROJECT_ROOT)/.gocache-t383" $(GO) build -o "$$bin_path" ./cmd/osm; \
	cd "$$repo"; \
	HOME="$$home_dir" OSM_CONFIG= TERM=xterm-256color GIT_TERMINAL_PROMPT=0 GIT_PAGER=cat NO_COLOR=1 \
		"$$bin_path" pr-split -interactive=false -base=main -strategy=auto -claude-command="$$mock_dir/mock-claude" -claude-arg=--custom-flag -claude-arg=--verbose --store=memory --session=t383-auto run \
		>"$$stdout_log" 2>"$$stderr_log" || true; \
	echo '--- stdout ---'; tail -n 40 "$$stdout_log"; \
	echo '--- stderr ---'; tail -n 40 "$$stderr_log"; \
	echo '--- mock args ---'; if [ -f "$$arg_log" ]; then cat "$$arg_log"; else echo '(mock not invoked)'; fi

.PHONY: run-t383-failure-modes
run-t383-failure-modes: ## Exercise various pr-split failure modes and capture outputs
run-t383-failure-modes: SHELL := /bin/bash
run-t383-failure-modes:
	@set -uo pipefail; \
	log="$(PROJECT_ROOT)/scratch/t383-failure-modes.log"; \
	>$$log; \
	bin_path="$$(mktemp -d)/osm"; \
	echo "=== Building binary ===" | tee -a $$log; \
	$(GO) build -o "$$bin_path" ./cmd/osm 2>&1 | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 1: Invalid strategy ===" | tee -a $$log; \
	tmp="$$(mktemp -d)"; cd "$$tmp" && git init -q && git config user.name X && git config user.email x@x && git checkout -b main 2>/dev/null && echo f > f && git add . && git commit -qm i && git checkout -b feat 2>/dev/null && echo g > f && git add . && git commit -qm f; \
	(HOME="$$tmp" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false -base=main -strategy=BOGUS --store=memory --session=s1 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 2: Missing base branch ===" | tee -a $$log; \
	(HOME="$$tmp" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false -base=nonexistent-branch --store=memory --session=s2 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 3: Not a git repo ===" | tee -a $$log; \
	tmp2="$$(mktemp -d)"; cd "$$tmp2"; \
	(HOME="$$tmp2" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false --store=memory --session=s3 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 4: Invalid max files ===" | tee -a $$log; \
	cd "$$tmp"; \
	(HOME="$$tmp" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false -base=main -max=0 --store=memory --session=s4 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 5: No diff (same branch) ===" | tee -a $$log; \
	cd "$$tmp" && git checkout main 2>/dev/null; \
	(HOME="$$tmp" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false -base=main --store=memory --session=s5 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 6: Bad timeout format ===" | tee -a $$log; \
	cd "$$tmp" && git checkout feat 2>/dev/null; \
	(HOME="$$tmp" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false -timeout=bananas --store=memory --session=s6 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== SCENARIO 7: Bad claude-env format ===" | tee -a $$log; \
	(HOME="$$tmp" OSM_CONFIG= NO_COLOR=1 "$$bin_path" pr-split -interactive=false -base=main -claude-env='NOT_VALID' --store=memory --session=s7 run 2>&1 || true) | tee -a $$log; \
	echo "" | tee -a $$log; \
	echo "=== DONE ===" | tee -a $$log; \
	echo "Full log: $$log"

.PHONY: git-amend
git-amend: ## Amend last commit with staged changes (no message edit)
git-amend: SHELL := /bin/bash
git-amend:
	cd $(PROJECT_ROOT) && git add -A && git commit --amend --no-edit && git log --oneline -1

.PHONY: git-commit-file
git-commit-file: ## Stage all and commit using .commit-msg.txt, then clean up
git-commit-file: SHELL := /bin/bash
git-commit-file:
	cd $(PROJECT_ROOT) && git add -A && git commit -F .commit-msg.txt && rm -f .commit-msg.txt && git log --oneline -1

.PHONY: git-commit-staged
git-commit-staged: ## Commit only staged files using .commit-msg.txt
git-commit-staged: SHELL := /bin/bash
git-commit-staged:
	cd $(PROJECT_ROOT) && git commit -F .commit-msg.txt && rm -f .commit-msg.txt && git log --oneline -1

.PHONY: clean-gocache
clean-gocache: ## Remove task-specific Go caches (.gocache-*)
	rm -rf $(PROJECT_ROOT)/.gocache-*

.PHONY: git-stage-t396
git-stage-t396: ## Stage T396 key-forwarding files
	cd $(PROJECT_ROOT) && git add \
		internal/command/pr_split_16d_tui_handlers_claude.js \
		internal/command/pr_split_16e_tui_update.js \
		internal/command/pr_split_16_verify_fixes_test.go \
		internal/command/pr_split_16_vterm_key_forwarding_test.go && \
	git diff --stat --staged

.PHONY: test-t394
test-t394: ## Run T394 Ctrl+] toggle tests
	go test -timeout=120s -race -run 'TestKeyHandling_CtrlBracket|TestChunk16_CtrlBracket|TestStatusBar_CtrlBracket' ./internal/command/... -v 2>&1 | tail -n 60

.PHONY: commit-t394
commit-t394: ## Stage and commit T394
commit-t394: SHELL := /bin/bash
commit-t394:
	cd $(PROJECT_ROOT) && git add \
		internal/command/pr_split_16f_tui_model.js \
		internal/command/pr_split_16e_tui_update.js \
		internal/command/pr_split_16_ctrl_bracket_test.go \
		internal/command/pr_split_16_keyboard_crash_test.go \
		internal/command/pr_split_16_focus_nav_edge_test.go \
		blueprint.json && \
	git add -f scratch/t394-termmux-audit.md WIP.md config.mk && \
	git commit -m $$'Fix Ctrl+] stdin contention in Claude passthrough\n\nWire toggleKey/onToggle options in tea.run() so BubbleTea wraps the\nwizard model in toggleModel. The Go-level wrapper intercepts Ctrl+]\nand calls ReleaseTerminal() before invoking onToggle (which calls\ntuiMux.switchTo for RunPassthrough), then RestoreTerminal() after.\nThis prevents the previous data corruption where BubbleTea cancelreader\nand RunPassthrough stdin reader goroutines concurrently read os.Stdin.\n\nChanges:\n- Extract _onToggle callback from startWizard for testability\n- Remove manual Ctrl+] handler from JS update function\n- Add ToggleReturn message handler for skip notifications\n- Rewrite 8 tests across 3 files to exercise _onToggle directly\n- Add termmux audit document (scratch/t394-termmux-audit.md)\n\nT394'

.PHONY: commit-t395
commit-t395: ## Stage and commit T395
commit-t395: SHELL := /bin/bash
commit-t395:
	cd $(PROJECT_ROOT) && git add \
		internal/command/pick_and_place_harness_test.go \
		internal/command/shooter_game_unix_test.go \
		internal/command/prompt_flow_editor_test.go \
		blueprint.json && \
	git add -f WIP.md config.mk && \
	git commit -m $$'Skip slow E2E tests under -short flag\n\nAdd skipSlow(t) to the three harness/binary builder functions that\nall heavy E2E tests funnel through: NewPickAndPlaceHarness,\nbuildTestBinary, and buildPromptFlowTestBinary. These tests build\nthe full osm binary and launch PTY terminal harnesses, taking\n7-28 seconds each across ~45 test functions.\n\nWith -short flag, the test suite drops from 601s (timeout) to 105s.\n3188 tests pass, 427 skip (including these and pre-existing -short\nskips), 0 fail. No build tag exclusions per CLAUDE.md constraint.\n\nT395'

.PHONY: git-stage-t393
git-stage-t393: ## Stage T393 Ask Claude fix files
	cd $(PROJECT_ROOT) && git add \
		internal/command/pr_split_10d_pipeline_orchestrator.js \
		internal/command/pr_split_16c_tui_handlers_verify.js \
		internal/command/pr_split_16d_tui_handlers_claude.js \
		internal/command/pr_split_16e_tui_update.js \
		internal/command/pr_split_16_claude_attach_test.go \
		blueprint.json && \
	git diff --stat --staged

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
