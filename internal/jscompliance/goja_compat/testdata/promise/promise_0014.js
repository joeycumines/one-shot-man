/*---
description: goja compat promise 14
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(14) instanceof Promise, true, 'promise instance 14'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 14');
