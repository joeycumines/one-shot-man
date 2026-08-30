/*---
description: goja compat promise 53
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(53) instanceof Promise, true, 'promise instance 53'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 53');
