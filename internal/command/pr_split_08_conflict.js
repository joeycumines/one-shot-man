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

    // fileExistsSync checks file existence using osmod (preferred) or the
    // platform shell as fallback. Avoids hardcoded 'test -f' on Windows.
    function fileExistsSync(path) {
        var osmod = prSplit._modules.osmod;
        if (osmod && typeof osmod.fileExists === 'function') {
            return osmod.fileExists(path);
        }
        var exec = prSplit._modules.exec;
        var isWindows = prSplit._isWindows;
        if (isWindows && isWindows()) {
            return exec.execv(['cmd.exe', '/C', 'if exist "' + path + '" (exit 0) else (exit 1)']).code === 0;
        }
        return exec.execv(['test', '-f', path]).code === 0;
    }

    // AUTO_FIX_STRATEGIES: sequential repair strategies to try when a split
    // branch fails verification. Each: {name, detect(dir, verifyOutput), fix(dir, ...)}.
    var AUTO_FIX_STRATEGIES = [
        {
            name: 'go-mod-tidy',
            detect: function(dir) {
                var path = (dir !== '.' ? dir + '/' : '') + 'go.mod';
                return fileExistsSync(path);
            },
            fix: async function(dir) {
                var shellExecAsync = prSplit._shellExecAsync;
                var gitExecAsync = prSplit._gitExecAsync;
                var shellQuote = prSplit._shellQuote;
                var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                var tidyResult = await shellExecAsync(
                    cdCmd + shellQuote(dir) + ' && go mod tidy');
                if (tidyResult.code !== 0) {
                    return { fixed: false, error: 'go mod tidy failed: ' + tidyResult.stderr.trim() };
                }
                var status = await gitExecAsync(dir, ['status', '--porcelain', 'go.mod', 'go.sum']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'go mod tidy succeeded but made no changes to go.mod/go.sum — strategy not applicable for this failure' };
                }
                var addRes = await gitExecAsync(dir, ['add', 'go.mod', 'go.sum']);
                if (addRes.code !== 0) {
                    return { fixed: false, error: 'git add go.mod/go.sum failed: ' + addRes.stderr.trim() };
                }
                var commit = await gitExecAsync(dir, ['commit', '-m', 'fix: go mod tidy for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'go-generate-sum',
            detect: function(dir) {
                var path = (dir !== '.' ? dir + '/' : '') + 'go.sum';
                return fileExistsSync(path);
            },
            fix: async function(dir) {
                var shellExecAsync = prSplit._shellExecAsync;
                var gitExecAsync = prSplit._gitExecAsync;
                var shellQuote = prSplit._shellQuote;
                var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                var dlResult = await shellExecAsync(
                    cdCmd + shellQuote(dir) + ' && go mod download');
                if (dlResult.code !== 0) {
                    return { fixed: false, error: 'go mod download failed: ' + dlResult.stderr.trim() };
                }
                var status = await gitExecAsync(dir, ['status', '--porcelain', 'go.sum']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'go mod download succeeded but made no changes to go.sum — strategy not applicable for this failure' };
                }
                var addRes = await gitExecAsync(dir, ['add', 'go.sum']);
                if (addRes.code !== 0) {
                    return { fixed: false, error: 'git add go.sum failed: ' + addRes.stderr.trim() };
                }
                var commit = await gitExecAsync(dir, ['commit', '-m', 'fix: update go.sum for split']);
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
            fix: async function(dir) {
                var shellExecAsync = prSplit._shellExecAsync;
                var gitExecAsync = prSplit._gitExecAsync;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFilesAsync = prSplit._gitAddChangedFilesAsync;
                var lookupBinaryAsync = prSplit._lookupBinaryAsync;
                var goimportsLookup = await lookupBinaryAsync('goimports');
                if (goimportsLookup.found) {
                    var result;
                    if (prSplit._isWindows && prSplit._isWindows()) {
                        // On Windows, use a recursive find equivalent.
                        result = await shellExecAsync(
                            'cd /d ' + shellQuote(dir) + ' && for /r %f in (*.go) do goimports -w "%f"');
                    } else {
                        result = await shellExecAsync(
                            'cd ' + shellQuote(dir) + ' && find . -name "*.go" -exec goimports -w {} +');
                    }
                    if (result.code !== 0) {
                        return { fixed: false, error: 'goimports failed: ' + result.stderr.trim() };
                    }
                } else {
                    return { fixed: false, error: 'goimports not available' };
                }
                var status = await gitExecAsync(dir, ['status', '--porcelain']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'goimports made no changes' };
                }
                await gitAddChangedFilesAsync(dir);
                var commit = await gitExecAsync(dir, ['commit', '-m', 'fix: goimports for split']);
                if (commit.code !== 0) {
                    return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
                }
                return { fixed: true, error: null };
            }
        },
        {
            name: 'npm-install',
            detect: function(dir) {
                var path = (dir !== '.' ? dir + '/' : '') + 'package.json';
                return fileExistsSync(path);
            },
            fix: async function(dir) {
                var shellExecAsync = prSplit._shellExecAsync;
                var gitExecAsync = prSplit._gitExecAsync;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFilesAsync = prSplit._gitAddChangedFilesAsync;
                var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                var result = await shellExecAsync(
                    cdCmd + shellQuote(dir) + ' && npm install --no-audit --no-fund 2>&1');
                if (result.code !== 0) {
                    return { fixed: false, error: 'npm install failed: ' + (result.stderr || result.stdout || '').trim() };
                }
                var status = await gitExecAsync(dir, ['status', '--porcelain']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'npm install made no changes' };
                }
                await gitAddChangedFilesAsync(dir);
                var commit = await gitExecAsync(dir, ['commit', '-m', 'fix: npm install for split']);
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
                var shellQuote = prSplit._shellQuote;
                var makPath = (dir !== '.' ? dir + '/' : '') + 'Makefile';
                var hasMakefile = fileExistsSync(makPath);
                if (hasMakefile) {
                    // Check if the Makefile has a 'generate:' target.
                    // Prefer osmod.readFile to avoid shell-dependent grep.
                    if (osmod && typeof osmod.readFile === 'function') {
                        try {
                            var readResult = osmod.readFile(makPath);
                            if (!readResult.error) {
                                var fileContent = readResult.content;
                                if (fileContent.indexOf('\ngenerate:') >= 0 || fileContent.indexOf('generate:') === 0) {
                                    return true;
                                }
                            }
                        } catch (e) { /* fall through */ }
                    } else {
                        // Fallback: use platform shell to check Makefile contents.
                        var exec = prSplit._modules.exec;
                        var shellSpawnSync = prSplit._shellSpawnSync;
                        var isWindows = prSplit._isWindows;
                        if (shellSpawnSync) {
                            var cdFallback = (isWindows && isWindows()) ? 'cd /d ' : 'cd ';
                            var grepCmd = (isWindows && isWindows())
                                ? cdFallback + shellQuote(dir) + ' && findstr /b "generate:" Makefile'
                                : cdFallback + shellQuote(dir) + ' && grep -q "^generate:" Makefile';
                            var grep = shellSpawnSync(grepCmd);
                            if (grep.code === 0) return true;
                        }
                    }
                }
                // Check for //go:generate directives in .go files.
                if (osmod && typeof osmod.readDir === 'function') {
                    // Use osmod to scan for go:generate (avoids grep -rl).
                    // Simple heuristic: check top-level .go files only.
                    try {
                        var entries = osmod.readDir(dir === '.' ? '.' : dir);
                        for (var i = 0; i < entries.length; i++) {
                            if (entries[i].match && entries[i].match(/\.go$/)) {
                                var goPath = (dir !== '.' ? dir + '/' : '') + entries[i];
                                var goReadResult = osmod.readFile(goPath);
                                if (!goReadResult.error && goReadResult.content.indexOf('//go:generate') >= 0) {
                                    return true;
                                }
                            }
                        }
                    } catch (e) { /* fall through */ }
                    return false;
                }
                // Last resort: use platform shell to scan for go:generate.
                var shellSpawnSync2 = prSplit._shellSpawnSync;
                var isWindows2 = prSplit._isWindows;
                if (shellSpawnSync2) {
                    var cdLast = (isWindows2 && isWindows2()) ? 'cd /d ' : 'cd ';
                    var scanCmd = (isWindows2 && isWindows2())
                        ? cdLast + shellQuote(dir) + ' && findstr /s /m "go:generate" *.go'
                        : cdLast + shellQuote(dir) + ' && grep -rl "//go:generate" --include="*.go" . 2>/dev/null | head -1';
                    var goGen = shellSpawnSync2(scanCmd);
                    return goGen.code === 0 && (goGen.stdout || '').trim() !== '';
                }
                return false;
            },
            fix: async function(dir) {
                var shellExecAsync = prSplit._shellExecAsync;
                var gitExecAsync = prSplit._gitExecAsync;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFilesAsync = prSplit._gitAddChangedFilesAsync;
                var osmod = prSplit._modules.osmod;
                var cdPrefix = (prSplit._isWindows && prSplit._isWindows())
                    ? 'cd /d ' + shellQuote(dir) + ' && '
                    : 'cd ' + shellQuote(dir) + ' && ';
                // Determine if Makefile has a 'generate:' target.
                var hasMakeTarget = false;
                var makPath = (dir !== '.' ? dir + '/' : '') + 'Makefile';
                if (osmod && typeof osmod.readFile === 'function') {
                    try {
                        var readResult2 = osmod.readFile(makPath);
                        if (!readResult2.error) {
                            hasMakeTarget = readResult2.content.indexOf('\ngenerate:') >= 0 || readResult2.content.indexOf('generate:') === 0;
                        }
                    } catch (e) { /* fall through */ }
                } else {
                    var grepFixCmd = (prSplit._isWindows && prSplit._isWindows())
                        ? cdPrefix + 'findstr /b "generate:" Makefile'
                        : cdPrefix + 'grep -q "^generate:" Makefile 2>/dev/null';
                    hasMakeTarget = (await shellExecAsync(grepFixCmd)).code === 0;
                }
                var result;
                if (hasMakeTarget) {
                    result = await shellExecAsync(cdPrefix + 'make generate');
                } else {
                    result = await shellExecAsync(cdPrefix + 'go generate ./...');
                }
                if (result.code !== 0) {
                    return { fixed: false, error: 'generate failed: ' + result.stderr.trim() };
                }
                var status = await gitExecAsync(dir, ['status', '--porcelain']);
                if (status.stdout.trim() === '') {
                    return { fixed: false, error: 'generate made no changes' };
                }
                await gitAddChangedFilesAsync(dir);
                var commit = await gitExecAsync(dir, ['commit', '-m', 'fix: run code generation for split']);
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
            fix: async function(dir, failedBranch, plan) {
                var gitExecAsync = prSplit._gitExecAsync;
                var gitAddChangedFilesAsync = prSplit._gitAddChangedFilesAsync;
                if (!plan || !plan.sourceBranch) {
                    return { fixed: false, error: 'no source branch to pull files from' };
                }
                var diffFiles = await gitExecAsync(dir, ['diff', '--name-only', failedBranch, plan.sourceBranch]);
                if (diffFiles.code !== 0 || diffFiles.stdout.trim() === '') {
                    return { fixed: false, error: 'no candidate files to add' };
                }
                var candidates = diffFiles.stdout.trim().split('\n');
                var added = 0;
                for (var f = 0; f < candidates.length; f++) {
                    var co = await gitExecAsync(dir, ['checkout', plan.sourceBranch, '--', candidates[f]]);
                    if (co.code === 0) {
                        added++;
                    }
                }
                if (added === 0) {
                    return { fixed: false, error: 'no files could be checked out from source' };
                }
                await gitAddChangedFilesAsync(dir);
                var commit = await gitExecAsync(dir, ['commit', '-m', 'fix: add missing files from source branch']);
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
                var gitExecAsync = prSplit._gitExecAsync;
                var gitAddChangedFilesAsync = prSplit._gitAddChangedFilesAsync;
                var shellQuote = prSplit._shellQuote;
                var shellExecAsync = prSplit._shellExecAsync;
                var osmod = prSplit._modules.osmod;
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
                var resolutionPoll = await waitForLogged('reportResolution', resolveTimeoutMs, {
                    aliveCheck: aliveCheckFn,
                    checkIntervalMs: pollIntervalMs
                });
                if (resolutionPoll.error) {
                    return { fixed: false, error: 'Claude resolution timed out waiting for reportResolution tool call: ' + resolutionPoll.error + ' — check Claude process logs for errors' };
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
                            // Write to the worktree directory (dir), not the CWD.
                            osmod.writeFile(dir + '/' + patch.file, patch.content);
                        }
                    }
                }
                if (resolution.commands && resolution.commands.length > 0) {
                    for (var c = 0; c < resolution.commands.length; c++) {
                        // Run commands in the worktree directory (async — T078: does not block event loop).
                        var cdCmd2 = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                        await shellExecAsync(cdCmd2 + shellQuote(dir) + ' && ' + resolution.commands[c]);
                    }
                }
                var status = await gitExecAsync(dir, ['status', '--porcelain']);
                if (status.stdout.trim() !== '') {
                    await gitAddChangedFilesAsync(dir);
                    var commit = await gitExecAsync(dir, ['commit', '--amend', '--no-edit']);
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
        var gitExecAsync = prSplit._gitExecAsync;
        var shellExecAsync = prSplit._shellExecAsync;
        var resolveDir = prSplit._resolveDir;
        var shellQuote = prSplit._shellQuote;
        var isCancelled = prSplit.isCancelled;
        var isForceCancelled = prSplit.isForceCancelled;  // T117: honor force-cancel
        var runtime = prSplit.runtime;
        var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS || {};

        if (!plan || !plan.splits) {
            return { fixed: [], errors: [{ name: '(plan)', error: 'invalid plan: missing splits' }], reSplitNeeded: false, totalRetries: 0, reSplitFiles: [], reSplitReason: '' };
        }
        options = options || {};
        var dir = resolveDir(plan.dir || '.');
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

        var fixed = [];
        var errorsOut = [];
        var reSplitNeeded = false;
        var reSplitFiles = [];

        for (var i = 0; i < plan.splits.length; i++) {
            if (isCancelled() || isForceCancelled()) {  // T117: honor force-cancel
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

            // Create a temporary worktree for this branch so the user's CWD
            // remains untouched during fix attempts.
            var worktreeDir = prSplit._worktreeTmpPath('osm-fix-');
            var wtAdd = await gitExecAsync(dir, ['worktree', 'add', worktreeDir, split.name]);
            if (wtAdd.code !== 0) {
                errorsOut.push({ name: split.name, error: 'create worktree failed: ' + wtAdd.stderr.trim() });
                continue;
            }

            var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
            var verifyResult = await shellExecAsync(cdCmd + shellQuote(worktreeDir) + ' && ' + verifyCommand);
            if (verifyResult.code === 0) {
                await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreeDir]);
                continue;
            }

            var verifyOutput = (verifyResult.stdout || '') + '\n' + (verifyResult.stderr || '');

            var resolved = false;
            while (branchRetries[split.name] < perBranchRetryBudget && totalRetries < retryBudget && !resolved) {
                if (Date.now() >= deadline) {
                    break;
                }
                if (isCancelled() || isForceCancelled()) {  // T117: honor force-cancel
                    break;
                }

                var madeProgress = false;
                for (var s = 0; s < strategies.length && branchRetries[split.name] < perBranchRetryBudget && totalRetries < retryBudget; s++) {
                    if (Date.now() >= deadline) {
                        break;
                    }
                    if (isCancelled() || isForceCancelled()) {  // T117: honor force-cancel
                        break;
                    }
                    var strategy = strategies[s];
                    if (!strategy.detect(worktreeDir, verifyOutput)) {
                        continue;
                    }

                    totalRetries++;
                    branchRetries[split.name]++;
                    madeProgress = true;
                    var fixResult = await strategy.fix(worktreeDir, split.name, plan, verifyOutput, strategyOptions);
                    if (!fixResult.fixed) {
                        log.warn('strategy ' + strategy.name + ' failed for ' + split.name +
                                 (fixResult.error ? ': ' + fixResult.error : ''));
                        continue;
                    }

                    var reVerifyCd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                    var reVerify = await shellExecAsync(reVerifyCd + shellQuote(worktreeDir) + ' && ' + verifyCommand);
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

            // Cleanup worktree for this branch.
            await gitExecAsync(dir, ['worktree', 'remove', '--force', worktreeDir]);
        }

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
