/*---
description: goja compat promise 93
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(93) instanceof Promise, true, 'promise instance 93'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 93');
