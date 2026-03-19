'use strict';
// pr_split_06b_verify_shell.js — Interactive shell in verify worktree via CaptureSession.
// Dependencies: chunk 06 (verification) must be loaded first.
// Uses: termmux.newCaptureSession (higher-level CaptureSession, NOT raw PTY).

(function(prSplit) {

    if (typeof ctx === 'undefined' || typeof log === 'undefined') { return; }

    // Detect whether an interactive shell can be spawned on this platform.
    // Returns false on Windows (no $SHELL, $COMSPEC present) or when
    // termmux.newCaptureSession is unavailable.
    function canSpawnInteractiveShell() {
        try {
            var termmux = require('osm:termmux');
            if (typeof termmux.newCaptureSession !== 'function') return false;
        } catch (e) { return false; }

        try {
            var osmod = require('osm:os');
            if (osmod && typeof osmod.getenv === 'function') {
                // Windows: COMSPEC is set, SHELL is absent.
                var comspec = osmod.getenv('COMSPEC') || '';
                var shell = osmod.getenv('SHELL') || '';
                if (comspec && !shell) return false;
            }
        } catch (e) { /* best-effort — assume capable */ }

        return true;
    }

    // Spawn an interactive shell in the given worktree directory using
    // termmux.newCaptureSession. Returns the CaptureSession object, or
    // throws if the session cannot be created.
    //
    // opts: { rows, cols, env }
    //   rows/cols default to 24/120.
    //   env is an optional object of additional environment variables.
    function spawnShellSession(worktreeDir, opts) {
        // T338: Call through prSplit.canSpawnInteractiveShell so tests can
        // override the platform check without dealing with closure binding.
        var checkFn = (typeof prSplit.canSpawnInteractiveShell === 'function')
            ? prSplit.canSpawnInteractiveShell
            : canSpawnInteractiveShell;
        if (!checkFn()) {
            throw new Error('Interactive shell requires Unix (Linux/macOS). ' +
                'CaptureSession-based PTY is not available on this platform.');
        }

        opts = opts || {};
        var termmux = require('osm:termmux');
        var rows = opts.rows || 24;
        var cols = opts.cols || 120;

        // Determine the user's preferred shell via osm:os module.
        var shell = '';
        try {
            var osmod = require('osm:os');
            if (osmod && typeof osmod.getenv === 'function') {
                shell = osmod.getenv('SHELL') || '';
            }
        } catch (e) { /* ignore */ }
        if (!shell) {
            shell = 'sh';
        }

        var sessionOpts = {
            dir: worktreeDir,
            rows: rows,
            cols: cols
        };
        if (opts.env) {
            sessionOpts.env = opts.env;
        }

        var session = termmux.newCaptureSession(shell, ['-i'], sessionOpts);
        session.start();
        return session;
    }

    prSplit.canSpawnInteractiveShell = canSpawnInteractiveShell;
    prSplit.spawnShellSession = spawnShellSession;

})(globalThis.prSplit);
