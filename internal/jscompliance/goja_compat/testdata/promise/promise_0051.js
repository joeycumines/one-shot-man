/*---
description: goja compat promise 51
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(51) instanceof Promise, true, 'promise instance 51'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 51');
