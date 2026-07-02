// core_promises.spec.js — Promise / async-await / combinators (incl ES2024).

test('Promise resolves with the given value', function () {
	return assert.resolves('resolve', Promise.resolve(7)).then(function (v) {
		assert.equal('resolved value', v, 7);
	});
});

test('Promise rejects and catch recovers', function () {
	return Promise.reject(new Error('nope')).then(
		function () { assert.equal('should not fulfill', true, false); },
		function (e) { assert.equal('rejection reason', e.message, 'nope'); }
	);
});

test('async/await unwraps a promise value', function () {
	return (async function () {
		var v = await Promise.resolve(42);
		assert.equal('await value', v, 42);
	})();
});

test('Promise chaining sequences transforms', function () {
	return Promise.resolve(1).then(function (v) { return v + 1; }).then(function (v) {
		assert.equal('chained', v, 2);
	});
});

test('Promise.all resolves with all values in order', function () {
	return Promise.all([Promise.resolve('a'), Promise.resolve('b')]).then(function (arr) {
		assert.deepEqual('all values', arr, ['a', 'b']);
	});
});

test('Promise.all rejects on first rejection', function () {
	return Promise.all([Promise.resolve(1), Promise.reject(new Error('x'))]).then(
		function () { assert.equal('all should reject', true, false); },
		function (e) { assert.equal('all rejection', e.message, 'x'); }
	);
});

test('Promise.race resolves with the first settled', function () {
	return Promise.race([Promise.resolve('fast'), new Promise(function (r) { setTimeout(function () { r('slow'); }, 20); })]).then(function (v) {
		assert.equal('race winner', v, 'fast');
	});
});

test('Promise.allSettled reports fulfill and reject outcomes', function () {
	return Promise.allSettled([Promise.resolve(1), Promise.reject(new Error('e'))]).then(function (res) {
		assert.equal('allSettled[0] status', res[0].status, 'fulfilled');
		assert.equal('allSettled[0] value', res[0].value, 1);
		assert.equal('allSettled[1] status', res[1].status, 'rejected');
		assert.equal('allSettled[1] reason.message', res[1].reason.message, 'e');
	});
});

test('Promise.any resolves with the first fulfilled', function () {
	return Promise.any([Promise.reject(new Error('a')), Promise.resolve('b')]).then(function (v) {
		assert.equal('any winner', v, 'b');
	});
});

test('Promise.withResolvers exposes resolve+reject (ES2024)', function () {
	if (typeof Promise.withResolvers !== 'function') {
		assert.equal('withResolvers present', typeof Promise.withResolvers, 'function');
		return;
	}
	var _Promise$withResolver = Promise.withResolvers();
	var promise = _Promise$withResolver.promise;
	var resolve = _Promise$withResolver.resolve;
	resolve('ok');
	return promise.then(function (v) { assert.equal('withResolvers value', v, 'ok'); });
});

test('Promise.try wraps a sync producer (ES2024)', function () {
	if (typeof Promise.try !== 'function') {
		assert.equal('Promise.try present', typeof Promise.try, 'function');
		return;
	}
	return Promise.try(function () { return 99; }).then(function (v) {
		assert.equal('Promise.try value', v, 99);
	});
});
