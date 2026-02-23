// pr_split_script.js — Built-in PR splitting command.
// Consolidates orchestrate-pr-split.js and bt-templates/claude-mux.js into
// a single embedded script exposed as `osm pr-split`.
//
// This script is loaded by the Go command via //go:embed; it receives injected
// globals from the Go side:
//   prSplitConfig  — {baseBranch, strategy, maxFiles, branchPrefix, verifyCommand, dryRun, aiMode, provider, model}
//   prSplitTemplate — Markdown template for plan rendering
//   config.name    — "pr-split"
//   args           — CLI positional args
//   output         — output manager (print, clipboard)
//   ctx            — context manager (run, toTxtar)
//   tui            — TUI manager (registerMode, switchMode, createState)
//   log            — logger (debug, info, warn, error)

'use strict';

var bt = require('osm:bt');
var exec = require('osm:exec');

// TUI-only modules: loaded conditionally so that the script can also be
// require()'d from test environments that lack these modules.
var template, shared;
try {
    template = require('osm:text/template');
    shared = require('osm:sharedStateSymbols');
} catch (e) {
    template = null;
    shared = null;
}

// Optional: claudemux for AI-powered classification and planning.
var claudemux;
try {
    claudemux = require('osm:claudemux');
} catch (e) {
    claudemux = null;
}

// Read injected configuration with defaults.
var cfg = (typeof prSplitConfig !== 'undefined') ? prSplitConfig : {};
var COMMAND_NAME = (typeof config !== 'undefined' && config.name) ? config.name : 'pr-split';
var MODE_NAME = 'pr-split';

// Mutable runtime state — can be changed via TUI commands.
var runtime = {
    baseBranch:    cfg.baseBranch    || 'main',
    strategy:      cfg.strategy      || 'directory',
    maxFiles:      cfg.maxFiles      || 10,
    branchPrefix:  cfg.branchPrefix  || 'split/',
    verifyCommand: cfg.verifyCommand || 'make test',
    dryRun:        cfg.dryRun        || false,
    aiMode:        cfg.aiMode        || false,
    provider:      cfg.provider      || 'ollama',
    model:         cfg.model         || ''
};

// ---------------------------------------------------------------------------
//  Internal Helpers
// ---------------------------------------------------------------------------

function gitExec(dir, args) {
    var cmd = ['git'];
    if (dir && dir !== '' && dir !== '.') {
        cmd.push('-C');
        cmd.push(dir);
    }
    for (var i = 0; i < args.length; i++) {
        cmd.push(args[i]);
    }
    return exec.execv(cmd);
}

