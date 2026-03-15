'use strict';
// pr_split_04_validation.js — Classification, Plan, SplitPlan, Resolution validation
// Dependencies: none — all 4 validators are pure functions.
// Attaches to prSplit: validateClassification, validatePlan,
//   validateSplitPlan, validateResolution.

(function(prSplit) {

    // -----------------------------------------------------------------------
    //  validateClassification — validates Claude classification output
    // -----------------------------------------------------------------------
    // Accepts the categories array format: [{name, description, files}, ...].
    // Optionally cross-checks file paths against knownFiles (array of paths).
    // Returns {valid: true} or {valid: false, errors: [...]}.
    function validateClassification(categories, knownFiles) {
        var errors = [];

        if (!categories || !Array.isArray(categories) || categories.length === 0) {
            errors.push('categories must be a non-empty array');
            return { valid: false, errors: errors };
        }

        var allFiles = {};
        var duplicates = [];

        for (var i = 0; i < categories.length; i++) {
            var cat = categories[i];

            if (!cat || typeof cat !== 'object') {
                errors.push('category at index ' + i + ' is not an object');
                continue;
            }

            if (!cat.name || typeof cat.name !== 'string' || cat.name.trim() === '') {
                errors.push('category at index ' + i + ' has no name');
            }

            if (!cat.description || typeof cat.description !== 'string' || cat.description.trim() === '') {
                errors.push('category at index ' + i + ' (' + (cat.name || 'unnamed') + ') has no description');
            }

            if (!cat.files || !Array.isArray(cat.files) || cat.files.length === 0) {
                errors.push('category ' + (cat.name || 'at index ' + i) + ' has no files');
                continue;
            }

            for (var j = 0; j < cat.files.length; j++) {
                var f = cat.files[j];
                if (typeof f !== 'string' || f.trim() === '') {
                    errors.push('category ' + cat.name + ' has empty/invalid file at index ' + j);
                    continue;
                }
                if (allFiles[f]) {
                    duplicates.push(f + ' (in ' + allFiles[f] + ' and ' + cat.name + ')');
                } else {
                    allFiles[f] = cat.name;
                }
            }
        }

        if (duplicates.length > 0) {
            errors.push('duplicate files across categories: ' + duplicates.join(', '));
        }

        // Warn about unknown files but don't fail validation.
        if (knownFiles && Array.isArray(knownFiles) && knownFiles.length > 0) {
            var knownSet = {};
            for (var k = 0; k < knownFiles.length; k++) {
                knownSet[knownFiles[k]] = true;
            }
            var unknown = [];
            for (var path in allFiles) {
                if (!knownSet[path]) {
                    unknown.push(path);
                }
            }
            if (unknown.length > 0 && typeof log !== 'undefined' && log.printf) {
                log.printf('validateClassification: %d unknown files (not in diff): %s',
                    unknown.length, unknown.join(', '));
            }
        }

        return { valid: errors.length === 0, errors: errors };
    }

    // -----------------------------------------------------------------------
    //  validatePlan — validates internal split plan structure
    // -----------------------------------------------------------------------
    // T096: Share the same invalid branch-name character regex used by
    // validateSplitPlan so that user-edited plans loaded from JSON are also
    // validated.  Git rejects names containing spaces, tildes (~), carets (^),
    // colons (:), backslashes (\), asterisks (*), question marks (?), or
    // open brackets ([).  We also reject double-dot (..) and .lock suffix.
    var INVALID_BRANCH_CHARS = /[\s~\^:\\*\?\[]/;

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

            if (!split.name || (typeof split.name === 'string' && split.name.trim() === '')) {
                errors.push('split at index ' + i + ' has no name');
            } else if (typeof split.name === 'string') {
                // T096: Validate branch name characters.
                if (INVALID_BRANCH_CHARS.test(split.name)) {
                    var m = INVALID_BRANCH_CHARS.exec(split.name);
                    errors.push('split "' + split.name + '" contains invalid branch name character at position ' + m.index);
                }
                if (split.name.indexOf('..') !== -1) {
                    errors.push('split "' + split.name + '" contains ".." which is invalid in branch names');
                }
                if (split.name.slice(-5) === '.lock') {
                    errors.push('split "' + split.name + '" ends with ".lock" which is reserved by git');
                }
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

    // -----------------------------------------------------------------------
    //  validateSplitPlan — validates Claude-generated split plan (stages)
    // -----------------------------------------------------------------------
    // The plan has a stages/splits array where each element has
    // {name, files, ...}.
    function validateSplitPlan(stages) {
        var errors = [];

        if (!stages || !Array.isArray(stages) || stages.length === 0) {
            errors.push('stages must be a non-empty array');
            return { valid: false, errors: errors };
        }

        var allFiles = {};
        var duplicates = [];

        for (var i = 0; i < stages.length; i++) {
            var stage = stages[i];

            if (!stage || typeof stage !== 'object') {
                errors.push('stage at index ' + i + ' is not an object');
                continue;
            }

            if (!stage.name || typeof stage.name !== 'string' || stage.name.trim() === '') {
                errors.push('stage at index ' + i + ' has no name');
            } else if (INVALID_BRANCH_CHARS.test(stage.name)) {
                errors.push('stage ' + stage.name + ' has invalid branch name characters');
            }

            if (!stage.files || !Array.isArray(stage.files) || stage.files.length === 0) {
                errors.push('stage ' + (stage.name || 'at index ' + i) + ' has no files');
                continue;
            }

            for (var j = 0; j < stage.files.length; j++) {
                var f = stage.files[j];
                if (typeof f !== 'string' || f.trim() === '') {
                    errors.push('stage ' + stage.name + ' has empty/invalid file at index ' + j);
                    continue;
                }
                if (allFiles[f]) {
                    duplicates.push(f + ' (in ' + allFiles[f] + ' and ' + stage.name + ')');
                } else {
                    allFiles[f] = stage.name;
                }
            }
        }

        if (duplicates.length > 0) {
            errors.push('duplicate files across stages: ' + duplicates.join(', '));
        }

        return { valid: errors.length === 0, errors: errors };
    }

    // -----------------------------------------------------------------------
    //  validateResolution — validates Claude conflict resolution output
    // -----------------------------------------------------------------------
    // A valid resolution has at least one of:
    //   - patches: non-empty array of {file, content} objects
    //   - commands: non-empty array of {command, ...} objects
    //   - preExistingFailure: true
    function validateResolution(resolution) {
        var errors = [];

        if (!resolution || typeof resolution !== 'object') {
            errors.push('resolution must be an object');
            return { valid: false, errors: errors };
        }

        var hasPatches = resolution.patches && Array.isArray(resolution.patches) && resolution.patches.length > 0;
        var hasCommands = resolution.commands && Array.isArray(resolution.commands) && resolution.commands.length > 0;
        var hasPreExisting = !!resolution.preExistingFailure;

        if (!hasPatches && !hasCommands && !hasPreExisting) {
            errors.push('resolution must have at least one of: patches, commands, or preExistingFailure');
            return { valid: false, errors: errors };
        }

        // T097: preExistingFailure requires a non-empty reason explaining why
        // the failure pre-dates this split, to prevent silent no-ops masking real issues.
        if (hasPreExisting) {
            if (!resolution.reason || typeof resolution.reason !== 'string' || resolution.reason.trim() === '') {
                errors.push('preExistingFailure:true requires a non-empty reason field');
            }
            if (!hasPatches && !hasCommands) {
                // Pure pre-existing marker — log warning for operator awareness.
                if (typeof log !== 'undefined' && log.warn) {
                    log.warn('pr-split: resolution accepts preExistingFailure without patches/commands — verify this is intentional');
                }
            }
        }

        if (hasPatches) {
            for (var i = 0; i < resolution.patches.length; i++) {
                var patch = resolution.patches[i];
                if (!patch || typeof patch !== 'object') {
                    errors.push('patches[' + i + '] must be an object with file and content');
                } else {
                    if (!patch.file || typeof patch.file !== 'string' || patch.file.trim() === '') {
                        errors.push('patches[' + i + '] must have a non-empty file path');
                    }
                    if (typeof patch.content !== 'string') {
                        errors.push('patches[' + i + '] must have a content string');
                    }
                }
            }
        }

        if (hasCommands) {
            for (var j = 0; j < resolution.commands.length; j++) {
                var cmd = resolution.commands[j];
                if (!cmd || typeof cmd !== 'object') {
                    errors.push('commands[' + j + '] must be an object');
                } else if (!cmd.command || typeof cmd.command !== 'string' || cmd.command.trim() === '') {
                    errors.push('commands[' + j + '] must have a non-empty command string');
                }
            }
        }

        return { valid: errors.length === 0, errors: errors };
    }

    // -----------------------------------------------------------------------
    //  Exports
    // -----------------------------------------------------------------------
    prSplit.validateClassification = validateClassification;
    prSplit.validatePlan = validatePlan;
    prSplit.validateSplitPlan = validateSplitPlan;
    prSplit.validateResolution = validateResolution;
    // T096: Shared regex for cross-chunk branch name validation (e.g. rename dialog).
    prSplit.INVALID_BRANCH_CHARS = INVALID_BRANCH_CHARS;
})(globalThis.prSplit);
