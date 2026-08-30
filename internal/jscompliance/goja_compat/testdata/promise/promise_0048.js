/*---
description: goja compat promise 48
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(48) instanceof Promise, true, 'promise instance 48'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 48');
