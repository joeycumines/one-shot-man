/*---
description: goja compat promise 58
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(58) instanceof Promise, true, 'promise instance 58'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 58');
