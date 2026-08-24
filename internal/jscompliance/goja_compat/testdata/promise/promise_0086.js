/*---
description: goja compat promise 86
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(86) instanceof Promise, true, 'promise instance 86'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 86');
