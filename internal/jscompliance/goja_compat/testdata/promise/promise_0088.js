/*---
description: goja compat promise 88
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(88) instanceof Promise, true, 'promise instance 88'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 88');
