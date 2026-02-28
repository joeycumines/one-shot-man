# IMPORTANT: the build must succeed on all supported OSes (Linux, Windows,
# macOS). Use the provided `make-all-in-container` and `make-all-run-windows`
# targets to verify platform-specific behavior. `make-all-with-log` for this
# platform IN ADDITION to the external-dependency-requiring targets which CAN
# and MUST be run on the host OS, that are defined in project.mk.

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

.PHONY: test-pr-split-pty
test-pr-split-pty: ## Run only PTY-related pr-split tests (deadlock regression + integration)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestPTY_AutoSplit|TestProcess_Write' ./internal/command/... ./internal/builtin/pty/...

.PHONY: commit-staged
commit-staged: ## Commit staged changes using scratch/commit-msg.txt
	cd $(PROJECT_ROOT) && git add -A && git commit -F scratch/commit-msg.txt

.PHONY: amend-commit
amend-commit: ## Amend the last commit message using scratch/commit-msg.txt
	cd $(PROJECT_ROOT) && git commit --amend -F scratch/commit-msg.txt

.PHONY: test-complex-project
test-complex-project: ## Run complex Go project heuristic split integration test
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=300s -run 'TestIntegration_ComplexGoProject_HeuristicSplit' ./internal/command/...

.PHONY: git-status
git-status: ## Show git diff stats
	cd $(PROJECT_ROOT) && git add -A && git diff --stat HEAD && echo "---DIFF---" && git diff --cached HEAD

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

.PHONY: test-termmux-ui
test-termmux-ui: ## Run termmux/ui package tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s ./internal/termmux/ui/... 2>&1 | tail -100

.PHONY: test-termmux
test-termmux: ## Run all termmux package tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s ./internal/termmux/... 2>&1 | tail -100

.PHONY: test-spawn-args
test-spawn-args: ## Run spawn arg and health check integration tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestIntegration_SpawnArgs|TestIntegration_SpawnHealthCheck' ./internal/command/... 2>&1 | tail -50

.PHONY: test-isalive-guards
test-isalive-guards: ## Run isAlive guard tests (T021) and sendToHandle TUI path tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestIntegration_IsAliveGuard|TestPrSplitCommand_ClaudeCommand/dead_handle|TestIntegration_SendToHandle_TUI' ./internal/command/... 2>&1 | tail -80

.PHONY: test-statusbar
test-statusbar: ## Run statusbar package tests including concurrent access
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=60s ./internal/termmux/statusbar/... 2>&1 | tail -80

.PHONY: test-mcp-instance
test-mcp-instance: ## Run mcp-instance command tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=60s -run TestMCPInstanceCommand ./internal/command/... 2>&1 | tail -50

.PHONY: test-cleanup-executor
test-cleanup-executor: ## Run cleanupExecutor ordering integration tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestIntegration_CleanupExecutor' ./internal/command/... 2>&1 | tail -50

.PHONY: test-sync-utils
test-sync-utils: ## Run sync utility function unit tests (matchEntry, deduplicatePath, discoverEntries, printConfigDiffSummary)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=60s -run 'TestMatchEntry|TestDeduplicatePath|TestDiscoverEntries|TestPrintConfigDiffSummary' ./internal/command/... 2>&1 | tail -100

.PHONY: test-batch5
test-batch5: ## Run batch 5 coverage gap tests (containsGlobMeta, configKeys, session confirm-abort)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=60s -run 'TestContainsGlobMeta|TestConfigKeys|TestSessionClean_ConfirmAbort|TestSessionPurge_ConfirmAbort' ./internal/command/... 2>&1 | tail -100

.PHONY: test-batch6
test-batch6: ## Run batch 6 JS bridge coverage tests (context/output/logging APIs)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestJsContextAddPath$$|TestJsContextRemovePath$$|TestJsContextRefreshPath$$|TestJsContextToTxtar$$|TestJsContextGetStats$$|TestJsContextFilterPaths$$|TestJsContextGetFilesByExtension$$|TestJsOutputPrint$$|TestJsOutputPrintf$$|TestJsLogWarn$$|TestJsLogError$$|TestJsLogPrintf$$|TestJsLogSearch$$|TestJsGetLogs_WithCount|TestJsGetLogs_ZeroCount' ./internal/scripting/... 2>&1 | tail -100

