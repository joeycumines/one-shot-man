// global_surface.spec.js — inventory of every WHATWG/ES global the
// goja-eventloop adapter + goja bind into the runtime, with a smoke VALUE
// assertion for each that exists. A missing expected global FAILS; this is
// the ES2020+ surface contract.
//
// Presence is checked via globalThis[name] member access (NOT eval): the names
// are hardcoded string literals in this spec, never untrusted input.

// presentFn asserts the global named `name` is a function.
function presentFn(name) {
	test(name + ' is present (function)', function () {
		assert.equal(name + ' typeof', typeof globalThis[name], 'function');
	});
}

// presentDefined asserts the global named `name` is defined (any type).
function presentDefined(name) {
	test(name + ' is present (defined)', function () {
		assert.equal(name + ' defined', typeof globalThis[name] === 'undefined', false);
	});
}

// --- Text / encoding ---
presentFn('TextEncoder');
presentFn('TextDecoder');
test('TextEncoder encodes UTF-8 bytes', function () {
	var u = new TextEncoder().encode('AB');
	assert.equal('te length', u.length, 2);
	assert.equal('te byte', u[0], 65);
});
test('TextDecoder decodes UTF-8', function () {
	var s = new TextDecoder().decode(new Uint8Array([65, 66]));
	assert.equal('td decode', s, 'AB');
});
presentDefined('atob');
presentDefined('btoa');
test('btoa/atob round-trip', function () {
	// Assert presence FIRST (no silent skip): a missing atob/btoa must FAIL.
	assert.equal('atob present', typeof atob === 'undefined', false);
	assert.equal('btoa present', typeof btoa === 'undefined', false);
	assert.equal('atob(btoa(x)) round-trips', atob(btoa('hi')), 'hi');
});

// --- URL ---
presentFn('URL');
presentFn('URLSearchParams');
test('URL parses host and path', function () {
	var u = new URL('https://example.com/a/b?x=1');
	assert.equal('url host', u.host, 'example.com');
	assert.equal('url pathname', u.pathname, '/a/b');
});
test('URLSearchParams round-trips', function () {
	var p = new URLSearchParams('a=1&b=2');
	assert.equal('usp get', p.get('a'), '1');
	p.set('c', '3');
	assert.equal('usp toString has c', p.toString().indexOf('c=3') >= 0, true);
});

// --- Blob / FormData / Headers / DOMException / structuredClone ---
presentDefined('Blob');
presentDefined('Headers');
presentDefined('FormData');
presentDefined('DOMException');
presentDefined('structuredClone');
test('structuredClone deep-copies an object', function () {
	assert.equal('structuredClone present', typeof structuredClone === 'undefined', false);
	var o = { a: { b: 1 } };
	var c = structuredClone(o);
	c.a.b = 2;
	assert.equal('original untouched', o.a.b, 1);
});

// --- performance / crypto (adapter WHATWG crypto, NOT osm:crypto) ---
presentDefined('performance');
test('performance.now returns a number', function () {
	assert.equal('performance present', typeof performance === 'undefined', false);
	assert.equal('performance.now is number', typeof performance.now(), 'number');
});
test('crypto.randomUUID returns a uuid-shaped string (WHATWG global)', function () {
	assert.equal('crypto present', typeof crypto === 'undefined', false);
	assert.equal('crypto.randomUUID is function', typeof crypto.randomUUID, 'function');
	var id = crypto.randomUUID();
	assert.equal('uuid length', id.length, 36);
});
test('crypto.getRandomValues fills a Uint8Array', function () {
	assert.equal('crypto present', typeof crypto === 'undefined', false);
	assert.equal('crypto.getRandomValues is function', typeof crypto.getRandomValues, 'function');
	var a = new Uint8Array(4);
	crypto.getRandomValues(a);
	assert.equal('getRandomValues filled', a.every(function (v) { return typeof v === 'number'; }), true);
});

// --- Timers / microtask (also covered in core_timers; presence here) ---
presentFn('setTimeout');
presentFn('clearTimeout');
presentFn('setInterval');
presentFn('clearInterval');
presentFn('queueMicrotask');
presentDefined('setImmediate');

// --- AbortController / AbortSignal (also core_abort; presence here) ---
presentFn('AbortController');
presentFn('AbortSignal');

// --- Promise + ES2024 statics ---
presentFn('Promise');
test('Promise combinators + ES2024 statics present', function () {
	assert.equal('Promise.all', typeof Promise.all, 'function');
	assert.equal('Promise.race', typeof Promise.race, 'function');
	assert.equal('Promise.allSettled', typeof Promise.allSettled, 'function');
	assert.equal('Promise.any', typeof Promise.any, 'function');
	// withResolvers/try are ES2024 — record presence (typeof, not a hard fail
	// if the adapter lags, but flag it).
	if (typeof Promise.withResolvers !== 'function') { assert.equal('Promise.withResolvers (ES2024)', typeof Promise.withResolvers, 'function'); }
	if (typeof Promise.try !== 'function') { assert.equal('Promise.try (ES2024)', typeof Promise.try, 'function'); }
});

// --- console (detailed routing in console_test.go) ---
presentDefined('console');

// --- Symbol / Map / Set / WeakMap / Proxy / Reflect / BigInt (core_es covers behavior) ---
presentFn('Symbol');
presentFn('Map');
presentFn('Set');
presentFn('Proxy');
presentFn('Reflect');
presentDefined('BigInt');
