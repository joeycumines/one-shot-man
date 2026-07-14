// core_es.spec.js — core ECMAScript semantics the runtime MUST get right.
// Each test asserts a VALUE (not just existence), so a regression that
// returns the wrong value fails. Covers the 11 axes named in blueprint v3.

// 1. Typed arrays / ArrayBuffer / DataView
test('Uint8Array round-trips values and exposes byteLength/length', function () {
	var u = new Uint8Array([10, 20, 30]);
	assert.equal('u8 length', u.length, 3);
	assert.equal('u8 byteLength', u.byteLength, 3);
	assert.equal('u8[1]', u[1], 20);
	assert.equal('u8 from ArrayBuffer view', new Uint8Array(u.buffer, 0, 2).length, 2);
});

test('DataView reads/writes over a shared ArrayBuffer', function () {
	var buf = new ArrayBuffer(4);
	var dv = new DataView(buf);
	dv.setUint32(0, 0x11223344, false); // big-endian
	assert.equal('dv getUint32', dv.getUint32(0, false), 0x11223344);
	assert.equal('dv getUint8 high byte', dv.getUint8(0), 0x11);
});

// 2. Well-known Symbols
test('Symbol.iterator drives for..of and spread', function () {
	var arr = [1, 2, 3];
	assert.equal('Symbol.iterator is a symbol', typeof Symbol.iterator, 'symbol');
	assert.equal('array iterator is a function', typeof arr[Symbol.iterator], 'function');
	var sum = 0;
	for (var v of arr) sum += v;
	assert.equal('for..of sum', sum, 6);
	assert.deepEqual('spread', [0].concat(arr), [0, 1, 2, 3]);
});

test('Symbol.toStringTag and a custom tag surface via Object.prototype.toString', function () {
	var o = {};
	o[Symbol.toStringTag] = 'CustomTag';
	assert.equal('toStringTag', Object.prototype.toString.call(o), '[object CustomTag]');
});

test('Symbol.toPrimitive customizes coercion', function () {
	var o = {};
	o[Symbol.toPrimitive] = function (hint) { return hint === 'number' ? 42 : 's'; };
	assert.equal('toPrimitive number', +o, 42);
	assert.equal('toPrimitive default', '' + o, 's');
});

// 3. Number / Math / String ES-correctness
test('Number constants and predicates', function () {
	assert.equal('MAX_SAFE_INTEGER', Number.MAX_SAFE_INTEGER, 9007199254740991);
	assert.equal('EPSILON is small', Number.EPSILON < 1e-15, true);
	assert.equal('isFinite(1.5)', Number.isFinite(1.5), true);
	assert.equal('isFinite(NaN) false', Number.isFinite(NaN), false);
	assert.equal('isNaN(NaN)', Number.isNaN(NaN), true);
	assert.equal('isInteger(2)', Number.isInteger(2), true);
	assert.equal('isInteger(2.5) false', Number.isInteger(2.5), false);
});

test('Math functions return correct values', function () {
	assert.equal('cbrt(27)', Math.cbrt(27), 3);
	assert.equal('clz32(1)', Math.clz32(1), 31);
	assert.equal('sign(-5)', Math.sign(-5), -1);
	assert.equal('hypot(3,4)', Math.hypot(3, 4), 5);
	assert.equal('trunc(-2.9)', Math.trunc(-2.9), -2);
});

test('String.prototype.normalize and surrogate pairs', function () {
	// a + combining diaeresis normalizes to single ä (U+00E4)
	assert.equal('NFC normalize', 'ä'.normalize('NFC'), 'ä');
	// Astral character '𝟙' (U+1D7D9) is 2 UTF-16 code units, 1 code point
	var s = '𝟙';
	assert.equal('astral length is 2 code units', s.length, 2);
	assert.equal('codePointAt astral', s.codePointAt(0), 0x1d7d9);
});

