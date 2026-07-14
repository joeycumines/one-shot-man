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

// Per-test outcome tracking. Tests run SEQUENTIALLY (one at a time, each
// fully settled before the next starts) — the same model as Jest/Mocha. This
// eliminates the class of microtask-interleaving bugs where a failure in one
// test's async continuation could be attributed to another test. assert.*
// mark the currently-running test failed via __fail, which writes to the
// single __activeSink — the failure collector of the test whose body is
// executing RIGHT NOW. The `name` argument to assert.* and __fail is a
// DIAGNOSTIC LABEL only (used in the recorded error message); it is NOT used
// as a lookup key, so a mismatch between the test() name and the assert name
// can never cause a failure to be silently dropped (the bug this redesign
// fixes). Each test sets __activeSink before running its body and clears it
// after recording the outcome.
var __testQueue = []; // [{ name: string, fn: function }]
var __activeSink = null; // { failures: bool, failMsg: string|null } or null

function __fail(name, message) {
	if (__activeSink === null) {
		return; // No active test — defensive; shouldn't happen in well-formed specs.
	}
	if (!__activeSink.failures) {
		__activeSink.failures = true;
		__activeSink.failMsg = String(message);
	}
}

// __runTests processes the test queue SEQUENTIALLY. Each test's body (which
// may be async) runs to full settlement before the next test starts. This
// ensures __activeSink is unambiguous at all times: exactly one test is
// "running" — even when its body suspends at an await, no other test body
// executes until it resumes and completes.
async function __runTests() {
	for (var i = 0; i < __testQueue.length; i++) {
		var entry = __testQueue[i];
		__activeSink = { failures: false, failMsg: null };
		var threw = false;
		var threwMsg = null;
		try {
			await entry.fn();
		} catch (e) {
			threw = true;
			threwMsg = __errMsg(e);
		}
		if (__activeSink.failures) {
			__record(entry.name, false, __activeSink.failMsg);
		} else if (threw) {
			__record(entry.name, false, threwMsg);
		} else {
			__record(entry.name, true, null);
		}
		__activeSink = null;
		__settleOne();
	}
}

// test registers a test. fn may be async (returns a Promise) or sync, and may
// throw. The test is recorded as failed if any assert fails or fn throws/rejects.
// Tests are queued and run sequentially by __runTests (invoked after registration
// closes via __finishRegistration).
function test(name, fn) {
	__pending += 1;
	__testQueue.push({ name: name, fn: fn });
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
	// assert.resolves: returns a promise (it's async) the test body awaits.
	// Fails the named test if the promise rejects or never settles (the
	// never-settle case is caught by the Go runner timeout). async/await.
	resolves: async function (name, promise, message) {
		assert.isPromise(name, promise, message);
		try {
			return await promise;
		} catch (e) {
			__fail(name, message || ('expected promise to resolve, rejected with: ' + __errMsg(e)));
			throw e;
		}
	},
	rejects: async function (name, promiseOrFn, message) {
		// Normalize a fn arg to a promise (async IIFE, .then-free).
		var p = (typeof promiseOrFn === 'function') ? (async function () { return await promiseOrFn(); })() : promiseOrFn;
		try {
			var v = await p;
			__fail(name, message || ('expected promise to reject, resolved with: ' + JSON.stringify(v)));
			return v;
		} catch (e) {
			// expected rejection
		}
	},
};

// __finishRegistration is invoked by the Go runner immediately after the spec
// body has executed (all test() calls are therefore registered). It closes
// registration and kicks off __runTests, which drains the queue sequentially.
// __done resolves when the last pending test settles (via __settleOne).
function __finishRegistration() {
	__registrationOpen = false;
	if (__pending <= 0 && __resolveDone !== null) {
		var resolve = __resolveDone;
		__resolveDone = null;
		resolve();
		return;
	}
	// Run the queued tests sequentially. This is async; __done settles via
	// __settleOne when all tests complete. The promise is intentionally not
	// awaited (it runs as a detached microtask chain).
	__runTests();
}
