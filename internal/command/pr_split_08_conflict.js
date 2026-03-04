'use strict';
// pr_split_08_conflict.js — Auto-fix strategies & local conflict resolution
// Dependencies: chunks 00, 04 must be loaded first
// Late-binds: renderConflictPrompt (09), sendToHandle (10), waitForLogged (10),
//             AUTOMATED_DEFAULTS (10), _claudeExecutor (10), _mcpCallbackObj (10)
//
// All external symbols are read from prSplit.* INSIDE function bodies for
// late-binding. This allows mocking in tests and avoids capturing undefined
// references at load time.

(function(prSplit) {

    // AUTO_FIX_STRATEGIES: sequential repair strategies to try when a split
    // branch fails verification. Each: {name, detect(dir, verifyOutput), fix(dir, ...)}.
    var AUTO_FIX_STRATEGIES = [
        {
            name: 'go-mod-tidy',
            detect: function(dir) {
                var osmod = prSplit._modules.osmod;
                var exec = prSplit._modules.exec;
                var path = (dir !== '.' ? dir + '/' : '') + 'go.mod';
                if (osmod) return osmod.fileExists(path);
                return exec.execv(['test', '-f', path]).code === 0;
            },
            fix: function(dir) {
                var exec = prSplit._modules.exec;
                var gitExec = prSplit._gitExec;
                var shellQuote = prSplit._shellQuote;
                var tidyResult = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && go mod tidy']);
                if (tidyResult.code !== 0) {
                    return { fixed: false, error: 'go mod tidy failed: ' + tidyResult.stderr.trim() };
                }
                var status = gitExec(dir, ['status', '--porcelain', 'go.mod', 'go.sum']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'go mod tidy made no changes' };
                }
                gitExec(dir, ['add', 'go.mod', 'go.sum']);
                var commit = gitExec(dir, ['commit', '-m', 'fix: go mod tidy for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'go-generate-sum',
            detect: function(dir) {
                var osmod = prSplit._modules.osmod;
                var exec = prSplit._modules.exec;
                var path = (dir !== '.' ? dir + '/' : '') + 'go.sum';
                if (osmod) return osmod.fileExists(path);
                return exec.execv(['test', '-f', path]).code === 0;
            },
            fix: function(dir) {
                var exec = prSplit._modules.exec;
                var gitExec = prSplit._gitExec;
                var shellQuote = prSplit._shellQuote;
                var dlResult = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && go mod download']);
                if (dlResult.code !== 0) {
                    return { fixed: false, error: 'go mod download failed: ' + dlResult.stderr.trim() };
                }
                var status = gitExec(dir, ['status', '--porcelain', 'go.sum']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'go mod download made no changes' };
                }
                gitExec(dir, ['add', 'go.sum']);
                var commit = gitExec(dir, ['commit', '-m', 'fix: update go.sum for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'go-build-missing-imports',
            detect: function(dir, verifyOutput) {
                if (!verifyOutput) return false;
                return verifyOutput.indexOf('undefined:') >= 0 ||
                       verifyOutput.indexOf('imported and not used') >= 0 ||
                       verifyOutput.indexOf('could not import') >= 0;
            },
            fix: function(dir) {
                var exec = prSplit._modules.exec;
                var gitExec = prSplit._gitExec;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFiles = prSplit._gitAddChangedFiles;
                var which = exec.execv(['which', 'goimports']);
                if (which.code === 0) {
                    var result = exec.execv(['sh', '-c',
                        'cd ' + shellQuote(dir) + ' && find . -name "*.go" -exec goimports -w {} +']);
                    if (result.code !== 0) {
                        return { fixed: false, error: 'goimports failed: ' + result.stderr.trim() };
                    }
                } else {
                    return { fixed: false, error: 'goimports not available' };
                }
                var status = gitExec(dir, ['status', '--porcelain']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'goimports made no changes' };
                }
                gitAddChangedFiles(dir);
                var commit = gitExec(dir, ['commit', '-m', 'fix: goimports for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'npm-install',
            detect: function(dir) {
                var osmod = prSplit._modules.osmod;
                var exec = prSplit._modules.exec;
                var path = (dir !== '.' ? dir + '/' : '') + 'package.json';
                if (osmod) return osmod.fileExists(path);
                return exec.execv(['test', '-f', path]).code === 0;
            },
            fix: function(dir) {
                var exec = prSplit._modules.exec;
                var gitExec = prSplit._gitExec;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFiles = prSplit._gitAddChangedFiles;
                var result = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && npm install --no-audit --no-fund 2>&1']);
                if (result.code !== 0) {
                    return { fixed: false, error: 'npm install failed: ' + (result.stderr || result.stdout || '').trim() };
                }
                var status = gitExec(dir, ['status', '--porcelain']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'npm install made no changes' };
                }
                gitAddChangedFiles(dir);
                var commit = gitExec(dir, ['commit', '-m', 'fix: npm install for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'make-generate',
            detect: function(dir) {
                var osmod = prSplit._modules.osmod;
                var exec = prSplit._modules.exec;
                var shellQuote = prSplit._shellQuote;
                var makPath = (dir !== '.' ? dir + '/' : '') + 'Makefile';
                var hasMakefile = osmod ? osmod.fileExists(makPath) : exec.execv(['test', '-f', makPath]).code === 0;
                if (hasMakefile) {
                    var grep = exec.execv(['sh', '-c',
                        'cd ' + shellQuote(dir) + ' && grep -q "^generate:" Makefile']);
                    if (grep.code === 0) return true;
                }
                var goGen = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && grep -rl "//go:generate" --include="*.go" . 2>/dev/null | head -1']);
                return goGen.code === 0 && goGen.stdout.trim() !== '';
            },
            fix: function(dir) {
                var exec = prSplit._modules.exec;
                var gitExec = prSplit._gitExec;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFiles = prSplit._gitAddChangedFiles;
                var hasMakeTarget = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && grep -q "^generate:" Makefile 2>/dev/null']).code === 0;
                var result;
                if (hasMakeTarget) {
                    result = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && make generate']);
                } else {
                    result = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && go generate ./...']);
                }
                if (result.code !== 0) {
                    return { fixed: false, error: 'generate failed: ' + result.stderr.trim() };
                }
                var status = gitExec(dir, ['status', '--porcelain']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'generate made no changes' };
                }
                gitAddChangedFiles(dir);
                var commit = gitExec(dir, ['commit', '-m', 'fix: run code generation for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'add-missing-files',
            detect: function(dir, verifyOutput) {
                if (!verifyOutput) return false;
                return verifyOutput.indexOf('no such file or directory') >= 0 ||
                       verifyOutput.indexOf('cannot find') >= 0 ||
                       verifyOutput.indexOf('file not found') >= 0;
            },
            fix: function(dir, failedBranch, plan) {
                var gitExec = prSplit._gitExec;
                var gitAddChangedFiles = prSplit._gitAddChangedFiles;
                if (!plan || !plan.sourceBranch) {
                    return { fixed: false, error: 'no source branch to pull files from' };
                }
                var diffFiles = gitExec(dir, ['diff', '--name-only', failedBranch, plan.sourceBranch]);
                if (diffFiles.code !== 0 || diffFiles.stdout.trim() === '') {
                    return { fixed: false, error: 'no candidate files to add' };
                }
                var candidates = diffFiles.stdout.trim().split('\n');
                var added = 0;
                for (var f = 0; f < candidates.length; f++) {
                    var co = gitExec(dir, ['checkout', plan.sourceBranch, '--', candidates[f]]);
                    if (co.code === 0) {
                        added++;
                    }
                }
                if (added === 0) {
                    return { fixed: false, error: 'no files could be checked out from source' };
                }
                gitAddChangedFiles(dir);
                var commit = gitExec(dir, ['commit', '-m', 'fix: add missing files from source branch']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'claude-fix',
            // Late-bound: claudeExecutor set by pipeline chunk (10)
            detect: function() {
                var claudeExecutor = prSplit._claudeExecutor;
                return !!(claudeExecutor && claudeExecutor.handle && claudeExecutor.isAvailable());
            },
            // Async: sendToHandle uses setTimeout delay for PTY write separation
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var claudeExecutor = prSplit._claudeExecutor;
                var renderConflictPrompt = prSplit.renderConflictPrompt;
                var sendToHandle = prSplit.sendToHandle;
                var waitForLogged = prSplit.waitForLogged;
                var mcpCallbackObj = prSplit._mcpCallbackObj;
                var validateResolution = prSplit.validateResolution;
                var gitExec = prSplit._gitExec;
                var gitAddChangedFiles = prSplit._gitAddChangedFiles;
                var osmod = prSplit._modules.osmod;
                var exec = prSplit._modules.exec;
                var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS || {};

                if (!claudeExecutor || !claudeExecutor.handle) {
                    return { fixed: false, error: 'Claude executor not available' };
                }
                options = options || {};
                var resolveTimeoutMs = options.resolveTimeoutMs || AUTOMATED_DEFAULTS.resolveTimeoutMs;
                var pollIntervalMs = options.pollIntervalMs || AUTOMATED_DEFAULTS.pollIntervalMs;
                var aliveCheckFn = typeof options.aliveCheckFn === 'function' ? options.aliveCheckFn : null;
                var promptResult = renderConflictPrompt({
                    branchName: failedBranch,
                    files: plan ? plan.splits.filter(function(s) { return s.name === failedBranch; }).reduce(function(acc, s) { return acc.concat(s.files); }, []) : [],
                    exitCode: 1,
                    errorOutput: verifyOutput || '',
                    goModContent: '',
                    sessionId: claudeExecutor.sessionId || ''
                });
                if (promptResult.error) {
                    return { fixed: false, error: 'render conflict prompt failed: ' + promptResult.error };
                }
                var sendResult = await sendToHandle(claudeExecutor.handle, promptResult.text);
                if (sendResult.error) {
                    return { fixed: false, error: 'failed to send to Claude: ' + sendResult.error };
                }
                mcpCallbackObj.resetWaiter('reportResolution');
                var resolutionPoll = waitForLogged('reportResolution', resolveTimeoutMs, {
                    aliveCheck: aliveCheckFn,
                    checkIntervalMs: pollIntervalMs
                });
                if (resolutionPoll.error) {
                    return { fixed: false, error: 'Claude resolution timeout: ' + resolutionPoll.error };
                }
                var resolution = resolutionPoll.data;
                var resVal = validateResolution(resolution);
                if (!resVal.valid) {
                    log.printf('auto-split: resolution validation errors: %s', resVal.errors.join('; '));
                    return { fixed: false, error: 'invalid resolution: ' + resVal.errors.join('; ') };
                }
                if (resolution.patches && resolution.patches.length > 0) {
                    for (var p = 0; p < resolution.patches.length; p++) {
                        var patch = resolution.patches[p];
                        if (osmod) {
                            osmod.writeFile(patch.file, patch.content);
                        }
                    }
                }
                if (resolution.commands && resolution.commands.length > 0) {
                    for (var c = 0; c < resolution.commands.length; c++) {
                        exec.execv(['sh', '-c', resolution.commands[c]]);
                    }
                }
                var status = gitExec(dir, ['status', '--porcelain']);
                if (status.stdout.trim() !== '') {
                    gitAddChangedFiles(dir);
                    var commit = gitExec(dir, ['commit', '--amend', '--no-edit']);
                    if (commit.code !== 0) {
                        return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                    }
                }
                return { fixed: true, error: null };
            }
        }
    ];

    // resolveConflicts: attempts to auto-fix split branches that fail verification.
    // For each split: checkout → verify → if fails, try strategies with retry budget.
    // Returns: {fixed: [], errors: [], totalRetries, branchRetries, reSplitNeeded, reSplitFiles, reSplitReason}
    prSplit.resolveConflicts = async function resolveConflicts(plan, options) {
        var gitExec = prSplit._gitExec;
        var exec = prSplit._modules.exec;
        var shellQuote = prSplit._shellQuote;
        var isCancelled = prSplit.isCancelled;
        var runtime = prSplit.runtime;
        var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS || {};

        if (!plan || !plan.splits) {
            return { fixed: [], errors: [{ name: '(plan)', error: 'invalid plan: missing splits' }], reSplitNeeded: false, totalRetries: 0, reSplitFiles: [], reSplitReason: '' };
        }
        options = options || {};
        var dir = plan.dir || '.';
        var verifyCommand = options.verifyCommand || plan.verifyCommand || runtime.verifyCommand;
        var retryBudget = typeof options.retryBudget === 'number' ? options.retryBudget : (typeof runtime.retryBudget === 'number' ? runtime.retryBudget : 3);
        var perBranchRetryBudget = typeof options.perBranchRetryBudget === 'number' ? options.perBranchRetryBudget : 2;
        var strategies = options.strategies || AUTO_FIX_STRATEGIES;
        var totalRetries = 0;
        var branchRetries = {};

        var strategyOptions = {
            resolveTimeoutMs: options.resolveTimeoutMs || AUTOMATED_DEFAULTS.resolveTimeoutMs,
            pollIntervalMs: options.pollIntervalMs || AUTOMATED_DEFAULTS.pollIntervalMs,
            aliveCheckFn: typeof options.aliveCheckFn === 'function' ? options.aliveCheckFn : null
        };

        var wallClockTimeoutMs = typeof options.wallClockTimeoutMs === 'number'
            ? options.wallClockTimeoutMs
            : (AUTOMATED_DEFAULTS.resolveWallClockTimeoutMs || 300000);
        var deadline = Date.now() + wallClockTimeoutMs;

        if (!verifyCommand || verifyCommand === 'true') {
            return { fixed: [], skipped: 'no verify command configured', errors: [], reSplitNeeded: false };
        }

        var savedBranch = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        if (savedBranch.code !== 0) {
            return { fixed: [], errors: [{ name: '(setup)', error: 'failed to get current branch' }], reSplitNeeded: false };
        }
        var restoreBranch = savedBranch.stdout.trim();

        var fixed = [];
        var errorsOut = [];
        var reSplitNeeded = false;
        var reSplitFiles = [];

        for (var i = 0; i < plan.splits.length; i++) {
            if (isCancelled()) {
                gitExec(dir, ['checkout', restoreBranch]);
                return {
                    fixed: fixed, errors: errorsOut,
                    totalRetries: totalRetries, branchRetries: branchRetries,
                    reSplitNeeded: reSplitNeeded, reSplitFiles: reSplitFiles,
                    reSplitReason: '', cancelledByUser: true
                };
            }

            if (Date.now() >= deadline) {
                var elapsed = Date.now() - (deadline - wallClockTimeoutMs);
                errorsOut.push({ name: plan.splits[i].name, error: 'wall-clock timeout after ' + elapsed + 'ms (limit: ' + wallClockTimeoutMs + 'ms)' });
                for (var remaining = i + 1; remaining < plan.splits.length; remaining++) {
                    errorsOut.push({ name: plan.splits[remaining].name, error: 'wall-clock timeout after ' + elapsed + 'ms (limit: ' + wallClockTimeoutMs + 'ms)' });
                }
                break;
            }

            if (totalRetries >= retryBudget) {
                errorsOut.push({ name: plan.splits[i].name, error: 'global retry budget exhausted (' + retryBudget + ')' });
                continue;
            }

            var split = plan.splits[i];
            branchRetries[split.name] = branchRetries[split.name] || 0;
            if (branchRetries[split.name] >= perBranchRetryBudget) {
                errorsOut.push({ name: split.name, error: 'per-branch retry budget exhausted (' + perBranchRetryBudget + ')' });
                continue;
            }

            var co = gitExec(dir, ['checkout', split.name]);
            if (co.code !== 0) {
                errorsOut.push({ name: split.name, error: 'checkout failed: ' + co.stderr.trim() });
                continue;
            }

            var verifyResult = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && ' + verifyCommand]);
            if (verifyResult.code === 0) {
                continue;
            }

            var verifyOutput = (verifyResult.stdout || '') + '\n' + (verifyResult.stderr || '');

            var resolved = false;
            while (branchRetries[split.name] < perBranchRetryBudget && totalRetries < retryBudget && !resolved) {
                if (Date.now() >= deadline) {
                    break;
                }
                if (isCancelled()) {
                    break;
                }

                var madeProgress = false;
                for (var s = 0; s < strategies.length && branchRetries[split.name] < perBranchRetryBudget && totalRetries < retryBudget; s++) {
                    if (Date.now() >= deadline) {
                        break;
                    }
                    if (isCancelled()) {
                        break;
                    }
                    var strategy = strategies[s];
                    if (!strategy.detect(dir, verifyOutput)) {
                        continue;
                    }

                    totalRetries++;
                    branchRetries[split.name]++;
                    madeProgress = true;
                    var fixResult = await strategy.fix(dir, split.name, plan, verifyOutput, strategyOptions);
                    if (!fixResult.fixed) {
                        log.warn('strategy ' + strategy.name + ' failed for ' + split.name +
                                 (fixResult.error ? ': ' + fixResult.error : ''));
                        continue;
                    }

                    var reVerify = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && ' + verifyCommand]);
                    if (reVerify.code === 0) {
                        fixed.push({ name: split.name, strategy: strategy.name });
                        resolved = true;
                        break;
                    }
                    verifyOutput = (reVerify.stdout || '') + '\n' + (reVerify.stderr || '');
                    break;
                }

                if (!madeProgress) {
                    break;
                }
            }

            if (!resolved) {
                errorsOut.push({
                    name: split.name,
                    error: 'verification failed after all auto-fix strategies',
                    lastOutput: verifyOutput
                });
                reSplitNeeded = true;
                reSplitFiles = reSplitFiles.concat(split.files || []);
            }
        }

        gitExec(dir, ['checkout', restoreBranch]);

        return {
            fixed: fixed,
            errors: errorsOut,
            totalRetries: totalRetries,
            branchRetries: branchRetries,
            reSplitNeeded: reSplitNeeded,
            reSplitFiles: reSplitFiles,
            reSplitReason: reSplitNeeded ? 'Auto-fix strategies exhausted for: ' + errorsOut.map(function(e) { return e.name; }).join(', ') : ''
        };
    };

    prSplit.AUTO_FIX_STRATEGIES = AUTO_FIX_STRATEGIES;

})(globalThis.prSplit);
