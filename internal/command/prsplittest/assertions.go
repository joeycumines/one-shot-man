package prsplittest

// NumVal safely extracts a numeric value from Goja results which may return
// int64 or float64 depending on the JS value. Returns float64 for comparison.
func NumVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

// ChunkCompatShim is a JavaScript snippet that, when evaluated after loading
// all chunks (00-16f), re-exports the monolith's formerly-global symbols onto
// globalThis. This lets satellite test files (written for the monolith's
// flat namespace) run unchanged against the chunked architecture.
//
// For functions: Object.defineProperty with get/set proxies so that
// executeSplit = function() {...} transparently updates prSplit.executeSplit.
//
// For state vars: same get/set proxy pointing at prSplit._state.
// For modules: Object.defineProperty proxies so that test overrides
// like exec = newProxy propagate to prSplit._modules.exec.
const ChunkCompatShim = `
(function() {
    var ps = globalThis.prSplit;
    if (!ps) return;
    var st = ps._state || {};
    var mods = ps._modules || {};

    // --- Module proxies (get/set → prSplit._modules.*) ---
    var modNames = ['bt', 'exec', 'osmod', 'template', 'shared', 'lip'];
    modNames.forEach(function(m) {
        if (!mods[m]) return;
        try {
            Object.defineProperty(globalThis, m, {
                get: function() { return mods[m]; },
                set: function(v) { mods[m] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Function proxies (get/set → prSplit.*) ---
    var funcNames = [
        'analyzeDiff', 'analyzeDiffStats',
        'groupByDirectory', 'groupByExtension', 'groupByPattern',
        'groupByChunks', 'groupByDependency', 'applyStrategy', 'selectStrategy',
        'parseGoImports', 'detectGoModulePath',
        'createSplitPlan', 'savePlan', 'loadPlan',
        'validateClassification', 'validatePlan', 'validateSplitPlan', 'validateResolution',
        'executeSplit',
        'verifySplit', 'verifySplits', 'verifyEquivalence', 'verifyEquivalenceDetailed',
        'cleanupBranches',
        'createPRs',
        'resolveConflicts',
        'ClaudeCodeExecutor',
        'renderClassificationPrompt', 'renderSplitPlanPrompt', 'renderConflictPrompt',
        'renderPrompt',
        'detectLanguage',
        'automatedSplit', 'heuristicFallback', 'sendToHandle', 'waitForLogged',
        'classificationToGroups',
        'assessIndependence', 'splitsAreIndependent', 'splitsAreIndependentFromMaps',
        'recordConversation', 'getConversationHistory',
        'recordTelemetry', 'getTelemetrySummary', 'saveTelemetry',
        'renderColorizedDiff', 'getSplitDiff',
        'buildDependencyGraph', 'renderAsciiGraph',
        'analyzeRetrospective',
        'cleanupExecutor',
        'analyzeDiffAsync', 'createSplitPlanAsync', 'executeSplitAsync',
        'verifySplitAsync', 'verifySplitsAsync', 'verifyEquivalenceAsync',
        'cleanupBranchesAsync'
    ];

    funcNames.forEach(function(k) {
        if (typeof ps[k] === 'undefined') return;
        try {
            Object.defineProperty(globalThis, k, {
                get: function() { return ps[k]; },
                set: function(v) { ps[k] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Internal helpers with _ prefix (monolith had bare names) ---
    var internalNames = {
        'gitExec':           '_gitExec',
        'shellQuote':        '_shellQuote',
        'gitAddChangedFiles':'_gitAddChangedFiles',
        'dirname':           '_dirname',
        'fileExtension':     '_fileExtension',
        'sanitizeBranchName':'_sanitizeBranchName',
        'padIndex':          '_padIndex',
        'isCancelled':       'isCancelled',
        'isPaused':          'isPaused',
        'isForceCancelled':  'isForceCancelled'
    };
    Object.keys(internalNames).forEach(function(bare) {
        var real = internalNames[bare];
        if (typeof ps[real] === 'undefined') return;
        try {
            Object.defineProperty(globalThis, bare, {
                get: function() { return ps[real]; },
                set: function(v) { ps[real] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- Constants ---
    if (ps.AUTOMATED_DEFAULTS) globalThis.AUTOMATED_DEFAULTS = ps.AUTOMATED_DEFAULTS;
    if (ps.AUTO_FIX_STRATEGIES) globalThis.AUTO_FIX_STRATEGIES = ps.AUTO_FIX_STRATEGIES;
    if (ps.DEFAULT_PLAN_PATH) globalThis.DEFAULT_PLAN_PATH = ps.DEFAULT_PLAN_PATH;
    if (ps.CLASSIFICATION_PROMPT_TEMPLATE) globalThis.CLASSIFICATION_PROMPT_TEMPLATE = ps.CLASSIFICATION_PROMPT_TEMPLATE;
    if (ps.SPLIT_PLAN_PROMPT_TEMPLATE) globalThis.SPLIT_PLAN_PROMPT_TEMPLATE = ps.SPLIT_PLAN_PROMPT_TEMPLATE;
    if (ps.CONFLICT_RESOLUTION_PROMPT_TEMPLATE) globalThis.CONFLICT_RESOLUTION_PROMPT_TEMPLATE = ps.CONFLICT_RESOLUTION_PROMPT_TEMPLATE;

    // --- runtime proxy (bare global → prSplit.runtime) ---
    try {
        Object.defineProperty(globalThis, 'runtime', {
            get: function() { return ps.runtime; },
            set: function(v) { ps.runtime = v; },
            configurable: true,
            enumerable: false
        });
    } catch(e) {}

    // --- State variable proxies (get/set → prSplit._state.*) ---
    var stateNames = [
        'analysisCache', 'groupsCache', 'planCache',
        'executionResultCache', 'conversationHistory',
        'claudeExecutor', 'mcpCallbackObj'
    ];
    stateNames.forEach(function(k) {
        try {
            Object.defineProperty(globalThis, k, {
                get: function() { return st[k]; },
                set: function(v) { st[k] = v; },
                configurable: true,
                enumerable: false
            });
        } catch(e) {}
    });

    // --- _mcpCallbackObj bridge ---
    try {
        Object.defineProperty(ps, '_mcpCallbackObj', {
            get: function() { return st.mcpCallbackObj; },
            set: function(v) { st.mcpCallbackObj = v; },
            configurable: true
        });
    } catch(e) {}

    // --- _extract* aliases ---
    if (ps.extractDirs) ps._extractDirs = ps.extractDirs;
    if (ps.extractGoPkgs) ps._extractGoPkgs = ps.extractGoPkgs;
    if (ps.extractGoImports) ps._extractGoImports = ps.extractGoImports;

    // --- verify helpers ---
    if (ps.discoverVerifyCommand) globalThis.discoverVerifyCommand = ps.discoverVerifyCommand;
    if (ps.scopedVerifyCommand) globalThis.scopedVerifyCommand = ps.scopedVerifyCommand;
})();
`

