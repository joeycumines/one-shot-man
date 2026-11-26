'use strict';
// pr_split_08_conflict.js — Auto-fix strategies & local conflict resolution
// Dependencies: chunks 00, 04 must be loaded first
// Late-binds: renderConflictPrompt (09), sendToHandle (10), waitForLogged (10),
//             AUTOMATED_DEFAULTS (10), _agentExecutor (10), _mcpCallbackObj (10)
//
// All external symbols are read from prSplit.* INSIDE function bodies for
// late-binding. This allows mocking in tests and avoids capturing undefined
// references at load time.

(function(prSplit) {

    function commandError(result, label) {
        var error = new Error(label + ': ' + repairFailureReason(result));
        error.commandResult = result;
        if (result && result.treeCleanupError) error.treeCleanupError = result.treeCleanupError;
        return error;
    }

    function repairFailureReason(result) {
        return prSplit._execFailReason(result || {}) || String(result && result.error || 'command outcome unavailable');
    }

    async function repairCommand(fn, args, observe) {
        var result;
        try { result = await fn.apply(null, args); }
        catch (error) {
            if (observe) observe(error, true);
            throw error;
        }
        if (observe) observe(result);
        if (!result || typeof result.code !== 'number' || result.code < 0 ||
            prSplit._commandFailed(result, [result.code])) {
            throw commandError(result, 'repair command execution failed');
        }
        return result;
    }

    // Repair commands share one resource owner. Structural execution failures
    // stop every later mutation, including commands nested inside strategies.
    function resolutionWorktree(dir, path, branch, isCancelled) {
        var runGit = prSplit._gitExecAsync, runShell = prSplit._shellExecAsync;
        var attempted = false, treeCleanupError = '', stopped = null;
        function observe(value, rejected) {
            if (value && value.treeCleanupError) treeCleanupError = value.treeCleanupError;
            if (rejected || !value || typeof value.code !== 'number' || value.code < 0 ||
                prSplit._commandFailed(value, [value.code])) {
                stopped = value || {message:'repair command outcome unavailable'};
            }
        }
        function checkCancellation() {
            if (!stopped && isCancelled && isCancelled()) stopped = {cancelled:true, message:'repair cancelled'};
            if (stopped) throw commandError(stopped, 'repair stopped after execution failure');
        }
        function run(fn, args) {
            try { checkCancellation(); }
            catch (error) { return Promise.reject(error); }
            return repairCommand(fn, args, observe);
        }
        var owner = {
            git: function() { return run(runGit, arguments); },
            shell: function() { return run(runShell, arguments); },
            require: function(result, label) {
                observe(result);
                if (prSplit._commandFailed(result)) throw commandError(result, label);
                return result;
            },
            open: async function() {
                checkCancellation();
                attempted = true;
                owner.require(await owner.git(dir, ['worktree', 'add', path, branch]), 'create worktree failed');
            },
            stage: function(worktreeDir) { return prSplit._gitAddChangedFilesAsync(worktreeDir, owner.git); },
            failure: function(error) {
                observe(error);
                var result = error && (error.commandResult || error.cause) || error;
                observe(result);
                var failure = {name:branch, error:String(error && (error.message || error.error) || error), execFailed:true};
                if (result && result.code !== undefined) failure.exitCode = result.code;
                for (var key of ['truncated', 'timedOut', 'cancelled']) {
                    if (result && result[key] !== undefined) failure[key] = result[key];
                }
                return failure;
            },
            cleanup: async function(outcome) {
                if (!attempted) return outcome;
                var message = '', cleanupTreeError = treeCleanupError;
                if (treeCleanupError) {
                    outcome.worktreeRetained = true;
                    message = 'process tree teardown failed: ' + treeCleanupError + '; worktree retained at ' + path;
                } else {
                    var removed, rejected = false;
                    try { removed = await runGit(dir, ['worktree', 'remove', '--force', path]); }
                    catch (error) { removed = error; rejected = true; }
                    if (rejected || prSplit._commandFailed(removed)) {
                        cleanupTreeError = removed && removed.treeCleanupError;
                        message = 'worktree removal failed: ' + repairFailureReason(removed) +
                            '; worktree removal was attempted at ' + path;
                    }
                }
                if (message) {
                    outcome.cleanupFailed = true;
                    outcome.execFailed = true;
                    outcome.worktreeDir = path;
                    if (cleanupTreeError) outcome.treeCleanupError = cleanupTreeError;
                    outcome.error = (outcome.error ? outcome.error + '; ' : '') + 'cleanup: ' + message;
                }
                return outcome;
            }
        };
        return owner;
    }

    function strategyCommands(options) {
        return options && options.commands || {
            git: function() { return repairCommand(prSplit._gitExecAsync, arguments); },
            shell: function() { return repairCommand(prSplit._shellExecAsync, arguments); },
            stage: prSplit._gitAddChangedFilesAsync
        };
    }

    function shellExecOptions(timeoutMs) {
        if (!(typeof timeoutMs === 'number' && timeoutMs > 0) &&
            typeof prSplitConfig !== 'undefined' &&
            typeof prSplitConfig.timeoutMs === 'number' && prSplitConfig.timeoutMs > 0) {
            timeoutMs = prSplitConfig.timeoutMs;
        }
        return (typeof timeoutMs === 'number' && timeoutMs > 0) ? { timeoutMs: timeoutMs } : {};
    }

    // fileExistsSync checks file existence using osmod (preferred) or the
    // platform shell as fallback. Avoids hardcoded 'test -f' on Windows.
    async function fileExistsSync(path) {
        var osmod = prSplit._modules.osmod;
        if (osmod && typeof osmod.fileExists === 'function') {
            var r = await osmod.fileExists(path);
            return !!(r && r.exists);
        }
        // osmod.fileExists is a core module that should always be registered
        // in production; reaching here means broken module wiring. Treat the
        // file as not found (don't block) but warn loudly, otherwise every
        // auto-fix strategy that uses fileExistsSync silently no-ops and the
        // real cause (missing osmod) is invisible.
        if (typeof log !== 'undefined' && log.warn) {
            log.warn('fileExistsSync: osmod.fileExists unavailable; treating "' + path + '" as not found');
        }
        return false;
    }

    // AUTO_FIX_STRATEGIES: sequential repair strategies to try when a split
    // branch fails verification. Each: {name, detect(dir, verifyOutput), fix(dir, ...)}.
    var AUTO_FIX_STRATEGIES = [
        {
            name: 'go-mod-tidy',
            detect: async function(dir) {
                var path = (dir !== '.' ? dir + '/' : '') + 'go.mod';
                return await fileExistsSync(path);
            },
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var shellExecAsync = strategyCommands(options).shell;
                var gitExecAsync = strategyCommands(options).git;
                var shellQuote = prSplit._shellQuote;
                var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                var tidyResult = await shellExecAsync(
                    cdCmd + shellQuote(dir) + ' && go mod tidy', shellExecOptions(options && options.verifyTimeoutMs));
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
            detect: async function(dir) {
                var path = (dir !== '.' ? dir + '/' : '') + 'go.sum';
                return await fileExistsSync(path);
            },
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var shellExecAsync = strategyCommands(options).shell;
                var gitExecAsync = strategyCommands(options).git;
                var shellQuote = prSplit._shellQuote;
                var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                var dlResult = await shellExecAsync(
                    cdCmd + shellQuote(dir) + ' && go mod download', shellExecOptions(options && options.verifyTimeoutMs));
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
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var shellExecAsync = strategyCommands(options).shell;
                var gitExecAsync = strategyCommands(options).git;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFilesAsync = strategyCommands(options).stage;
                var lookupBinaryAsync = prSplit._lookupBinaryAsync;
                var goimportsLookup = await lookupBinaryAsync('goimports');
                if (goimportsLookup.found) {
                    var result;
                    if (prSplit._isWindows && prSplit._isWindows()) {
                        // On Windows, use a recursive find equivalent.
                        result = await shellExecAsync(
                            'cd /d ' + shellQuote(dir) + ' && for /r %f in (*.go) do goimports -w "%f"', shellExecOptions(options && options.verifyTimeoutMs));
                    } else {
                        result = await shellExecAsync(
                            'cd ' + shellQuote(dir) + ' && find . -name "*.go" -exec goimports -w {} +', shellExecOptions(options && options.verifyTimeoutMs));
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
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var shellExecAsync = strategyCommands(options).shell;
                var gitExecAsync = strategyCommands(options).git;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFilesAsync = strategyCommands(options).stage;
                var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                var result = await shellExecAsync(
                    cdCmd + shellQuote(dir) + ' && npm install --no-audit --no-fund 2>&1', shellExecOptions(options && options.verifyTimeoutMs));
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
            detect: async function(dir, verifyOutput, options) {
                var osmod = prSplit._modules.osmod;
                var shellQuote = prSplit._shellQuote;
                var makPath = (dir !== '.' ? dir + '/' : '') + 'Makefile';
                var hasMakefile = await fileExistsSync(makPath);
                if (hasMakefile) {
                    // Check if the Makefile has a 'generate:' target.
                    // Prefer osmod.readFile to avoid shell-dependent grep.
                    if (osmod && typeof osmod.readFile === 'function') {
                        try {
                            var readResult = await osmod.readFile(makPath);
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
                        var shellSpawnSync = strategyCommands(options).shell;
                        var isWindows = prSplit._isWindows;
                        if (shellSpawnSync) {
                            var cdFallback = (isWindows && isWindows()) ? 'cd /d ' : 'cd ';
                            var grepCmd = (isWindows && isWindows())
                                ? cdFallback + shellQuote(dir) + ' && findstr /b "generate:" Makefile'
                                : cdFallback + shellQuote(dir) + ' && grep -q "^generate:" Makefile';
                            var grep = await shellSpawnSync(grepCmd);
                            if (grep.code === 0) return true;
                        }
                    }
                }
                // Check for //go:generate directives in .go files.
                if (osmod && typeof osmod.readDir === 'function') {
                    // Use osmod to scan for go:generate (avoids grep -rl).
                    // Simple heuristic: check top-level .go files only.
                    try {
                        var entries = await osmod.readDir(dir === '.' ? '.' : dir);
                        for (var i = 0; i < entries.length; i++) {
                            if (entries[i].match && entries[i].match(/\.go$/)) {
                                var goPath = (dir !== '.' ? dir + '/' : '') + entries[i];
                                var goReadResult = await osmod.readFile(goPath);
                                if (!goReadResult.error && goReadResult.content.indexOf('//go:generate') >= 0) {
                                    return true;
                                }
                            }
                        }
                    } catch (e) { /* fall through */ }
                    return false;
                }
                // Last resort: use platform shell to scan for go:generate.
                var execMod = prSplit._modules.exec;
                var isWindows2 = prSplit._isWindows;
                if (execMod) {
                    var scanCmd = (isWindows2 && isWindows2())
                        ? 'findstr /s /m "go:generate" *.go'
                        : 'grep -rl "//go:generate" --include="*.go" . 2>/dev/null | head -1';
                    var cdPrefix = (isWindows2 && isWindows2()) ? 'cd /d ' : 'cd ';
                    var goGen = await strategyCommands(options).shell(cdPrefix + shellQuote(dir) + ' && ' + scanCmd);
                    return goGen.code === 0 && (goGen.stdout || '').trim() !== '';
                }
                return false;
            },
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var shellExecAsync = strategyCommands(options).shell;
                var gitExecAsync = strategyCommands(options).git;
                var shellQuote = prSplit._shellQuote;
                var gitAddChangedFilesAsync = strategyCommands(options).stage;
                var osmod = prSplit._modules.osmod;
                var cdPrefix = (prSplit._isWindows && prSplit._isWindows())
                    ? 'cd /d ' + shellQuote(dir) + ' && '
                    : 'cd ' + shellQuote(dir) + ' && ';
                // Determine if Makefile has a 'generate:' target.
                var hasMakeTarget = false;
                var makPath = (dir !== '.' ? dir + '/' : '') + 'Makefile';
                if (osmod && typeof osmod.readFile === 'function') {
                    try {
                        var readResult2 = await osmod.readFile(makPath);
                        if (!readResult2.error) {
                            hasMakeTarget = readResult2.content.indexOf('\ngenerate:') >= 0 || readResult2.content.indexOf('generate:') === 0;
                        }
                    } catch (e) { /* fall through */ }
                } else {
                    var grepFixCmd = (prSplit._isWindows && prSplit._isWindows())
                        ? cdPrefix + 'findstr /b "generate:" Makefile'
                        : cdPrefix + 'grep -q "^generate:" Makefile 2>/dev/null';
                    hasMakeTarget = (await shellExecAsync(grepFixCmd, shellExecOptions(options && options.verifyTimeoutMs))).code === 0;
                }
                var result;
                if (hasMakeTarget) {
                    result = await shellExecAsync(cdPrefix + 'make generate', shellExecOptions(options && options.verifyTimeoutMs));
                } else {
                    result = await shellExecAsync(cdPrefix + 'go generate ./...', shellExecOptions(options && options.verifyTimeoutMs));
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
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var gitExecAsync = strategyCommands(options).git;
                var gitAddChangedFilesAsync = strategyCommands(options).stage;
                if (!plan || !plan.sourceBranch) {
                    return { fixed: false, error: 'no source branch to pull files from' };
                }
                var diffFiles = await gitExecAsync(dir, ['diff', '--name-only', failedBranch, plan.sourceBranch]);
                if (diffFiles.code !== 0 || diffFiles.stdout.trim() === '') {
                    return { fixed: false, error: 'no candidate files to add' };
                }
                var candidates = diffFiles.stdout.trim().split('\n');
                var added = 0;
                // Batch checkout candidates in one command to avoid per-file command churn.
                var batchCo = await gitExecAsync(dir, ['checkout', plan.sourceBranch, '--'].concat(candidates));
                if (batchCo.code === 0) {
                    added = candidates.length;
                } else {
                    // Fall back to per-file checkout if batch failed.
                    for (var f = 0; f < candidates.length; f++) {
                        var co = await gitExecAsync(dir, ['checkout', plan.sourceBranch, '--', candidates[f]]);
                        if (co.code === 0) {
                            added++;
                        }
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
            name: 'agent-fix',
            // Late-bound: agentExecutor set by pipeline chunk (10)
            detect: function() {
                var agentExecutor = prSplit._agentExecutor;
                if (!agentExecutor || !agentExecutor.handle) return false;
                // handle is set only after resolve()/resolveAsync() succeeds
                // (chunk 09) and is cleared together with `resolved` (in
                // close()/kill(), handle is nulled first), so handle != null
                // implies resolved != null. With resolved set, isAvailable()
                // short-circuits to true (chunk 09: `if (this.resolved) return
                // true`), so the `resolved` proxy below is exactly equivalent
                // to awaiting isAvailable() — without making detect async and
                // without risking the sync resolve() path on an unresolved
                // executor. Net effect: agent-fix is enabled precisely when an
                // agent is actually running, never silently skipped. Sync mocks
                // (tests) return a non-Promise from isAvailable() and take the
                // !!avail path.
                var avail = agentExecutor.isAvailable();
                if (avail && typeof avail.then === 'function') {
                    return !!agentExecutor.resolved;
                }
                return !!avail;
            },
            // Async: sendToHandle uses setTimeout delay for PTY write separation
            fix: async function(dir, failedBranch, plan, verifyOutput, options) {
                var agentExecutor = prSplit._agentExecutor;
                var renderConflictPrompt = prSplit.renderConflictPrompt;
                var sendToHandle = prSplit.sendToHandle;
                var waitForLogged = prSplit.waitForLogged;
                var mcpCallbackObj = prSplit._mcpCallbackObj;
                var validateResolution = prSplit.validateResolution;
                var gitExecAsync = strategyCommands(options).git;
                var gitAddChangedFilesAsync = strategyCommands(options).stage;
                var shellQuote = prSplit._shellQuote;
                var shellExecAsync = strategyCommands(options).shell;
                var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS || {};

                if (!agentExecutor || !agentExecutor.handle) {
                    return { fixed: false, error: 'Agent executor not available' };
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
                    sessionId: agentExecutor.sessionId || ''
                });
                if (promptResult.error) {
                    return { fixed: false, error: 'render conflict prompt failed: ' + promptResult.error };
                }
                var sendResult = await sendToHandle(agentExecutor.handle, promptResult.text);
                if (sendResult.error) {
                    return { fixed: false, error: 'failed to send to Agent: ' + sendResult.error };
                }
                mcpCallbackObj.resetWaiter('reportResolution');
                var resolutionPoll = await waitForLogged('reportResolution', resolveTimeoutMs, {
                    aliveCheck: aliveCheckFn,
                    checkIntervalMs: pollIntervalMs
                });
                if (resolutionPoll.error) {
                    return { fixed: false, error: 'Agent resolution timed out waiting for reportResolution tool call: ' + resolutionPoll.error + ' — check Agent process logs for errors' };
                }
                var resolution = resolutionPoll.data;
                var resVal = validateResolution(resolution);
                if (!resVal.valid) {
                    log.printf('auto-split: resolution validation errors: %s', resVal.errors.join('; '));
                    return { fixed: false, error: 'invalid resolution: ' + resVal.errors.join('; ') };
                }
                var patchResult = await prSplit._applyResolutionPatches(resolution, dir);
                if (patchResult.error) {
                    return { fixed: false, error: 'failed to apply resolution patches: ' + patchResult.error };
                }
                if (resolution.commands && resolution.commands.length > 0) {
                    for (var c = 0; c < resolution.commands.length; c++) {
                        // Run commands in the worktree directory (async — T078: does not block event loop).
                        var cdCmd2 = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                        await shellExecAsync(cdCmd2 + shellQuote(dir) + ' && ' + resolution.commands[c], shellExecOptions(options && options.verifyTimeoutMs));
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
        options = options || {};
        var pipelineOwner = options.pipelineOwner;
        var ownsPipelineLease = !prSplit._ownsPipeline(pipelineOwner);
        if (ownsPipelineLease) pipelineOwner = prSplit._claimPipelineOwner();
        if (!pipelineOwner) {
            return {fixed:[], errors:[], reSplitNeeded:false, totalRetries:0, reSplitFiles:[], reSplitReason:'',
                error:'A pipeline is still running; wait for verification, repair, and cleanup to finish.', executionPending:true};
        }
        try {
            var blocked = prSplit._executionCleanupFailure(options.completedResults, options.completedOutcome);
            if (blocked) return Object.assign({}, blocked.execution, {
                fixed:[], errors:[{name:'(cleanup)', error:blocked.error}], reSplitNeeded:false,
                totalRetries:0, reSplitFiles:[], reSplitReason:'', error:blocked.error
            });
            var resolveDir = prSplit._resolveDir;
            var shellQuote = prSplit._shellQuote;
            var isCancelled = function() {
                return prSplit.isCancelled() || prSplit.isForceCancelled() ||
                    (typeof options.isCancelled === 'function' && options.isCancelled());
            };
            var runtime = prSplit.runtime;
            var AUTOMATED_DEFAULTS = prSplit.AUTOMATED_DEFAULTS || {};

            if (!plan || !plan.splits) {
                return { fixed: [], errors: [{ name: '(plan)', error: 'invalid plan: missing splits' }], reSplitNeeded: false, totalRetries: 0, reSplitFiles: [], reSplitReason: '' };
            }
            var dir = resolveDir(plan.dir || '.');
            var verifyCommand = options.verifyCommand || plan.verifyCommand || runtime.verifyCommand;
            var retryBudget = typeof options.retryBudget === 'number' ? options.retryBudget : (typeof runtime.retryBudget === 'number' ? runtime.retryBudget : 3);
            var perBranchRetryBudget = typeof options.perBranchRetryBudget === 'number' ? options.perBranchRetryBudget : 2;
            var strategies = options.strategies || AUTO_FIX_STRATEGIES;
            var totalRetries = 0;
            var branchRetries = {};

            var verifyTimeoutMs = (typeof options.verifyTimeoutMs === 'number' && options.verifyTimeoutMs > 0)
                ? options.verifyTimeoutMs
                : AUTOMATED_DEFAULTS.verifyTimeoutMs;
            var strategyOptions = {
                verifyTimeoutMs: verifyTimeoutMs,
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
                if (isCancelled()) {
                    return {
                        fixed: fixed, errors: errorsOut,
                        totalRetries: totalRetries, branchRetries: branchRetries,
                        reSplitNeeded: reSplitNeeded, reSplitFiles: reSplitFiles,
                        reSplitReason: '', cancelledByUser: true, cancelled: true
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
                var owner = resolutionWorktree(dir, worktreeDir, split.name, isCancelled);
                var branchOutcome = {name:split.name};
                var fixedStrategy = null;
                try {
                    if (typeof options.worktreeFn === 'function') await options.worktreeFn(worktreeDir, split.name);
                    await owner.open();
                    var cdCmd = (prSplit._isWindows && prSplit._isWindows()) ? 'cd /d ' : 'cd ';
                    var verifyResult = await owner.shell(
                        cdCmd + shellQuote(worktreeDir) + ' && ' + verifyCommand,
                        shellExecOptions(verifyTimeoutMs)
                    );
                    branchOutcome.exitCode = verifyResult.code;
                    if (verifyResult.code !== 0) {
                        var verifyOutput = (verifyResult.stdout || '') + '\n' + (verifyResult.stderr || '');
                        var resolved = false;
                        var ownedStrategyOptions = Object.assign({}, strategyOptions, {commands:owner});
                        while (branchRetries[split.name] < perBranchRetryBudget && totalRetries < retryBudget && !resolved) {
                            if (Date.now() >= deadline || isCancelled()) break;
                            var madeProgress = false;
                            for (var strategyIndex = 0; strategyIndex < strategies.length && branchRetries[split.name] < perBranchRetryBudget && totalRetries < retryBudget; strategyIndex++) {
                                if (Date.now() >= deadline || isCancelled()) break;
                                var strategy = strategies[strategyIndex];
                                if (!(await strategy.detect(worktreeDir, verifyOutput, ownedStrategyOptions))) continue;
                                if (isCancelled()) break;
                                totalRetries++;
                                branchRetries[split.name]++;
                                madeProgress = true;
                                var fixResult = await strategy.fix(worktreeDir, split.name, plan, verifyOutput, ownedStrategyOptions);
                                if (fixResult && (fixResult.treeCleanupError || fixResult.truncated || fixResult.execFailed)) {
                                    throw Object.assign(new Error(fixResult.error || 'repair execution failed'), fixResult);
                                }
                                if (!fixResult.fixed) {
                                    log.warn('repair strategy failed', {strategy:strategy.name, branch:split.name, error:fixResult.error});
                                    continue;
                                }
                                var reVerify = await owner.shell(
                                    cdCmd + shellQuote(worktreeDir) + ' && ' + verifyCommand,
                                    shellExecOptions(verifyTimeoutMs)
                                );
                                branchOutcome.exitCode = reVerify.code;
                                if (reVerify.code === 0) {
                                    fixedStrategy = strategy.name;
                                    resolved = true;
                                    break;
                                }
                                verifyOutput = (reVerify.stdout || '') + '\n' + (reVerify.stderr || '');
                                break;
                            }
                            if (!madeProgress) break;
                        }
                        if (!resolved) {
                            branchOutcome.error = 'verification failed after all auto-fix strategies';
                            branchOutcome.lastOutput = verifyOutput;
                            reSplitNeeded = true;
                            reSplitFiles = reSplitFiles.concat(split.files || []);
                        }
                    }
                } catch (error) {
                    Object.assign(branchOutcome, owner.failure(error));
                } finally {
                    await owner.cleanup(branchOutcome);
                }
                if (branchOutcome.error) errorsOut.push(branchOutcome);
                else if (fixedStrategy) fixed.push({name:split.name, strategy:fixedStrategy});
                // An unknown command outcome or failed teardown cannot authorize
                // another repair attempt or a new branch mutation.
                if (branchOutcome.execFailed || branchOutcome.cleanupFailed) break;
            }

            var result = {
                fixed: fixed,
                errors: errorsOut,
                totalRetries: totalRetries,
                branchRetries: branchRetries,
                reSplitNeeded: reSplitNeeded,
                reSplitFiles: reSplitFiles,
                reSplitReason: reSplitNeeded ? 'Auto-fix strategies exhausted for: ' + errorsOut.map(function(e) { return e.name; }).join(', ') : ''
            };
            var fatal = errorsOut.filter(function(error) { return error.execFailed || error.cleanupFailed; })[0];
            if (fatal) {
                result.error = fatal.error;
                result.reSplitNeeded = false;
                for (var key of ['execFailed', 'cleanupFailed', 'worktreeDir', 'worktreeRetained', 'treeCleanupError', 'truncated', 'timedOut', 'cancelled']) {
                    if (fatal[key] !== undefined) result[key] = fatal[key];
                }
            }
            return result;
        } finally {
            if (ownsPipelineLease) prSplit._releasePipelineOwner(pipelineOwner);
        }
    };

    prSplit.AUTO_FIX_STRATEGIES = AUTO_FIX_STRATEGIES;
    prSplit._resolutionWorktree = resolutionWorktree;

})(globalThis.prSplit);
