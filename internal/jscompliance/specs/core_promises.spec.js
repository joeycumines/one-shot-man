// core_promises.spec.js — Promise / async-await / combinators (incl ES2024).
// Test bodies use async/await (per the codebase style mandate), not .then chains.

test('Promise resolves with the given value', async function () {
	var v = await assert.resolves('resolve', Promise.resolve(7));
	assert.equal('resolved value', v, 7);
});

test('Promise rejects and catch recovers', async function () {
	try {
		await Promise.reject(new Error('nope'));
		assert.equal('should not fulfill', true, false);
	} catch (e) {
		assert.equal('rejection reason', e.message, 'nope');
	}
});

test('async/await unwraps a promise value', async function () {
	var v = await Promise.resolve(42);
	assert.equal('await value', v, 42);
});

test('Promise chaining sequences transforms', async function () {
	// await each step (async/await) instead of .then chains.
	var v = await Promise.resolve(1);
	v = await Promise.resolve(v + 1);
	assert.equal('chained', v, 2);
});

test('Promise.all resolves with all values in order', async function () {
	var arr = await Promise.all([Promise.resolve('a'), Promise.resolve('b')]);
	assert.deepEqual('all values', arr, ['a', 'b']);
});

test('Promise.all rejects on first rejection', async function () {
	try {
		await Promise.all([Promise.resolve(1), Promise.reject(new Error('x'))]);
		assert.equal('all should reject', true, false);
	} catch (e) {
		assert.equal('all rejection', e.message, 'x');
	}
});

test('Promise.race resolves with the first settled', async function () {
	var v = await Promise.race([Promise.resolve('fast'), new Promise(function (r) { setTimeout(function () { r('slow'); }, 20); })]);
	assert.equal('race winner', v, 'fast');
});

test('Promise.allSettled reports fulfill and reject outcomes', async function () {
	var res = await Promise.allSettled([Promise.resolve(1), Promise.reject(new Error('e'))]);
	assert.equal('allSettled[0] status', res[0].status, 'fulfilled');
	assert.equal('allSettled[0] value', res[0].value, 1);
	assert.equal('allSettled[1] status', res[1].status, 'rejected');
	assert.equal('allSettled[1] reason.message', res[1].reason.message, 'e');
});

test('Promise.any resolves with the first fulfilled', async function () {
	var v = await Promise.any([Promise.reject(new Error('a')), Promise.resolve('b')]);
	assert.equal('any winner', v, 'b');
});

test('Promise.withResolvers exposes resolve+reject (ES2024)', async function () {
	if (typeof Promise.withResolvers !== 'function') {
		assert.equal('withResolvers present', typeof Promise.withResolvers, 'function');
		return;
	}
	var ref = Promise.withResolvers();
	ref.resolve('ok');
	var v = await ref.promise;
	assert.equal('withResolvers value', v, 'ok');
});

test('Promise.try wraps a sync producer (ES2024)', async function () {
	if (typeof Promise.try !== 'function') {
		assert.equal('Promise.try present', typeof Promise.try, 'function');
		return;
	}
	var v = await Promise.try(function () { return 99; });
	assert.equal('Promise.try value', v, 99);
});
