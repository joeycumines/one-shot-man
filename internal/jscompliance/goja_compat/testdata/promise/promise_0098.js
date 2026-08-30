/*---
description: goja compat promise 98
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(98) instanceof Promise, true, 'promise instance 98'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 98');