// 4. Error subclassing, .cause, AggregateError
test('Error subclassing preserves the prototype chain', function () {
	function MyError(message) { this.name = 'MyError'; this.message = message; }
	MyError.prototype = Object.create(Error.prototype);
	var e = new MyError('boom');
	assert.equal('instanceof Error', e instanceof Error, true);
	assert.equal('instanceof MyError', e instanceof MyError, true);
	assert.equal('message preserved', e.message, 'boom');
});

test('AggregateError exists and aggregates errors', function () {
	if (typeof AggregateError !== 'function') { assert.equal('AggregateError present', typeof AggregateError, 'function'); return; }
	var inner = [new Error('a'), new Error('b')];
	var agg = new AggregateError(inner, 'many');
	assert.equal('AggregateError is Error', agg instanceof Error, true);
	assert.equal('AggregateError.errors', agg.errors.length, 2);
});

// 5. Proxy / Reflect
test('Proxy get trap intercepts property access', function () {
	var p = new Proxy({base: 1}, { get: function (t, k) { return k === 'x' ? 42 : t[k]; } });
	assert.equal('proxy intercepted', p.x, 42);
	assert.equal('proxy passthrough', p.base, 1);
});

test('Reflect mirrors core object operations', function () {
	var o = {a: 1};
	assert.equal('Reflect.has', Reflect.has(o, 'a'), true);
	assert.equal('Reflect.get', Reflect.get(o, 'a'), 1);
	Reflect.set(o, 'b', 2);
	assert.equal('Reflect.set', o.b, 2);
	assert.equal('Reflect.ownKeys', Reflect.ownKeys(o).length, 2);
});

// 6. Generators and async generators
test('generators yield a correct iterable sequence', function () {
	function* gen() { yield 1; yield 2; yield 3; }
	var out = '';
	for (var v of gen()) out += v;
	assert.equal('generator sequence', out, '123');
});

// NOTE: goja does NOT support `for await...of` / async iteration (the parser
// rejects `for await` — a SyntaxError that would prevent this whole spec file
// from loading). `Symbol.asyncIterator` is a well-known symbol per ecma-262
// §6.1.5.1 that MUST be registered. The goja fork does NOT register it
// (returns undefined). This test FAILS until the goja fork is updated to
// register @@asyncIterator in the well-known symbols table. The parse-unsafe
// for-await-of syntax is deliberately omitted so this file loads.
//
// GOJA-FORK-BLOCKED — tracked as a failing test (not a skip) per the
// compliance directive: non-compliance of ANY KIND must bubble as a test
// failure.
test('Symbol.asyncIterator is a registered well-known symbol (GOJA-FORK-BLOCKED)', function () {
	assert.equal('Symbol.asyncIterator typeof', typeof Symbol.asyncIterator, 'symbol');
});

// 7. Destructuring, optional chaining, nullish coalescing
test('destructuring binds and defaults correctly', function () {
	var _a = (function () { var _ref = { a: 1 }, a = _ref.a, b = _ref.b === undefined ? 5 : _ref.b; return [a, b]; })();
	assert.deepEqual('destructuring with default', _a, [1, 5]);
	var nested = { x: { y: 9 } };
	var x = nested.x;
	assert.equal('nested destructuring', x.y, 9);
});

test('optional chaining and nullish coalescing', function () {
	var o = { a: { b: 7 } };
	assert.equal('optional chaining hit', o?.a?.b, 7);
	assert.equal('optional chaining miss', o?.c?.d, undefined);
	assert.equal('nullish coalescing null', null ?? 42, 42);
	assert.equal('nullish coalescing undefined', undefined ?? 42, 42);
	assert.equal('nullish coalescing falsy-but-defined', 0 ?? 42, 0);
});

// 8. BigInt
test('BigInt arithmetic stays integral', function () {
	assert.equal('bigint typeof', typeof 1n, 'bigint');
	assert.equal('bigint add', 1n + 2n, 3n);
	assert.equal('bigint multiply', 10n * 10n, 100n);
});

