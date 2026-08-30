/*---
description: goja compat promise 65
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(65) instanceof Promise, true, 'promise instance 65'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 65');
