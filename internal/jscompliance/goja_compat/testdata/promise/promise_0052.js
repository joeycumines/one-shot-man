/*---
description: goja compat promise 52
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(52) instanceof Promise, true, 'promise instance 52'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 52');
