/*---
description: goja compat promise 60
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(60) instanceof Promise, true, 'promise instance 60'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 60');
