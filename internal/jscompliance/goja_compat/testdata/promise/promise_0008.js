/*---
description: goja compat promise 8
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(8) instanceof Promise, true, 'promise instance 8'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 8');
