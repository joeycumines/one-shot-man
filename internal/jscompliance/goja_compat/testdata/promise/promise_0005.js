/*---
description: goja compat promise 5
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(5) instanceof Promise, true, 'promise instance 5'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 5');
