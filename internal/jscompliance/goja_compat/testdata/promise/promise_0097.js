/*---
description: goja compat promise 97
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(97) instanceof Promise, true, 'promise instance 97'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 97');
