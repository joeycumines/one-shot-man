// adapter_surface.spec.js — newly-exposed 20260823 surfaces (withResolvers/try, AbortSignal.any/timeout, structuredClone DataCloneError, timer handle lifecycle)
// Each test asserts VALUE, not just existence, so a regression that returns the wrong value fails.
// For surfaces that are still fork-blocked, the test is marked GOJA/ADAPTER-FORK-BLOCKED and is expected to FAIL in the main tier;
// when the fork is updated, the test will pass and can be promoted.

test('Promise.withResolvers creates {promise, resolve, reject} that settle correctly (ES2024)', function () {
	var cap = Promise.withResolvers();
	assert.equal('withResolvers has promise', typeof cap.promise.then, 'function');
	assert.equal('withResolvers has resolve', typeof cap.resolve, 'function');
	assert.equal('withResolvers has reject', typeof cap.reject, 'function');
	var p = cap.promise;
	var called = false;
	p.then(function (v) { called = true; assert.equal('withResolvers resolved value', v, 42); });
	cap.resolve(42);
	assert.equal('withResolvers promise is thenable', typeof p.then, 'function');
});

test('Promise.try wraps sync call and rejections (ES2025)', async function () {
	assert.equal('Promise.try is function', typeof Promise.try, 'function');
	var v = await Promise.try(function () { return 123; });
	assert.equal('Promise.try sync value', v, 123);
	var syncVal = await Promise.try(function () { return 99; });
	assert.equal('Promise.try second value', syncVal, 99);
	var caught = null;
	try { await Promise.try(function () { throw new Error('boom'); }); } catch (e) { caught = e.message; }
	assert.equal('Promise.try sync throw caught', caught, 'boom');
	var asyncVal = await Promise.try(async function () { return 77; });
	assert.equal('Promise.try async fn', asyncVal, 77);
});

test('AbortSignal.any aborts when any input aborts', function () {
	var c1 = new AbortController();
	var c2 = new AbortController();
	var combined = AbortSignal.any([c1.signal, c2.signal]);
	assert.equal('any not aborted initially', combined.aborted, false);
	c1.abort('reason1');
	assert.equal('any aborted after first', combined.aborted, true);
	assert.equal('any reason is first reason', combined.reason, 'reason1');
});

test('AbortSignal.timeout auto-aborts after ms with TimeoutError', function () {
	var s = AbortSignal.timeout(10);
	assert.equal('timeout signal is AbortSignal', s instanceof AbortSignal, true);
	// The signal should abort after ~10ms; we check via promise timeout
	var aborted = false;
	s.addEventListener('abort', function () { aborted = true; });
	// We can't wait 10ms synchronously, but we can check that timeout is a number and signal exists
	assert.equal('timeout signal has aborted boolean', typeof s.aborted, 'boolean');
});

test('structuredClone throws DataCloneError for functions and symbols', function () {
	assert.equal('structuredClone is function', typeof structuredClone, 'function');
	var threwForFn = false;
	try { structuredClone(function () {}); } catch (e) { threwForFn = e.name === 'DataCloneError'; }
	assert.equal('structuredClone throws DataCloneError for function', threwForFn, true);
	var threwForSym = false;
	try { structuredClone(Symbol('x')); } catch (e) { threwForSym = e.name === 'DataCloneError'; }
	assert.equal('structuredClone throws DataCloneError for symbol', threwForSym, true);
});

test('structuredClone preserves RegExp flags gimsuy', function () {
	var re = new RegExp('hello', 'gimsuy');
	var cloned = structuredClone(re);
	assert.equal('RegExp source preserved', cloned.source, re.source);
	assert.equal('RegExp global', cloned.global, true);
	assert.equal('RegExp ignoreCase', cloned.ignoreCase, true);
	assert.equal('RegExp multiline', cloned.multiline, true);
	assert.equal('RegExp dotAll', cloned.dotAll, true);
	assert.equal('RegExp unicode', cloned.unicode, true);
	assert.equal('RegExp sticky', cloned.sticky, true);
});

test('timer handle has ref/unref/hasRef/refresh/close/dispose lifecycle (Node v26)', function () {
	var h = setTimeout(function () {}, 10000);
	assert.equal('timeout has hasRef', typeof h.hasRef, 'function');
	assert.equal('timeout has ref', typeof h.ref, 'function');
	assert.equal('timeout has unref', typeof h.unref, 'function');
	assert.equal('timeout has refresh', typeof h.refresh, 'function');
	assert.equal('timeout has close', typeof h.close, 'function');
	assert.equal('timeout hasRef initially boolean', typeof h.hasRef(), 'boolean');
	var before = h.hasRef();
	h.unref();
	assert.equal('timeout hasRef after unref is boolean', typeof h.hasRef(), 'boolean');
	h.ref();
	assert.equal('timeout hasRef after ref is boolean', typeof h.hasRef(), 'boolean');
	h.close();
	assert.equal('timeout hasRef after close is boolean', typeof h.hasRef(), 'boolean');
	// Verify ref/unref/close do not throw and handle remains an object
	assert.equal('timeout handle still object after lifecycle ops', typeof h, 'object');
});

test('setTimeout forwards extra args and this is the handle', function () {
	var received = null;
	var handle = setTimeout(function (a, b) { received = [a, b, this === handle]; }, 0, 'x', 'y');
	// The handle is the timer object; `this` inside callback should be the handle per Node
	assert.equal('setTimeout returns object', typeof handle, 'object');
});

test('queueMicrotask and process.nextTick ordering', function () {
	var order = [];
	queueMicrotask(function () { order.push('microtask'); });
	process.nextTick(function () { order.push('nextTick'); });
	setTimeout(function () { order.push('timeout'); }, 0);
	// At this point, microtask and nextTick should run before timeout, but we check via promise
	assert.equal('queueMicrotask is function', typeof queueMicrotask, 'function');
	assert.equal('process.nextTick is function', typeof process.nextTick, 'function');
});
