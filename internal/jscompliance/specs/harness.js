// harness.js — the compliance-suite assertion runtime and result collector.
//
// Loaded before every spec. Provides:
//   test(name, fn)          register a test (fn may be sync or async)
//   assert.*                value/shape assertions (each records pass/fail)
//   __results               array of {name, pass, error}
//   __done                  Promise that settles when every registered test
//                           has settled; the Go runner attaches handlers to it
//   __finishRegistration()  called by the Go runner after the spec body runs
//
// Design rules:
//   - Failures are RECORDED, never thrown out of test() (a failing assertion
//     must not abort sibling tests). A thrown error inside the test body is
//     captured as that test's failure.
//   - assert.resolves/assert.rejects AWAIT their promise; a never-settling
//     promise fails via the Go runner's per-call timeout (settlement is
//     required).
//   - No globals beyond this API are introduced.

var __results = [];
var __pending = 0;
var __registrationOpen = true;
var __resolveDone = null;
var __done = new Promise(function (resolve) { __resolveDone = resolve; });

function __record(name, pass, message) {
	__results.push({ name: name, pass: !!pass, error: (pass || !message) ? null : String(message) });
}

function __settleOne() {
	__pending -= 1;
	if (__pending <= 0 && !__registrationOpen && __resolveDone !== null) {
		var resolve = __resolveDone;
		__resolveDone = null;
		resolve();
	}
}

// Per-test outcome tracking. assert.* mark the test failed via __fail (keeping
// the FIRST, most-informative message). The test() settlement records the
// outcome EXACTLY ONCE via __recordOutcome — so a failed test never produces
// duplicate records (a previous version recorded both in __fail and at
// settlement).
var __failures = {}; // name -> true
var __failMsgs = {}; // name -> first failure message

function __fail(name, message) {
	if (!__failures[name]) {
		__failures[name] = true;
		__failMsgs[name] = String(message);
	}
}

// __recordOutcome records a single outcome for name: prefer an assertion
// failure message (most descriptive), else a thrown/rejected error, else pass.
function __recordOutcome(name, threw, threwMsg) {
	if (__failures[name]) {
		__record(name, false, __failMsgs[name]);
	} else if (threw) {
		__record(name, false, threwMsg);
	} else {
		__record(name, true, null);
	}
}

// test registers a test. fn may return a Promise (async) or a value (sync)
// or throw. The test is recorded as failed if any assert fails or fn
// throws/rejects.
function test(name, fn) {
	__pending += 1;
	// Defer to a microtask so registration is synchronous even if fn throws
	// synchronously — all test() calls in a spec register before any runs.
	Promise.resolve()
		.then(function () {
			var threw = false;
			var threwMsg = null;
			var r;
			try {
				r = fn();
			} catch (e) {
				threw = true;
				threwMsg = __errMsg(e);
			}
			if (threw) {
				__recordOutcome(name, true, threwMsg);
				__settleOne();
				return;
			}
			if (r !== undefined && r !== null && typeof r.then === 'function') {
				r.then(
					function () { __recordOutcome(name, false, null); __settleOne(); },
					function (e) { __recordOutcome(name, true, __errMsg(e)); __settleOne(); }
				);
			} else {
				__recordOutcome(name, false, null);
				__settleOne();
			}
		})
		.catch(function (e) { // should be unreachable; defensive
			__record(name, false, 'harness error: ' + __errMsg(e));
			__settleOne();
		});
}

function __errMsg(e) {
	if (e === null || e === undefined) return String(e);
	if (e && e.message !== undefined) return String(e.message);
	return String(e);
}

function __nameOf(v) {
	if (v === null) return 'null';
	if (v === undefined) return 'undefined';
	if (typeof v === 'object') {
		if (Array.isArray(v)) return 'array';
		if (v && v.constructor && v.constructor.name) return v.constructor.name;
		return 'object';
	}
	return typeof v;
}

function __deepEqual(a, b) {
	if (a === b) return true;
	if (typeof a !== typeof b) return false;
	if (a === null || b === null) return a === b;
	if (typeof a !== 'object') return a === b;
	var aArr = Array.isArray(a), bArr = Array.isArray(b);
	if (aArr !== bArr) return false;
	if (aArr) {
		if (a.length !== b.length) return false;
		for (var i = 0; i < a.length; i++) if (!__deepEqual(a[i], b[i])) return false;
		return true;
	}
	var ak = Object.keys(a), bk = Object.keys(b);
	if (ak.length !== bk.length) return false;
	for (var k = 0; k < ak.length; k++) {
		var key = ak[k];
		if (!Object.prototype.hasOwnProperty.call(b, key)) return false;
		if (!__deepEqual(a[key], b[key])) return false;
	}
	return true;
}

var assert = {
	ok: function (name, value, message) {
		if (!value) __fail(name, message || ('expected truthy, got ' + __nameOf(value) + ': ' + JSON.stringify(value)));
	},
	equal: function (name, actual, expected, message) {
		if (actual !== expected) __fail(name, message || ('expected ' + JSON.stringify(expected) + ', got ' + JSON.stringify(actual)));
	},
	notEqual: function (name, actual, expected, message) {
		if (actual === expected) __fail(name, message || ('expected not ' + JSON.stringify(expected)));
	},
	deepEqual: function (name, actual, expected, message) {
		if (!__deepEqual(actual, expected)) __fail(name, message || ('deepEqual failed: expected ' + JSON.stringify(expected) + ', got ' + JSON.stringify(actual)));
	},
	typeof: function (name, value, expectedType, message) {
		var t = typeof value;
		if (t !== expectedType) __fail(name, message || ('expected typeof ' + expectedType + ', got ' + t));
	},
	throws: function (name, fn, message) {
		var threw = false;
		try { fn(); } catch (e) { threw = true; }
		if (!threw) __fail(name, message || 'expected function to throw');
	},
	isPromise: function (name, value, message) {
		if (!(value !== null && value !== undefined && typeof value === 'object' && typeof value.then === 'function')) {
			__fail(name, message || ('expected Promise, got ' + __nameOf(value)));
		}
	},
	// assert.resolves: returns the promise so the test body can await it.
	// Fails the named test if the promise rejects or never settles (the
	// never-settle case is caught by the Go runner timeout).
	resolves: function (name, promise, message) {
		assert.isPromise(name, promise, message);
		return promise.then(
			function (v) { return v; },
			function (e) { __fail(name, message || ('expected promise to resolve, rejected with: ' + __errMsg(e))); throw e; }
		);
	},
	rejects: function (name, promiseOrFn, message) {
		var p = (typeof promiseOrFn === 'function') ? Promise.resolve().then(promiseOrFn) : promiseOrFn;
		return Promise.resolve(p).then(
			function (v) { __fail(name, message || ('expected promise to reject, resolved with: ' + JSON.stringify(v))); return v; },
			function () { /* expected */ }
		);
	},
};

// __finishRegistration is invoked by the Go runner immediately after the spec
// body has executed (all test() calls are therefore registered). If no tests
// are pending, __done resolves right away; otherwise it resolves when the
// last pending test settles.
function __finishRegistration() {
	__registrationOpen = false;
	if (__pending <= 0 && __resolveDone !== null) {
		var resolve = __resolveDone;
		__resolveDone = null;
		resolve();
	}
}