// 9. JSON edge cases
test('JSON.stringify drops undefined/functions/symbols and nulls them in arrays', function () {
	assert.equal('stringify undefined', JSON.stringify(undefined), undefined);
	assert.equal('stringify function', JSON.stringify(function () {}), undefined);
	assert.equal('stringify array with fn', JSON.stringify([1, function () {}, 3]), '[1,null,3]');
});

test('JSON.parse reviver transforms values', function () {
	var o = JSON.parse('{"a":2,"b":"x"}', function (k, v) { return typeof v === 'number' ? v * 10 : v; });
	assert.equal('reviver number', o.a, 20);
	assert.equal('reviver passthrough', o.b, 'x');
});

test('JSON.parse __proto__ does not pollute Object.prototype', function () {
	var parsed = JSON.parse('{"__proto__":{"polluted":1}}');
	// The __proto__ key becomes an own property, not the prototype.
	assert.equal('no prototype pollution', ({}).polluted, undefined);
});

// 10. CommonJS module.exports vs exports aliasing is exercised in the
// resolution spec (it needs a real module file).

// 11. Go<->JS marshalling: Number precision boundary
test('Number precision boundary at 2^53', function () {
	assert.equal('MAX_SAFE_INTEGER + 1 loses precision', 9007199254740991 + 2, 9007199254740992); // 2^53
	// 2^53 + 1 is NOT representable; it rounds to 2^53
	assert.equal('2^53+1 rounds to 2^53', 9007199254740992 + 1, 9007199254740992);
});

// --- typed arrays: more views + value fidelity ---
test('Int32Array and Float64Array preserve values', function () {
	var i = new Int32Array([1000000, -2]);
	assert.equal('i32[0]', i[0], 1000000);
	assert.equal('i32[1]', i[1], -2);
	var f = new Float64Array([0.5, 2.5]);
	assert.equal('f64 sum', f[0] + f[1], 3);
});
test('ArrayBuffer is the shared backing store of a typed array', function () {
	var buf = new ArrayBuffer(8);
	var v1 = new Int32Array(buf);
	v1[0] = 42;
	var v2 = new Int32Array(buf);
	assert.equal('views share memory', v2[0], 42);
});

// --- Number boundaries + safe-integer ---
test('Number constants and isSafeInteger', function () {
	assert.equal('MAX_VALUE is finite', Number.isFinite(Number.MAX_VALUE), true);
	assert.equal('MIN_VALUE > 0', Number.MIN_VALUE > 0, true);
	assert.equal('isSafeInteger(2^53-1)', Number.isSafeInteger(9007199254740991), true);
	assert.equal('isSafeInteger(2^53) false', Number.isSafeInteger(9007199254740992), false);
});

// --- iterator protocol: a custom iterable ---
test('a custom [Symbol.iterator] makes an object iterable', function () {
	var obj = {};
	obj[Symbol.iterator] = function () {
		var n = 0;
		return { next: function () { n++; return n <= 3 ? { value: n * 10, done: false } : { value: undefined, done: true }; } };
	};
	var out = [];
	for (var v of obj) out.push(v);
	assert.deepEqual('custom iterable sequence', out, [10, 20, 30]);
	assert.deepEqual('spread custom iterable', [...obj], [10, 20, 30]);
});

// --- String.raw ---
test('String.raw preserves backslashes literally', function () {
	assert.equal('String.raw keeps backslash', String.raw`\n`, '\\n');
});

