/*---
description: goja compat promise 39
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(39) instanceof Promise, true, 'promise instance 39'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 39');
