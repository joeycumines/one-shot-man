/*---
description: goja compat promise 6
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(6) instanceof Promise, true, 'promise instance 6'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 6');
