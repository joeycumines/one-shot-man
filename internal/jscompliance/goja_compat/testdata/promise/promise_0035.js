/*---
description: goja compat promise 35
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(35) instanceof Promise, true, 'promise instance 35'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 35');
