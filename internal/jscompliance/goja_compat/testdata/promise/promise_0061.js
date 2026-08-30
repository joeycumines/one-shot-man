/*---
description: goja compat promise 61
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(61) instanceof Promise, true, 'promise instance 61'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 61');
