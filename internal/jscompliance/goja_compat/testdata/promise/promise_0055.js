/*---
description: goja compat promise 55
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(55) instanceof Promise, true, 'promise instance 55'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 55');
