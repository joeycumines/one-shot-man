// modules_framework.spec.js — behavioral checks for the TUI/framework modules
// whose core surface is constructible without heavy fixtures. Deep lifecycle
// (real TUI run, gRPC services, MCP IPC, PTY) lives in the dedicated packages'
// own tests; here we pin the documented VALUE behavior of the primitives.

// --- bt (behavior tree primitives) ---
test('bt status constants are the documented strings', function () {
	var bt = require('osm:bt');
	assert.equal('success', bt.success, 'success');
	assert.equal('failure', bt.failure, 'failure');
	assert.equal('running', bt.running, 'running');
});
test('bt.createLeafNode + tick returns running then the leaf status (async)', async function () {
	var bt = require('osm:bt');
	var n = bt.createLeafNode(function () { return bt.success; });
	// JSLeafAdapter is an async state machine (adapter.go:38-40): the first tick
	// dispatches to JS and returns Running; the leaf's JS function settles on the
	// event loop. A subsequent tick returns the final status.
	assert.equal('tick returns running (dispatch)', bt.tick(n), bt.running);
	// Wait for the leaf's JS callback to settle, then tick again.
	await new Promise(function (resolve) { setTimeout(resolve, 50); });
	assert.equal('tick returns success (settled)', bt.tick(n), bt.success);
});
test('bt.Blackboard get/set/has/delete', function () {
	var bt = require('osm:bt');
	var bb = new bt.Blackboard();
	bb.set('k', 42);
	assert.equal('blackboard get', bb.get('k'), 42);
	assert.equal('blackboard has', bb.has('k'), true);
	bb.delete('k');
	assert.equal('blackboard has after delete', bb.has('k'), false);
});

// --- lipgloss (style chain + render) ---
test('lipgloss newStyle is chainable and render produces a string', function () {
	var l = require('osm:lipgloss');
	var s = l.newStyle().bold().width(10);
	var out = s.render('hi');
	assert.equal('render is string', typeof out, 'string');
	assert.equal('render contains content', out.indexOf('hi') >= 0, true);
});
test('lipgloss width measures display width', function () {
	var l = require('osm:lipgloss');
	assert.equal('width(abc)', l.width('abc'), 3);
});

// --- bubblezone (mark/scan/inBounds) ---
test('bubblezone mark wraps content with a zone id', function () {
	var bz = require('osm:bubblezone');
	var marked = bz.mark('z1', 'hello');
	assert.equal('mark is string', typeof marked, 'string');
	assert.equal('mark contains content', marked.indexOf('hello') >= 0, true);
});

// --- bubbles viewport + textarea (construct + core methods) ---
test('bubbles/viewport constructs and sets content', function () {
	var v = require('osm:bubbles/viewport').new();
	v.setContent('line');
	assert.equal('viewport typeof', typeof v, 'object');
	assert.equal('viewport totalLineCount', v.totalLineCount() >= 1, true);
});
test('bubbles/textarea constructs, sets + reads value', function () {
	var ta = require('osm:bubbles/textarea').new();
	ta.setValue('hello');
	assert.equal('textarea value', ta.value(), 'hello');
});

// --- termui components: construction is covered by the contract table
// (TestModuleContract); per-component view() internals are covered by each
// termui/* package's own tests, and the public method names drift from
// scripting.md (DRIFT-10), so they are not re-asserted here.

// --- aimux parser + event constants (no real agent) ---
test('aimux exposes all EVENT_* constants', function () {
	var aimux = require('osm:aimux');
	var consts = ['EVENT_TEXT', 'EVENT_RATE_LIMIT', 'EVENT_PERMISSION', 'EVENT_MODEL_SELECT', 'EVENT_SSO_LOGIN', 'EVENT_COMPLETION', 'EVENT_TOOL_USE', 'EVENT_ERROR', 'EVENT_THINKING'];
	for (var i = 0; i < consts.length; i++) {
		assert.equal('aimux ' + consts[i] + ' defined', typeof aimux[consts[i]] === 'undefined', false);
	}
});
test('aimux newParser parse classifies a line', function () {
	var aimux = require('osm:aimux');
	var p = aimux.newParser();
	var ev = p.parse('an ordinary line of text');
	// parse returns an event with a type; an ordinary line is EVENT_TEXT-ish.
	assert.equal('parse returns object', typeof ev, 'object');
	assert.equal('parse has type', typeof ev.type !== 'undefined', true);
});
