// pr_split_script.js — Built-in PR splitting command.
// Consolidates orchestrate-pr-split.js and bt-templates/claude-mux.js into
// a single embedded script exposed as `osm pr-split`.
//
// This script is loaded by the Go command via //go:embed; it receives injected
// globals from the Go side:
//   prSplitConfig  — {baseBranch, strategy, maxFiles, branchPrefix, verifyCommand, dryRun}
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

// Optional: os module for file I/O (plan persistence).
var osmod;
try {
    osmod = require('osm:os');
} catch (e) {
    osmod = null;
}

// Optional: time module for non-busy sleep.
var timemod;
try {
    timemod = require('osm:time');
} catch (e) {
    timemod = null;
}

// Optional: lipgloss for styled terminal output.
var lip;
try {
    lip = require('osm:lipgloss');
} catch (e) {
    lip = null;
}

// ---------------------------------------------------------------------------
//  Styled Output Helpers
// ---------------------------------------------------------------------------

// style creates terminal-styled text using Lipgloss when available.
// Degrades to plain text when Lipgloss is not loaded.
var style = (function() {
    if (!lip) {
        // No-op styles: return text unchanged.
        return {
            success: function(s) { return s; },
            error: function(s) { return s; },
            warning: function(s) { return s; },
            info: function(s) { return s; },
            header: function(s) { return s; },
            dim: function(s) { return s; },
            bold: function(s) { return s; },
            progressBar: function(current, total, width) {
                width = width || 20;
                var filled = total > 0 ? Math.round((current / total) * width) : 0;
                var bar = '';
                for (var i = 0; i < width; i++) {
                    bar += i < filled ? '█' : '░';
                }
                return '[' + bar + '] ' + current + '/' + total;
            }
        };
    }

    var successStyle = lip.newStyle().foreground('#22c55e').bold();
    var errorStyle = lip.newStyle().foreground('#ef4444').bold();
    var warningStyle = lip.newStyle().foreground('#f59e0b');
    var infoStyle = lip.newStyle().foreground('#3b82f6');
    var headerStyle = lip.newStyle().foreground('#a78bfa').bold();
    var dimStyle = lip.newStyle().foreground('#6b7280');
    var boldStyle = lip.newStyle().bold();
    var barFilledStyle = lip.newStyle().foreground('#22c55e');
    var barEmptyStyle = lip.newStyle().foreground('#374151');

    return {
        success: function(s) { return successStyle.render(s); },
        error: function(s) { return errorStyle.render(s); },
        warning: function(s) { return warningStyle.render(s); },
        info: function(s) { return infoStyle.render(s); },
        header: function(s) { return headerStyle.render(s); },
        dim: function(s) { return dimStyle.render(s); },
        bold: function(s) { return boldStyle.render(s); },
        progressBar: function(current, total, width) {
            width = width || 20;
            var filled = total > 0 ? Math.round((current / total) * width) : 0;
            var bar = '';
            for (var i = 0; i < width; i++) {
                if (i < filled) {
                    bar += barFilledStyle.render('█');
                } else {
                    bar += barEmptyStyle.render('░');
                }
            }
            return '[' + bar + '] ' + current + '/' + total;
        }
    };
})();

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
    jsonOutput:    cfg.jsonOutput    || false,
    mode:          cfg.mode          || 'heuristic',   // 'auto' or 'heuristic'
    retryBudget:   typeof cfg.retryBudget === 'number' ? cfg.retryBudget : 3,
    view:          cfg.view          || 'toggle',      // 'toggle' or 'split'
    autoMerge:     cfg.autoMerge     || false,         // enable merge queue auto-merge
    mergeMethod:   cfg.mergeMethod   || 'squash'       // squash, merge, rebase
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

    var emptyResult = function(error, currentBranch) {
        return {
            files: [],
            fileStatuses: {},
            error: error,
            baseBranch: baseBranch,
            currentBranch: currentBranch || ''
        };
    };

    var branchResult = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    if (branchResult.code !== 0) {
        return emptyResult('failed to get current branch: ' + branchResult.stderr.trim(), '');
    }
    var currentBranch = branchResult.stdout.trim();

    var mergeBase = gitExec(dir, ['merge-base', baseBranch, currentBranch]);
    if (mergeBase.code !== 0) {
        return emptyResult('merge-base failed: ' + mergeBase.stderr.trim(), currentBranch);
    }

    // Use --name-status to capture diff status (A/M/D/R/C) per file.
    var diffResult = gitExec(dir, ['diff', '--name-status', mergeBase.stdout.trim(), currentBranch]);
    if (diffResult.code !== 0) {
        return emptyResult('git diff failed: ' + diffResult.stderr.trim(), currentBranch);
    }

    var raw = diffResult.stdout.trim();
    var files = [];
    var fileStatuses = {};

    // Valid status codes that executeSplit knows how to handle.
    var KNOWN_STATUSES = { A: 1, M: 1, D: 1, R: 1, C: 1, T: 1 };

    if (raw !== '') {
        var lines = raw.split('\n');
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            if (line === '') continue;
            // Format: STATUS\tPATH (or STATUS\tOLD\tNEW for R/C)
            var parts = line.split('\t');
            if (parts.length < 2) continue;
            var status = parts[0].charAt(0);

            // Unmerged paths (U) mean unresolved conflicts — fail early.
            if (status === 'U') {
                return emptyResult(
                    'unmerged path detected: ' + parts[1] +
                    ' — resolve all merge conflicts before splitting',
                    currentBranch
                );
            }

            // Skip unknown status codes with a warning.
            if (!KNOWN_STATUSES[status]) {
                log.warn('pr-split: unknown git status "' + parts[0] + '" for ' + parts[1] + ' — skipping');
                continue;
            }

            if (parts.length >= 3 && (status === 'R' || status === 'C')) {
                // Rename/copy: track ONLY the new (destination) path.
                // The old path is irrelevant — the source branch has the
                // final state, and executeSplit operates on source branch content.
                var newPath = parts[2];
                files.push(newPath);
                fileStatuses[newPath] = status;
            } else {
                var path = parts[1];
                files.push(path);
                fileStatuses[path] = status;
            }
        }
    }

    return {
        files: files,
        fileStatuses: fileStatuses,
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
    if (!files || !files.length) return {};
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
    if (!files || !files.length) return {};
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
    if (!files || !files.length) return {};
    if (!patterns || typeof patterns !== 'object') return { '(other)': (files || []).slice() };
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
    if (!files || !files.length) return {};
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

// ---------------------------------------------------------------------------
//  Dependency-Aware Grouping (Go)
// ---------------------------------------------------------------------------

// parseGoImports extracts import paths from Go source code.
// Handles single-line (`import "path"`) and block (`import ( ... )`) forms.
// Stops parsing at the first func/type/var/const declaration for efficiency.
function parseGoImports(content) {
    if (!content || typeof content !== 'string') return [];
    var imports = [];
    var lines = content.split('\n');
    var inBlock = false;

    for (var i = 0; i < lines.length; i++) {
        var line = lines[i].trim();

        // Single import: import "path"
        if (!inBlock && line.indexOf('import ') === 0 && line.indexOf('(') === -1) {
            var q1 = line.indexOf('"');
            if (q1 >= 0) {
                var q2 = line.indexOf('"', q1 + 1);
                if (q2 > q1) {
                    imports.push(line.substring(q1 + 1, q2));
                }
            }
            continue;
        }

        // Block import start: `import (`  or  `import(`
        if (!inBlock && line.indexOf('import') === 0 && line.indexOf('(') >= 0) {
            inBlock = true;
            // Check if there's also an import on the same line as the (
            var qi = line.indexOf('"');
            if (qi >= 0) {
                var qi2 = line.indexOf('"', qi + 1);
                if (qi2 > qi) {
                    imports.push(line.substring(qi + 1, qi2));
                }
            }
            continue;
        }

        // Block import end
        if (inBlock && line.indexOf(')') >= 0) {
            inBlock = false;
            continue;
        }

        // Inside block import: `"path"` or `alias "path"`
        if (inBlock) {
            var qs = line.indexOf('"');
            if (qs >= 0) {
                var qe = line.indexOf('"', qs + 1);
                if (qe > qs) {
                    imports.push(line.substring(qs + 1, qe));
                }
            }
            continue;
        }

        // Stop at first declaration (imports must precede declarations).
        if (line.indexOf('func ') === 0 || line.indexOf('type ') === 0 ||
            line.indexOf('var ') === 0 || line.indexOf('const ') === 0 ||
            line.indexOf('var(') === 0 || line.indexOf('const(') === 0) {
            break;
        }
    }

    return imports;
}

// detectGoModulePath reads go.mod from the current directory and returns the
// module path, or '' if not a Go module.
function detectGoModulePath() {
    var content = '';
    if (osmod) {
        var result = osmod.readFile('go.mod');
        if (result.error) {
            return '';
        }
        content = result.content;
    } else {
        // Fallback for environments without osm:os module.
        var result = exec.execv(['cat', 'go.mod']);
        if (result.code !== 0) {
            return '';
        }
        content = result.stdout;
    }
    var lines = content.split('\n');
    for (var i = 0; i < lines.length; i++) {
        var line = lines[i].trim();
        if (line.indexOf('module ') === 0) {
            return line.substring(7).trim();
        }
    }
    return '';
}

// groupByDependency groups changed files using dependency-aware heuristics:
//   1. Go files are grouped by package directory (same dir = same package).
//   2. If package A imports package B and both have changes in the
//      changeset, they are merged into a single group (union-find).
//   3. Test files (_test.go) naturally stay with their package.
//   4. Non-Go files are placed into the nearest matching group or their
//      own directory-based group.
// Falls back to directory grouping for non-Go projects.
function groupByDependency(files, options) {
    if (!files || !files.length) return {};
    options = options || {};

    // Separate Go files from non-Go files.
    var goFiles = [];
    var otherFiles = [];
    for (var i = 0; i < files.length; i++) {
        if (files[i].length > 3 && files[i].substring(files[i].length - 3) === '.go') {
            goFiles.push(files[i]);
        } else {
            otherFiles.push(files[i]);
        }
    }

    // No Go files → fall back to directory grouping.
    if (goFiles.length === 0) {
        return groupByDirectory(files, 1);
    }

    // Group Go files by package directory.
    var pkgFiles = {};
    for (var i = 0; i < goFiles.length; i++) {
        var parts = goFiles[i].split('/');
        var pkg = parts.length > 1 ? parts.slice(0, -1).join('/') : '.';
        if (!pkgFiles[pkg]) {
            pkgFiles[pkg] = [];
        }
        pkgFiles[pkg].push(goFiles[i]);
    }
    var pkgDirs = Object.keys(pkgFiles);

    // Detect Go module path from go.mod (for resolving intra-module imports).
    var modulePath = detectGoModulePath();

    // Union-Find: merge packages that have import relationships.
    var parent = {};
    for (var i = 0; i < pkgDirs.length; i++) {
        parent[pkgDirs[i]] = pkgDirs[i];
    }

    function find(x) {
        while (parent[x] !== x) {
            parent[x] = parent[parent[x]]; // path halving
            x = parent[x];
        }
        return x;
    }

    function union(a, b) {
        var ra = find(a);
        var rb = find(b);
        if (ra !== rb) {
            parent[ra] = rb;
        }
    }

    // Parse imports from each changed Go file and merge related packages.
    if (modulePath && pkgDirs.length > 1) {
        for (var i = 0; i < goFiles.length; i++) {
            // Skip test files for import analysis (they often import the
            // package under test, which is in the same directory anyway).
            if (goFiles[i].substring(goFiles[i].length - 8) === '_test.go') {
                continue;
            }

            var filePkgParts = goFiles[i].split('/');
            var filePkg = filePkgParts.length > 1
                ? filePkgParts.slice(0, -1).join('/')
                : '.';

            // Read file and parse imports.
            var catResult = exec.execv(['cat', goFiles[i]]);
            if (catResult.code !== 0) {
                continue;
            }

            var imports = parseGoImports(catResult.stdout);
            for (var j = 0; j < imports.length; j++) {
                var imp = imports[j];
                // Only consider intra-module imports.
                if (imp.indexOf(modulePath + '/') !== 0) {
                    continue;
                }
                var relPath = imp.substring(modulePath.length + 1);

                // If this imported package is in the changeset, merge them.
                if (parent[relPath] !== undefined) {
                    union(filePkg, relPath);
                }
            }
        }
    }

    // Build groups from union-find roots.
    var groups = {};
    for (var i = 0; i < pkgDirs.length; i++) {
        var root = find(pkgDirs[i]);
        if (!groups[root]) {
            groups[root] = [];
        }
        var fileList = pkgFiles[pkgDirs[i]];
        for (var j = 0; j < fileList.length; j++) {
            groups[root].push(fileList[j]);
        }
    }

    // Place non-Go files into nearest matching group or separate group.
    for (var i = 0; i < otherFiles.length; i++) {
        var otherParts = otherFiles[i].split('/');
        var otherDir = otherParts.length > 1
            ? otherParts.slice(0, -1).join('/')
            : '.';

        var placed = false;

        // Try exact directory match first.
        if (groups[otherDir]) {
            groups[otherDir].push(otherFiles[i]);
            placed = true;
        }

        // Try to find a group whose root matches after find().
        if (!placed && parent[otherDir] !== undefined) {
            var resolved = find(otherDir);
            if (groups[resolved]) {
                groups[resolved].push(otherFiles[i]);
                placed = true;
            }
        }

        // Fall back: create a directory-based group.
        if (!placed) {
            var fallbackDir = dirname(otherFiles[i], 1);
            if (!groups[fallbackDir]) {
                groups[fallbackDir] = [];
            }
            groups[fallbackDir].push(otherFiles[i]);
        }
    }

    return groups;
}

// applyStrategy selects and applies a grouping strategy.
function applyStrategy(files, strategy, options) {
    if (!files || !files.length) return {};
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
        case 'dependency':
            return groupByDependency(files, options);
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
        { name: 'chunks', groups: groupByChunks(files, maxPerGroup) },
        { name: 'dependency', groups: groupByDependency(files, options) }
    ];

    // Score strategies heuristically and pick the best one.
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

        // Compute a composite score inline.
        var splitScore;
        var n = groupNames.length;
        if (n <= 0) splitScore = 0;
        else if (n >= 3 && n <= 7) splitScore = 1.0;
        else if (n < 3) splitScore = n / 3;
        else splitScore = Math.max(0, 1.0 - (n - 7) * 0.1);

        var maxSizeScore;
        if (maxGroupSize <= maxPerGroup) maxSizeScore = 1.0;
        else maxSizeScore = Math.max(0, 1.0 - (maxGroupSize - maxPerGroup) * 0.05);

        var compositeScore = splitScore * 0.4 + balance * 0.3 + maxSizeScore * 0.3;

        candidates.push({
            name: s.name,
            groups: s.groups,
            score: compositeScore,
            groupCount: groupNames.length,
            maxGroupSize: maxGroupSize
        });
    }

    // Sort by composite score descending.
    candidates.sort(function(a, b) { return b.score - a.score; });

    var winner = candidates[0];
    return {
        strategy: winner.name,
        groups: winner.groups,
        reason: winner.name + ': ' + winner.groupCount + ' groups, max ' + winner.maxGroupSize + ' files (score ' + Math.round(winner.score * 100) / 100 + ')',
        needsConfirm: candidates.length > 1 && candidates[0].score - candidates[1].score < 0.15,
        scored: candidates.map(function(c) { return { name: c.name, score: c.score }; })
    };
}

// ---------------------------------------------------------------------------
//  Planning
// ---------------------------------------------------------------------------

