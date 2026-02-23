// orchestrate-pr-split.js — PR splitting workflow for creating linear branch series.
//
// This module provides functions for analyzing a diff, grouping changed files
// by various strategies, creating split plans, executing splits as stacked
// branches, and verifying each split independently plus final equivalence.
//
// Usage:
//   var prSplit = require('./scripts/orchestrate-pr-split.js');
//
//   // 1. Analyze what changed
//   var analysis = prSplit.analyzeDiff({ baseBranch: 'main', dir: '.' });
//
//   // 2. Group files into logical units
//   var groups = prSplit.groupByDirectory(analysis.files, 1);
//
//   // 3. Create a split plan
//   var plan = prSplit.createSplitPlan(groups, {
//       baseBranch: 'main',
//       sourceBranch: analysis.currentBranch,
//       branchPrefix: 'split/'
//   });
//
//   // 4. Execute the split (creates stacked branches)
//   var result = prSplit.executeSplit(plan);
//
//   // 5. Verify equivalence (tree hashes match)
//   var equiv = prSplit.verifyEquivalence(plan);
//
// BT Integration (heuristic):
//   var bb = new bt.Blackboard();
//   var tree = prSplit.createWorkflowTree(bb, { baseBranch: 'main' });
//   bt.tick(tree);
//
// BT Integration (AI-powered via ClaudeMux):
//   var bb = new bt.Blackboard();
//   bb.set('claudemuxRegistry', registry);
//   var tree = prSplit.createClaudeMuxWorkflowTree(bb, {
//       baseBranch: 'main',
//       classifyOpts: { registry: registry, providerName: 'ollama' },
//       planOpts:     { registry: registry, providerName: 'ollama' }
//   });
//   bt.tick(tree);
//
// Each split branch builds on the previous, creating a linear series:
//   main → split/01-types → split/02-impl → split/03-docs
//
// The final branch in the series should have the same tree hash as the
// source branch, ensuring no content is lost or duplicated.

'use strict';

var bt = require('osm:bt');
var exec = require('osm:exec');

// Optional: claudemux ChoiceResolver for intelligent strategy selection.
var claudemux;
try {
    claudemux = require('osm:claudemux');
} catch (e) {
    claudemux = null;
}

// ---------------------------------------------------------------------------
//  Internal Helpers
// ---------------------------------------------------------------------------

// gitExec runs a git command with the given args in the specified directory.
// Returns the exec result: { stdout, stderr, code, error, message }
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

