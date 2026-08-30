/*---
description: goja compat promise 16
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(16) instanceof Promise, true, 'promise instance 16'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 16');
