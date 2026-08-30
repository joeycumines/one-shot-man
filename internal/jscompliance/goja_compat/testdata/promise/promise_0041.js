/*---
description: goja compat promise 41
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(41) instanceof Promise, true, 'promise instance 41'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 41');
