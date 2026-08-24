/*---
description: goja compat promise 24
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(24) instanceof Promise, true, 'promise instance 24'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 24');
