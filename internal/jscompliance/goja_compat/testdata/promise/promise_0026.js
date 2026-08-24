/*---
description: goja compat promise 26
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(26) instanceof Promise, true, 'promise instance 26'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 26');
