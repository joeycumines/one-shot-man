/*---
description: goja compat promise 66
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(66) instanceof Promise, true, 'promise instance 66'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 66');
