/*---
description: goja compat promise 38
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(38) instanceof Promise, true, 'promise instance 38'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 38');
