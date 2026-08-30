/*---
description: goja compat promise 4
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(4) instanceof Promise, true, 'promise instance 4'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 4');