.PHONY: test-batch7
test-batch7: ## Run batch 7 tui completion/parsing coverage tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestGetFilepathSuggestions|TestGetExecutableSuggestions|TestIsUndefined|TestCurrentWord|TestTokenizeCommandLine' ./internal/scripting/... 2>&1 | tail -100

.PHONY: test-batch8
test-batch8: ## Run batch 8 VTerm CSI/ESC dispatch coverage tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestCSI_CUD|TestCSI_CNL|TestCSI_CPL|TestCSI_EL_|TestCSI_IL_|TestCSI_DL_|TestCSI_SU_|TestCSI_SD_|TestCSI_CUP_AliasF|TestCSI_SM_RM_NonPrivate|TestCSI_DECRST_Cursor|TestCSI_DECRST_Alt|TestESC_IND|TestESC_NEL|TestScreen_EraseLine_Mode|TestScreen_EraseDisplay_Mode' ./internal/termmux/vt/... 2>&1 | tail -100

.PHONY: test-batch9
test-batch9: ## Run batch 9 edge case tests (SGRDiff, Parser, Screen boundaries)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestSGRDiff_NoOp|TestSGRDiff_DimRemoved|TestSGRDiff_UnderRemoved|TestSGRDiff_BlinkRemoved|TestSGRDiff_InverseRemoved|TestSGRDiff_HiddenRemoved|TestSGRDiff_StrikeRemoved|TestSGRDiff_FGRevert|TestSGRDiff_BGRevert|TestSGRDiff_AllFlags|TestSGRDiff_Kind8_Bright|TestSGRDiff_256_BG|TestSGRDiff_RGB_BG|TestSGRDiff_ColorKind|TestParseSGR_Extended|TestParseSGR_Truncated|TestParseSGR_AllClear|TestParseSGR_Code2|TestParser_DEL|TestParser_HighByte|TestParser_ESC_Inside|TestParser_CSI_Intermediate|TestParser_OSC_ESC|TestParser_OSC_Max|TestParser_DCS_ESC|TestParser_Escape_Unrec|TestParser_Escape_High|TestScreen_EraseLine_OutOf|TestScreen_EraseChars_Zero|TestScreen_InsertChars_Huge|TestScreen_DeleteChars_Huge|TestScreen_LineFeed_Mid|TestScreen_LineFeed_Bottom|TestScreen_ReverseIndex_Mid|TestScreen_ReverseIndex_Top|TestScreen_Resize_Saved' ./internal/termmux/vt/... 2>&1 | tail -150

.PHONY: test-batch10
test-batch10: ## Run batch 10 tests (handleControl, scroll, diff splitter, countRelSegments)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestHandleControl_|TestScrollRegion|TestMakeDefaultTabStops|TestSwitchToAlt|TestSwitchToPrimary' ./internal/termmux/vt/... 2>&1 | tail -80
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestSplitIntoFileDiffs|TestSplitFileAtHunks|TestCountRelSegments|TestExtractFileName|TestCountLines' ./internal/command/... 2>&1 | tail -80

.PHONY: git-status-short
git-status-short: ## Show git status (short format, no staging)
	cd $(PROJECT_ROOT) && git status --short

.PHONY: git-stage-vt-dispatch
git-stage-vt-dispatch: ## Stage vt dispatch_coverage_test.go and config.mk
	cd $(PROJECT_ROOT) && git add internal/termmux/vt/dispatch_coverage_test.go && git add -f config.mk

.PHONY: git-staged-stat
git-staged-stat: ## Show staged diff stat
	cd $(PROJECT_ROOT) && git diff --staged --stat

.PHONY: commit-batch10
commit-batch10: ## Stage and commit batch10 test files
	cd $(PROJECT_ROOT) && git add internal/termmux/vt/control_scroll_test.go internal/command/coverage_gaps_batch10_splitter_test.go config.mk && git commit -F scratch/commit-msg-batch10.txt && git log --oneline -1

