'use strict';
// pr_split_14b_tui_commands_ext.js — TUI: extended commands, HUD overlay, bell handling
// Dependencies: chunks 00-13 + pr_split_14a_tui_commands_core.js must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    var st = prSplit._state;
    var tuiState = prSplit._tuiState;
    var buildReport = prSplit._buildReport;
    var WizardState = prSplit.WizardState;
    var handleConfigState = prSplit._handleConfigState;
    var handleBaselineFailState = prSplit._handleBaselineFailState;

    // --- buildExtCommands ---
    function buildExtCommands(tuiStateArg) {
        var style = prSplit._style;
        var padIndex = prSplit._padIndex;
        var sanitizeBranchName = prSplit._sanitizeBranchName;
        var runtime = prSplit.runtime;

        return {
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
                            if (query === 'forceCancelled') return wizard.current === 'FORCE_CANCEL';  // T117: was 'CANCELLED' — bug
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
                        } else {
                            // T090: Baseline verification deferred to async.
                            var bvc = configResult.baselineVerifyConfig;
                            var skipBaseline = !bvc || !bvc.verifyCommand || bvc.verifyCommand === 'true';
                            if (!skipBaseline) {
                                var verifyResult = await prSplit.verifySplitAsync(runtime.baseBranch, bvc);
                                if (!verifyResult.passed) {
                                    wizard.transition('BASELINE_FAIL', {
                                        error: verifyResult.error,
                                        output: verifyResult.output
                                    });

                                    // --- BASELINE_FAIL menu ---
                                    output.print('');
                                    output.print(style.error('\u274c Baseline verification failed'));
                                    output.print('  Error: ' + verifyResult.error);
                                    if (verifyResult.output) {
                                        output.print('  Output: ' + style.dim(verifyResult.output));
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
                                }
                            }
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
                        if (query === 'forceCancelled') return wizard.current === 'FORCE_CANCEL';  // T117: was 'CANCELLED' — bug
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
                description: 'Edit plan (TUI mode only \u2014 use move/rename/merge in REPL)',
                usage: 'edit-plan',
                handler: function() {
                    if (!st.planCache || !st.planCache.splits || st.planCache.splits.length === 0) {
                        output.print('No plan to edit. Run "plan" first.'); return;
                    }
                    output.print('[edit-plan] Interactive plan editing requires TUI mode (default). In REPL mode, use text commands: move <from> <to>, rename <index> <name>, merge <i> <j>.');
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

    function buildCommands(tuiStateArg) {
        var core = prSplit._buildCoreCommands(tuiStateArg);
        var ext = buildExtCommands(tuiStateArg);
        var merged = {};
        var k;
        for (k in core) { if (Object.prototype.hasOwnProperty.call(core, k)) merged[k] = core[k]; }
        for (k in ext) { if (Object.prototype.hasOwnProperty.call(ext, k)) merged[k] = ext[k]; }
        return merged;
    }

    prSplit._buildCommands = buildCommands;

    // --- HUD Overlay — Process state display when Claude pane is backgrounded ---
    //
    //  Shows: (1) activity indicator (live/idle), (2) last N lines of Claude
    //  output from VTerm screenshot, (3) current wizard state.
    //  Rendered with lipgloss for clean terminal styling.
    //  Toggled via the "hud" command. When enabled, the status bar shows a
    //  compact summary and "hud" prints the full panel.

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
        // T091: _ws.current is a string property (WizardState.current),
        // not a function. Read it directly.
        if (typeof _ws !== 'undefined' && _ws && typeof _ws.current === 'string') {
            wizState = _ws.current;
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
        // T091: _ws.current is a string property (WizardState.current),
        // not a function. Read it directly.
        if (typeof _ws !== 'undefined' && _ws && typeof _ws.current === 'string') {
            wizState = _ws.current;
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

    // --- Bell Event Handling ---
    //
    //  When the Claude pane emits a BEL character (e.g., command completion,
    //  error, or explicit notification), flash the status bar and log the
    //  event. This provides feedback when the user is viewing the OSM TUI
    //  and the Claude process needs attention.

    if (typeof tuiMux !== 'undefined' && tuiMux && typeof tuiMux.on === 'function') {
        var _bellCount = 0;
        var _bellFlashTimer = null;

        tuiMux.on('bell', function(data) {
            _bellCount++;
            log.printf('bell: received bell #%d from pane=%s', _bellCount, data && data.pane || 'unknown');

            // Flash the status bar to draw the user's attention.
            if (typeof tuiMux.setStatus === 'function') {
                // T091: Capture the current HUD status dynamically instead of
                // relying on tuiState._bellPrevStatus (which was never set).
                // _renderHudStatusLine() produces the correct contextual status.
                var prevStatus = (_hudEnabled && typeof _renderHudStatusLine === 'function')
                    ? _renderHudStatusLine()
                    : 'idle';
                tuiMux.setStatus('\u0007 BELL — Claude needs attention');

                // Restore status after 3 seconds (debounced).
                if (_bellFlashTimer) {
                    try { clearTimeout(_bellFlashTimer); } catch(e) { log.debug('bell: clearTimeout failed: ' + (e.message || e)); }
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

})(globalThis.prSplit);
