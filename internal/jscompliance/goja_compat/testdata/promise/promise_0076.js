/*---
description: goja compat promise 76
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(76) instanceof Promise, true, 'promise instance 76'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 76');
