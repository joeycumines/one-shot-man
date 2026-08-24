/*---
description: goja compat promise 32
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(32) instanceof Promise, true, 'promise instance 32'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 32');
