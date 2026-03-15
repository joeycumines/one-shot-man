'use strict';
// pr_split_02_grouping.js — Grouping strategies & strategy selection
//
// Provides multiple strategies for grouping changed files into logical splits:
// directory, extension, pattern, chunks, dependency-aware (Go). Includes an
// auto-selection scorer that picks the best strategy heuristically.
//
// Dependencies: chunk 00 (prSplit._dirname, prSplit._fileExtension,
//               prSplit._modules.exec, prSplit._modules.osmod, prSplit.runtime)
// Go-injected globals: none (uses chunk 00's modules only)

(function(prSplit) {
    var dirname = prSplit._dirname;
    var fileExtension = prSplit._fileExtension;
    var exec = prSplit._modules.exec;
    var osmod = prSplit._modules.osmod;
    var runtime = prSplit.runtime;

    // groupByDirectory groups files by their top-level directory (at depth).
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

    // groupByExtension groups files by file extension.
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

    // groupByPattern groups files matching regex patterns in the given object.
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

    // groupByChunks splits files into N roughly-equal groups.
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

    // parseGoImports extracts import paths from Go source code.
    // Handles single-line and block import forms.
    function parseGoImports(content) {
        if (!content || typeof content !== 'string') return [];
        var imports = [];
        var lines = content.split('\n');
        var inBlock = false;

        for (var i = 0; i < lines.length; i++) {
            var line = lines[i].trim();

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

            if (!inBlock && line.indexOf('import') === 0 && line.indexOf('(') >= 0) {
                inBlock = true;
                var qi = line.indexOf('"');
                if (qi >= 0) {
                    var qi2 = line.indexOf('"', qi + 1);
                    if (qi2 > qi) {
                        imports.push(line.substring(qi + 1, qi2));
                    }
                }
                continue;
            }

            if (inBlock && line.indexOf(')') >= 0) {
                inBlock = false;
                continue;
            }

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

            if (line.indexOf('func ') === 0 || line.indexOf('type ') === 0 ||
                line.indexOf('var ') === 0 || line.indexOf('const ') === 0 ||
                line.indexOf('var(') === 0 || line.indexOf('const(') === 0) {
                break;
            }
        }

        return imports;
    }

    // detectGoModulePath reads go.mod and returns the module path, or ''.
    function detectGoModulePath() {
        var content = '';
        if (osmod) {
            var result = osmod.readFile('go.mod');
            if (result.error) {
                return '';
            }
            content = result.content;
        } else {
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

    // groupByDependency groups Go files by package+import relationships
    // using union-find. Non-Go files are placed in nearest matching group.
    function groupByDependency(files, options) {
        if (!files || !files.length) return {};
        options = options || {};

        var goFiles = [];
        var otherFiles = [];
        for (var i = 0; i < files.length; i++) {
            if (files[i].length > 3 && files[i].substring(files[i].length - 3) === '.go') {
                goFiles.push(files[i]);
            } else {
                otherFiles.push(files[i]);
            }
        }

        if (goFiles.length === 0) {
            return groupByDirectory(files, 1);
        }

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

        var modulePath = detectGoModulePath();

        // Union-Find.
        var parent = {};
        for (var i = 0; i < pkgDirs.length; i++) {
            parent[pkgDirs[i]] = pkgDirs[i];
        }

        function find(x) {
            while (parent[x] !== x) {
                parent[x] = parent[parent[x]];
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

        if (modulePath && pkgDirs.length > 1) {
            for (var i = 0; i < goFiles.length; i++) {
                if (goFiles[i].substring(goFiles[i].length - 8) === '_test.go') {
                    continue;
                }

                var filePkgParts = goFiles[i].split('/');
                var filePkg = filePkgParts.length > 1
                    ? filePkgParts.slice(0, -1).join('/')
                    : '.';

                var fileContent = '';
                if (osmod) {
                    var readResult = osmod.readFile(goFiles[i]);
                    if (readResult.error) {
                        continue;
                    }
                    fileContent = readResult.content;
                } else {
                    var catResult = exec.execv(['cat', goFiles[i]]);
                    if (catResult.code !== 0) {
                        continue;
                    }
                    fileContent = catResult.stdout;
                }

                var imports = parseGoImports(fileContent);
                for (var j = 0; j < imports.length; j++) {
                    var imp = imports[j];
                    if (imp.indexOf(modulePath + '/') !== 0) {
                        continue;
                    }
                    var relPath = imp.substring(modulePath.length + 1);
                    if (parent[relPath] !== undefined) {
                        union(filePkg, relPath);
                    }
                }
            }
        }

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

        for (var i = 0; i < otherFiles.length; i++) {
            var otherParts = otherFiles[i].split('/');
            var otherDir = otherParts.length > 1
                ? otherParts.slice(0, -1).join('/')
                : '.';

            var placed = false;

            if (groups[otherDir]) {
                groups[otherDir].push(otherFiles[i]);
                placed = true;
            }

            if (!placed && parent[otherDir] !== undefined) {
                var resolved = find(otherDir);
                if (groups[resolved]) {
                    groups[resolved].push(otherFiles[i]);
                    placed = true;
                }
            }

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

    // selectStrategy auto-detects the best grouping strategy by scoring.
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

    // ─── Async variants (T092) ──────────────────────────────────────────
    //
    // groupByDependencyAsync: Non-blocking version of groupByDependency.
    // Uses exec.spawn (via shellExecAsync) for file reads in the fallback
    // path, and yields to the event loop periodically (every 20 files) when
    // using the osmod fast-path. This prevents the Goja event loop from
    // being blocked for >50 ms even on repos with 200+ changed .go files.
    //
    // groupByDirectory, groupByExtension, groupByChunks, groupByPattern are
    // pure in-memory (no I/O) — they complete in <1 ms and need no async variant.

    async function groupByDependencyAsync(files, options) {
        if (!files || !files.length) return {};
        options = options || {};

        var shellExecAsync = prSplit._shellExecAsync;
        var shellQuote = prSplit._shellQuote;

        var goFiles = [];
        var otherFiles = [];
        for (var i = 0; i < files.length; i++) {
            if (files[i].length > 3 && files[i].substring(files[i].length - 3) === '.go') {
                goFiles.push(files[i]);
            } else {
                otherFiles.push(files[i]);
            }
        }

        if (goFiles.length === 0) {
            return groupByDirectory(files, 1);
        }

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

        var modulePath = detectGoModulePath();

        // Union-Find (same as sync version).
        var parent = {};
        for (var i = 0; i < pkgDirs.length; i++) {
            parent[pkgDirs[i]] = pkgDirs[i];
        }
        function find(x) {
            while (parent[x] !== x) {
                parent[x] = parent[parent[x]];
                x = parent[x];
            }
            return x;
        }
        function union(a, b) {
            var ra = find(a);
            var rb = find(b);
            if (ra !== rb) { parent[ra] = rb; }
        }

        if (modulePath && pkgDirs.length > 1) {
            for (var i = 0; i < goFiles.length; i++) {
                if (goFiles[i].substring(goFiles[i].length - 8) === '_test.go') {
                    continue;
                }

                // Yield to event loop every 20 files so BubbleTea can render.
                if (i > 0 && i % 20 === 0) {
                    await Promise.resolve();
                }

                var filePkgParts = goFiles[i].split('/');
                var filePkg = filePkgParts.length > 1
                    ? filePkgParts.slice(0, -1).join('/')
                    : '.';

                var fileContent = '';
                if (osmod) {
                    // osmod.readFile is a fast Go syscall (~0.1 ms per file).
                    var readResult = osmod.readFile(goFiles[i]);
                    if (readResult.error) { continue; }
                    fileContent = readResult.content;
                } else if (shellExecAsync) {
                    // Async fallback: exec.spawn('sh', ['-c', 'cat FILE']).
                    // Genuinely non-blocking — yields to event loop during I/O.
                    var catResult = await shellExecAsync('cat ' + shellQuote(goFiles[i]));
                    if (catResult.error) { continue; }
                    fileContent = catResult.stdout;
                } else {
                    // Last resort: sync exec (only if neither osmod nor spawn available).
                    var catResult = exec.execv(['cat', goFiles[i]]);
                    if (catResult.code !== 0) { continue; }
                    fileContent = catResult.stdout;
                }

                var imports = parseGoImports(fileContent);
                for (var j = 0; j < imports.length; j++) {
                    var imp = imports[j];
                    if (imp.indexOf(modulePath + '/') !== 0) { continue; }
                    var relPath = imp.substring(modulePath.length + 1);
                    if (parent[relPath] !== undefined) {
                        union(filePkg, relPath);
                    }
                }
            }
        }

        // Build groups (same as sync version — pure compute, no I/O).
        var groups = {};
        for (var i = 0; i < pkgDirs.length; i++) {
            var root = find(pkgDirs[i]);
            if (!groups[root]) { groups[root] = []; }
            var fileList = pkgFiles[pkgDirs[i]];
            for (var j = 0; j < fileList.length; j++) {
                groups[root].push(fileList[j]);
            }
        }

        for (var i = 0; i < otherFiles.length; i++) {
            var otherParts = otherFiles[i].split('/');
            var otherDir = otherParts.length > 1
                ? otherParts.slice(0, -1).join('/')
                : '.';
            var placed = false;
            if (groups[otherDir]) {
                groups[otherDir].push(otherFiles[i]);
                placed = true;
            }
            if (!placed && parent[otherDir] !== undefined) {
                var resolved = find(otherDir);
                if (groups[resolved]) {
                    groups[resolved].push(otherFiles[i]);
                    placed = true;
                }
            }
            if (!placed) {
                var fallbackDir = dirname(otherFiles[i], 1);
                if (!groups[fallbackDir]) { groups[fallbackDir] = []; }
                groups[fallbackDir].push(otherFiles[i]);
            }
        }

        return groups;
    }

    // selectStrategyAsync: Non-blocking version of selectStrategy.
    // The only I/O-bound strategy is 'dependency' — all others are in-memory.
    // We compute the 4 fast strategies synchronously, then await the async
    // dependency strategy, and score all 5 together.
    async function selectStrategyAsync(files, options) {
        options = options || {};
        var maxPerGroup = options.maxPerGroup || runtime.maxFiles;

        // Fast strategies (pure compute, sub-millisecond).
        var fastStrategies = [
            { name: 'directory', groups: groupByDirectory(files, 1) },
            { name: 'directory-deep', groups: groupByDirectory(files, 2) },
            { name: 'extension', groups: groupByExtension(files) },
            { name: 'chunks', groups: groupByChunks(files, maxPerGroup) }
        ];

        // Async dependency strategy (may do file I/O).
        var depGroups = await groupByDependencyAsync(files, options);

        var strategies = fastStrategies.concat([
            { name: 'dependency', groups: depGroups }
        ]);

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

    // applyStrategyAsync: Non-blocking version of applyStrategy.
    // Routes 'dependency' and 'auto' to their async variants; all other
    // strategies are in-memory and return synchronously (wrapped in a resolved
    // promise for consistent caller semantics).
    async function applyStrategyAsync(files, strategy, options) {
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
                return await groupByDependencyAsync(files, options);
            case 'auto':
                return (await selectStrategyAsync(files, options)).groups;
            default:
                return groupByDirectory(files, 1);
        }
    }

    // Attach exports.
    prSplit.groupByDirectory = groupByDirectory;
    prSplit.groupByExtension = groupByExtension;
    prSplit.groupByPattern = groupByPattern;
    prSplit.groupByChunks = groupByChunks;
    prSplit.parseGoImports = parseGoImports;
    prSplit.detectGoModulePath = detectGoModulePath;
    prSplit.groupByDependency = groupByDependency;
    prSplit.applyStrategy = applyStrategy;
    prSplit.selectStrategy = selectStrategy;
    // T092: async variants for TUI use (non-blocking event loop).
    prSplit.groupByDependencyAsync = groupByDependencyAsync;
    prSplit.selectStrategyAsync = selectStrategyAsync;
    prSplit.applyStrategyAsync = applyStrategyAsync;
})(globalThis.prSplit);
