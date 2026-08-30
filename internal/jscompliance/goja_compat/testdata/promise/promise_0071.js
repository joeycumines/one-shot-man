/*---
description: goja compat promise 71
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(71) instanceof Promise, true, 'promise instance 71'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 71');
