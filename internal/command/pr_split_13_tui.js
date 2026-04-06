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

    // --- TUI Guard ---

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') {
        return;
    }

    var shared = prSplit._modules.shared;
    if (!shared) return;

    // --- Shared State ---
    // Uses prSplit._state for cross-chunk cache coherence. The pipeline
    // (chunk 10) writes state.analysisCache, state.planCache, etc. during
    // automatedSplit, and TUI commands below read/write the same properties
    // so the REPL reflects pipeline results.

    var st = prSplit._state;

    var tuiState = tui.createState(prSplit._COMMAND_NAME, {
        [shared.contextItems]: {defaultValue: []}
    });

    // --- WizardState ---
    // Guarded state machine for the pr-split wizard.
    // States: IDLE, CONFIG, BASELINE_FAIL, PLAN_GENERATION, PLAN_REVIEW,
    //   PLAN_EDITOR, BRANCH_BUILDING, ERROR_RESOLUTION, EQUIV_CHECK,
    //   FINALIZATION, DONE, CANCELLED, FORCE_CANCEL, PAUSED, ERROR
    // See scratch/w01-state-machine.md for full design.

    // Valid transitions: { fromState: { toState: true, ... }, ... }
    var VALID_TRANSITIONS = {
        'IDLE':             { 'CONFIG': true },
        'CONFIG':           { 'PLAN_GENERATION': true, 'BASELINE_FAIL': true, 'BRANCH_BUILDING': true, 'ERROR_RESOLUTION': true, 'CANCELLED': true, 'ERROR': true },
        'BASELINE_FAIL':    { 'PLAN_GENERATION': true, 'CANCELLED': true },
        'PLAN_GENERATION':  { 'PLAN_REVIEW': true, 'ERROR_RESOLUTION': true, 'CANCELLED': true, 'FORCE_CANCEL': true, 'PAUSED': true, 'ERROR': true },
        'PLAN_REVIEW':      { 'PLAN_EDITOR': true, 'BRANCH_BUILDING': true, 'PLAN_GENERATION': true, 'CANCELLED': true },
        'PLAN_EDITOR':      { 'PLAN_REVIEW': true },
        'BRANCH_BUILDING':  { 'EQUIV_CHECK': true, 'ERROR_RESOLUTION': true, 'CANCELLED': true, 'FORCE_CANCEL': true, 'PAUSED': true, 'ERROR': true },
        'ERROR_RESOLUTION': { 'EQUIV_CHECK': true, 'BRANCH_BUILDING': true, 'PLAN_GENERATION': true, 'CANCELLED': true, 'FORCE_CANCEL': true },
        'EQUIV_CHECK':      { 'FINALIZATION': true, 'PLAN_REVIEW': true, 'ERROR_RESOLUTION': true, 'CANCELLED': true, 'ERROR': true },
        'FINALIZATION':     { 'FINALIZATION': true, 'DONE': true },
        'DONE':             { 'IDLE': true },
        'CANCELLED':        { 'DONE': true },
        'FORCE_CANCEL':     { 'DONE': true },
        'PAUSED':           { 'DONE': true, 'PLAN_GENERATION': true, 'BRANCH_BUILDING': true, 'CANCELLED': true },  // T084: resume paths
        // ERROR is recoverable via Back navigation, so the state machine
        // permits returning to the last active wizard state.
        'ERROR':            { 'DONE': true, 'CONFIG': true, 'BASELINE_FAIL': true, 'PLAN_GENERATION': true, 'PLAN_REVIEW': true, 'PLAN_EDITOR': true, 'BRANCH_BUILDING': true, 'ERROR_RESOLUTION': true, 'EQUIV_CHECK': true, 'FINALIZATION': true }
    };

    // Terminal states — no pipeline activity, waiting for reset or dismiss.
    var TERMINAL_STATES = { 'DONE': true, 'CANCELLED': true, 'FORCE_CANCEL': true, 'PAUSED': true, 'ERROR': true };

    // States from which pause is allowed.
    var PAUSABLE_STATES = { 'PLAN_GENERATION': true, 'BRANCH_BUILDING': true };

    // --- Verification Phase State Machine ---
    // Tracks the per-branch verification + equivalence lifecycle
    // independently from the wizard's high-level state machine.
    // The wizard tracks which screen the user sees; verifyPhase tracks
    // what the verification subsystem is doing within that screen.

    // Verification lifecycle phases.
    var verifyPhases = {
        NOT_STARTED:   'not-started',      // No verification has begun
        RUNNING:       'running',          // Branches being verified one-by-one
        PAUSED:        'paused',           // Verify subprocess paused (SIGSTOP)
        EQUIV_CHECK:   'equiv-check',      // Equivalence check in progress
        COMPLETE:      'complete',         // All verification and equiv passed
        FAILED:        'failed',           // Some verification or equiv failed
        ERROR:         'error'             // Unrecoverable error
    };

    // Per-branch verification statuses.
    var branchStatuses = {
        PENDING:  'pending',    // Not yet verified
        ACTIVE:   'active',     // Currently being verified
        PASSED:   'passed',     // Verification passed
        FAILED:   'failed',     // Verification failed
        SKIPPED:  'skipped'     // Skipped (dependency failed)
    };

    // Valid transitions for verification phases.
    var _VERIFY_TRANSITIONS = {};
    _VERIFY_TRANSITIONS[verifyPhases.NOT_STARTED] = [verifyPhases.RUNNING, verifyPhases.EQUIV_CHECK, verifyPhases.ERROR];
    _VERIFY_TRANSITIONS[verifyPhases.RUNNING]     = [verifyPhases.PAUSED, verifyPhases.EQUIV_CHECK, verifyPhases.FAILED, verifyPhases.ERROR];
    _VERIFY_TRANSITIONS[verifyPhases.PAUSED]      = [verifyPhases.RUNNING, verifyPhases.ERROR];
    _VERIFY_TRANSITIONS[verifyPhases.EQUIV_CHECK]  = [verifyPhases.COMPLETE, verifyPhases.FAILED, verifyPhases.ERROR];
    _VERIFY_TRANSITIONS[verifyPhases.COMPLETE]    = [];  // Terminal
    _VERIFY_TRANSITIONS[verifyPhases.FAILED]      = [verifyPhases.RUNNING, verifyPhases.NOT_STARTED, verifyPhases.ERROR];  // Retry / error escalation
    _VERIFY_TRANSITIONS[verifyPhases.ERROR]       = [verifyPhases.RUNNING, verifyPhases.NOT_STARTED];  // Recovery

    // transitionVerifyPhase validates and applies a verification phase
    // transition. Returns true on success, false if the transition is
    // invalid (with a log warning). Callers should check the return value
    // but may proceed — invalid transitions are informational, not fatal.
    function transitionVerifyPhase(s, toPhase) {
        var from = s.verifyPhase || verifyPhases.NOT_STARTED;
        var allowed = _VERIFY_TRANSITIONS[from];
        if (!allowed || allowed.indexOf(toPhase) < 0) {
            log.printf('verify: invalid phase transition %s \u2192 %s (allowed: %s)',
                from, toPhase, allowed ? allowed.join(', ') : 'none');
            return false;
        }
        log.debug('verify: phase %s \u2192 %s', from, toPhase);
        s.verifyPhase = toPhase;
        return true;
    }

    // resetVerifyPhase unconditionally resets to NOT_STARTED (used by
    // retry and wizard reset paths that bypass normal transitions).
    function resetVerifyPhase(s) {
        s.verifyPhase = verifyPhases.NOT_STARTED;
    }

    // WizardState is the pr-split wizard state machine.
    function WizardState() {
        this.current = 'IDLE';
        this.data = {};
        this.history = [];
        this.checkpoint = null;
        this.listeners = [];
    }

    // transition moves to a new state with guard check.
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

    // cancel transitions to CANCELLED from any non-terminal active state.
    WizardState.prototype.cancel = function() {
        if (this.current === 'PAUSED') {
            delete this.data.pausedFrom;  // T084: clean up before cancel
            this.transition('CANCELLED');
            return;
        }
        if (TERMINAL_STATES[this.current]) return;
        this.transition('CANCELLED');
    };

    // forceCancel transitions to FORCE_CANCEL from any state except DONE and FORCE_CANCEL.
    WizardState.prototype.forceCancel = function() {
        if (this.current === 'DONE' || this.current === 'FORCE_CANCEL') return;
        // Force cancel bypasses normal transition matrix — always allowed
        // from active states and even from CANCELLED.
        delete this.data.pausedFrom;  // T084: clean up if force-cancelling from PAUSED
        this.history.push({ from: this.current, to: 'FORCE_CANCEL', at: Date.now() });
        this.current = 'FORCE_CANCEL';
        for (var j = 0; j < this.listeners.length; j++) {
            try { this.listeners[j](this.current, 'FORCE_CANCEL', this.data); } catch (e) { log.debug('wizard: listener[' + j + '] failed: ' + (e.message || e)); }
        }
    };

    // pause transitions to PAUSED if current state supports pausing.
    WizardState.prototype.pause = function() {
        if (!PAUSABLE_STATES[this.current]) return;
        this.data.pausedFrom = this.current;  // T084: remember origin for resume
        this.transition('PAUSED');
    };

    // resume transitions from PAUSED back to the original state.
    WizardState.prototype.resume = function() {  // T084
        if (this.current !== 'PAUSED') return false;
        var target = this.data.pausedFrom;
        if (!target || !PAUSABLE_STATES[target]) return false;
        this.transition(target);
        delete this.data.pausedFrom;  // clean up stale resume context
        return true;
    };

    // error transitions to ERROR from any non-terminal state.
    // PAUSED is terminal so error() is a no-op from PAUSED (intentional).
    WizardState.prototype.error = function(message) {
        if (TERMINAL_STATES[this.current]) return;
        this.transition('ERROR', message ? { error: message } : undefined);
    };

    // isTerminal returns true if no further pipeline activity is expected.
    WizardState.prototype.isTerminal = function() {
        return !!TERMINAL_STATES[this.current];
    };

    // onTransition registers a listener for state changes.
    WizardState.prototype.onTransition = function(fn) {
        if (typeof fn === 'function') this.listeners.push(fn);
    };

    // reset returns to IDLE, clearing data and history.
    WizardState.prototype.reset = function() {
        this.current = 'IDLE';
        this.data = {};
        this.history = [];
        this.checkpoint = null;
        this.listeners = [];
    };

    // saveCheckpoint captures current state for resume. T085: Also persists
    // plan caches to disk via savePlan() for future resume via loadPlan().
    WizardState.prototype.saveCheckpoint = function() {
        this.checkpoint = {
            state: this.current,
            data: JSON.parse(JSON.stringify(this.data)),
            at: Date.now()
        };
        // T085: Persist plan + runtime caches to disk so resume can restore
        // them via loadPlan().  savePlan() is defined in chunk 03; guard
        // against it not being available (e.g. minimal test environments).
        if (typeof prSplit.savePlan === 'function') {
            try {
                prSplit.savePlan(null, 'checkpoint:' + this.current);
            } catch (e) {
                // Best-effort — checkpoint still succeeds in-memory.
                if (typeof log !== 'undefined' && log.warn) {
                    log.warn('saveCheckpoint: savePlan failed: ' + (e.message || e));
                }
            }
        }
        return this.checkpoint;
    };

    // Export on prSplit for cross-chunk access.
    prSplit.WizardState = WizardState;
    prSplit.WIZARD_VALID_TRANSITIONS = VALID_TRANSITIONS;
    prSplit.WIZARD_TERMINAL_STATES = TERMINAL_STATES;

    // --- buildReport — JSON-serializable status report ---

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
            // T089: Use cached equivalence result from TUI pipeline if available,
            // avoiding a synchronous verifyEquivalence() call that blocks the
            // event loop on final-branch tree comparison.
            var equiv = st.equivalenceResult || prSplit.verifyEquivalence(st.planCache);
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

    function getClaudePaneSession() {
        if (typeof tuiMux === 'undefined' || !tuiMux) return null;
        if (typeof tuiMux.session === 'function') {
            try { return tuiMux.session(); } catch (e) { log.debug('getClaudePaneSession: tuiMux.session failed: ' + (e.message || e)); }
        }
        if (typeof tuiMux.writeToChild !== 'function' &&
            typeof tuiMux.childScreen !== 'function' &&
            typeof tuiMux.screenshot !== 'function') {
            return null;
        }
        return {
            write: function(data) {
                if (typeof tuiMux.writeToChild !== 'function') throw new Error('Claude session is not writable');
                return tuiMux.writeToChild(data);
            },
            screen: function() {
                if (typeof tuiMux.childScreen === 'function') return tuiMux.childScreen();
                return '';
            },
            output: function() {
                if (typeof tuiMux.screenshot === 'function') return tuiMux.screenshot();
                return '';
            },
            resize: function() { return null; },
            target: function() { return { name: 'claude', kind: 'pty' }; }
        };
    }

    function getInteractivePaneSession(s, tab) {
        var pane = tab || s.splitViewTab || 'claude';
        if (pane === 'claude') return getClaudePaneSession();
        if (pane === 'verify') return s.activeVerifySession || null;
        return null;
    }

    // Task 8: Shell tab removed — verify pane IS the interactive shell.
    // No separate shell pane state to reset.

    function closeInteractivePaneSession(s, tab, debugPrefix) {
        var session = getInteractivePaneSession(s, tab);
        if (!session || typeof session.close !== 'function') return false;
        try {
            session.close();
            return true;
        } catch (e) {
            log.debug((debugPrefix || 'interactivePane') + ': ' + tab + 'Session.close failed: ' + (e.message || e));
            return false;
        }
    }

    // Task 8: Shell tab removed — cleanupShellPaneSession no longer needed.
    // All interactive session cleanup goes through clearVerifyPaneSession.

    function clearVerifyPaneSession(s, opts) {
        var options = opts || {};
        var keepDisplay = options.keepDisplay === true;
        var cleanupWorktree = options.cleanupWorktree !== false;

        closeInteractivePaneSession(s, 'verify', options.debugPrefix || 'verifyCleanup');

        if (cleanupWorktree && s.activeVerifyWorktree && s.activeVerifyDir) {
            try {
                prSplit.cleanupVerifyWorktree(s.activeVerifyDir, s.activeVerifyWorktree);
            } catch (e) {
                log.debug((options.debugPrefix || 'verifyCleanup') + ': verifyWorktree cleanup failed: ' + (e.message || e));
            }
        }

        s.activeVerifySession = null;
        s.activeVerifyWorktree = null;
        s.activeVerifyDir = null;
        s.activeVerifyStartTime = 0;
        s.verifyViewportOffset = 0;
        s.verifyAutoScroll = true;
        s.lastVerifyInterruptTime = 0;
        s.verifyPaused = false;

        if (!keepDisplay) {
            s.activeVerifyBranch = null;
            s.verifyElapsedMs = 0;
            s.verifyScreen = '';
        }
        // T007 (Task 7): Also preserve display state for the persistent shell
        // user-signal fields (so p/f/c footer stays visible until next branch).
        if (keepDisplay) {
            // Keep verifySignal state for the footer to remain active
            // (it will be cleared by pollVerifySession after recording result).
        } else {
            s.verifyShellExited = false;
            s.verifyHint = '';
        }
    }

    function hasInteractivePaneSession(s, tab) {
        return !!getInteractivePaneSession(s, tab);
    }

    // Task 8: Shell tab removed — only claude, output, verify tabs remain.
    function listSplitViewTabs(s) {
        var tabs = ['claude', 'output'];
        if (hasInteractivePaneSession(s, 'verify') || s.verifyFallbackRunning || s.verifyScreen) tabs.push('verify');
        return tabs;
    }

    prSplit._getClaudePaneSession = getClaudePaneSession;
    prSplit._getInteractivePaneSession = getInteractivePaneSession;
    prSplit._closeInteractivePaneSession = closeInteractivePaneSession;
    prSplit._clearVerifyPaneSession = clearVerifyPaneSession;
    // Task 8: _cleanupShellPaneSession and _openVerifyWorktreeShell removed — shell tab is unified into verify pane.
    prSplit._hasInteractivePaneSession = hasInteractivePaneSession;
    prSplit._listSplitViewTabs = listSplitViewTabs;

    // --- Wizard state handlers ---

    // handleConfigState validates configuration and prepares baseline verify
    // config. T090: Actual baseline verification is deferred to the async
    // pipeline so the TUI event loop is never blocked.
    function handleConfigState(config) {
        var runtime = prSplit.runtime;
        var gitExec = prSplit._gitExec;
        var resolveDir = prSplit._resolveDir;
        var automatedDefaults = prSplit.AUTOMATED_DEFAULTS || {};
        var loadPlan = prSplit.loadPlan;
        var dir = resolveDir(config.dir || '.');

        // --- Step 1: Validate required configuration ---
        var errors = [];
        var availableBranches = null; // T43: populated on branch detection failure

        if (!runtime.baseBranch) {
            errors.push('baseBranch is required (set via config or "set baseBranch <name>")');
        }

        // Detect source branch (current branch).
        var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (branchResult.code !== 0) {
            // T43: Distinguish empty repo from other failures.
            var stderrMsg = (branchResult.stderr || '').trim();
            if (stderrMsg.indexOf('ambiguous argument') !== -1 ||
                stderrMsg.indexOf('bad default revision') !== -1 ||
                stderrMsg.indexOf('unknown revision') !== -1) {
                errors.push('No commits on current branch. Please make at least one commit before splitting.');
            } else {
                errors.push('Cannot determine current branch: ' + stderrMsg);
            }
            // T43: Try to list available branches as fallback.
            var branchListResult = gitExec(dir, ['branch', '--list', '--format=%(refname:short)']);
            if (branchListResult.code === 0 && branchListResult.stdout.trim()) {
                availableBranches = branchListResult.stdout.trim().split('\n');
            }
        } else {
            var sourceBranch = branchResult.stdout.trim();
            // T43: Detect detached HEAD state.
            if (sourceBranch === 'HEAD') {
                errors.push('Detached HEAD state detected. Please checkout a branch before splitting (git checkout <branch>).');
                // T43: List available branches for reference.
                var detachedBranchList = gitExec(dir, ['branch', '--list', '--format=%(refname:short)']);
                if (detachedBranchList.code === 0 && detachedBranchList.stdout.trim()) {
                    availableBranches = detachedBranchList.stdout.trim().split('\n');
                }
            } else if (sourceBranch === runtime.baseBranch) {
                errors.push('Currently on base branch (' + runtime.baseBranch + '); checkout a feature branch first');
            }
        }

        // T43: Validate target (base) branch exists.
        if (runtime.baseBranch) {
            var targetCheck = gitExec(dir, ['rev-parse', '--verify', 'refs/heads/' + runtime.baseBranch]);
            if (targetCheck.code !== 0) {
                // Also check for remote tracking branch.
                var remoteCheck = gitExec(dir, ['rev-parse', '--verify', 'refs/remotes/origin/' + runtime.baseBranch]);
                if (remoteCheck.code !== 0) {
                    errors.push('Target branch "' + runtime.baseBranch + '" does not exist locally or as origin remote');
                }
            }
        }

        if (errors.length > 0) {
            var result = { error: errors.join('; ') };
            if (availableBranches) {
                result.availableBranches = availableBranches;
            }
            return result;
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

        // --- Step 3: Baseline verification config ---
        // T090: Actual verification moved to async pipeline (runAnalysisAsync /
        // automatedSplit pre-step) so the TUI event loop is never blocked.
        // We just resolve the config here and pass it back to the caller.
        var verifyCommand = runtime.verifyCommand;
        var verifyTimeoutMs = 0;
        if (typeof config.verifyTimeoutMs === 'number' && config.verifyTimeoutMs > 0) {
            verifyTimeoutMs = config.verifyTimeoutMs;
        } else if (typeof automatedDefaults.verifyTimeoutMs === 'number' &&
                   automatedDefaults.verifyTimeoutMs > 0) {
            verifyTimeoutMs = automatedDefaults.verifyTimeoutMs;
        }

        return {
            error: null,
            baselineVerifyConfig: {
                verifyCommand: verifyCommand,
                dir: dir,
                verifyTimeoutMs: verifyTimeoutMs
            }
        };
    }

    prSplit._handleConfigState = handleConfigState;

    // handleBaselineFailState handles BASELINE_FAIL: offers override or abort.
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

    // handlePlanReviewState handles PLAN_REVIEW: user approves, edits,
    // regenerates, or cancels the plan.
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

    // handlePlanEditorState handles PLAN_EDITOR: validates edited plan and
    // transitions back to PLAN_REVIEW.
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

    // handleBranchBuildingState executes plan splits and verifies each branch.
    // Transitions to EQUIV_CHECK (all pass) or ERROR_RESOLUTION (any fail).
    function handleBranchBuildingState(wizard, plan, opts) {
        if (wizard.current !== 'BRANCH_BUILDING') {
            return { error: 'wizard is not in BRANCH_BUILDING state (current: ' + wizard.current + ')' };
        }

        opts = opts || {};
        var isCancelled = opts.isCancelled || function() { return false; };
        var isForceCancelled = (typeof prSplit.isForceCancelled === 'function') ? prSplit.isForceCancelled : function() { return false; };  // T117

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
        if (isCancelled() || isForceCancelled()) {  // T117
            wizard.cancel();
            return { action: 'cancelled', state: 'CANCELLED', results: execResult.results };
        }

        // Verify each branch.
        var results = [];
        var failedBranches = [];
        for (var i = 0; i < execResult.results.length; i++) {
            if (isCancelled() || isForceCancelled()) {  // T117
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

    // handleErrorResolutionState decides how to handle failed branches:
    // auto-resolve, skip, retry, manual, or abort.
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
            // NOTE: Do NOT call output.print() here — this runs inside
            // BubbleTea context and would corrupt the terminal. The caller
            // (handleErrorResolutionChoice in chunk 16) handles switching
            // to the Claude pane and storing context for the view.
            var failedBranches = (wizard.data && wizard.data.failedBranches) || [];
            wizard.transition('BRANCH_BUILDING');
            return { action: 'manual', state: 'BRANCH_BUILDING', failedBranches: failedBranches };
        }

        // Default: abort.
        wizard.cancel();
        return { action: 'abort', state: 'CANCELLED' };
    }

    prSplit._handleErrorResolutionState = handleErrorResolutionState;

    // handleEquivCheckState runs equivalence check and transitions to FINALIZATION.
    function handleEquivCheckState(wizard, plan) {
        if (wizard.current !== 'EQUIV_CHECK') {
            return { error: 'wizard is not in EQUIV_CHECK state (current: ' + wizard.current + ')' };
        }

        if (!plan) {
            wizard.error('no plan for equivalence check — run "plan" and "execute" first');
            return { error: 'no plan for equivalence check — run "plan" and "execute" first', action: 'error', state: 'ERROR' };
        }

        // T089: Prefer cached result from TUI pipeline to avoid sync git calls.
        var equivResult = st.equivalenceResult || prSplit.verifyEquivalence(plan);
        st.equivalenceResult = equivResult; // cache for buildReport()

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

    // handleFinalizationState lets user create PRs, view report, or finish.
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

    // Cross-chunk exports — verification state machine.
    prSplit._verifyPhases = verifyPhases;
    prSplit._branchStatuses = branchStatuses;
    prSplit._transitionVerifyPhase = transitionVerifyPhase;
    prSplit._resetVerifyPhase = resetVerifyPhase;

    // Cross-chunk export — tuiState for command and core chunks.
    prSplit._tuiState = tuiState;

})(globalThis.prSplit);
