
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
        lastGen: 0
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
        if (live.pane && live.cid === cid) return true;
        destroyAgentTermpane();
        if (!s) {
            s = prSplit._state || null;
            // The pipeline may attach the provider before the TUI has
            // delivered its first WindowSize message. Use the manager's
            // controlling-terminal dimensions rather than the model's
            // 80x24 defaults for that first resize.
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
        syncAgentTermpaneBounds(s);
        log.info('agent live pane attached', { sessionId: cid, width: box.width, height: box.height });
        return true;
    }

    function destroyAgentTermpane() {
        if (live.pane) {
            try { live.pane.close(); } catch (e) {
                log.debug('agent live pane close failed', { error: e.message || String(e) });
            }
        }
        live.pane = null;
        live.cid = 0;
        live.cursor = null;
        live.lastGen = 0;
    }

    // Resize authority: called only from handleWindowResize plus attach paths.
    // Never called from render. Applies the handleWindowResize inner-size
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
            live.pane.update({ type: 'WindowSize', width: box.width, height: box.height });
        } catch (e) {
            log.debug('agent live resize update failed', { error: e.message || String(e) });
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
    // expected. Returns null always. Caller keeps its own tick.
    function routeKeyToAgentTermpane(msg) {
        if (!liveActive()) return null;
        if (!liveSessionAlive()) {
            destroyAgentTermpane();
            return null;
        }
        try {
            live.pane.update(msg);
        } catch (e) {
            log.debug('agent live key route failed', { error: e.message || String(e) });
            return null;
        }
        return null;
    }

    function routeMouseToAgentTermpane(msg) {
        if (!liveActive()) return false;
        if (!liveSessionAlive()) {
            destroyAgentTermpane();
            return false;
        }
        try {
            live.pane.update(msg);
        } catch (e) {
            log.debug('agent live mouse route failed', { error: e.message || String(e) });
            return false;
        }
        return true;
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
        destroyAgentTermpane();
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
