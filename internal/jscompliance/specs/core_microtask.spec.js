// core_microtask.spec.js — pins the runtime's WithStrictMicrotaskOrdering
// guarantee: microtasks (Promise.then) drain after EVERY macrotask, so a
// .then chain always completes before a setTimeout callback runs.

test('Promise.then drains before setTimeout fires', function () {
	return new Promise(function (resolve) {
		var order = [];
		Promise.resolve().then(function () { order.push('micro'); });
		setTimeout(function () { order.push('macro'); resolve(order); }, 5);
	}).then(function (order) {
		assert.deepEqual('microtask before macrotask', order, ['micro', 'macro']);
	});
});

test('a chain of .then runs to completion before the next macrotask', function () {
	return new Promise(function (resolve) {
		var order = [];
		Promise.resolve()
			.then(function () { order.push(1); })
			.then(function () { order.push(2); })
			.then(function () { order.push(3); });
		setTimeout(function () { order.push('t'); resolve(order); }, 5);
	}).then(function (order) {
		assert.deepEqual('full microtask chain first', order, [1, 2, 3, 't']);
	});
});

test('queueMicrotask runs after the current sync block, before setTimeout', function () {
	return new Promise(function (resolve) {
		var order = [];
		queueMicrotask(function () { order.push('qm'); });
		setTimeout(function () { order.push('st'); resolve(order); }, 5);
		order.push('sync');
	}).then(function (order) {
		assert.deepEqual('queueMicrotask ordering', order, ['sync', 'qm', 'st']);
	});
});
