/*---
description: goja compat promise 79
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(79) instanceof Promise, true, 'promise instance 79'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 79');
