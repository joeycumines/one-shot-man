// core_abort.spec.js — AbortController / AbortSignal (incl ES2024 statics).

test('abort() flips aborted and fires listeners', function () {
	var ac = new AbortController();
	assert.equal('not aborted initially', ac.signal.aborted, false);
	var seen = 0;
	ac.signal.addEventListener('abort', function () { seen++; });
	ac.abort();
	assert.equal('aborted after abort()', ac.signal.aborted, true);
	assert.equal('listener fired', seen, 1);
});

// ADAPTER-FORK-BLOCKED — abort(reason) does not store the reason as
// signal.reason (the reason is lost, exposed as an empty object). This test
// FAILS until the goja-eventloop adapter fork is updated to store the reason
// passed to abort() and expose it via signal.reason. Per the compliance
// directive: non-compliance of ANY KIND must bubble as a test failure.
test('abort(reason) is exposed as signal.reason (ADAPTER-FORK-BLOCKED)', function () {
	var ac = new AbortController();
	ac.abort(new Error('stopped'));
	assert.equal('reason is the error', ac.signal.reason.message, 'stopped');
});

test('AbortSignal.timeout(ms) resolves aborted=true (ES2024)', async function () {
	if (typeof AbortSignal.timeout !== 'function') {
		assert.equal('AbortSignal.timeout present', typeof AbortSignal.timeout, 'function');
		return;
	}
	var aborted = await new Promise(function (resolve) {
		var s = AbortSignal.timeout(15);
		s.addEventListener('abort', function () { resolve(s.aborted); });
	});
	assert.equal('timeout aborted', aborted, true);
});

test('AbortSignal.any() aborts when any source aborts (ES2024)', function () {
	if (typeof AbortSignal.any !== 'function') {
		assert.equal('AbortSignal.any present', typeof AbortSignal.any, 'function');
		return;
	}
	var a = new AbortController();
	var b = new AbortController();
	var combined = AbortSignal.any([a.signal, b.signal]);
	assert.equal('combined not aborted initially', combined.aborted, false);
	b.abort();
	assert.equal('combined aborts with any source', combined.aborted, true);
});
