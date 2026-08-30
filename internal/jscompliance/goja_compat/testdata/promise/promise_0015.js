/*---
description: goja compat promise 15
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(15) instanceof Promise, true, 'promise instance 15'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 15');
