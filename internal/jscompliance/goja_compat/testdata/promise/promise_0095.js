/*---
description: goja compat promise 95
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(95) instanceof Promise, true, 'promise instance 95'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 95');