// GitMockSetupJS returns JS code that installs a git-focused exec mock.
// The mock matches git subcommands by stripping 'git' and optional '-C dir'
// prefixes, then looking up responses by the remaining args joined with space.
//
// Mock state globals:
//
//	_gitCalls      — array of {argv:[...]} records
//	_gitResponses  — map of subcommand key → response object
//	_gitOk(stdout) — helper: success response
//	_gitFail(stderr) — helper: failure response
//
// Non-git commands (e.g. sh -c from verifySplit) are matched via a
// "!sh" prefix key in _gitResponses.
func GitMockSetupJS() string {
	return `(function() {
    var execMod = require('osm:exec');
    globalThis._gitCalls = [];
    globalThis._gitResponses = {};

    function ok(stdout) {
        return {stdout: stdout || '', stderr: '', code: 0, error: false, message: ''};
    }
    function fail(stderr) {
        return {stdout: '', stderr: stderr || 'error', code: 1, error: true, message: stderr || 'error'};
    }
    globalThis._gitOk = ok;
    globalThis._gitFail = fail;

    execMod.execv = function(argv) {
        globalThis._gitCalls.push({argv: argv.slice()});

        // 'test' command: check _testFileExists set for T24 tests.
        if (argv[0] === 'test' && argv[1] === '-f') {
            var path = argv[2] || '';
            if (globalThis._testFileExists && globalThis._testFileExists[path]) {
                return ok('');
            }
            return fail('');
        }

        // Non-git commands: route via '!' + argv[0] key (e.g. '!sh', '!which').
        if (argv[0] !== 'git') {
            var nonGitKey = '!' + argv[0];
            if (globalThis._gitResponses[nonGitKey] !== undefined) {
                var r = globalThis._gitResponses[nonGitKey];
                if (typeof r === 'function') return r(argv);
                return r;
            }
            return ok('');
        }

        // Strip 'git' and optional '-C dir'.
        var args = argv.slice(1);
        if (args.length >= 2 && args[0] === '-C') args = args.slice(2);

        // Exact match.
        var key = args.join(' ');
        if (globalThis._gitResponses[key] !== undefined) {
            var r = globalThis._gitResponses[key];
            if (typeof r === 'function') return r(argv);
            return r;
        }

        // Prefix matching (longest first).
        for (var i = args.length - 1; i >= 1; i--) {
            var prefix = args.slice(0, i).join(' ');
            if (globalThis._gitResponses[prefix] !== undefined) {
                var r = globalThis._gitResponses[prefix];
                if (typeof r === 'function') return r(argv);
                return r;
            }
        }

        // Default: success with empty output.
        return ok('');
    };

    // execStream delegates to the execv mock but adapts the interface.
    execMod.execStream = function(argv, opts) {
        var r = execMod.execv(argv);
        opts = opts || {};
        if (opts.onStdout && r.stdout) opts.onStdout(r.stdout);
        if (opts.onStderr && r.stderr) opts.onStderr(r.stderr);
        return {code: r.code, error: r.error, message: r.message};
    };

    // Mock exec.spawn to route through the same mock dispatcher as execv.
    execMod.spawn = function(cmd, args) {
        var fullArgv = [cmd].concat(args || []);
        var r = execMod.execv(fullArgv);
        var stdoutRead = false;
        var stderrRead = false;
        return {
            stdout: {
                read: function() {
                    if (!stdoutRead) {
                        stdoutRead = true;
                        if (r.stdout) return Promise.resolve({done: false, value: r.stdout});
                    }
                    return Promise.resolve({done: true});
                }
            },
            stderr: {
                read: function() {
                    if (!stderrRead) {
                        stderrRead = true;
                        if (r.stderr) return Promise.resolve({done: false, value: r.stderr});
                    }
                    return Promise.resolve({done: true});
                }
            },
            wait: function() { return Promise.resolve({code: r.code}); },
            kill: function() {}
        };
    };
})();`
}
