/*---
description: goja compat promise 36
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(36) instanceof Promise, true, 'promise instance 36'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 36');
