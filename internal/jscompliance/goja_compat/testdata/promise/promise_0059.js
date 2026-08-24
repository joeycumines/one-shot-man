/*---
description: goja compat promise 59
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(59) instanceof Promise, true, 'promise instance 59'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 59');
