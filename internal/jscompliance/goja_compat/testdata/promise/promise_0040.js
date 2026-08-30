/*---
description: goja compat promise 40
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(40) instanceof Promise, true, 'promise instance 40'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 40');
