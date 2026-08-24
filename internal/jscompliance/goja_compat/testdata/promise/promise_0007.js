/*---
description: goja compat promise 7
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(7) instanceof Promise, true, 'promise instance 7'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 7');
