/*---
description: goja compat promise 1
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(1) instanceof Promise, true, 'promise instance 1'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 1');
