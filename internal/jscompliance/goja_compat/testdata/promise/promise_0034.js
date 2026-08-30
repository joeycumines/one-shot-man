/*---
description: goja compat promise 34
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(34) instanceof Promise, true, 'promise instance 34'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 34');
