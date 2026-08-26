#!/usr/bin/env osm script

// example-08-aimux-session.js -- Interactive PTY TUI with aimux orchestration.
//
// Demonstrates a real-time interactive terminal UI over an osm:aimux agent:
//   1. Spawns a deterministic shell via async aimux APIs.
//   2. Renders a scrollable output pane fed by the PTY stream.
//   3. Shows a live status bar with aimux TUIStateMachine state + parsed events.
//   4. Provides a text input to send keystrokes and a help bar for controls.
//   5. Exits cleanly with async cleanup (close Promise + BubbleTea quit).
//
// Keys:
//   q / ctrl+c   Quit
//   enter        Send text input to the agent
//   ctrl+l       Clear output pane
//   ctrl+d       Send EOF
//
// Run: osm script scripts/example-08-aimux-session.js
// Smoke: osm script scripts/example-08-aimux-session.js --smoke

(function main() {
    var aimux = require('osm:aimux');
    var tea = require('osm:bubbletea');
    var lipgloss = require('osm:lipgloss');
    var viewportLib = require('osm:bubbles/viewport');
    var textareaLib = require('osm:bubbles/textarea');
    var flag = require('osm:flag');

    var POLL_MS = 50;

    // -- Flag Parsing ----------------------------------------------------------------

    var fs = flag.newFlagSet('aimux-demo');
    fs.bool('smoke', false, 'run a non-interactive smoke test and exit');
    var parseResult = fs.parse(args);
    if (parseResult.error !== null) {
        output.print('flag parse failed: ' + parseResult.error);
        return;
    }
    var SMOKE = fs.get('smoke');

    // -- Styles --------------------------------------------------------------------

    var s = {
        title: lipgloss.newStyle().foreground('#7D56F4').bold(true),
        statusLabel: lipgloss.newStyle().foreground('#E0AF68').bold(true),
        eventText: lipgloss.newStyle().foreground('#565F89'),
        eventThinking: lipgloss.newStyle().foreground('#FFA500'),
        eventError: lipgloss.newStyle().foreground('#FF5370'),
        eventReady: lipgloss.newStyle().foreground('#9ECE6A'),
        paneBorder: lipgloss.newStyle().foreground('#3B4261'),
        inputLabel: lipgloss.newStyle().foreground('#C0CAF5').bold(true),
        help: lipgloss.newStyle().foreground('#565F89'),
    };

    function stateColor(stateName) {
        switch (stateName) {
            case 'Initializing': return lipgloss.newStyle().foreground('#C0CAF5');
            case 'Ready': return lipgloss.newStyle().foreground('#9ECE6A');
            case 'Processing': return lipgloss.newStyle().foreground('#FFA500');
            case 'Responding': return lipgloss.newStyle().foreground('#7AA2F7');
            case 'Error': return lipgloss.newStyle().foreground('#FF5370');
            case 'RateLimited': return lipgloss.newStyle().foreground('#FF9E64');
            case 'PermissionPrompt': return lipgloss.newStyle().foreground('#E0AF68');
            default: return lipgloss.newStyle().foreground('#C0CAF5');
        }
    }

    function colorForEvent(name) {
        switch (name) {
            case 'THINKING': return s.eventThinking;
            case 'ERROR': return s.eventError;
            case 'COMPLETION': return s.eventReady;
            case 'READY': return s.eventReady;
            default: return s.eventText;
        }
    }

    // -- Helpers -------------------------------------------------------------------

    function appendOutput(model, text) {
        if (!text) return;
        var lines = String(text).split(/\r?\n/);
        for (var i = 0; i < lines.length; i++) {
            if (lines[i] !== '' || model.outputLines.length === 0 || model.outputLines[model.outputLines.length - 1] !== '') {
                model.outputLines.push(lines[i]);
            }
        }
        model.viewport.setContent(model.outputLines.join('\n'));
        if (model.viewport.atBottom()) {
            model.viewport.gotoBottom();
        }
    }

    function applyWindowSize(model, width, height) {
        model.width = width;
        model.height = height;

        var headerH = 2;
        var statusH = 2;
        var inputH = 3;
        var helpH = 1;
        var vpHeight = Math.max(5, height - headerH - statusH - inputH - helpH);

        model.viewport.setWidth(Math.max(10, width - 2));
        model.viewport.setHeight(vpHeight);
        model.textarea.setWidth(Math.max(10, width - 6));

        if (model.outputLines.length > 0) {
            model.viewport.setContent(model.outputLines.join('\n'));
        }
    }

    // -- Model ---------------------------------------------------------------------

    function initModel() {
        var vp = viewportLib.new(78, 16);

        var ta = textareaLib.new();
        ta.setPlaceholder('Type and press Enter to send...');
        ta.focus();
        ta.setHeight(1);
        ta.setShowLineNumbers(false);
        ta.setWidth(72);

        return {
            width: 80,
            height: 24,
            viewport: vp,
            textarea: ta,
            outputLines: [],
            stateName: 'Initializing',
            lastEventName: '',
            alive: false,
            quitting: false,
            handle: null,
            sm: null,
            parser: null,
            pendingChunks: [],
            receiving: false,
            pollTimer: POLL_MS,
        };
    }

    // -- Agent lifecycle (synchronous setup, async for I/O) ------------------------

    function startAgent(model) {
        var registry = aimux.newRegistry();
        var provider = aimux.processProvider({
            name: 'demo-agent',
            command: 'sh',
            defaultArgs: ['-c', 'echo "ready"; echo "thinking..."; echo "done"; cat'],
            capabilities: { streaming: true, multiTurn: true, resizable: true }
        });
        registry.register(provider);

        registry.spawn('demo-agent', { rows: 24, cols: 80 }).then(function(handle) {
            model.handle = handle;
            model.alive = handle.isAlive();
        });

        // Parser
        var parser = aimux.newParser();
        parser.addPattern('ready', '^(?i)\\s*(?:ready|done)\\s*$', aimux.EVENT_COMPLETION);
        parser.addPattern('thinking', '^(?i)\\s*thinking\\b', aimux.EVENT_THINKING);
        parser.addPattern('error', '^(?i)error\\b', aimux.EVENT_ERROR);
        model.parser = parser;

        // State machine
        var cfg = aimux.defaultTUIStateConfig();
        cfg.readyPatterns.push('(?i)^\\s*(?:ready|done)\\s*$');
        cfg.processingPatterns.push('(?i)^\\s*thinking\\b');
        var sm = aimux.newTUIStateMachine(cfg);
        model.sm = sm;
        model.stateName = sm.stateName();

        // Kick off async ready wait. When it resolves, store output in pendingChunks.
        handle.waitReadyAsync(5000).then(
            function() {
                model.stateName = sm.stateName() || 'Ready';
                model.alive = handle.isAlive();
                handle.drainOutputAsync().then(
                    function(chunk) {
                        if (chunk && chunk !== null) {
                            model.pendingChunks.push(String(chunk));
                        }
                        model.alive = handle.isAlive();
                    },
                    function() { model.alive = handle.isAlive(); }
                );
            },
            function(err) {
                model.stateName = 'Error';
                log.error('agent never became ready: ' + (err && err.message ? err.message : String(err)));
            }
        );
    }

    function triggerReceive(model) {
        if (!model.handle || !model.handle.isAlive() || model.receiving) return;
        model.receiving = true;
        model.handle.receiveAsync().then(
            function(chunk) {
                model.receiving = false;
                if (chunk && chunk !== null) {
                    model.pendingChunks.push(String(chunk));
                }
                model.alive = model.handle ? model.handle.isAlive() : false;
            },
            function() {
                model.receiving = false;
                model.alive = model.handle ? model.handle.isAlive() : false;
            }
        );
    }

    function closeAgentAsync(model) {
        model.quitting = true;
        if (model.handle) {
            model.handle.close().then(function() {}, function() {});
            model.handle = null;
        }
        model.alive = false;
    }

    // -- Update --------------------------------------------------------------------

    function updateModel(msg, model) {
        if (msg.type === 'WindowSize') {
            applyWindowSize(model, msg.width, msg.height);
            return [model, null];
        }

        if (msg.type === 'Tick' && msg.id === 'poll') {
            if (model.pendingChunks.length > 0) {
                for (var i = 0; i < model.pendingChunks.length; i++) {
                    var chunk = model.pendingChunks[i];
                    appendOutput(model, chunk);
                    if (model.sm && model.parser) {
                        var lines = String(chunk).split(/\r?\n/);
                        for (var j = 0; j < lines.length; j++) {
                            var ev = model.parser.parse(lines[j]);
                            if (ev) {
                                model.lastEventName = aimux.eventTypeName(ev.type);
                            }
                            var update = model.sm.processOutput(lines[j]);
                            if (update && update.changed) {
                                model.stateName = update.stateName;
                            }
                        }
                    }
                }
                model.pendingChunks = [];
            }
            if (!model.quitting) {
                triggerReceive(model);
            }
            return [model, tea.tick(model.pollTimer || POLL_MS, 'poll')];
        }

        if (msg.type === 'Key') {
            switch (msg.key) {
                case 'ctrl+c':
                case 'q':
                    closeAgentAsync(model);
                    return [model, tea.quit()];
                case 'ctrl+l':
                    model.outputLines = [];
                    model.viewport.setContent('');
                    return [model, tea.clearScreen()];
                case 'ctrl+d':
                    if (model.handle && model.handle.isAlive()) {
                        model.handle.send('\x04');
                    }
                    return [model, null];
                case 'enter':
                    var val = model.textarea.value();
                    if (val) {
                        if (model.handle && model.handle.isAlive()) {
                            model.handle.send(val + '\n');
                        }
                        appendOutput(model, '> ' + val);
                        model.textarea.setValue('');
                    }
                    return [model, null];
            }

            var newTa = model.textarea.update(msg);
            if (newTa && newTa.length >= 1) {
                model.textarea = newTa[0];
            }
            return [model, null];
        }

        return [model, null];
    }

    // -- View ----------------------------------------------------------------------

    function buildStatusBar(model) {
        var stateStyle = stateColor(model.stateName);
        var left = s.statusLabel.render(' State: ') + stateStyle.render(model.stateName || 'Unknown');

        var eventStr = '';
        if (model.lastEventName) {
            eventStr = colorForEvent(model.lastEventName).render('Last: ' + model.lastEventName);
        }

        var aliveStr = model.alive ?
            lipgloss.newStyle().foreground('#9ECE6A').render('Alive') :
            lipgloss.newStyle().foreground('#565F89').render('Dead');

        var right = lipgloss.joinHorizontal(lipgloss.Top, aliveStr, '  ', eventStr);
        var fillW = Math.max(0, model.width - lipgloss.width(left) - lipgloss.width(right));
        if (fillW > 0) {
            right = ' '.repeat(fillW) + right;
        }
        return left + right;
    }

    function buildHelpBar(model) {
        return s.help.render('q/ctrl+c: Quit  |  enter: Send  |  ctrl+l: Clear  |  ctrl+d: EOF');
    }

    function viewModel(model) {
        var title = s.title.render('AIMux Interactive Session');
        var statusBar = buildStatusBar(model);
        var vpView = model.viewport.view();
        var inputLabel = s.inputLabel.render('> ');
        var inputView = inputLabel + model.textarea.view();
        var helpBar = buildHelpBar(model);

        var content = lipgloss.joinVertical(lipgloss.Left,
            title,
            statusBar,
            vpView,
            inputView,
            helpBar
        );

        return {
            content: content,
            altScreen: true,
            mouseMode: 'cellMotion',
        };
    }

    // -- Smoke Test ----------------------------------------------------------------

    function runSmoke() {
        var model = initModel();
        startAgent(model);

        // Simulate a few ticks
        for (var i = 0; i < 5; i++) {
            var tickMsg = { type: 'Tick', id: 'poll', time: Date.now() };
            var result = updateModel(tickMsg, model);
            model = result[0];
        }

        // Verify the view renders
        var view = viewModel(model);
        if (!view || !view.content) {
            throw new Error('smoke: view returned empty');
        }
        var content = view.content;

        output.print('smoke: content length=' + content.length);
        output.print('smoke: has title=' + (content.indexOf('AIMux') >= 0));
        output.print('smoke: has state=' + (content.indexOf('State:') >= 0));
        output.print('smoke: has help=' + (content.indexOf('Quit') >= 0));
        output.print('smoke: altScreen=' + view.altScreen);
        output.print('smoke: stateName=' + model.stateName);
        output.print('smoke: alive=' + model.alive);

        if (content.length < 50) {
            throw new Error('smoke: rendered content too short');
        }
        if (content.indexOf('AIMux') < 0) {
            throw new Error('smoke: missing title');
        }

        closeAgentAsync(model);
        log.info('aimux smoke test passed');
    }

    // -- Entry point ---------------------------------------------------------------

    if (SMOKE) {
        runSmoke();
        return;
    }

    var model = initModel();
    startAgent(model);

    var program = tea.newModel({
        init: function () {
            return [model, tea.tick(POLL_MS, 'poll')];
        },
        update: updateModel,
        view: viewModel,
    });

    tea.run(program);

    log.info('aimux interactive demo started');
})();
