// globals_state.spec.js — tui.createState contract surface.
// createState requires an initialized StateManager (command/TUI context);
// the detailed Symbol-key get/set behavior is covered in internal/scripting.
// Here we pin the CONTRACT: tui exposes createState and the related surface.

test('tui exposes createState and the state API surface', function () {
	assert.equal('createState typeof', typeof tui.createState, 'function');
});

test('createState validates its arguments (throws on bad input)', function () {
	// command name cannot contain ':' (reserved separator) — pin the guard.
	var threw = false;
	try { tui.createState('bad:name', {}); } catch (e) { threw = true; }
	assert.equal('createState rejects ":" in name', threw, true);
});
