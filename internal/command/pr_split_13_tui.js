'use strict';
// pr_split_13_tui.js — TUI mode: command dispatch, buildReport, mode registration
// Dependencies: all prior chunks (00-12) must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig.
//
// Guarded: only activates when tui, ctx, output, and shared are available.
// Skipped in test environments that lack these globals.
//
// Cross-chunk references (late-bound via prSplit.* inside handlers):
//   Chunks 00-11: All major functions.
//   Chunk 00: runtime, _style, _padIndex, _sanitizeBranchName,
//             _COMMAND_NAME, _MODE_NAME, _modules.shared, _modules.template

(function(prSplit) {

    // -----------------------------------------------------------------------
    //  TUI Guard
    // -----------------------------------------------------------------------

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') {
        return;
    }

    var shared = prSplit._modules.shared;
    if (!shared) return;

    // -----------------------------------------------------------------------
    //  Shared State — uses prSplit._state for cross-chunk cache coherence.
    //  The pipeline (chunk 10) writes state.analysisCache, state.planCache,
    //  etc. during automatedSplit, and TUI commands below read/write the
    //  same properties so the REPL reflects pipeline results.
    // -----------------------------------------------------------------------

    var st = prSplit._state;

    var tuiState = tui.createState(prSplit._COMMAND_NAME, {
        [shared.contextItems]: {defaultValue: []}
    });

    // -----------------------------------------------------------------------
    //  WizardState — Guarded state machine for the pr-split wizard.
    //
    //  States: IDLE, CONFIG, BASELINE_FAIL, PLAN_GENERATION, PLAN_REVIEW,
    //          PLAN_EDITOR, BRANCH_BUILDING, ERROR_RESOLUTION, EQUIV_CHECK,
    //          FINALIZATION, DONE, CANCELLED, FORCE_CANCEL, PAUSED, ERROR
    //
    //  See scratch/w01-state-machine.md for full design.
    // -----------------------------------------------------------------------

    // Valid transitions: { fromState: { toState: true, ... }, ... }
    var VALID_TRANSITIONS = {
        'IDLE':             { 'CONFIG': true },
        'CONFIG':           { 'PLAN_GENERATION': true, 'BASELINE_FAIL': true, 'BRANCH_BUILDING': true, 'CANCELLED': true, 'ERROR': true },
        'BASELINE_FAIL':    { 'PLAN_GENERATION': true, 'CANCELLED': true },
        'PLAN_GENERATION':  { 'PLAN_REVIEW': true, 'CANCELLED': true, 'FORCE_CANCEL': true, 'PAUSED': true, 'ERROR': true },
        'PLAN_REVIEW':      { 'PLAN_EDITOR': true, 'BRANCH_BUILDING': true, 'PLAN_GENERATION': true, 'CANCELLED': true },
        'PLAN_EDITOR':      { 'PLAN_REVIEW': true },
        'BRANCH_BUILDING':  { 'EQUIV_CHECK': true, 'ERROR_RESOLUTION': true, 'CANCELLED': true, 'FORCE_CANCEL': true, 'PAUSED': true, 'ERROR': true },
        'ERROR_RESOLUTION': { 'EQUIV_CHECK': true, 'BRANCH_BUILDING': true, 'PLAN_GENERATION': true, 'CANCELLED': true, 'FORCE_CANCEL': true },
        'EQUIV_CHECK':      { 'FINALIZATION': true, 'CANCELLED': true, 'ERROR': true },
        'FINALIZATION':     { 'FINALIZATION': true, 'DONE': true },
        'DONE':             { 'IDLE': true },
        'CANCELLED':        { 'DONE': true },
        'FORCE_CANCEL':     { 'DONE': true },
        'PAUSED':           { 'DONE': true },
        'ERROR':            { 'DONE': true }
    };

    // Terminal states — no pipeline activity, waiting for reset or dismiss.
    var TERMINAL_STATES = { 'DONE': true, 'CANCELLED': true, 'FORCE_CANCEL': true, 'PAUSED': true, 'ERROR': true };

    // States from which pause is allowed.
    var PAUSABLE_STATES = { 'PLAN_GENERATION': true, 'BRANCH_BUILDING': true };

    /**
     * WizardState — pr-split wizard state machine.
     *
     * @constructor
     */
    function WizardState() {
        this.current = 'IDLE';
        this.data = {};
        this.history = [];
        this.checkpoint = null;
        this.listeners = [];
    }

    /**
     * transition — move to a new state with guard check.
     *
     * @param {string} to - Target state name.
     * @param {Object} [mergeData] - Data to merge into this.data.
     * @throws {Error} If the transition is not in VALID_TRANSITIONS.
     */
    WizardState.prototype.transition = function(to, mergeData) {
        var from = this.current;
        var allowed = VALID_TRANSITIONS[from];
        if (!allowed || !allowed[to]) {
            throw new Error('Invalid transition: ' + from + ' \u2192 ' + to);
        }
        this.history.push({ from: from, to: to, at: Date.now() });
        this.current = to;
        if (mergeData) {
            var keys = Object.keys(mergeData);
            for (var i = 0; i < keys.length; i++) {
                this.data[keys[i]] = mergeData[keys[i]];
            }
        }
        // Notify listeners
        for (var j = 0; j < this.listeners.length; j++) {
            try { this.listeners[j](from, to, this.data); } catch (e) { log.error('WizardState listener error during transition ' + from + ' \u2192 ' + to + ' (listener ' + j + '): ' + e); }
        }
    };

    /**
     * cancel — transition to CANCELLED from any non-terminal active state.
     * No-op if already terminal.
     */
    WizardState.prototype.cancel = function() {
        if (TERMINAL_STATES[this.current]) return;
        this.transition('CANCELLED');
    };

    /**
     * forceCancel — transition to FORCE_CANCEL. Allowed from any state
     * except DONE and FORCE_CANCEL.
     */
    WizardState.prototype.forceCancel = function() {
        if (this.current === 'DONE' || this.current === 'FORCE_CANCEL') return;
        // Force cancel bypasses normal transition matrix — always allowed
        // from active states and even from CANCELLED.
        this.history.push({ from: this.current, to: 'FORCE_CANCEL', at: Date.now() });
        this.current = 'FORCE_CANCEL';
        for (var j = 0; j < this.listeners.length; j++) {
            try { this.listeners[j](this.current, 'FORCE_CANCEL', this.data); } catch (e) { /* swallow */ }
        }
    };

    /**
     * pause — transition to PAUSED if current state supports pausing.
     */
    WizardState.prototype.pause = function() {
        if (!PAUSABLE_STATES[this.current]) return;
        this.transition('PAUSED');
    };

    /**
     * error — transition to ERROR from any non-terminal state.
     * @param {string} [message] - Error description.
     */
    WizardState.prototype.error = function(message) {
        if (TERMINAL_STATES[this.current]) return;
        this.transition('ERROR', message ? { error: message } : undefined);
    };

    /**
     * isTerminal — returns true if no further pipeline activity is expected.
     * @returns {boolean}
     */
    WizardState.prototype.isTerminal = function() {
        return !!TERMINAL_STATES[this.current];
    };

    /**
     * onTransition — register a listener for state changes.
     * @param {function(string,string,Object)} fn - Called with (from, to, data).
     */
    WizardState.prototype.onTransition = function(fn) {
        if (typeof fn === 'function') this.listeners.push(fn);
    };

    /**
     * reset — return to IDLE, clear data and history.
     */
    WizardState.prototype.reset = function() {
        this.current = 'IDLE';
        this.data = {};
        this.history = [];
        this.checkpoint = null;
        this.listeners = [];
    };

    /**
     * saveCheckpoint — capture current state for resume.
     * @returns {Object} Checkpoint data.
     */
    WizardState.prototype.saveCheckpoint = function() {
        this.checkpoint = {
            state: this.current,
            data: JSON.parse(JSON.stringify(this.data)),
            at: Date.now()
        };
        return this.checkpoint;
    };

    // Export on prSplit for cross-chunk access.
    prSplit.WizardState = WizardState;
    prSplit.WIZARD_VALID_TRANSITIONS = VALID_TRANSITIONS;
    prSplit.WIZARD_TERMINAL_STATES = TERMINAL_STATES;

    // -----------------------------------------------------------------------
    //  buildReport — JSON-serializable status report
    // -----------------------------------------------------------------------

    function buildReport() {
        var runtime = prSplit.runtime;
        var report = {
            version: prSplit.VERSION || 'unknown',
            baseBranch: runtime.baseBranch,
            strategy: runtime.strategy,
            dryRun: runtime.dryRun,
            analysis: null,
            groups: null,
            plan: null
        };
        if (st.analysisCache && !st.analysisCache.error) {
            report.analysis = {
                currentBranch: st.analysisCache.currentBranch,
                baseBranch: st.analysisCache.baseBranch,
                fileCount: st.analysisCache.files.length,
                files: st.analysisCache.files,
                fileStatuses: st.analysisCache.fileStatuses || {}
            };
        }
        if (st.groupsCache) {
            var gNames = Object.keys(st.groupsCache).sort();
            report.groups = gNames.map(function(name) {
                return { name: name, files: st.groupsCache[name] };
            });
        }
        if (st.planCache) {
            report.plan = {
                splitCount: st.planCache.splits.length,
                splits: st.planCache.splits.map(function(s) {
                    return {
                        name: s.name,
                        files: s.files,
                        message: s.message,
                        order: s.order
                    };
                })
            };
            var equiv = prSplit.verifyEquivalence(st.planCache);
            report.equivalence = {
                verified: equiv.equivalent,
                splitTree: equiv.splitTree,
                sourceTree: equiv.sourceTree,
                error: equiv.error || null
            };
        }
        return report;
    }

    prSplit._buildReport = buildReport;

    // -----------------------------------------------------------------------
    //  Wizard state handlers — implement per-state logic for the wizard flow
    // -----------------------------------------------------------------------

    /**
     * handleConfigState — validates configuration and runs baseline verification.
     * Called when the wizard enters CONFIG state.
     *
     * @param {Object} config - Pipeline configuration overrides.
     * @returns {Object} { error, resume, checkpoint, baselineFailed, baselineError }
     */
    function handleConfigState(config) {
        var runtime = prSplit.runtime;
        var gitExec = prSplit._gitExec;
        var resolveDir = prSplit._resolveDir;
        var verifySplit = prSplit.verifySplit;
        var automatedDefaults = prSplit.AUTOMATED_DEFAULTS || {};
        var loadPlan = prSplit.loadPlan;
        var dir = resolveDir(config.dir || '.');

        // --- Step 1: Validate required configuration ---
        var errors = [];

        if (!runtime.baseBranch) {
            errors.push('baseBranch is required (set via config or "set baseBranch <name>")');
        }

        // Detect source branch (current branch).
        var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (branchResult.code !== 0) {
            errors.push('cannot determine current branch: ' + (branchResult.stderr || '').trim());
        } else {
            var sourceBranch = branchResult.stdout.trim();
            if (sourceBranch === runtime.baseBranch) {
                errors.push('currently on base branch (' + runtime.baseBranch + '); checkout a feature branch first');
            }
        }

        if (errors.length > 0) {
            return { error: errors.join('; ') };
        }

        // --- Step 2: Check for --resume flag ---
        if (config.resume) {
            var checkpoint = loadPlan();
            if (checkpoint && !checkpoint.error && checkpoint.plan) {
                return { resume: true, checkpoint: checkpoint };
            }
            // No valid checkpoint found — fall through to normal flow.
            log.printf('wizard: --resume specified but no valid checkpoint found; starting fresh');
        }

        // --- Step 3: Run baseline verification ---
        // Verify the base branch passes the verifyCommand before we start splitting.
        // Skip if verifyCommand is trivial ('true') or absent.
        var verifyCommand = runtime.verifyCommand;
        var verifyTimeoutMs = 0;
        if (typeof config.verifyTimeoutMs === 'number' && config.verifyTimeoutMs > 0) {
            verifyTimeoutMs = config.verifyTimeoutMs;
        } else if (typeof automatedDefaults.verifyTimeoutMs === 'number' &&
                   automatedDefaults.verifyTimeoutMs > 0) {
            verifyTimeoutMs = automatedDefaults.verifyTimeoutMs;
        }
        if (verifyCommand && verifyCommand !== 'true') {
            if (verifyTimeoutMs > 0) {
                output.print('[auto-split] Verify baseline (timeout ' + Math.ceil(verifyTimeoutMs / 1000) + 's)...');
            } else {
                output.print('[auto-split] Verify baseline...');
            }
            var baselineStart = Date.now();
            var baselineResult = verifySplit(runtime.baseBranch, {
                verifyCommand: verifyCommand,
                dir: dir,
                verifyTimeoutMs: verifyTimeoutMs,
                outputFn: function(line) { output.print(line); }
            });
            var baselineElapsedMs = Date.now() - baselineStart;
            // Worktree isolation: user's branch is never modified, no restore needed.

            if (!baselineResult.passed) {
                return {
                    baselineFailed: true,
                    baselineError: baselineResult.error || 'baseline verification failed (exit code non-zero)',
                    baselineOutput: baselineResult.output || '',
                    baselineDurationMs: baselineElapsedMs
                };
            }
            output.print('[auto-split] Verify baseline OK (' + baselineElapsedMs + 'ms)');
        }

        return { error: null };
    }

    prSplit._handleConfigState = handleConfigState;

    /**
     * handleBaselineFailState — BASELINE_FAIL state handler.
     * Called when baseline verification has failed. Offers override or abort.
     *
     * @param {WizardState} wizard - Wizard in BASELINE_FAIL state.
     * @param {string} choice - 'override' to proceed anyway, or 'abort' to cancel.
     * @returns {Object} { error?, action, state }
     */
    function handleBaselineFailState(wizard, choice) {
        if (wizard.current !== 'BASELINE_FAIL') {
            return { error: 'wizard is not in BASELINE_FAIL state (current: ' + wizard.current + ')' };
        }

        if (choice === 'override') {
            wizard.transition('PLAN_GENERATION');
            return { action: 'override', state: 'PLAN_GENERATION' };
        }

        // Default: abort.
        wizard.cancel();
        return { action: 'abort', state: 'CANCELLED' };
    }

    prSplit._handleBaselineFailState = handleBaselineFailState;

    /**
     * handlePlanReviewState — PLAN_REVIEW state handler.
     * Called when a plan is ready for user review. The plan should already be
     * stored in wizard.data.plan (set during PLAN_GENERATION → PLAN_REVIEW
     * transition).
     *
     * @param {WizardState} wizard - Wizard in PLAN_REVIEW state.
     * @param {string} choice - 'approve', 'edit', 'regenerate', or 'cancel'.
     * @param {Object} [opts] - Options for the choice (e.g. feedback for regenerate).
     * @returns {Object} { error?, action, state }
     */
    function handlePlanReviewState(wizard, choice, opts) {
        if (wizard.current !== 'PLAN_REVIEW') {
            return { error: 'wizard is not in PLAN_REVIEW state (current: ' + wizard.current + ')' };
        }

        opts = opts || {};

        if (choice === 'approve') {
            wizard.transition('BRANCH_BUILDING');
            return { action: 'approve', state: 'BRANCH_BUILDING' };
        }

        if (choice === 'edit') {
            wizard.transition('PLAN_EDITOR');
            return { action: 'edit', state: 'PLAN_EDITOR' };
        }

        if (choice === 'regenerate') {
            wizard.transition('PLAN_GENERATION', { feedback: opts.feedback || null });
            return { action: 'regenerate', state: 'PLAN_GENERATION' };
        }

        // Default: cancel.
        wizard.cancel();
        return { action: 'cancel', state: 'CANCELLED' };
    }

    prSplit._handlePlanReviewState = handlePlanReviewState;

    /**
     * handlePlanEditorState — PLAN_EDITOR state handler.
     * Called when the user chooses to edit the plan. Validates the plan after
     * edits and transitions back to PLAN_REVIEW.
     *
     * @param {WizardState} wizard - Wizard in PLAN_EDITOR state.
     * @param {string} choice - 'done' to finish editing, or 'cancel' (unused, reserved).
     * @param {Object} [plan] - The (possibly modified) plan to validate.
     * @returns {Object} { error?, action, state, validationErrors? }
     */
    function handlePlanEditorState(wizard, choice, plan) {
        if (wizard.current !== 'PLAN_EDITOR') {
            return { error: 'wizard is not in PLAN_EDITOR state (current: ' + wizard.current + ')' };
        }

        if (choice === 'done') {
            // Validate plan schema before accepting.
            if (plan) {
                var validation = prSplit.validatePlan(plan);
                if (validation && validation.errors && validation.errors.length > 0) {
                    return {
                        action: 'validation_failed',
                        state: 'PLAN_EDITOR',
                        validationErrors: validation.errors
                    };
                }
                // Store validated plan in wizard data.
                wizard.data.plan = plan;
            }

            wizard.transition('PLAN_REVIEW');
            return { action: 'done', state: 'PLAN_REVIEW' };
        }

        // Default: return to review without saving.
        wizard.transition('PLAN_REVIEW');
        return { action: 'done', state: 'PLAN_REVIEW' };
    }

    prSplit._handlePlanEditorState = handlePlanEditorState;

    /**
     * handleBranchBuildingState — BRANCH_BUILDING state handler.
     * Executes plan splits, verifies each branch, and tracks per-branch status.
     * On completion: transitions to EQUIV_CHECK (all pass) or ERROR_RESOLUTION (any fail).
     *
     * @param {WizardState} wizard - Wizard in BRANCH_BUILDING state.
     * @param {Object} plan - The approved plan to execute.
     * @param {Object} [opts] - Options (isCancelled: fn returning bool).
     * @returns {Object} { error?, action, state, results, failedBranches }
     */
    function handleBranchBuildingState(wizard, plan, opts) {
        if (wizard.current !== 'BRANCH_BUILDING') {
            return { error: 'wizard is not in BRANCH_BUILDING state (current: ' + wizard.current + ')' };
        }

        opts = opts || {};
        var isCancelled = opts.isCancelled || function() { return false; };

        if (!plan || !plan.splits || plan.splits.length === 0) {
            wizard.error('no plan or empty plan to execute — run "plan" first');
            return { error: 'no plan or empty plan to execute — run "plan" first', action: 'error', state: 'ERROR' };
        }

        // Execute splits.
        var execResult = prSplit.executeSplit(plan);
        if (execResult.error) {
            wizard.error(execResult.error);
            return { error: execResult.error, action: 'error', state: 'ERROR' };
        }

        // Check for cancellation.
        if (isCancelled()) {
            wizard.cancel();
            return { action: 'cancelled', state: 'CANCELLED', results: execResult.results };
        }

        // Verify each branch.
        var results = [];
        var failedBranches = [];
        for (var i = 0; i < execResult.results.length; i++) {
            if (isCancelled()) {
                wizard.cancel();
                return { action: 'cancelled', state: 'CANCELLED', results: results, failedBranches: failedBranches };
            }

            var branch = execResult.results[i];
            var status = {
                name: branch.name,
                files: branch.files,
                sha: branch.sha,
                execError: branch.error || null,
                verifyPassed: false,
                verifyOutput: '',
                verifyError: null
            };

            if (branch.error) {
                status.verifyError = 'skipped — execution failed: ' + branch.error;
                failedBranches.push(status);
            } else if (plan.verifyCommand && plan.verifyCommand !== 'true') {
                var verifyResult = prSplit.verifySplit(branch.name, {
                    verifyCommand: plan.verifyCommand,
                    dir: plan.dir || '.'
                });
                status.verifyPassed = verifyResult.passed;
                status.verifyOutput = verifyResult.output || '';
                status.verifyError = verifyResult.error || null;
                if (!verifyResult.passed) {
                    failedBranches.push(status);
                }
            } else {
                // No verify command — mark as passed.
                status.verifyPassed = true;
            }
            results.push(status);
        }

        // Store results in wizard data.
        wizard.data.branchResults = results;
        wizard.data.failedBranches = failedBranches;

        if (failedBranches.length > 0) {
            wizard.transition('ERROR_RESOLUTION', {
                failedBranches: failedBranches,
                results: results
            });
            return {
                action: 'failed',
                state: 'ERROR_RESOLUTION',
                results: results,
                failedBranches: failedBranches
            };
        }

        wizard.transition('EQUIV_CHECK', { results: results });
        return {
            action: 'success',
            state: 'EQUIV_CHECK',
            results: results,
            failedBranches: []
        };
    }

    prSplit._handleBranchBuildingState = handleBranchBuildingState;

    /**
     * handleErrorResolutionState — ERROR_RESOLUTION state handler.
     * Decides how to handle failed branches: auto-resolve, skip, manual, or abort.
     *
     * @param {WizardState} wizard - Wizard in ERROR_RESOLUTION state.
     * @param {string} choice - 'auto-resolve', 'skip', 'retry', 'manual', or 'abort'.
     * @returns {Object} { error?, action, state }
     */
    function handleErrorResolutionState(wizard, choice) {
        if (wizard.current !== 'ERROR_RESOLUTION') {
            return { error: 'wizard is not in ERROR_RESOLUTION state (current: ' + wizard.current + ')' };
        }

        if (choice === 'auto-resolve') {
            // Re-enter BRANCH_BUILDING after auto-resolve to re-verify.
            wizard.transition('BRANCH_BUILDING');
            return { action: 'auto-resolve', state: 'BRANCH_BUILDING' };
        }

        if (choice === 'skip') {
            // Record which branches are being skipped so downstream
            // operations (equivalence check, PR creation) can account
            // for the missing content.
            var skipped = (wizard.data && wizard.data.failedBranches) || [];
            wizard.data.skippedBranches = skipped;

            // Build a name-set for fast lookup and persist it on the
            // shared TUI state so REPL commands (e.g., create-prs) can
            // filter out skipped branches.
            var skipSet = {};
            for (var sk = 0; sk < skipped.length; sk++) {
                skipSet[skipped[sk].name || skipped[sk]] = true;
            }
            wizard.data.skipBranchSet = skipSet;
            st.skipBranchSet = skipSet;

            wizard.transition('EQUIV_CHECK');
            return { action: 'skip', state: 'EQUIV_CHECK', skippedBranches: skipped };
        }

        if (choice === 'retry') {
            // Regenerate plan.
            wizard.transition('PLAN_GENERATION');
            return { action: 'retry', state: 'PLAN_GENERATION' };
        }

        if (choice === 'manual') {
            // Manual fix: user interacts with Claude pane to fix branches,
            // then re-enters BRANCH_BUILDING for re-verification.
            var failedBranches = (wizard.data && wizard.data.failedBranches) || [];
            if (typeof output !== 'undefined') {
                output.print('[pr-split] Manual fix mode — use the Claude pane (toggle key) to fix these branches:');
                for (var fb = 0; fb < failedBranches.length; fb++) {
                    var fb_name = failedBranches[fb].name || failedBranches[fb];
                    var fb_error = failedBranches[fb].verifyError || failedBranches[fb].error || '';
                    output.print('  • ' + fb_name + (fb_error ? ': ' + fb_error : ''));
                }
                output.print('[pr-split] When done, run "fix" or "verify" to re-check branches.');
            }
            wizard.transition('BRANCH_BUILDING');
            return { action: 'manual', state: 'BRANCH_BUILDING' };
        }

        // Default: abort.
        wizard.cancel();
        return { action: 'abort', state: 'CANCELLED' };
    }

    prSplit._handleErrorResolutionState = handleErrorResolutionState;

    /**
     * handleEquivCheckState — EQUIV_CHECK state handler.
     * Runs equivalence check and transitions to FINALIZATION.
     *
     * @param {WizardState} wizard - Wizard in EQUIV_CHECK state.
     * @param {Object} plan - The plan to verify equivalence for.
     * @returns {Object} { error?, action, state, equivalence }
     */
    function handleEquivCheckState(wizard, plan) {
        if (wizard.current !== 'EQUIV_CHECK') {
            return { error: 'wizard is not in EQUIV_CHECK state (current: ' + wizard.current + ')' };
        }

        if (!plan) {
            wizard.error('no plan for equivalence check — run "plan" and "execute" first');
            return { error: 'no plan for equivalence check — run "plan" and "execute" first', action: 'error', state: 'ERROR' };
        }

        var equivResult = prSplit.verifyEquivalence(plan);

        // Annotate with skip information so callers understand the context.
        var skipped = wizard.data && wizard.data.skippedBranches;
        if (skipped && skipped.length > 0) {
            equivResult.skippedBranches = skipped.map(function(b) { return b.name || b; });
            equivResult.incomplete = true;
            // Equivalence is expected to fail when branches are skipped
            // because the last split's tree won't match the source tree
            // (skipped branches removed content).
        }
        wizard.data.equivalence = equivResult;

        wizard.transition('FINALIZATION', { equivalence: equivResult });
        return {
            action: 'checked',
            state: 'FINALIZATION',
            equivalence: equivResult
        };
    }

    prSplit._handleEquivCheckState = handleEquivCheckState;

    /**
     * handleFinalizationState — FINALIZATION state handler.
     * User decides: create PRs, view report, or finish.
     *
     * @param {WizardState} wizard - Wizard in FINALIZATION state.
     * @param {string} choice - 'create-prs', 'done', or 'report'.
     * @returns {Object} { error?, action, state }
     */
    function handleFinalizationState(wizard, choice) {
        if (wizard.current !== 'FINALIZATION') {
            return { error: 'wizard is not in FINALIZATION state (current: ' + wizard.current + ')' };
        }

        if (choice === 'create-prs') {
            // FINALIZATION → FINALIZATION (self-transition for PR creation step).
            wizard.transition('FINALIZATION', { prsRequested: true });
            return { action: 'create-prs', state: 'FINALIZATION' };
        }

        if (choice === 'report') {
            // Stay in FINALIZATION — caller displays the report.
            return { action: 'report', state: 'FINALIZATION' };
        }

        // Default: done.
        wizard.transition('DONE');
        return { action: 'done', state: 'DONE' };
    }

    prSplit._handleFinalizationState = handleFinalizationState;

    // ═══════════════════════════════════════════════════════════════════════
    function buildCommands(tuiStateArg) {
        var style = prSplit._style;
        var padIndex = prSplit._padIndex;
        var sanitizeBranchName = prSplit._sanitizeBranchName;
        var runtime = prSplit.runtime;

        return {
            analyze: {
                description: 'Analyze diff between current branch and base',
                usage: 'analyze [base-branch]',
                handler: function(args) {
                    try {
                    var base = (args && args.length > 0) ? args[0] : runtime.baseBranch;
                    output.print('Analyzing diff against ' + base + '...');
                    st.analysisCache = prSplit.analyzeDiff({ baseBranch: base });
                    if (st.analysisCache.error) {
                        output.print('Error: ' + st.analysisCache.error);
                        return;
                    }
                    output.print('Branch: ' + st.analysisCache.currentBranch + ' \u2192 ' + st.analysisCache.baseBranch);
                    output.print('Changed files: ' + st.analysisCache.files.length);
                    for (var i = 0; i < st.analysisCache.files.length; i++) {
                        var s = (st.analysisCache.fileStatuses && st.analysisCache.fileStatuses[st.analysisCache.files[i]]) || '?';
                        output.print('  [' + s + '] ' + st.analysisCache.files[i]);
                    }
                    } catch (e) {
                        output.print('Error in analyze: ' + (e && e.message ? e.message : String(e)));
                        if (e && e.stack) { log.error('pr-split analyze error stack: ' + e.stack); }
                    }
                }
            },

            stats: {
                description: 'Show diff stats with addition/deletion counts',
                usage: 'stats',
                handler: function() {
                    var stats = prSplit.analyzeDiffStats({ baseBranch: runtime.baseBranch });
                    if (stats.error) {
                        output.print('Error: ' + stats.error);
                        return;
                    }
                    output.print('File stats (' + stats.files.length + ' files):');
                    for (var i = 0; i < stats.files.length; i++) {
                        var f = stats.files[i];
                        output.print('  ' + f.name + ' (+' + f.additions + '/-' + f.deletions + ')');
                    }
                }
            },

            group: {
                description: 'Group files by strategy',
                usage: 'group [strategy]',
                handler: function(args) {
                    if (!st.analysisCache || !st.analysisCache.files || st.analysisCache.files.length === 0) {
                        output.print('Run "analyze" first.');
                        return;
                    }
                    var strategy = (args && args.length > 0) ? args[0] : runtime.strategy;
                    st.groupsCache = prSplit.applyStrategy(st.analysisCache.files, strategy);
                    var groupNames = Object.keys(st.groupsCache).sort();
                    output.print('Groups (' + strategy + '): ' + groupNames.length);
                    for (var i = 0; i < groupNames.length; i++) {
                        var name = groupNames[i];
                        output.print('  ' + name + ': ' + st.groupsCache[name].length + ' files');
                        for (var j = 0; j < st.groupsCache[name].length; j++) {
                            output.print('    ' + st.groupsCache[name][j]);
                        }
                    }
                }
            },

            plan: {
                description: 'Create split plan from current groups',
                usage: 'plan',
                handler: function() {
                    if (!st.groupsCache) {
                        output.print('Run "group" first.');
                        return;
                    }
                    var planConfig = {
                        baseBranch: runtime.baseBranch,
                        branchPrefix: runtime.branchPrefix,
                        verifyCommand: runtime.verifyCommand
                    };
                    if (st.analysisCache) {
                        planConfig.sourceBranch = st.analysisCache.currentBranch;
                        planConfig.fileStatuses = st.analysisCache.fileStatuses;
                    }
                    st.planCache = prSplit.createSplitPlan(st.groupsCache, planConfig);
                    var validation = prSplit.validatePlan(st.planCache);
                    if (!validation.valid) {
                        output.print('Plan validation errors:');
                        for (var i = 0; i < validation.errors.length; i++) {
                            output.print('  - ' + validation.errors[i]);
                        }
                        return;
                    }
                    output.print('Plan created: ' + st.planCache.splits.length + ' splits');
                    output.print('Base: ' + st.planCache.baseBranch + ' \u2192 Source: ' + st.planCache.sourceBranch);
                    for (var i = 0; i < st.planCache.splits.length; i++) {
                        var s = st.planCache.splits[i];
                        output.print('  ' + padIndex(s.order + 1) + '. ' + s.name + ' (' + s.files.length + ' files)');
                    }
                }
            },

            preview: {
                description: 'Show detailed plan preview (dry run)',
                usage: 'preview',
                handler: function() {
                    if (!st.planCache) {
                        output.print('Run "plan" first.');
                        return;
                    }
                    output.print('=== Split Plan Preview ===');
                    output.print('Base branch:    ' + st.planCache.baseBranch);
                    output.print('Source branch:  ' + st.planCache.sourceBranch);
                    output.print('Verify command: ' + st.planCache.verifyCommand);
                    output.print('Splits:         ' + st.planCache.splits.length);
                    output.print('');
                    var prevBranch = st.planCache.baseBranch;
                    for (var i = 0; i < st.planCache.splits.length; i++) {
                        var s = st.planCache.splits[i];
                        output.print('Split ' + padIndex(i + 1) + ': ' + s.name);
                        output.print('  Parent: ' + prevBranch);
                        output.print('  Commit: ' + s.message);
                        output.print('  Files:');
                        for (var j = 0; j < s.files.length; j++) {
                            output.print('    ' + s.files[j]);
                        }
                        prevBranch = s.name;
                        output.print('');
                    }
                }
            },

            move: {
                description: 'Move a file from one split to another',
                usage: 'move <file> <from-index> <to-index>',
                handler: function(args) {
                    if (!st.planCache) { output.print('No plan \u2014 run "plan" first.'); return; }
                    if (!args || args.length < 3) {
                        output.print('Usage: move <file-path> <from-split-index> <to-split-index>');
                        return;
                    }
                    var file = args[0];
                    var fromIdx = parseInt(args[1], 10) - 1;
                    var toIdx = parseInt(args[2], 10) - 1;
                    if (isNaN(fromIdx) || fromIdx < 0 || fromIdx >= st.planCache.splits.length) {
                        output.print('Invalid from-index: ' + args[1]); return;
                    }
                    if (isNaN(toIdx) || toIdx < 0 || toIdx >= st.planCache.splits.length) {
                        output.print('Invalid to-index: ' + args[2]); return;
                    }
                    if (fromIdx === toIdx) { output.print('From and to are the same split.'); return; }
                    var fromSplit = st.planCache.splits[fromIdx];
                    var fileIdx = -1;
                    for (var i = 0; i < fromSplit.files.length; i++) {
                        if (fromSplit.files[i] === file) { fileIdx = i; break; }
                    }
                    if (fileIdx === -1) {
                        output.print('File "' + file + '" not found in split ' + (fromIdx + 1)); return;
                    }
                    fromSplit.files.splice(fileIdx, 1);
                    st.planCache.splits[toIdx].files.push(file);
                    st.planCache.splits[toIdx].files.sort();
                    output.print('Moved "' + file + '" from split ' + (fromIdx + 1) + ' to split ' + (toIdx + 1));
                    if (fromSplit.files.length === 0) {
                        st.planCache.splits.splice(fromIdx, 1);
                        output.print('Split ' + (fromIdx + 1) + ' is now empty \u2014 removed.');
                        for (var r = 0; r < st.planCache.splits.length; r++) {
                            st.planCache.splits[r].order = r;
                        }
                    }
                }
            },

            rename: {
                description: 'Rename a split branch',
                usage: 'rename <split-index> <new-name>',
                handler: function(args) {
                    if (!st.planCache) { output.print('No plan \u2014 run "plan" first.'); return; }
                    if (!args || args.length < 2) { output.print('Usage: rename <split-index> <new-name>'); return; }
                    var idx = parseInt(args[0], 10) - 1;
                    if (isNaN(idx) || idx < 0 || idx >= st.planCache.splits.length) {
                        output.print('Invalid index: ' + args[0]); return;
                    }
                    var newName = args.slice(1).join('-');
                    var oldName = st.planCache.splits[idx].name;
                    st.planCache.splits[idx].name = sanitizeBranchName(
                        runtime.branchPrefix + padIndex(idx + 1) + '-' + newName
                    );
                    st.planCache.splits[idx].message = newName;
                    output.print('Renamed split ' + (idx + 1) + ': ' + oldName + ' \u2192 ' + st.planCache.splits[idx].name);
                }
            },

            merge: {
                description: 'Merge two splits into one',
                usage: 'merge <split-a> <split-b>',
                handler: function(args) {
                    if (!st.planCache) { output.print('No plan \u2014 run "plan" first.'); return; }
                    if (!args || args.length < 2) { output.print('Usage: merge <split-index-a> <split-index-b>'); return; }
                    var idxA = parseInt(args[0], 10) - 1;
                    var idxB = parseInt(args[1], 10) - 1;
                    if (isNaN(idxA) || idxA < 0 || idxA >= st.planCache.splits.length) { output.print('Invalid index A'); return; }
                    if (isNaN(idxB) || idxB < 0 || idxB >= st.planCache.splits.length) { output.print('Invalid index B'); return; }
                    if (idxA === idxB) { output.print('Cannot merge a split with itself.'); return; }
                    var splitA = st.planCache.splits[idxA];
                    var splitB = st.planCache.splits[idxB];
                    for (var i = 0; i < splitB.files.length; i++) { splitA.files.push(splitB.files[i]); }
                    splitA.files.sort();
                    st.planCache.splits.splice(idxB, 1);
                    for (var r = 0; r < st.planCache.splits.length; r++) { st.planCache.splits[r].order = r; }
                    output.print('Merged split ' + (idxB + 1) + ' into split ' + (idxA + 1) +
                        '. Plan now has ' + st.planCache.splits.length + ' splits.');
                }
            },

            reorder: {
                description: 'Move a split to a different position',
                usage: 'reorder <split-index> <new-position>',
                handler: function(args) {
                    if (!st.planCache) { output.print('No plan \u2014 run "plan" first.'); return; }
                    if (!args || args.length < 2) { output.print('Usage: reorder <split-index> <new-position>'); return; }
                    var fromIdx = parseInt(args[0], 10) - 1;
                    var toIdx = parseInt(args[1], 10) - 1;
                    if (isNaN(fromIdx) || fromIdx < 0 || fromIdx >= st.planCache.splits.length) { output.print('Invalid index'); return; }
                    if (isNaN(toIdx) || toIdx < 0 || toIdx >= st.planCache.splits.length) { output.print('Invalid position'); return; }
                    if (fromIdx === toIdx) { output.print('Already at that position.'); return; }
                    var split = st.planCache.splits.splice(fromIdx, 1)[0];
                    st.planCache.splits.splice(toIdx, 0, split);
                    for (var r = 0; r < st.planCache.splits.length; r++) {
                        st.planCache.splits[r].order = r;
                        var oldName = st.planCache.splits[r].name;
                        var nameParts = oldName.split('/');
                        if (nameParts.length > 1) {
                            var lastPart = nameParts[nameParts.length - 1];
                            var dashIdx = lastPart.indexOf('-');
                            if (dashIdx >= 0) {
                                var suffix = lastPart.substring(dashIdx + 1);
                                nameParts[nameParts.length - 1] = padIndex(r + 1) + '-' + suffix;
                                st.planCache.splits[r].name = nameParts.join('/');
                            }
                        }
                    }
                    output.print('Moved split from position ' + (fromIdx + 1) + ' to ' + (toIdx + 1));
                }
            },

            execute: {
                description: 'Execute the split plan (creates branches)',
                usage: 'execute',
                handler: function() {
                    try {
                    if (!st.planCache) { output.print('Run "plan" first.'); return; }
                    if (runtime.dryRun) {
                        output.print('Dry-run mode: showing plan without executing.');
                        return;
                    }
                    output.print('Executing split plan (' + st.planCache.splits.length + ' splits)...');
                    var result = prSplit.executeSplit(st.planCache);
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    st.executionResultCache = result.results;
                    output.print(style.success('Split completed successfully!'));
                    for (var i = 0; i < result.results.length; i++) {
                        var r = result.results[i];
                        output.print('  ' + style.success('\u2713') + ' ' + r.name +
                            ' (' + r.files.length + ' files, SHA: ' + style.dim(r.sha.substring(0, 8)) + ')');
                    }
                    var equiv = prSplit.verifyEquivalence(st.planCache);
                    if (equiv.equivalent) {
                        output.print(style.success('\u2705 Tree hash equivalence verified'));
                    } else if (equiv.error) {
                        output.print(style.warning('\u26a0\ufe0f  Equivalence check error: ' + equiv.error));
                    } else {
                        output.print(style.error('\u274c Tree hash mismatch!'));
                        output.print('   Split tree:  ' + style.dim(equiv.splitTree));
                        output.print('   Source tree:  ' + style.dim(equiv.sourceTree));
                    }
                    } catch (e) {
                        output.print('Error in execute: ' + (e && e.message ? e.message : String(e)));
                        if (e && e.stack) { log.error('pr-split execute error stack: ' + e.stack); }
                    }
                }
            },

            verify: {
                description: 'Verify all split branches (run tests on each)',
                usage: 'verify',
                handler: function() {
                    if (!st.planCache) { output.print('Run "plan" first.'); return; }
                    output.print('Verifying ' + st.planCache.splits.length + ' splits...');
                    var result = prSplit.verifySplits(st.planCache);
                    for (var i = 0; i < result.results.length; i++) {
                        var r = result.results[i];
                        var icon = r.passed ? style.success('\u2713') :
                                   r.skipped ? style.warning('\u2298') :
                                   r.preExisting ? style.dim('\u26a0') :
                                   style.error('\u2717');
                        output.print('  ' + icon + ' ' + r.name + (r.error ? ': ' + r.error : ''));
                    }
                    if (result.allPassed) {
                        output.print(style.success('\u2705 All splits pass verification'));
                    } else {
                        output.print(style.error('\u274c Some splits failed'));
                    }
                }
            },

            equivalence: {
                description: 'Check tree hash equivalence',
                usage: 'equivalence',
                handler: function() {
                    if (!st.planCache) { output.print('Run "plan" first.'); return; }
                    var result = prSplit.verifyEquivalenceDetailed(st.planCache);
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    if (result.equivalent) {
                        output.print(style.success('\u2705 Trees are equivalent'));
                        output.print('   Hash: ' + style.dim(result.splitTree));
                    } else {
                        output.print(style.error('\u274c Trees differ'));
                        output.print('   Split tree:  ' + style.dim(result.splitTree));
                        output.print('   Source tree:  ' + style.dim(result.sourceTree));
                        if (result.diffFiles && result.diffFiles.length > 0) {
                            output.print('   Differing files:');
                            for (var i = 0; i < result.diffFiles.length; i++) {
                                output.print('     ' + result.diffFiles[i]);
                            }
                        }
                    }
                }
            },

            cleanup: {
                description: 'Delete all split branches',
                usage: 'cleanup',
                handler: function() {
                    if (!st.planCache) { output.print('No plan to clean up.'); return; }
                    var result = prSplit.cleanupBranches(st.planCache);
                    if (result.deleted.length > 0) {
                        output.print('Deleted branches:');
                        for (var i = 0; i < result.deleted.length; i++) output.print('  ' + result.deleted[i]);
                    }
                    if (result.errors.length > 0) {
                        output.print('Errors:');
                        for (var i = 0; i < result.errors.length; i++) output.print('  ' + result.errors[i]);
                    }
                }
            },

            fix: {
                description: 'Auto-fix split branches that fail verification',
                usage: 'fix',
                handler: async function() {
                    if (!st.planCache) { output.print('No plan \u2014 run "plan" or "run" first.'); return; }
                    output.print('Checking splits for verification failures...');
                    var result = await prSplit.resolveConflicts(st.planCache);
                    if (result.skipped) { output.print('Skipped: ' + result.skipped); return; }
                    if (result.fixed.length > 0) {
                        output.print(style.success('Fixed:'));
                        for (var i = 0; i < result.fixed.length; i++) {
                            output.print('  ' + style.success('\u2713') + ' ' + result.fixed[i].name +
                                ' (' + style.dim(result.fixed[i].strategy) + ')');
                        }
                    }
                    if (result.errors.length > 0) {
                        output.print(style.error('Unresolved:'));
                        for (var i = 0; i < result.errors.length; i++) {
                            output.print('  ' + style.error('\u274c') + ' ' + result.errors[i].name + ': ' + result.errors[i].error);
                        }
                    }
                    if (result.fixed.length === 0 && result.errors.length === 0) {
                        output.print('All splits pass verification \u2014 no fixes needed.');
                    }
                }
            },

            'create-prs': {
                description: 'Push branches and create stacked GitHub PRs',
                usage: 'create-prs [--draft] [--push-only] [--auto-merge]',
                handler: function(args) {
                    if (!st.planCache) { output.print('No plan \u2014 run "plan" or "run" first.'); return; }
                    if (!st.executionResultCache || st.executionResultCache.length === 0) {
                        output.print('No splits executed \u2014 run "execute" or "run" first.'); return;
                    }
                    var draft = true, pushOnly = false;
                    var autoMerge = runtime.autoMerge || false;
                    var mergeMethod = runtime.mergeMethod || 'squash';
                    if (args) {
                        for (var i = 0; i < args.length; i++) {
                            if (args[i] === '--no-draft') draft = false;
                            if (args[i] === '--push-only') pushOnly = true;
                            if (args[i] === '--auto-merge') autoMerge = true;
                            if (args[i] === '--merge-method' && i + 1 < args.length) { mergeMethod = args[++i]; }
                        }
                    }

                    // Filter out skipped/failed branches so we don't create
                    // PRs for branches that failed verification.
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
                            output.print('[pr-split] Excluding ' +
                                (effectivePlan.splits.length - filtered.length) +
                                ' skipped branch(es) from PR creation.');
                        }
                        effectivePlan = Object.assign({}, effectivePlan, { splits: filtered });
                    }

                    output.print('Creating PRs for ' + effectivePlan.splits.length + ' splits...');
                    var result = prSplit.createPRs(effectivePlan, {
                        draft: draft, pushOnly: pushOnly,
                        autoMerge: autoMerge, mergeMethod: mergeMethod
                    });
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    for (var i = 0; i < result.results.length; i++) {
                        var r = result.results[i];
                        if (r.error) {
                            output.print('  ' + style.error('\u274c') + ' ' + r.name + ': ' + r.error);
                        } else if (r.prUrl) {
                            output.print('  ' + style.success('\u2713') + ' ' + r.name + ' \u2192 ' + style.info(r.prUrl));
                        } else {
                            output.print('  ' + style.success('\u2713') + ' ' + r.name + ' (pushed)');
                        }
                    }
                }
            },

            'set': {
                description: 'Set runtime configuration',
                usage: 'set <key> <value>',
                handler: function(args) {
                    if (!args || args.length < 2) {
                        output.print('Usage: set <key> <value>');
                        output.print('Keys: base, strategy, max, prefix, verify, dry-run, mode, retry-budget, view, auto-merge, merge-method');
                        output.print('Current:');
                        output.print('  base:         ' + runtime.baseBranch);
                        output.print('  strategy:     ' + runtime.strategy);
                        output.print('  max:          ' + runtime.maxFiles);
                        output.print('  prefix:       ' + runtime.branchPrefix);
                        output.print('  verify:       ' + runtime.verifyCommand);
                        output.print('  dry-run:      ' + runtime.dryRun);
                        output.print('  mode:         ' + runtime.mode);
                        output.print('  retry-budget: ' + runtime.retryBudget);
                        output.print('  view:         ' + runtime.view);
                        output.print('  auto-merge:   ' + runtime.autoMerge);
                        output.print('  merge-method: ' + runtime.mergeMethod);
                        return;
                    }
                    var key = args[0], value = args.slice(1).join(' ');
                    switch (key) {
                        case 'base': runtime.baseBranch = value; break;
                        case 'strategy': runtime.strategy = value; break;
                        case 'max': runtime.maxFiles = parseInt(value, 10) || 10; break;
                        case 'prefix': runtime.branchPrefix = value; break;
                        case 'verify': runtime.verifyCommand = value; break;
                        case 'dry-run': runtime.dryRun = (value === 'true' || value === '1'); break;
                        case 'mode':
                            if (value !== 'auto' && value !== 'heuristic') {
                                output.print('Invalid mode. Use "auto" or "heuristic".'); return;
                            }
                            runtime.mode = value; break;
                        case 'retry-budget': case 'retryBudget':
                            var budget = parseInt(value, 10);
                            if (isNaN(budget) || budget < 0) { output.print('Invalid retry budget.'); return; }
                            runtime.retryBudget = budget; break;
                        case 'view':
                            if (value !== 'toggle' && value !== 'split') {
                                output.print('Invalid view. Use "toggle" or "split".'); return;
                            }
                            runtime.view = value; break;
                        case 'auto-merge': case 'autoMerge':
                            runtime.autoMerge = (value === 'true' || value === '1'); break;
                        case 'merge-method': case 'mergeMethod':
                            if (value !== 'squash' && value !== 'merge' && value !== 'rebase') {
                                output.print('Invalid merge method.'); return;
                            }
                            runtime.mergeMethod = value; break;
                        default: output.print('Unknown key: ' + key); return;
                    }
                    output.print('Set ' + key + ' = ' + value);
                }
            },

            run: {
                description: 'Run full workflow: analyze \u2192 group \u2192 plan \u2192 execute',
                usage: 'run [--mode auto|heuristic]',
                handler: async function(args) {
                    try {
                    var useAuto = runtime.mode === 'auto';
                    if (args && args.length > 0) {
                        for (var a = 0; a < args.length; a++) {
                            if (args[a] === '--mode' && a + 1 < args.length) { useAuto = args[++a] === 'auto'; }
                        }
                    }
                    if (useAuto) {
                        if (!st.claudeExecutor) {
                            st.claudeExecutor = new (prSplit.ClaudeCodeExecutor)(prSplitConfig);
                        }
                        if (st.claudeExecutor.isAvailable()) {
                            output.print(style.info('Mode: automated (Claude detected)'));
                            var autoConfig = {
                                baseBranch: runtime.baseBranch,
                                strategy: runtime.strategy,
                                cleanupOnFailure: prSplitConfig.cleanupOnFailure
                            };
                            if (prSplitConfig.timeoutMs > 0) {
                                autoConfig.classifyTimeoutMs = prSplitConfig.timeoutMs;
                                autoConfig.planTimeoutMs = prSplitConfig.timeoutMs;
                                autoConfig.resolveTimeoutMs = prSplitConfig.timeoutMs;
                                autoConfig.verifyTimeoutMs = prSplitConfig.timeoutMs;
                            }
                            var result = await prSplit.automatedSplit(autoConfig);
                            if (result.error) output.print(style.error('Auto-split error: ' + result.error));
                            if (runtime.jsonOutput && result.report) output.print(JSON.stringify(result.report, null, 2));
                            return;
                        }
                        output.print(style.warning('Claude not available \u2014 using heuristic mode.'));
                    }
                    var workflowStart = Date.now();
                    output.print(style.header('Running full PR split workflow...'));
                    output.print('Base:     ' + style.bold(runtime.baseBranch));
                    output.print('Strategy: ' + style.bold(runtime.strategy));
                    output.print('Max:      ' + style.bold(String(runtime.maxFiles)));
                    output.print('');

                    // Step 1: Analyze
                    st.analysisCache = prSplit.analyzeDiff({ baseBranch: runtime.baseBranch });
                    if (st.analysisCache.error) {
                        output.print(style.error('Analysis failed: ' + st.analysisCache.error)); return;
                    }
                    if (!st.analysisCache.files || st.analysisCache.files.length === 0) {
                        output.print(style.warning('No changes found.')); return;
                    }
                    output.print(style.success('\u2713 Analysis: ') + st.analysisCache.files.length + ' changed files');

                    // Step 2: Group
                    st.groupsCache = prSplit.applyStrategy(st.analysisCache.files, runtime.strategy);
                    var groupNames = Object.keys(st.groupsCache).sort();
                    if (groupNames.length === 0) { output.print('No groups created.'); return; }
                    output.print(style.success('\u2713 Grouped into ' + groupNames.length + ' groups') +
                        ' (' + runtime.strategy + ')');

                    // Step 3: Plan
                    st.planCache = prSplit.createSplitPlan(st.groupsCache, {
                        baseBranch: runtime.baseBranch,
                        sourceBranch: st.analysisCache.currentBranch,
                        branchPrefix: runtime.branchPrefix,
                        verifyCommand: runtime.verifyCommand,
                        fileStatuses: st.analysisCache.fileStatuses
                    });
                    var validation = prSplit.validatePlan(st.planCache);
                    if (!validation.valid) { output.print('Plan invalid: ' + validation.errors.join('; ')); return; }
                    output.print(style.success('\u2713 Plan created: ' + st.planCache.splits.length + ' splits'));

                    // Step 4: Execute
                    if (runtime.dryRun) {
                        output.print('');
                        output.print(style.warning('DRY RUN') + ' \u2014 plan preview:');
                        for (var i = 0; i < st.planCache.splits.length; i++) {
                            var s = st.planCache.splits[i];
                            output.print('  ' + padIndex(i + 1) + '. ' + s.name + ' (' + s.files.length + ' files)');
                        }
                        return;
                    }
                    output.print(style.info('Executing ' + st.planCache.splits.length + ' splits...'));
                    var result = prSplit.executeSplit(st.planCache);
                    if (result.error) { output.print(style.error('Execution failed: ' + result.error)); return; }
                    st.executionResultCache = result.results;
                    output.print(style.success('\u2713 Split executed: ' + result.results.length + ' branches created'));

                    // Step 5: Verify equivalence
                    var equiv = prSplit.verifyEquivalence(st.planCache);
                    if (equiv.equivalent) {
                        output.print(style.success('\u2705 Tree hash equivalence verified'));
                    } else if (equiv.error) {
                        output.print(style.warning('Equivalence error: ' + equiv.error));
                    } else {
                        output.print(style.error('\u274c Tree hash mismatch'));
                    }
                    var totalMs = Date.now() - workflowStart;
                    output.print('');
                    output.print(style.success('Done') + ' in ' + style.bold(totalMs < 1000 ? totalMs + 'ms' :
                        (totalMs / 1000).toFixed(1) + 's'));
                    if (runtime.jsonOutput) output.print(JSON.stringify(buildReport(), null, 2));
                    } catch (e) {
                        output.print('Error in run workflow: ' + (e && e.message ? e.message : String(e)));
                        if (e && e.stack) log.error('pr-split run error stack: ' + e.stack);
                    }
                }
            },

            copy: {
                description: 'Copy the split plan to clipboard',
                usage: 'copy',
                handler: function() {
                    if (!st.planCache) { output.print('Run "plan" first.'); return; }
                    var tmpl = prSplit._modules.template;
                    if (!tmpl) { output.print('Template module not available.'); return; }
                    try {
                        var text = tmpl.execute(typeof prSplitTemplate !== 'undefined' ? prSplitTemplate : '', {
                            baseBranch: st.planCache.baseBranch,
                            currentBranch: st.planCache.sourceBranch,
                            fileCount: st.analysisCache ? st.analysisCache.files.length : 0,
                            strategy: runtime.strategy,
                            groupCount: Object.keys(st.groupsCache || {}).length,
                            groups: Object.keys(st.groupsCache || {}).sort().map(function(name) {
                                return { label: name, files: st.groupsCache[name] };
                            }),
                            plan: st.planCache.splits.map(function(s, i) {
                                return { index: i + 1, branch: s.name, fileCount: s.files.length, description: s.message };
                            }),
                            verified: false
                        });
                        output.toClipboard(text);
                        output.print('Plan copied to clipboard.');
                    } catch (e) {
                        output.print('Error copying: ' + (e && e.message ? e.message : e));
                    }
                }
            },

            'save-plan': {
                description: 'Save current plan to a JSON file',
                usage: 'save-plan [path]',
                handler: function(args) {
                    var path = (args && args.length > 0) ? args[0] : undefined;
                    var result = prSplit.savePlan(path);
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    output.print('Plan saved to ' + result.path);
                }
            },

            'load-plan': {
                description: 'Load a previously-saved plan from a JSON file',
                usage: 'load-plan [path]',
                handler: function(args) {
                    var path = (args && args.length > 0) ? args[0] : undefined;
                    var result = prSplit.loadPlan(path);
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    output.print('Plan loaded from ' + result.path);
                    output.print('  Total splits: ' + result.totalSplits + '  Executed: ' + result.executedSplits +
                        '  Pending: ' + result.pendingSplits);
                }
            },

            report: {
                description: 'Output current state as JSON',
                usage: 'report',
                handler: function() { output.print(JSON.stringify(buildReport(), null, 2)); }
            },

            'auto-split': {
                description: 'Automated split via Claude Code (falls back to heuristic)',
                usage: 'auto-split [--resume]',
                handler: async function(args) {
                    try {
                        var autoConfig = {
                            baseBranch: runtime.baseBranch,
                            strategy: runtime.strategy,
                            maxGroups: 0,
                            cleanupOnFailure: prSplitConfig.cleanupOnFailure
                        };
                        if (prSplitConfig.timeoutMs > 0) {
                            autoConfig.classifyTimeoutMs = prSplitConfig.timeoutMs;
                            autoConfig.planTimeoutMs = prSplitConfig.timeoutMs;
                            autoConfig.resolveTimeoutMs = prSplitConfig.timeoutMs;
                            autoConfig.verifyTimeoutMs = prSplitConfig.timeoutMs;
                        }

                        // Check for --resume flag.
                        var resumeFlag = false;
                        if (args) {
                            var argStr = typeof args === 'string' ? args : String(args);
                            resumeFlag = argStr.indexOf('--resume') >= 0;
                        }
                        if (resumeFlag) autoConfig.resume = true;

                        // --- Wizard: CONFIG state ---
                        var wizard = new WizardState();
                        wizard.onTransition(function(from, to, data) {
                            log.printf('wizard: %s → %s', from, to);
                        });

                        // Wire cooperative cancellation to wizard state.
                        // The pipeline checks prSplit._cancelSource('cancelled')
                        // at each step boundary.
                        prSplit._cancelSource = function(query) {
                            if (query === 'cancelled') return wizard.current === 'CANCELLED';
                            if (query === 'forceCancelled') return wizard.current === 'CANCELLED';
                            if (query === 'paused') return false; // pause not yet wired to wizard
                            return false;
                        };

                        wizard.transition('CONFIG', { config: autoConfig });
                        var configResult = handleConfigState(autoConfig);

                        if (configResult.error) {
                            wizard.error(configResult.error);
                            output.print(style.error('Config error: ' + configResult.error));
                            return;
                        }

                        if (configResult.resume) {
                            wizard.transition('BRANCH_BUILDING', configResult.checkpoint);
                            output.print('[wizard] Resuming from checkpoint...');
                            // Checkpoint resume falls through to automatedSplit with
                            // the saved plan — full BRANCH_BUILDING state entry from
                            // checkpoint is not yet implemented.
                            autoConfig.resumePlan = configResult.checkpoint.plan;
                        } else if (configResult.baselineFailed) {
                            wizard.transition('BASELINE_FAIL', {
                                error: configResult.baselineError,
                                output: configResult.baselineOutput
                            });

                            // --- BASELINE_FAIL menu ---
                            output.print('');
                            output.print(style.error('\u274c Baseline verification failed'));
                            output.print('  Error: ' + configResult.baselineError);
                            if (configResult.baselineOutput) {
                                output.print('  Output: ' + style.dim(configResult.baselineOutput));
                            }
                            output.print('');
                            output.print('Options:');
                            output.print('  ' + style.warning('override') + ' — proceed to plan generation despite baseline failure');
                            output.print('  ' + style.error('abort') + '    — cancel the auto-split');
                            output.print('');
                            output.print('Type "override" or "abort" at the prompt.');

                            // Store wizard for override/abort commands to access.
                            tuiState._activeWizard = wizard;
                            tuiState._activeAutoConfig = autoConfig;
                            return;
                        } else {
                            wizard.transition('PLAN_GENERATION');
                        }

                        // --- Run the automated pipeline ---
                        var result = await prSplit.automatedSplit(autoConfig);
                        if (result.error) output.print(style.error('Auto-split failed: ' + result.error));
                        if (runtime.jsonOutput && result.report) output.print(JSON.stringify(result.report, null, 2));

                        // Transition to terminal.
                        if (!wizard.isTerminal()) {
                            if (result.error) {
                                wizard.error(result.error);
                            } else {
                                // Fast-path remaining transitions.
                                if (wizard.current === 'PLAN_GENERATION') wizard.transition('PLAN_REVIEW');
                                if (wizard.current === 'PLAN_REVIEW') wizard.transition('BRANCH_BUILDING');
                                if (wizard.current === 'BRANCH_BUILDING') wizard.transition('EQUIV_CHECK');
                                if (wizard.current === 'EQUIV_CHECK') wizard.transition('FINALIZATION');
                                if (wizard.current === 'FINALIZATION') wizard.transition('DONE');
                            }
                        }
                    } catch (e) {
                        output.print('Error: ' + (e && e.message ? e.message : String(e)));
                        if (e && e.stack) log.error('auto-split error: ' + e.stack);
                    }
                }
            },

            'override': {
                description: 'Override baseline failure and proceed to plan generation',
                usage: 'override',
                handler: async function() {
                    var wizard = tuiState._activeWizard;
                    if (!wizard || wizard.current !== 'BASELINE_FAIL') {
                        output.print(style.error('No baseline failure to override. Run "auto-split" first.'));
                        return;
                    }

                    var result = handleBaselineFailState(wizard, 'override');
                    if (result.error) {
                        output.print(style.error(result.error));
                        return;
                    }

                    output.print(style.warning('[wizard] Overriding baseline failure — proceeding to plan generation...'));

                    var autoConfig = tuiState._activeAutoConfig;
                    tuiState._activeWizard = null;
                    tuiState._activeAutoConfig = null;

                    // Wire cooperative cancellation for the override pipeline run.
                    prSplit._cancelSource = function(query) {
                        if (query === 'cancelled') return wizard.current === 'CANCELLED';
                        if (query === 'forceCancelled') return wizard.current === 'CANCELLED';
                        return false;
                    };

                    // Continue the pipeline from PLAN_GENERATION.
                    try {
                        var pipelineResult = await prSplit.automatedSplit(autoConfig);
                        if (pipelineResult.error) output.print(style.error('Auto-split failed: ' + pipelineResult.error));
                        if (runtime.jsonOutput && pipelineResult.report) output.print(JSON.stringify(pipelineResult.report, null, 2));

                        if (!wizard.isTerminal()) {
                            if (pipelineResult.error) {
                                wizard.error(pipelineResult.error);
                            } else {
                                if (wizard.current === 'PLAN_GENERATION') wizard.transition('PLAN_REVIEW');
                                if (wizard.current === 'PLAN_REVIEW') wizard.transition('BRANCH_BUILDING');
                                if (wizard.current === 'BRANCH_BUILDING') wizard.transition('EQUIV_CHECK');
                                if (wizard.current === 'EQUIV_CHECK') wizard.transition('FINALIZATION');
                                if (wizard.current === 'FINALIZATION') wizard.transition('DONE');
                            }
                        }
                    } catch (e) {
                        output.print('Error: ' + (e && e.message ? e.message : String(e)));
                        if (e && e.stack) log.error('override pipeline error: ' + e.stack);
                    }
                }
            },

            'abort': {
                description: 'Abort after baseline failure',
                usage: 'abort',
                handler: function() {
                    var wizard = tuiState._activeWizard;
                    if (!wizard || wizard.current !== 'BASELINE_FAIL') {
                        output.print(style.error('No baseline failure to abort. Run "auto-split" first.'));
                        return;
                    }

                    var result = handleBaselineFailState(wizard, 'abort');
                    if (result.error) {
                        output.print(style.error(result.error));
                        return;
                    }

                    output.print('[wizard] Auto-split aborted.');
                    tuiState._activeWizard = null;
                    tuiState._activeAutoConfig = null;
                }
            },

            'edit-plan': {
                description: 'Open interactive plan editor (BubbleTea TUI)',
                usage: 'edit-plan',
                handler: function() {
                    if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
                        output.print('No plan to edit. Run "plan" first.'); return;
                    }
                    var items = [];
                    for (var i = 0; i < st.planCache.splits.length; i++) {
                        var s = st.planCache.splits[i];
                        items.push({
                            name: s.name || ('split-' + (i + 1)),
                            files: s.files || [],
                            branchName: s.branchName || s.name || '',
                            description: s.message || ''
                        });
                    }
                    output.print('[edit-plan] Plan editor not available. Use text commands: move, rename, merge.');
                }
            },

            diff: {
                description: 'Show colorized diff for a specific split',
                usage: 'diff <split-index|split-name>',
                handler: function(args) {
                    if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
                        output.print('No plan \u2014 run "plan" or "run" first.'); return;
                    }
                    if (!args || args.length === 0) {
                        output.print('Usage: diff <split-index|split-name>');
                        for (var i = 0; i < st.planCache.splits.length; i++) {
                            output.print('  ' + (i + 1) + '. ' + st.planCache.splits[i].name);
                        }
                        return;
                    }
                    var target = args.join(' ');
                    var splitIndex = -1;
                    var idx = parseInt(target, 10);
                    if (!isNaN(idx) && idx >= 1 && idx <= st.planCache.splits.length) {
                        splitIndex = idx - 1;
                    } else {
                        for (var s = 0; s < st.planCache.splits.length; s++) {
                            if (st.planCache.splits[s].name === target) { splitIndex = s; break; }
                        }
                    }
                    if (splitIndex < 0) { output.print('Unknown split: ' + target); return; }
                    output.print('Diff for split ' + (splitIndex + 1) + ': ' + st.planCache.splits[splitIndex].name);
                    var result = prSplit.getSplitDiff(st.planCache, splitIndex);
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    if (!result.diff) { output.print('(empty diff)'); return; }
                    output.print(prSplit.renderColorizedDiff(result.diff));
                }
            },

            conversation: {
                description: 'Show Claude conversation history for this session',
                usage: 'conversation',
                handler: function() {
                    var history = prSplit.getConversationHistory();
                    if (history.length === 0) {
                        output.print('No Claude conversations recorded in this session.'); return;
                    }
                    output.print('Claude Conversation History (' + history.length + ' interactions):');
                    output.print('');
                    for (var i = 0; i < history.length; i++) {
                        var conv = history[i];
                        output.print('  [' + (i + 1) + '] ' + conv.action + ' @ ' + conv.timestamp);
                        if (conv.prompt) {
                            var pp = conv.prompt.substring(0, 100);
                            if (conv.prompt.length > 100) pp += '...';
                            output.print('      Prompt: ' + pp);
                        }
                        if (conv.response) {
                            var rp = conv.response.substring(0, 100);
                            if (conv.response.length > 100) rp += '...';
                            output.print('      Response: ' + rp);
                        }
                        output.print('');
                    }
                }
            },

            graph: {
                description: 'Show dependency graph between splits',
                usage: 'graph',
                handler: function() {
                    if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
                        output.print('No plan \u2014 run "plan" or "run" first.'); return;
                    }
                    output.print(prSplit.renderAsciiGraph(prSplit.buildDependencyGraph(st.planCache, null)));
                    var indPairs = prSplit.assessIndependence(st.planCache, null);
                    if (indPairs.length > 0) {
                        output.print('');
                        output.print('Independent pairs (can merge in parallel):');
                        for (var p = 0; p < indPairs.length; p++) {
                            output.print('  ' + indPairs[p][0] + '  \u2194  ' + indPairs[p][1]);
                        }
                    }
                }
            },

            telemetry: {
                description: 'Show session telemetry (local, never sent externally)',
                usage: 'telemetry [save]',
                handler: function(args) {
                    if (args && args[0] === 'save') {
                        var saveResult = prSplit.saveTelemetry();
                        if (saveResult.error) output.print('Error: ' + saveResult.error);
                        else output.print('Telemetry saved to: ' + saveResult.path);
                        return;
                    }
                    var summary = prSplit.getTelemetrySummary();
                    output.print('Session Telemetry (local only):');
                    output.print('  Files analyzed:      ' + (summary.filesAnalyzed || 0));
                    output.print('  Splits created:      ' + (summary.splitCount || 0));
                    output.print('  Claude interactions: ' + (summary.claudeInteractions || 0));
                    output.print('  Conflicts resolved:  ' + (summary.conflictsResolved || 0));
                }
            },

            retro: {
                description: 'Analyze completed split \u2014 insights and suggestions',
                usage: 'retro',
                handler: function() {
                    if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
                        output.print('No plan to analyze.'); return;
                    }
                    var verifyResults = st.executionResultCache && st.executionResultCache.length > 0 ? st.executionResultCache : null;
                    var result = prSplit.analyzeRetrospective(st.planCache, verifyResults, null);
                    output.print('Retrospective Analysis');
                    output.print('======================');
                    output.print('');
                    output.print('Statistics:');
                    output.print('  Total files:      ' + result.stats.totalFiles);
                    output.print('  Splits:           ' + result.stats.splitCount);
                    output.print('  Avg files/split:  ' + result.stats.avgFiles);
                    output.print('  Max files/split:  ' + result.stats.maxFiles);
                    output.print('  Min files/split:  ' + result.stats.minFiles);
                    output.print('  Size balance:     ' + result.stats.balance);
                    output.print('  Failed splits:    ' + result.stats.failedSplits);
                    output.print('');
                    output.print('Score: ' + result.score + '/100');
                    output.print('');
                    if (result.insights.length > 0) {
                        for (var i = 0; i < result.insights.length; i++) {
                            var ins = result.insights[i];
                            var icon = ins.type === 'error' ? '\u274c' : (ins.type === 'warning' ? '\u26a0\ufe0f' : (ins.type === 'success' ? '\u2705' : '\u2139\ufe0f'));
                            output.print('  ' + icon + ' ' + ins.message);
                            if (ins.suggestion) output.print('    \u2192 ' + ins.suggestion);
                        }
                    }
                }
            },

            hud: {
                description: 'Toggle HUD overlay showing Claude process state',
                usage: 'hud [on|off|detail|lines N]',
                handler: function(args) {
                    var sub = (args && args.length > 0) ? args[0] : '';

                    if (sub === 'on') {
                        _hudEnabled = true;
                        _updateHudStatus();
                        output.print('HUD overlay enabled. Status bar shows live process state.');
                        return;
                    }
                    if (sub === 'off') {
                        _hudEnabled = false;
                        if (typeof tuiMux !== 'undefined' && tuiMux &&
                            typeof tuiMux.setStatus === 'function') {
                            tuiMux.setStatus('idle');
                        }
                        output.print('HUD overlay disabled.');
                        return;
                    }
                    if (sub === 'detail') {
                        output.print(_renderHudPanel());
                        return;
                    }
                    if (sub === 'lines') {
                        var n = parseInt(args[1], 10);
                        if (isNaN(n) || n < 1 || n > 50) {
                            output.print('Usage: hud lines <1-50>');
                            return;
                        }
                        _hudMaxLines = n;
                        output.print('HUD output lines set to ' + n + '.');
                        return;
                    }

                    // Default: toggle + show panel.
                    _hudEnabled = !_hudEnabled;
                    if (_hudEnabled) {
                        _updateHudStatus();
                        output.print(_renderHudPanel());
                    } else {
                        if (typeof tuiMux !== 'undefined' && tuiMux &&
                            typeof tuiMux.setStatus === 'function') {
                            tuiMux.setStatus('idle');
                        }
                        output.print('HUD overlay disabled.');
                    }
                }
            },

            help: {
                description: 'Show available commands',
                usage: 'help',
                handler: function() {
                    output.print('PR Split Commands:');
                    output.print('');
                    output.print('  analyze [base]   Analyze diff against base branch');
                    output.print('  stats            Show file-level diff stats');
                    output.print('  group [strategy] Group files by strategy');
                    output.print('  plan             Create split plan from groups');
                    output.print('  preview          Show detailed plan preview');
                    output.print('  move             Move file between splits');
                    output.print('  rename           Rename a split');
                    output.print('  merge            Merge two splits');
                    output.print('  reorder          Reorder splits');
                    output.print('  execute          Execute the split (create branches)');
                    output.print('  verify           Verify each split branch');
                    output.print('  equivalence      Check tree hash equivalence');
                    output.print('  fix              Auto-fix splits that fail verification');
                    output.print('  cleanup          Delete all split branches');
                    output.print('  create-prs       Push + create stacked GitHub PRs');
                    output.print('  run              Full workflow: analyze\u2192group\u2192plan\u2192execute');
                    output.print('  auto-split       Automated split via Claude Code');
                    output.print('  edit-plan        Interactive plan editor');
                    output.print('  diff <n|name>    Colorized diff for a split');
                    output.print('  conversation     Claude conversation history');
                    output.print('  graph            Dependency graph between splits');
                    output.print('  telemetry [save] Session telemetry (local only)');
                    output.print('  retro            Retrospective: insights and suggestions');
                    output.print('  set <key> <val>  Set runtime config');
                    output.print('  copy             Copy plan to clipboard');
                    output.print('  report           Output current state as JSON');
                    output.print('  save-plan [path] Save plan to file');
                    output.print('  load-plan [path] Load plan from file');
                    output.print('  hud [on|off]     Toggle Claude process HUD overlay');
                    output.print('  help             Show this help');
                }
            }
        };
    }

    prSplit._buildCommands = buildCommands;

    // -----------------------------------------------------------------------
    //  HUD Overlay — Process state display when Claude pane is backgrounded
    //
    //  Shows: (1) activity indicator (live/idle), (2) last N lines of Claude
    //  output from VTerm screenshot, (3) current wizard state.
    //  Rendered with lipgloss for clean terminal styling.
    //  Toggled via the "hud" command. When enabled, the status bar shows a
    //  compact summary and "hud" prints the full panel.
    // -----------------------------------------------------------------------

    var _hudEnabled = false;
    var _hudMaxLines = 5;

    // _getActivityInfo returns { icon, label, ms } describing child activity.
    function _getActivityInfo() {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.lastActivityMs !== 'function') {
            return { icon: '\u2753', label: 'unknown', ms: -1 }; // ❓
        }
        var ms = tuiMux.lastActivityMs();
        if (ms < 0) {
            return { icon: '\u23f8\ufe0f', label: 'no output yet', ms: ms }; // ⏸️
        }
        if (ms < 2000) {
            return { icon: '\ud83d\udd04', label: 'LIVE (' + (ms < 1000 ? '<1s' : Math.round(ms / 1000) + 's') + ' ago)', ms: ms }; // 🔄
        }
        if (ms < 10000) {
            return { icon: '\u23f3', label: 'idle (' + Math.round(ms / 1000) + 's ago)', ms: ms }; // ⏳
        }
        if (ms < 60000) {
            return { icon: '\ud83d\udca4', label: 'quiet (' + Math.round(ms / 1000) + 's ago)', ms: ms }; // 💤
        }
        return { icon: '\ud83d\udca4', label: 'quiet (' + Math.round(ms / 60000) + 'm ago)', ms: ms }; // 💤
    }

    // _getLastOutputLines extracts the last N non-empty lines from VTerm.
    function _getLastOutputLines(n) {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.screenshot !== 'function') {
            return [];
        }
        try {
            var screen = tuiMux.screenshot();
            if (!screen) return [];
            var lines = screen.split('\n');
            // Remove trailing empty lines.
            while (lines.length > 0 && lines[lines.length - 1].trim() === '') {
                lines.pop();
            }
            return lines.slice(-n);
        } catch (e) {
            log.printf('hud: screenshot failed — %s', e.message || String(e));
            return ['(screenshot unavailable)'];
        }
    }

    // _renderHudPanel builds the full HUD panel using lipgloss.
    function _renderHudPanel() {
        var style = prSplit._style;
        if (!style || typeof style.header !== 'function') {
            return '(HUD unavailable — styles not loaded)';
        }
        var activity = _getActivityInfo();
        var _ws = prSplit._wizardState;
        var wizState;
        if (typeof _ws !== 'undefined' && _ws && typeof _ws.current === 'function') {
            wizState = _ws.current();
        } else {
            wizState = 'N/A';
        }
        var lastLines = _getLastOutputLines(_hudMaxLines);

        var lines = [];
        lines.push(style.header('\u250c\u2500 Claude Process HUD \u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500'));
        lines.push('\u2502 Status: ' + activity.icon + ' ' + style.bold(activity.label));
        lines.push('\u2502 Wizard: ' + style.info(String(wizState)));
        lines.push('\u2502');

        if (lastLines.length > 0) {
            lines.push('\u2502 ' + style.dim('Last output:'));
            for (var i = 0; i < lastLines.length; i++) {
                // Truncate long lines for clean display.
                var line = lastLines[i];
                if (line.length > 72) {
                    line = line.substring(0, 69) + '...';
                }
                lines.push('\u2502   ' + style.dim(line));
            }
        } else {
            lines.push('\u2502 ' + style.dim('(no output captured)'));
        }

        lines.push(style.header('\u2514\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500'));
        return lines.join('\n');
    }

    // _renderHudStatusLine builds a compact one-line status for the status bar.
    function _renderHudStatusLine() {
        var activity = _getActivityInfo();
        var _ws = prSplit._wizardState;
        var wizState;
        if (typeof _ws !== 'undefined' && _ws && typeof _ws.current === 'function') {
            wizState = _ws.current();
        } else {
            wizState = '?';
        }
        var lastLines = _getLastOutputLines(1);
        var lastSnippet = '';
        if (lastLines.length > 0) {
            lastSnippet = lastLines[0];
            if (lastSnippet.length > 30) {
                lastSnippet = lastSnippet.substring(0, 27) + '...';
            }
            lastSnippet = ' | "' + lastSnippet.trim() + '"';
        }
        return '[' + activity.icon + '] ' + wizState + lastSnippet;
    }

    // _updateHudStatus refreshes the status bar if HUD is active.
    function _updateHudStatus() {
        if (!_hudEnabled) return;
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.setStatus !== 'function') return;
        try {
            tuiMux.setStatus(_renderHudStatusLine());
        } catch (e) {
            log.printf('hud: setStatus failed — %s', e.message || String(e));
        }
    }

    prSplit._hudEnabled = function() { return _hudEnabled; };
    prSplit._renderHudPanel = _renderHudPanel;
    prSplit._renderHudStatusLine = _renderHudStatusLine;
    prSplit._getActivityInfo = _getActivityInfo;
    prSplit._getLastOutputLines = _getLastOutputLines;

    // -----------------------------------------------------------------------
    //  Bell Event Handling
    //
    //  When the Claude pane emits a BEL character (e.g., command completion,
    //  error, or explicit notification), flash the status bar and log the
    //  event. This provides feedback when the user is viewing the OSM TUI
    //  and the Claude process needs attention.
    // -----------------------------------------------------------------------

    if (typeof tuiMux !== 'undefined' && tuiMux && typeof tuiMux.on === 'function') {
        var _bellCount = 0;
        var _bellFlashTimer = null;

        tuiMux.on('bell', function(data) {
            _bellCount++;
            log.printf('bell: received bell #%d from pane=%s', _bellCount, data && data.pane || 'unknown');

            // Flash the status bar to draw the user's attention.
            if (typeof tuiMux.setStatus === 'function') {
                var prevStatus = tuiState._bellPrevStatus || 'idle';
                tuiMux.setStatus('\u0007 BELL — Claude needs attention');

                // Restore status after 3 seconds (debounced).
                if (_bellFlashTimer) {
                    try { clearTimeout(_bellFlashTimer); } catch(e) { /* ignore */ }
                }
                _bellFlashTimer = setTimeout(function() {
                    tuiMux.setStatus(prevStatus);
                    _bellFlashTimer = null;
                }, 3000);
            }
        });

        // Expose bell count for test observability.
        prSplit._bellCount = function() { return _bellCount; };
    }

    // -----------------------------------------------------------------------

    //  BubbleTea Wizard — Full-screen TUI replacing the go-prompt REPL
    // ═══════════════════════════════════════════════════════════════════════

    var tea = require('osm:bubbletea');
    var lipgloss = require('osm:lipgloss');
    var zone = require('osm:bubblezone');
    var viewportLib = require('osm:bubbles/viewport');
    var scrollbarLib = require('osm:termui/scrollbar');

    // -----------------------------------------------------------------------
    //  COLORS & Styles (T006)
    //
    //  Design spec: docs/pr-split-tui-design.md §2
    // -----------------------------------------------------------------------

    // Adaptive color palette: auto-detects light/dark terminal background.
    // Uses {light, dark} objects resolved by lipgloss.AdaptiveColor.
    // Palette: high-contrast, distinct hues, WCAG AA compliant.
    var COLORS = {
        primary:   {light: '#6D28D9', dark: '#A78BFA'},  // Purple accent
        secondary: {light: '#4338CA', dark: '#818CF8'},  // Indigo
        success:   {light: '#15803D', dark: '#4ADE80'},  // Green
        warning:   {light: '#A16207', dark: '#FACC15'},  // Amber
        error:     {light: '#DC2626', dark: '#F87171'},  // Red
        muted:     {light: '#6B7280', dark: '#9CA3AF'},  // Gray
        surface:   {light: '#F3F4F6', dark: '#1F2937'},  // Card bg
        border:    {light: '#D1D5DB', dark: '#4B5563'},  // Borders
        text:      {light: '#111827', dark: '#F9FAFB'},  // Primary text
        textDim:   {light: '#6B7280', dark: '#9CA3AF'}   // Secondary text
    };

    // Resolve adaptive color to a plain string (for APIs that don't support objects).
    function resolveColor(c) {
        if (typeof c === 'string') return c;
        if (c && typeof c === 'object' && c.light && c.dark) {
            return lipgloss.hasDarkBackground() ? c.dark : c.light;
        }
        return '';
    }

    var styles = {
        titleBar: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.text)
                .background(COLORS.primary)
                .padding(0, 1);
        },
        stepIndicator: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.textDim);
        },
        activeCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.primary)
                .padding(1, 2);
        },
        inactiveCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.border)
                .padding(1, 2);
        },
        errorCard: function() {
            return lipgloss.newStyle()
                .border(lipgloss.normalBorder())
                .borderForeground(COLORS.error)
                .padding(1, 2);
        },
        successBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground('#000000')
                .background(COLORS.success)
                .padding(0, 1);
        },
        warningBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground('#000000')
                .background(COLORS.warning)
                .padding(0, 1);
        },
        errorBadge: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground('#000000')
                .background(COLORS.error)
                .padding(0, 1);
        },
        primaryButton: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.text)
                .background(COLORS.primary)
                .padding(0, 2);
        },
        secondaryButton: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.text)
                .background(COLORS.surface)
                .padding(0, 2)
                .border(lipgloss.roundedBorder())
                .borderForeground(COLORS.border);
        },
        disabledButton: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.muted)
                .background(COLORS.surface)
                .padding(0, 2);
        },
        progressFull: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.success);
        },
        progressEmpty: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.border);
        },
        divider: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.border);
        },
        dim: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.textDim);
        },
        bold: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.text);
        },
        label: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.text);
        },
        fieldValue: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.secondary);
        },
        statusIdle: function() {
            return lipgloss.newStyle()
                .foreground(COLORS.muted);
        },
        statusActive: function() {
            return lipgloss.newStyle()
                .bold(true)
                .foreground(COLORS.warning);
        }
    };

    // Export styles for test access.
    prSplit._wizardStyles = styles;
    prSplit._wizardColors = COLORS;

    // -----------------------------------------------------------------------
    //  Chrome Renderers (T007)
    //
    //  Title bar, navigation bar, status bar, step dots
    // -----------------------------------------------------------------------

    var STEP_LABELS = [
        'Configure',
        'Analysis',
        'Review Plan',
        'Edit Plan',
        'Execution',
        'Verification',
        'Finalization'
    ];

    // Map wizard states to step indices (0-based).
    var STATE_TO_STEP = {
        'IDLE':             0,
        'CONFIG':           0,
        'BASELINE_FAIL':    0,
        'PLAN_GENERATION':  1,
        'PLAN_REVIEW':      2,
        'PLAN_EDITOR':      3,
        'BRANCH_BUILDING':  4,
        'ERROR_RESOLUTION': 4,
        'EQUIV_CHECK':      5,
        'FINALIZATION':     6,
        'DONE':             6,
        'CANCELLED':        6,
        'FORCE_CANCEL':     6,
        'PAUSED':           6,
        'ERROR':            6
    };

    function renderTitleBar(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var stepLabel = STEP_LABELS[stepIdx] || 'Unknown';
        var stepNum = stepIdx + 1;
        var totalSteps = 7;

        // Elapsed time.
        var elapsed = s.startTime ? Math.floor((Date.now() - s.startTime) / 1000) : 0;
        var mins = Math.floor(elapsed / 60);
        var secs = elapsed % 60;
        var timeStr = mins + ':' + (secs < 10 ? '0' : '') + secs;

        var left = styles.titleBar().render('\ud83d\udd00 PR Split Wizard');
        var right = styles.stepIndicator().render(
            'Step ' + stepNum + '/' + totalSteps + ': ' + stepLabel + '  \u23f1 ' + timeStr
        );

        var w = s.width || 80;
        var leftW = lipgloss.width(left);
        var rightW = lipgloss.width(right);
        var gap = Math.max(1, w - leftW - rightW);
        var gapStr = '';
        for (var i = 0; i < gap; i++) gapStr += ' ';

        return left + gapStr + right;
    }

    function renderStepDots(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var dots = '';
        for (var i = 0; i < 7; i++) {
            if (i <= stepIdx) {
                dots += styles.progressFull().render('\u25cf');
            } else {
                dots += styles.progressEmpty().render('\u25cb');
            }
        }
        return dots;
    }

    function renderNavBar(s) {
        var stepIdx = STATE_TO_STEP[s.wizardState] || 0;
        var w = s.width || 80;
        var narrow = w < 50;

        // Back button (not on first screen).
        var backBtn = '';
        if (stepIdx > 0 && !s.isProcessing) {
            var backLabel = narrow ? '\u2190' : '\u2190 Back';
            backBtn = zone.mark('nav-back',
                styles.secondaryButton().render(backLabel));
        }

        // Cancel button.
        var cancelLabel = narrow ? '\u00d7' : 'Cancel';
        var cancelBtn = zone.mark('nav-cancel',
            styles.secondaryButton().render(cancelLabel));

        // Dots.
        var dots = renderStepDots(s);

        // Next/action button.
        var nextBtn = '';
        if (s.isProcessing) {
            nextBtn = styles.disabledButton().render(narrow ? '...' : 'Processing...');
        } else {
            var nextLabel = getNextButtonLabel(s);
            if (nextLabel) {
                if (narrow && nextLabel.length > 8) {
                    nextLabel = nextLabel.split(' ')[0];
                }
                nextBtn = zone.mark('nav-next',
                    styles.primaryButton().render(nextLabel + ' \u2192'));
            }
        }

        // Build left section (back + cancel).
        var leftParts = [];
        if (backBtn) leftParts.push(backBtn);
        leftParts.push(cancelBtn);
        var leftSection = leftParts.length > 1
            ? lipgloss.joinHorizontal(lipgloss.Center, leftParts[0], '  ', leftParts[1])
            : leftParts[0];

        // Build right section (dots + next).
        var rightParts = [dots];
        if (nextBtn) rightParts.push(nextBtn);
        var rightSection = rightParts.length > 1
            ? lipgloss.joinHorizontal(lipgloss.Center, rightParts[0], '  ', rightParts[1])
            : rightParts[0];

        // Compose full nav bar: left ... right, spread across width.
        var leftW = lipgloss.width(leftSection);
        var rightW = lipgloss.width(rightSection);
        var gap = Math.max(2, w - leftW - rightW);
        var spacer = lipgloss.newStyle().width(gap).render('');

        return lipgloss.joinHorizontal(lipgloss.Center, leftSection, spacer, rightSection);
    }

    function getNextButtonLabel(s) {
        switch (s.wizardState) {
            case 'IDLE': case 'CONFIG':
                return 'Start Analysis';
            case 'PLAN_REVIEW':
                return 'Execute Plan';
            case 'PLAN_EDITOR':
                return 'Save & Review';
            case 'ERROR_RESOLUTION':
                return 'Retry';
            case 'FINALIZATION':
                return 'Finish';
            default:
                return '';
        }
    }

    function renderStatusBar(s) {
        var w = s.width || 80;
        var narrow = w < 60;
        var veryNarrow = w < 40;

        // Left: termmux toggle hint.
        var left = styles.dim().render(veryNarrow ? 'C-]' : 'Ctrl+] Claude');

        // Center: help.
        var center = veryNarrow ? '' : styles.dim().render('? Help');

        // Right: Claude process status.
        var claudeStatus = getClaudeStatusText(s);
        var right = narrow ? '' : zone.mark('claude-status', claudeStatus);

        // Build status line with guaranteed minimum spacing.
        var items = [left];
        if (center) items.push(center);
        if (right) items.push(right);

        var totalItemW = 0;
        for (var ii = 0; ii < items.length; ii++) {
            totalItemW += lipgloss.width(items[ii]);
        }

        var statusLine;
        if (items.length === 1) {
            statusLine = items[0];
        } else {
            // Distribute remaining space evenly between items.
            var gapCount = items.length - 1;
            var remaining = Math.max(gapCount * 2, w - totalItemW);
            var perGap = Math.floor(remaining / gapCount);
            var parts = items[0];
            for (var ix = 1; ix < items.length; ix++) {
                var g = '';
                for (var gi = 0; gi < perGap; gi++) g += ' ';
                parts += g + items[ix];
            }
            statusLine = parts;
        }

        return styles.dim().render(
            styles.divider().render(repeatStr('\u2500', w))
        ) + '\n' + statusLine;
    }

    function getClaudeStatusText(s) {
        if (typeof tuiMux === 'undefined' || !tuiMux ||
            typeof tuiMux.lastActivityMs !== 'function') {
            return styles.statusIdle().render('\ud83d\udca4 Claude: N/A');
        }
        var ms = tuiMux.lastActivityMs();
        if (ms < 0) return styles.statusIdle().render('\u23f8\ufe0f Claude: no output');
        if (ms < 2000) return styles.statusActive().render('\ud83d\udd04 Claude: LIVE');
        if (ms < 10000) return styles.statusIdle().render('\u23f3 Claude: idle (' + Math.round(ms / 1000) + 's)');
        return styles.statusIdle().render('\ud83d\udca4 Claude: quiet');
    }

    // Export chrome for testing.
    prSplit._renderTitleBar = renderTitleBar;
    prSplit._renderNavBar = renderNavBar;
    prSplit._renderStatusBar = renderStatusBar;
    prSplit._renderStepDots = renderStepDots;

    // -----------------------------------------------------------------------
    //  Helpers (T008)
    //
    //  Progress bar, truncation, padding
    // -----------------------------------------------------------------------

    function renderProgressBar(percent, width) {
        var barW = Math.max(10, (width || 40) - 10);
        var filled = Math.round(barW * Math.min(1, Math.max(0, percent)));
        var empty = barW - filled;
        var bar = styles.progressFull().render(repeatStr('\u2588', filled)) +
                  styles.progressEmpty().render(repeatStr('\u2591', empty));
        var pctStr = Math.round(percent * 100) + '%';
        return bar + '  ' + pctStr;
    }

    function truncate(str, maxLen) {
        if (!str) return '';
        if (str.length <= maxLen) return str;
        return str.substring(0, maxLen - 3) + '...';
    }

    function padRight(str, width) {
        str = str || '';
        while (str.length < width) str += ' ';
        return str;
    }

    function repeatStr(ch, n) {
        var s = '';
        for (var i = 0; i < n; i++) s += ch;
        return s;
    }

    prSplit._renderProgressBar = renderProgressBar;

    // -----------------------------------------------------------------------
    //  Screen Renderers (T009-T017)
    //
    //  Each screen returns a string for the content area.
    //  The update function sets model state; view calls the appropriate
    //  screen renderer based on wizardState.
    // -----------------------------------------------------------------------

    // ----- Screen 1: Configuration (IDLE / CONFIG) -----

    function viewConfigScreen(s) {
        var runtime = prSplit.runtime;
        var lines = [];

        // Repository info.
        lines.push(styles.bold().render('Repository'));
        lines.push('  ' + styles.fieldValue().render(runtime.dir || '.'));
        lines.push('');

        // Config form (non-editable display for now, wizard sets these).
        lines.push(styles.bold().render('Source Branch'));
        var srcBranch = (st.analysisCache && st.analysisCache.currentBranch) || '(auto-detect)';
        lines.push('  ' + styles.activeCard().width(Math.max(20, (s.width || 80) - 12)).render(
            styles.fieldValue().render(srcBranch)
        ));
        lines.push('');

        lines.push(styles.bold().render('Target Branch'));
        lines.push('  ' + styles.activeCard().width(Math.max(20, (s.width || 80) - 12)).render(
            styles.fieldValue().render(runtime.baseBranch || 'main')
        ));
        lines.push('');

        // Strategy selection.
        lines.push(styles.bold().render('Strategy'));
        var strategies = ['auto', 'heuristic', 'directory'];
        var currentMode = runtime.mode || 'heuristic';
        for (var si = 0; si < strategies.length; si++) {
            var strat = strategies[si];
            var selected = (strat === currentMode);
            var bullet = selected ? styles.primaryButton().render(' \u25cf ') : '  \u25cb ';
            var label = styles.label().render(strat.charAt(0).toUpperCase() + strat.slice(1));
            var stratId = 'strategy-' + strat;
            lines.push('  ' + zone.mark(stratId, bullet + ' ' + label));
        }
        lines.push('');

        // Advanced options toggle.
        if (s.showAdvanced) {
            lines.push(styles.dim().render('\u25be Advanced Options'));
            lines.push('  Max files per chunk: ' +
                styles.fieldValue().render(String(runtime.maxFiles || 10)));
            lines.push('  Branch prefix:       ' +
                styles.fieldValue().render(runtime.branchPrefix || 'split/'));
            lines.push('  Verify command:      ' +
                styles.fieldValue().render(runtime.verifyCommand || 'true'));
            lines.push('  ' + (runtime.dryRun ? '\u2611' : '\u2610') + ' Dry run');
        } else {
            lines.push(zone.mark('toggle-advanced',
                styles.dim().render('\u25b8 Advanced Options')));
        }

        return lines.join('\n');
    }

    // ----- Screen 2: Analysis (PLAN_GENERATION) -----

    function viewAnalysisScreen(s) {
        var lines = [];
        var runtime = prSplit.runtime;

        lines.push(styles.bold().render('Analyzing Changes'));
        lines.push('  ' + styles.fieldValue().render(
            (st.analysisCache && st.analysisCache.currentBranch || '?') +
            ' \u2192 ' + (runtime.baseBranch || 'main')
        ));
        lines.push('');

        // Progress steps.
        var steps = s.analysisSteps || [];
        for (var i = 0; i < steps.length; i++) {
            var step = steps[i];
            var icon = step.done ? styles.successBadge().render(' \u2713 ') :
                       step.active ? styles.warningBadge().render(' \u25b6 ') :
                       styles.dim().render(' \u25cb ');
            var stepLabel = styles.label().render(step.label);
            var stepTime = step.elapsed ? styles.dim().render(' (' + step.elapsed + 'ms)') : '';
            lines.push('  ' + icon + ' ' + stepLabel + stepTime);
        }

        if (s.analysisProgress >= 0) {
            lines.push('');
            lines.push('  ' + renderProgressBar(s.analysisProgress, (s.width || 80) - 8));
        }

        // Show analysis results if available.
        if (st.analysisCache && st.analysisCache.files && !st.analysisCache.error) {
            lines.push('');
            lines.push(styles.successBadge().render(' Analysis Complete '));
            lines.push('  Changed files: ' +
                styles.fieldValue().render(String(st.analysisCache.files.length)));

            // File list (abbreviated).
            var maxShow = Math.min(10, st.analysisCache.files.length);
            for (var f = 0; f < maxShow; f++) {
                var file = st.analysisCache.files[f];
                var status = (st.analysisCache.fileStatuses && st.analysisCache.fileStatuses[file]) || '?';
                lines.push('    [' + status + '] ' + truncate(file, (s.width || 80) - 16));
            }
            if (st.analysisCache.files.length > maxShow) {
                lines.push(styles.dim().render(
                    '    ... and ' + (st.analysisCache.files.length - maxShow) + ' more'));
            }
        }

        if (st.analysisCache && st.analysisCache.error) {
            lines.push('');
            lines.push(styles.errorBadge().render(' Error ') + ' ' +
                styles.errorCard().render(st.analysisCache.error));
        }

        return lines.join('\n');
    }

    // ----- Screen 3: Plan Review (PLAN_REVIEW) -----

    function viewPlanReviewScreen(s) {
        var lines = [];

        if (!st.planCache) {
            lines.push(styles.warningBadge().render(' No Plan ') +
                ' Run analysis first.');
            return lines.join('\n');
        }

        var plan = st.planCache;
        lines.push(styles.bold().render('Split Plan Overview'));
        lines.push('  Splits: ' + styles.fieldValue().render(String(plan.splits.length)));
        lines.push('  Base: ' + styles.fieldValue().render(plan.baseBranch || 'main'));
        lines.push('');

        // Split cards.
        var w = (s.width || 80) - 8;
        var selectedIdx = s.selectedSplitIdx || 0;

        for (var i = 0; i < plan.splits.length; i++) {
            var split = plan.splits[i];
            var isSelected = (i === selectedIdx);
            var cardStyle = isSelected ? styles.activeCard() : styles.inactiveCard();
            var cardId = 'split-card-' + i;

            var cardContent = '';
            cardContent += styles.bold().render(
                (i + 1) + '. ' + (split.name || 'split-' + i)) + '\n';
            cardContent += styles.dim().render(split.message || '') + '\n';
            cardContent += styles.fieldValue().render(
                split.files.length + ' file' + (split.files.length !== 1 ? 's' : ''));

            // Show files for selected split.
            if (isSelected && split.files) {
                cardContent += '\n';
                for (var fi = 0; fi < split.files.length; fi++) {
                    var fStatus = (plan.fileStatuses && plan.fileStatuses[split.files[fi]]) || '?';
                    cardContent += '\n  [' + fStatus + '] ' + truncate(split.files[fi], w - 10);
                }
            }

            lines.push(zone.mark(cardId, cardStyle.width(w).render(cardContent)));
        }

        // Action buttons.
        lines.push('');
        lines.push(
            zone.mark('plan-edit', styles.secondaryButton().render('Edit Plan \u270f')) +
            '  ' +
            zone.mark('plan-regenerate', styles.secondaryButton().render('Regenerate \ud83d\udd04'))
        );

        return lines.join('\n');
    }

    // ----- Screen 4: Plan Editor (PLAN_EDITOR) -----

    function viewPlanEditorScreen(s) {
        var lines = [];

        if (!st.planCache) {
            lines.push('No plan to edit.');
            return lines.join('\n');
        }

        lines.push(styles.bold().render('Edit Split Plan'));
        lines.push(styles.dim().render(
            'Click a split to select. Use Move/Rename buttons to modify.'));
        lines.push('');

        var plan = st.planCache;
        var selectedIdx = s.selectedSplitIdx || 0;
        var w = (s.width || 80) - 8;

        for (var i = 0; i < plan.splits.length; i++) {
            var split = plan.splits[i];
            var isSelected = (i === selectedIdx);
            var badge = isSelected ? styles.primaryButton().render(' \u25b6 ') : '  ' + (i + 1) + '. ';
            var name = styles.bold().render(split.name || 'split-' + i);
            var files = styles.dim().render(split.files.length + ' files');
            var cardId = 'edit-split-' + i;

            lines.push(zone.mark(cardId, badge + ' ' + name + '  ' + files));

            if (isSelected && split.files) {
                for (var fi = 0; fi < split.files.length; fi++) {
                    var fileId = 'edit-file-' + i + '-' + fi;
                    lines.push('    ' + zone.mark(fileId,
                        styles.dim().render(split.files[fi])));
                }
            }
        }

        if (isSelected) {
            lines.push('');
            lines.push(
                zone.mark('editor-move', styles.secondaryButton().render('Move File')) +
                '  ' +
                zone.mark('editor-rename', styles.secondaryButton().render('Rename Split')) +
                '  ' +
                zone.mark('editor-merge', styles.secondaryButton().render('Merge Splits'))
            );
        }

        return lines.join('\n');
    }

    // ----- Screen 5: Execution (BRANCH_BUILDING) -----

    function viewExecutionScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('Executing Split Plan'));
        lines.push('');

        if (!st.planCache || !st.planCache.splits) {
            lines.push('No plan to execute.');
            return lines.join('\n');
        }

        var splits = st.planCache.splits;
        var results = s.executionResults || [];
        var currentIdx = s.executingIdx || 0;

        for (var i = 0; i < splits.length; i++) {
            var split = splits[i];
            var result = results[i];
            var icon, statusText;

            if (result && result.error) {
                icon = styles.errorBadge().render(' \u2718 ');
                statusText = styles.errorCard().width((s.width || 80) - 16).render(
                    split.name + '\n' + result.error);
            } else if (result) {
                icon = styles.successBadge().render(' \u2713 ');
                statusText = styles.label().render(split.name) +
                    styles.dim().render(' \u2192 ' + (result.sha ? result.sha.substring(0, 8) : ''));
            } else if (i === currentIdx && s.isProcessing) {
                icon = styles.warningBadge().render(' \u25b6 ');
                statusText = styles.statusActive().render(split.name + '...');
            } else {
                icon = styles.dim().render(' \u25cb ');
                statusText = styles.dim().render(split.name);
            }

            lines.push('  ' + icon + ' ' + statusText);
        }

        // Overall progress.
        if (splits.length > 0) {
            lines.push('');
            var progress = results.length / splits.length;
            lines.push('  ' + renderProgressBar(progress, (s.width || 80) - 8));
        }

        return lines.join('\n');
    }

    // ----- Screen 6: Verification (EQUIV_CHECK) -----

    function viewVerificationScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('Verifying Equivalence'));
        lines.push('');

        if (s.isProcessing) {
            lines.push('  ' + styles.warningBadge().render(' \u25b6 ') +
                ' Checking tree hash equivalence...');
            lines.push('');
            lines.push('  ' + renderProgressBar(0.5, (s.width || 80) - 8));
        } else if (s.equivalenceResult) {
            var equiv = s.equivalenceResult;
            if (equiv.equivalent) {
                lines.push('  ' + styles.successBadge().render(' PASS ') +
                    ' Tree hashes match!');
                lines.push('');
                lines.push(styles.dim().render(
                    '  All splits merge to produce identical content as the source branch.'));
            } else if (equiv.error) {
                lines.push('  ' + styles.errorBadge().render(' ERROR ') + ' ' + equiv.error);
            } else {
                lines.push('  ' + styles.errorBadge().render(' FAIL ') +
                    ' Tree hash mismatch');
                if (equiv.expected) {
                    lines.push('    Expected: ' + styles.fieldValue().render(equiv.expected));
                }
                if (equiv.actual) {
                    lines.push('    Actual:   ' + styles.fieldValue().render(equiv.actual));
                }
            }

            // Skipped branches note.
            if (equiv.skippedBranches && equiv.skippedBranches.length > 0) {
                lines.push('');
                lines.push(styles.warningBadge().render(' Note ') +
                    ' Skipped ' + equiv.skippedBranches.length + ' branch(es)');
            }
        }

        return lines.join('\n');
    }

    // ----- Screen 7: Finalization (FINALIZATION / DONE) -----

    function viewFinalizationScreen(s) {
        var lines = [];

        lines.push(styles.bold().render('PR Split Complete'));
        lines.push('');

        // Summary stats.
        var plan = st.planCache;
        if (plan && plan.splits) {
            lines.push('  Splits created: ' +
                styles.successBadge().render(' ' + plan.splits.length + ' '));
            lines.push('');

            for (var i = 0; i < plan.splits.length; i++) {
                var split = plan.splits[i];
                lines.push('    ' + (i + 1) + '. ' +
                    styles.fieldValue().render(split.name) +
                    styles.dim().render(' (' + split.files.length + ' files)'));
            }
        }

        // Equivalence result.
        if (s.equivalenceResult) {
            lines.push('');
            if (s.equivalenceResult.equivalent) {
                lines.push('  ' + styles.successBadge().render(' \u2705 Equivalence Verified '));
            } else {
                lines.push('  ' + styles.warningBadge().render(' \u26a0 Equivalence Not Verified '));
            }
        }

        // Elapsed time.
        if (s.startTime) {
            var totalSec = Math.floor((Date.now() - s.startTime) / 1000);
            lines.push('');
            lines.push(styles.dim().render(
                '  Total time: ' + Math.floor(totalSec / 60) + 'm ' + (totalSec % 60) + 's'));
        }

        // Actions.
        lines.push('');
        lines.push(
            zone.mark('final-report',
                styles.secondaryButton().render('View Report')) +
            '  ' +
            zone.mark('final-create-prs',
                styles.primaryButton().render('Create PRs')) +
            '  ' +
            zone.mark('final-done',
                styles.primaryButton().render('Done'))
        );

        return lines.join('\n');
    }

    // ----- Error Resolution Overlay (T018) -----

    function viewErrorResolutionScreen(s) {
        var lines = [];

        lines.push(styles.errorBadge().render(' Error Resolution '));
        lines.push('');

        if (s.errorDetails) {
            lines.push(styles.errorCard().width((s.width || 80) - 8).render(
                s.errorDetails));
            lines.push('');
        }

        var failedBranches = (s.wizard && s.wizard.data && s.wizard.data.failedBranches) || [];
        if (failedBranches.length > 0) {
            lines.push(styles.bold().render('Failed Branches:'));
            for (var i = 0; i < failedBranches.length; i++) {
                var fb = failedBranches[i];
                var name = fb.name || fb;
                var err = fb.verifyError || fb.error || '';
                lines.push('  ' + styles.errorBadge().render(' \u2718 ') + ' ' +
                    styles.fieldValue().render(name) +
                    (err ? '\n    ' + styles.dim().render(err) : ''));
            }
            lines.push('');
        }

        // Resolution options.
        lines.push(styles.bold().render('Choose Resolution:'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-auto',
            styles.primaryButton().render('Auto-Resolve')) +
            styles.dim().render('  Let Claude fix the issues'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-manual',
            styles.secondaryButton().render('Manual Fix')) +
            styles.dim().render('  Switch to Claude pane to fix manually'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-skip',
            styles.secondaryButton().render('Skip')) +
            styles.dim().render('  Skip failed branches'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-retry',
            styles.secondaryButton().render('Retry')) +
            styles.dim().render('  Regenerate plan from scratch'));
        lines.push('');
        lines.push('  ' + zone.mark('resolve-abort',
            styles.secondaryButton().render('Abort')) +
            styles.dim().render('  Cancel the split'));

        return lines.join('\n');
    }

    // ----- Help Overlay (T019) -----

    function viewHelpOverlay(s) {
        var w = Math.min(60, (s.width || 80) - 4);
        var lines = [];

        lines.push(styles.bold().render('Keyboard Shortcuts'));
        lines.push('');
        lines.push(padRight('  ? / F1', 20) + 'Toggle this help');
        lines.push(padRight('  Tab', 20) + 'Next field / option');
        lines.push(padRight('  Shift+Tab', 20) + 'Previous field / option');
        lines.push(padRight('  Enter', 20) + 'Confirm / select');
        lines.push(padRight('  Esc', 20) + 'Back / close overlay');
        lines.push(padRight('  Ctrl+C', 20) + 'Cancel wizard');
        lines.push(padRight('  Ctrl+]', 20) + 'Toggle Claude pane');
        lines.push(padRight('  j / \u2193', 20) + 'Move down');
        lines.push(padRight('  k / \u2191', 20) + 'Move up');
        lines.push(padRight('  PgUp / PgDn', 20) + 'Scroll page');
        lines.push(padRight('  Home / End', 20) + 'Jump to top / bottom');

        var content = lines.join('\n');
        return styles.activeCard().width(w).render(content);
    }

    // ----- Confirm Cancel Overlay -----

    function viewConfirmCancelOverlay(s) {
        var w = Math.min(50, (s.width || 80) - 4);
        var lines = [];

        lines.push(styles.warningBadge().render(' Cancel Wizard? '));
        lines.push('');
        lines.push('Are you sure you want to cancel the PR split?');
        lines.push('All progress will be lost.');
        lines.push('');
        lines.push(
            zone.mark('confirm-yes',
                styles.errorBadge().render(' Yes, Cancel ')) +
            '    ' +
            zone.mark('confirm-no',
                styles.primaryButton().render(' No, Continue '))
        );

        var content = lines.join('\n');
        return styles.activeCard().width(w).render(content);
    }

    // Map wizard state to the correct screen renderer.
    function viewForState(s) {
        switch (s.wizardState) {
            case 'IDLE':
            case 'CONFIG':
            case 'BASELINE_FAIL':
                return viewConfigScreen(s);
            case 'PLAN_GENERATION':
                return viewAnalysisScreen(s);
            case 'PLAN_REVIEW':
                return viewPlanReviewScreen(s);
            case 'PLAN_EDITOR':
                return viewPlanEditorScreen(s);
            case 'BRANCH_BUILDING':
                return viewExecutionScreen(s);
            case 'ERROR_RESOLUTION':
                return viewErrorResolutionScreen(s);
            case 'EQUIV_CHECK':
                return viewVerificationScreen(s);
            case 'FINALIZATION':
            case 'DONE':
                return viewFinalizationScreen(s);
            case 'CANCELLED':
            case 'FORCE_CANCEL':
                return styles.warningBadge().render(' Cancelled ') +
                    '\n\nThe PR split was cancelled.';
            case 'ERROR':
                return styles.errorBadge().render(' Error ') +
                    '\n\n' + (s.errorDetails || 'An unexpected error occurred.');
            default:
                return 'Unknown state: ' + s.wizardState;
        }
    }

    // Export screen renderers for testing.
    prSplit._viewConfigScreen = viewConfigScreen;
    prSplit._viewAnalysisScreen = viewAnalysisScreen;
    prSplit._viewPlanReviewScreen = viewPlanReviewScreen;
    prSplit._viewPlanEditorScreen = viewPlanEditorScreen;
    prSplit._viewExecutionScreen = viewExecutionScreen;
    prSplit._viewVerificationScreen = viewVerificationScreen;
    prSplit._viewFinalizationScreen = viewFinalizationScreen;
    prSplit._viewErrorResolutionScreen = viewErrorResolutionScreen;
    prSplit._viewHelpOverlay = viewHelpOverlay;
    prSplit._viewConfirmCancelOverlay = viewConfirmCancelOverlay;
    prSplit._viewForState = viewForState;

    // -----------------------------------------------------------------------
    //  BubbleTea Model — init / update / view (T020-T025)
    // -----------------------------------------------------------------------

    function createWizardModel() {
        var wizard = new WizardState();
        prSplit._wizardState = wizard;

        // Track transitions to update the TUI model state.
        wizard.onTransition(function(from, to, data) {
            log.printf('wizard: %s \u2192 %s', from, to);
        });

        var vp = viewportLib.new(80, 24);
        vp.setMouseWheelEnabled(true);
        var sb = scrollbarLib.new();

        // Named lifecycle functions — exported for unit testing.
        var _initFn = function() {
            return {
                // Wizard state.
                wizard: wizard,
                wizardState: 'IDLE',

                // Dimensions.
                width: 80,
                height: 24,

                // Viewport.
                vp: vp,
                scrollbar: sb,

                // Time.
                startTime: Date.now(),

                // UI state.
                showHelp: false,
                showConfirmCancel: false,
                showAdvanced: false,
                selectedSplitIdx: 0,
                isProcessing: false,

                // Analysis progress.
                analysisSteps: [],
                analysisProgress: -1,

                // Execution state.
                executionResults: [],
                executingIdx: 0,

                // Results.
                equivalenceResult: null,
                errorDetails: null,

                // First render flag.
                needsInitClear: true
            };
        };

        var _updateFn = function(msg, s) {
            // WindowSize — always handle.
            if (msg.type === 'WindowSize') {
                s.width = msg.width;
                s.height = msg.height;

                if (s.needsInitClear) {
                    s.needsInitClear = false;
                    // Start the wizard on first render.
                    s.wizardState = 'CONFIG';
                    wizard.transition('CONFIG');
                    return [s, tea.clearScreen()];
                }
                return [s, null];
            }

            // Overlays intercept all input when active.
            if (s.showHelp) {
                if (msg.type === 'Key') {
                    // Any key closes help.
                    s.showHelp = false;
                    return [s, null];
                }
                return [s, null];
            }

            if (s.showConfirmCancel) {
                return updateConfirmCancel(msg, s);
            }

            // Global key bindings.
            if (msg.type === 'Key') {
                var k = msg.key;
                // Help toggle.
                if (k === '?' || k === 'f1') {
                    s.showHelp = true;
                    return [s, null];
                }
                // Cancel.
                if (k === 'ctrl+c') {
                    s.showConfirmCancel = true;
                    return [s, null];
                }
                // Escape — back or close.
                if (k === 'esc') {
                    return handleBack(s);
                }
                // Enter — forward action.
                if (k === 'enter') {
                    return handleNext(s);
                }
                // Navigation: j/k, up/down, tab/shift+tab.
                if (k === 'j' || k === 'down') {
                    return handleNavDown(s);
                }
                if (k === 'k' || k === 'up') {
                    return handleNavUp(s);
                }
                if (k === 'tab') {
                    return handleNavDown(s);
                }
                if (k === 'shift+tab') {
                    return handleNavUp(s);
                }
                // Viewport scroll.
                if (k === 'pgdown') {
                    if (s.vp) s.vp.halfPageDown();
                    return [s, null];
                }
                if (k === 'pgup') {
                    if (s.vp) s.vp.halfPageUp();
                    return [s, null];
                }
                if (k === 'home') {
                    if (s.vp) s.vp.gotoTop();
                    return [s, null];
                }
                if (k === 'end') {
                    if (s.vp) s.vp.gotoBottom();
                    return [s, null];
                }
                // Screen-specific key shortcuts.
                if (k === 'e' && s.wizardState === 'PLAN_REVIEW' && !s.isProcessing) {
                    // Enter plan editor.
                    s.wizard.transition('PLAN_EDITOR');
                    s.wizardState = 'PLAN_EDITOR';
                    return [s, null];
                }
                // termmux toggle.
                if (k === 'ctrl+]') {
                    if (typeof tuiMux !== 'undefined' && tuiMux &&
                        typeof tuiMux.switchTo === 'function') {
                        tuiMux.switchTo('claude');
                    }
                    return [s, null];
                }
            }

            // Mouse handling.
            if (msg.type === 'Mouse' && msg.action === 'press') {
                return handleMouseClick(msg, s);
            }

            // Mouse wheel for viewport.
            if (msg.type === 'Mouse') {
                if (msg.action === 'wheelUp' && s.vp) {
                    s.vp.scrollUp(3);
                    return [s, null];
                }
                if (msg.action === 'wheelDown' && s.vp) {
                    s.vp.scrollDown(3);
                    return [s, null];
                }
            }

            return [s, null];
        };

        var _viewFn = function(s) {
            var w = s.width || 80;
            var h = s.height || 24;

            // Title bar.
            var titleBar = renderTitleBar(s);

            // Divider.
            var divider = styles.divider().render(repeatStr('\u2500', w));

            // Navigation bar.
            var navBar = renderNavBar(s);

            // Status bar.
            var statusBar = renderStatusBar(s);

            // Screen content.
            var screenContent = viewForState(s);

            // Wrap in viewport.
            if (s.vp) {
                s.vp.setWidth(w);
                // Reserve chrome lines dynamically from actual rendered heights.
                // +2 for the two dividers (each 1 line).
                var chromeH = lipgloss.height(titleBar) + 2 + lipgloss.height(navBar) + lipgloss.height(statusBar);
                var vpHeight = Math.max(3, h - chromeH);
                s.vp.setHeight(vpHeight);
                s.vp.setContent(screenContent);

                // Scrollbar.
                if (s.scrollbar) {
                    s.scrollbar.setViewportHeight(vpHeight);
                    s.scrollbar.setContentHeight(s.vp.totalLineCount());
                    s.scrollbar.setYOffset(s.vp.yOffset());
                    s.scrollbar.setChars('\u2588', '\u2591');
                    s.scrollbar.setThumbForeground(resolveColor(COLORS.primary));
                    s.scrollbar.setTrackForeground(resolveColor(COLORS.border));
                }

                var vpView = s.vp.view();
                var sbView = s.scrollbar ? s.scrollbar.view() : '';
                screenContent = lipgloss.joinHorizontal(lipgloss.Top, vpView, sbView);
            }

            // Compose.
            var fullView = lipgloss.joinVertical(lipgloss.Left,
                titleBar,
                divider,
                screenContent,
                divider,
                navBar,
                statusBar
            );

            // Overlay: Help.
            if (s.showHelp) {
                var helpPanel = viewHelpOverlay(s);
                fullView = lipgloss.place(w, h,
                    lipgloss.Center, lipgloss.Center,
                    helpPanel,
                    {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
            }

            // Overlay: Confirm Cancel.
            if (s.showConfirmCancel) {
                var confirmPanel = viewConfirmCancelOverlay(s);
                fullView = lipgloss.place(w, h,
                    lipgloss.Center, lipgloss.Center,
                    confirmPanel,
                    {whitespaceChars: '\u2591', whitespaceForeground: COLORS.border});
            }

            return zone.scan(fullView);
        };

        var model = tea.newModel({
            init: _initFn,

            update: _updateFn,

            view: _viewFn
        });

        // Export lifecycle functions for unit testing.
        prSplit._wizardInit = _initFn;
        prSplit._wizardUpdate = _updateFn;
        prSplit._wizardView = _viewFn;

        return model;
    }

    // -----------------------------------------------------------------------
    //  Update Handlers — screen-specific input handling
    // -----------------------------------------------------------------------

    function updateConfirmCancel(msg, s) {
        if (msg.type === 'Key') {
            var k = msg.key;
            if (k === 'y' || k === 'enter') {
                s.showConfirmCancel = false;
                s.wizard.cancel();
                s.wizardState = 'CANCELLED';
                return [s, tea.quit()];
            }
            if (k === 'n' || k === 'esc') {
                s.showConfirmCancel = false;
                return [s, null];
            }
        }
        if (msg.type === 'Mouse' && msg.action === 'press') {
            if (zone.inBounds('confirm-yes', msg)) {
                s.showConfirmCancel = false;
                s.wizard.cancel();
                s.wizardState = 'CANCELLED';
                return [s, tea.quit()];
            }
            if (zone.inBounds('confirm-no', msg)) {
                s.showConfirmCancel = false;
                return [s, null];
            }
        }
        return [s, null];
    }

    function handleMouseClick(msg, s) {
        // Navigation bar clicks.
        if (zone.inBounds('nav-back', msg)) {
            return handleBack(s);
        }
        if (zone.inBounds('nav-cancel', msg)) {
            s.showConfirmCancel = true;
            return [s, null];
        }
        if (zone.inBounds('nav-next', msg)) {
            return handleNext(s);
        }
        // Claude status badge.
        if (zone.inBounds('claude-status', msg)) {
            if (typeof tuiMux !== 'undefined' && tuiMux &&
                typeof tuiMux.switchTo === 'function') {
                tuiMux.switchTo('claude');
            }
            return [s, null];
        }
        // Screen-specific zone clicks.
        return handleScreenMouseClick(msg, s);
    }

    function handleScreenMouseClick(msg, s) {
        // Config screen: strategy selection, advanced toggle.
        if (s.wizardState === 'CONFIG' || s.wizardState === 'IDLE') {
            var strategies = ['auto', 'heuristic', 'directory'];
            for (var si = 0; si < strategies.length; si++) {
                if (zone.inBounds('strategy-' + strategies[si], msg)) {
                    prSplit.runtime.mode = strategies[si];
                    return [s, null];
                }
            }
            if (zone.inBounds('toggle-advanced', msg)) {
                s.showAdvanced = !s.showAdvanced;
                return [s, null];
            }
        }

        // Plan review: split card selection + edit button.
        if (s.wizardState === 'PLAN_REVIEW') {
            if (st.planCache && st.planCache.splits) {
                for (var i = 0; i < st.planCache.splits.length; i++) {
                    if (zone.inBounds('split-card-' + i, msg)) {
                        s.selectedSplitIdx = i;
                        return [s, null];
                    }
                }
            }
            if (zone.inBounds('plan-edit', msg) && !s.isProcessing) {
                s.wizard.transition('PLAN_EDITOR');
                s.wizardState = 'PLAN_EDITOR';
                return [s, null];
            }
            if (zone.inBounds('plan-regenerate', msg) && !s.isProcessing) {
                handlePlanReviewState(s.wizard, 'regenerate');
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            }
        }

        // Plan editor: split selection, file selection, action buttons.
        if (s.wizardState === 'PLAN_EDITOR') {
            if (st.planCache && st.planCache.splits) {
                for (var i = 0; i < st.planCache.splits.length; i++) {
                    if (zone.inBounds('edit-split-' + i, msg)) {
                        s.selectedSplitIdx = i;
                        return [s, null];
                    }
                    // File selection within currently selected split.
                    if (i === (s.selectedSplitIdx || 0) && st.planCache.splits[i].files) {
                        for (var fi = 0; fi < st.planCache.splits[i].files.length; fi++) {
                            if (zone.inBounds('edit-file-' + i + '-' + fi, msg)) {
                                s.selectedFileIdx = fi;
                                return [s, null];
                            }
                        }
                    }
                }
            }
            // Editor action buttons.
            if (zone.inBounds('editor-move', msg)) {
                // TODO: Implement move-file dialog (future enhancement).
                log.printf('editor-move: not yet implemented');
                return [s, null];
            }
            if (zone.inBounds('editor-rename', msg)) {
                // TODO: Implement rename-split dialog (future enhancement).
                log.printf('editor-rename: not yet implemented');
                return [s, null];
            }
            if (zone.inBounds('editor-merge', msg)) {
                // TODO: Implement merge-splits dialog (future enhancement).
                log.printf('editor-merge: not yet implemented');
                return [s, null];
            }
        }

        // Error resolution: resolution choice buttons.
        if (s.wizardState === 'ERROR_RESOLUTION') {
            var resolveChoices = ['auto', 'manual', 'skip', 'retry', 'abort'];
            for (var ri = 0; ri < resolveChoices.length; ri++) {
                if (zone.inBounds('resolve-' + resolveChoices[ri], msg)) {
                    return handleErrorResolutionChoice(s, resolveChoices[ri] === 'auto' ? 'auto-resolve' : resolveChoices[ri]);
                }
            }
        }

        // Finalization: action buttons.
        if (s.wizardState === 'FINALIZATION') {
            if (zone.inBounds('final-report', msg)) {
                output.print(JSON.stringify(buildReport(), null, 2));
                return [s, null];
            }
            if (zone.inBounds('final-create-prs', msg)) {
                handleFinalizationState(s.wizard, 'create-prs');
                return [s, null];
            }
            if (zone.inBounds('final-done', msg)) {
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            }
        }

        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Navigation Handlers — Back / Next / Up / Down
    // -----------------------------------------------------------------------

    function handleBack(s) {
        switch (s.wizardState) {
            case 'PLAN_REVIEW':
                s.wizardState = 'CONFIG';
                s.wizard.reset();
                s.wizard.transition('CONFIG');
                return [s, null];
            case 'PLAN_EDITOR':
                s.wizardState = 'PLAN_REVIEW';
                handlePlanEditorState(s.wizard, 'back', st.planCache);
                return [s, null];
            default:
                return [s, null];
        }
    }

    function handleNext(s) {
        if (s.isProcessing) return [s, null];

        switch (s.wizardState) {
            case 'IDLE':
            case 'CONFIG':
                return startAnalysis(s);
            case 'PLAN_REVIEW':
                return startExecution(s);
            case 'PLAN_EDITOR':
                // Save edits and return to review.
                handlePlanEditorState(s.wizard, 'save', st.planCache);
                s.wizardState = 'PLAN_REVIEW';
                return [s, null];
            case 'ERROR_RESOLUTION':
                return handleErrorResolutionChoice(s, 'auto-resolve');
            case 'FINALIZATION':
                handleFinalizationState(s.wizard, 'done');
                s.wizardState = 'DONE';
                return [s, tea.quit()];
            default:
                return [s, null];
        }
    }

    function handleNavDown(s) {
        if (s.wizardState === 'PLAN_REVIEW' || s.wizardState === 'PLAN_EDITOR') {
            if (st.planCache && st.planCache.splits) {
                s.selectedSplitIdx = Math.min(
                    (s.selectedSplitIdx || 0) + 1,
                    st.planCache.splits.length - 1
                );
            }
        }
        return [s, null];
    }

    function handleNavUp(s) {
        if (s.wizardState === 'PLAN_REVIEW' || s.wizardState === 'PLAN_EDITOR') {
            s.selectedSplitIdx = Math.max((s.selectedSplitIdx || 0) - 1, 0);
        }
        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Async Pipeline Handlers — drive wizard state machine
    // -----------------------------------------------------------------------

    function startAnalysis(s) {
        s.isProcessing = true;
        s.analysisProgress = 0;
        s.analysisSteps = [
            { label: 'Analyze diff', active: true, done: false },
            { label: 'Group files', active: false, done: false },
            { label: 'Generate plan', active: false, done: false },
            { label: 'Validate plan', active: false, done: false }
        ];

        // Run config state handler.
        var configResult = handleConfigState({
            baseBranch: prSplit.runtime.baseBranch,
            dir: prSplit.runtime.dir,
            strategy: prSplit.runtime.strategy,
            verifyCommand: prSplit.runtime.verifyCommand
        });

        if (configResult.error) {
            s.isProcessing = false;
            s.errorDetails = configResult.error;
            s.wizardState = 'ERROR';
            return [s, null];
        }

        // Transition to CONFIG if needed.
        if (s.wizard.current === 'IDLE') {
            s.wizard.transition('CONFIG');
        }

        // Step 1: Analyze diff.
        s.analysisSteps[0].active = true;
        var analysisStart = Date.now();
        try {
            st.analysisCache = prSplit.analyzeDiff({ baseBranch: prSplit.runtime.baseBranch });
        } catch (e) {
            s.isProcessing = false;
            s.errorDetails = 'Analysis failed: ' + (e.message || String(e));
            s.wizardState = 'ERROR';
            return [s, null];
        }
        s.analysisSteps[0].done = true;
        s.analysisSteps[0].active = false;
        s.analysisSteps[0].elapsed = Date.now() - analysisStart;
        s.analysisProgress = 0.25;

        if (st.analysisCache.error) {
            s.isProcessing = false;
            s.errorDetails = st.analysisCache.error;
            s.wizardState = 'ERROR';
            return [s, null];
        }
        if (!st.analysisCache.files || st.analysisCache.files.length === 0) {
            s.isProcessing = false;
            s.errorDetails = 'No changes found between branches.';
            s.wizardState = 'CONFIG';
            return [s, null];
        }

        // Step 2: Group files.
        s.analysisSteps[1].active = true;
        var groupStart = Date.now();
        st.groupsCache = prSplit.applyStrategy(
            st.analysisCache.files, prSplit.runtime.strategy);
        s.analysisSteps[1].done = true;
        s.analysisSteps[1].active = false;
        s.analysisSteps[1].elapsed = Date.now() - groupStart;
        s.analysisProgress = 0.5;

        // Step 3: Create plan.
        s.analysisSteps[2].active = true;
        var planStart = Date.now();
        st.planCache = prSplit.createSplitPlan(st.groupsCache, {
            baseBranch: prSplit.runtime.baseBranch,
            sourceBranch: st.analysisCache.currentBranch,
            branchPrefix: prSplit.runtime.branchPrefix,
            verifyCommand: prSplit.runtime.verifyCommand,
            fileStatuses: st.analysisCache.fileStatuses
        });
        s.analysisSteps[2].done = true;
        s.analysisSteps[2].active = false;
        s.analysisSteps[2].elapsed = Date.now() - planStart;
        s.analysisProgress = 0.75;

        // Step 4: Validate.
        s.analysisSteps[3].active = true;
        var validation = prSplit.validatePlan(st.planCache);
        s.analysisSteps[3].done = true;
        s.analysisSteps[3].active = false;
        s.analysisProgress = 1.0;

        if (!validation.valid) {
            s.isProcessing = false;
            s.errorDetails = 'Plan validation failed: ' + validation.errors.join('; ');
            s.wizardState = 'ERROR';
            return [s, null];
        }

        s.isProcessing = false;

        // Transition wizard to PLAN_GENERATION then PLAN_REVIEW.
        if (s.wizard.current === 'CONFIG') {
            s.wizard.transition('PLAN_GENERATION');
        }
        s.wizard.transition('PLAN_REVIEW');
        s.wizardState = 'PLAN_REVIEW';
        return [s, null];
    }

    function startExecution(s) {
        if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
            s.errorDetails = 'No plan to execute.';
            return [s, null];
        }

        s.isProcessing = true;
        s.executionResults = [];
        s.executingIdx = 0;

        // Transition: PLAN_REVIEW → BRANCH_BUILDING.
        if (s.wizard.current === 'PLAN_REVIEW') {
            handlePlanReviewState(s.wizard, 'approve');
        }
        s.wizardState = 'BRANCH_BUILDING';

        // Dry run check.
        if (prSplit.runtime.dryRun) {
            s.isProcessing = false;
            s.wizardState = 'FINALIZATION';
            s.wizard.transition('FINALIZATION');
            return [s, null];
        }

        // Execute.
        try {
            var result = prSplit.executeSplit(st.planCache);
            if (result.error) {
                s.isProcessing = false;
                s.errorDetails = result.error;
                s.wizard.data.failedBranches = result.results ?
                    result.results.filter(function(r) { return r.error; }) : [];
                s.wizard.transition('ERROR_RESOLUTION');
                s.wizardState = 'ERROR_RESOLUTION';
                return [s, null];
            }
            st.executionResultCache = result.results;
            s.executionResults = result.results || [];
        } catch (e) {
            s.isProcessing = false;
            s.errorDetails = 'Execution error: ' + (e.message || String(e));
            s.wizard.transition('ERROR_RESOLUTION');
            s.wizardState = 'ERROR_RESOLUTION';
            return [s, null];
        }

        s.isProcessing = false;

        // Move to equivalence check.
        s.wizard.transition('EQUIV_CHECK');
        s.wizardState = 'EQUIV_CHECK';

        // Run equivalence check.
        s.isProcessing = true;
        var equivResult = handleEquivCheckState(s.wizard, st.planCache);
        s.isProcessing = false;
        s.equivalenceResult = equivResult.equivalence || {};
        s.wizardState = s.wizard.current;

        return [s, null];
    }

    function handleErrorResolutionChoice(s, choice) {
        var result = handleErrorResolutionState(s.wizard, choice);
        s.wizardState = s.wizard.current;

        if (choice === 'manual' && typeof tuiMux !== 'undefined' && tuiMux &&
            typeof tuiMux.switchTo === 'function') {
            tuiMux.switchTo('claude');
        }

        return [s, null];
    }

    // -----------------------------------------------------------------------
    //  Program Launch (T025 + T027)
    // -----------------------------------------------------------------------

    var _wizardModel = createWizardModel();

    // Export for Go-side launching and test access.
    prSplit._wizardModel = _wizardModel;
    prSplit._createWizardModel = createWizardModel;
    prSplit._buildReport = buildReport;

    // startWizard — called by pr_split.go to launch the BubbleTea wizard.
    // Blocks the calling goroutine until the user exits the wizard.
    prSplit.startWizard = function() {
        return tea.run(_wizardModel, {altScreen: true, mouse: true});
    };

    // -----------------------------------------------------------------------
    //  Bell Event Handling
    // -----------------------------------------------------------------------

    if (typeof tuiMux !== 'undefined' && tuiMux && typeof tuiMux.on === 'function') {
        var _bellCount = 0;
        tuiMux.on('bell', function(data) {
            _bellCount++;
            log.printf('bell: received bell #%d from pane=%s', _bellCount, data && data.pane || 'unknown');
        });
        prSplit._bellCount = function() { return _bellCount; };
    }

    // -----------------------------------------------------------------------
    //  Mode Registration — commands remain for programmatic/test dispatch.
    //  The BubbleTea wizard above is launched by pr_split.go for interactive
    //  use. This registration exposes all commands so existing tests and
    //  the scripting API continue to work.
    // -----------------------------------------------------------------------

    ctx.run('register-mode', function() {
        var runtime = prSplit.runtime;
        tui.registerMode({
            name: prSplit._MODE_NAME,
            tui: {
                title: 'PR Split',
                prompt: '(pr-split) > '
            },
            onEnter: function() {
                output.print('PR Split Wizard active. Type "help" for commands.');
                output.print('');
                output.print('Config: base=' + runtime.baseBranch + ' strategy=' + runtime.strategy +
                    ' max=' + runtime.maxFiles + (runtime.dryRun ? ' [DRY RUN]' : ''));
            },
            commands: function() {
                return buildCommands(tuiState);
            }
        });
    });

    ctx.run('enter-pr-split', function() {
        tui.switchMode(prSplit._MODE_NAME);
    });

})(globalThis.prSplit);

// ---------------------------------------------------------------------------
//  CommonJS exports for require() compatibility (test environments).
// ---------------------------------------------------------------------------
if (typeof module !== 'undefined' && module.exports) {
    module.exports = globalThis.prSplit;
}
