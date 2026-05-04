'use strict';
// pr_split_14a_tui_commands_core.js — TUI: core workflow commands (analyze, plan, execute, verify)
// Dependencies: chunks 00-13 must be loaded first.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    var st = prSplit._state;
    var buildReport = prSplit._buildReport;

    // --- buildCoreCommands ---
    function buildCoreCommands(tuiStateArg) {
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
                        // T100: Binary files have null additions/deletions.
                        if (f.binary) {
                            output.print('  ' + f.name + ' (binary)');
                        } else {
                            output.print('  ' + f.name + ' (+' + (f.additions || 0) + '/-' + (f.deletions || 0) + ')');
                        }
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
                        autoMerge: autoMerge, mergeMethod: mergeMethod,
                        dryRun: runtime.dryRun || false  // T077
                    });
                    if (result.dryRun) {
                        output.print('[DRY RUN] No branches pushed, no PRs created.');
                    }
                    if (result.error) { output.print('Error: ' + result.error); return; }
                    for (var i = 0; i < result.results.length; i++) {
                        var r = result.results[i];
                        if (r.dryRun) {
                            output.print('  \u25cb ' + r.name + (r.dryRunMsg ? ': ' + r.dryRunMsg : ''));
                        } else if (r.error) {
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
                        case 'max': { var n = parseInt(value, 10); runtime.maxFiles = isNaN(n) ? 10 : n; break; }
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
        };
    }

    prSplit._buildCoreCommands = buildCoreCommands;

})(globalThis.prSplit);
