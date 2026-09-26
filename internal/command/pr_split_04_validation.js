'use strict';
// pr_split_04_validation.js — Classification, Plan, SplitPlan, Resolution validation
// Dependencies: none — validators are pure; patch application is isolated below.
// Attaches to prSplit: validateClassification, validatePlan,
//   validateSplitPlan, validateResolution.

(function(prSplit) {

    // --- validateClassification — validates Agent classification output ---
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

    // --- validatePlan — validates internal split plan structure ---
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

    // --- validateSplitPlan — validates Agent-generated split plan (stages) ---
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

    // --- validateResolution — validates Agent conflict resolution output ---
    // A valid resolution has at least one of:
    //   - patches: non-empty array of {file, content} objects
    //   - commands: non-empty array of command strings
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
            var seenPaths = Object.create(null);
            for (var i = 0; i < resolution.patches.length; i++) {
                var patch = resolution.patches[i];
                if (!patch || typeof patch !== 'object') {
                    errors.push('patches[' + i + '] must be an object with file and content');
                } else {
                    var pathError = validateResolutionPath(patch.file);
                    if (pathError) {
                        errors.push('patches[' + i + ']: ' + pathError);
                    } else {
                        var normalizedPath = patch.file.split(String.fromCharCode(92)).join('/');
                        if (seenPaths[normalizedPath]) {
                            errors.push('patches[' + i + '] duplicates file ' + normalizedPath);
                        }
                        seenPaths[normalizedPath] = true;
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
                if (typeof cmd !== 'string' || cmd.trim() === '') {
                    errors.push('commands[' + j + '] must be a non-empty command string');
                }
            }
        }

        return { valid: errors.length === 0, errors: errors };
    }

    // validateResolutionPath accepts only a relative path inside the resolution
    // worktree. Agent output is untrusted; rejecting traversal components here
    // prevents a patch from escaping the temporary checkout on every platform.
    function validateResolutionPath(file) {
        if (typeof file !== 'string' || file.trim() === '') {
            return 'must have a non-empty file path relative to the resolution worktree';
        }
        if (file.indexOf(String.fromCharCode(0)) !== -1) {
            return 'must not contain NUL bytes';
        }
        var normalized = file.split(String.fromCharCode(92)).join('/');
        if (normalized.charAt(0) === '/' || /^[A-Za-z]:\//.test(normalized) ||
            normalized.indexOf(':') !== -1 || normalized.charAt(0) === '~') {
            return 'must be relative to the resolution worktree';
        }
        var parts = normalized.split('/');
        for (var i = 0; i < parts.length; i++) {
            if (parts[i] === '' || parts[i] === '.' || parts[i] === '..') {
                return 'must not contain empty, ".", or ".." path components';
            }
        }
        return null;
    }

    function resolutionPath(root, file) {
        if (typeof root !== 'string' || root.trim() === '') {
            return { path: null, error: 'resolution worktree path is missing' };
        }
        var pathError = validateResolutionPath(file);
        if (pathError) {
            return { path: null, error: pathError };
        }
        var normalizedRoot = String(root || '');
        var separator = String.fromCharCode(92);
        while (normalizedRoot.length > 1 &&
               (normalizedRoot.charAt(normalizedRoot.length - 1) === '/' ||
                normalizedRoot.charAt(normalizedRoot.length - 1) === separator)) {
            normalizedRoot = normalizedRoot.substring(0, normalizedRoot.length - 1);
        }
        var normalizedFile = file.split(separator).join('/');
        return { path: normalizedRoot + '/' + normalizedFile, error: null };
    }

    // applyResolutionPatches is the single write path for Agent-provided patch
    // content. It revalidates paths at the point of use and awaits every write;
    // callers must not commit a partially written worktree after an error.
    async function applyResolutionPatches(resolution, root) {
        if (!resolution || !Array.isArray(resolution.patches) || resolution.patches.length === 0) {
            return { error: null };
        }
        var osmod = prSplit._modules && prSplit._modules.osmod;
        if (!osmod || typeof osmod.writeFileScoped !== 'function') {
            return { error: 'osm:os scoped writer unavailable — cannot apply resolution patches' };
        }
        var seen = Object.create(null);
        for (var i = 0; i < resolution.patches.length; i++) {
            var patch = resolution.patches[i];
            if (!patch || typeof patch !== 'object' || typeof patch.content !== 'string') {
                return { error: 'invalid resolution patch at index ' + i };
            }
            var target = resolutionPath(root, patch.file);
            if (target.error) {
                return { error: 'patches[' + i + ']: ' + target.error };
            }
            var normalized = patch.file.split(String.fromCharCode(92)).join('/');
            if (seen[normalized]) {
                return { error: 'duplicate resolution patch for ' + normalized };
            }
            seen[normalized] = true;
            try {
                await osmod.writeFileScoped(root, patch.file, patch.content, { createDirs: true });
            } catch (e) {
                return { error: 'failed to write patch ' + normalized + ': ' + (e && e.message ? e.message : String(e)) };
            }
        }
        return { error: null };
    }

    // --- Exports ---
    prSplit.validateClassification = validateClassification;
    prSplit.validatePlan = validatePlan;
    prSplit.validateSplitPlan = validateSplitPlan;
    prSplit.validateResolution = validateResolution;
    prSplit._validateResolutionPath = validateResolutionPath;
    prSplit._applyResolutionPatches = applyResolutionPatches;
    // T096: Shared regex for cross-chunk branch name validation (e.g. rename dialog).
    prSplit.INVALID_BRANCH_CHARS = INVALID_BRANCH_CHARS;
})(globalThis.prSplit);
