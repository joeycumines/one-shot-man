/*---
description: goja compat promise 92
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(92) instanceof Promise, true, 'promise instance 92'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 92');
