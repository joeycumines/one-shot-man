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
    //  buildCommands — all TUI REPL commands
    // -----------------------------------------------------------------------

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
                    output.print('Creating PRs for ' + st.planCache.splits.length + ' splits...');
                    var result = prSplit.createPRs(st.planCache, {
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
                usage: 'auto-split',
                handler: async function() {
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
                        var result = await prSplit.automatedSplit(autoConfig);
                        if (result.error) output.print(style.error('Auto-split failed: ' + result.error));
                        if (runtime.jsonOutput && result.report) output.print(JSON.stringify(result.report, null, 2));
                    } catch (e) {
                        output.print('Error: ' + (e && e.message ? e.message : String(e)));
                        if (e && e.stack) log.error('auto-split error: ' + e.stack);
                    }
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
                    if (typeof planEditorFactory !== 'undefined' && planEditorFactory.create) {
                        var editor = planEditorFactory.create(items);
                        output.print('[edit-plan] Opening interactive plan editor...');
                        var updatedItems;
                        try { updatedItems = editor.run(); } catch (e) {
                            output.print('[edit-plan] Editor error: ' + (e && e.message ? e.message : String(e)));
                            return;
                        }
                        if (!updatedItems || updatedItems.length === 0) {
                            output.print('[edit-plan] No changes.'); return;
                        }
                        st.planCache.splits = [];
                        for (var u = 0; u < updatedItems.length; u++) {
                            var item = updatedItems[u];
                            st.planCache.splits.push({
                                name: item.name || item.branchName || ('split-' + (u + 1)),
                                files: item.files || [],
                                message: item.description || item.name || ('Split ' + (u + 1)),
                                order: u
                            });
                        }
                        output.print('[edit-plan] Plan updated: ' + st.planCache.splits.length + ' splits.');
                    } else {
                        output.print('[edit-plan] Plan editor not available. Use text commands: move, rename, merge.');
                    }
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
                    output.print('  help             Show this help');
                }
            }
        };
    }

    prSplit._buildCommands = buildCommands;

    // -----------------------------------------------------------------------
    //  Mode Registration
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
                output.print('PR Split \u2014 split large PRs into reviewable stacked branches.');
                output.print('Type "help" for commands. Quick start: "run"');
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