// shellQuote single-quotes a string for safe shell interpolation.
function shellQuote(s) {
    return "'" + s.replace(/'/g, "'\\''") + "'";
}

// dirname extracts the directory component of a file path at a given depth.
//   dirname('pkg/sub/file.go', 1) → 'pkg'
//   dirname('pkg/sub/file.go', 2) → 'pkg/sub'
//   dirname('file.go', 1)         → '.'
function dirname(filepath, depth) {
    depth = depth || 1;
    var parts = filepath.split('/');
    if (parts.length <= 1) {
        return '.';
    }
    var taken = parts.slice(0, Math.min(depth, parts.length - 1));
    return taken.join('/');
}

// fileExtension extracts the extension from a filename (including dot).
// Returns '' if no extension.
function fileExtension(filepath) {
    var base = filepath.split('/').pop();
    var dot = base.lastIndexOf('.');
    if (dot <= 0) {
        return '';
    }
    return base.substring(dot);
}

// sanitizeBranchName replaces characters that are invalid in git branch names.
function sanitizeBranchName(name) {
    return name.replace(/[^a-zA-Z0-9_/-]/g, '-');
}

// ---------------------------------------------------------------------------
//  Analysis
// ---------------------------------------------------------------------------

// analyzeDiff returns the list of files changed between the current branch
// and the specified base branch.
//
// Parameters:
//   config.baseBranch — branch to diff against (default: 'main')
//   config.dir        — working directory (default: '.')
//
// Returns:
//   { files: string[], error: string|null, baseBranch: string,
//     currentBranch: string }
exports.analyzeDiff = function(config) {
    config = config || {};
    var baseBranch = config.baseBranch || 'main';
    var dir = config.dir || '.';

    // Get current branch name
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

    // Find merge base between base and current branch
    var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
    if (mergeBase.code !== 0) {
        return {
            files: [],
            error: 'merge-base failed: ' + mergeBase.stderr.trim(),
            baseBranch: baseBranch,
            currentBranch: currentBranch
        };
    }

    // Get changed files relative to merge base
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
};

// analyzeDiffStats returns changed files with addition/deletion counts.
//
// Returns:
//   { files: [{name, additions, deletions}], error, baseBranch, currentBranch }
exports.analyzeDiffStats = function(config) {
    config = config || {};
    var baseBranch = config.baseBranch || 'main';
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
};

// ---------------------------------------------------------------------------
//  Grouping Strategies
// ---------------------------------------------------------------------------

// groupByDirectory groups files by their directory prefix at a given depth.
//
//   depth=1: 'pkg/foo/bar.go' → group 'pkg'
//   depth=2: 'pkg/foo/bar.go' → group 'pkg/foo'
//
// Returns: { 'dir': ['file1', 'file2'], ... }
exports.groupByDirectory = function(files, depth) {
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
};

// groupByExtension groups files by their file extension.
//
// Returns: { '.go': ['file1.go'], '.js': ['file2.js'], '(none)': ['Makefile'] }
exports.groupByExtension = function(files) {
    var groups = {};
    for (var i = 0; i < files.length; i++) {
        var ext = fileExtension(files[i]) || '(none)';
        if (!groups[ext]) {
            groups[ext] = [];
        }
        groups[ext].push(files[i]);
    }
    return groups;
};

// groupByPattern groups files by matching against named regex patterns.
// Files that match no pattern go into '(other)'.
//
// patterns: { 'groupName': /regex/ }
//
// Returns: { 'groupName': ['matching files'], '(other)': ['unmatched'] }
exports.groupByPattern = function(files, patterns) {
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
};

// groupByChunks splits files into equal-sized chunks.
//
// maxPerGroup: maximum files per group (default: 5)
//
// Returns: { 'chunk-1': ['file1', ...], 'chunk-2': [...], ... }
exports.groupByChunks = function(files, maxPerGroup) {
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
};

// ---------------------------------------------------------------------------
//  Strategy Selection (claudemux integration)
// ---------------------------------------------------------------------------

// selectStrategy evaluates grouping strategies and recommends the best one
// using the claudemux ChoiceResolver. Falls back to directory grouping
// when claudemux is unavailable.
//
// Parameters:
//   files:   string[]  — list of changed file paths
//   options: { maxPerGroup?: number }
//
// Returns:
//   { strategy: string, groups: object, reason: string,
//     needsConfirm: bool, scored: array }
exports.selectStrategy = function(files, options) {
    options = options || {};
    var maxPerGroup = options.maxPerGroup || 10;

    // Compute groups for each strategy.
    var strategies = [
        { name: 'directory', groups: exports.groupByDirectory(files, 1) },
        { name: 'directory-deep', groups: exports.groupByDirectory(files, 2) },
        { name: 'extension', groups: exports.groupByExtension(files) },
        { name: 'chunks', groups: exports.groupByChunks(files, maxPerGroup) }
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

    // Build candidates from strategy results.
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
};

// ---------------------------------------------------------------------------
//  ClaudeMux AI Integration
// ---------------------------------------------------------------------------
// These functions use Claude Code (via claudemux PTY harness + MCP tools)
// to perform AI-powered classification and planning. They require:
//   - claudemux module available
//   - A provider registered in the registry (e.g., 'claude-code' or 'ollama')
//   - The MCP result channel (--result-dir) for structured result passing

// classifyChangesWithClaudeMux uses an AI provider to classify changed files
// into logical categories (e.g., 'types', 'impl', 'docs', 'tests').
//
// Parameters:
//   files:    string[]  — list of changed file paths from analyzeDiff
//   options:  {
//     registry:     object  — claudemux provider Registry (required)
//     providerName: string  — provider name (default: 'claude-code')
//     sessionId:    string  — MCP session ID (default: 'classify-' + Date.now())
//     spawnOpts:    object  — provider spawn options {model, dir, rows, cols}
//     maxWaitMs:    number  — max wait time in ms (default: 120000)
//   }
//
// Returns: { files: object, error: string|null }
//   files: map of filepath -> category name (e.g., { 'foo.go': 'impl' })
exports.classifyChangesWithClaudeMux = function(files, options) {
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

    // Create MCP instance with result directory.
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

    // Build the classification prompt.
    var fileList = files.join('\n');
    var prompt = 'You are a PR splitting assistant. Classify each of the following changed files into a logical category.\n' +
        'Use the MCP tool "reportClassification" to report your results.\n' +
        'Categories should be concise labels like: types, impl, tests, docs, config, deps, refactor.\n' +
        'Each file must be assigned exactly one category.\n\n' +
        'Changed files:\n' + fileList + '\n\n' +
        'Call the reportClassification tool with sessionId "' + sessionId + '" and a files map.';

    // Spawn Claude Code with MCP config.
    var spawnOpts = options.spawnOpts || {};
    spawnOpts.args = (spawnOpts.args || []).concat(mcpInst.spawnArgs());

    var handle;
    try {
        handle = options.registry.spawn(providerName, spawnOpts);
    } catch (e) {
        mcpInst.close();
        return { files: {}, error: 'spawn failed: ' + e };
    }

    // Send prompt and wait for completion.
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
                    // Claude finished — result should be in result dir.
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

        // Timeout or agent exit — try reading result anyway.
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
};

// suggestSplitPlanWithClaudeMux uses an AI provider to suggest an ordered
// split plan given a set of files and (optionally) their classifications.
//
// Parameters:
//   files:          string[]  — list of changed file paths
//   classification: object    — optional map of filepath -> category from classifyChangesWithClaudeMux
//   options:  {
//     registry:     object  — claudemux provider Registry (required)
//     providerName: string  — provider name (default: 'claude-code')
//     sessionId:    string  — MCP session ID (default: 'plan-' + Date.now())
//     spawnOpts:    object  — provider spawn options
//     maxWaitMs:    number  — max wait time in ms (default: 120000)
//   }
//
// Returns: { stages: array, error: string|null }
//   stages: [{ name, files, message, order }] from reportSplitPlan
exports.suggestSplitPlanWithClaudeMux = function(files, classification, options) {
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

    // Build planning prompt.
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
};

// ---------------------------------------------------------------------------
//  Planning
// ---------------------------------------------------------------------------

// createSplitPlan creates a structured plan from file groups.
//
// groups: { 'groupName': ['file1', 'file2'], ... }
// config: {
//   baseBranch:    string  — default: 'main'
//   sourceBranch:  string  — default: auto-detected from HEAD
//   dir:           string  — default: '.'
//   branchPrefix:  string  — default: 'split/'
//   verifyCommand: string  — default: 'make test'
//   commitPrefix:  string  — default: ''
// }
//
// Returns: SplitPlan with baseBranch, sourceBranch, dir, verifyCommand,
//          and splits array (sorted by group name).
exports.createSplitPlan = function(groups, config) {
    config = config || {};
    var dir = config.dir || '.';
    var baseBranch = config.baseBranch || 'main';
    var branchPrefix = config.branchPrefix || 'split/';
    var commitPrefix = config.commitPrefix || '';
    var verifyCommand = config.verifyCommand || 'make test';

    // Auto-detect source branch if not provided
    var sourceBranch = config.sourceBranch;
    if (!sourceBranch) {
        var result = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
        sourceBranch = result.code === 0 ? result.stdout.trim() : 'HEAD';
    }

    var groupNames = Object.keys(groups).sort();
    var splits = [];

    for (var i = 0; i < groupNames.length; i++) {
        var name = groupNames[i];
        var paddedIdx = String(i + 1);
        while (paddedIdx.length < 2) {
            paddedIdx = '0' + paddedIdx;
        }

        splits.push({
            name: sanitizeBranchName(branchPrefix + paddedIdx + '-' + name),
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
};

// validatePlan validates a split plan for correctness.
//
// Checks:
//   1. Plan has at least one split
//   2. No empty splits (every split has files)
//   3. Every split has a name
//   4. No duplicate files across splits
//
// Returns: { valid: bool, errors: string[] }
exports.validatePlan = function(plan) {
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
};

// ---------------------------------------------------------------------------
//  Execution
// ---------------------------------------------------------------------------

// executeSplit executes a split plan, creating a linear series of stacked
// branches. Each branch builds on the previous:
//
//   base → split/01-types → split/02-impl → split/03-docs
//
// Files are checked out from the source branch into each split branch.
// The last branch should have the same tree as sourceBranch.
//
// Returns: { error: string|null, results: [{name, files, sha, error}] }
exports.executeSplit = function(plan) {
    var dir = plan.dir || '.';
    var results = [];

    // Validate plan first
    var validation = exports.validatePlan(plan);
    if (!validation.valid) {
        return { error: 'invalid plan: ' + validation.errors.join('; '), results: [] };
    }

    // Save current branch to restore later
    var savedBranch = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    if (savedBranch.code !== 0) {
        return { error: 'failed to get current branch', results: [] };
    }
    var restoreBranch = savedBranch.stdout.trim();

    var currentBase = plan.baseBranch;

    for (var i = 0; i < plan.splits.length; i++) {
        var split = plan.splits[i];
        var splitResult = { name: split.name, files: split.files, sha: '', error: null };

        // Checkout base for this split
        var co = gitExec(dir, ['checkout', currentBase]);
        if (co.code !== 0) {
            splitResult.error = 'checkout ' + currentBase + ' failed: ' + co.stderr.trim();
            results.push(splitResult);
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: splitResult.error, results: results };
        }

        // Create new branch
        var br = gitExec(dir, ['checkout', '-b', split.name]);
        if (br.code !== 0) {
            splitResult.error = 'branch creation failed: ' + br.stderr.trim();
            results.push(splitResult);
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: splitResult.error, results: results };
        }

        // Checkout files from source branch one at a time
        for (var j = 0; j < split.files.length; j++) {
            var file = split.files[j];
            var checkout = gitExec(dir, ['checkout', plan.sourceBranch, '--', file]);
            if (checkout.code !== 0) {
                // Classify the error for conflict handling.
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

        // Stage and commit
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

        // Record SHA
        var sha = gitExec(dir, ['rev-parse', 'HEAD']);
        splitResult.sha = sha.code === 0 ? sha.stdout.trim() : '';

        results.push(splitResult);

        // Current split becomes base for next (stacking)
        currentBase = split.name;
    }

    // Restore original branch
    gitExec(dir, ['checkout', restoreBranch]);

    return { error: null, results: results };
};

// ---------------------------------------------------------------------------
//  Verification
// ---------------------------------------------------------------------------

// verifySplit runs a verification command against a specific split branch.
//
// Returns: { name: string, passed: bool, output: string, error: string|null }
exports.verifySplit = function(branchName, config) {
    config = config || {};
    var dir = config.dir || '.';
    var command = config.verifyCommand || 'make test';

    // Checkout the branch
    var co = gitExec(dir, ['checkout', branchName]);
    if (co.code !== 0) {
        return {
            name: branchName,
            passed: false,
            output: '',
            error: 'checkout failed: ' + co.stderr.trim()
        };
    }

    // Run verify command in the repo directory
    var result = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && ' + command]);

    return {
        name: branchName,
        passed: result.code === 0,
        output: result.stdout,
        error: result.code !== 0 ? 'verify failed (exit ' + result.code + '): ' + result.stderr : null
    };
};

// verifySplits runs verification on all split branches in the plan.
//
// Returns: { allPassed: bool, results: [{name, passed, output, error}] }
exports.verifySplits = function(plan) {
    var dir = plan.dir || '.';
    var results = [];
    var allPassed = true;

    // Save current branch
    var saved = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    var restoreBranch = saved.code === 0 ? saved.stdout.trim() : plan.sourceBranch;

    for (var i = 0; i < plan.splits.length; i++) {
        var result = exports.verifySplit(plan.splits[i].name, {
            dir: dir,
            verifyCommand: plan.verifyCommand
        });
        results.push(result);
        if (!result.passed) {
            allPassed = false;
        }
    }

    // Restore branch
    gitExec(dir, ['checkout', restoreBranch]);

    return { allPassed: allPassed, results: results };
};

// verifyEquivalence checks that the last split branch has the same tree
// as the source branch. A correctly executed stacked split produces a
// final branch with identical content to the source.
//
// Returns:
//   { equivalent: bool, splitTree: string, sourceTree: string,
//     error: string|null }
exports.verifyEquivalence = function(plan) {
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

    // Get tree hash of last split
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

    // Get tree hash of source branch
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
};

// verifyEquivalenceDetailed performs tree-hash equivalence checking and,
// on mismatch, provides a detailed diff showing which files differ between
// the final split branch and the source branch.
//
// Returns:
//   { equivalent, splitTree, sourceTree, error,
//     diffFiles: string[], diffSummary: string }
exports.verifyEquivalenceDetailed = function(plan) {
    var dir = plan.dir || '.';
    var base = exports.verifyEquivalence(plan);

    if (base.error || base.equivalent) {
        base.diffFiles = [];
        base.diffSummary = '';
        return base;
    }

    // Trees differ — find which files are different.
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
};

// cleanupBranches deletes all split branches created by a plan.
//
// Returns: { deleted: string[], errors: string[] }
exports.cleanupBranches = function(plan) {
    var dir = plan.dir || '.';
    var deleted = [];
    var errors = [];

    // Ensure we are not on any split branch before deleting
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
};

// ---------------------------------------------------------------------------
//  BT Integration — Workflow Nodes
// ---------------------------------------------------------------------------
// These functions create BT leaf nodes that store state on a Blackboard.
//
// Blackboard keys used by the workflow:
//   'splitConfig'     — SplitConfig object (set before workflow)
//   'analysisResult'  — result from analyzeDiff
//   'fileGroups'      — result from grouping strategy
//   'splitPlan'       — the SplitPlan object
//   'splitResults'    — results from executeSplit
//   'verifyResults'   — results from verifySplits
//   'equivalence'     — result from verifyEquivalence
//   'lastError'       — last error message

// createAnalyzeNode creates a BT leaf that analyzes the diff.
exports.createAnalyzeNode = function(bb, config) {
    return bt.createBlockingLeafNode(function() {
        var result = exports.analyzeDiff(config);
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
};

// createGroupNode creates a BT leaf that groups files.
//
// strategy: 'directory' | 'extension' | 'chunks'
// options:  { depth?, maxPerGroup? }
exports.createGroupNode = function(bb, strategy, options) {
    options = options || {};
    return bt.createBlockingLeafNode(function() {
        var analysis = bb.get('analysisResult');
        if (!analysis || !analysis.files) {
            bb.set('lastError', 'no analysis result available');
            return bt.failure;
        }

        var groups;
        switch (strategy) {
            case 'directory':
                groups = exports.groupByDirectory(analysis.files, options.depth);
                break;
            case 'extension':
                groups = exports.groupByExtension(analysis.files);
                break;
            case 'chunks':
                groups = exports.groupByChunks(analysis.files, options.maxPerGroup);
                break;
            default:
                groups = exports.groupByDirectory(analysis.files, 1);
        }

        bb.set('fileGroups', groups);
        return bt.success;
    });
};

// createPlanNode creates a BT leaf that builds a split plan from groups.
exports.createPlanNode = function(bb, config) {
    return bt.createBlockingLeafNode(function() {
        var groups = bb.get('fileGroups');
        if (!groups) {
            bb.set('lastError', 'no file groups available');
            return bt.failure;
        }

        var analysis = bb.get('analysisResult');
        var planConfig = {};

        // Copy config values
        if (config) {
            var keys = Object.keys(config);
            for (var i = 0; i < keys.length; i++) {
                planConfig[keys[i]] = config[keys[i]];
            }
        }

        // Merge analysis data
        if (analysis) {
            if (!planConfig.sourceBranch) {
                planConfig.sourceBranch = analysis.currentBranch;
            }
            if (!planConfig.baseBranch) {
                planConfig.baseBranch = analysis.baseBranch;
            }
        }

        var plan = exports.createSplitPlan(groups, planConfig);
        var validation = exports.validatePlan(plan);
        if (!validation.valid) {
            bb.set('lastError', 'invalid plan: ' + validation.errors.join('; '));
            return bt.failure;
        }

        bb.set('splitPlan', plan);
        return bt.success;
    });
};

// createSplitNode creates a BT leaf that executes the split plan.
exports.createSplitNode = function(bb) {
    return bt.createBlockingLeafNode(function() {
        var plan = bb.get('splitPlan');
        if (!plan) {
            bb.set('lastError', 'no split plan available');
            return bt.failure;
        }

        var result = exports.executeSplit(plan);
        bb.set('splitResults', result);
        if (result.error) {
            bb.set('lastError', result.error);
            return bt.failure;
        }
        return bt.success;
    });
};

// createVerifyNode creates a BT leaf that verifies all split branches.
exports.createVerifyNode = function(bb) {
    return bt.createBlockingLeafNode(function() {
        var plan = bb.get('splitPlan');
        if (!plan) {
            bb.set('lastError', 'no split plan available');
            return bt.failure;
        }

        var result = exports.verifySplits(plan);
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
};

// createEquivalenceNode creates a BT leaf that verifies tree equivalence.
exports.createEquivalenceNode = function(bb) {
    return bt.createBlockingLeafNode(function() {
        var plan = bb.get('splitPlan');
        if (!plan) {
            bb.set('lastError', 'no split plan available');
            return bt.failure;
        }

        var result = exports.verifyEquivalence(plan);
        bb.set('equivalence', result);
        if (!result.equivalent) {
            bb.set('lastError',
                'tree mismatch: split=' + result.splitTree +
                ' source=' + result.sourceTree);
            return bt.failure;
        }
        return bt.success;
    });
};

// createWorkflowTree creates a complete BT tree for the PR split workflow.
//
// The tree executes: analyze → group → plan → split → equivalence
//
// config: {
//   baseBranch:     string  (default: 'main')
//   dir:            string  (default: '.')
//   groupStrategy:  string  (default: 'directory')
//   groupOptions:   object  (default: {})
//   branchPrefix:   string  (default: 'split/')
//   verifyCommand:  string  (default: 'make test')
// }
exports.createWorkflowTree = function(bb, config) {
    config = config || {};

    var analyzeConfig = {
        baseBranch: config.baseBranch,
        dir: config.dir
    };

    var planConfig = {
        baseBranch: config.baseBranch,
        dir: config.dir,
        branchPrefix: config.branchPrefix,
        verifyCommand: config.verifyCommand
    };

    return bt.node(bt.sequence,
        exports.createAnalyzeNode(bb, analyzeConfig),
        exports.createGroupNode(bb, config.groupStrategy || 'directory', config.groupOptions),
        exports.createPlanNode(bb, planConfig),
        exports.createSplitNode(bb),
        exports.createEquivalenceNode(bb)
    );
};

// createSelectStrategyNode creates a BT leaf that selects the best grouping
// strategy using the claudemux ChoiceResolver. Stores the selected strategy
// and resulting file groups on the blackboard.
exports.createSelectStrategyNode = function(bb, options) {
    return bt.createBlockingLeafNode(function() {
        var analysis = bb.get('analysisResult');
        if (!analysis || !analysis.files || analysis.files.length === 0) {
            bb.set('lastError', 'no analysis result available for strategy selection');
            return bt.failure;
        }

        var result = exports.selectStrategy(analysis.files, options);
        bb.set('selectedStrategy', result);
        bb.set('fileGroups', result.groups);
        return bt.success;
    });
};

// ---------------------------------------------------------------------------
//  BT Integration — ClaudeMux AI Workflow Nodes
// ---------------------------------------------------------------------------
// These nodes use AI-powered classification and planning via claudemux.
// They require: claudemux module, a provider registry on the blackboard.
//
// Additional blackboard keys:
//   'claudemuxRegistry' — claudemux provider Registry (set before workflow)
//   'providerName'      — provider name (default: 'claude-code')
//   'classification'    — result from classifyChangesWithClaudeMux
//   'aiSplitPlan'       — result from suggestSplitPlanWithClaudeMux

// createClaudeMuxClassifyNode creates a BT leaf that classifies files using AI.
//
// Reads:  analysisResult, claudemuxRegistry, providerName
// Writes: classification, lastError
exports.createClaudeMuxClassifyNode = function(bb, options) {
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

        var result = exports.classifyChangesWithClaudeMux(analysis.files, opts);
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
};

// createClaudeMuxPlanNode creates a BT leaf that suggests a split plan using AI.
//
// Reads:  analysisResult, classification, claudemuxRegistry, providerName
// Writes: fileGroups (converted from stages), splitPlan, lastError
exports.createClaudeMuxPlanNode = function(bb, options) {
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

        var result = exports.suggestSplitPlanWithClaudeMux(analysis.files, classification, opts);
        if (result.error) {
            bb.set('lastError', 'planning: ' + result.error);
            return bt.failure;
        }
        if (!result.stages || result.stages.length === 0) {
            bb.set('lastError', 'planning returned no stages');
            return bt.failure;
        }

        bb.set('aiSplitPlan', result.stages);

        // Convert stages to groups format for compatibility with createPlanNode.
        var groups = {};
        for (var i = 0; i < result.stages.length; i++) {
            var stage = result.stages[i];
            groups[stage.name] = stage.files;
        }
        bb.set('fileGroups', groups);

        return bt.success;
    });
};

// createClaudeMuxWorkflowTree creates a complete AI-powered BT tree.
//
// The tree executes:
//   analyze → claudemux-classify → claudemux-plan → plan → split → equivalence
//
// config: {
//   baseBranch:     string  (default: 'main')
//   dir:            string  (default: '.')
//   branchPrefix:   string  (default: 'split/')
//   verifyCommand:  string  (default: 'make test')
//   classifyOpts:   object  (options for classifyChangesWithClaudeMux)
//   planOpts:       object  (options for suggestSplitPlanWithClaudeMux)
// }
//
// Before running, set on the blackboard:
//   bb.set('claudemuxRegistry', registry);
//   bb.set('providerName', 'ollama');  // optional
exports.createClaudeMuxWorkflowTree = function(bb, config) {
    config = config || {};

    var analyzeConfig = {
        baseBranch: config.baseBranch,
        dir: config.dir
    };

    var planConfig = {
        baseBranch: config.baseBranch,
        dir: config.dir,
        branchPrefix: config.branchPrefix,
        verifyCommand: config.verifyCommand
    };

    return bt.node(bt.sequence,
        exports.createAnalyzeNode(bb, analyzeConfig),
        exports.createClaudeMuxClassifyNode(bb, config.classifyOpts),
        exports.createClaudeMuxPlanNode(bb, config.planOpts),
        exports.createPlanNode(bb, planConfig),
        exports.createSplitNode(bb),
        exports.createEquivalenceNode(bb)
    );
};

// ---------------------------------------------------------------------------
//  Module version
// ---------------------------------------------------------------------------
exports.VERSION = '4.0.0';
