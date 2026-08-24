/*---
description: goja compat promise 43
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(43) instanceof Promise, true, 'promise instance 43'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 43');
