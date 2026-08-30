/*---
description: goja compat promise 96
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(96) instanceof Promise, true, 'promise instance 96'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 96');
