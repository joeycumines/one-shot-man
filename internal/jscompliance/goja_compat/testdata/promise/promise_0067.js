/*---
description: goja compat promise 67
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(67) instanceof Promise, true, 'promise instance 67'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 67');
