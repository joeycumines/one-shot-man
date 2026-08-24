/*---
description: goja compat promise 21
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(21) instanceof Promise, true, 'promise instance 21'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 21');
