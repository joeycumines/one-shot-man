/*---
description: goja compat promise 90
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(90) instanceof Promise, true, 'promise instance 90'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 90');
