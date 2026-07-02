// core_microtask.spec.js — pins the runtime's WithStrictMicrotaskOrdering
// guarantee: microtasks (Promise.then) drain after EVERY macrotask, so a
// .then chain always completes before a setTimeout callback runs.
// Test bodies use async/await for sequencing (the inner .then calls are the
// subject under test — they verify microtask draining).

test('Promise.then drains before setTimeout fires', async function () {
	var order = await new Promise(function (resolve) {
		var seq = [];
		Promise.resolve().then(function () { seq.push('micro'); });
		setTimeout(function () { seq.push('macro'); resolve(seq); }, 5);
	});
	assert.deepEqual('microtask before macrotask', order, ['micro', 'macro']);
});

test('a chain of .then runs to completion before the next macrotask', async function () {
	var order = await new Promise(function (resolve) {
		var seq = [];
		var p = Promise.resolve();
		p.then(function () { seq.push(1); }).then(function () { seq.push(2); }).then(function () { seq.push(3); });
		setTimeout(function () { seq.push('t'); resolve(seq); }, 5);
	});
	assert.deepEqual('full microtask chain first', order, [1, 2, 3, 't']);
});

test('queueMicrotask runs after the current sync block, before setTimeout', async function () {
	var order = await new Promise(function (resolve) {
		var seq = [];
		queueMicrotask(function () { seq.push('qm'); });
		setTimeout(function () { seq.push('st'); resolve(seq); }, 5);
		seq.push('sync');
	});
	assert.deepEqual('queueMicrotask ordering', order, ['sync', 'qm', 'st']);
});