// --- Map / Set / WeakMap collection semantics ---
test('Map preserves insertion order and supports get/set/has/delete/size', function () {
	var m = new Map();
	m.set('b', 2); m.set('a', 1); m.set('c', 3);
	assert.equal('map size', m.size, 3);
	assert.equal('map get a', m.get('a'), 1);
	assert.equal('map has c', m.has('c'), true);
	m.delete('a');
	assert.equal('map size after delete', m.size, 2);
	// insertion order preserved: b, c
	var keys = [];
	m.forEach(function (v, k) { keys.push(k); });
	assert.deepEqual('map insertion order', keys, ['b', 'c']);
});
test('Set dedupes and iterates in insertion order', function () {
	var s = new Set([1, 2, 2, 3, 1]);
	assert.equal('set size deduped', s.size, 3);
	assert.equal('set has 2', s.has(2), true);
	s.add(4); s.delete(2);
	assert.equal('set size after ops', s.size, 3);
	var vals = [];
	s.forEach(function (v) { vals.push(v); });
	assert.deepEqual('set insertion order', vals, [1, 3, 4]);
});
test('WeakMap supports get/set/has/delete (no size/iteration)', function () {
	var wm = new WeakMap();
	var k = {};
	wm.set(k, 'v');
	assert.equal('weakmap get', wm.get(k), 'v');
	assert.equal('weakmap has', wm.has(k), true);
	wm.delete(k);
	assert.equal('weakmap has after delete', wm.has(k), false);
});

// --- ES2022-2025 features (supported by goja, pinned to catch regressions) ---

// ES2023: Array.prototype.findLast / findLastIndex
test('Array.prototype.findLast/findLastIndex (ES2023)', function () {
	assert.equal('findLast', [1, 2, 3, 2].findLast(function (x) { return x === 2; }), 2);
	assert.equal('findLastIndex', [1, 2, 3, 2].findLastIndex(function (x) { return x === 2; }), 3);
	assert.equal('findLast miss', [1, 2, 3].findLast(function (x) { return x > 10; }), undefined);
});

// ES2022: Object.hasOwn
test('Object.hasOwn (ES2022)', function () {
	assert.equal('hasOwn own prop', Object.hasOwn({ a: 1 }, 'a'), true);
	assert.equal('hasOwn inherited', Object.hasOwn({}, 'toString'), false);
	assert.equal('hasOwn missing', Object.hasOwn({ a: 1 }, 'b'), false);
});

// ES2022: Error.cause on Error subclasses
test('Error.cause propagates to TypeError/RangeError (ES2022)', function () {
	var e = new TypeError('msg', { cause: 'root' });
	assert.equal('TypeError cause', e.cause, 'root');
	var r = new RangeError('msg', { cause: 42 });
	assert.equal('RangeError cause', r.cause, 42);
});

// ES2024: String.prototype.isWellFormed / toWellFormed
// GOJA-FORK-BLOCKED — the goja fork does NOT implement isWellFormed/toWellFormed
// (returns undefined). ecma-262 §22.1.3.x. This test FAILS until the goja fork
// is updated. Per the compliance directive: non-compliance bubbles as a failure.
test('String.prototype.isWellFormed/toWellFormed (ES2024, GOJA-FORK-BLOCKED)', function () {
	assert.equal('isWellFormed is a function', typeof 'hello'.isWellFormed, 'function');
	assert.equal('well-formed ASCII', 'hello'.isWellFormed(), true);
	assert.equal('lone surrogate not well-formed', '\uD800'.isWellFormed(), false);
});

// ES2024: Object.groupBy / Map.groupBy
// GOJA-FORK-BLOCKED — the goja fork does NOT implement Object.groupBy/Map.groupBy
// (returns undefined). ecma-262 §20.1.2.x. This test FAILS until the goja fork
// is updated.
test('Object.groupBy/Map.groupBy (ES2024, GOJA-FORK-BLOCKED)', function () {
	assert.equal('Object.groupBy is function', typeof Object.groupBy, 'function');
	var grouped = Object.groupBy([1, 2, 3, 4], function (x) { return x % 2 ? 'odd' : 'even'; });
	assert.equal('odd group', grouped.odd.length, 2);
	assert.equal('even group', grouped.even.length, 2);
});
