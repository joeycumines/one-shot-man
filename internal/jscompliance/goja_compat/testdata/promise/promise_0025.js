/*---
description: goja compat promise 25
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(25) instanceof Promise, true, 'promise instance 25'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 25');
