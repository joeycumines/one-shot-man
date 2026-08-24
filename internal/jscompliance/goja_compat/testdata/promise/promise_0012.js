/*---
description: goja compat promise 12
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(12) instanceof Promise, true, 'promise instance 12'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 12');