function createSplitPlan(groups, config) {
    if (!groups || typeof groups !== 'object') groups = {};
    config = config || {};
    var dir = config.dir || '.';
    var baseBranch = config.baseBranch || runtime.baseBranch;
    var branchPrefix = config.branchPrefix || runtime.branchPrefix;
    var commitPrefix = config.commitPrefix || '';
    var verifyCommand = config.verifyCommand || runtime.verifyCommand;
    var fileStatuses = config.fileStatuses || {};

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
        fileStatuses: fileStatuses,
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
//  Plan Persistence
// ---------------------------------------------------------------------------

var DEFAULT_PLAN_PATH = '.pr-split-plan.json';

// savePlan serializes the current plan, analysis, and execution state to a
// JSON file so a future session can resume where this one left off.
function savePlan(path) {
    path = path || DEFAULT_PLAN_PATH;
    if (!osmod) {
        return { error: 'osm:os module not available — cannot persist plan' };
    }
    if (!planCache) {
        return { error: 'no plan to save — run "plan" or "run" first' };
    }

    var snapshot = {
        version: 1,
        savedAt: new Date().toISOString(),
        runtime: {
            baseBranch:    runtime.baseBranch,
            strategy:      runtime.strategy,
            maxFiles:      runtime.maxFiles,
            branchPrefix:  runtime.branchPrefix,
            verifyCommand: runtime.verifyCommand,
            dryRun:        runtime.dryRun
        },
        analysis: analysisCache ? {
            files: analysisCache.files,
            fileStatuses: analysisCache.fileStatuses,
            baseBranch: analysisCache.baseBranch,
            currentBranch: analysisCache.currentBranch
        } : null,
        groups: groupsCache,
        plan: planCache,
        executed: executionResultCache || [],
        conversations: getConversationHistory()
    };

    try {
        osmod.writeFile(path, JSON.stringify(snapshot, null, 2));
        return { path: path, error: null };
    } catch (e) {
        return { error: 'failed to write plan: ' + String(e) };
    }
}

// loadPlan reads a previously-saved plan snapshot from disk and restores the
// analysis, groups, plan, and execution state.
function loadPlan(path) {
    path = path || DEFAULT_PLAN_PATH;
    if (!osmod) {
        return { error: 'osm:os module not available — cannot load plan' };
    }

    var result = osmod.readFile(path);
    if (result.error) {
        return { error: 'failed to read plan: ' + result.error };
    }

    var snapshot;
    try {
        snapshot = JSON.parse(result.content);
    } catch (e) {
        return { error: 'invalid JSON in plan file: ' + String(e) };
    }

    if (!snapshot.version || snapshot.version < 1) {
        return { error: 'unsupported plan version: ' + String(snapshot.version) };
    }
    if (!snapshot.plan || !snapshot.plan.splits) {
        return { error: 'plan file missing splits' };
    }

    // Restore state.
    if (snapshot.runtime) {
        runtime.baseBranch    = snapshot.runtime.baseBranch    || runtime.baseBranch;
        runtime.strategy      = snapshot.runtime.strategy      || runtime.strategy;
        runtime.maxFiles      = snapshot.runtime.maxFiles      || runtime.maxFiles;
        runtime.branchPrefix  = snapshot.runtime.branchPrefix  || runtime.branchPrefix;
        runtime.verifyCommand = snapshot.runtime.verifyCommand || runtime.verifyCommand;
        if (snapshot.runtime.dryRun !== undefined) {
            runtime.dryRun = snapshot.runtime.dryRun;
        }
    }

    if (snapshot.analysis) {
        analysisCache = snapshot.analysis;
    }
    if (snapshot.groups) {
        groupsCache = snapshot.groups;
    }
    planCache = snapshot.plan;
    executionResultCache = snapshot.executed || [];

    // Restore conversation history if present.
    if (snapshot.conversations && Array.isArray(snapshot.conversations)) {
        for (var c = 0; c < snapshot.conversations.length; c++) {
            conversationHistory.push(snapshot.conversations[c]);
        }
    }

    return {
        path: path,
        error: null,
        totalSplits: planCache.splits.length,
        executedSplits: executionResultCache.length,
        pendingSplits: planCache.splits.length - executionResultCache.length
    };
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

    // fileStatuses is REQUIRED — it determines whether each file
    // should be checked out (A/M/R/C/T) or removed (D).
    if (!plan.fileStatuses || typeof plan.fileStatuses !== 'object') {
        return {
            error: 'plan.fileStatuses is required — pass fileStatuses from analyzeDiff() to createSplitPlan()',
            results: []
        };
    }
    var fileStatuses = plan.fileStatuses;

    var savedBranch = gitExec(dir, ['rev-parse', '--abbrev-ref', 'HEAD']);
    if (savedBranch.code !== 0) {
        return { error: 'failed to get current branch', results: [] };
    }
    var restoreBranch = savedBranch.stdout.trim();

    // Pre-flight: delete any pre-existing split branches to allow re-runs.
    for (var k = 0; k < plan.splits.length; k++) {
        var existCheck = gitExec(dir, ['rev-parse', '--verify', 'refs/heads/' + plan.splits[k].name]);
        if (existCheck.code === 0) {
            // Branch exists — delete it so we can recreate cleanly.
            gitExec(dir, ['branch', '-D', plan.splits[k].name]);
        }
    }

    var currentBase = plan.baseBranch;

    for (var i = 0; i < plan.splits.length; i++) {
        // Intra-step cancellation: check between each branch creation.
        if (isCancelled()) {
            gitExec(dir, ['checkout', restoreBranch]);
            return { error: 'cancelled by user after ' + i + ' of ' + plan.splits.length + ' branches', results: results };
        }

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
            var status = fileStatuses[file];

            if (!status) {
                splitResult.error = 'file "' + file + '" has no entry in plan.fileStatuses — '
                    + 'ensure analyzeDiff() results are passed to createSplitPlan()';
                results.push(splitResult);
                gitExec(dir, ['checkout', restoreBranch]);
                return { error: splitResult.error, results: results };
            }

            if (status === 'D') {
                // File was deleted in the source branch — remove it from the split.
                var rm = gitExec(dir, ['rm', '--ignore-unmatch', '-f', file]);
                if (rm.code !== 0) {
                    splitResult.error = 'git rm ' + file + ': ' + rm.stderr.trim();
                    results.push(splitResult);
                    gitExec(dir, ['checkout', restoreBranch]);
                    return { error: splitResult.error, results: results };
                }
            } else {
                // File was added, modified, renamed-to, etc. — checkout from source.
                var checkout = gitExec(dir, ['checkout', plan.sourceBranch, '--', file]);
                if (checkout.code !== 0) {
                    splitResult.error = 'checkout file ' + file + ': ' + checkout.stderr.trim();
                    results.push(splitResult);
                    gitExec(dir, ['checkout', restoreBranch]);
                    return { error: splitResult.error, results: results };
                }
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
            // It is possible that the split has no effective changes
            // (e.g. all files in this group are deletions that don't exist
            // on the base branch). Allow empty commits.
            var commitAllow = gitExec(dir, ['commit', '--allow-empty', '-m', msg]);
            if (commitAllow.code !== 0) {
                splitResult.error = 'git commit failed: ' + commitAllow.stderr.trim();
                results.push(splitResult);
                gitExec(dir, ['checkout', restoreBranch]);
                return { error: splitResult.error, results: results };
            }
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
        // Intra-step cancellation: check between each branch verification.
        if (isCancelled()) {
            gitExec(dir, ['checkout', restoreBranch]);
            return { allPassed: false, results: results, error: 'verification cancelled by user' };
        }

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
//  GitHub PR Creation
// ---------------------------------------------------------------------------

// createPRs pushes split branches and creates stacked GitHub PRs using gh CLI.
// Options:
//   draft:     bool   - Create as draft PRs (default: true)
//   remote:    string - Git remote to push to (default: 'origin')
//   pushOnly:  bool   - Push branches but don't create PRs
//   autoMerge: bool   - Enable auto-merge via merge queue (default: false)
//   mergeMethod: string - Merge method: squash, merge, rebase (default: 'squash')
function createPRs(plan, options) {
    options = options || {};
    var dir = plan.dir || '.';
    var draft = options.draft !== false;  // default true
    var remote = options.remote || 'origin';
    var pushOnly = options.pushOnly || false;
    var autoMerge = options.autoMerge || false;
    var mergeMethod = options.mergeMethod || 'squash';

    if (!plan.splits || plan.splits.length === 0) {
        return { error: 'no splits in plan', results: [] };
    }

    // Verify gh CLI is available (unless push-only mode).
    if (!pushOnly) {
        var ghCheck = exec.execv(['gh', '--version']);
        if (ghCheck.code !== 0) {
            return { error: 'gh CLI not found — install GitHub CLI (https://cli.github.com) or use --push-only', results: [] };
        }
    }

    var results = [];

    // Step 1: Push all split branches.
    for (var i = 0; i < plan.splits.length; i++) {
        var split = plan.splits[i];
        var pushResult = gitExec(dir, ['push', '-f', remote, split.name]);
        if (pushResult.code !== 0) {
            results.push({
                name: split.name,
                pushed: false,
                prUrl: '',
                error: 'push failed: ' + pushResult.stderr.trim()
            });
            return { error: 'push failed for ' + split.name + ': ' + pushResult.stderr.trim(), results: results };
        }
        results.push({
            name: split.name,
            pushed: true,
            prUrl: '',
            error: null
        });
    }

    if (pushOnly) {
        return { error: null, results: results };
    }

    // Step 2: Create PRs, stacked.
    for (var pi = 0; pi < plan.splits.length; pi++) {
        var split = plan.splits[pi];
        var base = pi === 0 ? plan.baseBranch : plan.splits[pi - 1].name;
        var title = '[' + padIndex(pi + 1) + '/' + padIndex(plan.splits.length) + '] ' + split.message;
        var body = 'Part ' + (pi + 1) + ' of ' + plan.splits.length + ' — auto-generated by `osm pr-split`.\n\n';
        body += '**Files:**\n';
        for (var j = 0; j < split.files.length; j++) {
            body += '- `' + split.files[j] + '`\n';
        }

        if (pi < plan.splits.length - 1) {
            body += '\n> ⚠️ **Stacked PR** — merge in order. Next: `' + plan.splits[pi + 1].name + '`';
        } else {
            body += '\n> ✅ **Last PR in stack** — merging this completes the split.';
        }

        var ghArgs = ['gh', 'pr', 'create',
            '--base', base,
            '--head', split.name,
            '--title', title,
            '--body', body
        ];
        if (draft) {
            ghArgs.push('--draft');
        }

        var ghResult = exec.execv(ghArgs);
        if (ghResult.code !== 0) {
            results[pi].error = 'gh pr create failed: ' + ghResult.stderr.trim();
            // Don't abort — continue creating remaining PRs.
        } else {
            results[pi].prUrl = ghResult.stdout.trim();
        }
    }

    // Step 3: Auto-merge via merge queue (if enabled).
    if (autoMerge) {
        for (var k = 0; k < plan.splits.length; k++) {
            var splitForMerge = plan.splits[k];
            if (results[k].error) continue; // Skip failed PRs.
            var mergeArgs = ['gh', 'pr', 'merge', splitForMerge.name,
                '--' + mergeMethod,
                '--auto'
            ];
            var mergeResult = exec.execv(mergeArgs);
            if (mergeResult.code !== 0) {
                results[k].mergeError = 'auto-merge failed: ' + mergeResult.stderr.trim();
            } else {
                results[k].autoMerge = true;
            }
        }
    }

    return { error: null, results: results };
}

// ---------------------------------------------------------------------------
//  Split Conflict Resolution
// ---------------------------------------------------------------------------

// AUTO_FIX_STRATEGIES defines sequential repair strategies to try when a
// split branch fails verification. Each strategy is {name, detect, fix}.
var AUTO_FIX_STRATEGIES = [
    {
        name: 'go-mod-tidy',
        // Detect: check if go.mod exists in the repo.
        detect: function(dir) {
            return exec.execv(['test', '-f', (dir !== '.' ? dir + '/' : '') + 'go.mod']).code === 0;
        },
        // Fix: run `go mod tidy` and commit if changes were made.
        fix: function(dir) {
            var tidyResult = exec.execv(['sh', '-c',
                'cd ' + shellQuote(dir) + ' && go mod tidy']);
            if (tidyResult.code !== 0) {
                return { fixed: false, error: 'go mod tidy failed: ' + tidyResult.stderr.trim() };
            }
            // Check if tidy changed anything.
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
        // Detect: go.sum exists but might be incomplete.
        detect: function(dir) {
            return exec.execv(['test', '-f', (dir !== '.' ? dir + '/' : '') + 'go.sum']).code === 0;
        },
        // Fix: run `go mod download` to populate go.sum.
        fix: function(dir) {
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
        // Detect: a Go project with build errors mentioning undefined names.
        detect: function(dir, verifyOutput) {
            if (!verifyOutput) return false;
            return verifyOutput.indexOf('undefined:') >= 0 ||
                   verifyOutput.indexOf('imported and not used') >= 0 ||
                   verifyOutput.indexOf('could not import') >= 0;
        },
        // Fix: run goimports to add missing imports and remove unused ones.
        fix: function(dir) {
            // Try goimports if available, fall back to go build attempt.
            var which = exec.execv(['which', 'goimports']);
            if (which.code === 0) {
                var result = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && find . -name "*.go" -exec goimports -w {} +']);
                if (result.code !== 0) {
                    return { fixed: false, error: 'goimports failed: ' + result.stderr.trim() };
                }
            } else {
                // Without goimports, try go build to see if it's just missing go.sum.
                return { fixed: false, error: 'goimports not available' };
            }
            var status = gitExec(dir, ['status', '--porcelain']);
            if (status.stdout.trim() === '') {
                return { fixed: false, error: 'goimports made no changes' };
            }
            gitExec(dir, ['add', '-A']);
            var commit = gitExec(dir, ['commit', '-m', 'fix: goimports for split']);
            if (commit.code !== 0) {
                return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
            }
            return { fixed: true, error: null };
        }
    },
    {
        name: 'npm-install',
        // Detect: package.json exists.
        detect: function(dir) {
            return exec.execv(['test', '-f', (dir !== '.' ? dir + '/' : '') + 'package.json']).code === 0;
        },
        // Fix: run `npm install` and commit changes to package lock.
        fix: function(dir) {
            var result = exec.execv(['sh', '-c',
                'cd ' + shellQuote(dir) + ' && npm install --no-audit --no-fund 2>&1']);
            if (result.code !== 0) {
                return { fixed: false, error: 'npm install failed: ' + (result.stderr || result.stdout || '').trim() };
            }
            var status = gitExec(dir, ['status', '--porcelain']);
            if (status.stdout.trim() === '') {
                return { fixed: false, error: 'npm install made no changes' };
            }
            gitExec(dir, ['add', '-A']);
            var commit = gitExec(dir, ['commit', '-m', 'fix: npm install for split']);
            if (commit.code !== 0) {
                return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
            }
            return { fixed: true, error: null };
        }
    },
    {
        name: 'make-generate',
        // Detect: Makefile with 'generate' target or go generate files.
        detect: function(dir) {
            var hasMakefile = exec.execv(['test', '-f', (dir !== '.' ? dir + '/' : '') + 'Makefile']).code === 0;
            if (hasMakefile) {
                var grep = exec.execv(['sh', '-c',
                    'cd ' + shellQuote(dir) + ' && grep -q "^generate:" Makefile']);
                if (grep.code === 0) return true;
            }
            // Check for //go:generate directives.
            var goGen = exec.execv(['sh', '-c',
                'cd ' + shellQuote(dir) + ' && grep -rl "//go:generate" --include="*.go" . 2>/dev/null | head -1']);
            return goGen.code === 0 && goGen.stdout.trim() !== '';
        },
        // Fix: run make generate or go generate.
        fix: function(dir) {
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
            gitExec(dir, ['add', '-A']);
            var commit = gitExec(dir, ['commit', '-m', 'fix: run code generation for split']);
            if (commit.code !== 0) {
                return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
            }
            return { fixed: true, error: null };
        }
    },
    {
        name: 'add-missing-files',
        // Detect: build/test output references files not in the split.
        detect: function(dir, verifyOutput) {
            if (!verifyOutput) return false;
            // Look for 'no such file or directory' or 'cannot find' patterns.
            return verifyOutput.indexOf('no such file or directory') >= 0 ||
                   verifyOutput.indexOf('cannot find') >= 0 ||
                   verifyOutput.indexOf('file not found') >= 0;
        },
        // Fix: check out missing files from the source branch.
        fix: function(dir, failedBranch, plan) {
            if (!plan || !plan.sourceBranch) {
                return { fixed: false, error: 'no source branch to pull files from' };
            }
            // Get file list from source not in current branch.
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
            gitExec(dir, ['add', '-A']);
            var commit = gitExec(dir, ['commit', '-m', 'fix: add missing files from source branch']);
            if (commit.code !== 0) {
                return { fixed: false, error: 'commit failed: ' + commit.stderr.trim() };
            }
            return { fixed: true, error: null };
        }
    },
    {
        name: 'claude-fix',
        // Detect: Claude executor is available and spawned.
        detect: function() {
            return !!(claudeExecutor && claudeExecutor.handle && claudeExecutor.isAvailable());
        },
        // Fix: send verification error to Claude for analysis and patching.
        fix: function(dir, failedBranch, plan, verifyOutput) {
            if (!claudeExecutor || !claudeExecutor.handle) {
                return { fixed: false, error: 'Claude executor not available' };
            }
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
            try {
                claudeExecutor.handle.send(promptResult.text);
            } catch (e) {
                return { fixed: false, error: 'failed to send to Claude: ' + (e.message || String(e)) };
            }
            // Poll for resolution.
            var resultDir = claudeExecutor.mcpInstance ? claudeExecutor.mcpInstance.resultDir() : '';
            if (!resultDir) {
                return { fixed: false, error: 'no result directory for Claude response' };
            }
            // Delete old resolution file.
            try { exec.execv(['rm', '-f', resultDir + '/resolution.json']); } catch (e) { /* ignore */ }
            var resolutionPoll = pollForFile(resultDir, 'resolution.json',
                AUTOMATED_DEFAULTS.resolveTimeoutMs, AUTOMATED_DEFAULTS.pollIntervalMs);
            if (resolutionPoll.error) {
                return { fixed: false, error: 'Claude resolution timeout: ' + resolutionPoll.error };
            }
            var resolution = resolutionPoll.data;
            // Apply patches.
            if (resolution.patches && resolution.patches.length > 0) {
                for (var p = 0; p < resolution.patches.length; p++) {
                    var patch = resolution.patches[p];
                    if (osmod) {
                        osmod.writeFile(patch.file, patch.content);
                    }
                }
            }
            // Run suggested commands.
            if (resolution.commands && resolution.commands.length > 0) {
                for (var c = 0; c < resolution.commands.length; c++) {
                    exec.execv(['sh', '-c', resolution.commands[c]]);
                }
            }
            // Commit changes.
            var status = gitExec(dir, ['status', '--porcelain']);
            if (status.stdout.trim() !== '') {
                gitExec(dir, ['add', '-A']);
                gitExec(dir, ['commit', '--amend', '--no-edit']);
            }
            return { fixed: true, error: null };
        }
    }
];

// resolveConflicts attempts to auto-fix split branches that fail verification.
// For each split:
//   1. Check out the branch.
//   2. Run the verify command.
//   3. If it fails, try auto-fix strategies in order (with retry budget).
//   4. After each fix, re-run verification.
//   5. If still fails and budget allows, continue trying.
//   6. If all strategies exhausted, flag for re-split.
//
// Options:
//   retryBudget: max total fix attempts across all branches (default: runtime.retryBudget || 3)
//   verifyCommand: override verify command
//   strategies: override strategy list (default: AUTO_FIX_STRATEGIES)
function resolveConflicts(plan, options) {
    options = options || {};
    var dir = plan.dir || '.';
    var verifyCommand = options.verifyCommand || plan.verifyCommand || runtime.verifyCommand;
    var retryBudget = typeof options.retryBudget === 'number' ? options.retryBudget : (typeof runtime.retryBudget === 'number' ? runtime.retryBudget : 3);
    var strategies = options.strategies || AUTO_FIX_STRATEGIES;
    var totalRetries = 0;

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
        if (totalRetries >= retryBudget) {
            errorsOut.push({ name: plan.splits[i].name, error: 'retry budget exhausted (' + retryBudget + ')' });
            continue;
        }

        var split = plan.splits[i];
        var co = gitExec(dir, ['checkout', split.name]);
        if (co.code !== 0) {
            errorsOut.push({ name: split.name, error: 'checkout failed: ' + co.stderr.trim() });
            continue;
        }

        // Run verify.
        var verifyResult = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && ' + verifyCommand]);
        if (verifyResult.code === 0) {
            // Already passes — no fix needed.
            continue;
        }

        var verifyOutput = (verifyResult.stdout || '') + '\n' + (verifyResult.stderr || '');

        // Verification failed — try auto-fix strategies.
        var resolved = false;
        for (var s = 0; s < strategies.length && totalRetries < retryBudget; s++) {
            var strategy = strategies[s];
            // Pass verifyOutput to detect for content-aware strategies.
            if (!strategy.detect(dir, verifyOutput)) {
                continue;
            }

            totalRetries++;
            var fixResult = strategy.fix(dir, split.name, plan, verifyOutput);
            if (!fixResult.fixed) {
                continue;
            }

            // Re-run verification.
            var reVerify = exec.execv(['sh', '-c', 'cd ' + shellQuote(dir) + ' && ' + verifyCommand]);
            if (reVerify.code === 0) {
                fixed.push({ name: split.name, strategy: strategy.name });
                resolved = true;
                break;
            }
            // Fix was applied but didn't resolve — update verifyOutput and continue trying.
            verifyOutput = (reVerify.stdout || '') + '\n' + (reVerify.stderr || '');
        }

        if (!resolved) {
            errorsOut.push({
                name: split.name,
                error: 'verification failed after all auto-fix strategies',
                lastOutput: verifyOutput
            });
            // Flag for re-split — collect problematic files.
            reSplitNeeded = true;
            reSplitFiles = reSplitFiles.concat(split.files || []);
        }
    }

    // Restore original branch.
    gitExec(dir, ['checkout', restoreBranch]);

    return {
        fixed: fixed,
        errors: errorsOut,
        totalRetries: totalRetries,
        reSplitNeeded: reSplitNeeded,
        reSplitFiles: reSplitFiles,
        reSplitReason: reSplitNeeded ? 'Auto-fix strategies exhausted for: ' + errorsOut.map(function(e) { return e.name; }).join(', ') : ''
    };
}

// ---------------------------------------------------------------------------
//  Claude Code Executor
// ---------------------------------------------------------------------------

// ClaudeCodeExecutor manages spawning and communicating with Claude Code.
// It reads configuration from prSplitConfig (claudeCommand, claudeArgs,
// claudeModel, claudeConfigDir, claudeEnv) and uses the osm:claudemux
// module for MCP integration.
function ClaudeCodeExecutor(config) {
    this.command = config.claudeCommand || '';
    this.args = config.claudeArgs || [];
    this.model = config.claudeModel || '';
    this.configDir = config.claudeConfigDir || '';
    this.env = config.claudeEnv || {};
    this.resolved = null;   // resolved command after auto-detection
    this.handle = null;     // agent handle from provider
    this.sessionId = null;  // MCP session ID
    this.cm = null;         // claudemux module reference
}

// resolve determines which Claude binary to use.
// Priority: explicit config > 'claude' on PATH > 'ollama' on PATH > error.
ClaudeCodeExecutor.prototype.resolve = function() {
    if (this.command) {
        // Explicit command — verify it exists.
        var check = exec.execv(['which', this.command]);
        if (check.code !== 0) {
            return { error: 'Claude command not found: ' + this.command };
        }
        this.resolved = { command: this.command, type: 'explicit' };
        return { error: null };
    }

    // Auto-detect: try 'claude' first.
    var claudeCheck = exec.execv(['which', 'claude']);
    if (claudeCheck.code === 0) {
        this.resolved = { command: 'claude', type: 'claude-code' };
        return { error: null };
    }

    // Try 'ollama'.
    var ollamaCheck = exec.execv(['which', 'ollama']);
    if (ollamaCheck.code === 0) {
        this.resolved = { command: 'ollama', type: 'ollama' };
        return { error: null };
    }

    return {
        error: 'No Claude-compatible binary found. Install Claude Code CLI ' +
               '(claude) or Ollama (ollama), or set --claude-command explicitly.'
    };
};

// spawn creates an MCP session and launches the Claude process.
// Returns { error: string|null, sessionId: string }.
ClaudeCodeExecutor.prototype.spawn = function(sessionId) {
    // Lazy-load claudemux to avoid errors in test environments.
    if (!this.cm) {
        try {
            this.cm = require('osm:claudemux');
        } catch (e) {
            return { error: 'osm:claudemux module not available: ' + e.message };
        }
    }

    var resolveResult = this.resolve();
    if (resolveResult.error) {
        return { error: resolveResult.error };
    }

    this.sessionId = sessionId || ('prsplit-' + Date.now());

    // Create MCP instance with result directory.
    try {
        this.mcpInstance = this.cm.newMCPInstance(this.sessionId);
        this.mcpInstance.setResultDir(this.mcpInstance.configDir() + '/results');
        // Create result directory.
        exec.execv(['mkdir', '-p', this.mcpInstance.resultDir()]);
        this.mcpInstance.writeConfigFile();
    } catch (e) {
        return { error: 'MCP instance creation failed: ' + (e.message || String(e)) };
    }

    // Spawn via provider registry.
    var spawnOpts;
    try {
        var registry = this.cm.newRegistry();
        var provider;
        spawnOpts = {
            model: this.model || undefined,
            env: this.env || {},
            args: (this.args || []).concat(['--mcp-config', this.mcpInstance.configPath()])
        };

        if (this.resolved.type === 'claude-code' || this.resolved.type === 'explicit') {
            provider = this.cm.claudeCode({
                command: this.resolved.command,
                mcp: true
            });
        } else if (this.resolved.type === 'ollama') {
            provider = this.cm.ollama({
                command: this.resolved.command,
                model: this.model || '',
                mcp: true
            });
        } else {
            return { error: 'unknown provider type: ' + this.resolved.type };
        }

        registry.register(provider);
        this.handle = registry.spawn(provider.name(), spawnOpts);
    } catch (e) {
        // Clean up MCP instance on spawn failure.
        try { this.mcpInstance.close(); } catch (e2) { /* best effort */ }
        this.mcpInstance = null;
        // Show the full command that was attempted so the user can debug.
        var cmdDesc = this.resolved.command;
        if (spawnOpts && spawnOpts.args && spawnOpts.args.length > 0) {
            cmdDesc += ' ' + spawnOpts.args.join(' ');
        }
        return {
            error: 'Claude spawn failed: ' + (e.message || String(e)) +
                   '\n  Command attempted: ' + cmdDesc +
                   '\n  Provider type: ' + this.resolved.type
        };
    }

    log.printf('Claude executor: spawned command=%s type=%s session=%s',
        this.resolved.command, this.resolved.type, this.sessionId);

    return { error: null, sessionId: this.sessionId, resultDir: this.mcpInstance.resultDir() };
};

// isAvailable returns true if a Claude-compatible binary can be resolved.
ClaudeCodeExecutor.prototype.isAvailable = function() {
    var result = this.resolve();
    return !result.error;
};

// close terminates the Claude process and cleans up.
ClaudeCodeExecutor.prototype.close = function() {
    if (this.handle && typeof this.handle.close === 'function') {
        try { this.handle.close(); } catch (e) { /* best effort */ }
    }
    if (this.mcpInstance && typeof this.mcpInstance.close === 'function') {
        try { this.mcpInstance.close(); } catch (e) { /* best effort */ }
    }
    this.handle = null;
    this.mcpInstance = null;
    this.sessionId = null;
    this.resolved = null;
};

// ---------------------------------------------------------------------------
//  Prompt Templates (Go text/template syntax)
// ---------------------------------------------------------------------------

// CLASSIFICATION_PROMPT_TEMPLATE is sent to Claude via PTY stdin.
// It instructs Claude to use the reportClassification MCP tool.
var CLASSIFICATION_PROMPT_TEMPLATE =
    'You are a code reviewer helping split a large pull request into smaller, ' +
    'reviewable stacked PRs.\n\n' +
    'The repository is a {{.Language}} project' +
    '{{if .ModulePath}} with module path `{{.ModulePath}}`{{end}}.\n' +
    'The base branch is `{{.BaseBranch}}`.\n\n' +
    '## Changed Files\n\n' +
    'The following files have been modified (status: A=added, M=modified, D=deleted, R=renamed):\n\n' +
    '{{range $path, $status := .FileStatuses}}' +
    '- `{{$path}}` ({{$status}})\n' +
    '{{end}}\n' +
    '## Task\n\n' +
    'Classify each file into a logical group for PR splitting. Group related changes together:\n' +
    '- Files in the same package/module that are tightly coupled\n' +
    '- Test files with the code they test\n' +
    '- Documentation with the features they document\n' +
    '- Refactoring changes separate from feature additions\n' +
    '- Infrastructure/config changes separate from application code\n\n' +
    '{{if gt .MaxGroups 0}}Use at most {{.MaxGroups}} groups.{{end}}\n\n' +
    '## Output\n\n' +
    'Use the `reportClassification` MCP tool to report your results. ' +
    'The `files` parameter should be a JSON object mapping each file path to its category name.\n\n' +
    'Use the session ID: `{{.SessionID}}`\n\n' +
    'Also assess which groups are independent (can be merged in any order). ' +
    'If any groups can merge independently, mention this in your response.\n';

// SPLIT_PLAN_PROMPT_TEMPLATE asks Claude to propose an ordered split plan.
var SPLIT_PLAN_PROMPT_TEMPLATE =
    'Based on the file classification below, create an ordered split plan for stacked PRs.\n\n' +
    '## Classification\n\n' +
    '{{range $path, $category := .Classification}}' +
    '- `{{$path}}` → {{$category}}\n' +
    '{{end}}\n' +
    '## Constraints\n\n' +
    '- Branch prefix: `{{.BranchPrefix}}`\n' +
    '{{if gt .MaxFilesPerSplit 0}}- Maximum {{.MaxFilesPerSplit}} files per split\n{{end}}' +
    '{{if .PreferIndependent}}- Prefer independently mergeable splits when possible\n{{end}}\n' +
    '## Task\n\n' +
    'Create an ordered plan where:\n' +
    '1. Each stage is a coherent, reviewable unit\n' +
    '2. Earlier stages should be foundations that later stages build on\n' +
    '3. Minimize cross-stage dependencies to reduce merge conflicts\n' +
    '4. Each stage should build and pass tests independently (when stacked)\n\n' +
    'Use the `reportSplitPlan` MCP tool with session ID `{{.SessionID}}`. ' +
    'Each stage needs: name, files array, commit message, and order (0-based).\n';

// CONFLICT_RESOLUTION_PROMPT_TEMPLATE asks Claude to fix a broken split.
var CONFLICT_RESOLUTION_PROMPT_TEMPLATE =
    'A split branch failed verification. Help fix it.\n\n' +
    '## Branch: `{{.BranchName}}`\n\n' +
    '### Files in this branch\n' +
    '{{range .Files}}- `{{.}}`\n{{end}}\n' +
    '### Verification Error (exit code {{.ExitCode}})\n\n' +
    '```\n{{.ErrorOutput}}\n```\n\n' +
    '{{if .GoModContent}}### go.mod content\n\n```\n{{.GoModContent}}\n```\n\n{{end}}' +
    '## Task\n\n' +
    'Analyze the error and propose a fix using the `reportResolution` MCP tool ' +
    'with session ID `{{.SessionID}}` and branch `{{.BranchName}}`.\n\n' +
    'You can suggest:\n' +
    '- File patches (full file content replacements)\n' +
    '- Commands to run (e.g., `go mod tidy`)\n' +
    '- If the split is fundamentally broken, set `reSplitSuggested: true` ' +
    'with a reason explaining which files conflict\n';

// ---------------------------------------------------------------------------
//  Prompt Rendering
// ---------------------------------------------------------------------------

// renderPrompt renders a Go text/template string with the given data.
// Returns { text: string, error: string|null }.
function renderPrompt(tmplStr, data) {
    if (!template) {
        return { text: '', error: 'osm:text/template module not available' };
    }
    try {
        var text = template.execute(tmplStr, data);
        return { text: text, error: null };
    } catch (e) {
        return { text: '', error: 'template render failed: ' + (e.message || String(e)) };
    }
}

// renderClassificationPrompt renders the classification prompt template.
function renderClassificationPrompt(analysis, config) {
    config = config || {};
    var modulePath = detectGoModulePath();
    var language = modulePath ? 'Go' : detectLanguage(analysis.files);
    return renderPrompt(CLASSIFICATION_PROMPT_TEMPLATE, {
        Language: language,
        ModulePath: modulePath,
        BaseBranch: analysis.baseBranch || runtime.baseBranch,
        FileStatuses: analysis.fileStatuses || {},
        MaxGroups: config.maxGroups || 0,
        SessionID: config.sessionId || ''
    });
}

// renderSplitPlanPrompt renders the split-plan prompt template.
function renderSplitPlanPrompt(classification, config) {
    config = config || {};
    return renderPrompt(SPLIT_PLAN_PROMPT_TEMPLATE, {
        Classification: classification,
        BranchPrefix: config.branchPrefix || runtime.branchPrefix || 'split/',
        MaxFilesPerSplit: config.maxFilesPerSplit || runtime.maxFiles || 0,
        PreferIndependent: config.preferIndependent || false,
        SessionID: config.sessionId || ''
    });
}

// renderConflictPrompt renders the conflict-resolution prompt template.
function renderConflictPrompt(conflict) {
    return renderPrompt(CONFLICT_RESOLUTION_PROMPT_TEMPLATE, {
        BranchName: conflict.branchName || '',
        Files: conflict.files || [],
        ExitCode: conflict.exitCode || 1,
        ErrorOutput: conflict.errorOutput || '',
        GoModContent: conflict.goModContent || '',
        SessionID: conflict.sessionId || ''
    });
}

// detectLanguage guesses the primary language from file extensions.
function detectLanguage(files) {
    var counts = {};
    var langMap = {
        '.go': 'Go', '.js': 'JavaScript', '.ts': 'TypeScript',
        '.py': 'Python', '.rb': 'Ruby', '.rs': 'Rust',
        '.java': 'Java', '.c': 'C', '.cpp': 'C++',
        '.cs': 'C#', '.swift': 'Swift', '.kt': 'Kotlin'
    };
    for (var i = 0; i < (files || []).length; i++) {
        var ext = fileExtension(files[i]);
        var lang = langMap[ext];
        if (lang) {
            counts[lang] = (counts[lang] || 0) + 1;
        }
    }
    var best = 'unknown';
    var bestCount = 0;
    for (var k in counts) {
        if (counts[k] > bestCount) {
            best = k;
            bestCount = counts[k];
        }
    }
    return best;
}

// ---------------------------------------------------------------------------
//  Automated Split Pipeline
// ---------------------------------------------------------------------------

// Default polling and timeout configuration.
var AUTOMATED_DEFAULTS = {
    classifyTimeoutMs: 120000,  // 2 minutes for classification
    planTimeoutMs: 120000,      // 2 minutes for plan
    resolveTimeoutMs: 180000,   // 3 minutes for conflict resolution
    pollIntervalMs: 500,        // Poll every 500ms for fast cancellation response
    maxResolveRetries: 3,       // Retries per branch
    maxReSplits: 1              // Maximum re-classification cycles
};

// Spinner characters for progress display.
var SPINNER_FRAMES = ['⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'];

// isCancelled checks cooperative cancellation from the auto-split TUI.
// Returns true when the user has pressed q/Ctrl+C.
function isCancelled() {
    return typeof autoSplitTUI !== 'undefined' && autoSplitTUI &&
           typeof autoSplitTUI.cancelled === 'function' && autoSplitTUI.cancelled();
}

// pollForFile polls a result directory for a specific file.
// Returns { data: object|null, error: string|null }.
function pollForFile(resultDir, filename, timeoutMs, intervalMs, stepName) {
    if (!osmod) {
        return { data: null, error: 'osm:os module not available for file polling' };
    }
    var hasDetailUpdate = stepName && typeof autoSplitTUI !== 'undefined' && autoSplitTUI &&
                          typeof autoSplitTUI.stepDetail === 'function';
    var elapsed = 0;
    var spinnerIdx = 0;
    while (elapsed < timeoutMs) {
        // Check cancellation before each poll iteration.
        if (isCancelled()) {
            return { data: null, error: 'cancelled by user' };
        }
        // Emit progress on every iteration so the TUI shows activity.
        if (hasDetailUpdate) {
            var spinner = SPINNER_FRAMES[spinnerIdx % SPINNER_FRAMES.length];
            var elapsedSec = Math.round(elapsed / 1000);
            var timeoutSec = Math.round(timeoutMs / 1000);
            autoSplitTUI.stepDetail(stepName, spinner + ' polling… ' + elapsedSec + 's / ' + timeoutSec + 's');
            spinnerIdx++;
        }
        var readResult = osmod.readFile(resultDir + '/' + filename);
        if (!readResult.error) {
            try {
                var data = JSON.parse(readResult.content);
                return { data: data, error: null };
            } catch (e) {
                return { data: null, error: 'failed to parse ' + filename + ': ' + e.message };
            }
        }
        // Sleep without burning CPU. Prefer osm:time.sleep (Go time.Sleep)
        // but fall back to exec.execv(['sleep', ...]) if unavailable.
        if (timemod && typeof timemod.sleep === 'function') {
            timemod.sleep(intervalMs);
        } else {
            exec.execv(['sleep', String(intervalMs / 1000)]);
        }
        elapsed += intervalMs;
    }
    return { data: null, error: 'timeout waiting for ' + filename + ' after ' + timeoutMs + 'ms' };
}

// automatedSplit orchestrates the full automated PR splitting pipeline.
// Steps: analyze → spawn → classify → receive → validate → plan →
//        execute → verify → resolve → report.
// Returns { error: string|null, report: object }.
function automatedSplit(config) {
    config = config || {};

    // Save the current branch so we can restore it on ALL exit paths.
    // This prevents the user from being stranded on baseBranch after
    // a re-split cycle (cleanupBranches checks out baseBranch).
    var originalBranchResult = gitExec('.', ['rev-parse', '--abbrev-ref', 'HEAD']);
    var originalBranch = originalBranchResult.code === 0 ? originalBranchResult.stdout.trim() : '';

    // Reset module-level state to prevent leakage across multiple runs
    // within the same JS VM (e.g., running auto-split twice in one session).
    conversationHistory = [];
    telemetryData = {
        filesAnalyzed: 0,
        splitCount: 0,
        strategy: '',
        claudeInteractions: 0,
        conflictsResolved: 0,
        conflictsFailed: 0,
        startTime: new Date().toISOString(),
        endTime: null
    };
    claudeExecutor = null;
    analysisCache = null;
    groupsCache = null;
    planCache = null;
    executionResultCache = [];

    var timeouts = {
        classify: config.classifyTimeoutMs || AUTOMATED_DEFAULTS.classifyTimeoutMs,
        plan: config.planTimeoutMs || AUTOMATED_DEFAULTS.planTimeoutMs,
        resolve: config.resolveTimeoutMs || AUTOMATED_DEFAULTS.resolveTimeoutMs
    };
    var pollInterval = config.pollIntervalMs || AUTOMATED_DEFAULTS.pollIntervalMs;
    var maxRetries = config.maxResolveRetries || AUTOMATED_DEFAULTS.maxResolveRetries;
    var maxReSplits = config.maxReSplits || AUTOMATED_DEFAULTS.maxReSplits;

    // Detect the auto-split BubbleTea TUI (injected from Go).
    // When available, route progress through it; when absent (tests,
    // non-interactive), fall back to text output.print.
    var hasTUI = typeof autoSplitTUI !== 'undefined' && autoSplitTUI &&
                 typeof autoSplitTUI.runAsync === 'function';
    if (hasTUI && !config.disableTUI) {
        autoSplitTUI.runAsync();
    }

    var report = {
        mode: 'automated',
        steps: [],
        classification: null,
        plan: null,
        splits: [],
        conflicts: [],
        resolutions: [],
        independencePairs: [],
        claudeInteractions: 0,
        fallbackUsed: false,
        error: null
    };

    function emitOutput(text) {
        if (hasTUI && !config.disableTUI) {
            autoSplitTUI.appendOutput(text);
        }
        output.print(text);
    }

    // Update sub-step detail shown in the progress bar (inline).
    function updateDetail(stepName, detail) {
        if (hasTUI && !config.disableTUI && typeof autoSplitTUI.stepDetail === 'function') {
            autoSplitTUI.stepDetail(stepName, detail);
        }
    }

    function step(name, fn) {
        // Check cancellation before each step — cooperative cancellation.
        if (hasTUI && !config.disableTUI && typeof autoSplitTUI.cancelled === 'function' && autoSplitTUI.cancelled()) {
            return { error: 'cancelled by user' };
        }
        var t0 = Date.now();
        if (hasTUI && !config.disableTUI) {
            autoSplitTUI.stepStart(name);
        }
        emitOutput('[auto-split] ' + name + '...');
        log.printf('auto-split step: %s', name);
        var result;
        try {
            result = fn();
        } catch (e) {
            result = { error: e.message || String(e) };
        }
        var elapsed = Date.now() - t0;
        report.steps.push({ name: name, elapsedMs: elapsed, error: result.error || null });
        if (result.error) {
            var errMsg = '[auto-split] ' + name + ' FAILED (' + elapsed + 'ms): ' + result.error;
            emitOutput(errMsg);
            if (hasTUI && !config.disableTUI) {
                autoSplitTUI.stepDone(name, result.error, elapsed);
            }
        } else {
            emitOutput('[auto-split] ' + name + ' OK (' + elapsed + 'ms)');
            if (hasTUI && !config.disableTUI) {
                autoSplitTUI.stepDone(name, '', elapsed);
            }
        }
        return result;
    }

    // finishTUI signals the auto-split TUI is done and waits for user
    // dismissal. Must be called on every exit path.
    function finishTUI(result) {
        if (hasTUI && !config.disableTUI) {
            var summary = result.error ? ('Error: ' + result.error) : 'Complete';
            autoSplitTUI.done(summary);
            autoSplitTUI.wait();
        }
        return result;
    }

    // Step 1: Analyze diff.
    var analysis = step('Analyze diff', function() {
        updateDetail('Analyze diff', 'Reading git diff...');
        var result = analyzeDiff(config);
        if (!result.error && result.files) {
            updateDetail('Analyze diff', result.files.length + ' files found');
        }
        return result;
    });
    if (analysis.error) {
        report.error = analysis.error;
        return finishTUI({ error: analysis.error, report: report });
    }
    if (analysis.files.length === 0) {
        report.error = 'No changes detected';
        return finishTUI({ error: report.error, report: report });
    }
    recordTelemetry('filesAnalyzed', analysis.files.length);

    // Step 2: Spawn Claude (or fall back to heuristic).
    var executor = step('Spawn Claude', function() {
        updateDetail('Spawn Claude', 'Resolving Claude executable...');
        if (!claudeExecutor) {
            claudeExecutor = new ClaudeCodeExecutor(prSplitConfig);
        }
        var resolveResult = claudeExecutor.resolve();
        if (resolveResult.error) {
            return { error: resolveResult.error };
        }
        updateDetail('Spawn Claude', 'Starting Claude process...');
        var spawnResult = claudeExecutor.spawn();
        if (spawnResult.error) {
            return { error: spawnResult.error };
        }
        return { error: null, sessionId: spawnResult.sessionId, resultDir: spawnResult.resultDir };
    });

    // If Claude is unavailable, fall back to heuristic mode.
    if (executor.error) {
        emitOutput('[auto-split] Claude unavailable — falling back to heuristic mode.');
        report.fallbackUsed = true;
        return finishTUI(heuristicFallback(analysis, config, report));
    }

    var sessionId = executor.sessionId;
    var resultDir = executor.resultDir;

    // Attach Claude's PTY handle to tuiMux so ctrl+] can forward
    // stdin/stdout to Claude during the pipeline.
    if (claudeExecutor && claudeExecutor.handle && typeof tuiMux !== 'undefined' && tuiMux) {
        try {
            tuiMux.attach(claudeExecutor.handle);
            log.printf('auto-split: attached Claude handle to tuiMux for ctrl+] toggle');
        } catch (e) {
            // Non-fatal — toggle just won't work.
            log.printf('auto-split: tuiMux attach warning: %s', e.message || String(e));
        }
    }

    // Step 3: Send classification request.
    var classifyResult = step('Send classification request', function() {
        updateDetail('Send classification request', 'Rendering prompt for ' + analysis.files.length + ' files...');
        var renderResult = renderClassificationPrompt(analysis, {
            sessionId: sessionId, maxGroups: config.maxGroups || 0
        });
        if (renderResult.error) {
            return { error: renderResult.error };
        }
        // Write prompt to Claude's stdin via handle.
        try {
            claudeExecutor.handle.send(renderResult.text);
            report.claudeInteractions++;
            recordConversation('classification', renderResult.text, '');
            recordTelemetry('claudeInteractions', 1);
        } catch (e) {
            return { error: 'failed to send prompt to Claude: ' + (e.message || String(e)) };
        }
        // Check cancellation immediately after the blocking send() call.
        if (isCancelled()) {
            return { error: 'cancelled by user' };
        }
        return { error: null };
    });
    if (classifyResult.error) {
        report.error = classifyResult.error;
        cleanupExecutor();
        return finishTUI({ error: classifyResult.error, report: report });
    }

    // Step 4: Receive classification.
    var classification = step('Receive classification', function() {
        updateDetail('Receive classification', 'Polling for response...');
        var pollResult = pollForFile(resultDir, 'classification.json', timeouts.classify, pollInterval, 'Receive classification');
        if (pollResult.error) {
            return { error: pollResult.error };
        }
        // Validate: every file must be classified.
        var classMap = pollResult.data;
        updateDetail('Receive classification', 'Validating ' + analysis.files.length + ' file classifications...');
        var missing = [];
        for (var i = 0; i < analysis.files.length; i++) {
            if (!classMap[analysis.files[i]]) {
                missing.push(analysis.files[i]);
            }
        }
        if (missing.length > 0) {
            log.printf('auto-split: %d files not classified: %s', missing.length, missing.join(', '));
            // Assign missing files to an 'uncategorized' group.
            for (var j = 0; j < missing.length; j++) {
                classMap[missing[j]] = 'uncategorized';
            }
        }
        report.classification = classMap;
        recordConversation('classification-result', '', JSON.stringify(classMap));
        return { error: null, classification: classMap };
    });
    if (classification.error) {
        report.error = classification.error;
        cleanupExecutor();
        return finishTUI({ error: classification.error, report: report });
    }

    // Step 5: Generate plan (from Claude or locally).
    var planResult = step('Generate split plan', function() {
        updateDetail('Generate split plan', 'Checking for Claude-generated plan...');
        // Check if Claude also provided a split plan.
        var planPoll = pollForFile(resultDir, 'split-plan.json', 5000, 500, 'Generate split plan');
        if (!planPoll.error && planPoll.data) {
            // Claude provided a plan — validate it.
            var claudePlan = planPoll.data;
            if (Array.isArray(claudePlan) && claudePlan.length > 0) {
                report.claudeInteractions++;
                var plan = {
                    baseBranch: analysis.baseBranch,
                    sourceBranch: analysis.currentBranch,
                    dir: '.',
                    verifyCommand: runtime.verifyCommand,
                    fileStatuses: analysis.fileStatuses || {},
                    splits: claudePlan.map(function(s, i) {
                        return {
                            name: s.name || (runtime.branchPrefix + padIndex(i, claudePlan.length)),
                            files: s.files || [],
                            message: s.message || ('Split ' + (i + 1)),
                            order: typeof s.order === 'number' ? s.order : i
                        };
                    })
                };
                var validation = validatePlan(plan, analysis.files);
                if (validation.valid) {
                    report.plan = plan;
                    return { error: null, plan: plan };
                }
                log.printf('auto-split: Claude plan invalid: %s — generating locally', validation.errors.join('; '));
            }
        }

        // Generate plan locally from classification.
        var groups = classificationToGroups(classification.classification);
        var plan = createSplitPlan(groups, {
            baseBranch: analysis.baseBranch,
            sourceBranch: analysis.currentBranch,
            branchPrefix: runtime.branchPrefix,
            maxFiles: runtime.maxFiles,
            fileStatuses: analysis.fileStatuses
        });
        report.plan = plan;
        return { error: null, plan: plan };
    });
    if (planResult.error) {
        report.error = planResult.error;
        cleanupExecutor();
        return finishTUI({ error: planResult.error, report: report });
    }

    var plan = planResult.plan;
    planCache = plan;

    // Step 6: Execute split.
    var execResult = step('Execute split plan', function() {
        if (runtime.dryRun) {
            return { error: null, dryRun: true };
        }
        updateDetail('Execute split plan', plan.splits.length + ' branches to create...');
        var result = executeSplit(plan);
        if (result.error) {
            return { error: result.error };
        }
        report.splits = result.results || [];
        updateDetail('Execute split plan', report.splits.length + ' branches created');
        return { error: null };
    });
    if (execResult.error) {
        report.error = execResult.error;
        cleanupExecutor();
        return finishTUI({ error: execResult.error, report: report });
    }
    if (runtime.dryRun) {
        emitOutput('[auto-split] Dry run — skipping verification.');
        cleanupExecutor();
        return finishTUI({ error: null, report: report });
    }

    // Step 7: Verify splits.
    var verifyResult = step('Verify splits', function() {
        updateDetail('Verify splits', 'Running tree hash verification...');
        var verifyObj = verifySplits(plan);
        var failures = [];
        for (var i = 0; i < verifyObj.results.length; i++) {
            if (!verifyObj.results[i].passed) {
                failures.push(verifyObj.results[i]);
            }
        }
        return { error: null, failures: failures, allPassed: verifyObj.allPassed };
    });

    // Step 8: Resolve conflicts (if any failures).
    var reSplitCount = 0;
    if (verifyResult.failures && verifyResult.failures.length > 0) {
        var resolved = step('Resolve conflicts via Claude', function() {
            return resolveConflictsWithClaude(
                verifyResult.failures, sessionId, resultDir,
                timeouts, pollInterval, maxRetries, report
            );
        });

        // Step 9: Re-split fallback if needed.
        if (resolved.reSplitNeeded && reSplitCount < maxReSplits) {
            reSplitCount++;
            output.print('[auto-split] Re-split requested — re-classifying...');
            // Clean up old branches.
            cleanupBranches(plan);
            // Re-classify with constraint.
            var reClassifyResult = step('Re-classify (retry ' + reSplitCount + ')', function() {
                var constraintPrompt = 'Re-classify these files with the constraint: ' +
                    resolved.reSplitReason + '\n\nUse reportClassification MCP tool ' +
                    'with session ID ' + sessionId + '.\n';
                try {
                    claudeExecutor.handle.send(constraintPrompt);
                    report.claudeInteractions++;
                    recordConversation('re-classify', constraintPrompt, '');
                } catch (e) {
                    return { error: 'failed to send re-classify prompt: ' + (e.message || String(e)) };
                }
                // Delete old classification file so we wait for the new one.
                try { exec.execv(['rm', '-f', resultDir + '/classification.json']); } catch (e) { /* ignore */ }
                var rePoll = pollForFile(resultDir, 'classification.json', timeouts.classify, pollInterval, 'Re-classify (retry ' + reSplitCount + ')');
                if (rePoll.error) {
                    return { error: rePoll.error };
                }
                return { error: null, classification: rePoll.data };
            });
            if (!reClassifyResult.error) {
                // Rebuild plan from new classification.
                var newGroups = classificationToGroups(reClassifyResult.classification);
                plan = createSplitPlan(newGroups, {
                    baseBranch: analysis.baseBranch,
                    sourceBranch: analysis.currentBranch,
                    branchPrefix: runtime.branchPrefix,
                    maxFiles: runtime.maxFiles,
                    fileStatuses: analysis.fileStatuses
                });
                planCache = plan;
                report.plan = plan;
                var reExec = step('Re-execute split', function() {
                    var result = executeSplit(plan);
                    if (result.error) return { error: result.error };
                    report.splits = result.results || [];
                    return { error: null };
                });
                if (!reExec.error) {
                    step('Re-verify splits', function() {
                        return { error: null, results: verifySplits(plan) };
                    });
                }
            }
        }
    }

    // Step 10: Equivalence check and report.
    var equivResult = step('Verify equivalence', function() {
        var result = verifyEquivalence(plan);
        return { error: result.equivalent ? null : 'tree hash mismatch', result: result };
    });

    // Assess independence.
    report.independencePairs = assessIndependence(plan, classification.classification || {});

    // Summary.
    emitOutput('');
    emitOutput('=== Auto-Split Complete ===');
    emitOutput('Splits: ' + plan.splits.length);
    emitOutput('Claude interactions: ' + report.claudeInteractions);
    emitOutput('Equivalence: ' + (equivResult.result && equivResult.result.equivalent ? 'PASS' : 'FAIL'));
    if (report.independencePairs.length > 0) {
        emitOutput('Independent pairs: ' + report.independencePairs.map(function(p) {
            return p[0] + ' + ' + p[1];
        }).join(', '));
    }
    if (report.fallbackUsed) {
        emitOutput('Mode: heuristic (Claude unavailable)');
    }

    cleanupExecutor();

    // Restore the original branch. This is critical after re-split cycles
    // where cleanupBranches may have checked out baseBranch.
    if (originalBranch) {
        gitExec('.', ['checkout', originalBranch]);
    }

    return finishTUI({ error: report.error, report: report });
}

// heuristicFallback runs the standard heuristic split flow.
function heuristicFallback(analysis, config, report) {
    var strategy = config.strategy || runtime.strategy;
    var groups = applyStrategy(analysis.files, strategy, {
        fileStatuses: analysis.fileStatuses,
        maxFiles: runtime.maxFiles,
        baseBranch: analysis.baseBranch
    });
    groupsCache = groups;

    var plan = createSplitPlan(groups, {
        baseBranch: analysis.baseBranch,
        sourceBranch: analysis.currentBranch,
        branchPrefix: runtime.branchPrefix,
        maxFiles: runtime.maxFiles,
        fileStatuses: analysis.fileStatuses
    });
    planCache = plan;
    report.plan = plan;

    if (!runtime.dryRun) {
        var execResult = executeSplit(plan);
        if (execResult.error) {
            report.error = execResult.error;
            return { error: execResult.error, report: report };
        }
        report.splits = execResult.results || [];

        var verifyObj = verifySplits(plan);
        var failures = verifyObj.results.filter(function(r) { return !r.passed; });
        if (failures.length > 0) {
            resolveConflicts(plan);
        }

        var equiv = verifyEquivalence(plan);
        if (!equiv.equivalent) {
            report.error = 'tree hash mismatch after heuristic split';
        }
    }

    output.print('=== Heuristic Split Complete ===');
    output.print('Splits: ' + plan.splits.length);

    return { error: report.error, report: report };
}

// classificationToGroups converts a classification map (file→category) to
// groups map (category→[files]).
function classificationToGroups(classification) {
    var groups = {};
    for (var path in classification) {
        var cat = classification[path];
        if (!groups[cat]) groups[cat] = [];
        groups[cat].push(path);
    }
    return groups;
}

// resolveConflictsWithClaude attempts to fix failing splits using Claude.
function resolveConflictsWithClaude(failures, sessionId, resultDir, timeouts, pollInterval, maxRetries, report) {
    var reSplitNeeded = false;
    var reSplitReason = '';

    for (var i = 0; i < failures.length; i++) {
        // Intra-step cancellation: check between each failure resolution.
        if (isCancelled()) {
            return { reSplitNeeded: false, reSplitReason: 'cancelled by user' };
        }

        var fail = failures[i];
        var fixed = false;

        for (var attempt = 0; attempt < maxRetries && !fixed; attempt++) {
            // Check cancellation between retry attempts.
            if (isCancelled()) {
                return { reSplitNeeded: false, reSplitReason: 'cancelled by user' };
            }
            report.conflicts.push({
                branch: fail.branch || fail.name,
                attempt: attempt + 1,
                error: fail.error || fail.output || ''
            });

            // Send conflict prompt to Claude.
            var promptResult = renderConflictPrompt({
                branchName: fail.branch || fail.name,
                files: fail.files || [],
                exitCode: fail.exitCode || 1,
                errorOutput: fail.error || fail.output || '',
                goModContent: '',
                sessionId: sessionId
            });
            if (promptResult.error) {
                log.printf('auto-split: conflict prompt render failed: %s', promptResult.error);
                break;
            }

            try {
                claudeExecutor.handle.send(promptResult.text);
                report.claudeInteractions++;
                recordConversation('conflict-resolution', promptResult.text, '');
            } catch (e) {
                log.printf('auto-split: failed to send conflict prompt: %s', e.message || String(e));
                break;
            }

            // Wait for resolution.
            // Delete old resolution file first.
            try { exec.execv(['rm', '-f', resultDir + '/resolution.json']); } catch (e) { /* ignore */ }
            var resolutionPoll = pollForFile(resultDir, 'resolution.json', timeouts.resolve, pollInterval, 'Resolve conflicts');
            if (resolutionPoll.error) {
                log.printf('auto-split: resolution timeout for %s (attempt %d)', fail.branch || fail.name, attempt + 1);
                continue;
            }

            var resolution = resolutionPoll.data;
            report.resolutions.push(resolution);

            // Check if re-split is suggested.
            if (resolution.reSplitSuggested) {
                reSplitNeeded = true;
                reSplitReason = resolution.reSplitReason || 'Claude suggested re-split';
                break;
            }

            // Apply patches.
            if (resolution.patches && resolution.patches.length > 0) {
                for (var p = 0; p < resolution.patches.length; p++) {
                    var patch = resolution.patches[p];
                    if (osmod) {
                        osmod.writeFile(patch.file, patch.content);
                    }
                }
                // Commit changes.
                gitExec('.', ['add', '-A']);
                gitExec('.', ['commit', '--amend', '--no-edit']);
            }

            // Run suggested commands.
            if (resolution.commands && resolution.commands.length > 0) {
                for (var c = 0; c < resolution.commands.length; c++) {
                    exec.execv(['sh', '-c', resolution.commands[c]]);
                }
                gitExec('.', ['add', '-A']);
                gitExec('.', ['commit', '--amend', '--no-edit']);
            }

            // Re-verify this branch.
            var reVerify = verifySplit(fail.branch || fail.name, { verifyCommand: runtime.verifyCommand });
            if (reVerify.passed) {
                fixed = true;
                output.print('[auto-split] Fixed: ' + (fail.branch || fail.name));
            }
        }

        if (!fixed && !reSplitNeeded) {
            // Try local AUTO_FIX_STRATEGIES as last resort.
            log.printf('auto-split: Claude resolution exhausted for %s, trying local strategies', fail.branch || fail.name);
        }
    }

    return { reSplitNeeded: reSplitNeeded, reSplitReason: reSplitReason };
}

// cleanupExecutor closes the Claude executor and cleans up resources.
// Detaches from tuiMux first so ctrl+] stops trying to forward to a
// dead child process.
function cleanupExecutor() {
    if (typeof tuiMux !== 'undefined' && tuiMux) {
        try { tuiMux.detach(); } catch (e) { /* best effort — may already be detached */ }
    }
    if (claudeExecutor) {
        try { claudeExecutor.close(); } catch (e) { /* best effort */ }
    }
}

// ---------------------------------------------------------------------------
//  Independence Detection
// ---------------------------------------------------------------------------

// assessIndependence determines which split pairs can merge independently.
// Returns an array of [nameA, nameB] pairs.
function assessIndependence(plan, classification) {
    if (!plan || !plan.splits || plan.splits.length < 2) {
        return [];
    }

    var pairs = [];
    for (var i = 0; i < plan.splits.length; i++) {
        for (var j = i + 1; j < plan.splits.length; j++) {
            var a = plan.splits[i];
            var b = plan.splits[j];
            if (splitsAreIndependent(a, b, classification)) {
                pairs.push([a.name, b.name]);
            }
        }
    }
    return pairs;
}

// splitsAreIndependent checks if two splits have no directory overlap
// and no import dependency overlap (for Go files).
function splitsAreIndependent(a, b, classification) {
    var dirsA = extractDirs(a.files);
    var dirsB = extractDirs(b.files);

    // Check directory overlap.
    for (var d in dirsA) {
        if (dirsB[d]) return false;
    }

    // For Go files, check import overlap.
    var importsA = extractGoImports(a.files);
    var importsB = extractGoImports(b.files);
    var pkgsA = extractGoPkgs(a.files);
    var pkgsB = extractGoPkgs(b.files);

    // If A imports a package that B modifies (or vice versa), they're dependent.
    for (var imp in importsA) {
        if (pkgsB[imp]) return false;
    }
    for (var imp2 in importsB) {
        if (pkgsA[imp2]) return false;
    }

    return true;
}

// extractDirs returns a set of directory paths from a file list.
function extractDirs(files) {
    var dirs = {};
    for (var i = 0; i < (files || []).length; i++) {
        dirs[dirname(files[i])] = true;
    }
    return dirs;
}

// extractGoImports returns a set of imported package paths from Go files.
function extractGoImports(files) {
    var imports = {};
    for (var i = 0; i < (files || []).length; i++) {
        if (files[i].match(/\.go$/)) {
            // Read file content before parsing imports.
            var content = '';
            try {
                var cat = exec.execv(['cat', files[i]]);
                if (cat.code === 0) {
                    content = cat.stdout;
                }
            } catch (e) {
                // File may not exist in working tree — skip.
                continue;
            }
            if (!content) continue;
            var fileImports = parseGoImports(content);
            for (var j = 0; j < fileImports.length; j++) {
                imports[fileImports[j]] = true;
            }
        }
    }
    return imports;
}

// extractGoPkgs returns a set of Go package directories from Go files.
function extractGoPkgs(files) {
    var pkgs = {};
    var modulePath = detectGoModulePath();
    for (var i = 0; i < (files || []).length; i++) {
        if (files[i].match(/\.go$/)) {
            var dir = dirname(files[i]);
            if (modulePath && dir !== '.') {
                pkgs[modulePath + '/' + dir] = true;
            }
            pkgs[dir] = true;
        }
    }
    return pkgs;
}

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
            if (!planConfig.fileStatuses && analysis.fileStatuses) planConfig.fileStatuses = analysis.fileStatuses;
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
// strategy using heuristic scoring.
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
//  Reusable BT Templates
// ---------------------------------------------------------------------------

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
//  Composite BT Workflow Functions
// ---------------------------------------------------------------------------

// verifyAndCommit creates a sequence: run tests → optionally verify → commit.
// Order: tests FIRST (fast feedback), then optional heavy verification, then
// commit.
function verifyAndCommit(bb, opts) {
    opts = opts || {};
    var testCmd = opts.testCommand || 'make test';
    var verifyCmd = opts.verifyCommand || null;
    var commitMsg = opts.message || 'Automated commit';

    if (verifyCmd) {
        return bt.node(bt.sequence,
            btRunTests(bb, testCmd),
            btVerifyOutput(bb, verifyCmd),
            btCommitChanges(bb, commitMsg)
        );
    }
    return bt.node(bt.sequence,
        btRunTests(bb, testCmd),
        btCommitChanges(bb, commitMsg)
    );
}

// ---------------------------------------------------------------------------
//  Diff Visualization (T123)
// ---------------------------------------------------------------------------

// renderColorizedDiff takes raw diff text and returns a styled version.
// Uses ANSI codes: green for additions, red for deletions, gray for context.
function renderColorizedDiff(diffText) {
    if (!diffText) return '';
    var lines = diffText.split('\n');
    var result = [];
    for (var i = 0; i < lines.length; i++) {
        var line = lines[i];
        if (line.indexOf('+') === 0 && line.indexOf('+++') !== 0) {
            result.push('\x1b[32m' + line + '\x1b[0m'); // green
        } else if (line.indexOf('-') === 0 && line.indexOf('---') !== 0) {
            result.push('\x1b[31m' + line + '\x1b[0m'); // red
        } else if (line.indexOf('@@') === 0) {
            result.push('\x1b[36m' + line + '\x1b[0m'); // cyan
        } else if (line.indexOf('diff ') === 0 || line.indexOf('index ') === 0 ||
                   line.indexOf('---') === 0 || line.indexOf('+++') === 0) {
            result.push('\x1b[1m' + line + '\x1b[0m'); // bold
        } else {
            result.push('\x1b[90m' + line + '\x1b[0m'); // gray
        }
    }
    return result.join('\n');
}

// getSplitDiff returns the git diff for a specific split.
function getSplitDiff(plan, splitIndex) {
    if (!plan || !plan.splits || splitIndex < 0 || splitIndex >= plan.splits.length) {
        return { error: 'invalid split index', diff: '' };
    }
    var split = plan.splits[splitIndex];
    var dir = plan.dir || '.';
    var files = split.files || [];
    if (files.length === 0) {
        return { error: 'no files in split', diff: '' };
    }

    // Try getting diff from the split branch.
    var diffArgs = ['diff', plan.baseBranch + '...' + split.name, '--'];
    for (var i = 0; i < files.length; i++) {
        diffArgs.push(files[i]);
    }
    var result = gitExec(dir, diffArgs);
    if (result.code !== 0) {
        // Fallback: diff against base for just these files.
        var fallbackArgs = ['diff', plan.baseBranch, '--'];
        for (var j = 0; j < files.length; j++) {
            fallbackArgs.push(files[j]);
        }
        result = gitExec(dir, fallbackArgs);
    }
    if (result.code !== 0) {
        return { error: 'git diff failed: ' + result.stderr.trim(), diff: '' };
    }
    return { error: null, diff: result.stdout };
}

// ---------------------------------------------------------------------------
//  Claude Conversation History (T124)
// ---------------------------------------------------------------------------

// conversationHistory stores Claude's responses during a session.
var conversationHistory = [];

// recordConversation saves a Claude interaction to the history.
function recordConversation(action, prompt, response) {
    conversationHistory.push({
        timestamp: new Date().toISOString(),
        action: action,
        prompt: prompt,
        response: response
    });
}

// getConversationHistory returns all recorded conversations.
function getConversationHistory() {
    return conversationHistory.slice();
}

// ---------------------------------------------------------------------------
//  Split Dependency Graph (T127)
// ---------------------------------------------------------------------------

// buildDependencyGraph creates an adjacency list from plan splits.
// Each split that shares directory or import dependencies with another
// gets an edge. Returns {nodes: [{name, index}], edges: [{from, to}]}.
function buildDependencyGraph(plan, classification) {
    if (!plan || !plan.splits) return { nodes: [], edges: [] };
    var nodes = [];
    for (var i = 0; i < plan.splits.length; i++) {
        nodes.push({ name: plan.splits[i].name, index: i });
    }
    var edges = [];
    for (var a = 0; a < plan.splits.length; a++) {
        for (var b = a + 1; b < plan.splits.length; b++) {
            if (!splitsAreIndependent(plan.splits[a], plan.splits[b], classification)) {
                edges.push({ from: a, to: b });
            }
        }
    }
    return { nodes: nodes, edges: edges };
}

// renderAsciiGraph renders a dependency graph as ASCII art.
function renderAsciiGraph(graph) {
    if (!graph.nodes || graph.nodes.length === 0) return '(empty graph)';
    var lines = [];
    lines.push('Split Dependency Graph');
    lines.push('======================');
    lines.push('');

    // Build adjacency sets.
    var adj = {};
    for (var i = 0; i < graph.nodes.length; i++) adj[i] = [];
    for (var e = 0; e < graph.edges.length; e++) {
        adj[graph.edges[e].from].push(graph.edges[e].to);
        adj[graph.edges[e].to].push(graph.edges[e].from);
    }

    // Topological-ish ordering (degree-based: nodes with fewest deps first).
    var sorted = graph.nodes.slice().sort(function(a, b) {
        return adj[a.index].length - adj[b.index].length;
    });

    // Render each node with its connections.
    for (var n = 0; n < sorted.length; n++) {
        var node = sorted[n];
        var deps = adj[node.index];
        var marker = deps.length === 0 ? '◯' : '●';
        var line = '  ' + marker + ' [' + (node.index + 1) + '] ' + node.name;
        if (deps.length > 0) {
            var depNames = [];
            for (var d = 0; d < deps.length; d++) {
                depNames.push('[' + (deps[d] + 1) + ']');
            }
            line += '  ←→  ' + depNames.join(', ');
        } else {
            line += '  (independent)';
        }
        lines.push(line);
    }

    // Summary.
    var independent = [];
    for (var j = 0; j < graph.nodes.length; j++) {
        if (adj[j].length === 0) independent.push(graph.nodes[j].name);
    }
    lines.push('');
    if (independent.length > 0) {
        lines.push('Independent splits (safe to merge in any order): ' + independent.join(', '));
    }
    lines.push('Merge recommendation: process in listed order (fewest dependencies first).');
    return lines.join('\n');
}

// ---------------------------------------------------------------------------
//  Telemetry and Analytics (T129)
// ---------------------------------------------------------------------------

// telemetryData accumulates local telemetry for this session.
var telemetryData = {
    filesAnalyzed: 0,
    splitCount: 0,
    strategy: '',
    claudeInteractions: 0,
    conflictsResolved: 0,
    conflictsFailed: 0,
    startTime: new Date().toISOString(),
    endTime: null
};

// recordTelemetry updates telemetry counters.
function recordTelemetry(key, value) {
    if (typeof telemetryData[key] === 'number') {
        telemetryData[key] += (typeof value === 'number' ? value : 1);
    } else {
        telemetryData[key] = value;
    }
}

// getTelemetrySummary returns a formatted telemetry report.
function getTelemetrySummary() {
    telemetryData.endTime = new Date().toISOString();
    return telemetryData;
}

// saveTelemetry persists telemetry to disk (opt-in, local only).
function saveTelemetry(dir) {
    dir = dir || '.osm/telemetry';
    if (!osmod) {
        return { error: 'osm:os module not available — cannot persist telemetry' };
    }
    try {
        var mkResult = exec.execv(['mkdir', '-p', dir]);
        if (mkResult.code !== 0) return { error: 'mkdir failed' };
        var filename = dir + '/session-' + telemetryData.startTime.replace(/[:.]/g, '-') + '.json';
        var data = JSON.stringify(getTelemetrySummary(), null, 2);
        osmod.writeFile(filename, data);
        return { error: null, path: filename };
    } catch (e) {
        return { error: e.message };
    }
}

// ---------------------------------------------------------------------------
//  Retrospective Analysis (T131)
// ---------------------------------------------------------------------------

// analyzeRetrospective examines a completed split for insights.
function analyzeRetrospective(plan, verifyResults, equivalenceResult) {
    var insights = [];
    if (!plan || !plan.splits) return { insights: insights, score: 0 };

    var totalFiles = 0;
    var maxFiles = 0;
    var minFiles = Infinity;
    var failedSplits = [];

    for (var i = 0; i < plan.splits.length; i++) {
        var split = plan.splits[i];
        var fc = (split.files || []).length;
        totalFiles += fc;
        if (fc > maxFiles) maxFiles = fc;
        if (fc < minFiles) minFiles = fc;
    }

    // Check verification results.
    if (verifyResults && verifyResults.length > 0) {
        for (var v = 0; v < verifyResults.length; v++) {
            if (verifyResults[v] && !verifyResults[v].passed) {
                failedSplits.push(verifyResults[v].name || ('split-' + (v + 1)));
            }
        }
    }

    // Size balance analysis.
    var avgFiles = plan.splits.length > 0 ? totalFiles / plan.splits.length : 0;
    var balance = maxFiles > 0 ? minFiles / maxFiles : 1;

    if (balance < 0.2) {
        insights.push({
            type: 'warning',
            message: 'Split size imbalance: smallest split has ' + minFiles + ' files, largest has ' + maxFiles + '. Consider rebalancing.',
            suggestion: 'Use "move" command to transfer files from large splits to small ones.'
        });
    }
    if (avgFiles > 20) {
        insights.push({
            type: 'info',
            message: 'Average split size is ' + Math.round(avgFiles) + ' files — consider splitting further for easier review.',
            suggestion: 'Lower max-files setting or use a more granular strategy.'
        });
    }
    if (failedSplits.length > 0) {
        insights.push({
            type: 'error',
            message: failedSplits.length + ' splits failed verification: ' + failedSplits.join(', '),
            suggestion: 'Use "fix" command to auto-repair, or manually check dependency ordering.'
        });
    }
    if (equivalenceResult && !equivalenceResult.equivalent) {
        insights.push({
            type: 'error',
            message: 'Tree-hash equivalence failed — combined splits do not reproduce original changes.',
            suggestion: 'Check for missing files or cherry-pick conflicts. Re-run "execute" with updated plan.'
        });
    }

    // Score: 100 = perfect, 0 = terrible.
    var score = 100;
    if (failedSplits.length > 0) score -= failedSplits.length * 15;
    if (equivalenceResult && !equivalenceResult.equivalent) score -= 30;
    if (balance < 0.3) score -= 10;
    if (score < 0) score = 0;

    // Commendations.
    if (score >= 90) {
        insights.push({
            type: 'success',
            message: 'Excellent split! All verifications passed with good balance.',
            suggestion: 'Proceed with PR creation.'
        });
    }

    return { insights: insights, score: score, stats: {
        totalFiles: totalFiles,
        splitCount: plan.splits.length,
        avgFiles: Math.round(avgFiles * 10) / 10,
        maxFiles: maxFiles,
        minFiles: minFiles,
        balance: Math.round(balance * 100) + '%',
        failedSplits: failedSplits.length
    }};
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
    groupByDependency: groupByDependency,
    parseGoImports: parseGoImports,
    detectGoModulePath: detectGoModulePath,
    applyStrategy: applyStrategy,
    selectStrategy: selectStrategy,

    // Planning
    createSplitPlan: createSplitPlan,
    validatePlan: validatePlan,
    savePlan: savePlan,
    loadPlan: loadPlan,

    // Execution
    executeSplit: executeSplit,

    // Verification
    verifySplit: verifySplit,
    verifySplits: verifySplits,
    verifyEquivalence: verifyEquivalence,
    verifyEquivalenceDetailed: verifyEquivalenceDetailed,
    cleanupBranches: cleanupBranches,
    createPRs: createPRs,
    resolveConflicts: resolveConflicts,
    AUTO_FIX_STRATEGIES: AUTO_FIX_STRATEGIES,

    // Claude Code executor
    ClaudeCodeExecutor: ClaudeCodeExecutor,

    // Automated pipeline
    automatedSplit: automatedSplit,
    heuristicFallback: heuristicFallback,
    assessIndependence: assessIndependence,
    classificationToGroups: classificationToGroups,
    isCancelled: isCancelled,

    // Prompt rendering
    renderClassificationPrompt: renderClassificationPrompt,
    renderSplitPlanPrompt: renderSplitPlanPrompt,
    renderConflictPrompt: renderConflictPrompt,
    detectLanguage: detectLanguage,

    // Prompt templates (constants)
    CLASSIFICATION_PROMPT_TEMPLATE: CLASSIFICATION_PROMPT_TEMPLATE,
    SPLIT_PLAN_PROMPT_TEMPLATE: SPLIT_PLAN_PROMPT_TEMPLATE,
    CONFLICT_RESOLUTION_PROMPT_TEMPLATE: CONFLICT_RESOLUTION_PROMPT_TEMPLATE,

    // BT nodes
    createAnalyzeNode: createAnalyzeNode,
    createGroupNode: createGroupNode,
    createPlanNode: createPlanNode,
    createSplitNode: createSplitNode,
    createVerifyNode: createVerifyNode,
    createEquivalenceNode: createEquivalenceNode,
    createSelectStrategyNode: createSelectStrategyNode,
    createWorkflowTree: createWorkflowTree,

    // BT templates
    btVerifyOutput: btVerifyOutput,
    btRunTests: btRunTests,
    btCommitChanges: btCommitChanges,
    btSplitBranch: btSplitBranch,

    // Composite BT workflow functions
    verifyAndCommit: verifyAndCommit,

    // Internal helpers (exposed for testing)
    _gitExec: gitExec,
    _dirname: dirname,
    _fileExtension: fileExtension,
    _sanitizeBranchName: sanitizeBranchName,
    _padIndex: padIndex,
    _splitsAreIndependent: splitsAreIndependent,
    _extractDirs: extractDirs,
    _extractGoImports: extractGoImports,
    _extractGoPkgs: extractGoPkgs,

    // Diff visualization
    renderColorizedDiff: renderColorizedDiff,
    getSplitDiff: getSplitDiff,

    // Conversation history
    recordConversation: recordConversation,
    getConversationHistory: getConversationHistory,

    // Dependency graph
    buildDependencyGraph: buildDependencyGraph,
    renderAsciiGraph: renderAsciiGraph,

    // Telemetry
    recordTelemetry: recordTelemetry,
    getTelemetrySummary: getTelemetrySummary,
    saveTelemetry: saveTelemetry,

    // Retrospective analysis
    analyzeRetrospective: analyzeRetrospective,

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
var executionResultCache = [];
var claudeExecutor = null; // Lazy-created ClaudeCodeExecutor instance

var state = tui.createState(COMMAND_NAME, {
    [shared.contextItems]: {defaultValue: []}
});

// buildReport creates a JSON-serializable report object from the current
// analysis, groups, plan, and equivalence caches.
function buildReport() {
    var report = {
        version: globalThis.prSplit ? globalThis.prSplit.VERSION : 'unknown',
        baseBranch: runtime.baseBranch,
        strategy: runtime.strategy,
        dryRun: runtime.dryRun,
        analysis: null,
        groups: null,
        plan: null
    };
    if (analysisCache && !analysisCache.error) {
        report.analysis = {
            currentBranch: analysisCache.currentBranch,
            baseBranch: analysisCache.baseBranch,
            fileCount: analysisCache.files.length,
            files: analysisCache.files,
            fileStatuses: analysisCache.fileStatuses || {}
        };
    }
    if (groupsCache) {
        var gNames = Object.keys(groupsCache).sort();
        report.groups = gNames.map(function(name) {
            return { name: name, files: groupsCache[name] };
        });
    }
    if (planCache) {
        report.plan = {
            splitCount: planCache.splits.length,
            splits: planCache.splits.map(function(s) {
                return {
                    name: s.name,
                    files: s.files,
                    message: s.message,
                    order: s.order
                };
            })
        };
        var equiv = verifyEquivalence(planCache);
        report.equivalence = {
            verified: equiv.equivalent,
            splitTree: equiv.splitTree,
            sourceTree: equiv.sourceTree,
            error: equiv.error || null
        };
    }
    return report;
}

function buildCommands(stateArg) {
    return {
        analyze: {
            description: 'Analyze diff between current branch and base',
            usage: 'analyze [base-branch]',
            handler: function(args) {
                try {
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
                    var st = (analysisCache.fileStatuses && analysisCache.fileStatuses[analysisCache.files[i]]) || '?';
                    output.print('  [' + st + '] ' + analysisCache.files[i]);
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
                    planConfig.fileStatuses = analysisCache.fileStatuses;
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

        // --- Plan Editing Commands ---

        move: {
            description: 'Move a file from one split to another',
            usage: 'move <file> <from-index> <to-index>',
            handler: function(args) {
                if (!planCache) {
                    output.print('No plan — run "plan" first.');
                    return;
                }
                if (!args || args.length < 3) {
                    output.print('Usage: move <file-path> <from-split-index> <to-split-index>');
                    output.print('Indexes are 1-based. Example: move cmd/main.go 1 2');
                    return;
                }
                var file = args[0];
                var fromIdx = parseInt(args[1], 10) - 1;
                var toIdx = parseInt(args[2], 10) - 1;

                if (fromIdx < 0 || fromIdx >= planCache.splits.length) {
                    output.print('Invalid from-index: ' + args[1] + ' (range: 1-' + planCache.splits.length + ')');
                    return;
                }
                if (toIdx < 0 || toIdx >= planCache.splits.length) {
                    output.print('Invalid to-index: ' + args[2] + ' (range: 1-' + planCache.splits.length + ')');
                    return;
                }
                if (fromIdx === toIdx) {
                    output.print('From and to are the same split.');
                    return;
                }

                // Find and remove file from source split.
                var fromSplit = planCache.splits[fromIdx];
                var fileIdx = -1;
                for (var i = 0; i < fromSplit.files.length; i++) {
                    if (fromSplit.files[i] === file) {
                        fileIdx = i;
                        break;
                    }
                }
                if (fileIdx === -1) {
                    output.print('File "' + file + '" not found in split ' + (fromIdx + 1) + ' (' + fromSplit.name + ')');
                    return;
                }
                fromSplit.files.splice(fileIdx, 1);

                // Add to destination split.
                planCache.splits[toIdx].files.push(file);
                planCache.splits[toIdx].files.sort();

                output.print('Moved "' + file + '" from split ' + (fromIdx + 1) + ' to split ' + (toIdx + 1));

                // Remove empty splits.
                if (fromSplit.files.length === 0) {
                    planCache.splits.splice(fromIdx, 1);
                    output.print('Split ' + (fromIdx + 1) + ' is now empty — removed.');
                    // Re-number remaining splits.
                    for (var r = 0; r < planCache.splits.length; r++) {
                        planCache.splits[r].order = r;
                    }
                }
            }
        },

        rename: {
            description: 'Rename a split branch',
            usage: 'rename <split-index> <new-name>',
            handler: function(args) {
                if (!planCache) {
                    output.print('No plan — run "plan" first.');
                    return;
                }
                if (!args || args.length < 2) {
                    output.print('Usage: rename <split-index> <new-name>');
                    output.print('Index is 1-based. Example: rename 2 refactoring');
                    return;
                }
                var idx = parseInt(args[0], 10) - 1;
                if (idx < 0 || idx >= planCache.splits.length) {
                    output.print('Invalid index: ' + args[0] + ' (range: 1-' + planCache.splits.length + ')');
                    return;
                }
                var newName = args.slice(1).join('-');
                var oldName = planCache.splits[idx].name;
                planCache.splits[idx].name = sanitizeBranchName(
                    runtime.branchPrefix + padIndex(idx + 1) + '-' + newName
                );
                planCache.splits[idx].message = newName;
                output.print('Renamed split ' + (idx + 1) + ': ' + oldName + ' → ' + planCache.splits[idx].name);
            }
        },

        merge: {
            description: 'Merge two splits into one',
            usage: 'merge <split-a> <split-b>',
            handler: function(args) {
                if (!planCache) {
                    output.print('No plan — run "plan" first.');
                    return;
                }
                if (!args || args.length < 2) {
                    output.print('Usage: merge <split-index-a> <split-index-b>');
                    output.print('Merges B into A. Index is 1-based.');
                    return;
                }
                var idxA = parseInt(args[0], 10) - 1;
                var idxB = parseInt(args[1], 10) - 1;
                if (idxA < 0 || idxA >= planCache.splits.length) {
                    output.print('Invalid index A: ' + args[0]);
                    return;
                }
                if (idxB < 0 || idxB >= planCache.splits.length) {
                    output.print('Invalid index B: ' + args[1]);
                    return;
                }
                if (idxA === idxB) {
                    output.print('Cannot merge a split with itself.');
                    return;
                }

                var splitA = planCache.splits[idxA];
                var splitB = planCache.splits[idxB];

                // Merge B's files into A.
                for (var i = 0; i < splitB.files.length; i++) {
                    splitA.files.push(splitB.files[i]);
                }
                splitA.files.sort();

                // Remove B (handle index shift).
                var removeIdx = idxB;
                planCache.splits.splice(removeIdx, 1);

                // Re-number.
                for (var r = 0; r < planCache.splits.length; r++) {
                    planCache.splits[r].order = r;
                }

                output.print('Merged split ' + (idxB + 1) + ' (' + splitB.name + ') into split ' + (idxA + 1) +
                    ' (' + splitA.name + ')');
                output.print('Plan now has ' + planCache.splits.length + ' splits.');
            }
        },

        reorder: {
            description: 'Move a split to a different position',
            usage: 'reorder <split-index> <new-position>',
            handler: function(args) {
                if (!planCache) {
                    output.print('No plan — run "plan" first.');
                    return;
                }
                if (!args || args.length < 2) {
                    output.print('Usage: reorder <split-index> <new-position>');
                    output.print('Both are 1-based. Example: reorder 3 1');
                    return;
                }
                var fromIdx = parseInt(args[0], 10) - 1;
                var toIdx = parseInt(args[1], 10) - 1;
                if (fromIdx < 0 || fromIdx >= planCache.splits.length) {
                    output.print('Invalid index: ' + args[0]);
                    return;
                }
                if (toIdx < 0 || toIdx >= planCache.splits.length) {
                    output.print('Invalid position: ' + args[1]);
                    return;
                }
                if (fromIdx === toIdx) {
                    output.print('Already at that position.');
                    return;
                }

                // Remove and re-insert.
                var split = planCache.splits.splice(fromIdx, 1)[0];
                planCache.splits.splice(toIdx, 0, split);

                // Re-number and rename to reflect new order.
                for (var r = 0; r < planCache.splits.length; r++) {
                    planCache.splits[r].order = r;
                    // Update the numeric prefix in the branch name.
                    var oldName = planCache.splits[r].name;
                    var nameParts = oldName.split('/');
                    if (nameParts.length > 1) {
                        var lastPart = nameParts[nameParts.length - 1];
                        var dashIdx = lastPart.indexOf('-');
                        if (dashIdx >= 0) {
                            var suffix = lastPart.substring(dashIdx + 1);
                            nameParts[nameParts.length - 1] = padIndex(r + 1) + '-' + suffix;
                            planCache.splits[r].name = nameParts.join('/');
                        }
                    }
                }

                output.print('Moved split from position ' + (fromIdx + 1) + ' to ' + (toIdx + 1));
                output.print('Updated plan: ' + planCache.splits.length + ' splits');
                for (var i = 0; i < planCache.splits.length; i++) {
                    output.print('  ' + (i + 1) + '. ' + planCache.splits[i].name +
                        ' (' + planCache.splits[i].files.length + ' files)');
                }
            }
        },

        execute: {
            description: 'Execute the split plan (creates branches)',
            usage: 'execute',
            handler: function() {
                try {
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
                executionResultCache = result.results;
                output.print(style.success('Split completed successfully!'));
                for (var i = 0; i < result.results.length; i++) {
                    var r = result.results[i];
                    output.print('  ' + style.success('✓') + ' ' + r.name + ' (' + r.files.length + ' files, SHA: ' + style.dim(r.sha.substring(0, 8)) + ')');
                }

                // Auto-verify equivalence
                var equiv = verifyEquivalence(planCache);
                if (equiv.equivalent) {
                    output.print(style.success('✅ Tree hash equivalence verified'));
                } else if (equiv.error) {
                    output.print(style.warning('⚠️  Equivalence check error: ' + equiv.error));
                } else {
                    output.print(style.error('❌ Tree hash mismatch!'));
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
                if (!planCache) {
                    output.print('Run "plan" first.');
                    return;
                }
                output.print('Verifying ' + planCache.splits.length + ' splits...');
                var result = verifySplits(planCache);
                for (var i = 0; i < result.results.length; i++) {
                    var r = result.results[i];
                    var icon = r.passed ? style.success('✓') : style.error('✗');
                    output.print('  ' + icon + ' ' + r.name + (r.error ? ': ' + r.error : ''));
                }
                output.print(result.allPassed ? style.success('✅ All splits pass verification') : style.error('❌ Some splits failed'));
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
                    output.print(style.success('✅ Trees are equivalent'));
                    output.print('   Hash: ' + style.dim(result.splitTree));
                } else {
                    output.print(style.error('❌ Trees differ'));
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

        fix: {
            description: 'Auto-fix split branches that fail verification',
            usage: 'fix',
            handler: function() {
                if (!planCache) {
                    output.print('No plan — run "plan" or "run" first.');
                    return;
                }
                output.print('Checking splits for verification failures...');
                var result = resolveConflicts(planCache);
                if (result.skipped) {
                    output.print('Skipped: ' + result.skipped);
                    return;
                }
                if (result.fixed.length > 0) {
                    output.print(style.success('Fixed:'));
                    for (var i = 0; i < result.fixed.length; i++) {
                        output.print('  ' + style.success('✓') + ' ' + result.fixed[i].name + ' (' + style.dim(result.fixed[i].strategy) + ')');
                    }
                }
                if (result.errors.length > 0) {
                    output.print(style.error('Unresolved:'));
                    for (var i = 0; i < result.errors.length; i++) {
                        output.print('  ' + style.error('❌') + ' ' + result.errors[i].name + ': ' + result.errors[i].error);
                    }
                }
                if (result.fixed.length === 0 && result.errors.length === 0) {
                    output.print('All splits pass verification — no fixes needed.');
                }
            }
        },

        'create-prs': {
            description: 'Push branches and create stacked GitHub PRs',
            usage: 'create-prs [--draft] [--push-only] [--auto-merge] [--merge-method squash|merge|rebase]',
            handler: function(args) {
                if (!planCache) {
                    output.print('No plan — run "plan" or "run" first.');
                    return;
                }
                if (executionResultCache.length === 0) {
                    output.print('No splits executed — run "execute" or "run" first.');
                    return;
                }

                var draft = true;
                var pushOnly = false;
                var autoMerge = runtime.autoMerge || false;
                var mergeMethod = runtime.mergeMethod || 'squash';
                if (args) {
                    for (var i = 0; i < args.length; i++) {
                        if (args[i] === '--no-draft') draft = false;
                        if (args[i] === '--push-only') pushOnly = true;
                        if (args[i] === '--auto-merge') autoMerge = true;
                        if (args[i] === '--merge-method' && i + 1 < args.length) {
                            mergeMethod = args[i + 1];
                            i++;
                        }
                    }
                }

                output.print('Creating PRs for ' + planCache.splits.length + ' splits...');
                if (draft) output.print('  Mode: draft');
                if (pushOnly) output.print('  Mode: push-only (no PR creation)');
                if (autoMerge) output.print('  Auto-merge: enabled (method: ' + mergeMethod + ')');

                var result = createPRs(planCache, {
                    draft: draft,
                    pushOnly: pushOnly,
                    autoMerge: autoMerge,
                    mergeMethod: mergeMethod
                });

                if (result.error) {
                    output.print('Error: ' + result.error);
                    return;
                }

                for (var i = 0; i < result.results.length; i++) {
                    var r = result.results[i];
                    if (r.error) {
                        output.print('  ' + style.error('❌') + ' ' + r.name + ': ' + r.error);
                    } else if (r.prUrl) {
                        var suffix = r.autoMerge ? ' (auto-merge queued)' : '';
                        output.print('  ' + style.success('✓') + ' ' + r.name + ' → ' + style.info(r.prUrl) + suffix);
                        if (r.mergeError) {
                            output.print('    ⚠ ' + r.mergeError);
                        }
                    } else {
                        output.print('  ' + style.success('✓') + ' ' + r.name + ' (pushed)');
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
                    case 'mode':
                        if (value !== 'auto' && value !== 'heuristic') {
                            output.print('Invalid mode: ' + value + '. Use "auto" or "heuristic".');
                            return;
                        }
                        runtime.mode = value;
                        break;
                    case 'retryBudget':
                    case 'retry-budget':
                        var budget = parseInt(value, 10);
                        if (isNaN(budget) || budget < 0) {
                            output.print('Invalid retry budget: ' + value + '. Must be a non-negative integer.');
                            return;
                        }
                        runtime.retryBudget = budget;
                        break;
                    case 'view':
                        if (value !== 'toggle' && value !== 'split') {
                            output.print('Invalid view: ' + value + '. Use "toggle" or "split".');
                            return;
                        }
                        runtime.view = value;
                        break;
                    case 'auto-merge':
                    case 'autoMerge':
                        runtime.autoMerge = (value === 'true' || value === '1');
                        break;
                    case 'merge-method':
                    case 'mergeMethod':
                        if (value !== 'squash' && value !== 'merge' && value !== 'rebase') {
                            output.print('Invalid merge method: ' + value + '. Use "squash", "merge", or "rebase".');
                            return;
                        }
                        runtime.mergeMethod = value;
                        break;
                    default:
                        output.print('Unknown key: ' + key);
                        return;
                }
                output.print('Set ' + key + ' = ' + value);
            }
        },

        run: {
            description: 'Run full workflow: analyze → group → plan → execute',
            usage: 'run [--mode auto|heuristic]',
            handler: function(args) {
                try {
                // Check if automated mode should be used.
                var useAuto = runtime.mode === 'auto';
                if (args && args.length > 0) {
                    for (var a = 0; a < args.length; a++) {
                        if (args[a] === '--mode' && a + 1 < args.length) {
                            useAuto = args[a + 1] === 'auto';
                            a++;
                        }
                    }
                }

                if (useAuto) {
                    // Try automated mode — falls back to heuristic if Claude unavailable.
                    if (!claudeExecutor) {
                        claudeExecutor = new ClaudeCodeExecutor(prSplitConfig);
                    }
                    if (claudeExecutor.isAvailable()) {
                        output.print(style.info('Mode: automated (Claude detected)'));
                        var autoConfig = {
                            baseBranch: runtime.baseBranch,
                            strategy: runtime.strategy
                        };
                        // If --timeout was specified, apply to all Claude timeouts.
                        if (prSplitConfig.timeoutMs > 0) {
                            autoConfig.classifyTimeoutMs = prSplitConfig.timeoutMs;
                            autoConfig.planTimeoutMs = prSplitConfig.timeoutMs;
                            autoConfig.resolveTimeoutMs = prSplitConfig.timeoutMs;
                        }
                        var result = automatedSplit(autoConfig);
                        if (result.error) {
                            output.print(style.error('Auto-split error: ' + result.error));
                        }
                        if (runtime.jsonOutput && result.report) {
                            output.print('');
                            output.print(JSON.stringify(result.report, null, 2));
                        }
                        return;
                    }
                    output.print(style.warning('Claude not available — using heuristic mode.'));
                }
                var workflowStart = Date.now();

                output.print(style.header('Running full PR split workflow...'));
                output.print('Base:     ' + style.bold(runtime.baseBranch));
                output.print('Strategy: ' + style.bold(runtime.strategy));
                output.print('Max:      ' + style.bold(String(runtime.maxFiles)));
                output.print('');

                // Step 1: Analyze
                var stepStart = Date.now();
                analysisCache = analyzeDiff({ baseBranch: runtime.baseBranch });
                if (analysisCache.error) {
                    output.print(style.error('Analysis failed: ' + analysisCache.error));
                    return;
                }
                if (!analysisCache.files || analysisCache.files.length === 0) {
                    output.print(style.warning('No changes found between ' + analysisCache.currentBranch +
                        ' and ' + analysisCache.baseBranch + '.'));
                    output.print('Ensure you are on a feature branch with changes against the base.');
                    return;
                }
                output.print(style.success('✓ Analysis: ') + analysisCache.files.length + ' changed files ' +
                    style.dim('(' + (Date.now() - stepStart) + 'ms)'));

                // Step 2: Group (heuristic)
                stepStart = Date.now();
                groupsCache = applyStrategy(analysisCache.files, runtime.strategy);

                var groupNames = Object.keys(groupsCache).sort();
                if (groupNames.length === 0) {
                    output.print('No groups created — strategy "' + runtime.strategy + '" produced no groups.');
                    return;
                }
                output.print(style.success('✓ Grouped into ' + groupNames.length + ' groups') +
                    ' (' + runtime.strategy + ')' +
                    ' ' + style.dim('(' + (Date.now() - stepStart) + 'ms)'));

                // Step 3: Plan
                stepStart = Date.now();
                planCache = createSplitPlan(groupsCache, {
                    baseBranch: runtime.baseBranch,
                    sourceBranch: analysisCache.currentBranch,
                    branchPrefix: runtime.branchPrefix,
                    verifyCommand: runtime.verifyCommand,
                    fileStatuses: analysisCache.fileStatuses
                });
                var validation = validatePlan(planCache);
                if (!validation.valid) {
                    output.print('Plan invalid: ' + validation.errors.join('; '));
                    return;
                }
                output.print(style.success('✓ Plan created: ' + planCache.splits.length + ' splits') +
                    ' ' + style.dim('(' + (Date.now() - stepStart) + 'ms)'));

                // Step 4: Execute (unless dry-run)
                if (runtime.dryRun) {
                    output.print('');
                    output.print(style.warning('DRY RUN') + ' — plan preview:');
                    for (var i = 0; i < planCache.splits.length; i++) {
                        var s = planCache.splits[i];
                        output.print('  ' + padIndex(i + 1) + '. ' + s.name + ' (' + s.files.length + ' files)');
                        for (var j = 0; j < s.files.length; j++) {
                            output.print('      ' + s.files[j]);
                        }
                    }
                    output.print('');
                    output.print('Use "set dry-run false" then "run" or "execute" to create branches.');
                    return;
                }

                output.print(style.info('Executing ' + planCache.splits.length + ' splits...'));
                stepStart = Date.now();
                var result = executeSplit(planCache);
                if (result.error) {
                    output.print(style.error('Execution failed: ' + result.error));
                    return;
                }
                executionResultCache = result.results;
                output.print(style.success('✓ Split executed: ' + result.results.length + ' branches created') +
                    ' ' + style.dim('(' + (Date.now() - stepStart) + 'ms)'));
                for (var i = 0; i < result.results.length; i++) {
                    var r = result.results[i];
                    output.print('  ' + style.success('✓') + ' ' + r.name + ' (' + r.files.length + ' files, SHA: ' + style.dim(r.sha.substring(0, 8)) + ')');
                }

                // Step 5: Verify equivalence
                var equiv = verifyEquivalence(planCache);
                if (equiv.equivalent) {
                    output.print(style.success('✅ Tree hash equivalence verified'));
                } else if (equiv.error) {
                    output.print(style.warning('⚠️  Equivalence check error: ' + equiv.error));
                } else {
                    output.print(style.error('❌ Tree hash mismatch — content may be lost'));
                    output.print('   Split tree:  ' + style.dim(equiv.splitTree));
                    output.print('   Source tree:  ' + style.dim(equiv.sourceTree));
                }

                var totalMs = Date.now() - workflowStart;
                output.print('');
                output.print(style.success('Done') + ' in ' + style.bold(totalMs < 1000 ? totalMs + 'ms' :
                    (totalMs / 1000).toFixed(1) + 's') + '.');

                // If --json flag is set, output the full report as JSON.
                if (runtime.jsonOutput) {
                    output.print('');
                    output.print(JSON.stringify(buildReport(), null, 2));
                }
                } catch (e) {
                    output.print('Error in run workflow: ' + (e && e.message ? e.message : String(e)));
                    if (e && e.stack) {
                        log.error('pr-split run error stack: ' + e.stack);
                    }
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

        'save-plan': {
            description: 'Save current plan to a JSON file',
            usage: 'save-plan [path]',
            handler: function(args) {
                var path = (args && args.length > 0) ? args[0] : undefined;
                var result = savePlan(path);
                if (result.error) {
                    output.print('Error: ' + result.error);
                    return;
                }
                output.print('Plan saved to ' + result.path);
                if (planCache && planCache.splits) {
                    output.print('  Splits: ' + planCache.splits.length);
                    output.print('  Executed: ' + executionResultCache.length);
                }
            }
        },

        'load-plan': {
            description: 'Load a previously-saved plan from a JSON file',
            usage: 'load-plan [path]',
            handler: function(args) {
                var path = (args && args.length > 0) ? args[0] : undefined;
                var result = loadPlan(path);
                if (result.error) {
                    output.print('Error: ' + result.error);
                    return;
                }
                output.print('Plan loaded from ' + result.path);
                output.print('  Total splits:   ' + result.totalSplits);
                output.print('  Executed:        ' + result.executedSplits);
                output.print('  Pending:         ' + result.pendingSplits);
                output.print('');
                output.print('Use "preview" to inspect, "execute" to run pending splits.');
            }
        },

        report: {
            description: 'Output current state as JSON',
            usage: 'report',
            handler: function() {
                output.print(JSON.stringify(buildReport(), null, 2));
            }
        },

        claude: {
            description: 'Switch to Claude Code TUI (Ctrl+] to return)',
            usage: 'claude [spawn]',
            handler: function(args) {
                try {
                    // Lazy-create executor.
                    if (!claudeExecutor) {
                        claudeExecutor = new ClaudeCodeExecutor(prSplitConfig);
                    }
                    // 'claude spawn' — resolve and spawn Claude.
                    if (args && args.length > 0 && args[0] === 'spawn') {
                        var resolveResult = claudeExecutor.resolve();
                        if (resolveResult.error) {
                            output.print('Error: ' + resolveResult.error);
                            return;
                        }
                        var spawnResult = claudeExecutor.spawn();
                        if (spawnResult.error) {
                            output.print('Error: ' + spawnResult.error);
                            return;
                        }
                        output.print('Claude spawned (session: ' + spawnResult.sessionId + ')');
                        return;
                    }
                    // 'claude' — switch to Claude's TUI.
                    if (!claudeExecutor.handle) {
                        output.print('No Claude process running. Use "claude spawn" first.');
                        return;
                    }
                    tuiMux.attach(claudeExecutor.handle);
                    output.print('Switching to Claude TUI… (Ctrl+] to return)');
                    var result = tuiMux.switchToClaude();
                    output.print('Back to osm. (reason: ' + result.reason + ')');
                    if (result.error) {
                        output.print('Error: ' + result.error);
                    }
                    try { tuiMux.detach(); } catch (e) { /* best effort */ }
                } catch (e) {
                    output.print('Error: ' + (e && e.message ? e.message : String(e)));
                    if (e && e.stack) { log.error('claude command error: ' + e.stack); }
                }
            }
        },

        'claude-status': {
            description: 'Show Claude Code process status',
            usage: 'claude-status',
            handler: function() {
                if (!claudeExecutor) {
                    output.print('Claude: not initialized');
                    return;
                }
                var resolved = claudeExecutor.resolved;
                var handle = claudeExecutor.handle;
                var sessionId = claudeExecutor.sessionId;
                output.print('Claude Status:');
                output.print('  Command:  ' + (resolved ? resolved.command + ' (' + resolved.type + ')' : 'not resolved'));
                output.print('  Session:  ' + (sessionId || 'none'));
                output.print('  Process:  ' + (handle ? (handle.isAlive ? (handle.isAlive() ? 'running' : 'exited') : 'unknown') : 'not spawned'));
                output.print('  Mux:      ' + (tuiMux.isClaudeActive() ? 'Claude active' : 'osm active'));
            }
        },

        'auto-split': {
            description: 'Automated split via Claude Code (falls back to heuristic)',
            usage: 'auto-split',
            handler: function() {
                try {
                    var autoConfig = {
                        baseBranch: runtime.baseBranch,
                        strategy: runtime.strategy,
                        maxGroups: 0
                    };
                    // If --timeout was specified, apply to all Claude timeouts.
                    if (prSplitConfig.timeoutMs > 0) {
                        autoConfig.classifyTimeoutMs = prSplitConfig.timeoutMs;
                        autoConfig.planTimeoutMs = prSplitConfig.timeoutMs;
                        autoConfig.resolveTimeoutMs = prSplitConfig.timeoutMs;
                    }
                    var result = automatedSplit(autoConfig);
                    if (result.error) {
                        output.print(style.error('Auto-split failed: ' + result.error));
                    }
                    if (runtime.jsonOutput && result.report) {
                        output.print('');
                        output.print(JSON.stringify(result.report, null, 2));
                    }
                } catch (e) {
                    output.print('Error: ' + (e && e.message ? e.message : String(e)));
                    if (e && e.stack) { log.error('auto-split error: ' + e.stack); }
                }
            }
        },

        'edit-plan': {
            description: 'Open interactive plan editor (BubbleTea TUI)',
            usage: 'edit-plan',
            handler: function() {
                if (!planCache || !planCache.splits || planCache.splits.length === 0) {
                    output.print('No plan to edit. Run "plan" first.');
                    return;
                }
                // Convert plan splits to editor items.
                var items = [];
                for (var i = 0; i < planCache.splits.length; i++) {
                    var s = planCache.splits[i];
                    items.push({
                        name: s.name || ('split-' + (i + 1)),
                        files: s.files || [],
                        branchName: s.branchName || s.name || '',
                        description: s.message || ''
                    });
                }
                // Create and run editor via Go factory (if available).
                if (typeof planEditorFactory !== 'undefined' && planEditorFactory.create) {
                    var editor = planEditorFactory.create(items);
                    output.print('[edit-plan] Opening interactive plan editor with ' + items.length + ' splits...');
                    var updatedItems;
                    try {
                        updatedItems = editor.run();
                    } catch (e) {
                        output.print('[edit-plan] Editor error: ' + (e && e.message ? e.message : String(e)));
                        return;
                    }
                    if (!updatedItems || updatedItems.length === 0) {
                        output.print('[edit-plan] No changes — editor returned empty result.');
                        return;
                    }
                    // Update planCache with edited items.
                    planCache.splits = [];
                    for (var u = 0; u < updatedItems.length; u++) {
                        var item = updatedItems[u];
                        planCache.splits.push({
                            name: item.name || item.branchName || ('split-' + (u + 1)),
                            files: item.files || [],
                            message: item.description || item.name || ('Split ' + (u + 1)),
                            order: u
                        });
                    }
                    output.print('[edit-plan] Plan updated: ' + planCache.splits.length + ' splits.');
                    for (var p = 0; p < planCache.splits.length; p++) {
                        output.print('  ' + (p + 1) + '. ' + planCache.splits[p].name + ' (' + planCache.splits[p].files.length + ' files)');
                    }
                } else {
                    output.print('[edit-plan] Plan editor not available (requires interactive mode).');
                    output.print('[edit-plan] Use text commands: move <file> <from> <to>, rename <idx> <name>, merge <a> <b>');
                }
            }
        },

        // T123: Diff visualization
        diff: {
            description: 'Show colorized diff for a specific split',
            usage: 'diff <split-index|split-name>',
            handler: function(args) {
                if (!planCache || !planCache.splits || planCache.splits.length === 0) {
                    output.print('No plan — run "plan" or "run" first.');
                    return;
                }
                if (!args || args.length === 0) {
                    output.print('Usage: diff <split-index|split-name>');
                    output.print('Available splits:');
                    for (var i = 0; i < planCache.splits.length; i++) {
                        output.print('  ' + (i + 1) + '. ' + planCache.splits[i].name);
                    }
                    return;
                }

                // Resolve split by index or name.
                var target = args.join(' ');
                var splitIndex = -1;
                var idx = parseInt(target, 10);
                if (!isNaN(idx) && idx >= 1 && idx <= planCache.splits.length) {
                    splitIndex = idx - 1;
                } else {
                    for (var s = 0; s < planCache.splits.length; s++) {
                        if (planCache.splits[s].name === target) {
                            splitIndex = s;
                            break;
                        }
                    }
                }
                if (splitIndex < 0) {
                    output.print('Unknown split: ' + target);
                    return;
                }

                output.print('Diff for split ' + (splitIndex + 1) + ': ' + planCache.splits[splitIndex].name);
                output.print('─'.repeat(60));
                var result = getSplitDiff(planCache, splitIndex);
                if (result.error) {
                    output.print('Error: ' + result.error);
                    return;
                }
                if (!result.diff) {
                    output.print('(empty diff)');
                    return;
                }
                output.print(renderColorizedDiff(result.diff));
            }
        },

        // T124: Claude conversation history
        'conversation': {
            description: 'Show Claude conversation history for this session',
            usage: 'conversation',
            handler: function() {
                var history = getConversationHistory();
                if (history.length === 0) {
                    output.print('No Claude conversations recorded in this session.');
                    output.print('Conversations are recorded during auto-split and conflict resolution.');
                    return;
                }
                output.print('Claude Conversation History (' + history.length + ' interactions):');
                output.print('');
                for (var i = 0; i < history.length; i++) {
                    var conv = history[i];
                    output.print('  [' + (i + 1) + '] ' + conv.action + ' @ ' + conv.timestamp);
                    if (conv.prompt) {
                        var promptPreview = conv.prompt.substring(0, 100);
                        if (conv.prompt.length > 100) promptPreview += '...';
                        output.print('      Prompt: ' + promptPreview);
                    }
                    if (conv.response) {
                        var responsePreview = conv.response.substring(0, 100);
                        if (conv.response.length > 100) responsePreview += '...';
                        output.print('      Response: ' + responsePreview);
                    }
                    output.print('');
                }
            }
        },

        // T127: Split dependency graph
        graph: {
            description: 'Show dependency graph between splits',
            usage: 'graph',
            handler: function() {
                if (!planCache || !planCache.splits || planCache.splits.length === 0) {
                    output.print('No plan — run "plan" or "run" first.');
                    return;
                }
                var depGraph = buildDependencyGraph(planCache, null);
                output.print(renderAsciiGraph(depGraph));

                // Also show independent pairs.
                var indPairs = assessIndependence(planCache, null);
                if (indPairs.length > 0) {
                    output.print('');
                    output.print('Independent pairs (can merge in parallel):');
                    for (var p = 0; p < indPairs.length; p++) {
                        output.print('  ' + indPairs[p][0] + '  ↔  ' + indPairs[p][1]);
                    }
                }
            }
        },

        // T129: Telemetry
        telemetry: {
            description: 'Show session telemetry (local, never sent externally)',
            usage: 'telemetry [save]',
            handler: function(args) {
                if (args && args[0] === 'save') {
                    var saveResult = saveTelemetry();
                    if (saveResult.error) {
                        output.print('Error saving telemetry: ' + saveResult.error);
                    } else {
                        output.print('Telemetry saved to: ' + saveResult.path);
                    }
                    return;
                }
                var summary = getTelemetrySummary();
                output.print('Session Telemetry (local only — never sent externally):');
                output.print('');
                output.print('  Files analyzed:      ' + summary.filesAnalyzed);
                output.print('  Splits created:      ' + summary.splitCount);
                output.print('  Strategy used:       ' + (summary.strategy || '(none)'));
                output.print('  Claude interactions: ' + summary.claudeInteractions);
                output.print('  Conflicts resolved:  ' + summary.conflictsResolved);
                output.print('  Conflicts failed:    ' + summary.conflictsFailed);
                output.print('  Session start:       ' + summary.startTime);
                output.print('');
                output.print('Use "telemetry save" to persist to .osm/telemetry/.');
            }
        },

        // T131: Retrospective analysis
        retro: {
            description: 'Analyze completed split — insights and suggestions',
            usage: 'retro',
            handler: function() {
                if (!planCache || !planCache.splits || planCache.splits.length === 0) {
                    output.print('No plan to analyze — run a full workflow first.');
                    return;
                }

                // Collect verification results if available.
                var verifyResults = null;
                if (executionResultCache && executionResultCache.length > 0) {
                    verifyResults = executionResultCache;
                }

                output.print('Retrospective Analysis');
                output.print('======================');
                output.print('');

                var result = analyzeRetrospective(planCache, verifyResults, null);

                // Stats.
                output.print('Statistics:');
                output.print('  Total files:      ' + result.stats.totalFiles);
                output.print('  Splits:           ' + result.stats.splitCount);
                output.print('  Avg files/split:  ' + result.stats.avgFiles);
                output.print('  Max files/split:  ' + result.stats.maxFiles);
                output.print('  Min files/split:  ' + result.stats.minFiles);
                output.print('  Size balance:     ' + result.stats.balance);
                output.print('  Failed splits:    ' + result.stats.failedSplits);
                output.print('');

                // Insights.
                output.print('Score: ' + result.score + '/100');
                output.print('');
                if (result.insights.length === 0) {
                    output.print('No specific insights — looks good!');
                } else {
                    output.print('Insights:');
                    for (var i = 0; i < result.insights.length; i++) {
                        var insight = result.insights[i];
                        var icon = insight.type === 'error' ? '❌' : (insight.type === 'warning' ? '⚠️' : (insight.type === 'success' ? '✅' : 'ℹ️'));
                        output.print('  ' + icon + ' ' + insight.message);
                        if (insight.suggestion) {
                            output.print('    → ' + insight.suggestion);
                        }
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
                output.print('  group [strategy] Group files (directory/extension/chunks/dependency/auto)');
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
                output.print('  create-prs       Push branches + create stacked GitHub PRs');
                output.print('  run              Full workflow: analyze→group→plan→execute');
                output.print('  auto-split       Automated split via Claude Code');
                output.print('  edit-plan        Interactive plan editor (BubbleTea TUI)');
                output.print('  diff <n|name>    Show colorized diff for a split');
                output.print('  conversation     Show Claude conversation history');
                output.print('  graph            Show dependency graph between splits');
                output.print('  telemetry [save] Show/save session telemetry (local only)');
                output.print('  retro            Retrospective: insights and suggestions');
                output.print('  set <key> <val>  Set runtime config (no args to show current)');
                output.print('  copy             Copy plan to clipboard');
                output.print('  report           Output current state as JSON');
                output.print('  save-plan [path] Save plan to file (default: .pr-split-plan.json)');
                output.print('  load-plan [path] Load plan from file');
                output.print('  claude [spawn]   Switch to Claude Code TUI (or spawn it)');
                output.print('  claude-status    Show Claude process status');
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
