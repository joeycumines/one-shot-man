// pr_split_16g_persistence.js — Session persistence across pr-split restarts
//
// T63: Wires EventBus-driven auto-save and startup resume for the terminal
// multiplexer. The Go layer (tuiMux.exportState/saveState/loadState) handles
// the heavy lifting; this chunk adds the pr-split-specific orchestration:
//
//   1. Auto-save state to disk on session lifecycle transitions
//   2. On startup, detect a previous state file and report findings
//   3. Check PID liveness for previously running sessions
//   4. Clean up the state file on normal exit
//
// Dependencies: pr_split_16f_tui_model.js (TUI model), tuiMux global,
// prSplitConfig global (persistStatePath).

(function () {
    'use strict';

    // Guard: tuiMux must be available.
    if (typeof tuiMux === 'undefined' || !tuiMux) {
        return;
    }

    var statePath = '';
    if (typeof prSplitConfig !== 'undefined' && prSplitConfig && prSplitConfig.persistStatePath) {
        statePath = prSplitConfig.persistStatePath;
    }
    if (!statePath) {
        log.debug('persistence: no statePath configured, skipping auto-save');
        return;
    }

    // ── Auto-save on EventBus transitions ───────────────────────────
    //
    // Subscribe to mux events and persist state on lifecycle transitions.
    // Output events are intentionally excluded to avoid excessive disk I/O.

    var SAVE_EVENTS = ['registered', 'activated', 'exit', 'closed'];

    /**
     * _persistState writes the current SessionManager state to disk.
     * Errors are logged but never propagated — persistence is best-effort
     * to avoid disrupting the TUI.
     */
    function _persistState(reason) {
        try {
            tuiMux.saveState(statePath);
            log.debug('persistence: saved state', { reason: reason, path: statePath });
        } catch (e) {
            log.warn('persistence: save failed', { reason: reason, error: e.message || String(e) });
        }
    }

    // Wire event listeners. The muxEvents API (from WrapSessionManager)
    // delivers events synchronously on the Goja goroutine when drained.
    if (typeof tuiMux.on === 'function') {
        for (var i = 0; i < SAVE_EVENTS.length; i++) {
            (function (eventName) {
                tuiMux.on(eventName, function () {
                    _persistState(eventName);
                });
            })(SAVE_EVENTS[i]);
        }
        log.debug('persistence: event listeners registered', { events: SAVE_EVENTS });
    }

    // ── Startup resume detection ────────────────────────────────────
    //
    // On load, check for a previous state file and annotate each session
    // with PID liveness. The result is stored on prSplit.previousState
    // for the TUI to inspect and present resume options.

    /**
     * prSplit.loadPreviousState() → object | null
     *
     * Returns the previous persisted state with a `sessions` array where
     * each entry has an extra `alive` boolean indicating PID liveness.
     * Returns null if no state file exists.
     */
    function loadPreviousState() {
        try {
            var state = tuiMux.loadState(statePath);
            if (!state) {
                return null;
            }
            // Annotate each session with PID liveness.
            var sessions = state.sessions || [];
            for (var j = 0; j < sessions.length; j++) {
                var s = sessions[j];
                s.alive = (s.pid > 0) ? tuiMux.processAlive(s.pid) : false;
            }
            log.info('persistence: previous state loaded', {
                sessionCount: sessions.length,
                path: statePath
            });
            return state;
        } catch (e) {
            log.warn('persistence: load failed', { error: e.message || String(e), path: statePath });
            return null;
        }
    }

    // ── Clean exit handler ──────────────────────────────────────────
    //
    // Remove the state file on clean exit so the next startup doesn't
    // offer stale resume data. This is registered as a cleanup callback
    // on the prSplit namespace.

    function cleanupStateFile() {
        try {
            tuiMux.removeState(statePath);
            log.debug('persistence: state file removed on clean exit', { path: statePath });
        } catch (e) {
            log.debug('persistence: remove failed on exit', { error: e.message || String(e) });
        }
    }

    // ── Export on prSplit namespace ──────────────────────────────────
    if (typeof prSplit === 'undefined') {
        globalThis.prSplit = {};
    }

    prSplit.persistence = {
        /** Save current state to disk. */
        save: function () { _persistState('manual'); },

        /** Load and annotate previous state. Returns object or null. */
        loadPrevious: loadPreviousState,

        /** Remove the state file (used on clean exit). */
        cleanup: cleanupStateFile,

        /** The resolved state file path. */
        statePath: statePath
    };

    // Auto-load previous state on chunk initialization so the TUI can
    // check prSplit.previousState during model setup.
    prSplit.previousState = loadPreviousState();

})();
