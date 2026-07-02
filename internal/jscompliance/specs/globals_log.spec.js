// globals_log.spec.js — the `log` global (structured logging API surface).
// Detailed entry semantics are covered in internal/scripting; here we pin the
// CONTRACT SURFACE and one round-trip. NOTE: goja exports the logEntry struct
// with CAPITALIZED Go field names (Message/Level/Attrs); slog.Level exports as
// its integer value (INFO=0, WARN=4, ERROR=8), NOT the string — pinned here.

test('log exposes the documented method surface', function () {
	assert.equal('log.debug', typeof log.debug, 'function');
	assert.equal('log.info', typeof log.info, 'function');
	assert.equal('log.warn', typeof log.warn, 'function');
	assert.equal('log.error', typeof log.error, 'function');
	assert.equal('log.printf', typeof log.printf, 'function');
	assert.equal('log.getLogs', typeof log.getLogs, 'function');
	assert.equal('log.searchLogs', typeof log.searchLogs, 'function');
	assert.equal('log.clearLogs', typeof log.clearLogs, 'function');
});

test('log.info records an entry retrievable via getLogs', function () {
	log.clearLogs();
	log.info('hello');
	var last = log.getLogs().pop();
	assert.equal('entry Message', last.Message, 'hello');
	assert.equal('entry Level (INFO=0)', last.Level, 0);
});

test('log entries carry stringified structured attributes (Attrs)', function () {
	log.clearLogs();
	log.info('with-attrs', { userId: 42 });
	var last = log.getLogs().pop();
	// Attrs is map[string]string; numeric values are stringified.
	assert.equal('attr userId', last.Attrs.userId, '42');
});

test('log.clearLogs empties the buffer', function () {
	log.info('x');
	log.clearLogs();
	assert.equal('cleared', log.getLogs().length, 0);
});
