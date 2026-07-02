// core_timers.spec.js — timer semantics (setTimeout/setInterval/clear*/setImmediate).
// Durations are tiny and bounded; the harness per-spec timeout catches hangs.
// Test bodies use async/await for sequencing.

test('setTimeout fires exactly once', async function () {
	var count = await new Promise(function (resolve) {
		var n = 0;
		setTimeout(function () { n++; }, 5);
		// Observe at 30ms: a duplicate fire would make n > 1.
		setTimeout(function () { resolve(n); }, 30);
	});
	assert.equal('fired once', count, 1);
});

test('clearTimeout cancels a pending callback', async function () {
	var fired = await new Promise(function (resolve) {
		var got = false;
		var id = setTimeout(function () { got = true; }, 10);
		clearTimeout(id);
		setTimeout(function () { resolve(got); }, 30);
	});
	assert.equal('cleared did not fire', fired, false);
});

test('setInterval fires repeatedly until cleared', async function () {
	var count = await new Promise(function (resolve) {
		var n = 0;
		var id = setInterval(function () {
			n++;
			if (n >= 3) { clearInterval(id); resolve(n); }
		}, 5);
		setTimeout(function () { clearInterval(id); resolve(n); }, 200);
	});
	assert.equal('interval fired >=3 times', count >= 3, true);
});

test('setImmediate fires in the next macrotask turn', async function () {
	if (typeof setImmediate !== 'function') {
		assert.equal('setImmediate present', typeof setImmediate, 'function');
		return;
	}
	var order = await new Promise(function (resolve) {
		var seq = [];
		Promise.resolve().then(function () { seq.push('micro'); });
		setImmediate(function () { seq.push('immediate'); resolve(seq); });
	});
	// microtask still drains before the immediate macrotask
	assert.deepEqual('immediate after microtask', order, ['micro', 'immediate']);
});
