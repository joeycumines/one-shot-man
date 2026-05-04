'use strict';
// pr_split_06b_verify_shell.js — Interactive shell in verify worktree via CaptureSession.
// Dependencies: chunk 06 (verification) must be loaded first.
// Uses: termmux.newCaptureSession (higher-level CaptureSession, NOT raw PTY).

(function(prSplit) {

    if (typeof ctx === 'undefined' || typeof log === 'undefined') { return; }

    // Detect whether an interactive shell can be spawned on this platform.
    // Returns false when termmux.newCaptureSession is unavailable.
    // On Windows, CaptureSession uses ConPTY; on Unix, it uses pty.
    //
    // Also returns false when OSM_VERIFY_ONE_SHOT=1 is set, which forces
    // one-shot verification mode (used by automated E2E tests where no human
    // is available to press p/f/c in the persistent shell).
    function canSpawnInteractiveShell() {
        try {
            var termmux = require('osm:termmux');
            if (typeof termmux.newCaptureSession !== 'function') return false;
        } catch (e) { log.debug('canSpawnInteractiveShell: require termmux failed: ' + (e.message || e)); return false; }

        try {
            var osmod = require('osm:os');
            if (osmod && typeof osmod.getenv === 'function') {
                // E2E test escape hatch: force one-shot verify mode.
                var oneShot = osmod.getenv('OSM_VERIFY_ONE_SHOT') || '';
                if (oneShot === '1' || oneShot === 'true') return false;
            }
        } catch (e) { log.debug('canSpawnInteractiveShell: os.getenv check failed: ' + (e.message || e)); }

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
            throw new Error('Interactive shell is not available: ' +
                'termmux.newCaptureSession unavailable on this platform.');
        }

        opts = opts || {};
        var termmux = require('osm:termmux');
        var rows = opts.rows || 24;
        var cols = opts.cols || 120;

        // Determine the user's preferred shell and arguments.
        var shell = '';
        var shellArgs = [];
        try {
            var osmod = require('osm:os');
            if (osmod && typeof osmod.getenv === 'function') {
                var isWin = osmod.platform && osmod.platform() === 'windows';
                if (isWin) {
                    // Windows: prefer COMSPEC (cmd.exe) or powershell.exe.
                    shell = osmod.getenv('COMSPEC') || 'cmd.exe';
                    // No '-i' flag for cmd.exe.
                } else {
                    shell = osmod.getenv('SHELL') || '';
                }
            }
        } catch (e) { log.debug('spawnShell: getenv failed: ' + (e.message || e)); }
        if (!shell) {
            shell = 'sh';
            shellArgs = ['-i'];
        } else if (shell.indexOf('cmd') === -1 && shell.indexOf('powershell') === -1 && shell.indexOf('pwsh') === -1) {
            // Unix shells: pass -i for interactive mode.
            shellArgs = ['-i'];
        }

        var sessionOpts = {
            dir: worktreeDir,
            rows: rows,
            cols: cols
        };
        if (opts.env) {
            sessionOpts.env = opts.env;
        }

        var session = termmux.newCaptureSession(shell, shellArgs, sessionOpts);
        session.start();
        return session;
    }

    prSplit.canSpawnInteractiveShell = canSpawnInteractiveShell;
    prSplit.spawnShellSession = spawnShellSession;

})(globalThis.prSplit);
