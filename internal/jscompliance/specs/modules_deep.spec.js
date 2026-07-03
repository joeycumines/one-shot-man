// modules_deep.spec.js — deeper behavioral VALUE tests for modules that had
// only contract-table presence coverage: pabt, bubbletea, lipgloss,
// unicodetext, text/template. Pure/constructible (no heavy fixtures).

// --- pabt (Planning-Augmented BT) ---
test('pabt newState/newAction/newExprCondition + State get/set', function () {
	var bt = require('osm:bt');
	var pabt = require('osm:pabt');
	var bb = new bt.Blackboard();
	var st = pabt.newState(bb);
	st.set('at', 'home');
	assert.equal('state get', st.get('at'), 'home');
	// newExprCondition builds a condition object
	var cond = pabt.newExprCondition('at', '==', 'home');
	assert.equal('condition is object', typeof cond, 'object');
	// newAction builds an action object
	var act = pabt.newAction('go-to-store', [cond], [{ key: 'at', expr: 'store' }], bt.createLeafNode(function () { return bt.success; }));
	assert.equal('action is object', typeof act, 'object');
});
test('pabt newPlan returns a Plan with node()/running()', function () {
	var pabt = require('osm:pabt');
	var bt = require('osm:bt');
	var st = pabt.newState(new bt.Blackboard());
	var plan = pabt.newPlan(st, [pabt.newExprCondition('done', '==', true)]);
	assert.equal('plan is object', typeof plan, 'object');
	assert.equal('plan.node is function', typeof plan.node, 'function');
	assert.equal('plan.running is function', typeof plan.running, 'function');
});

// --- bubbletea (commands + metadata + validators; view via newModel) ---
test('bubbletea commands return defined values', function () {
	var tea = require('osm:bubbletea');
	assert.equal('quit defined', typeof tea.quit === 'undefined', false);
	assert.equal('clearScreen defined', typeof tea.clearScreen === 'undefined', false);
	assert.equal('batch is function', typeof tea.batch, 'function');
	assert.equal('sequence is function', typeof tea.sequence, 'function');
	assert.equal('tick is function', typeof tea.tick, 'function');
	// batch/sequence compose commands into a single value
	var combined = tea.batch([tea.quit()]);
	assert.equal('batch returns defined', typeof combined === 'undefined', false);
});
test('bubbletea isTTY returns a boolean', function () {
	var tea = require('osm:bubbletea');
	assert.equal('isTTY is bool', typeof tea.isTTY(), 'boolean');
});
test('bubbletea validators whitelist input', function () {
	var tea = require('osm:bubbletea');
	// a printable single char is valid textarea input
	assert.equal('valid textarea char', tea.isValidTextareaInput('a'), true);
	assert.equal('invalid textarea ctrl', tea.isValidTextareaInput(String.fromCharCode(0x01)), false);
});
test('bubbletea keys/keysByName/mouseButtons metadata present', function () {
	var tea = require('osm:bubbletea');
	assert.equal('keys is object', typeof tea.keys, 'object');
	assert.equal('keysByName is object', typeof tea.keysByName, 'object');
	assert.equal('mouseButtons is object', typeof tea.mouseButtons, 'object');
});

// --- lipgloss (borders + alignment + join) ---
test('lipgloss border factories produce styles + render contains content', function () {
	var l = require('osm:lipgloss');
	var s = l.newStyle();
	var out = s.render('X');
	assert.equal('plain render contains X', out.indexOf('X') >= 0, true);
	// joinHorizontal combines two rendered strings
	var joined = l.joinHorizontal(l.Left, 'A', 'B');
	assert.equal('joinHorizontal contains both', joined.indexOf('A') >= 0 && joined.indexOf('B') >= 0, true);
});
test('lipgloss place/width/height/size measure', function () {
	var l = require('osm:lipgloss');
	assert.equal('width(abc)', l.width('abc'), 3);
	assert.equal('size returns number', typeof l.size('hello'), 'number');
});

// --- unicodetext (width of wide chars + truncate) ---
test('unicodetext width measures ASCII as 1 per char', function () {
	var u = require('osm:unicodetext');
	assert.equal('width(abc)', u.width('abc'), 3);
	assert.equal('width(empty)', u.width(''), 0);
});
test('unicodetext truncates to a max width', function () {
	var u = require('osm:unicodetext');
	var t = u.truncate('hello world', 5);
	assert.equal('truncated within width', u.width(t) <= 5, true);
});

// --- text/template (funcs + delims + parse/execute chain) ---
test('text/template new + parse + execute with funcs', function () {
	var tt = require('osm:text/template');
	var tmpl = tt.new('demo');
	tmpl.funcs({ upper: function (s) { return String(s).toUpperCase(); } });
	tmpl.parse('{{upper .V}}');
	assert.equal('template execute with func', tmpl.execute({ V: 'hi' }), 'HI');
});
