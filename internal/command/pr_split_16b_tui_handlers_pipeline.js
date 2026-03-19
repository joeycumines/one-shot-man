'use strict';
// pr_split_16b_tui_handlers_pipeline.js — TUI: async pipeline handlers (analysis, execution, equiv, PR creation)
// Dependencies: chunks 00-16a must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    // Cross-chunk imports.
    var tea = prSplit._tea;
    var st = prSplit._state;
    var C = prSplit._TUI_CONSTANTS;
    var handleConfigState = prSplit._handleConfigState;
    var handlePlanReviewState = prSplit._handlePlanReviewState;

    // -----------------------------------------------------------------------
    //  Report Formatting — Human-readable display for the report overlay
    // -----------------------------------------------------------------------

    function formatReportForDisplay(report) {
        if (!report) {
            return 'Report generation failed: no data available.\n\nPress Esc to close.';
        }

        var lines = [];

        lines.push('PR Split Report');
        lines.push('═══════════════════════════════════════');
        lines.push('');

        // Metadata.
        lines.push('Version:    ' + (report.version || 'unknown'));
        lines.push('Base:       ' + (report.baseBranch || '—'));
        lines.push('Strategy:   ' + (report.strategy || '—'));
        lines.push('Dry Run:    ' + (report.dryRun ? 'yes' : 'no'));
        lines.push('');

        // Analysis.
        if (report.analysis) {
            lines.push('Analysis');
            lines.push('───────────────────────────────────────');
            lines.push('  Source Branch:  ' + (report.analysis.currentBranch || '—'));
            lines.push('  Base Branch:    ' + (report.analysis.baseBranch || '—'));
            lines.push('  File Count:     ' + (report.analysis.fileCount || 0));
            if (report.analysis.files && report.analysis.files.length > 0) {
                lines.push('');
                for (var fi = 0; fi < report.analysis.files.length; fi++) {
                    var f = report.analysis.files[fi];
                    if (!f) continue;
                    var status = '';
                    if (report.analysis.fileStatuses && report.analysis.fileStatuses[f]) {
                        status = ' (' + report.analysis.fileStatuses[f] + ')';
                    }
                    lines.push('    ' + f + status);
                }
            }
            lines.push('');
        }

        // Groups.
        if (report.groups && report.groups.length > 0) {
            lines.push('Groups');
            lines.push('───────────────────────────────────────');
            for (var gi = 0; gi < report.groups.length; gi++) {
                var g = report.groups[gi];
                if (!g) continue;
                lines.push('  ' + (g.name || '(unnamed)') + ' (' + (g.files ? g.files.length : 0) + ' files)');
                if (g.files) {
                    for (var gfi = 0; gfi < g.files.length; gfi++) {
                        if (g.files[gfi]) lines.push('    ' + g.files[gfi]);
                    }
                }
            }
            lines.push('');
        }

        // Plan.
        if (report.plan) {
            lines.push('Split Plan (' + (report.plan.splitCount || 0) + ' splits)');
            lines.push('───────────────────────────────────────');
            if (report.plan.splits) {
                for (var pi = 0; pi < report.plan.splits.length; pi++) {
                    var sp = report.plan.splits[pi];
                    if (!sp) continue;
                    lines.push('  ' + (pi + 1) + '. ' + (sp.name || '(unnamed)'));
                    lines.push('     Message:  ' + (sp.message || '—'));
                    lines.push('     Files:    ' + (sp.files ? sp.files.length : 0));
                    if (sp.files) {
                        for (var sfi = 0; sfi < sp.files.length; sfi++) {
                            if (sp.files[sfi]) lines.push('       ' + sp.files[sfi]);
                        }
                    }
                }
            }
            lines.push('');
        }

        // Equivalence.
        if (report.equivalence) {
            lines.push('Equivalence Check');
            lines.push('───────────────────────────────────────');
            lines.push('  Verified:     ' + (report.equivalence.verified ? 'YES ✅' : 'NO ⚠'));
            if (report.equivalence.splitTree) {
                lines.push('  Split Tree:   ' + report.equivalence.splitTree);
            }
            if (report.equivalence.sourceTree) {
                lines.push('  Source Tree:  ' + report.equivalence.sourceTree);
            }
            if (report.equivalence.error) {
                lines.push('  Error:        ' + report.equivalence.error);
            }
            lines.push('');
        }

        lines.push('═══════════════════════════════════════');
        lines.push('Press c to copy • Esc to close');

        return lines.join('\n');
    }

    // -----------------------------------------------------------------------
    //  Async Pipeline Handlers — drive wizard state machine
    // -----------------------------------------------------------------------

    function startAnalysis(s) {
        s.isProcessing = true;
        s.analysisProgress = 0;
        s.analysisStartedAt = Date.now();  // T002: track start time for timeout
        s.analysisSlowWarning = false;     // T002: reset slow warning
        s.configValidationError = null; // T43: clear previous validation error on retry
        s.availableBranches = [];       // T43: clear branch list on retry
        s.analysisSteps = [
            { label: 'Verify baseline', active: true, done: false },
            { label: 'Analyze diff', active: false, done: false },
            { label: 'Group files', active: false, done: false },
            { label: 'Generate plan', active: false, done: false },
            { label: 'Validate plan', active: false, done: false }
        ];

        // Run config state handler.
        // Pass outputFn to suppress output.print (which would corrupt
        // BubbleTea terminal). Use log.printf instead — safe in TUI context.
        var configResult = handleConfigState({
            baseBranch: prSplit.runtime.baseBranch,
            dir: prSplit.runtime.dir,
            strategy: prSplit.runtime.strategy,
            verifyCommand: prSplit.runtime.verifyCommand,
            outputFn: function(s) { log.printf('wizard: %s', s); }
        });

        if (configResult.error) {
            // T43: Stay on CONFIG with inline validation error instead of jumping to ERROR.
            s.isProcessing = false;
            s.configValidationError = configResult.error;
            if (configResult.availableBranches) {
                s.availableBranches = configResult.availableBranches;
            }
            s.wizardState = 'CONFIG';
            return [s, null];
        }

        // T090: Stash baseline verify config on the model so
        // runAnalysisAsync can run it asynchronously (non-blocking).
        s._baselineVerifyConfig = configResult.baselineVerifyConfig || null;

        // Transition to CONFIG if needed.
        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }

        // Launch all analysis steps as an async pipeline on the event loop.
        // The Promise resolves when all steps complete. We poll for
        // completion via tea.tick so BubbleTea can render progress, animate
        // the spinner, and let the user cancel with Ctrl+C.
        s.analysisRunning = true;
        s.analysisError = null;

        // T44: Install global output capture to pipe git command output to Output tab.
        prSplit._outputCaptureFn = function(line) {
            s.outputLines.push(line);
            // Cap output buffer to prevent unbounded memory growth.
            if (s.outputLines.length > C.OUTPUT_BUFFER_CAP) {
                s.outputLines = s.outputLines.slice(-4000);
            }
            // Auto-scroll to bottom when new output arrives.
            if (s.outputAutoScroll) {
                s.outputViewOffset = 0;
            }
        };

        runAnalysisAsync(s).then(
            function() {
                s.analysisRunning = false;
            },
            function(err) {
                s.analysisError = (err && err.message) ? err.message : String(err);
                s.analysisRunning = false;
            }
        );

        // Poll at tick interval for responsive spinner animation.
        return [s, tea.tick(C.TICK_INTERVAL_MS, 'analysis-poll')];
    }

    // runAnalysisAsync: Runs all 5 analysis steps as an async function.
    // Step 0: Verify baseline (non-blocking via verifySplitAsync).
    // Steps 1-4: analyzeDiffAsync, applyStrategyAsync, createSplitPlanAsync,
    // validatePlan. Updates s.analysisSteps progress between each step so
    // the poll handler can render progress.
    async function runAnalysisAsync(s) {
        // ── Step 0: Verify baseline (T090: non-blocking via verifySplitAsync) ──
        var bvc = s._baselineVerifyConfig;
        if (bvc && bvc.verifyCommand && bvc.verifyCommand !== 'true') {
            s.analysisSteps[0].active = true;
            var baseStart = Date.now();
            try {
                var baselineResult = await prSplit.verifySplitAsync(prSplit.runtime.baseBranch, {
                    verifyCommand: bvc.verifyCommand,
                    dir: bvc.dir,
                    verifyTimeoutMs: bvc.verifyTimeoutMs,
                    outputFn: function(line) { log.printf('wizard: %s', line); }
                });
                if (!baselineResult.passed) {
                    s.isProcessing = false;
                    s.configValidationError = 'Baseline verification failed: ' +
                        (baselineResult.error || 'exit code non-zero') +
                        (baselineResult.output ? '\n' + baselineResult.output : '');
                    s.wizardState = 'CONFIG';
                    return;
                }
            } catch (e) {
                if (s.wizard.current === 'CANCELLED') return;
                s.isProcessing = false;
                s.configValidationError = 'Baseline verify error: ' + (e.message || String(e));
                s.wizardState = 'CONFIG';
                return;
            }
            s.analysisSteps[0].done = true;
            s.analysisSteps[0].active = false;
            s.analysisSteps[0].elapsed = Date.now() - baseStart;
            log.printf('wizard: baseline verify OK (%dms)', s.analysisSteps[0].elapsed);
        } else {
            // No verify command — skip baseline step.
            s.analysisSteps[0].done = true;
            s.analysisSteps[0].active = false;
            s.analysisSteps[0].elapsed = 0;
        }
        s.analysisProgress = 0.1;

        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

        // ── Step 1: Analyze diff (I/O-bound: git rev-parse, merge-base, diff) ──
        s.analysisSteps[1].active = true;
        var analysisStart = Date.now();
        try {
            st.analysisCache = await prSplit.analyzeDiffAsync({ baseBranch: prSplit.runtime.baseBranch });
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // wizard already cancelled
            s.isProcessing = false;
            s.errorDetails = 'Analysis failed: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }
        s.analysisSteps[1].done = true;
        s.analysisSteps[1].active = false;
        s.analysisSteps[1].elapsed = Date.now() - analysisStart;
        s.analysisProgress = 0.3;

        // Check for cancellation between steps.
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;

        if (st.analysisCache.error) {
            s.isProcessing = false;
            s.errorDetails = st.analysisCache.error;
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }
        if (!st.analysisCache.files || st.analysisCache.files.length === 0) {
            s.isProcessing = false;
            s.errorDetails = 'No changes found between branches.';
            s.wizardState = 'CONFIG';
            return;
        }

        // ── Step 2: Group files (T092: async for dependency/auto strategies) ──
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;
        s.analysisSteps[2].active = true;
        var groupStart = Date.now();
        try {
            // T099: When strategy is 'auto', use selectStrategyAsync to capture
            // needsConfirm and scored alternatives for TUI display.
            if (prSplit.runtime.strategy === 'auto') {
                var autoResult = await prSplit.selectStrategyAsync(
                    st.analysisCache.files);
                st.groupsCache = autoResult.groups;
                s.strategyNeedsConfirm = autoResult.needsConfirm || false;
                s.strategyAlternatives = autoResult.scored || [];
                s.autoStrategyName = autoResult.strategy || '';
            } else {
                st.groupsCache = await prSplit.applyStrategyAsync(
                    st.analysisCache.files, prSplit.runtime.strategy);
            }
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // T001: guard
            s.isProcessing = false;
            s.errorDetails = 'Grouping failed: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }
        s.analysisSteps[2].done = true;
        s.analysisSteps[2].active = false;
        s.analysisSteps[2].elapsed = Date.now() - groupStart;
        s.analysisProgress = 0.55;

        // ── Step 3: Create plan (I/O-bound: optional git rev-parse) ──
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;
        s.analysisSteps[3].active = true;
        var planStart = Date.now();
        try {
            st.planCache = await prSplit.createSplitPlanAsync(st.groupsCache, {
                baseBranch: prSplit.runtime.baseBranch,
                sourceBranch: st.analysisCache.currentBranch,
                branchPrefix: prSplit.runtime.branchPrefix,
                verifyCommand: prSplit.runtime.verifyCommand,
                fileStatuses: st.analysisCache.fileStatuses
            });
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // T001: guard
            s.isProcessing = false;
            s.errorDetails = 'Plan creation failed: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }
        s.analysisSteps[3].done = true;
        s.analysisSteps[3].active = false;
        s.analysisSteps[3].elapsed = Date.now() - planStart;
        s.analysisProgress = 0.8;

        // ── Step 4: Validate plan (pure compute, non-blocking) ──
        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return;
        s.analysisSteps[4].active = true;
        var validation = prSplit.validatePlan(st.planCache);
        s.analysisSteps[4].done = true;
        s.analysisSteps[4].active = false;
        s.analysisProgress = 1.0;

        if (!validation.valid) {
            s.isProcessing = false;
            s.errorDetails = 'Plan validation failed: ' + validation.errors.join('; ');
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }

        s.isProcessing = false;

        // Transition wizard to PLAN_GENERATION then PLAN_REVIEW.
        // Guard against CANCELLED (user may have cancelled during last step).
        if (s.wizard.current === 'CANCELLED') return;
        if (s.wizard.current === 'CONFIG') {
            s.wizard.transition('PLAN_GENERATION');
        }
        s.wizard.transition('PLAN_REVIEW');
        s.wizardState = 'PLAN_REVIEW';
    }

    // handleAnalysisPoll: Called every 100ms to check if the async
    // analysis pipeline has completed. Updates spinner animation,
    // checks for timeout warning, and handles cancellation.
    var ANALYSIS_TIMEOUT_MS = 60000; // T002: default 60s warning threshold

    function handleAnalysisPoll(s) {
        // If cancelled (Ctrl+C), stop polling.
        if (!s.isProcessing && !s.analysisRunning) {
            return [s, null];
        }

        // Still running — poll again for spinner animation.
        if (s.analysisRunning) {
            // T002: Check for slow analysis and set warning flag.
            if (s.analysisStartedAt) {
                var elapsed = Date.now() - s.analysisStartedAt;
                var threshold = (typeof prSplitConfig !== 'undefined' && prSplitConfig.analysisTimeoutMs > 0)
                    ? prSplitConfig.analysisTimeoutMs
                    : ANALYSIS_TIMEOUT_MS;
                if (elapsed >= threshold && !s.analysisSlowWarning) {
                    s.analysisSlowWarning = true;
                    s.analysisElapsedMs = elapsed;
                }
                if (s.analysisSlowWarning) {
                    s.analysisElapsedMs = elapsed;
                }
            }
            s.spinnerFrame = (s.spinnerFrame || 0) + 1;
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'analysis-poll')];
        }

        // Pipeline completed. Check for error set by .then() rejection.
        if (s.analysisError) {
            s.isProcessing = false;
            s.errorDetails = s.analysisError;
            try { s.wizard.transition('ERROR'); } catch (e) { log.debug('wizard: transition to ERROR failed: ' + (e.message || e)); }
            s.wizardState = s.wizard.current;
            return [s, null];
        }

        // Success or handled inline (error/cancel paths set state directly).
        // The async function already transitioned the wizard state.
        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Resolve Handler
    // -----------------------------------------------------------------------

    function handleResolvePoll(s) {
        if (!s.isProcessing) {
            return [s, null];
        }

        if (s.resolveRunning) {
            return [s, tea.tick(500, 'resolve-poll')];
        }

        // Resolve completed — process result.
        var result = s.resolveResult;
        s.isProcessing = false;

        if (result && result.error) {
            s.errorDetails = result.error;
            s.wizard.transition('ERROR_RESOLUTION');
            s.wizardState = 'ERROR_RESOLUTION';
            return [s, null];
        }

        // Check if any branches still have errors after auto-resolve.
        var errors = (result && result.errors) || [];
        if (errors.length > 0) {
            // Some branches still failing — re-enter error resolution.
            s.wizard.data.failedBranches = errors;
            s.errorDetails = 'Auto-resolve fixed ' +
                ((result && result.fixed) ? result.fixed.length : 0) +
                ' branch(es), but ' + errors.length + ' still failing.';
            s.wizard.transition('ERROR_RESOLUTION');
            s.wizardState = 'ERROR_RESOLUTION';
            return [s, null];
        }

        // All resolved — continue to equivalence check.
        s.wizard.transition('EQUIV_CHECK');
        s.wizardState = 'EQUIV_CHECK';
        s.isProcessing = true;
        return startEquivCheck(s);
    }

    // -----------------------------------------------------------------------
    //  Execution Handlers
    // -----------------------------------------------------------------------

    function startExecution(s) {
        if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
            s.errorDetails = 'No plan to execute.';
            return [s, null];
        }

        s.isProcessing = true;
        s.executionResults = [];
        s.executingIdx = 0;
        // Reset verification state from any prior run.
        s.verificationResults = [];
        s.verifyingIdx = -1;
        s.verifyOutput = {};
        s.expandedVerifyBranch = null;
        // Reset live verification session state.
        if (s.activeVerifySession) {
            try { s.activeVerifySession.close(); } catch (e) { log.debug('pipelineCleanup: verifySession.close failed: ' + (e.message || e)); }
        }
        if (s.activeVerifyWorktree && s.activeVerifyDir) {
            try { prSplit.cleanupVerifyWorktree(s.activeVerifyDir, s.activeVerifyWorktree); } catch (e) { log.debug('pipelineCleanup: verifyWorktree cleanup failed: ' + (e.message || e)); }
        }
        s.activeVerifySession = null;
        s.activeVerifyWorktree = null;
        s.activeVerifyBranch = null;
        s.activeVerifyDir = null;
        s.activeVerifyStartTime = 0;
        s.verifyElapsedMs = 0;
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;
        s.lastVerifyInterruptTime = 0;
        s.verifyPaused = false;  // T059

        // Transition: PLAN_REVIEW → BRANCH_BUILDING.
        if (s.wizard.current === 'PLAN_REVIEW') {
            handlePlanReviewState(s.wizard, 'approve');
        }
        s.wizardState = 'BRANCH_BUILDING';

        // Dry run check — skip execution entirely.
        if (prSplit.runtime.dryRun) {
            s.isProcessing = false;
            s.wizardState = 'FINALIZATION';
            s.wizard.transition('FINALIZATION');
            return [s, null];
        }

        // Launch async execution pipeline. executeSplitAsync uses
        // exec.spawn for non-blocking git operations. We pass a progressFn
        // that updates s.executingIdx in real-time so the poll handler can
        // render per-branch progress with spinner animation.
        s.executionRunning = true;
        s.executionError = null;
        runExecutionAsync(s).then(
            function() {
                s.executionRunning = false;
            },
            function(err) {
                s.executionError = (err && err.message) ? err.message : String(err);
                s.executionRunning = false;
            }
        );

        // Poll at tick interval for responsive spinner animation.
        return [s, tea.tick(C.TICK_INTERVAL_MS, 'execution-poll')];
    }

    // runExecutionAsync: Runs the split execution as an async function.
    // Uses executeSplitAsync (non-blocking I/O via exec.spawn) with a
    // progressFn that updates s.executingIdx for real-time per-branch
    // progress in the TUI. On completion, chains to per-branch verification
    // or equivalence check.
    async function runExecutionAsync(s) {
        var result;
        try {
            result = await prSplit.executeSplitAsync(st.planCache, {
                progressFn: function(msg) {
                    // Parse branch index from progress messages like
                    // "Creating branch 2/5: split/02-feature".
                    var match = msg.match(/(\d+)\/(\d+)/);
                    if (match) {
                        s.executingIdx = parseInt(match[1], 10) - 1;
                        s.executionBranchTotal = parseInt(match[2], 10);
                    }
                    s.executionProgressMsg = msg;
                    // T44: Pipe progress message to Output tab.
                    if (s.outputLines) {
                        s.outputLines.push('\u25b6 ' + msg);
                        if (s.outputAutoScroll) s.outputViewOffset = 0;
                    }
                }
            });
        } catch (e) {
            if (s.wizard.current === 'CANCELLED') return; // wizard already cancelled
            s.isProcessing = false;
            s.errorDetails = 'Execution error: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR_RESOLUTION'); } catch (te) { log.debug('wizard: transition to ERROR_RESOLUTION failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }

        if (!s.isProcessing || s.wizard.current === 'CANCELLED') return; // cancelled

        if (result.error) {
            s.isProcessing = false;
            s.errorDetails = result.error;
            s.wizard.data.failedBranches = result.results ?
                result.results.filter(function(r) { return r.error; }) : [];
            try { s.wizard.transition('ERROR_RESOLUTION'); } catch (te) { log.debug('wizard: transition to ERROR_RESOLUTION failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }

        st.executionResultCache = result.results;
        s.executionResults = result.results || [];

        // Chain: If verify command configured, start per-branch verification.
        // Otherwise, start the equivalence check.
        // (These set state directly; the poll handler will detect completion.)
        s.executionNextStep = prSplit.runtime.verifyCommand ? 'verify' : 'equiv';
    }

    // handleExecutionPoll: Called every 100ms to check if the async
    // execution pipeline has completed. Updates spinner animation and
    // handles cancellation.
    function handleExecutionPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing && !s.executionRunning) {
            return [s, null];
        }

        // Still running — poll again for spinner animation.
        if (s.executionRunning) {
            s.spinnerFrame = (s.spinnerFrame || 0) + 1;
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'execution-poll')];
        }

        // Pipeline completed. Check for error set by .then() rejection.
        if (s.executionError) {
            s.isProcessing = false;
            s.errorDetails = s.executionError;
            try { s.wizard.transition('ERROR_RESOLUTION'); } catch (te) { log.debug('wizard: transition to ERROR_RESOLUTION failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return [s, null];
        }

        // Check what the async function determined as the next step.
        if (s.executionNextStep === 'verify') {
            // Start per-branch verification.
            s.executionNextStep = null;
            s.verificationResults = [];
            s.verifyingIdx = 0;
            s.verifyOutput = {};
            s.expandedVerifyBranch = null;
            return [s, tea.tick(1, 'verify-branch')];
        }

        if (s.executionNextStep === 'equiv') {
            // Start equivalence check.
            s.executionNextStep = null;
            return startEquivCheck(s);
        }

        // Async function set state directly (error paths). Stop polling.
        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Equivalence Handlers
    // -----------------------------------------------------------------------

    function startEquivCheck(s) {
        // Guard: only transition if not already in EQUIV_CHECK
        // (handleResolvePoll pre-transitions for skip/resolve paths).
        if (s.wizard.current !== 'EQUIV_CHECK') {
            s.wizard.transition('EQUIV_CHECK');
        }
        s.wizardState = 'EQUIV_CHECK';

        s.equivRunning = true;
        s.equivError = null;
        runEquivCheckAsync(s).then(
            function() {
                s.equivRunning = false;
            },
            function(err) {
                s.equivError = (err && err.message) ? err.message : String(err);
                s.equivRunning = false;
            }
        );

        return [s, tea.tick(C.TICK_INTERVAL_MS, 'equiv-poll')];
    }

    // runEquivCheckAsync: Runs equivalence check as an async function.
    // T075: Uses verifyEquivalenceDetailedAsync to include per-file diff
    // info (diffFiles, diffSummary) when trees don't match.
    async function runEquivCheckAsync(s) {
        var equivResult;
        try {
            // T075: prefer detailed variant for diff information on mismatch;
            // fall back to basic if the detailed export isn't available.
            var checkFn = prSplit.verifyEquivalenceDetailedAsync || prSplit.verifyEquivalenceAsync;
            equivResult = await checkFn(st.planCache);
        } catch (e) {
            // T308: If user navigated away from EQUIV_CHECK, don't mutate state.
            if (s.wizard.current === 'CANCELLED' || s.wizardState !== 'EQUIV_CHECK') return;
            s.isProcessing = false;
            s.errorDetails = 'Equivalence check failed: ' + (e.message || String(e));
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }

        // T308: If user navigated away, don't mutate state.
        if (!s.isProcessing || s.wizard.current === 'CANCELLED' || s.wizardState !== 'EQUIV_CHECK') return;

        // Defensive: treat null/undefined result as error.
        if (!equivResult) {
            s.isProcessing = false;
            s.errorDetails = 'Equivalence check returned no result.';
            try { s.wizard.transition('ERROR'); } catch (te) { log.debug('wizard: transition to ERROR failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
            return;
        }

        // Annotate with skip information.
        var skipped = s.wizard.data && s.wizard.data.skippedBranches;
        if (skipped && skipped.length > 0) {
            equivResult.skippedBranches = skipped.map(function(b) { return b.name || b; });
            equivResult.incomplete = true;
        }
        s.wizard.data.equivalence = equivResult;
        s.equivalenceResult = equivResult;
        st.equivalenceResult = equivResult; // T089: cache on shared state for buildReport()

        s.isProcessing = false;

        // T061: On PASS, auto-transition to FINALIZATION.
        // On FAIL/mismatch, stay on EQUIV_CHECK so user can interact
        // with Re-verify/Revise Plan/Continue buttons.
        if (equivResult.equivalent) {
            try { s.wizard.transition('FINALIZATION', { equivalence: equivResult }); } catch (te) { log.debug('wizard: transition to FINALIZATION failed: ' + (te.message || te)); }
            s.wizardState = s.wizard.current;
        }
        // On mismatch: wizardState stays EQUIV_CHECK, view shows results + buttons.
    }

    // handleEquivPoll: Called every 100ms to check if the async
    // equivalence check has completed.
    function handleEquivPoll(s) {
        // If cancelled, stop polling.
        if (!s.isProcessing && !s.equivRunning) {
            return [s, null];
        }

        // Still running — poll again.
        if (s.equivRunning) {
            return [s, tea.tick(C.TICK_INTERVAL_MS, 'equiv-poll')];
        }

        // Pipeline completed. Check for error.
        if (s.equivError) {
            s.isProcessing = false;
            s.errorDetails = 'Equivalence check failed: ' + s.equivError;
            try { s.wizard.transition('ERROR'); } catch (e) { log.debug('wizard: transition to ERROR failed: ' + (e.message || e)); }
            s.wizardState = s.wizard.current;
            return [s, null];
        }

        // Success — state already transitioned by runEquivCheckAsync.
        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  PR Creation Handlers
    // -----------------------------------------------------------------------

    function startPRCreation(s) {
        // Guard: already running or already completed.
        if (s.prCreationRunning) return [s, null];
        if (s.prCreationResults) return [s, null];

        // Check prerequisites.
        if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
            s.prCreationError = 'No plan available \u2014 run Execute Plan first.';
            return [s, null];
        }

        s.prCreationRunning = true;
        s.prCreationError = null;
        s.prCreationResults = null;
        s.prCreationProgressMsg = '';

        // Build effective plan: filter out skipped branches.
        var effectivePlan = st.planCache;
        if (st.skipBranchSet) {
            var skipSet = st.skipBranchSet;
            var filtered = [];
            for (var f = 0; f < effectivePlan.splits.length; f++) {
                if (!skipSet[effectivePlan.splits[f].name]) {
                    filtered.push(effectivePlan.splits[f]);
                }
            }
            if (filtered.length < effectivePlan.splits.length) {
                log.info('[pr-split] Excluding ' +
                    (effectivePlan.splits.length - filtered.length) +
                    ' skipped branch(es) from PR creation.');
            }
            effectivePlan = Object.assign({}, effectivePlan, { splits: filtered });
        }

        // Collect options from runtime config.
        var opts = {
            draft: prSplit.runtime.draft !== false,
            pushOnly: prSplit.runtime.pushOnly || false,
            autoMerge: prSplit.runtime.autoMerge || false,
            mergeMethod: prSplit.runtime.mergeMethod || 'squash',
            dryRun: prSplit.runtime.dryRun || false,  // T077
            progressFn: function(msg) {
                s.prCreationProgressMsg = msg;
            }
        };

        // T076: Fully async — uses exec.spawn for all git/gh operations.
        prSplit.createPRsAsync(effectivePlan, opts).then(function(result) {
            s.prCreationResults = result.results || [];
            s.prCreationDryRun = result.dryRun || false;  // T077
            if (result.error) {
                s.prCreationError = result.error;
            }
            s.prCreationRunning = false;
        })['catch'](function(err) {
            s.prCreationError = (err && err.message) ? err.message : String(err);
            s.prCreationRunning = false;
        });

        return [s, tea.tick(200, 'pr-creation-poll')];
    }

    function handlePRCreationPoll(s) {
        // Still running — keep polling for spinner animation + progress.
        if (s.prCreationRunning) {
            return [s, tea.tick(200, 'pr-creation-poll')];
        }
        // Done — no further ticks needed. View will read prCreationResults.
        return [s, null];
    }

    // Cross-chunk exports.
    prSplit._formatReportForDisplay = formatReportForDisplay;
    prSplit._startAnalysis = startAnalysis;
    prSplit._handleAnalysisPoll = handleAnalysisPoll;
    prSplit._handleResolvePoll = handleResolvePoll;
    prSplit._startExecution = startExecution;
    prSplit._handleExecutionPoll = handleExecutionPoll;
    prSplit._startEquivCheck = startEquivCheck;
    prSplit._handleEquivPoll = handleEquivPoll;
    prSplit._startPRCreation = startPRCreation;
    prSplit._handlePRCreationPoll = handlePRCreationPoll;

})(globalThis.prSplit);
