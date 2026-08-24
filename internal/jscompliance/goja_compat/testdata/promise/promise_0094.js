/*---
description: goja compat promise 94
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(94) instanceof Promise, true, 'promise instance 94'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 94');
