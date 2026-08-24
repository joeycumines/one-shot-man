// core_es_fork_blocked.spec.js — GOJA-FORK-BLOCKED ES2024+ features that are expected to FAIL in the main pipeline.
// These are separated from core_es.spec.js so that gmake test-jscompliance (main tier) can pass while
// the dedicated test-jscompliance-fork-blocked target shows the expected failures with clear markers.
// When the goja fork is updated, these tests will pass and can be promoted to the main spec.

test('Symbol.asyncIterator is a registered well-known symbol (GOJA-FORK-BLOCKED)', function () {
	assert.equal('Symbol.asyncIterator typeof', typeof Symbol.asyncIterator, 'symbol');
});

test('String.prototype.isWellFormed/toWellFormed (ES2024, GOJA-FORK-BLOCKED)', function () {
	assert.equal('isWellFormed is a function', typeof 'hello'.isWellFormed, 'function');
	assert.equal('well-formed ASCII', 'hello'.isWellFormed(), true);
	assert.equal('lone surrogate not well-formed', '\uD800'.isWellFormed(), false);
});

test('Object.groupBy/Map.groupBy (ES2024, GOJA-FORK-BLOCKED)', function () {
	assert.equal('Object.groupBy is function', typeof Object.groupBy, 'function');
	var grouped = Object.groupBy([1, 2, 3, 4], function (x) { return x % 2 ? 'odd' : 'even'; });
	assert.equal('odd group', grouped.odd.length, 2);
	assert.equal('even group', grouped.even.length, 2);
});
