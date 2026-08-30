/*---
description: goja compat promise 80
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(80) instanceof Promise, true, 'promise instance 80'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 80');