.PHONY: test-batch11
test-batch11: ## Run batch 11 tests (safety internals + ring buffer)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestClassifyIntent_|TestAssessScope_|TestEnforcePolicy_|TestCheckAllowedPaths_|TestCalculateRisk_' ./internal/builtin/claudemux/... 2>&1 | tail -100
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestGetFlatHistoryInternal_' ./internal/scripting/... 2>&1 | tail -80

.PHONY: test-batch12
test-batch12: ## Run batch 12 tests (writeResolvedTable, renderHelpBar, tickCmd)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestWriteResolvedTable_' ./internal/command/... 2>&1 | tail -50
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestRenderHelpBar_|TestTickCmd_' ./internal/termmux/ui/... 2>&1 | tail -50

.PHONY: commit-batch12
commit-batch12: ## Stage and commit batch12 test files
	cd $(PROJECT_ROOT) && git add internal/command/coverage_gaps_batch12_table_test.go internal/termmux/ui/coverage_gaps_batch12_test.go && git add -f config.mk && git commit -F scratch/commit-msg-batch12.txt && git log --oneline -3

.PHONY: test-batch13
test-batch13: ## Run batch 13 tests (code generator: keys/mouse generation, extraction)
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestGenerateKeysOutput_|TestGenerateMouseOutput_|TestExtractMouseMap_|TestExtractKeyNamesMap_' ./internal/cmd/generate-bubbletea-key-mapping/... 2>&1 | tail -100

.PHONY: commit-batch13
commit-batch13: ## Stage and commit batch13 test files
	cd $(PROJECT_ROOT) && git add internal/cmd/generate-bubbletea-key-mapping/main_test.go internal/command/coverage_gaps_batch10_test.go blueprint.json WIP.md && git add -f config.mk && git diff --staged --stat && git commit -F scratch/commit-msg-batch13.txt && git log --oneline -3

.PHONY: test-t068
test-t068: ## Run T068/T069/T070 wall-clock timeout and heartbeat tests
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestPrSplitCommand_ResolveConflictsWallClock|TestPrSplitCommand_ResolveConflictsWithClaudeWallClock|TestPrSplitCommand_PollForFile' ./internal/command/... 2>&1 | tail -80

.PHONY: test-t072
test-t072: ## Run T072-T128 tests: dependency, verification, MCP, timeout, sendToHandle, EAGAIN, retry, type-change, aliveCheckFn, auto-detect, resume, crash-recovery, conversation persistence, verify TUI output, progress feedback, branch restore, cancellation, AUTOMATED_DEFAULTS, sanitize, shellQuote, classificationToGroups, maxReSplits, pollInterval
	$(GO) -C $(PROJECT_ROOT) test -v -race -timeout=120s -run 'TestCreateSplitPlan|TestVerifySplits_Skips|TestVerifySplits_PerBranch|TestPrSplitCommand_FlagDefaults|TestPrSplitCommand_VerifyTimeout|TestMCPServer_ReportResolution|TestRenderConflictPrompt|TestPrSplitCommand_ResolveConflictsWithClaude|TestPrSplitCommand_ResolveConflicts_Timeout|TestPrSplitCommand_SendToHandle|TestPrSplitCommand_SetSendEnterDelay|TestPrSplitCommand_ResolveConflicts_PerBranch|TestPrSplitCommand_ResolveConflicts_AliveCheck|TestExecuteSplit_TypeChange|TestPrSplitCommand_ClaudeAutoDetect|TestAutoSplit_SaveAndResume|TestAutoSplit_CrashRecovery|TestPrSplitCommand_ResumeFlag|TestSessionPersistence_ConversationHistory|TestVerifySplit_TUIOutput|TestExecuteSplit_ProgressFeedback|TestResolveConflicts_RestoresBranchOnError|TestResolveConflicts_CancellationDuringStrategyLoop|TestExecuteSplit_CancellationMidFile|TestAutomatedDefaults_OverrideChain|TestVerifySplits_EmptyPlan|TestSanitizeBranchName_EdgeCases|TestShellQuote_EdgeCases|TestClassificationToGroups_EdgeCases|TestAutoSplit_NegativeMaxReSplits|TestPollForFile_MinInterval' ./internal/command/... 2>&1 | tail -200

