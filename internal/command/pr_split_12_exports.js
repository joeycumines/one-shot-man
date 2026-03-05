'use strict';
// pr_split_12_exports.js — Export manifest validation & VERSION
// Dependencies: all prior chunks (00-11) must be loaded first.
//
// This chunk verifies that every expected export exists on globalThis.prSplit
// and warns about missing ones. It also sets the VERSION constant.

(function(prSplit) {

    // Version of the chunked pr-split implementation.
    prSplit.VERSION = '6.0.0';

    // -----------------------------------------------------------------------
    //  Export Manifest — every name that chunks 00-11 should have attached.
    // -----------------------------------------------------------------------

    var EXPECTED_EXPORTS = [
        // Chunk 00: Core
        '_state', '_modules', '_style', '_cfg', '_COMMAND_NAME', '_MODE_NAME',
        'runtime', '_gitExec', '_resolveDir', '_shellQuote', '_gitAddChangedFiles',
        '_dirname', '_fileExtension', '_sanitizeBranchName', '_padIndex',
        'discoverVerifyCommand', 'scopedVerifyCommand',
        'isCancelled', '_isPaused', '_isForceCancelled',

        // Chunk 01: Analysis
        'analyzeDiff', 'analyzeDiffStats',

        // Chunk 02: Grouping
        'groupByDirectory', 'groupByExtension', 'groupByPattern',
        'groupByChunks', 'groupByDependency',
        'parseGoImports', 'detectGoModulePath',
        'applyStrategy', 'selectStrategy',

        // Chunk 03: Planning
        'DEFAULT_PLAN_PATH', 'createSplitPlan', 'savePlan', 'loadPlan',

        // Chunk 04: Validation
        'validateClassification', 'validatePlan',
        'validateSplitPlan', 'validateResolution',

        // Chunk 05: Execution
        'executeSplit',

        // Chunk 06: Verification
        'verifySplit', 'verifySplits',
        'verifyEquivalence', 'verifyEquivalenceDetailed',
        'cleanupBranches',

        // Chunk 07: PR creation
        'createPRs',

        // Chunk 08: Conflict resolution
        'resolveConflicts', 'AUTO_FIX_STRATEGIES',

        // Chunk 09: Claude
        'ClaudeCodeExecutor',
        'renderPrompt', 'renderClassificationPrompt',
        'renderSplitPlanPrompt', 'renderConflictPrompt',
        'detectLanguage',
        'CLASSIFICATION_PROMPT_TEMPLATE',
        'SPLIT_PLAN_PROMPT_TEMPLATE',
        'CONFLICT_RESOLUTION_PROMPT_TEMPLATE',

        // Chunk 10: Pipeline
        'AUTOMATED_DEFAULTS', 'SEND_TEXT_NEWLINE_DELAY_MS',
        'sendToHandle', 'waitForLogged',
        'classificationToGroups', 'cleanupExecutor',
        'heuristicFallback', 'resolveConflictsWithClaude',
        'automatedSplit',

        // Chunk 11: Utilities
        'extractDirs', 'extractGoImports', 'extractGoPkgs',
        'splitsAreIndependentFromMaps', 'assessIndependence',
        '_splitsAreIndependent',
        'recordConversation', 'getConversationHistory',
        'recordTelemetry', 'getTelemetrySummary', 'saveTelemetry',
        'renderColorizedDiff', 'getSplitDiff',
        'buildDependencyGraph', 'renderAsciiGraph',
        'analyzeRetrospective'
    ];

    // -----------------------------------------------------------------------
    //  Validation — log warnings for missing exports.
    // -----------------------------------------------------------------------

    var missing = [];
    for (var i = 0; i < EXPECTED_EXPORTS.length; i++) {
        var name = EXPECTED_EXPORTS[i];
        if (typeof prSplit[name] === 'undefined') {
            missing.push(name);
        }
    }

    if (missing.length > 0 && typeof log !== 'undefined' && log &&
        typeof log.printf === 'function') {
        log.printf('pr-split export manifest: %d missing exports: %s',
            missing.length, missing.join(', '));
    }

    // Expose manifest and validation result for testing.
    prSplit._EXPECTED_EXPORTS = EXPECTED_EXPORTS;
    prSplit._missingExports = missing;

})(globalThis.prSplit);
