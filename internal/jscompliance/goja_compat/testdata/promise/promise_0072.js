/*---
description: goja compat promise 72
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(72) instanceof Promise, true, 'promise instance 72'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 72');
