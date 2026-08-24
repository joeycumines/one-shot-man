/*---
description: goja compat promise 83
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(83) instanceof Promise, true, 'promise instance 83'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 83');
