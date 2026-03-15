'use strict';
// pr_split_11_utilities.js — Independence detection, BT nodes/templates,
//   diff visualization, conversation history, dependency graph, telemetry,
//   retrospective analysis.
// Dependencies: chunks 00-10 must be loaded first.
//
// Cross-chunk references (late-bound via prSplit.* inside function bodies):
//   Chunk 00: _dirname, _gitExec, _gitAddChangedFiles, _modules.bt,
//             _modules.exec, _modules.osmod, detectGoModulePath, parseGoImports
//   Chunk 01: analyzeDiff
//   Chunk 02: applyStrategy, selectStrategy
//   Chunk 03: createSplitPlan
//   Chunk 04: validatePlan
//   Chunk 05: executeSplit
//   Chunk 06: verifySplits, verifyEquivalence
//
// Shared state on prSplit._state:
//   conversationHistory (array), telemetryData (object)

(function(prSplit) {

    // -----------------------------------------------------------------------
    //  Independence Detection Helpers
    // -----------------------------------------------------------------------

    // extractDirs returns a set (object with true values) of directory paths
    // from a file list.
    function extractDirs(files) {
        var dirname = prSplit._dirname;
        var dirs = {};
        for (var i = 0; i < (files || []).length; i++) {
            dirs[dirname(files[i])] = true;
        }
        return dirs;
    }

    // extractGoImports returns a set of imported package paths from Go files.
    // Uses osmod.readFile for portability, with cat fallback.
    function extractGoImports(files) {
        var osmod = prSplit._modules.osmod;
        var exec = prSplit._modules.exec;
        var parseGoImports = prSplit.parseGoImports;
        var imports = {};
        for (var i = 0; i < (files || []).length; i++) {
            if (files[i].match(/\.go$/)) {
                var content = '';
                try {
                    if (osmod) {
                        var readResult = osmod.readFile(files[i]);
                        if (!readResult.error) {
                            content = readResult.content;
                        }
                    } else if (exec) {
                        var cat = exec.execv(['cat', files[i]]);
                        if (cat.code === 0) {
                            content = cat.stdout;
                        }
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
    // Accepts optional modulePath to avoid repeated detectGoModulePath() calls.
    function extractGoPkgs(files, modulePath) {
        var dirname = prSplit._dirname;
        var detectGoModulePath = prSplit.detectGoModulePath;
        var pkgs = {};
        if (typeof modulePath === 'undefined') {
            modulePath = detectGoModulePath();
        }
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

    // splitsAreIndependentFromMaps checks independence using pre-computed maps.
    // Pure function — no I/O.
    function splitsAreIndependentFromMaps(dirsA, dirsB, importsA, importsB, pkgsA, pkgsB) {
        for (var d in dirsA) {
            if (dirsB[d]) return false;
        }
        for (var imp in importsA) {
            if (pkgsB[imp]) return false;
        }
        for (var imp2 in importsB) {
            if (pkgsA[imp2]) return false;
        }
        return true;
    }

    // assessIndependence determines which split pairs can merge independently.
    // Returns an array of [nameA, nameB] pairs.
    // Pre-computes directory, import, and package maps once per split to
    // avoid O(N^2) repeated file reads and module path detection.
    function assessIndependence(plan, classification) {
        if (!plan || !plan.splits || plan.splits.length < 2) {
            return [];
        }
        var detectGoModulePath = prSplit.detectGoModulePath;
        var n = plan.splits.length;
        var modulePath = detectGoModulePath();
        var dirs = new Array(n);
        var imports = new Array(n);
        var pkgs = new Array(n);
        for (var k = 0; k < n; k++) {
            dirs[k] = extractDirs(plan.splits[k].files);
            imports[k] = extractGoImports(plan.splits[k].files);
            pkgs[k] = extractGoPkgs(plan.splits[k].files, modulePath);
        }
        var pairs = [];
        for (var i = 0; i < n; i++) {
            for (var j = i + 1; j < n; j++) {
                if (splitsAreIndependentFromMaps(dirs[i], dirs[j],
                    imports[i], imports[j], pkgs[i], pkgs[j])) {
                    pairs.push([plan.splits[i].name, plan.splits[j].name]);
                }
            }
        }
        return pairs;
    }

    // splitsAreIndependent checks if two splits have no directory overlap
    // and no import dependency overlap (for Go files).
    // Legacy wrapper — does NOT accept pre-computed modulePath.
    function splitsAreIndependent(a, b, classification) {
        var dirsA = extractDirs(a.files);
        var dirsB = extractDirs(b.files);
        for (var d in dirsA) {
            if (dirsB[d]) return false;
        }
        var importsA = extractGoImports(a.files);
        var importsB = extractGoImports(b.files);
        var pkgsA = extractGoPkgs(a.files);
        var pkgsB = extractGoPkgs(b.files);
        for (var imp in importsA) {
            if (pkgsB[imp]) return false;
        }
        for (var imp2 in importsB) {
            if (pkgsA[imp2]) return false;
        }
        return true;
    }

    // -----------------------------------------------------------------------
    //  Conversation History
    // -----------------------------------------------------------------------

    // recordConversation saves a Claude interaction to the history stored on
    // prSplit._state.conversationHistory.
    // T087: Maximum number of conversation entries to retain in memory.
    // Configurable via prSplitConfig.maxConversationHistory; defaults to 100.
    var MAX_CONVERSATION_HISTORY = (prSplit.runtime && prSplit.runtime.maxConversationHistory) || 100;
    var _conversationCapWarned = false;

    function recordConversation(action, prompt, response) {
        var state = prSplit._state;
        if (!state.conversationHistory) state.conversationHistory = [];
        state.conversationHistory.push({
            timestamp: new Date().toISOString(),
            action: action,
            prompt: prompt,
            response: response
        });
        // T087: Trim oldest entries when history exceeds the cap.
        if (state.conversationHistory.length > MAX_CONVERSATION_HISTORY) {
            state.conversationHistory = state.conversationHistory.slice(
                state.conversationHistory.length - MAX_CONVERSATION_HISTORY
            );
            if (!_conversationCapWarned && typeof log !== 'undefined' && log.warn) {
                log.warn('pr-split: conversation history capped at ' + MAX_CONVERSATION_HISTORY + ' entries — oldest entries discarded');
                _conversationCapWarned = true;
            }
        }
    }

    // getConversationHistory returns a defensive copy of all recorded
    // conversations.
    function getConversationHistory() {
        var state = prSplit._state;
        return (state.conversationHistory || []).slice();
    }

    // -----------------------------------------------------------------------
    //  Telemetry
    // -----------------------------------------------------------------------

    // recordTelemetry updates telemetry counters on prSplit._state.telemetryData.
    // Numeric keys are incremented; non-numeric keys are overwritten.
    function recordTelemetry(key, value) {
        var state = prSplit._state;
        if (!state.telemetryData) {
            state.telemetryData = {
                filesAnalyzed: 0,
                splitCount: 0,
                strategy: '',
                claudeInteractions: 0,
                conflictsResolved: 0,
                conflictsFailed: 0,
                startTime: new Date().toISOString(),
                endTime: null
            };
        }
        if (typeof state.telemetryData[key] === 'number') {
            state.telemetryData[key] += (typeof value === 'number' ? value : 1);
        } else {
            state.telemetryData[key] = value;
        }
    }

    // getTelemetrySummary returns the current telemetry data, setting endTime.
    // Initializes telemetryData with defaults if it hasn't been populated yet.
    function getTelemetrySummary() {
        var state = prSplit._state;
        if (!state.telemetryData) {
            state.telemetryData = {
                filesAnalyzed: 0,
                splitCount: 0,
                strategy: '',
                claudeInteractions: 0,
                conflictsResolved: 0,
                conflictsFailed: 0,
                startTime: new Date().toISOString(),
                endTime: null
            };
        }
        state.telemetryData.endTime = new Date().toISOString();
        return state.telemetryData;
    }

    // saveTelemetry persists telemetry to disk (opt-in, local only).
    // Uses osmod.writeFile with createDirs.
    function saveTelemetry(dir) {
        var osmod = prSplit._modules.osmod;
        dir = dir || '.osm/telemetry';
        if (!osmod) {
            return { error: 'osm:os module not available — cannot persist telemetry' };
        }
        try {
            var summary = getTelemetrySummary();
            var ts = (summary.startTime || new Date().toISOString()).replace(/[:.]/g, '-');
            var filename = dir + '/session-' + ts + '.json';
            var data = JSON.stringify(summary, null, 2);
            osmod.writeFile(filename, data, { createDirs: true });
            return { error: null, path: filename };
        } catch (e) {
            return { error: 'failed to save telemetry to "' + filename + '": ' + (e.message || String(e)) };
        }
    }

    // -----------------------------------------------------------------------
    //  Diff Visualization
    // -----------------------------------------------------------------------

    // renderColorizedDiff takes raw diff text and returns a styled version.
    // Uses lipgloss styles via prSplit._style for terminal-capability-aware
    // rendering. Falls back to plain text when lipgloss is not available.
    function renderColorizedDiff(diffText) {
        if (!diffText) return '';
        var s = prSplit._style;
        var lines = diffText.split('\n');
        var result = [];
        for (var i = 0; i < lines.length; i++) {
            var line = lines[i];
            if (line.indexOf('+') === 0 && line.indexOf('+++') !== 0) {
                result.push(s.diffAdd(line));
            } else if (line.indexOf('-') === 0 && line.indexOf('---') !== 0) {
                result.push(s.diffRemove(line));
            } else if (line.indexOf('@@') === 0) {
                result.push(s.diffHunk(line));
            } else if (line.indexOf('diff ') === 0 || line.indexOf('index ') === 0 ||
                       line.indexOf('---') === 0 || line.indexOf('+++') === 0) {
                result.push(s.diffMeta(line));
            } else {
                result.push(s.diffContext(line));
            }
        }
        return result.join('\n');
    }

    // getSplitDiff returns the git diff for a specific split.
    function getSplitDiff(plan, splitIndex) {
        var gitExec = prSplit._gitExec;
        var resolveDir = prSplit._resolveDir;
        if (!plan || !plan.splits || splitIndex < 0 || splitIndex >= plan.splits.length) {
            return { error: 'invalid split index ' + splitIndex + ' (plan has ' + (plan && plan.splits ? plan.splits.length : 0) + ' splits)', diff: '' };
        }
        var split = plan.splits[splitIndex];
        var dir = resolveDir(plan.dir || '.');
        var files = split.files || [];
        if (files.length === 0) {
            return { error: 'no files in split "' + (split.name || 'unnamed') + '" (index ' + splitIndex + ')', diff: '' };
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

    // -----------------------------------------------------------------------
    //  Dependency Graph
    // -----------------------------------------------------------------------

    // buildDependencyGraph creates an adjacency list from plan splits.
    // Each split that shares directory or import dependencies with another
    // gets an edge. Returns {nodes: [{name, index}], edges: [{from, to}]}.
    function buildDependencyGraph(plan, classification) {
        if (!plan || !plan.splits) return { nodes: [], edges: [] };
        var detectGoModulePath = prSplit.detectGoModulePath;
        var nodes = [];
        var n = plan.splits.length;
        for (var i = 0; i < n; i++) {
            nodes.push({ name: plan.splits[i].name, index: i });
        }
        var modulePath = detectGoModulePath();
        var dirs = new Array(n);
        var imports = new Array(n);
        var pkgs = new Array(n);
        for (var k = 0; k < n; k++) {
            dirs[k] = extractDirs(plan.splits[k].files);
            imports[k] = extractGoImports(plan.splits[k].files);
            pkgs[k] = extractGoPkgs(plan.splits[k].files, modulePath);
        }
        var edges = [];
        for (var a = 0; a < n; a++) {
            for (var b = a + 1; b < n; b++) {
                if (!splitsAreIndependentFromMaps(dirs[a], dirs[b],
                    imports[a], imports[b], pkgs[a], pkgs[b])) {
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

        // Topological-ish ordering: nodes with fewest deps first.
        var sorted = graph.nodes.slice().sort(function(a, b) {
            return adj[a.index].length - adj[b.index].length;
        });

        // Render each node with its connections.
        for (var n = 0; n < sorted.length; n++) {
            var node = sorted[n];
            var deps = adj[node.index];
            var marker = deps.length === 0 ? '\u25EF' : '\u25CF'; // ◯ : ●
            var line = '  ' + marker + ' [' + (node.index + 1) + '] ' + node.name;
            if (deps.length > 0) {
                var depNames = [];
                for (var d = 0; d < deps.length; d++) {
                    depNames.push('[' + (deps[d] + 1) + ']');
                }
                line += '  \u2190\u2192  ' + depNames.join(', '); // ←→
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

    // -----------------------------------------------------------------------
    //  Retrospective Analysis
    // -----------------------------------------------------------------------

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
                message: 'Split size imbalance: smallest split has ' + minFiles +
                    ' files, largest has ' + maxFiles + '. Consider rebalancing.',
                suggestion: 'Use "move" command to transfer files from large splits to small ones.'
            });
        }
        if (avgFiles > 20) {
            insights.push({
                type: 'info',
                message: 'Average split size is ' + Math.round(avgFiles) +
                    ' files \u2014 consider splitting further for easier review.',
                suggestion: 'Lower max-files setting or use a more granular strategy.'
            });
        }
        if (failedSplits.length > 0) {
            insights.push({
                type: 'error',
                message: failedSplits.length + ' splits failed verification: ' +
                    failedSplits.join(', '),
                suggestion: 'Use "fix" command to auto-repair, or manually check dependency ordering.'
            });
        }
        if (equivalenceResult && !equivalenceResult.equivalent) {
            insights.push({
                type: 'error',
                message: 'Tree-hash equivalence failed \u2014 combined splits do not reproduce original changes.',
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

        return {
            insights: insights,
            score: score,
            stats: {
                totalFiles: totalFiles,
                splitCount: plan.splits.length,
                avgFiles: Math.round(avgFiles * 10) / 10,
                maxFiles: maxFiles,
                minFiles: minFiles,
                balance: Math.round(balance * 100) + '%',
                failedSplits: failedSplits.length
            }
        };
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------

    // Independence detection.
    prSplit.extractDirs = extractDirs;
    prSplit.extractGoImports = extractGoImports;
    prSplit.extractGoPkgs = extractGoPkgs;
    prSplit.splitsAreIndependentFromMaps = splitsAreIndependentFromMaps;
    prSplit.assessIndependence = assessIndependence;
    prSplit._splitsAreIndependent = splitsAreIndependent;

    // Conversation history.
    prSplit.recordConversation = recordConversation;
    prSplit.getConversationHistory = getConversationHistory;

    // Telemetry.
    prSplit.recordTelemetry = recordTelemetry;
    prSplit.getTelemetrySummary = getTelemetrySummary;
    prSplit.saveTelemetry = saveTelemetry;

    // Diff visualization.
    prSplit.renderColorizedDiff = renderColorizedDiff;
    prSplit.getSplitDiff = getSplitDiff;

    // Dependency graph.
    prSplit.buildDependencyGraph = buildDependencyGraph;
    prSplit.renderAsciiGraph = renderAsciiGraph;

    // Retrospective analysis.
    prSplit.analyzeRetrospective = analyzeRetrospective;

})(globalThis.prSplit);