.PHONY: commit-t068-t075
commit-t068-t075: ## Stage and commit T068-T075 batch
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js internal/command/pr_split.go internal/command/pr_split_test.go internal/command/pr_split_planning_test.go internal/command/pr_split_verification_test.go blueprint.json WIP.md && \
	git add -f config.mk && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg-t068-t075.txt && \
	git log --oneline -3

.PHONY: commit-t076-t079
commit-t076-t079: ## Stage and commit T076-T079 batch
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add internal/command/mcp.go && \
	git add internal/command/mcp_test.go && \
	git add internal/command/pr_split_prompt_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit \
		-m 'Add pre-existing failure support, thread timeouts to strategies' \
		-m 'Add preExistingFailure field to reportResolution MCP tool schema, allowing Claude to indicate failures that exist on the base branch. resolveConflictsWithClaude records pre-existing failures without retrying or triggering re-split.' \
		-m 'Thread resolveTimeoutMs and pollIntervalMs through resolveConflicts to strategy.fix() via a strategyOptions parameter. The claude-fix strategy now uses dynamic timeouts from the caller instead of hardcoded AUTOMATED_DEFAULTS values.' \
		-m 'Update CONFLICT_RESOLUTION_PROMPT_TEMPLATE to document the new preExistingFailure option.' \
		-m 'Tasks: T076, T077, T078, T079' && \
	git log --oneline -1

.PHONY: commit-t080-t082
commit-t080-t082: ## Stage and commit T080-T082 batch
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit \
		-m 'Add configurable send delay, EAGAIN retry, per-branch retry budget' \
		-m 'Make sendToHandle() Enter delay configurable via module-level SEND_ENTER_DELAY_MS and setSendEnterDelay() setter. Add EAGAIN retry logic to sendToHandle() fallback path with 3 retries at 10ms intervals for transient PTY write buffer errors.' \
		-m 'Implement per-branch retry budget in resolveConflicts() via perBranchRetryBudget option (default 2). Each branch gets its own retry counter instead of sharing a global budget. A while-loop wrapper around the strategy iteration enables re-running strategies with updated verify output. Global retryBudget retained as an upper bound.' \
		-m 'Tasks: T080, T081, T082' && \
	git log --oneline -1

.PHONY: commit-t083-t086
commit-t083-t086: ## Stage and commit T083-T086 batch
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg-t083-t086.txt && \
	git log --oneline -1

.PHONY: commit-verify-timeout
commit-verify-timeout: ## Stage and commit verify timeout guard files
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add internal/command/pr_split_verification_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg.txt && \
	git log --oneline -1

.PHONY: commit-t091-t092
commit-t091-t092: ## Stage and commit T091-T092 batch (type-change + aliveCheckFn)
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add internal/command/pr_split_execution_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg-t091-t092.txt && \
	git log --oneline -3

.PHONY: commit-t093-t096
commit-t093-t096: ## Stage and commit T093-T096 batch (auto-detect, resume, checkpointing)
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split.go && \
	git add internal/command/pr_split_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg-t093-t096.txt && \
	git log --oneline -3

.PHONY: commit-t097-t105
commit-t097-t105: ## Stage and commit T097-T105 batch (callbacks, cancellation, persistence)
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add -f config.mk && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg-t097-t105.txt && \
	git log --oneline -3

.PHONY: commit-t110-t128
commit-t110-t128: ## Stage and commit T110-T128 batch (input validation, edge-case unit tests)
	cd $(PROJECT_ROOT) && \
	git add internal/command/pr_split_script.js && \
	git add internal/command/pr_split_test.go && \
	git add -f config.mk && \
	git add blueprint.json && \
	git add WIP.md && \
	git diff --staged --stat && \
	git commit -F scratch/commit-msg-t110-t128.txt && \
	git log --oneline -3

# IF YOU NEED A CUSTOM TARGET, DEFINE IT ABOVE THIS LINE, AFTER THE `##@ Custom Targets`
endif
