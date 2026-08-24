/*---
description: goja compat promise 28
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(28) instanceof Promise, true, 'promise instance 28'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 28');
