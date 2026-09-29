
'use strict';
// pr_split_17_agent_termpane.js - Live agent terminal surface.
// Clipped-pane embedding: one osm termpane, no compositor. The wizard keeps
// its string renderer and lipgloss chrome. Bounds derive from the shared
// split layout authority. Render path is pure.
// Requires Go-injected globals: tui, ctx, output, log, prSplitConfig, tuiMux.

(function(prSplit) {

    if (typeof tui === 'undefined' || typeof ctx === 'undefined' ||
        typeof output === 'undefined') { return; }

    var tp = null;
    try { tp = require('osm:termui/termpane'); } catch (e) { tp = null; }

    var lipgloss = prSplit._lipgloss || null;

    var live = {
        pane: null,
        cid: 0,
        x: 0,
        y: 0,
        cols: 80,
        rows: 24,
        cursor: null,
        lastGen: 0,
        pending: null
    };

    function liveAvailable() {
        return !!(tp && typeof tuiMux !== 'undefined' && tuiMux);
    }

    function liveActive() {
        return !!(live.pane && live.cid);
    }

    function liveSessionAlive() {
        if (!liveActive()) return false;
        try {
            if (typeof tuiMux.isDone === 'function' && tuiMux.isDone(live.cid)) return false;
        } catch (e) { return false; }
        return true;
    }

    function isThenable(value) {
        return !!value && typeof value.then === 'function';
    }

    function trackPanePromise(s, operation) {
        if (typeof prSplit._trackPaneOperation === 'function') {
            return prSplit._trackPaneOperation(s, operation);
        }
        if (isThenable(operation)) operation.then(function() {}, function() {});
        return operation;
    }

    function chromeEstimate() {
        if (prSplit._CHROME_ESTIMATE) return prSplit._CHROME_ESTIMATE;
        return 8;
    }

    function agentPaneInnerSize(s, w, agentH) {
        var width = w || (s && s.width) || 80;
        var height = agentH;
        if (typeof height !== 'number') {
            var h = (s && s.height) || 24;
            var vpH = Math.max(3, h - chromeEstimate());
            var minP = 3;
            var wH = Math.max(minP, Math.floor(vpH * ((s && s.splitViewRatio) || 0.6)));
            wH = Math.min(wH, vpH - minP - 1);
            var cH = vpH - wH - 1;
            height = Math.max(3, cH);
        }
        return {
            rows: Math.max(1, height - 3),
            cols: Math.max(20, width - 4)
        };
    }

    function agentPaneBox(s, agentH) {
        var w = (s && s.width) || 80;
        var inner = agentPaneInnerSize(s, w, agentH);
        var row = 0;
        var col = 1;
        try {
            if (typeof prSplit._computeSplitPaneContentOffset === 'function' && s) {
                var ofs = prSplit._computeSplitPaneContentOffset(s);
                if (ofs && typeof ofs.row === 'number') row = ofs.row;
                if (ofs && typeof ofs.col === 'number') col = ofs.col;
            }
        } catch (e) {
            log.debug('agent live offset read failed', { error: e.message || String(e) });
        }
        return { x: col, y: row, width: inner.cols, height: inner.rows };
    }

    function ensureAgentTermpane(cid, s) {
        if (!liveAvailable()) return false;
        if (!cid) return false;
        if (live.pane && live.cid === cid && !live.pending) return true;
        if (live.pending) {
            var queuedReplacement = live.pending.then(function() {
                return ensureAgentTermpane(cid, s);
            });
            return trackPanePromise(s, queuedReplacement);
        }
        var attachPane = function() {
            if (!s) {
                s = prSplit._state || null;
                if ((!s || !s.width || !s.height) &&
                    typeof tuiMux.termSize === 'function') {
                    try {
                        var size = tuiMux.termSize();
                        if (size && size.rows > 0 && size.cols > 0) {
                            s = {
                                width: size.cols,
                                height: size.rows,
                                splitViewRatio: 0.6
                            };
                        }
                    } catch (e) {
                        log.debug('agent live terminal size read failed', { error: e.message || String(e) });
                    }
                }
            }
            var box = agentPaneBox(s);
            try {
                live.pane = tp.termpane({
                    manager: tuiMux,
                    sessionId: cid,
                    bounds: { x: box.x, y: box.y, width: box.width, height: box.height }
                });
            } catch (e) {
                log.warn('agent live pane create failed', { sessionId: cid, error: e.message || String(e) });
                live.pane = null;
                return false;
            }
            live.cid = cid;
            live.x = box.x;
            live.y = box.y;
            live.cols = box.width;
            live.rows = box.height;
            var boundsUpdate = syncAgentTermpaneBounds(s);
            if (isThenable(boundsUpdate)) {
                return boundsUpdate.then(function() {
                    log.info('agent live pane attached', { sessionId: cid, width: box.width, height: box.height });
                    return true;
                }, function(e) {
                    log.debug('agent live replacement resize failed', { sessionId: cid, error: e.message || String(e) });
                    return true;
                });
            }
            log.info('agent live pane attached', { sessionId: cid, width: box.width, height: box.height });
            return true;
        };
        var closePromise = live.pane ? destroyAgentTermpane() : null;
        var replacement = isThenable(closePromise)
            ? closePromise.then(attachPane, function(e) {
                log.debug('agent live pane replacement close failed', { error: e.message || String(e) });
                return attachPane();
            })
            : attachPane();
        if (isThenable(replacement)) {
            live.pending = replacement;
            replacement.then(function() {
                if (live.pending === replacement) live.pending = null;
            }, function() {
                if (live.pending === replacement) live.pending = null;
            });
            trackPanePromise(s, replacement);
        }
        return replacement;
    }

    function destroyAgentTermpane(options) {
        options = options || {};
        if (live.pending) {
            return live.pending.then(function() {
                return destroyAgentTermpane();
            });
        }
        var pane = live.pane;
        live.pane = null;
        live.cid = 0;
        live.cursor = null;
        live.lastGen = 0;
        var waitForUpdates = !options.skipWait && prSplit._waitForPaneOperations && typeof prSplit._state === 'object' ?
            prSplit._waitForPaneOperations(prSplit._state) : Promise.resolve();
        var closeOperation = Promise.resolve(waitForUpdates).then(function() {
            if (!pane) return;
            return Promise.resolve(pane.close()).catch(function(e) {
                log.debug('agent live pane close failed', { error: e.message || String(e) });
            });
        }).catch(function(e) {
            log.debug('agent live pane close failed', { error: e.message || String(e) });
        });
        return trackPanePromise(prSplit._state, closeOperation);
    }

    // Resize authority: called only from handleWindowResize plus attach paths.
    // Never called from render. The returned Promise represents the tracked
    // termpane update. Applies the handleWindowResize inner-size
    // formula (paneRows = agentH - 3, paneCols = w - 4) so the PTY size
    // matches the size the screenshot path resizes interactive sessions to.
    function syncAgentTermpaneBounds(s) {
        if (!liveActive()) return;
        var box = agentPaneBox(s);
        live.x = box.x;
        live.y = box.y;
        live.cols = box.width;
        live.rows = box.height;
        try {
            live.pane.setBounds({ x: box.x, y: box.y, width: box.width, height: box.height });
        } catch (e) {
            log.debug('agent live setbounds failed', { error: e.message || String(e) });
        }
        try {
            var updateResult = live.pane.update({ type: 'WindowSize', width: box.width, height: box.height });
            return isThenable(updateResult) ? Promise.resolve(updateResult) : Promise.resolve();
        } catch (e) {
            log.debug('agent live resize update failed', { error: e.message || String(e) });
            return Promise.resolve();
        }
    }

    function clipAnsiToBox(content, cols, rows) {
        var maxCols = (typeof cols === 'number' && isFinite(cols)) ? Math.max(0, Math.floor(cols)) : 0;
        var maxRows = (typeof rows === 'number' && isFinite(rows)) ? Math.max(0, Math.floor(rows)) : 0;
        var lines = String(content || '').split('\n');
        var out = [];
        // Read lipgloss at call time so chunk load order shifts cannot pin
        // a stale null; fall back to the load-time capture.
        var lg = prSplit._lipgloss || lipgloss;
        for (var i = 0; i < maxRows; i++) {
            var ln = lines[i] || '';
            if (lg) {
                try {
                    if (lg.width(ln) > maxCols) {
                        ln = lg.newStyle().maxWidth(maxCols).render(ln);
                    }
                } catch (e) {
                    log.debug('agent live clip failed', { error: e.message || String(e) });
                }
            }
            out.push(ln);
        }
        return out.join('\n');
    }

    // Pure read: pane.view() plus clip. No setBounds, no WindowSize, no
    // compositor resize. Safe to call from the view path.
    function refreshAgentLiveView() {
        if (!liveActive()) return { content: '', gen: 0, cursor: null };
        var v = null;
        try {
            v = live.pane.view();
        } catch (e) {
            log.debug('agent live view failed', { error: e.message || String(e) });
            return { content: '', gen: live.lastGen, cursor: live.cursor };
        }
        var content = (v && v.content) || '';
        var gen = (v && typeof v.gen === 'number') ? v.gen : live.lastGen;
        live.cursor = (v && v.cursor) || null;
        live.lastGen = gen;
        return { content: content, gen: gen, cursor: live.cursor };
    }

    // Pure render: refresh plus clip to the pane box. Zero mutation.
    function renderAgentLivePane(s, width, height) {
        if (!liveActive()) return '';
        var v = refreshAgentLiveView();
        return clipAnsiToBox(v.content, live.cols, live.rows);
    }

    // Cursor passthrough with proof in Research Findings: pane cursor coords
    // already include bounds.Position, embedding origin equals bounds origin,
    // so no adjustment. Out-of-box cursors clip to null. Shape, blink, and
    // color pass through only when the binding populated them.
    function agentLiveCursor() {
        if (!liveActive() || !live.cursor) return null;
        var c = live.cursor;
        if (typeof c.x !== 'number' || typeof c.y !== 'number') return null;
        if (c.x < live.x || c.y < live.y) return null;
        if (c.x >= live.x + live.cols || c.y >= live.y + live.rows) return null;
        var out = { x: c.x, y: c.y };
        if (c.shape) out.shape = c.shape;
        if (typeof c.blink === 'boolean') out.blink = c.blink;
        if (c.color) out.color = c.color;
        return out;
    }

    // Key input yields Null cmd (Update KeyPress returns nil), so no batch is
    // expected. The returned Promise settles after the tracked update; callers
    // keep their own tick.
    function routeKeyToAgentTermpane(msg) {
        if (!liveActive()) return Promise.resolve(null);
        if (!liveSessionAlive()) {
            try {
                return Promise.resolve(destroyAgentTermpane()).then(function() {
                    return null;
                });
            } catch (e) {
                log.debug('agent live key route failed', { error: e.message || String(e) });
                return Promise.resolve(null);
            }
        }
        try {
            var keyUpdate = live.pane.update(msg);
            return Promise.resolve(keyUpdate).then(function() {
                return null;
            }, function() {
                return null;
            });
        } catch (e) {
            log.debug('agent live key route failed', { error: e.message || String(e) });
            return Promise.resolve(null);
        }
    }

    function routeMouseToAgentTermpane(msg) {
        // Synchronous decline paths must stay boolean so callers can fall
        // through to legacy child forwarding. Only an actual pane update
        // returns a Promise.
        if (!liveActive()) return false;
        if (!liveSessionAlive()) {
            try {
                trackPanePromise(prSplit._state, destroyAgentTermpane());
            } catch (e) {
                log.debug('agent live mouse route failed', { error: e.message || String(e) });
            }
            return false;
        }
        try {
            var mouseUpdate = live.pane.update(msg);
            return Promise.resolve(mouseUpdate).then(function() {
                return true;
            }, function() {
                return false;
            });
        } catch (e) {
            log.debug('agent live mouse route failed', { error: e.message || String(e) });
            return false;
        }
    }

    function isPointInAgentPane(x, y) {
        if (!liveActive()) return false;
        return x >= live.x && y >= live.y && x < live.x + live.cols && y < live.y + live.rows;
    }

    // note* return the pane outcome; liveness stays with _agentLiveActive /
    // _agentLiveSessionAlive and the pinned id with _state.agentSessionID,
    // so no per-state fields are written here.
    function noteAgentAttached(cid, s) {
        if (!cid) return false;
        return ensureAgentTermpane(cid, s);
    }

    function noteAgentDetached() {
        return destroyAgentTermpane();
    }

    prSplit._agentLiveAvailable = liveAvailable;
    prSplit._agentLiveActive = liveActive;
    prSplit._agentLiveSessionAlive = liveSessionAlive;
    prSplit._ensureAgentTermpane = ensureAgentTermpane;
    prSplit._destroyAgentTermpane = destroyAgentTermpane;
    prSplit._syncAgentTermpaneBounds = syncAgentTermpaneBounds;
    prSplit._refreshAgentLiveView = refreshAgentLiveView;
    prSplit._renderAgentLivePane = renderAgentLivePane;
    prSplit._agentLiveCursor = agentLiveCursor;
    prSplit._routeKeyToAgentTermpane = routeKeyToAgentTermpane;
    prSplit._routeMouseToAgentTermpane = routeMouseToAgentTermpane;
    prSplit._noteAgentAttached = noteAgentAttached;
    prSplit._noteAgentDetached = noteAgentDetached;
    prSplit._agentPaneBox = agentPaneBox;
    prSplit._agentPaneInnerSize = agentPaneInnerSize;
    prSplit._clipAnsiToBox = clipAnsiToBox;
    prSplit._isPointInAgentPane = isPointInAgentPane;
    prSplit._agentPaneHeight = function(s) {
        return agentPaneInnerSize(s).rows;
    };

    prSplit._agentEvidence = prSplit._agentEvidence || null;
    prSplit._classifyCheckpoint = prSplit._classifyCheckpoint || null;

    prSplit._stateRefresh = function() {
        try {
            if (typeof tui !== 'undefined' && tui && typeof tui.refresh === 'function') {
                tui.refresh();
            }
        } catch (e) {
            log.debug('state refresh failed', { error: e.message || String(e) });
        }
    };

})(globalThis.prSplit);
