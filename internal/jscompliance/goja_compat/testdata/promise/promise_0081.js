/*---
description: goja compat promise 81
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(81) instanceof Promise, true, 'promise instance 81'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 81');
