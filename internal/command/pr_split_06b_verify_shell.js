'use strict';
// pr_split_06b_verify_shell.js — Interactive shell in verify worktree via CaptureSession.
// Dependencies: chunk 06 (verification) must be loaded first.
// Uses: termmux.newCaptureSession (higher-level CaptureSession, NOT raw PTY).

(function(prSplit) {

    if (typeof ctx === 'undefined' || typeof log === 'undefined') { return; }

    // Spawn an interactive shell in the given worktree directory using
    // termmux.newCaptureSession. Returns the CaptureSession object, or
    // throws if the session cannot be created.
    //
    // opts: { rows, cols, env }
    //   rows/cols default to 24/120.
    //   env is an optional object of additional environment variables.
    function spawnShellSession(worktreeDir, opts) {
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

    prSplit.spawnShellSession = spawnShellSession;

})(globalThis.prSplit);