function shellQuote(s) {
    return "'" + s.replace(/'/g, "'\\''") + "'";
}

function dirname(filepath, depth) {
    depth = depth || 1;
    var parts = filepath.split('/');
    if (parts.length <= 1) {
        return '.';
    }
    var taken = parts.slice(0, Math.min(depth, parts.length - 1));
    return taken.join('/');
}

function fileExtension(filepath) {
    var base = filepath.split('/').pop();
    var dot = base.lastIndexOf('.');
    if (dot <= 0) {
        return '';
    }
    return base.substring(dot);
}

function sanitizeBranchName(name) {
    return name.replace(/[^a-zA-Z0-9_/-]/g, '-');
}

// padIndex pads a number to at least 2 digits: 1 → '01', 12 → '12'.
function padIndex(n) {
    var s = String(n);
    while (s.length < 2) {
        s = '0' + s;
    }
    return s;
}

// ---------------------------------------------------------------------------
//  Analysis
// ---------------------------------------------------------------------------

function analyzeDiff(config) {
    config = config || {};
    var baseBranch = config.baseBranch || runtime.baseBranch;
    var dir = config.dir || '.';

    var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    if (branchResult.code !== 0) {
        return {
            files: [],
            error: 'failed to get current branch: ' + branchResult.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: ''
        };
    }
    var currentBranch = branchResult.stdout.trim();

    var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
    if (mergeBase.code !== 0) {
        return {
            files: [],
            error: 'merge-base failed: ' + mergeBase.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    var diffResult = gitExec(dir, ['diff', '--name-only', mergeBase.stdout.trim(), currentBranch]);
    if (diffResult.code !== 0) {
        return {
            files: [],
            error: 'git diff failed: ' + diffResult.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    var raw = diffResult.stdout.trim();
    var files = raw === '' ? [] : raw.split('\n').filter(function(f) {
        return f !== '';
    });

    return {
        files: files,
        error: null,
        baseBranch: baseBranch,
        currentBranch: currentBranch
    };
}

function analyzeDiffStats(config) {
    config = config || {};
    var baseBranch = config.baseBranch || runtime.baseBranch;
    var dir = config.dir || '.';

    var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    if (branchResult.code !== 0) {
        return {
            files: [],
            error: 'failed to get current branch: ' + branchResult.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: ''
        };
    }
    var currentBranch = branchResult.stdout.trim();

    var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
    if (mergeBase.code !== 0) {
        return {
            files: [],
            error: 'merge-base failed: ' + mergeBase.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    var statResult = gitExec(dir, ['diff', '--numstat', mergeBase.stdout.trim(), currentBranch]);
    if (statResult.code !== 0) {
        return {
            files: [],
            error: 'git diff --numstat failed: ' + statResult.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    var files = [];
    var raw = statResult.stdout.trim();
    if (raw !== '') {
        var lines = raw.split('\n');
        for (var i = 0; i < lines.length; i++) {
            if (lines[i] === '') continue;
            var parts = lines[i].split('\t');
            if (parts.length >= 3) {
                files.push({
                    name: parts[2],
                    additions: parseInt(parts[0], 10) || 0,
                    deletions: parseInt(parts[1], 10) || 0
                });
            }
        }
    }

    return {
        files: files,
        error: null,
        baseBranch: baseBranch,
        currentBranch: currentBranch
    };
}

// ---------------------------------------------------------------------------
//  Grouping Strategies
// ---------------------------------------------------------------------------

function groupByDirectory(files, depth) {
    depth = depth || 1;
    var groups = {};
    for (var i = 0; i < files.length; i++) {
        var dir = dirname(files[i], depth);
        if (!groups[dir]) {
            groups[dir] = [];
        }
        groups[dir].push(files[i]);
    }
    return groups;
}

function groupByExtension(files) {
    var groups = {};
    for (var i = 0; i < files.length; i++) {
        var ext = fileExtension(files[i]) || '(none)';
        if (!groups[ext]) {
            groups[ext] = [];
        }
        groups[ext].push(files[i]);
    }
    return groups;
}

function groupByPattern(files, patterns) {
    var groups = {};
    var patternNames = Object.keys(patterns);

    for (var i = 0; i < files.length; i++) {
        var matched = false;
        for (var j = 0; j < patternNames.length; j++) {
            var name = patternNames[j];
            if (patterns[name].test(files[i])) {
                if (!groups[name]) {
                    groups[name] = [];
                }
                groups[name].push(files[i]);
                matched = true;
                break;
            }
        }
        if (!matched) {
            if (!groups['(other)']) {
                groups['(other)'] = [];
            }
            groups['(other)'].push(files[i]);
        }
    }
    return groups;
}

function groupByChunks(files, maxPerGroup) {
    maxPerGroup = maxPerGroup || 5;
    var groups = {};
    for (var i = 0; i < files.length; i++) {
        var chunkIdx = Math.floor(i / maxPerGroup) + 1;
        var name = 'chunk-' + chunkIdx;
        if (!groups[name]) {
            groups[name] = [];
        }
        groups[name].push(files[i]);
    }
    return groups;
}

// applyStrategy selects and applies a grouping strategy.
function applyStrategy(files, strategy, options) {
    options = options || {};
    switch (strategy) {
        case 'directory':
            return groupByDirectory(files, options.depth || 1);
        case 'directory-deep':
            return groupByDirectory(files, options.depth || 2);
        case 'extension':
            return groupByExtension(files);
        case 'chunks':
            return groupByChunks(files, options.maxPerGroup || runtime.maxFiles);
        case 'auto':
            return selectStrategy(files, options).groups;
        default:
            return groupByDirectory(files, 1);
    }
}

// ---------------------------------------------------------------------------
//  Strategy Selection (ChoiceResolver)
// ---------------------------------------------------------------------------

function selectStrategy(files, options) {
    options = options || {};
    var maxPerGroup = options.maxPerGroup || runtime.maxFiles;

    var strategies = [
        { name: 'directory', groups: groupByDirectory(files, 1) },
        { name: 'directory-deep', groups: groupByDirectory(files, 2) },
        { name: 'extension', groups: groupByExtension(files) },
        { name: 'chunks', groups: groupByChunks(files, maxPerGroup) }
    ];

    if (!claudemux) {
        return {
            strategy: 'directory',
            groups: strategies[0].groups,
            reason: 'claudemux not available, using default directory strategy',
            needsConfirm: false,
            scored: []
        };
    }

    var candidates = [];
    for (var i = 0; i < strategies.length; i++) {
        var s = strategies[i];
        var groupNames = Object.keys(s.groups);
        var totalFiles = 0;
        var maxGroupSize = 0;
        for (var j = 0; j < groupNames.length; j++) {
            var gsize = s.groups[groupNames[j]].length;
            totalFiles += gsize;
            if (gsize > maxGroupSize) maxGroupSize = gsize;
        }
        var avgGroupSize = groupNames.length > 0 ? totalFiles / groupNames.length : 0;
        var balance = groupNames.length > 0
            ? 1 - Math.abs(maxGroupSize - avgGroupSize) / Math.max(maxGroupSize, 1)
            : 0;

        candidates.push({
            id: s.name,
            name: s.name,
            description: groupNames.length + ' groups, max ' + maxGroupSize + ' files',
            attributes: {
                groupCount: String(groupNames.length),
                maxGroupSize: String(maxGroupSize),
                avgGroupSize: String(Math.round(avgGroupSize * 100) / 100),
                balance: String(Math.round(balance * 100) / 100)
            }
        });
    }

    var resolver = claudemux.newChoiceResolver({
        minCandidates: 2,
        confirmThreshold: 0.15
    });

    var criteria = [
        { name: 'splitCount', weight: 0.4 },
        { name: 'groupBalance', weight: 0.3 },
        { name: 'maxSize', weight: 0.3 }
    ];

    var result = resolver.analyze(candidates, criteria, function(cand, crit) {
        switch (crit.name) {
            case 'splitCount':
                var n = parseInt(cand.attributes.groupCount, 10) || 0;
                if (n <= 0) return 0;
                if (n >= 3 && n <= 7) return 1.0;
                if (n < 3) return n / 3;
                return Math.max(0, 1.0 - (n - 7) * 0.1);
            case 'groupBalance':
                return parseFloat(cand.attributes.balance) || 0;
            case 'maxSize':
                var mx = parseInt(cand.attributes.maxGroupSize, 10) || 0;
                if (mx <= maxPerGroup) return 1.0;
                return Math.max(0, 1.0 - (mx - maxPerGroup) * 0.05);
            default:
                return 0.5;
        }
    });

    var winnerName = result.rankings[0].name;
    var winnerGroups = null;
    for (var k = 0; k < strategies.length; k++) {
        if (strategies[k].name === winnerName) {
            winnerGroups = strategies[k].groups;
            break;
        }
    }

    return {
        strategy: winnerName,
        groups: winnerGroups,
        reason: result.justification,
        needsConfirm: result.needsConfirm,
        scored: result.rankings
    };
}

// ---------------------------------------------------------------------------
//  ClaudeMux AI Integration
// ---------------------------------------------------------------------------

function classifyChangesWithClaudeMux(files, options) {
    if (!claudemux) {
        return { files: {}, error: 'claudemux module not available' };
    }
    if (!options || !options.registry) {
        return { files: {}, error: 'registry is required in options' };
    }
    if (!files || files.length === 0) {
        return { files: {}, error: 'no files to classify' };
    }

    var sessionId = options.sessionId || ('classify-' + Date.now());
    var providerName = options.providerName || 'claude-code';
    var maxWaitMs = options.maxWaitMs || 120000;

    var mcpInst = claudemux.newMCPInstance(sessionId);
    var resultDir;
    try {
        resultDir = mcpInst.configDir();
        mcpInst.setResultDir(resultDir);
        mcpInst.writeConfigFile();
    } catch (e) {
        mcpInst.close();
        return { files: {}, error: 'MCP setup failed: ' + e };
    }

    var fileList = files.join('\n');
    var prompt = 'You are a PR splitting assistant. Classify each of the following changed files into a logical category.\n' +
        'Use the MCP tool "reportClassification" to report your results.\n' +
        'Categories should be concise labels like: types, impl, tests, docs, config, deps, refactor.\n' +
        'Each file must be assigned exactly one category.\n\n' +
        'Changed files:\n' + fileList + '\n\n' +
        'Call the reportClassification tool with sessionId "' + sessionId + '" and a files map.';

    var spawnOpts = options.spawnOpts || {};
    spawnOpts.args = (spawnOpts.args || []).concat(mcpInst.spawnArgs());

    var handle;
    try {
        handle = options.registry.spawn(providerName, spawnOpts);
    } catch (e) {
        mcpInst.close();
        return { files: {}, error: 'spawn failed: ' + e };
    }

    var parser = claudemux.newParser();
    try {
        handle.send(prompt + '\n');

        var startTime = Date.now();
        var emptyCount = 0;
        var maxEmpty = 200;

        while (handle.isAlive() && emptyCount < maxEmpty) {
            if (Date.now() - startTime > maxWaitMs) {
                break;
            }
            var data = handle.receive();
            if (data === '') {
                emptyCount++;
                continue;
            }
            emptyCount = 0;
            var lines = data.split('\n');
            for (var i = 0; i < lines.length; i++) {
                if (lines[i] === '') continue;
                var event = parser.parse(lines[i]);
                if (event.type === claudemux.EVENT_COMPLETION) {
                    try {
                        var result = claudemux.readClassificationResult(resultDir);
                        mcpInst.close();
                        return { files: result, error: null };
                    } catch (readErr) {
                        mcpInst.close();
                        return { files: {}, error: 'result read failed: ' + readErr };
                    }
                }
                if (event.type === claudemux.EVENT_PERMISSION) {
                    try { handle.send('n\n'); } catch (e2) { /* ignore */ }
                }
            }
        }

        try {
            var lateResult = claudemux.readClassificationResult(resultDir);
            mcpInst.close();
            return { files: lateResult, error: null };
        } catch (e3) {
            mcpInst.close();
            return { files: {}, error: 'timeout or no result: ' + e3 };
        }
    } catch (e4) {
        mcpInst.close();
        return { files: {}, error: 'classification failed: ' + e4 };
    }
}

function suggestSplitPlanWithClaudeMux(files, classification, options) {
    if (!claudemux) {
        return { stages: [], error: 'claudemux module not available' };
    }
    if (!options || !options.registry) {
        return { stages: [], error: 'registry is required in options' };
    }
    if (!files || files.length === 0) {
        return { stages: [], error: 'no files to plan' };
    }

    var sessionId = options.sessionId || ('plan-' + Date.now());
    var providerName = options.providerName || 'claude-code';
    var maxWaitMs = options.maxWaitMs || 120000;

    var mcpInst = claudemux.newMCPInstance(sessionId);
    var resultDir;
    try {
        resultDir = mcpInst.configDir();
        mcpInst.setResultDir(resultDir);
        mcpInst.writeConfigFile();
    } catch (e) {
        mcpInst.close();
        return { stages: [], error: 'MCP setup failed: ' + e };
    }

    var fileList = files.join('\n');
    var classificationContext = '';
    if (classification && Object.keys(classification).length > 0) {
        classificationContext = '\nFile classifications:\n';
        var keys = Object.keys(classification);
        for (var ci = 0; ci < keys.length; ci++) {
            classificationContext += '  ' + keys[ci] + ' -> ' + classification[keys[ci]] + '\n';
        }
    }

    var prompt = 'You are a PR splitting assistant. Create an ordered plan to split the following changes into logical, independently-reviewable PRs.\n' +
        'Use the MCP tool "reportSplitPlan" to report your plan.\n' +
        'Each stage should have: name, files array, commit message, and order (0-based).\n' +
        'Files should NOT overlap between stages. Every file must be assigned to exactly one stage.\n' +
        'Order stages so that dependencies are respected (foundational changes first).\n\n' +
        'Changed files:\n' + fileList + '\n' +
        classificationContext + '\n' +
        'Call the reportSplitPlan tool with sessionId "' + sessionId + '" and your stages array.';

    var spawnOpts = options.spawnOpts || {};
    spawnOpts.args = (spawnOpts.args || []).concat(mcpInst.spawnArgs());

    var handle;
    try {
        handle = options.registry.spawn(providerName, spawnOpts);
    } catch (e) {
        mcpInst.close();
        return { stages: [], error: 'spawn failed: ' + e };
    }

    var parser = claudemux.newParser();
    try {
        handle.send(prompt + '\n');

        var startTime = Date.now();
        var emptyCount = 0;
        var maxEmpty = 200;

        while (handle.isAlive() && emptyCount < maxEmpty) {
            if (Date.now() - startTime > maxWaitMs) {
                break;
            }
            var data = handle.receive();
            if (data === '') {
                emptyCount++;
                continue;
            }
            emptyCount = 0;
            var lines = data.split('\n');
            for (var i = 0; i < lines.length; i++) {
                if (lines[i] === '') continue;
                var event = parser.parse(lines[i]);
                if (event.type === claudemux.EVENT_COMPLETION) {
                    try {
                        var result = claudemux.readSplitPlanResult(resultDir);
                        mcpInst.close();
                        return { stages: result, error: null };
                    } catch (readErr) {
                        mcpInst.close();
                        return { stages: [], error: 'result read failed: ' + readErr };
                    }
                }
                if (event.type === claudemux.EVENT_PERMISSION) {
                    try { handle.send('n\n'); } catch (e2) { /* ignore */ }
                }
            }
        }

        try {
            var lateResult = claudemux.readSplitPlanResult(resultDir);
            mcpInst.close();
            return { stages: lateResult, error: null };
        } catch (e3) {
            mcpInst.close();
            return { stages: [], error: 'timeout or no result: ' + e3 };
        }
    } catch (e4) {
        mcpInst.close();
        return { stages: [], error: 'planning failed: ' + e4 };
    }
}

// ---------------------------------------------------------------------------
//  Planning
// ---------------------------------------------------------------------------

function createSplitPlan(groups, config) {
    config = config || {};
    var dir = config.dir || '.';
    var baseBranch = config.baseBranch || runtime.baseBranch;
    var branchPrefix = config.branchPrefix || runtime.branchPrefix;
    var commitPrefix = config.commitPrefix || '';
    var verifyCommand = config.verifyCommand || runtime.verifyCommand;

    var sourceBranch = config.sourceBranch;
    if (!sourceBranch) {
        var result = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        sourceBranch = result.code === 0 ? result.stdout.trim() : 'HEAD';
    }

    var groupNames = Object.keys(groups).sort();
    var splits = [];

    for (var i = 0; i < groupNames.length; i++) {
        var name = groupNames[i];
        splits.push({
            name: sanitizeBranchName(branchPrefix + padIndex(i + 1) + '-' + name),
            files: groups[name].slice().sort(),
            message: commitPrefix + name,
            order: i
        });
    }

    return {
        baseBranch: baseBranch,
        sourceBranch: sourceBranch,
        dir: dir,
        verifyCommand: verifyCommand,
        splits: splits
    };
}

function validatePlan(plan) {
    var errors = [];

    if (!plan || !plan.splits || plan.splits.length === 0) {
        errors.push('plan has no splits');
        return { valid: false, errors: errors };
    }

    var allFiles = {};
    var duplicates = [];

    for (var i = 0; i < plan.splits.length; i++) {
        var split = plan.splits[i];

        if (!split.name) {
            errors.push('split at index ' + i + ' has no name');
        }

        if (!split.files || split.files.length === 0) {
            errors.push('split ' + (split.name || i) + ' has no files');
        }

        if (split.files) {
            for (var j = 0; j < split.files.length; j++) {
                var f = split.files[j];
                if (allFiles[f]) {
                    duplicates.push(f + ' (in ' + allFiles[f] + ' and ' + split.name + ')');
                } else {
                    allFiles[f] = split.name;
                }
            }
        }
    }

    if (duplicates.length > 0) {
        errors.push('duplicate files: ' + duplicates.join(', '));
    }

    return { valid: errors.length === 0, errors: errors };
}

// ---------------------------------------------------------------------------
//  Execution
// ---------------------------------------------------------------------------

function executeSplit(plan) {
    var dir = plan.dir || '.';
    var results = [];

    var validation = validatePlan(plan);
    if (!validation.valid) {
        return { error: 'invalid plan: ' + validation.errors.join('; '), results: [] };
    }

    var savedBranch = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    if (savedBranch.code !== 0) {
        return { error: 'failed to get current branch', results: [] };
    }
    var restoreBranch = savedBranch.stdout.trim();

    var currentBase = plan.baseBranch;

    for (var i = 0; i < plan.splits.length; i++) {
        var split = plan.splits[i];
        var splitResult = { name: split.name, files: split.files, sha: '', error: null };

        var co = gitExec(dir, ['checkout', currentBase]);
        if (co.code !== 0) {
            splitResult.error = 'checkout ' + currentBase + ' failed: ' + co.stderr.trim();
            results.push(splitResult);
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: splitResult.error, results: results };
        }

        var br = gitExec(dir, ['checkout', '-b', split.name]);
        if (br.code !== 0) {
            splitResult.error = 'branch creation failed: ' + br.stderr.trim();
            results.push(splitResult);
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: splitResult.error, results: results };
        }

        for (var j = 0; j < split.files.length; j++) {
            var file = split.files[j];
            var checkout = gitExec(dir, ['checkout', plan.sourceBranch, '--', file]);
            if (checkout.code !== 0) {
                var stderrLower = checkout.stderr.toLowerCase();
                var errorType = 'checkout';
                if (stderrLower.indexOf('did not match any') !== -1 ||
                    stderrLower.indexOf('pathspec') !== -1) {
                    errorType = 'missing';
                } else if (stderrLower.indexOf('conflict') !== -1 ||
                           stderrLower.indexOf('overwritten') !== -1) {
                    errorType = 'conflict';
                }
                splitResult.error = 'checkout file ' + file + ': ' + checkout.stderr.trim();
                splitResult.errorType = errorType;
                results.push(splitResult);
                gitExec(dir, ['checkout', restoreBranch]);
                return { error: splitResult.error, results: results };
            }
        }

        var add = gitExec(dir, ['add', '-A']);
        if (add.code !== 0) {
            splitResult.error = 'git add failed: ' + add.stderr.trim();
            results.push(splitResult);
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: splitResult.error, results: results };
        }

        var msg = split.message || 'split: ' + split.name;
        var commit = gitExec(dir, ['commit', '-m', msg]);
        if (commit.code !== 0) {
            splitResult.error = 'git commit failed: ' + commit.stderr.trim();
            results.push(splitResult);
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: splitResult.error, results: results };
        }

        var sha = gitExec(dir, ['rev-parse', 'HEAD']);
        splitResult.sha = sha.code === 0 ? sha.stdout.trim() : '';

        results.push(splitResult);
        currentBase = split.name;
    }

    gitExec(dir, ['checkout', restoreBranch]);
    return { error: null, results: results };
}

// ---------------------------------------------------------------------------
//  Verification
// ---------------------------------------------------------------------------

function verifySplit(branchName, config) {
    config = config || {};
    var dir = config.dir || '.';
    var command = config.verifyCommand || runtime.verifyCommand;

    var co = gitExec(dir, ['checkout', branchName]);
    if (co.code !== 0) {
        return {
            name: branchName,
            passed: false,
            output: '',
            error: 'checkout failed: ' + co.stderr.trim()
        };
    }

    var result = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && ' + command]);

    return {
        name: branchName,
        passed: result.code === 0,
        output: result.stdout,
        error: result.code !== 0 ? 'verify failed (exit ' + result.code + '): ' + result.stderr : null
    };
}

function verifySplits(plan) {
    var dir = plan.dir || '.';
    var results = [];
    var allPassed = true;

    var saved = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    var restoreBranch = saved.code === 0 ? saved.stdout.trim() : plan.sourceBranch;

    for (var i = 0; i < plan.splits.length; i++) {
        var result = verifySplit(plan.splits[i].name, {
            dir: dir,
            verifyCommand: plan.verifyCommand
        });
        results.push(result);
        if (!result.passed) {
            allPassed = false;
        }
    }

    gitExec(dir, ['checkout', restoreBranch]);
    return { allPassed: allPassed, results: results };
}

function verifyEquivalence(plan) {
    var dir = plan.dir || '.';

    if (!plan.splits || plan.splits.length === 0) {
        return {
            equivalent: false,
            splitTree: '',
            sourceTree: '',
            error: 'no splits in plan'
        };
    }

    var lastSplit = plan.splits[plan.splits.length - 1].name;

    var splitTreeResult = gitExec(dir, ['rev-parse', lastSplit + '^{tree}']);
    if (splitTreeResult.code !== 0) {
        return {
            equivalent: false,
            splitTree: '',
            sourceTree: '',
            error: 'failed to get split tree: ' + splitTreeResult.stderr.trim()
        };
    }
    var splitTree = splitTreeResult.stdout.trim();

    var sourceTreeResult = gitExec(dir, ['rev-parse', plan.sourceBranch + '^{tree}']);
    if (sourceTreeResult.code !== 0) {
        return {
            equivalent: false,
            splitTree: splitTree,
            sourceTree: '',
            error: 'failed to get source tree: ' + sourceTreeResult.stderr.trim()
        };
    }
    var sourceTree = sourceTreeResult.stdout.trim();

    return {
        equivalent: splitTree === sourceTree,
        splitTree: splitTree,
        sourceTree: sourceTree,
        error: null
    };
}

function verifyEquivalenceDetailed(plan) {
    var dir = plan.dir || '.';
    var base = verifyEquivalence(plan);

    if (base.error || base.equivalent) {
        base.diffFiles = [];
        base.diffSummary = '';
        return base;
    }

    var lastSplit = plan.splits[plan.splits.length - 1].name;
    var diffResult = gitExec(dir, ['diff', '--stat', lastSplit, plan.sourceBranch]);
    base.diffSummary = diffResult.code === 0 ? diffResult.stdout.trim() : '';

    var diffNamesResult = gitExec(dir, ['diff', '--name-only', lastSplit, plan.sourceBranch]);
    if (diffNamesResult.code === 0 && diffNamesResult.stdout.trim() !== '') {
        base.diffFiles = diffNamesResult.stdout.trim().split('\n').filter(function(f) {
            return f !== '';
        });
    } else {
        base.diffFiles = [];
    }

    return base;
}

function cleanupBranches(plan) {
    var dir = plan.dir || '.';
    var deleted = [];
    var errors = [];

    gitExec(dir, ['checkout', plan.baseBranch]);

    for (var i = 0; i < plan.splits.length; i++) {
        var name = plan.splits[i].name;
        var result = gitExec(dir, ['branch', '-D', name]);
        if (result.code === 0) {
            deleted.push(name);
        } else {
            errors.push(name + ': ' + result.stderr.trim());
        }
    }

    return { deleted: deleted, errors: errors };
}

// ---------------------------------------------------------------------------
//  BT Integration — Leaf Nodes
// ---------------------------------------------------------------------------

function createAnalyzeNode(bb, config) {
    return bt.createBlockingLeafNode(function() {
        var result = analyzeDiff(config);
        bb.set('analysisResult', result);
        if (result.error) {
            bb.set('lastError', result.error);
            return bt.failure;
        }
        if (result.files.length === 0) {
            bb.set('lastError', 'no files changed');
            return bt.failure;
        }
        return bt.success;
    });
}

function createGroupNode(bb, strategy, options) {
    options = options || {};
    return bt.createBlockingLeafNode(function() {
        var analysis = bb.get('analysisResult');
        if (!analysis || !analysis.files) {
            bb.set('lastError', 'no analysis result available');
            return bt.failure;
        }
        var groups = applyStrategy(analysis.files, strategy, options);
        bb.set('fileGroups', groups);
        return bt.success;
    });
}

function createPlanNode(bb, config) {
    return bt.createBlockingLeafNode(function() {
        var groups = bb.get('fileGroups');
        if (!groups) {
            bb.set('lastError', 'no file groups available');
            return bt.failure;
        }

        var analysis = bb.get('analysisResult');
        var planConfig = {};
        if (config) {
            var keys = Object.keys(config);
            for (var i = 0; i < keys.length; i++) {
                planConfig[keys[i]] = config[keys[i]];
            }
        }
        if (analysis) {
            if (!planConfig.sourceBranch) planConfig.sourceBranch = analysis.currentBranch;
            if (!planConfig.baseBranch) planConfig.baseBranch = analysis.baseBranch;
        }

        var plan = createSplitPlan(groups, planConfig);
        var validation = validatePlan(plan);
        if (!validation.valid) {
            bb.set('lastError', 'invalid plan: ' + validation.errors.join('; '));
            return bt.failure;
        }
        bb.set('splitPlan', plan);
        return bt.success;
    });
}

function createSplitNode(bb) {
    return bt.createBlockingLeafNode(function() {
        var plan = bb.get('splitPlan');
        if (!plan) {
            bb.set('lastError', 'no split plan available');
            return bt.failure;
        }
        var result = executeSplit(plan);
        bb.set('splitResults', result);
        if (result.error) {
            bb.set('lastError', result.error);
            return bt.failure;
        }
        return bt.success;
    });
}

function createVerifyNode(bb) {
    return bt.createBlockingLeafNode(function() {
        var plan = bb.get('splitPlan');
        if (!plan) {
            bb.set('lastError', 'no split plan available');
            return bt.failure;
        }
        var result = verifySplits(plan);
        bb.set('verifyResults', result);
        if (!result.allPassed) {
            var failed = result.results.filter(function(r) { return !r.passed; });
            var names = [];
            for (var i = 0; i < failed.length; i++) {
                names.push(failed[i].name);
            }
            bb.set('lastError', 'verification failed: ' + names.join(', '));
            return bt.failure;
        }
        return bt.success;
    });
}

function createEquivalenceNode(bb) {
    return bt.createBlockingLeafNode(function() {
        var plan = bb.get('splitPlan');
        if (!plan) {
            bb.set('lastError', 'no split plan available');
            return bt.failure;
        }
        var result = verifyEquivalence(plan);
        bb.set('equivalence', result);
        if (!result.equivalent) {
            bb.set('lastError',
                'tree mismatch: split=' + result.splitTree +
                ' source=' + result.sourceTree);
            return bt.failure;
        }
        return bt.success;
    });
}

// createSelectStrategyNode creates a BT leaf that selects the best grouping
// strategy using the claudemux ChoiceResolver.
function createSelectStrategyNode(bb, options) {
    return bt.createBlockingLeafNode(function() {
        var analysis = bb.get('analysisResult');
        if (!analysis || !analysis.files || analysis.files.length === 0) {
            bb.set('lastError', 'no analysis result available for strategy selection');
            return bt.failure;
        }
        var result = selectStrategy(analysis.files, options);
        bb.set('selectedStrategy', result);
        bb.set('fileGroups', result.groups);
        return bt.success;
    });
}

// createWorkflowTree — heuristic (non-AI) workflow.
function createWorkflowTree(bb, config) {
    config = config || {};
    var analyzeConfig = { baseBranch: config.baseBranch, dir: config.dir };
    var planConfig = {
        baseBranch: config.baseBranch,
        dir: config.dir,
        branchPrefix: config.branchPrefix,
        verifyCommand: config.verifyCommand
    };
    return bt.node(bt.sequence,
        createAnalyzeNode(bb, analyzeConfig),
        createGroupNode(bb, config.groupStrategy || 'directory', config.groupOptions),
        createPlanNode(bb, planConfig),
        createSplitNode(bb),
        createEquivalenceNode(bb)
    );
}

// ---------------------------------------------------------------------------
//  ClaudeMux BT Nodes (from bt-templates/claude-mux.js)
// ---------------------------------------------------------------------------

function createClaudeMuxClassifyNode(bb, options) {
    return bt.createBlockingLeafNode(function() {
        var analysis = bb.get('analysisResult');
        if (!analysis || !analysis.files || analysis.files.length === 0) {
            bb.set('lastError', 'no analysis result for classification');
            return bt.failure;
        }
        var registry = bb.get('claudemuxRegistry');
        if (!registry) {
            bb.set('lastError', 'no claudemux registry on blackboard');
            return bt.failure;
        }
        var opts = options || {};
        opts.registry = registry;
        opts.providerName = opts.providerName || bb.get('providerName') || 'claude-code';

        var result = classifyChangesWithClaudeMux(analysis.files, opts);
        if (result.error) {
            bb.set('lastError', 'classification: ' + result.error);
            return bt.failure;
        }
        if (!result.files || Object.keys(result.files).length === 0) {
            bb.set('lastError', 'classification returned empty result');
            return bt.failure;
        }
        bb.set('classification', result.files);
        return bt.success;
    });
}

function createClaudeMuxPlanNode(bb, options) {
    return bt.createBlockingLeafNode(function() {
        var analysis = bb.get('analysisResult');
        if (!analysis || !analysis.files || analysis.files.length === 0) {
            bb.set('lastError', 'no analysis result for planning');
            return bt.failure;
        }
        var registry = bb.get('claudemuxRegistry');
        if (!registry) {
            bb.set('lastError', 'no claudemux registry on blackboard');
            return bt.failure;
        }
        var classification = bb.get('classification') || {};
        var opts = options || {};
        opts.registry = registry;
        opts.providerName = opts.providerName || bb.get('providerName') || 'claude-code';

        var result = suggestSplitPlanWithClaudeMux(analysis.files, classification, opts);
        if (result.error) {
            bb.set('lastError', 'planning: ' + result.error);
            return bt.failure;
        }
        if (!result.stages || result.stages.length === 0) {
            bb.set('lastError', 'planning returned no stages');
            return bt.failure;
        }
        bb.set('aiSplitPlan', result.stages);

        var groups = {};
        for (var i = 0; i < result.stages.length; i++) {
            var stage = result.stages[i];
            groups[stage.name] = stage.files;
        }
        bb.set('fileGroups', groups);
        return bt.success;
    });
}

// createClaudeMuxWorkflowTree — AI-powered workflow.
function createClaudeMuxWorkflowTree(bb, config) {
    config = config || {};
    var analyzeConfig = { baseBranch: config.baseBranch, dir: config.dir };
    var planConfig = {
        baseBranch: config.baseBranch,
        dir: config.dir,
        branchPrefix: config.branchPrefix,
        verifyCommand: config.verifyCommand
    };
    return bt.node(bt.sequence,
        createAnalyzeNode(bb, analyzeConfig),
        createClaudeMuxClassifyNode(bb, config.classifyOpts),
        createClaudeMuxPlanNode(bb, config.planOpts),
        createPlanNode(bb, planConfig),
        createSplitNode(bb),
        createEquivalenceNode(bb)
    );
}

// ---------------------------------------------------------------------------
//  Reusable BT Templates (from bt-templates/claude-mux.js)
// ---------------------------------------------------------------------------

function btSpawnClaude(bb, registry, providerName, spawnOpts) {
    return bt.createBlockingLeafNode(function() {
        try {
            var handle = registry.spawn(providerName || 'claude-code', spawnOpts || {});
            bb.set('agent', handle);
            bb.set('agentSpawned', true);
            bb.set('parser', claudemux ? claudemux.newParser() : null);
            return bt.success;
        } catch (e) {
            bb.set('lastError', String(e));
            return bt.failure;
        }
    });
}

function btSendPrompt(bb, prompt) {
    return bt.createBlockingLeafNode(function() {
        var agent = bb.get('agent');
        if (!agent) {
            bb.set('lastError', 'no agent spawned');
            return bt.failure;
        }
        try {
            agent.send(prompt + '\n');
            bb.set('promptSent', true);
            return bt.success;
        } catch (e) {
            bb.set('lastError', String(e));
            return bt.failure;
        }
    });
}

function btWaitForResponse(bb, opts) {
    var maxEmptyReads = (opts && opts.maxEmptyReads) || 100;
    return bt.createBlockingLeafNode(function() {
        var agent = bb.get('agent');
        var parser = bb.get('parser');
        if (!agent || !parser) {
            bb.set('lastError', 'no agent or parser available');
            return bt.failure;
        }

        var output = [];
        var emptyCount = 0;

        while (agent.isAlive() && emptyCount < maxEmptyReads) {
            var data = agent.receive();
            if (data === '') {
                emptyCount++;
                continue;
            }
            emptyCount = 0;
            var lines = data.split('\n');
            for (var i = 0; i < lines.length; i++) {
                if (lines[i] === '') continue;
                output.push(lines[i]);
                var event = parser.parse(lines[i]);
                if (event.type === claudemux.EVENT_COMPLETION) {
                    bb.set('response', output.join('\n'));
                    bb.set('responseReceived', true);
                    return bt.success;
                }
                if (event.type === claudemux.EVENT_RATE_LIMIT) {
                    bb.set('rateLimited', true);
                    bb.set('response', output.join('\n'));
                    return bt.running;
                }
                if (event.type === claudemux.EVENT_PERMISSION) {
                    try { agent.send('n\n'); } catch (e) { /* ignore */ }
                    bb.set('permissionRejected', true);
                }
                if (event.type === claudemux.EVENT_ERROR) {
                    bb.set('lastError', lines[i]);
                }
            }
        }

        if (output.length > 0) {
            bb.set('response', output.join('\n'));
            bb.set('responseReceived', true);
            return bt.success;
        }
        bb.set('lastError', 'no output received from agent');
        return bt.failure;
    });
}

function btVerifyOutput(bb, command) {
    return bt.createBlockingLeafNode(function() {
        var result = exec.exec('sh', '-c', command);
        bb.set('verifyCode', result.code);
        bb.set('verifyStdout', result.stdout);
        bb.set('verifyStderr', result.stderr);
        if (result.code === 0) {
            bb.set('verified', true);
            return bt.success;
        }
        bb.set('lastError', 'verify failed: exit ' + result.code);
        return bt.failure;
    });
}

function btRunTests(bb, command) {
    command = command || 'make test';
    return bt.createBlockingLeafNode(function() {
        var result = exec.exec('sh', '-c', command);
        bb.set('testCode', result.code);
        bb.set('testStdout', result.stdout);
        if (result.code === 0) {
            bb.set('testsPassed', true);
            return bt.success;
        }
        bb.set('lastError', 'tests failed: exit ' + result.code);
        return bt.failure;
    });
}

function btCommitChanges(bb, message) {
    return bt.createBlockingLeafNode(function() {
        var addResult = exec.exec('git', 'add', '-A');
        if (addResult.code !== 0) {
            bb.set('lastError', 'git add failed: ' + addResult.stderr);
            return bt.failure;
        }
        var commitResult = exec.exec('git', 'commit', '-m', message);
        if (commitResult.code !== 0) {
            bb.set('lastError', 'git commit failed: ' + commitResult.stderr);
            return bt.failure;
        }
        bb.set('commitOutput', commitResult.stdout.trim());
        bb.set('committed', true);
        return bt.success;
    });
}

function btSplitBranch(bb, branchName) {
    return bt.createBlockingLeafNode(function() {
        var result = exec.exec('git', 'checkout', '-b', branchName);
        if (result.code !== 0) {
            bb.set('lastError', 'branch creation failed: ' + result.stderr);
            return bt.failure;
        }
        bb.set('currentBranch', branchName);
        bb.set('branchCreated', true);
        return bt.success;
    });
}

// ---------------------------------------------------------------------------
//  Composite BT Workflow Functions (from claude-mux.js templates)
// ---------------------------------------------------------------------------

// spawnAndPrompt creates a sequence: spawn agent then send prompt.
function spawnAndPrompt(bb, registry, providerName, opts) {
    return bt.node(bt.sequence,
        btSpawnClaude(bb, registry, providerName, opts || {}),
        btSendPrompt(bb, (opts && opts.prompt) || '')
    );
}

// verifyAndCommit creates a sequence: optionally verify output, run tests, commit.
function verifyAndCommit(bb, opts) {
    opts = opts || {};
    var children = [];
    if (opts.verifyCommand) {
        children.push(btVerifyOutput(bb, opts.verifyCommand));
    }
    children.push(btRunTests(bb, opts.testCommand));
    children.push(btCommitChanges(bb, opts.message || 'automated commit'));
    return bt.node.apply(null, [bt.sequence].concat(children));
}

// spawnPromptAndReadResult creates a sequence: spawn → prompt → wait → read.
function spawnPromptAndReadResult(bb, registry, providerName, opts) {
    return bt.node(bt.sequence,
        btSpawnClaude(bb, registry, providerName, opts || {}),
        btSendPrompt(bb, (opts && opts.prompt) || ''),
        btWaitForResponse(bb, opts)
    );
}

// createPlanningActions creates PA-BT actions for the 7 BT template operations.
// Each action wraps the corresponding BT template as a pabt.newAction with
// empty conditions/effects, suitable for registration via state.registerAction().
function createPlanningActions(pabt, bb, registry, opts) {
    opts = opts || {};
    return {
        SpawnClaude: pabt.newAction('SpawnClaude', [], [],
            btSpawnClaude(bb, registry, opts.provider || 'claude-code', opts)
        ),
        SendPrompt: pabt.newAction('SendPrompt', [], [],
            btSendPrompt(bb, opts.prompt || '')
        ),
        WaitForResponse: pabt.newAction('WaitForResponse', [], [],
            btWaitForResponse(bb, opts)
        ),
        VerifyOutput: pabt.newAction('VerifyOutput', [], [],
            btVerifyOutput(bb, opts.verifyCommand || opts.testCommand || 'make test')
        ),
        RunTests: pabt.newAction('RunTests', [], [],
            btRunTests(bb, opts.testCommand || 'make test')
        ),
        CommitChanges: pabt.newAction('CommitChanges', [], [],
            btCommitChanges(bb, opts.message || 'automated commit')
        ),
        SplitBranch: pabt.newAction('SplitBranch', [], [],
            btSplitBranch(bb, opts.branchName || 'split-branch')
        )
    };
}

// ---------------------------------------------------------------------------
//  Exports for Global/Test Access
// ---------------------------------------------------------------------------
// These are exposed so that tests and cross-script access can call them.

globalThis.prSplit = {
    // Analysis
    analyzeDiff: analyzeDiff,
    analyzeDiffStats: analyzeDiffStats,

    // Grouping
    groupByDirectory: groupByDirectory,
    groupByExtension: groupByExtension,
    groupByPattern: groupByPattern,
    groupByChunks: groupByChunks,
    applyStrategy: applyStrategy,
    selectStrategy: selectStrategy,

    // AI
    classifyChangesWithClaudeMux: classifyChangesWithClaudeMux,
    suggestSplitPlanWithClaudeMux: suggestSplitPlanWithClaudeMux,

    // Planning
    createSplitPlan: createSplitPlan,
    validatePlan: validatePlan,

    // Execution
    executeSplit: executeSplit,

    // Verification
    verifySplit: verifySplit,
    verifySplits: verifySplits,
    verifyEquivalence: verifyEquivalence,
    verifyEquivalenceDetailed: verifyEquivalenceDetailed,
    cleanupBranches: cleanupBranches,

    // BT nodes
    createAnalyzeNode: createAnalyzeNode,
    createGroupNode: createGroupNode,
    createPlanNode: createPlanNode,
    createSplitNode: createSplitNode,
    createVerifyNode: createVerifyNode,
    createEquivalenceNode: createEquivalenceNode,
    createSelectStrategyNode: createSelectStrategyNode,
    createWorkflowTree: createWorkflowTree,
    createClaudeMuxClassifyNode: createClaudeMuxClassifyNode,
    createClaudeMuxPlanNode: createClaudeMuxPlanNode,
    createClaudeMuxWorkflowTree: createClaudeMuxWorkflowTree,

    // BT templates (from claude-mux.js)
    btSpawnClaude: btSpawnClaude,
    btSendPrompt: btSendPrompt,
    btWaitForResponse: btWaitForResponse,
    btVerifyOutput: btVerifyOutput,
    btRunTests: btRunTests,
    btCommitChanges: btCommitChanges,
    btSplitBranch: btSplitBranch,

    // Composite BT workflow functions
    spawnAndPrompt: spawnAndPrompt,
    verifyAndCommit: verifyAndCommit,
    spawnPromptAndReadResult: spawnPromptAndReadResult,
    createPlanningActions: createPlanningActions,

    // Internal helpers (exposed for testing)
    _gitExec: gitExec,
    _dirname: dirname,
    _fileExtension: fileExtension,
    _sanitizeBranchName: sanitizeBranchName,
    _padIndex: padIndex,

    // Runtime config access
    runtime: runtime,

    VERSION: '5.0.0'
};

// ---------------------------------------------------------------------------
//  TUI Mode — Interactive PR Splitting
//  (Guarded: only runs when tui/output/ctx globals are available, i.e.
//   when loaded by the embedded scripting engine. Skipped when loaded
//   via require() in test environments.)
// ---------------------------------------------------------------------------

if (typeof tui !== 'undefined' && typeof ctx !== 'undefined' && typeof output !== 'undefined' && shared !== null) {

var analysisCache = null;
var groupsCache = null;
var planCache = null;

var state = tui.createState(COMMAND_NAME, {
    [shared.contextItems]: {defaultValue: []}
});

function buildCommands(stateArg) {
    return {
        analyze: {
            description: 'Analyze diff between current branch and base',
            usage: 'analyze [base-branch]',
            handler: function(args) {
                var base = (args && args.length > 0) ? args[0] : runtime.baseBranch;
                output.print('Analyzing diff against ' + base + '...');
                analysisCache = analyzeDiff({ baseBranch: base });
                if (analysisCache.error) {
                    output.print('Error: ' + analysisCache.error);
                    return;
                }
                output.print('Branch: ' + analysisCache.currentBranch + ' → ' + analysisCache.baseBranch);
                output.print('Changed files: ' + analysisCache.files.length);
                for (var i = 0; i < analysisCache.files.length; i++) {
                    output.print('  ' + analysisCache.files[i]);
                }
            }
        },

        stats: {
            description: 'Show diff stats with addition/deletion counts',
            usage: 'stats',
            handler: function() {
                var stats = analyzeDiffStats({ baseBranch: runtime.baseBranch });
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
                if (!analysisCache || !analysisCache.files || analysisCache.files.length === 0) {
                    output.print('Run "analyze" first.');
                    return;
                }
                var strategy = (args && args.length > 0) ? args[0] : runtime.strategy;
                groupsCache = applyStrategy(analysisCache.files, strategy);
                var groupNames = Object.keys(groupsCache).sort();
                output.print('Groups (' + strategy + '): ' + groupNames.length);
                for (var i = 0; i < groupNames.length; i++) {
                    var name = groupNames[i];
                    output.print('  ' + name + ': ' + groupsCache[name].length + ' files');
                    for (var j = 0; j < groupsCache[name].length; j++) {
                        output.print('    ' + groupsCache[name][j]);
                    }
                }
            }
        },

        plan: {
            description: 'Create split plan from current groups',
            usage: 'plan',
            handler: function() {
                if (!groupsCache) {
                    output.print('Run "group" first.');
                    return;
                }
                var planConfig = {
                    baseBranch: runtime.baseBranch,
                    branchPrefix: runtime.branchPrefix,
                    verifyCommand: runtime.verifyCommand
                };
                if (analysisCache) {
                    planConfig.sourceBranch = analysisCache.currentBranch;
                }
                planCache = createSplitPlan(groupsCache, planConfig);
                var validation = validatePlan(planCache);
                if (!validation.valid) {
                    output.print('Plan validation errors:');
                    for (var i = 0; i < validation.errors.length; i++) {
                        output.print('  - ' + validation.errors[i]);
                    }
                    return;
                }
                output.print('Plan created: ' + planCache.splits.length + ' splits');
                output.print('Base: ' + planCache.baseBranch + ' → Source: ' + planCache.sourceBranch);
                for (var i = 0; i < planCache.splits.length; i++) {
                    var s = planCache.splits[i];
                    output.print('  ' + padIndex(s.order + 1) + '. ' + s.name + ' (' + s.files.length + ' files)');
                }
            }
        },

        preview: {
            description: 'Show detailed plan preview (dry run)',
            usage: 'preview',
            handler: function() {
                if (!planCache) {
                    output.print('Run "plan" first.');
                    return;
                }
                output.print('=== Split Plan Preview ===');
                output.print('Base branch:    ' + planCache.baseBranch);
                output.print('Source branch:  ' + planCache.sourceBranch);
                output.print('Verify command: ' + planCache.verifyCommand);
                output.print('Splits:         ' + planCache.splits.length);
                output.print('');
                var prevBranch = planCache.baseBranch;
                for (var i = 0; i < planCache.splits.length; i++) {
                    var s = planCache.splits[i];
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

        execute: {
            description: 'Execute the split plan (creates branches)',
            usage: 'execute',
            handler: function() {
                if (!planCache) {
                    output.print('Run "plan" first.');
                    return;
                }
                if (runtime.dryRun) {
                    output.print('Dry-run mode: showing plan without executing.');
                    output.print('Disable with: set dry-run false');
                    return;
                }
                output.print('Executing split plan (' + planCache.splits.length + ' splits)...');
                var result = executeSplit(planCache);
                if (result.error) {
                    output.print('Error: ' + result.error);
                    return;
                }
                output.print('Split completed successfully!');
                for (var i = 0; i < result.results.length; i++) {
                    var r = result.results[i];
                    output.print('  ✓ ' + r.name + ' (' + r.files.length + ' files, SHA: ' + r.sha.substring(0, 8) + ')');
                }

                // Auto-verify equivalence
                var equiv = verifyEquivalence(planCache);
                if (equiv.equivalent) {
                    output.print('✅ Tree hash equivalence verified');
                } else if (equiv.error) {
                    output.print('⚠️  Equivalence check error: ' + equiv.error);
                } else {
                    output.print('❌ Tree hash mismatch!');
                    output.print('   Split tree:  ' + equiv.splitTree);
                    output.print('   Source tree:  ' + equiv.sourceTree);
                }
            }
        },

        verify: {
            description: 'Verify all split branches (run tests on each)',
            usage: 'verify',
            handler: function() {
                if (!planCache) {
                    output.print('Run "plan" first.');
                    return;
                }
                output.print('Verifying ' + planCache.splits.length + ' splits...');
                var result = verifySplits(planCache);
                for (var i = 0; i < result.results.length; i++) {
                    var r = result.results[i];
                    var icon = r.passed ? '✓' : '✗';
                    output.print('  ' + icon + ' ' + r.name + (r.error ? ': ' + r.error : ''));
                }
                output.print(result.allPassed ? '✅ All splits pass verification' : '❌ Some splits failed');
            }
        },

        equivalence: {
            description: 'Check tree hash equivalence',
            usage: 'equivalence',
            handler: function() {
                if (!planCache) {
                    output.print('Run "plan" first.');
                    return;
                }
                var result = verifyEquivalenceDetailed(planCache);
                if (result.error) {
                    output.print('Error: ' + result.error);
                    return;
                }
                if (result.equivalent) {
                    output.print('✅ Trees are equivalent');
                    output.print('   Hash: ' + result.splitTree);
                } else {
                    output.print('❌ Trees differ');
                    output.print('   Split tree:  ' + result.splitTree);
                    output.print('   Source tree:  ' + result.sourceTree);
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
                if (!planCache) {
                    output.print('No plan to clean up.');
                    return;
                }
                var result = cleanupBranches(planCache);
                if (result.deleted.length > 0) {
                    output.print('Deleted branches:');
                    for (var i = 0; i < result.deleted.length; i++) {
                        output.print('  ' + result.deleted[i]);
                    }
                }
                if (result.errors.length > 0) {
                    output.print('Errors:');
                    for (var i = 0; i < result.errors.length; i++) {
                        output.print('  ' + result.errors[i]);
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
                    output.print('Keys: base, strategy, max, prefix, verify, dry-run, ai, provider, model');
                    output.print('Current:');
                    output.print('  base:      ' + runtime.baseBranch);
                    output.print('  strategy:  ' + runtime.strategy);
                    output.print('  max:       ' + runtime.maxFiles);
                    output.print('  prefix:    ' + runtime.branchPrefix);
                    output.print('  verify:    ' + runtime.verifyCommand);
                    output.print('  dry-run:   ' + runtime.dryRun);
                    output.print('  ai:        ' + runtime.aiMode);
                    output.print('  provider:  ' + runtime.provider);
                    output.print('  model:     ' + runtime.model);
                    return;
                }
                var key = args[0];
                var value = args.slice(1).join(' ');
                switch (key) {
                    case 'base':
                        runtime.baseBranch = value;
                        break;
                    case 'strategy':
                        runtime.strategy = value;
                        break;
                    case 'max':
                        runtime.maxFiles = parseInt(value, 10) || 10;
                        break;
                    case 'prefix':
                        runtime.branchPrefix = value;
                        break;
                    case 'verify':
                        runtime.verifyCommand = value;
                        break;
                    case 'dry-run':
                        runtime.dryRun = (value === 'true' || value === '1');
                        break;
                    case 'ai':
                        runtime.aiMode = (value === 'true' || value === '1');
                        break;
                    case 'provider':
                        runtime.provider = value;
                        break;
                    case 'model':
                        runtime.model = value;
                        break;
                    default:
                        output.print('Unknown key: ' + key);
                        return;
                }
                output.print('Set ' + key + ' = ' + value);
            }
        },

        classify: {
            description: 'Classify files using AI (requires --ai)',
            usage: 'classify',
            handler: function() {
                if (!claudemux) {
                    output.print('claudemux module not available. Install/enable AI provider.');
                    return;
                }
                if (!analysisCache || !analysisCache.files || analysisCache.files.length === 0) {
                    output.print('Run "analyze" first.');
                    return;
                }
                output.print('Classifying ' + analysisCache.files.length + ' files with AI...');
                output.print('(This spawns a Claude Code instance via PTY — may take a minute)');
                // Note: actual registry would need to be configured
                output.print('AI classification requires a configured provider registry.');
                output.print('Use: set provider <name> && set model <name>');
            }
        },

        run: {
            description: 'Run full workflow: analyze → group → plan → execute',
            usage: 'run',
            handler: function() {
                output.print('Running full PR split workflow...');
                output.print('Base:     ' + runtime.baseBranch);
                output.print('Strategy: ' + runtime.strategy);
                output.print('Max:      ' + runtime.maxFiles);
                output.print('');

                // Step 1: Analyze
                analysisCache = analyzeDiff({ baseBranch: runtime.baseBranch });
                if (analysisCache.error) {
                    output.print('Analysis failed: ' + analysisCache.error);
                    return;
                }
                output.print('✓ Analysis: ' + analysisCache.files.length + ' changed files');

                // Step 2: Group
                groupsCache = applyStrategy(analysisCache.files, runtime.strategy);
                var groupNames = Object.keys(groupsCache).sort();
                output.print('✓ Grouped into ' + groupNames.length + ' groups (' + runtime.strategy + ')');

                // Step 3: Plan
                planCache = createSplitPlan(groupsCache, {
                    baseBranch: runtime.baseBranch,
                    sourceBranch: analysisCache.currentBranch,
                    branchPrefix: runtime.branchPrefix,
                    verifyCommand: runtime.verifyCommand
                });
                var validation = validatePlan(planCache);
                if (!validation.valid) {
                    output.print('Plan invalid: ' + validation.errors.join('; '));
                    return;
                }
                output.print('✓ Plan created: ' + planCache.splits.length + ' splits');

                // Step 4: Execute (unless dry-run)
                if (runtime.dryRun) {
                    output.print('');
                    output.print('DRY RUN — not executing. Use "set dry-run false" then "execute".');
                    return;
                }

                var result = executeSplit(planCache);
                if (result.error) {
                    output.print('Execution failed: ' + result.error);
                    return;
                }
                output.print('✓ Split executed: ' + result.results.length + ' branches created');

                // Step 5: Verify equivalence
                var equiv = verifyEquivalence(planCache);
                if (equiv.equivalent) {
                    output.print('✅ Tree hash equivalence verified');
                } else if (equiv.error) {
                    output.print('⚠️  Equivalence check error: ' + equiv.error);
                } else {
                    output.print('❌ Tree hash mismatch — content may be lost');
                }
            }
        },

        copy: {
            description: 'Copy the split plan to clipboard',
            usage: 'copy',
            handler: function() {
                if (!planCache) {
                    output.print('Run "plan" first.');
                    return;
                }
                try {
                    var text = template.execute(prSplitTemplate, {
                        baseBranch: planCache.baseBranch,
                        currentBranch: planCache.sourceBranch,
                        fileCount: analysisCache ? analysisCache.files.length : 0,
                        strategy: runtime.strategy,
                        aiMode: runtime.aiMode,
                        provider: runtime.provider,
                        model: runtime.model,
                        groupCount: Object.keys(groupsCache || {}).length,
                        groups: Object.keys(groupsCache || {}).sort().map(function(name) {
                            return { label: name, files: groupsCache[name] };
                        }),
                        plan: planCache.splits.map(function(s, i) {
                            return {
                                index: i + 1,
                                branch: s.name,
                                fileCount: s.files.length,
                                description: s.message
                            };
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

        help: {
            description: 'Show available commands',
            usage: 'help',
            handler: function() {
                output.print('PR Split Commands:');
                output.print('');
                output.print('  analyze [base]   Analyze diff against base branch');
                output.print('  stats            Show file-level diff stats');
                output.print('  group [strategy] Group files (directory/extension/chunks/auto)');
                output.print('  plan             Create split plan from groups');
                output.print('  preview          Show detailed plan preview');
                output.print('  execute          Execute the split (create branches)');
                output.print('  verify           Verify each split branch');
                output.print('  equivalence      Check tree hash equivalence');
                output.print('  cleanup          Delete all split branches');
                output.print('  run              Full workflow: analyze→group→plan→execute');
                output.print('  set <key> <val>  Set runtime config (no args to show current)');
                output.print('  classify         Classify files with AI');
                output.print('  copy             Copy plan to clipboard');
                output.print('  help             Show this help');
            }
        }
    };
}

// ---------------------------------------------------------------------------
//  Mode Registration
// ---------------------------------------------------------------------------

ctx.run('register-mode', function() {
    tui.registerMode({
        name: MODE_NAME,
        tui: {
            title: 'PR Split',
            prompt: '(pr-split) > '
        },
        onEnter: function() {
            output.print('PR Split — split large PRs into reviewable stacked branches.');
            output.print('Type "help" for commands. Quick start: "run"');
            output.print('');
            output.print('Config: base=' + runtime.baseBranch + ' strategy=' + runtime.strategy +
                ' max=' + runtime.maxFiles + (runtime.dryRun ? ' [DRY RUN]' : ''));
        },
        commands: function() {
            return buildCommands(state);
        }
    });
});

ctx.run('enter-pr-split', function() {
    tui.switchMode(MODE_NAME);
});

} // end TUI guard

// ---------------------------------------------------------------------------
//  CommonJS exports for require() compatibility (test environments).
// ---------------------------------------------------------------------------
if (typeof module !== 'undefined' && module.exports) {
    module.exports = globalThis.prSplit;
}
