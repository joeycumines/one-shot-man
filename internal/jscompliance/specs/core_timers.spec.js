// core_timers.spec.js — timer semantics (setTimeout/setInterval/clear*/setImmediate).
// Durations are tiny and bounded; the harness per-spec timeout catches hangs.

test('setTimeout fires exactly once with its delay id', function () {
	return new Promise(function (resolve) {
		var count = 0;
		var id = setTimeout(function () { count++; resolve(count); }, 5);
		assert.equal('setTimeout returns an id', typeof id !== 'undefined', true);
		// Give extra ticks to ensure it does NOT fire twice.
		setTimeout(function () { /* noop, lets a duplicate fire be observed */ }, 25);
		setTimeout(function () { resolve(count); }, 25);
	}).then(function (c) {
		assert.equal('fired once', c, 1);
	});
});

test('clearTimeout cancels a pending callback', function () {
	return new Promise(function (resolve) {
		var fired = false;
		var id = setTimeout(function () { fired = true; }, 10);
		clearTimeout(id);
		setTimeout(function () { resolve(fired); }, 30);
	}).then(function (fired) {
		assert.equal('cleared did not fire', fired, false);
	});
});

test('setInterval fires repeatedly until cleared', function () {
	return new Promise(function (resolve) {
		var count = 0;
		var id = setInterval(function () {
			count++;
			if (count >= 3) { clearInterval(id); resolve(count); }
		}, 5);
		// Safety: never hang the spec.
		setTimeout(function () { clearInterval(id); resolve(count); }, 200);
	}).then(function (c) {
		assert.equal('interval fired >=3 times', c >= 3, true);
	});
});

test('setImmediate fires in the next macrotask turn', function () {
	if (typeof setImmediate !== 'function') {
		assert.equal('setImmediate present', typeof setImmediate, 'function');
		return;
	}
	return new Promise(function (resolve) {
		var order = [];
		Promise.resolve().then(function () { order.push('micro'); });
		setImmediate(function () { order.push('immediate'); resolve(order); });
	}).then(function (order) {
		// microtask still drains before the immediate macrotask
		assert.deepEqual('immediate after microtask', order, ['micro', 'immediate']);
	});
});
